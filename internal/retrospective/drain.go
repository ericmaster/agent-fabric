// Spec: docs/specs/task-retrospective.md
package retrospective

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Drain worker budgets and pacing.
const (
	// DrainRecheckPasses bounds how many times one worker re-scans for jobs
	// that arrived while it held the drain lock.
	DrainRecheckPasses = 3
	// DrainRecheckDelay paces the post-pass re-scans.
	DrainRecheckDelay = 200 * time.Millisecond
	// DrainMaxBackfillChildren bounds session-history fetches per job.
	DrainMaxBackfillChildren = 64
	// DrainJobSchemaVersion is the only accepted drain job schema.
	DrainJobSchemaVersion = 1
)

// Drain job statuses.
const (
	JobPending = "pending"
	JobRunning = "running"
	JobDone    = "done"
	JobFailed  = "failed"
	JobSkipped = "skipped"
)

// conclusivelySettled lists receipt statuses that terminated an incident.
var conclusivelySettled = map[string]bool{
	StatusRepaired: true, StatusSuggested: true, StatusConflict: true,
	StatusExhausted: true, StatusSuppressed: true, StatusSubmitted: true,
}

// DrainOptions are the drain worker dependencies. SourceRoot supports the
// dispatch copy refresh (the CLI passes its packaged source root).
// Spec: docs/specs/task-retrospective.md via RunDrain
type DrainOptions struct {
	Config     *Config
	ConfigPath string
	SourceRoot string
	// EnvKey is the host Jev key, read from the detached process environment.
	EnvKey string
	// Now stamps job persistence (injectable in tests).
	Now func() time.Time
}

// DrainSummary is the drain worker outcome.
// Spec: docs/specs/task-retrospective.md via RunDrain
type DrainSummary struct {
	Processed int
	Completed int
	Failed    int
	Skipped   int
	Paused    bool
	LockBusy  bool
}

