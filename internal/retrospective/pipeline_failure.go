// Spec: docs/specs/task-retrospective.md — Retrospective failure feedback.
package retrospective

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"
)

// pipelineSourceRevision observes only the declared implementation root's current
// loose/detached HEAD. It is not original-task provenance or a clean-tree claim.
// Reuse bounded, symlink-refusing local reads; no Git process/config or discovery.
// Unsupported ownership metadata/layouts (including packed-only refs) abstain.
func pipelineSourceRevision(root string) string {
	owned := func(path string) bool {
		info, err := os.Lstat(path)
		if err != nil || info.Mode()&os.ModeSymlink != 0 || os.Geteuid() < 0 {
			return false
		}
		// FileInfo.Sys is platform-specific. Reflect only the UID; platforms
		// without an observable UID fail closed rather than pretending ownership.
		value := reflect.ValueOf(info.Sys())
		if value.Kind() == reflect.Pointer {
			value = value.Elem()
		}
		if value.Kind() != reflect.Struct {
			return false
		}
		uid := value.FieldByName("Uid")
		return uid.IsValid() && uid.CanUint() && uid.Uint() == uint64(os.Geteuid())
	}
	read := func(path string) (string, error) {
		if !owned(path) {
			return "", ErrUnsafe
		}
		before, err := os.Lstat(path)
		if err != nil {
			return "", err
		}
		body, err := ReadLessonFile(path, 4096)
		if err != nil {
			return "", err
		}
		after, err := os.Lstat(path)
		if err != nil || !owned(path) || !os.SameFile(before, after) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
			return "", ErrUnsafe
		}
		return strings.TrimSpace(string(body)), nil
	}
	if checkCanonicalRoot(root) != nil || !owned(root) {
		return ""
	}
	real, err := filepath.EvalSymlinks(root)
	if err != nil || real != root {
		return ""
	}
	git := filepath.Join(root, ".git")
	info, err := os.Lstat(git)
	if err != nil || !owned(git) {
		return ""
	}
	if !info.IsDir() {
		pointer, err := read(git)
		if err != nil || !strings.HasPrefix(pointer, "gitdir: ") || strings.ContainsAny(pointer, "\r\n") {
			return ""
		}
		git = strings.TrimPrefix(pointer, "gitdir: ")
		if !filepath.IsAbs(git) {
			git = filepath.Join(root, git)
		}
		git = filepath.Clean(git)
	}
	if !owned(git) {
		return ""
	}
	head, err := read(filepath.Join(git, "HEAD"))
	if err != nil {
		return ""
	}
	revision := head
	if strings.HasPrefix(head, "ref: ") {
		ref := strings.TrimPrefix(head, "ref: ")
		if !strings.HasPrefix(ref, "refs/heads/") || filepath.Clean(ref) != ref || strings.Trim(ref, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_./-") != "" {
			return ""
		}
		common := git
		if _, err := os.Lstat(filepath.Join(git, "commondir")); err == nil {
			pointer, err := read(filepath.Join(git, "commondir"))
			if err != nil || pointer == "" || strings.ContainsAny(pointer, "\r\n") {
				return ""
			}
			common = pointer
			if !filepath.IsAbs(common) {
				common = filepath.Join(git, common)
			}
			common = filepath.Clean(common)
		} else if !errors.Is(err, os.ErrNotExist) {
			return ""
		}
		if !owned(common) {
			return ""
		}
		revision, err = read(filepath.Join(common, ref))
		if err != nil {
			return ""
		}
	}
	if !validObservedRevision(revision) {
		return ""
	}
	confirmed, err := read(filepath.Join(git, "HEAD"))
	if err != nil || confirmed != head {
		return ""
	}
	return revision
}