// RunDrain is the detached drain worker: process pending jobs under one
// single-host lock, resume unfinished work, then exit. There is no daemon
// and no always-on service.
// Spec: docs/specs/task-retrospective.md
func RunDrain(ctx context.Context, opts DrainOptions) (*DrainSummary, error) {
	cfg := opts.Config
	if cfg == nil {
		cfg = DefaultConfig()
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	stateDir := cfg.StateDir
	if stateDir == "" {
		resolved, err := DefaultStateDir()
		if err != nil {
			return nil, err
		}
		stateDir = resolved
	}
	store, err := OpenStore(stateDir)
	if err != nil {
		return nil, err
	}
	unlock, err := store.TryLockAt(DrainLockName)
	if err != nil {
		if errors.Is(err, ErrLockBusy) {
			// Another worker on this host owns the loop; it picks up late
			// jobs on its bounded re-check.
			return &DrainSummary{LockBusy: true}, nil
		}
		return nil, err
	}
	defer unlock()
	if _, err := Enabled(opts.ConfigPath); err != nil {
		return nil, err
	}
	summary := &DrainSummary{}
	clock := opts.Now
	if clock == nil {
		clock = func() time.Time { return time.Now().UTC() }
	}
	for pass := 0; pass < DrainRecheckPasses; pass++ {
		if ctx.Err() != nil {
			return summary, ctx.Err()
		}
		budget := cfg.DrainMaxJobs
		jobs, err := store.ListDrainJobs()
		if err != nil {
			return summary, err
		}
		for _, job := range jobs {
			if ctx.Err() != nil {
				return summary, ctx.Err()
			}
			if budget <= 0 {
				break
			}
			if job.Status != JobPending && job.Status != JobRunning {
				continue
			}
			budget--
			summary.Processed++
			outcome := processDrainJob(ctx, opts, store, clock, *job)
			switch outcome {
			case JobDone, JobSkipped:
				summary.Completed++
			case JobFailed:
				summary.Failed++
			case JobPending:
				// The global opt-out paused the worker: stop the whole loop
				// (the remaining jobs stay inspectable and resume later).
				summary.Paused = true
				return summary, nil
			}
		}
		if !hasPendingJobs(store, jobs) {
			return summary, nil
		}
		select {
		case <-ctx.Done():
			return summary, ctx.Err()
		case <-time.After(DrainRecheckDelay):
		}
	}
	return summary, nil
}

// hasPendingJobs reports whether any listed job still needs the worker.
// Spec: docs/specs/task-retrospective.md via RunDrain
func hasPendingJobs(jobsStore *Store, jobs []*DrainJob) bool {
	for _, job := range jobs {
		if job.Status == JobPending || job.Status == JobRunning {
			return true
		}
	}
	return false
}

// persistTriggerJob writes or refreshes the drain job of a triggered
// session: missing or terminal → pending; pending/running → kept (a later
// trigger only updates the server hint). It returns whether the job moved to
// pending under this call.
// Spec: docs/specs/task-retrospective.md via IngestEnvelope
func persistTriggerJob(opts IngestOptions, spool *Spool) (*DrainJob, bool, error) {
	store := opts.Store
	clock := opts.Now
	if clock == nil {
		clock = func() time.Time { return time.Now().UTC() }
	}
	stamp := clock().UTC().Format(time.RFC3339)
	existing, err := store.ReadDrainJob(spool.SessionID)
	if errors.Is(err, os.ErrNotExist) {
		job := &DrainJob{
			Schema:    DrainJobSchemaVersion,
			SessionID: spool.SessionID,
			ServerURL: firstNonEmpty(spool.ServerURL, opts.Config.DispatchServer),
			Status:    JobPending,
			CreatedAt: stamp,
		}
		return job, true, store.WriteDrainJob(job)
	}
	if err != nil {
		return nil, false, err
	}
	changed := false
	if existing.ServerURL == "" {
		if spool.ServerURL != "" {
			existing.ServerURL = spool.ServerURL
			changed = true
		} else if opts.Config.DispatchServer != "" {
			existing.ServerURL = opts.Config.DispatchServer
			changed = true
		}
	}
	if existing.Status == JobDone || existing.Status == JobFailed || existing.Status == JobSkipped {
		// A new terminal trigger re-arms the completed job; the incident
		// dedup map persists, so an identical turn never re-dispatches.
		existing.Status = JobPending
		existing.ReportPath = ""
		existing.ReceiptStatus = ""
		existing.Error = ""
		changed = true
	}
	if !changed {
		return existing, false, nil
	}
	existing.UpdatedAt = stamp
	if err := store.WriteDrainJob(existing); err != nil {
		return nil, false, err
	}
	return existing, true, nil
}

// processDrainJob reconciles, assembles, analyzes and dispatches one job,
// returning the resulting job status.
// Spec: docs/specs/task-retrospective.md via RunDrain
func processDrainJob(ctx context.Context, opts DrainOptions, store *Store, clock func() time.Time, job DrainJob) string {
	cfg := opts.Config
	originalJobURL := job.ServerURL
	if job.ServerURL != "" && !collectionServerURL(job.ServerURL, cfg) {
		job.ServerURL = ""
	}
	now := clock().UTC().Format(time.RFC3339)
	save := func(status, message string) string {
		job.Status = status
		if message != "" {
			job.Error = message
		} else {
			job.Error = ""
		}
		job.UpdatedAt = now
		if err := store.WriteDrainJob(&job); err != nil {
			return JobFailed
		}
		if status == JobFailed {
			if err := recordPipelineFailure(ctx, opts, store, &job); err != nil {
				job.Error += "; maintenance intake could not be retained"
			}
		}
		if err := store.WriteDrainJob(&job); err != nil {
			return JobFailed
		}
		return job.Status
	}
	job.Attempts++
	job.Status = JobRunning
	job.UpdatedAt = now
	if err := store.WriteDrainJob(&job); err != nil {
		return JobFailed
	}
	enabled, err := Enabled(opts.ConfigPath)
	if err != nil {
		return save(JobFailed, "read retrospective.enabled: "+err.Error())
	}
	if !enabled {
		// Global opt-out: paused work stays inspectable and resumes under a
		// later trigger or restart; no network and no dispatch runs.
		return save(JobPending, "paused: retrospective is disabled")
	}
	if _, err := os.Stat(filepath.Join(store.Dir, "origins", job.SessionID)); err == nil {
		return save(JobSkipped, "learning-origin session")
	}
	spool, err := loadSpool(store, job.SessionID)
	if err != nil {
		return save(JobFailed, "load spool: "+err.Error())
	}
	if spool.Suppressed {
		return save(JobSkipped, "suppressed observation session")
	}
	if spool.SessionID != job.SessionID {
		return save(JobFailed, fmt.Sprintf("staged spool belongs to %q, job says %q", spool.SessionID, job.SessionID))
	}
	invalidSpoolURL := spool.ServerURL != "" && !collectionServerURL(spool.ServerURL, cfg)
	if invalidSpoolURL {
		unlock, err := spoolLock(store, spool.SessionID)
		if err != nil {
			return save(JobFailed, "cannot sanitize invalid persisted server URL")
		}
		latest, err := loadSpool(store, spool.SessionID)
		if err == nil {
			latest.ServerURL = ""
			latest.appendGap(Gap{Kind: "partial_stream", Detail: "persisted observation server URL rejected; no backfill request sent"})
			err = writeSpoolBounded(cfg, store, latest)
			spool = latest
		}
		unlock()
		if err != nil {
			return save(JobFailed, "cannot sanitize invalid persisted server URL")
		}
	}
	var reconcileGaps []Gap
	var historySnapshot *Spool
	if !invalidSpoolURL {
		reconcileGaps, historySnapshot = reconcileServerSnapshot(ctx, cfg, store, spool, originalJobURL)
		unlock, err := spoolLock(store, spool.SessionID)
		if err != nil {
			return save(JobFailed, "cannot reload reconciled startup evidence")
		}
		latest, err := loadSpool(store, spool.SessionID)
		unlock()
		if err != nil {
			return save(JobFailed, "cannot load reconciled startup evidence")
		}
		spool = latest
	}
	if historySnapshot != nil {
		// Use the independently decoded native history order for episode
		// boundaries; backfill arrival order is not execution order. The
		// original spool, loss counters and reports remain untouched.
		spool = historySnapshot
	}
	trace := assembleTrace(store, spool, spool.SessionID)
	trace.Gaps = append(trace.Gaps, reconcileGaps...)
	episodes := executionEpisodes(spool, trace)
	if len(episodes) == 0 {
		if len(trace.Messages) == 0 && len(trace.Calls) == 0 && len(trace.Gaps) == 0 {
			return save(JobSkipped, "no observed execution records")
		}
		// Missing intake/idle is itself a retained failure, never a fabricated
		// episode and never sent to the provider as a whole-session fallback.
		path := filepath.Join(store.Dir, "reports", drainReportName("missing-episode-"+spool.SessionID))
		diagnostic := &Report{SignalReport: SignalReport{SchemaVersion: ReportSchemaVersion, Trace: ReportTrace{Identities: trace.Identities}}, Status: ReportStatusIncomplete, IncompleteReasons: []string{"missing_prompt_idle_boundary"}}
		if err := WriteReport(path, diagnostic); err != nil {
			return save(JobFailed, "cannot retain episode boundary diagnostic")
		}
		job.ReportPath = path
		return save(JobFailed, "collection startup incomplete or idle missing; full report: "+path)
	}
	// A failed older episode never grows to include later prompts. Each closed
	// episode has its own report, input digest and dispatch dedup identity.
	var processingError string
	for _, episode := range episodes {
		if err := ctx.Err(); err != nil {
			return save(JobFailed, "episode processing cancelled")
		}
		raw, err := json.Marshal(episode)
		if err != nil {
			return save(JobFailed, "marshal episode: "+err.Error())
		}
		digest := sha256hex(raw)
		id := episode.Identities.TaskID
		if job.EpisodeDigests[id] == digest {
			continue
		}
		reportPath, receiptStatus, err := analyzeDrainTrace(ctx, opts, store, enabled, episode, raw, choiceClientFor(cfg, opts.EnvKey))
		job.ReportPath, job.ReceiptStatus = reportPath, receiptStatus
		if err != nil {
			processingError = err.Error()
			continue
		}
		if job.EpisodeDigests == nil {
			job.EpisodeDigests = map[string]string{}
		}
		if receiptStatus == "" || conclusivelySettled[receiptStatus] {
			job.EpisodeDigests[id] = digest
		}
		if err := store.WriteDrainJob(&job); err != nil {
			return JobFailed
		}
	}
	if processingError != "" {
		return save(JobFailed, processingError)
	}
	return save(JobDone, "")
}

// analyzeDrainTrace retains an episode report even when interpretation abstains.
// Observation closure is not accepted task success and does not move the legacy
// accepted-success boundary. The ordinary independent repair gates still apply.
func analyzeDrainTrace(ctx context.Context, opts DrainOptions, store *Store, enabled bool, trace *Trace, raw []byte, client ChoiceClient) (string, string, error) {
	cfg := opts.Config
	report, err := Analyze(ctx, cfg, trace, raw, client)
	if err != nil {
		return "", "", fmt.Errorf("analyze episode: %w", err)
	}
	taskID := report.Trace.Identities.TaskID
	if taskID == "" {
		return "", "", errors.New("analyze produced no episode identity")
	}
	// The optional trusted coordinator packet is the only external
	// dispatch-evidence producer of the loop; the drain itself never
	// fabricates findings (analysis does not supply them).
	packet, packetErr := coordinatorFindings(store, taskID)
	if packetErr != nil {
		return "", "", fmt.Errorf("coordinator packet: %w", packetErr)
	}
	if len(packet) > 0 {
		report.Findings = packet
	}
	reportPath := filepath.Join(store.Dir, "reports", drainReportName(taskID))
	alreadySaved := false
	if isExecutionEpisode(trace) {
		// Later observed coverage may change the same episode's input. Keep
		// its earlier diagnostic bytes instead of rewriting historical evidence.
		stamp := report.GeneratedAt
		report.GeneratedAt = ""
		version, _ := json.Marshal(report)
		report.GeneratedAt = stamp
		reportPath = filepath.Join(store.Dir, "reports", drainReportName(taskID+"-"+sha256hex(version)))
		if existing, _, err := LoadReport(reportPath); err == nil {
			existing.GeneratedAt = ""
			body, _ := json.Marshal(existing)
			if !bytes.Equal(body, version) {
				return "", "", errors.New("episode report version conflict")
			}
			alreadySaved = true
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", "", fmt.Errorf("read episode report: %w", err)
		}
	}
	if !alreadySaved {
		if err := WriteReport(reportPath, report); err != nil {
			return "", "", fmt.Errorf("write report: %w", err)
		}
	}
	if report.Status == ReportStatusIncomplete {
		for _, gap := range report.Trace.Gaps {
			if gap.Kind == "missing_startup" && (gap.Scope == "current" || gap.Scope == "unknown") {
				return reportPath, "", fmt.Errorf("collection startup incomplete; full report: %s", reportPath)
			}
		}
		return reportPath, "", fmt.Errorf("episode evidence incomplete; full report: %s", reportPath)
	}
	for _, row := range report.Interpretation.Rows {
		if row.Status == "unavailable" || row.Reason == "oversized_packet" || row.Reason == "invalid_answer_or_citation" || row.Reason == "malformed_provider_response" || row.Reason == "invalid_packet" {
			return reportPath, "", fmt.Errorf("episode interpretation unavailable; full report: %s", reportPath)
		}
	}
	receiptStatus := ""
	if ineligible(report) == "" && report.Status == ReportStatusComplete {
		receipt, err := drainDispatch(ctx, opts, store, cfg, enabled, reportPath, taskID)
		if err != nil {
			return reportPath, "", fmt.Errorf("dispatch: %w", err)
		}
		receiptStatus = receipt.Status
	}
	return reportPath, receiptStatus, nil
}

// drainDispatch wraps Dispatch with the drain's task-level dedup: an
// incident for a task identity is never attempted twice, so an ambiguous or
// pending resolution fails the job for manual reconciliation instead of
// risking a duplicate visible root.
// Spec: docs/specs/task-retrospective.md via processDrainJob
func drainDispatch(ctx context.Context, opts DrainOptions, store *Store, cfg *Config, enabled bool, reportPath, taskID string) (*Receipt, error) {
	receipts, err := store.ListReceipts()
	if err != nil {
		return nil, err
	}
	for _, receipt := range receipts {
		if receipt.TaskID != taskID {
			continue
		}
		if conclusivelySettled[receipt.Status] {
			return receipt, nil
		}
		if receipt.Status == StatusQueued && !receipt.CreateAttempted {
			// Recoverable queue: the correcting-session capability was
			// missing earlier; a fresh dispatch now performs the first root
			// creation (no duplicate root exists).
			break
		}
		return nil, fmt.Errorf("previous dispatch for task %s is %s; reconcile receipts before retrying (no duplicate root)", taskID, receipt.Status)
	}
	client := OpenSessionClient(cfg)
	return Dispatch(ctx, DispatchOptions{
		Enabled:       enabled,
		Config:        cfg,
		ReportPath:    reportPath,
		Client:        client,
		PromptProbe:   client.messageProbe(),
		Now:           opts.Now,
		ManifestPaths: DispatchManifests(cfg.CanonicalRoots, opts.SourceRoot),
		SettingsPath:  opts.ConfigPath,
	})
}

// jobsPath returns the persisted job path of one session.
// Spec: docs/specs/task-retrospective.md via Store
func jobsPath(store *Store, sessionID string) string {
	return filepath.Join(store.Dir, JobsDirName, spoolKey(sessionID)+".json")
}

// CoordinatorDir is the optional trusted coordinator packet directory.
const CoordinatorDir = "coordinator"

// coordinatorFindings reads the optional trusted coordinator packet for one
// task identity; a missing packet is empty (the correcting root gets the
// intake locator and performs its own verified review). A malformed packet
// fails closed.
// Spec: docs/specs/task-retrospective.md via processDrainJob
func coordinatorFindings(store *Store, taskID string) ([]Finding, error) {
	path := filepath.Join(store.Dir, CoordinatorDir, spoolKey(taskID)+".json")
	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var packet struct {
		Schema   int       `json:"schema"`
		Findings []Finding `json:"findings"`
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&packet); err != nil {
		return nil, fmt.Errorf("decode coordinator packet %s: %w", path, err)
	}
	if packet.Schema != EnvelopeSchemaVersion {
		return nil, fmt.Errorf("coordinator packet %s: unsupported schema %d", path, packet.Schema)
	}
	return packet.Findings, nil
}

// drainReportName is one stable per-task report file name; WriteReport
// replaces it atomically. Task ids are always hashed to 65 chars by spoolKey
// (colon syntax never reaches a filename), so no secondary truncation is
// applied here.
// Spec: docs/specs/task-retrospective.md via processDrainJob
func drainReportName(taskID string) string {
	return "drain-" + spoolKey(taskID) + ".json"
}

// spoolIntervalEnd returns the highest staged sequence of one spool.
// Spec: docs/specs/task-retrospective.md via processDrainJob
func spoolIntervalEnd(spool *Spool) int {
	end := 0
	for _, part := range spool.Parts {
		if part.Seq > end {
			end = part.Seq
		}
	}
	for _, event := range spool.Events {
		if event.Seq > end {
			end = event.Seq
		}
	}
	return end
}

// acceptBoundary advances the spool's accepted-success boundary so the next
// triggered interval starts strictly after the analyzed turn.
// Spec: docs/specs/task-retrospective.md via processDrainJob
func acceptBoundary(store *Store, cfg *Config, sessionID string, afterSeq int) error {
	unlock, err := spoolLock(store, sessionID)
	if err != nil {
		return err
	}
	defer unlock()
	spool, err := loadSpool(store, sessionID)
	if err != nil {
		return err
	}
	if spool.AcceptedAfterSeq < afterSeq {
		spool.AcceptedAfterSeq = afterSeq
	}
	return writeSpoolBounded(cfg, store, spool)
}

// choiceClientFor builds the Jev evaluator of the detached worker; nil means
// evaluator-unavailable (the analysis proceeds with local abstention).
// Spec: docs/specs/task-retrospective.md via processDrainJob
func choiceClientFor(cfg *Config, apiKey string) ChoiceClient {
	if apiKey == "" || cfg.Endpoint == "" {
		return nil
	}
	return NewJevClient(cfg.Endpoint, apiKey, cfg)
}

// reconcileServer merges the terminal server snapshot (message history of the
// owner and its descendant tree) into the staged spools and returns gaps. A
// capability miss (404/501) leaves the streamed coverage intact; every other
// transport failure is a gap, never invented coverage. The owner's history is
// fetched exactly once per job and handed to the backfill; only a failed
// probe falls back to a diagnostic gap without any fetch.
// Spec: docs/specs/task-retrospective.md via processDrainJob
func reconcileServer(ctx context.Context, cfg *Config, store *Store, spool *Spool, serverURL string) []Gap {
	gaps, _ := reconcileServerSnapshot(ctx, cfg, store, spool, serverURL)
	return gaps
}

func reconcileServerSnapshot(ctx context.Context, cfg *Config, store *Store, spool *Spool, serverURL string) ([]Gap, *Spool) {
	base := firstNonEmpty(serverURL, cfg.DispatchServer, spool.ServerURL)
	if base == "" {
		return nil, nil
	}
	for _, location := range []string{base, spool.ServerURL, serverURL} {
		if location != "" && (!collectionServerURL(location, cfg) || !trustedServerURL(location, cfg.DispatchServer)) {
			return []Gap{{Kind: "partial_stream", Detail: "backfill server origin is not the configured trusted dispatch origin; no credentials or request sent"}}, nil
		}
	}
	client := OpenSessionClient(cfg)
	client.BaseURL = base
	var rows []SessionMessageRow
	var err error
	if client.messageGeneration() {
		rows, err = client.nativeHistoryMode(ctx, spool.SessionID, true)
	} else {
		rows, err = client.Messages(ctx, spool.SessionID)
	}
	if err != nil {
		if errors.Is(err, ErrCapability) {
			return nil, nil
		}
		return []Gap{{Kind: "partial_stream", Detail: "message reconcile failed: " + boundedGapDetail(err.Error())}}, nil
	}
	var gaps []Gap
	budget := DrainMaxBackfillChildren
	gaps = append(gaps, backfillSession(ctx, client, store, cfg, spool.SessionID, &budget, rows)...)
	return gaps, nativeHistorySnapshot(cfg, spool, rows)
}

// backfillSession merges one session's fetched message history into its
// staged spool (load+stage+write under that session's spool lock, the same
// lock the ingest path holds) and then walks its observed descendants. Each
// lock is held only for its own spool's load+stage+write and released before
// descending: a cyclic parent topology (each row naming the other its child)
// can never nest two spool locks of the same session in one walk, and the
// visited set plus the MaxDescendantDepth budget terminate the walk even on
// adversarial identity maps. Lock order is always drain.lock → one spool
// lock at a time (the ingest path takes only the spool lock), so a
// concurrent plugin flush cannot lose staged records to this backfill and no
// deadlock is possible.
// Spec: docs/specs/task-retrospective.md via reconcileServer
func backfillSession(ctx context.Context, client *HTTPSessionClient, store *Store, cfg *Config, sessionID string, budget *int, rows []SessionMessageRow) []Gap {
	seen := map[string]bool{}
	return backfillOne(ctx, client, store, cfg, sessionID, budget, rows, seen, 0)
}

// backfillOne is the recursion of backfillSession: visited-set plus depth
// budget guarding the observed-parent walk, one spool lock per level,
// released before the children are descended into.
// Spec: docs/specs/task-retrospective.md via backfillSession
func backfillOne(ctx context.Context, client *HTTPSessionClient, store *Store, cfg *Config, sessionID string, budget *int, rows []SessionMessageRow, seen map[string]bool, depth int) []Gap {
	if seen[sessionID] {
		return nil
	}
	seen[sessionID] = true
	if depth >= MaxDescendantDepth {
		return []Gap{{Kind: "partial_stream", Detail: "descendant depth budget reached at " + sessionID}}
	}
	childIDs, gaps := func() ([]string, []Gap) {
		unlock, err := spoolLock(store, sessionID)
		if err != nil {
			return nil, []Gap{{Kind: "partial_stream", Detail: "spool lock " + sessionID + ": " + boundedGapDetail(err.Error())}}
		}
		defer unlock()
		return backfillLoadStage(ctx, client, store, cfg, sessionID, rows)
	}()
	if gaps != nil {
		return gaps
	}
	if *budget <= 0 {
		return []Gap{{Kind: "partial_stream", Detail: "child reconcile cap reached before " + sessionID}}
	}
	*budget--
	for _, id := range childIDs {
		// The parent lock is released before descending: nested cycles can
		// only meet an already-visited session (skipped) or the depth
		// budget, never a second self-lock of one session in one walk.
		if *budget < 0 {
			return append(gaps, Gap{Kind: "partial_stream", Detail: "child reconcile cap reached before " + id})
		}
		gaps = append(gaps, backfillOne(ctx, client, store, cfg, id, budget, nil, seen, depth+1)...)
	}
	return gaps
}

// backfillLoadStage loads one spool, stages its fetched history, writes it
// back, and returns the observed child ids. The caller holds that session's
// spool lock across this call (via a defer-unlocking closure) and releases
// it before descending.
// Spec: docs/specs/task-retrospective.md via backfillOne
func backfillLoadStage(ctx context.Context, client *HTTPSessionClient, store *Store, cfg *Config, sessionID string, rows []SessionMessageRow) ([]string, []Gap) {
	spool, err := loadSpool(store, sessionID)
	if err != nil {
		return nil, []Gap{{Kind: "partial_stream", Detail: "spool " + sessionID + ": " + boundedGapDetail(err.Error())}}
	}
	if spool.ServerURL != "" && (!collectionServerURL(spool.ServerURL, cfg) || !trustedServerURL(spool.ServerURL, cfg.DispatchServer)) {
		gap := Gap{Kind: "partial_stream", Detail: "persisted descendant/server origin rejected; no backfill request sent"}
		if !collectionServerURL(spool.ServerURL, cfg) {
			spool.ServerURL = ""
			spool.appendGap(gap)
			if err := writeSpoolBounded(cfg, store, spool); err != nil {
				return nil, []Gap{{Kind: "partial_stream", Detail: "cannot sanitize persisted descendant server URL"}}
			}
		}
		return nil, []Gap{gap}
	}
	if rows == nil {
		fetched, err := client.Messages(ctx, sessionID)
		if err != nil {
			if errors.Is(err, ErrCapability) {
				return nil, nil
			}
			return nil, []Gap{{Kind: "partial_stream", Detail: "message reconcile failed: " + sessionID + ": " + boundedGapDetail(err.Error())}}
		}
		rows = fetched
	}
	records := historyRecords(spool, rows)
	if len(records) > EnvelopeMaxRecords {
		records = records[:EnvelopeMaxRecords]
		spool.appendGap(Gap{Kind: "partial_stream", Detail: "history record cap exceeded"})
	}
	redactor, redactorErr := NewRedactor(cfg)
	if redactorErr != nil {
		return nil, []Gap{{Kind: "partial_stream", Detail: "reconcile redactor: " + boundedGapDetail(redactorErr.Error())}}
	}
	stageRecords(redactor, capRecords(records), spool)
	if spool.ServerURL == "" {
		spool.ServerURL = firstNonEmpty(client.BaseURL, spool.ServerURL)
	}
	if err := writeSpoolBounded(cfg, store, spool); err != nil {
		return nil, []Gap{{Kind: "partial_stream", Detail: "spool write failed: " + sessionID + ": " + boundedGapDetail(err.Error())}}
	}
	var childIDs []string
	for _, id := range sortedSessionIDs(spool) {
		if id == sessionID || spool.Sessions[id].ParentID != sessionID {
			continue
		}
		childIDs = append(childIDs, id)
	}
	return childIDs, nil
}

// historyRecords uses the same validated converter for merge and ordered replay.
func historyRecords(spool *Spool, rows []SessionMessageRow) []EnvelopeRecord {
	var records []EnvelopeRecord
	for _, row := range rows {
		if row.Control != "" {
			// Remove only the old native adapter's exact empty control-as-text
			// projection, confirmed by this trusted, validated history snapshot.
			if message := spool.Messages[row.Info.ID]; message != nil && message.Role == row.Control {
				if part := spool.Parts[row.Info.ID]; part != nil && part.PartType == "text" && part.Text == "" && part.MessageID == row.Info.ID {
					if part.Tool != "" || part.CallID != "" || part.Input != "" || part.Output != "" || part.Error != "" {
						spool.appendGap(Gap{Kind: "partial_stream", Detail: "native control projection has unexpected tool payload; evidence retained"})
						continue
					}
					delete(spool.Parts, row.Info.ID)
					delete(spool.Messages, row.Info.ID)
				} else {
					spool.appendGap(Gap{Kind: "partial_stream", Detail: "native control projection has unverified payload; evidence retained"})
				}
			}
		}
		if row.Native && row.Info.Role != "" {
			if part := spool.Parts[row.Info.ID]; part != nil && part.PartType == "text" && part.MessageID == row.Info.ID && part.Tool == "" && part.CallID == "" && part.Input == "" && part.Output == "" && part.Error == "" {
				// The old converter emitted an extra empty assistant text for
				// every flat row. It is not a genuine text/content ordinal.
				if part.Text == "" {
					delete(spool.Parts, row.Info.ID)
				} else if row.Info.Role == "user" && len(row.Parts) == 1 {
					if original := spool.Parts[row.Parts[0].ID]; original != nil && original.Text == part.Text && row.Parts[0].Text == part.Text {
						delete(spool.Parts, row.Info.ID)
					}
				}
			}
		}
		if row.Info.ID != "" && row.Info.Role != "" {
			records = append(records, EnvelopeRecord{Kind: "message", SessionID: row.Info.SessionID, RecordID: row.Info.ID, Role: row.Info.Role})
		}
		records = append(records, row.Events...)
		for i := range row.Parts {
			records = append(records, partEnvelopeRecord(&row.Parts[i]))
		}
	}
	return records
}

// boundedGapDetail clips one gap detail.
// Spec: docs/specs/task-retrospective.md via reconcileServer
func boundedGapDetail(detail string) string {
	if len(detail) > EnvelopeErrorDetailBytes {
		return detail[:EnvelopeErrorDetailBytes]
	}
	return detail
}

// partEnvelopeRecord converts one server-history part row into the envelope
// record shape; tool state mapping stays canonical (pending/running →
// pending, completed/error terminal).
// Spec: docs/specs/task-retrospective.md via reconcileServer
func partEnvelopeRecord(row *SessionPartRow) EnvelopeRecord {
	record := EnvelopeRecord{
		Kind:      "part",
		SessionID: row.SessionID,
		RecordID:  row.ID,
		MessageID: row.MessageID,
		PartType:  row.Type,
		Synthetic: row.Synthetic,
		NonFinal:  row.NonFinal,
		Tool:      row.Tool,
		CallID:    row.CallID,
	}
	if row.Type == "text" || row.Type == "synthetic" {
		record.Text = row.Text
	}
	if row.Type == "tool" {
		state := SessionToolState{}
		if err := json.Unmarshal(row.State, &state); err == nil {
			switch state.Status {
			case "", "pending", "running":
				record.ToolStatus = "pending"
			case "completed", "error":
				record.ToolStatus = state.Status
			default:
				record.ToolStatus = "pending"
			}
			if len(state.Input) > 0 && strings.TrimSpace(string(state.Input)) != "{}" {
				record.Input = limitRecordBytes(string(state.Input))
				if len(record.Input) == EnvelopeRecordMaxBytes && len(state.Input) > EnvelopeRecordMaxBytes {
					record.Detail = "truncated input reconciled at the record cap"
				}
			}
			if state.Output != "" {
				record.Output = limitRecordBytes(state.Output)
				if len(record.Output) == EnvelopeRecordMaxBytes && len(state.Output) > EnvelopeRecordMaxBytes {
					record.Detail = "truncated output reconciled at the record cap"
				}
			}
			if name := rawErrorName(state.Error); name != "" {
				record.Error = boundedGapDetail(name)
			}
		}
	}
	return record
}

// limitRecordBytes clips a string to the record cap, backed off to the
// nearest rune start (same discipline as clipRuneBoundary).
// Spec: docs/specs/task-retrospective.md via partEnvelopeRecord
func limitRecordBytes(value string) string {
	return clipRuneBoundary(value, EnvelopeRecordMaxBytes)
}

// rawErrorName extracts a bounded error name/text from a raw error value.
// Spec: docs/specs/task-retrospective.md via partEnvelopeRecord
func rawErrorName(raw json.RawMessage) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return ""
	}
	var named struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &named); err == nil && named.Name != "" {
		return named.Name
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil && text != "" {
		return boundedGapDetail(text)
	}
	return boundedGapDetail(trimmed)
}

// capRecords bounds reconciled record fields (same policy as the live stream).
// Spec: docs/specs/task-retrospective.md via reconcileServer
func capRecords(records []EnvelopeRecord) []EnvelopeRecord {
	out := make([]EnvelopeRecord, 0, len(records))
	for i := range records {
		record := records[i]
		truncateRecord(&record)
		out = append(out, record)
	}
	return out
}

// assembleTrace converts one staged spool into the trace schema v1 document
// of that session, pulling linked descendants from their own spools.
// Spec: docs/specs/task-retrospective.md
func assembleTrace(store *Store, spool *Spool, sessionID string) *Trace {
	trace := &Trace{
		SchemaVersion: TraceSchemaVersion,
		Identities:    Identities{RootSessionID: sessionID, SessionID: sessionID},
	}
	directory := "unknown"
	if session := spool.Sessions[sessionID]; session != nil && session.Directory != "" {
		directory = session.Directory
	}
	trace.Identities.ExecutionRoot = directory

	ordered := orderedParts(spool)
	// Native durable synthetic input is material context, not an assistant
	// action or user turn. Keep its staged ordering and source-specific kind.
	for _, staged := range ordered {
		part := staged.record
		if part.PartType != "synthetic" {
			continue
		}
		if part.Reasoning || part.Synthetic || messageRole(spool, part.MessageID) != "context" {
			gap := Gap{Kind: "partial_stream", Detail: "native synthetic context is private or has incompatible ownership", SessionID: part.SessionID, RecordID: staged.id}
			if part.SessionID == sessionID && part.Seq > 0 {
				start, end := part.Seq, part.Seq
				gap.StartSeq, gap.EndSeq = &start, &end
			}
			trace.Gaps = append(trace.Gaps, gap)
			continue
		}
		trace.Messages = append(trace.Messages, Message{ID: "m:" + part.MessageID, Seq: part.Seq, Role: "context", Kind: "synthetic", Text: part.Text})
	}

	// Aggregate text parts per observable message id; a message record is
	// one normalized task message.
	type aggregate struct {
		texts    []string
		minSeq   int
		maxSeq   int
		nonFinal bool
	}
	texts := map[string]*aggregate{}
	var messageIDs []string
	for _, staged := range ordered {
		part := staged.record
		if part.Synthetic || part.PartType != "text" {
			continue
		}
		entry := texts[part.MessageID]
		if entry == nil {
			entry = &aggregate{minSeq: part.Seq, maxSeq: part.Seq}
			texts[part.MessageID] = entry
			messageIDs = append(messageIDs, part.MessageID)
		}
		entry.texts = append(entry.texts, part.Text)
		entry.nonFinal = entry.nonFinal || part.NonFinal
		if part.Seq < entry.minSeq {
			entry.minSeq = part.Seq
		}
		if part.Seq > entry.maxSeq {
			entry.maxSeq = part.Seq
		}
	}
	// Reasoning parts become excluded reasoning records (no text collected).
	for _, staged := range ordered {
		part := staged.record
		if part.Synthetic || part.PartType != "reasoning" {
			continue
		}
		trace.Messages = append(trace.Messages, Message{
			ID: "r:" + staged.id, Seq: part.Seq, Role: "assistant", Kind: "reasoning",
		})
	}
	// The last assistant text message by staged sequence carries the final.
	finalID := ""
	var lastFinalSeq int
	for _, id := range messageIDs {
		if messageRole(spool, id) != "assistant" || texts[id].nonFinal {
			continue
		}
		if seq := texts[id].maxSeq; seq > lastFinalSeq {
			lastFinalSeq = seq
			finalID = id
		}
	}
	sort.Slice(messageIDs, func(i, j int) bool { return texts[messageIDs[i]].minSeq < texts[messageIDs[j]].minSeq })
	for _, id := range messageIDs {
		entry := texts[id]
		role := messageRole(spool, id)
		kind := ""
		if id == finalID {
			kind = "final"
		}
		trace.Messages = append(trace.Messages, Message{
			ID: "m:" + id, Seq: entry.minSeq, Role: role, Kind: kind, Text: strings.Join(entry.texts, "\n"),
		})
	}
	// Calls: every tool part maps into the trace call schema; the compound
	// tool attempt is one call (callID identity).
	for _, staged := range ordered {
		part := staged.record
		if part.PartType != "tool" {
			continue
		}
		callID := part.CallID
		if callID == "" {
			callID = staged.id
		}
		call := CallRecord{ID: "c:" + callID, Seq: part.Seq, Tool: part.Tool}
		switch part.ToolStatus {
		case "completed":
			call.Status = StatusSuccess
		case "error":
			call.Status = StatusError
		default:
			call.Status = StatusRunning
		}
		if part.Input != "" {
			call.Input = limitDetail(part.Input)
		} else if part.ToolStatus != "" {
			call.InputOmitted = true
		}
		if path, digest := callSourceProvenance(spool, part); path != "" && digest != "" {
			call.SourceRefs = []SourceRef{{Path: path, SHA256: digest}}
		}
		if part.Output != "" {
			call.Result = append(call.Result, Fragment{ID: "f:" + staged.id, Text: part.Output})
		}
		if part.Error != "" {
			call.Result = append(call.Result, Fragment{ID: "f:" + staged.id + "-err", Text: part.Error})
		}
		trace.Calls = append(trace.Calls, call)
	}
	// Boundaries: one task_start anchor per user message after the global
	// first; the accepted-success boundary bounds the resolved interval.
	var userSeqs []int
	for _, message := range trace.Messages {
		if message.Role == "user" {
			userSeqs = append(userSeqs, message.Seq)
		}
	}
	sort.Ints(userSeqs)
	if spool.AcceptedAfterSeq > 0 {
		trace.Boundaries = append(trace.Boundaries, Boundary{Kind: "accepted_success", AfterSeq: spool.AcceptedAfterSeq})
	}
	for offset, seq := range userSeqs {
		if offset == 0 {
			continue
		}
		trace.Boundaries = append(trace.Boundaries, Boundary{Kind: "task_start", AfterSeq: seq})
	}
	if status, _ := terminalEventStatus(spool); status != "" {
		trace.TerminalStatus = status
	}
	// Observed session events map to trace gaps with interval semantics: a
	// history compaction is missing historical evidence.
	for _, event := range spool.Events {
		if event.SessionID != sessionID {
			continue
		}
		switch event.Event {
		case "session.evidence_gap":
			start, end := event.Seq, event.Seq
			trace.Gaps = append(trace.Gaps, Gap{Kind: "partial_stream", Detail: event.Detail, SessionID: sessionID, RecordID: event.RecordID, StartSeq: &start, EndSeq: &end})
		case "session.compacted":
			gap := Gap{Kind: "compacted", Detail: "session history compacted before seq " + fmt.Sprintf("%d", event.Seq), SessionID: sessionID, RecordID: event.RecordID}
			if event.Seq > 0 {
				start, end := 0, event.Seq
				gap.StartSeq, gap.EndSeq = &start, &end
			}
			trace.Gaps = append(trace.Gaps, gap)
		case "session.error", "session.idle", "session.status":
			// Terminal status derives above; idle/status are trigger
			// candidates, not evidence gaps.
		}
	}
	trace.Identities.TaskID = syntheticTaskID(spool, trace)
	refs, descendants := childTraces(store, sessionID, 0)
	trace.ChildRefs = append(trace.ChildRefs, refs...)
	trace.Descendants = append(trace.Descendants, descendants...)
	trace.SourceFiles = append(trace.SourceFiles, spool.SourceRefs...)
	// Also defend direct/legacy assembly, without modifying the caller's rows.
	coverage := *spool
	coverage.compactGaps()
	trace.Gaps = append(trace.Gaps, coverage.Gaps...)
	if coverage.GapOmissions > 0 || coverage.Overflows > 0 {
		trace.Gaps = append(trace.Gaps, Gap{Kind: "coverage_overflow", SessionID: sessionID, Omitted: gapCountAdd(coverage.GapOmissions, coverage.Overflows), Detail: "retained evidence was discarded; coverage scope unknown"})
	}
	for _, marker := range spool.StartupGaps {
		if marker.Seq > spool.AcceptedAfterSeq && !spool.startupCovered(marker) {
			trace.Gaps = append(trace.Gaps, Gap{Kind: "missing_startup", Detail: "terminal segment lacks its opening user evidence: " + marker.MessageID})
		}
	}
	// Keep the existing public assembled-trace child-gap projection. Ownership
	// stays explicit; normalization visits the original child once, not this copy.
	for _, child := range trace.Descendants {
		for _, gap := range child.Gaps {
			if gap.SessionID == "" {
				gap.SessionID = child.Identities.SessionID
			}
			trace.Gaps = append(trace.Gaps, gap)
		}
	}
	return trace
}