// recordPipelineFailure reuses the durable incident/native review path, not the
// incomplete original task's eligibility. Its ONLY outcome is reviewed advice;
// it never enters a text/code repair sink or starts another retrospective.
func recordPipelineFailure(ctx context.Context, opts DrainOptions, store *Store, job *DrainJob) error {
	code := "retrospective_job_failed"
	for _, item := range []struct{ match, code string }{
		{"startup incomplete", "missing_episode_intake"},
		{"evidence incomplete", "incomplete_episode_evidence"},
		{"dispatch:", "retrospective_dispatch_failed"},
		{"analyze episode:", "retrospective_analysis_failed"},
		{"interpretation unavailable", "retrospective_interpretation_failed"},
	} {
		if strings.Contains(job.Error, item.match) {
			code = item.code
			break
		}
	}
	// One incident per failure class and installed source root, not one root
	// per failed task or model attempt. No task content enters the key/prompt.
	identity, _ := json.Marshal([]string{"pipeline-failure-v1", opts.SourceRoot, code})
	id := "pipeline-" + sha256hex(identity)
	job.MaintenanceIncident = id
	receipt, err := store.ReadReceipt(id)
	if err == nil {
		return nil
	} // including ambiguous/pending: never auto-retry creation
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	stamp := time.Now().UTC().Format(time.RFC3339)
	marker := learningOrigin(id)
	revision := ""
	if opts.Config != nil && withinAny(opts.Config.CanonicalRoots, opts.SourceRoot) {
		revision = pipelineSourceRevision(opts.SourceRoot)
	}
	report := &Report{SignalReport: SignalReport{SchemaVersion: ReportSchemaVersion, Mode: "pipeline_failure",
		Trace:    ReportTrace{Identities: Identities{TaskID: id, ExecutionRoot: opts.SourceRoot, SourceRevision: revision}, InputSHA256: sha256hex(identity)},
		Evidence: []evidenceRecord{{ID: "pipeline-diagnostic", Kind: RecordSyntheticContext, SourceKind: "agf.pipeline_failure", Text: "Original report locator: " + job.ReportPath + "\nObserved job locator: " + jobsPath(store, job.SessionID)}}},
		Status: ReportStatusIncomplete, IncompleteReasons: []string{code}, GeneratedAt: stamp}
	path := filepath.Join(store.Dir, "reports", id+".json")
	if err := WriteReport(path, report); err != nil {
		return err
	}
	raw, _ := json.Marshal(report)
	receipt = &Receipt{SchemaVersion: 1, IncidentID: id, TaskID: id, LearningOrigin: marker,
		Directory: opts.SourceRoot, Title: marker + " retrospective pipeline failure",
		Status: StatusQueued, ReportLocator: path, ReportSHA256: sha256hex(raw), CreatedAt: stamp, UpdatedAt: stamp}
	if err := store.WriteReceipt(receipt); err != nil {
		return err
	}
	cfg := opts.Config
	// No new model selection, fallback target, or authority to change runtime
	// hooks/executables. Missing explicit capability leaves a visible queue.
	if cfg == nil || cfg.DispatchServer == "" || cfg.DispatchModel == "" || !filepath.IsAbs(opts.SourceRoot) || !withinAny(cfg.CanonicalRoots, opts.SourceRoot) {
		return nil
	}
	if revision == "" {
		receipt.Status, receipt.Error = StatusPending, "diagnosed implementation Git HEAD unavailable; pipeline review blocked, no invented original-task version"
		_, err = save(store, receipt)
		return err
	}
	enabled, err := Enabled(opts.ConfigPath)
	if err != nil || !enabled {
		return err
	}
	client := OpenSessionClient(cfg)
	dispatch := DispatchOptions{Config: cfg, SettingsPath: opts.ConfigPath}
	stageCtx, done := context.WithTimeout(ctx, cfg.DispatchTimeout)
	defer done()
	objective := "Investigate this retrospective PIPELINE FAILURE, not the incomplete original task. Read the local diagnostic/report locator, identify a concrete minimal correction and runnable regression check in summary. Do NOT edit any file, settings, hooks, executable or permissions. Do NOT classify missing original evidence as complete. Return ONLY {\"findings\":[],\"summary\":\"actionable diagnosis, affected path and proposed check\"}. This is advisory maintenance intake; implementation needs ordinary scoped change/review authority."
	text, err := nativeStage(stageCtx, dispatch, store, receipt, report, marker, LearningCorrectorID, "generate", nativePacket(dispatch, receipt, report, objective, nil, ""), client)
	if err != nil {
		receipt.Status, receipt.Error = StatusPending, "pipeline diagnosis unavailable; inspect existing receipt/root, no automatic retry"
		_, saveErr := save(store, receipt)
		return saveErr
	}
	redactor, err := NewRedactor(cfg)
	if err != nil {
		return err
	}
	text = redactor.Text(text)
	var proposal NativeProposal
	if nativeDecode(text, &proposal) != nil || proposal.Summary == "" || len(proposal.Findings) != 0 {
		receipt.Status, receipt.Error = StatusPending, "pipeline diagnosis must be advice only; no edits accepted"
		_, err = save(store, receipt)
		return err
	}
	receipt.CorrectorResult = text
	body, _ := json.Marshal(proposal)
	digest := sha256hex(body)
	verdict, err := nativeStage(stageCtx, dispatch, store, receipt, report, marker, cfg.Reviewer, "review", nativePacket(dispatch, receipt, report, "Independently verify only the advisory pipeline diagnosis and proposed regression check against the diagnosed implementation revision. This is not instruction-repair acceptance or original-task success; no old-instruction replacement or causal recovery is proposed. No edits, repair authority or invented executed coverage. Return only {\"approved\":true|false,\"candidate_sha256\":\"supplied digest\",\"reason\":\"evidence-backed verdict\"}.", proposal, digest), client)
	verdict = redactor.Text(verdict)
	var review nativeDecision
	if err != nil || nativeDecode(verdict, &review) != nil || !review.Approved || review.CandidateSHA != digest || review.Reason == "" {
		receipt.Status, receipt.Error = StatusPending, "pipeline diagnosis lacks independent acceptance"
	} else {
		receipt.ReviewVerdict, receipt.Status, receipt.NativeCompleted = verdict, StatusSuggested, true
	}
	_, err = save(store, receipt)
	return err
}