// syntheticTaskID is the stable per-turn task identity without a host task
// id: the session, the interval's first user message sequence and the final
// message identity. A re-analysis of the same completed turn is stable so
// the dispatch dedup holds across restarts.
// Spec: docs/specs/task-retrospective.md via assembleTrace
func syntheticTaskID(spool *Spool, trace *Trace) string {
	final := ""
	for index := len(trace.Messages) - 1; index >= 0; index-- {
		if trace.Messages[index].Kind == "final" {
			final = trace.Messages[index].ID
			break
		}
	}
	first := 0
	if accepted := spool.AcceptedAfterSeq; accepted > 0 {
		for _, message := range trace.Messages {
			if message.Role == "user" && message.Seq > accepted {
				first = message.Seq
				break
			}
		}
	} else {
		for _, message := range trace.Messages {
			if message.Role == "user" {
				first = message.Seq
				break
			}
		}
	}
	return fmt.Sprintf("retro:%s:%d:%s", spool.SessionID, first, final)
}

// orderedPart pairs a staged part id with its row.
// Spec: docs/specs/task-retrospective.md via assembleTrace
type orderedPart struct {
	id     string
	record *SpoolPart
}

// orderedParts returns the staged parts with their ids in stable sequence
// order.
// Spec: docs/specs/task-retrospective.md via assembleTrace
func orderedParts(spool *Spool) []orderedPart {
	out := make([]orderedPart, 0, len(spool.Parts))
	for id, part := range spool.Parts {
		out = append(out, orderedPart{id: id, record: part})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].record.Seq < out[j].record.Seq })
	return out
}

// messageRole returns the observed role of one message id; unknown defaults
// to assistant (user messages always arrive with their role via the
// chat.message hook or the message.updated reconcile).
// Spec: docs/specs/task-retrospective.md via assembleTrace
func messageRole(spool *Spool, messageID string) string {
	if message := spool.Messages[messageID]; message != nil && message.Role != "" {
		return message.Role
	}
	return "assistant"
}

// callSourceProvenance links one source-loading call to the at-use-time
// provenance row of the exact path it read (observed from the recorded tool
// input); the digest is the hash of the bytes as observed then.
// Spec: docs/specs/task-retrospective.md via assembleTrace
func callSourceProvenance(spool *Spool, part *SpoolPart) (string, string) {
	path, _ := observedReadPath(part)
	if path == "" || !instructionLikePath(path) {
		return "", ""
	}
	for _, row := range spool.SourceRefs {
		if row.Path == path {
			return path, row.SHA256
		}
	}
	return "", ""
}

// observedReadPath extracts the read tool input path of one staged call.
// Spec: docs/specs/task-retrospective.md via callSourceProvenance
func observedReadPath(part *SpoolPart) (string, bool) {
	if part.Tool != "read" || part.Input == "" {
		return "", false
	}
	var input struct {
		FilePath string `json:"filePath"`
		Path     string `json:"path"`
		File     string `json:"file"`
	}
	if err := json.Unmarshal([]byte(part.Input), &input); err != nil {
		return "", false
	}
	return firstNonEmpty(input.FilePath, input.Path, input.File), true
}

// childTraces assembles the linked descendant traces from their staged
// spools; a child with no staged spool stays a linked reference (the
// completeness gate catches it).
// Spec: docs/specs/task-retrospective.md via assembleTrace
func childTraces(store *Store, sessionID string, depth int) ([]string, []*Trace) {
	var refs []string
	var descendants []*Trace
	leafSpool, err := spoolOfStore(store, sessionID)
	if err != nil || leafSpool == nil {
		return nil, nil
	}
	if depth >= MaxDescendantDepth {
		for _, id := range sortedSessionIDs(leafSpool) {
			leafSession := leafSpool.Sessions[id]
			if id != sessionID && leafSession.ParentID == sessionID {
				refs = append(refs, id)
			}
		}
		return refs, nil
	}
	for _, id := range sortedSessionIDs(leafSpool) {
		session := leafSpool.Sessions[id]
		if id == sessionID || session.ParentID != sessionID {
			continue
		}
		refs = append(refs, id)
		child, err := loadSpool(store, id)
		if err != nil {
			continue
		}
		descendants = append(descendants, assembleTrace(store, child, id))
	}
	return refs, descendants
}

// spoolOfStore loads one staged spool (nil when absent, no error).
// Spec: docs/specs/task-retrospective.md via childTraces
func spoolOfStore(store *Store, sessionID string) (*Spool, error) {
	spool, err := loadSpool(store, sessionID)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return spool, err
}

// sortedSessionIDs lists staged session identity keys in stable order.
// Spec: docs/specs/task-retrospective.md via childTraces
func sortedSessionIDs(spool *Spool) []string {
	ids := make([]string, 0, len(spool.Sessions))
	for id := range spool.Sessions {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// terminalEventStatus derives the trace terminal status from the last
// session.error of the owner that follows the last user text part.
// Spec: docs/specs/task-retrospective.md via assembleTrace
func terminalEventStatus(spool *Spool) (string, string) {
	lastUser := 0
	for _, part := range spool.Parts {
		if part.PartType == "text" && messageRole(spool, part.MessageID) == "user" && part.Seq > lastUser {
			lastUser = part.Seq
		}
	}
	var last *SpoolEvent
	for index := range spool.Events {
		current := &spool.Events[index]
		if current.SessionID == spool.SessionID && current.Event == "session.error" && current.Seq > lastUser {
			last = current
		}
	}
	if last == nil {
		return "", ""
	}
	if strings.Contains(last.Detail, "MessageAbortedError") {
		return TaskStatusAborted, last.Detail
	}
	return TaskStatusError, last.Detail
}

// limitDetail clips a string to the record cap, rune-safe.
// Spec: docs/specs/task-retrospective.md via limitRecordBytes
func limitDetail(value string) string {
	return clipRuneBoundary(value, EnvelopeRecordMaxBytes)
}
