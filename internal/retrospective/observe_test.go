// Spec: docs/specs/task-retrospective.md
package retrospective

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// fixtureRoot resolves the recorded OpenCode replay fixtures (shared with
// the npm adapter tests).
// Spec: docs/specs/task-retrospective.md
func fixtureRoot(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(cwd, "..", "..", "fixtures", "retrospective", "opencode")
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		t.Fatalf("recorded fixture directory missing: %s", root)
	}
	return root
}

// replayFromFile replays the recorded envelopes (token-substituted) through
// the ingest pipeline in order and returns the staged count.
// Spec: docs/specs/task-retrospective.md
func replayFromFile(t *testing.T, store *Store, file string, tokens map[string]string) int {
	t.Helper()
	body, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	staged := 0
	for number, line := range strings.Split(strings.TrimSpace(string(body)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		for token, replacement := range tokens {
			line = strings.ReplaceAll(line, token, replacement)
		}
		line = strings.ReplaceAll(line, "{{server_url}}", "http://127.0.0.1:1")
		envelope := &Envelope{}
		decoder := json.NewDecoder(strings.NewReader(line))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(envelope); err != nil {
			t.Fatalf("decode envelope %s line %d: %v", file, number+1, err)
		}
		result, err := IngestEnvelope(IngestOptions{Config: DefaultConfig(), Store: store}, envelope)
		if err != nil {
			t.Fatalf("ingest %s line %d: %v", file, number+1, err)
		}
		if result.Suppressed {
			continue
		}
		staged += result.Staged
	}
	return staged
}

// testStore builds a disposable state store.
// Spec: docs/specs/task-retrospective.md
func testStore(t *testing.T) *Store {
	t.Helper()
	store, err := OpenStore(filepath.Join(t.TempDir(), "state"))
	if err != nil {
		t.Fatal(err)
	}
	return store
}

// observationConfig points the config at a disposable store.
// Spec: docs/specs/task-retrospective.md
func observationConfig(store *Store) *Config {
	cfg := DefaultConfig()
	cfg.StateDir = store.Dir
	return cfg
}

func loadSpoolFor(t *testing.T, store *Store, sessionID string) *Spool {
	t.Helper()
	spool, err := loadSpool(store, sessionID)
	if err != nil {
		t.Fatalf("load spool %s: %v", sessionID, err)
	}
	return spool
}

func recordsOf(record EnvelopeRecord) []EnvelopeRecord {
	return []EnvelopeRecord{record}
}

func redactorForTest() *Redactor {
	redactor, err := NewRedactor(DefaultConfig())
	if err != nil {
		panic(err)
	}
	return redactor
}

func TestRecordedReplayStreamedUpdatesCollapse(t *testing.T) {
	store := testStore(t)
	staged := replayFromFile(t, store,
		filepath.Join(fixtureRoot(t), "envelopes", "streamed.jsonl"),
		map[string]string{"{{execution_root}}": "/proj/stream", "{{wrong_root}}": "/proj/other"})
	if staged != 10 {
		t.Fatalf("staged = %d (want 10 records over one flush)", staged)
	}
	spool := loadSpoolFor(t, store, "ses_stream")
	part := spool.Parts["prt_a1"]
	if part == nil || part.Text != "Reading the onboarding instructions now and preparing the summary" {
		t.Fatalf("streamed updates did not collapse to the latest text: %+v", part)
	}
	if len(spool.Parts) != 4 {
		t.Fatalf("parts = %d (one record per observable part id)", len(spool.Parts))
	}
	if len(spool.Events) != 1 || spool.Events[0].Event != "session.idle" || spool.Events[0].SessionID != "ses_stream" {
		t.Fatalf("events = %+v", spool.Events)
	}
	job, err := store.ReadDrainJob("ses_stream")
	if err != nil {
		t.Fatalf("trigger did not create a drain job: %v", err)
	}
	if job.Status != JobPending || job.CreatedAt == "" {
		t.Fatalf("job = %+v", job)
	}
}

func TestRecordedReplayDuplicateTerminalEventsStayStable(t *testing.T) {
	store := testStore(t)
	file := filepath.Join(fixtureRoot(t), "envelopes", "duplicates.jsonl")
	tokens := map[string]string{"{{execution_root}}": "/proj/dup", "{{wrong_root}}": "/proj/other"}
	replayFromFile(t, store, file, tokens)
	spool := loadSpoolFor(t, store, "ses_dup")
	part := spool.Parts["prt_tool_1"]
	if part == nil || part.ToolStatus != "completed" || part.Output != "context bytes" {
		t.Fatalf("tool part = %+v", part)
	}
	firstSeq := part.Seq
	beforeParts := len(spool.Parts)
	// Out-of-order terminal redelivery after the first trigger: the record
	// is the same id; the part sequence space must not move, the staged
	// part set must not grow, and the pending job must stay exactly one.
	replayFromFile(t, store, file, tokens)
	again := loadSpoolFor(t, store, "ses_dup")
	if got := again.Parts["prt_tool_1"]; got.Seq != firstSeq {
		t.Fatalf("duplicate terminal events reordered the stable sequence: %d then %d", firstSeq, got.Seq)
	}
	if len(again.Parts) != beforeParts {
		t.Fatalf("redelivery grew the staged part set: %d -> %d", beforeParts, len(again.Parts))
	}
	job, err := store.ReadDrainJob("ses_dup")
	if err != nil || job.Status != JobPending {
		t.Fatalf("job state after duplicate trigger: %+v %v", job, err)
	}
}

func TestRecordedReplayChildAggregation(t *testing.T) {
	store := testStore(t)
	replayFromFile(t, store,
		filepath.Join(fixtureRoot(t), "envelopes", "childsession.jsonl"),
		map[string]string{"{{execution_root}}": "/proj/tree", "{{wrong_root}}": "/proj/other"})
	// The child trigger must not create its own job (non-root owner).
	if job, err := store.ReadDrainJob("ses_child"); err == nil {
		t.Fatalf("child session produced a job: %+v", job)
	}
	job, err := store.ReadDrainJob("ses_root")
	if err != nil || job.Status != JobPending {
		t.Fatalf("root job missing: %+v %v", job, err)
	}
	spool := loadSpoolFor(t, store, "ses_root")
	if row := spool.Sessions["ses_child"]; row == nil || row.ParentID != "ses_root" {
		t.Fatalf("observed identity map misses the child row: %+v", spool.Sessions)
	}
	for id := range spool.Parts {
		if strings.HasPrefix(id, "prt_child") {
			t.Fatalf("the root spool must not carry child parts: %+v", spool.Parts)
		}
	}
	if len(spool.Parts) < 2 {
		t.Fatalf("root parts missing: %+v", spool.Parts)
	}
	child := loadSpoolFor(t, store, "ses_child")
	if part := child.Parts["prt_tool_child"]; part == nil || part.ToolStatus != "completed" {
		t.Fatalf("child tool part missing: %+v", child.Parts)
	}
	trace := assembleTrace(store, spool, "ses_root")
	if len(trace.ChildRefs) != 1 || trace.ChildRefs[0] != "ses_child" {
		t.Fatalf("child refs = %+v", trace.ChildRefs)
	}
	if len(trace.Descendants) != 1 || trace.Descendants[0].Identities.SessionID != "ses_child" {
		t.Fatalf("descendants = %d", len(trace.Descendants))
	}
	normalized, err := Normalize(trace)
	if err != nil {
		t.Fatal(err)
	}
	if !normalized.Complete() {
		t.Fatalf("aggregated tree must be complete: %+v", normalized.Reasons)
	}
	if !containsSessionRecords(normalized, "ses_child", true) {
		t.Fatalf("child records not included in the aggregated trace")
	}
}

// containsSessionRecords reports whether records of a descendant session
// joined the aggregated interval.
// Spec: docs/specs/task-retrospective.md
func containsSessionRecords(n *Normalized, sessionID string, want bool) bool {
	for _, record := range n.Records {
		if record.SessionID == sessionID {
			return want
		}
	}
	return !want
}

func TestRecordedReplayAbortThenIdleGates(t *testing.T) {
	store := testStore(t)
	replayFromFile(t, store,
		filepath.Join(fixtureRoot(t), "envelopes", "abort-then-idle.jsonl"),
		map[string]string{"{{execution_root}}": "/proj/abort", "{{wrong_root}}": "/proj/other"})
	spool := loadSpoolFor(t, store, "ses_abort")
	trace := assembleTrace(store, spool, "ses_abort")
	if trace.TerminalStatus != TaskStatusAborted {
		t.Fatalf("terminal status = %q (want aborted)", trace.TerminalStatus)
	}
	normalized, err := Normalize(trace)
	if err != nil {
		t.Fatal(err)
	}
	if normalized.Complete() {
		t.Fatalf("aborted session must gate: %+v", normalized.Reasons)
	}
	kinds := map[string]bool{}
	for _, reason := range normalized.Reasons {
		kinds[reason.Kind] = true
	}
	if !kinds[ReasonTerminalAbortError] || !kinds[ReasonRunningTool] {
		t.Fatalf("missing abort / running-tool gates: %+v", normalized.Reasons)
	}
}

func TestRecordedReplayMultiTurnBoundary(t *testing.T) {
	store := testStore(t)
	replayFromFile(t, store,
		filepath.Join(fixtureRoot(t), "envelopes", "multiturn.jsonl"),
		map[string]string{"{{execution_root}}": "/proj/multi", "{{wrong_root}}": "/proj/other"})
	spool := loadSpoolFor(t, store, "ses_multi")
	trace := assembleTrace(store, spool, "ses_multi")
	normalized, err := Normalize(trace)
	if err != nil {
		t.Fatal(err)
	}
	if normalized.BoundarySource != "session-start" {
		t.Fatalf("boundary source = %s", normalized.BoundarySource)
	}
	// Three turns without an accepted boundary: two task_start anchors
	// inside the whole-session interval resolve ambiguous.
	ambiguous := 0
	for _, reason := range normalized.Reasons {
		if reason.Kind == ReasonAmbiguousBoundary {
			ambiguous++
		}
	}
	if ambiguous != 1 {
		t.Fatalf("multi-turn without an accepted boundary must be ambiguous: %+v", normalized.Reasons)
	}
	// After accepting turn one (its end sequence), the resolved interval is
	// only the newest turn.
	turn1End := spool.Parts["prt_f1"].Seq
	if err := acceptBoundary(store, DefaultConfig(), "ses_multi", turn1End); err != nil {
		t.Fatal(err)
	}
	spool = loadSpoolFor(t, store, "ses_multi")
	trace = assembleTrace(store, spool, "ses_multi")
	normalized, err = Normalize(trace)
	if err != nil {
		t.Fatal(err)
	}
	if normalized.BoundarySource != "accepted-success" || len(normalized.Reasons) != 0 {
		t.Fatalf("post-acceptance interval wrong: %s %+v", normalized.BoundarySource, normalized.Reasons)
	}
	users := 0
	for _, record := range normalized.Records {
		if record.Kind == RecordUserMessage && normalized.InInterval(record) {
			users++
		}
	}
	if users != 1 {
		t.Fatalf("post-acceptance interval carries %d user messages (want 1)", users)
	}
}

func TestRecordedReplayCompactionGap(t *testing.T) {
	store := testStore(t)
	replayFromFile(t, store,
		filepath.Join(fixtureRoot(t), "envelopes", "compaction.jsonl"),
		map[string]string{"{{execution_root}}": "/proj/compact", "{{wrong_root}}": "/proj/other"})
	spool := loadSpoolFor(t, store, "ses_compact")
	trace := assembleTrace(store, spool, "ses_compact")
	compacted := false
	for _, gap := range trace.Gaps {
		if gap.Kind == "compacted" {
			compacted = true
		}
	}
	if !compacted {
		t.Fatalf("compaction must mark the gap: %+v", trace.Gaps)
	}
	if _, err := Normalize(trace); err != nil {
		t.Fatal(err)
	}
}

func TestRecordedReplayRestartResumeIsIdempotent(t *testing.T) {
	store := testStore(t)
	tokens := map[string]string{"{{execution_root}}": "/proj/restart", "{{wrong_root}}": "/proj/other"}
	// The pre-restart window produced no recorded envelope (below the flush
	// threshold, no idle): stage it the way a restart-time server reconcile
	// would (an upsert of the same observable rows).
	pre := &Envelope{Version: EnvelopeSchemaVersion, Harness: HarnessOpenCode, SessionID: "ses_restart", Records: []EnvelopeRecord{
		{Kind: "session", SessionID: "ses_restart", Title: "Restart fixture", Directory: "/proj/restart"},
		{Kind: "message", RecordID: "msg_u1", SessionID: "ses_restart", Role: "user"},
		{Kind: "part", RecordID: "prt_u1", SessionID: "ses_restart", MessageID: "msg_u1", PartType: "text", Text: "Mirror the status page"},
		{Kind: "message", RecordID: "msg_a1", SessionID: "ses_restart", Role: "assistant"},
		{Kind: "part", RecordID: "prt_tool_1", SessionID: "ses_restart", MessageID: "msg_a1", PartType: "tool", Tool: "read", CallID: "call_pre_restart", ToolStatus: "completed", Input: `{"filePath":"/proj/restart/AGENTS.md"}`, Output: "agents read"},
	}}
	if _, err := IngestEnvelope(IngestOptions{Config: DefaultConfig(), Store: store}, pre); err != nil {
		t.Fatal(err)
	}
	spool := loadSpoolFor(t, store, "ses_restart")
	if len(spool.Parts) != 2 {
		t.Fatalf("pre-restart staging = %+v", spool.Parts)
	}
	// Redeliver the same records (a restart resume): the upsert must keep
	// the same id/sequence and only refresh the state.
	if _, err := IngestEnvelope(IngestOptions{Config: DefaultConfig(), Store: store}, pre); err != nil {
		t.Fatal(err)
	}
	again := loadSpoolFor(t, store, "ses_restart")
	if got := again.Parts["prt_tool_1"]; got == nil || got.Seq != spool.Parts["prt_tool_1"].Seq || got.Output != "agents read" {
		t.Fatalf("restart redelivery changed staged state: %+v", got)
	}
	// The recorded post-restart envelope (fresh plugin instance) continues
	// the same session.
	replayFromFile(t, store, filepath.Join(fixtureRoot(t), "envelopes", "restart-post.jsonl"), tokens)
	after := loadSpoolFor(t, store, "ses_restart")
	if len(after.Parts) != 4 {
		t.Fatalf("post-restart staging = %+v", after.Parts)
	}
	trace := assembleTrace(store, after, "ses_restart")
	normalized, err := Normalize(trace)
	if err != nil {
		t.Fatal(err)
	}
	if !normalized.Complete() {
		t.Fatalf("restored session must assemble completely: %+v", normalized.Reasons)
	}
	// The job is created on the post-restart trigger and is the same
	// session's job (one job per session, no second lifetime).
	job, err := store.ReadDrainJob("ses_restart")
	if err != nil || job.Status != JobPending {
		t.Fatalf("post-restart job: %+v %v", job, err)
	}
}

func TestRecordedReplaySuppressionRules(t *testing.T) {
	t.Run("origins row", func(t *testing.T) {
		store := testStore(t)
		if err := os.WriteFile(filepath.Join(store.Dir, "origins", "ses_root"), []byte("marker\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		staged := replayFromFile(t, store,
			filepath.Join(fixtureRoot(t), "envelopes", "childsession.jsonl"),
			map[string]string{"{{execution_root}}": "/proj/tree", "{{wrong_root}}": "/proj/other"})
		if staged > 0 {
			t.Fatalf("staged %d records for an origin-listed session", staged)
		}
		for _, id := range []string{"ses_root", "ses_child"} {
			if _, err := os.Stat(spoolPath(store, id)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("spool %s must be dropped after suppression: %v", id, err)
			}
		}
	})
	t.Run("learning title marker", func(t *testing.T) {
		store := testStore(t)
		staged := replayFromFile(t, store,
			filepath.Join(fixtureRoot(t), "envelopes", "learning-suppressed.jsonl"),
			map[string]string{"{{execution_root}}": "/proj/learn", "{{wrong_root}}": "/proj/other"})
		if staged > 0 {
			t.Fatalf("staged %d records of a marker-titled session", staged)
		}
		if job, err := store.ReadDrainJob("ses_learning"); err == nil {
			t.Fatalf("suppressed session created a job: %+v", job)
		}
	})
	t.Run("scoped disable marker", func(t *testing.T) {
		store := testStore(t)
		project := t.TempDir()
		if err := os.WriteFile(filepath.Join(project, ScopedDisableMarker), []byte("1\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		body, err := os.ReadFile(filepath.Join(fixtureRoot(t), "envelopes", "streamed.jsonl"))
		if err != nil {
			t.Fatal(err)
		}
		line := strings.ReplaceAll(strings.Split(string(body), "\n")[0], "{{execution_root}}", project)
		envelope := &Envelope{}
		decoder := json.NewDecoder(strings.NewReader(line))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(envelope); err != nil {
			t.Fatal(err)
		}
		envelope.ProjectDir = project
		result, err := IngestEnvelope(IngestOptions{Config: observationConfig(store), Store: store}, envelope)
		if err != nil {
			t.Fatal(err)
		}
		if !result.Suppressed {
			t.Fatalf("scoped disable marker must suppress: %+v", result)
		}
	})
}

func TestEnvelopeValidationFailsClosed(t *testing.T) {
	cases := map[string]string{
		"bad version":        `{"version":2,"harness":"opencode","session_id":"s","records":[]}`,
		"unknown harness":    `{"version":1,"harness":"claude","session_id":"s","records":[]}`,
		"unknown field":      `{"version":1,"harness":"opencode","session_id":"s","records":[],"extra":1}`,
		"bad kind":           `{"version":1,"harness":"opencode","session_id":"s","records":[{"kind":"weird"}]}`,
		"missing session":    `{"version":1,"harness":"opencode","records":[]}`,
		"part w/o message":   `{"version":1,"harness":"opencode","session_id":"s","records":[{"kind":"part","record_id":"p"}]}`,
		"unobserved role":    `{"version":1,"harness":"opencode","session_id":"s","records":[{"kind":"message","record_id":"m","role":"operator"}]}`,
		"unknown event":      `{"version":1,"harness":"opencode","session_id":"s","records":[{"kind":"event","event":"server.connected","session_id":"s"}]}`,
		"source without row": `{"version":1,"harness":"opencode","session_id":"s","records":[{"kind":"source"}]}`,
	}
	for name, document := range cases {
		if _, err := DecodeEnvelope(strings.NewReader(document)); err == nil {
			t.Fatalf("%s: envelope validation passed: %s", name, document)
		}
	}
	if _, err := DecodeEnvelope(strings.NewReader(`{"version":1,"harness":"opencode","session_id":"s","records":[]}`)); err != nil {
		t.Fatalf("valid envelope rejected: %v", err)
	}
}

func TestSpoolBoundOverflowDropsOldest(t *testing.T) {
	store := testStore(t)
	cfg := observationConfig(store)
	cfg.SpoolMaxBytes = 4096
	spool, err := loadSpool(store, "ses_bound")
	if err != nil {
		t.Fatal(err)
	}
	if spool.LastSeq != 0 {
		t.Fatalf("fresh spool: %+v", spool)
	}
	for index := 0; index < 64; index++ {
		record := EnvelopeRecord{Kind: "part", RecordID: fmt.Sprintf("prt_big_%d", index),
			MessageID: "msg", PartType: "text",
			Text: strings.Repeat("x", 1024)}
		stageRecords(redactorForTest(), recordsOf(record), spool)
	}
	if err := writeSpoolBounded(cfg, store, spool); err != nil {
		t.Fatalf("bounded write: %v", err)
	}
	reloaded := loadSpoolFor(t, store, "ses_bound")
	if reloaded.Overflows == 0 {
		t.Fatalf("overflow not recorded: %+v", reloaded)
	}
	found := false
	for _, gap := range reloaded.Gaps {
		if gap.Kind == "partial_stream" && strings.HasPrefix(gap.Detail, "spool overflow dropped") {
			found = true
		}
	}
	if !found {
		t.Fatalf("overflow gap missing: %+v", reloaded.Gaps)
	}
	if len(reloaded.Parts) >= 64 {
		t.Fatalf("oldest parts must drop first: %d remain", len(reloaded.Parts))
	}
	body, err := os.ReadFile(spoolPath(store, "ses_bound"))
	if err != nil {
		t.Fatal(err)
	}
	if int64(len(body)) > int64(cfg.SpoolMaxBytes) {
		t.Fatalf("bounded spool exceeded: %d > %d", len(body), cfg.SpoolMaxBytes)
	}
}

func TestIngestTriggerOnlyOnRootOwner(t *testing.T) {
	store := testStore(t)
	cfg := observationConfig(store)
	var spawnCalls atomic.Int64
	spawn := func() error { spawnCalls.Add(1); return nil }
	stage := &Envelope{Version: EnvelopeSchemaVersion, Harness: HarnessOpenCode, SessionID: "ses_owner", Records: []EnvelopeRecord{
		{Kind: "session", SessionID: "ses_owner", ParentID: "", Title: "owner", Directory: "/proj/owner"},
		{Kind: "session", SessionID: "ses_childx", ParentID: "ses_owner", Title: "child", Directory: "/proj/owner"},
		{Kind: "message", RecordID: "msg_ux", SessionID: "ses_owner", Role: "user"},
		{Kind: "part", RecordID: "prt_ux", SessionID: "ses_owner", MessageID: "msg_ux", PartType: "text", Text: "hello"},
		{Kind: "message", RecordID: "msg_child_u", SessionID: "ses_childx", Role: "user"},
		{Kind: "part", RecordID: "prt_childx", SessionID: "ses_childx", MessageID: "msg_child_u", PartType: "text", Text: "child turn"},
	}}
	if _, err := IngestEnvelope(IngestOptions{Config: cfg, Store: store, SpawnWorker: spawn}, stage); err != nil {
		t.Fatal(err)
	}
	// A trigger for the known child never creates a job or spawns a worker.
	if _, err := IngestEnvelope(IngestOptions{Config: cfg, Store: store, SpawnWorker: spawn}, &Envelope{
		Version: EnvelopeSchemaVersion, Harness: HarnessOpenCode,
		SessionID: "ses_childx", Trigger: true,
	}); err != nil {
		t.Fatal(err)
	}
	if job, err := store.ReadDrainJob("ses_childx"); err == nil {
		t.Fatalf("child owner produced a job: %+v", job)
	}
	if spawnCalls.Load() != 0 {
		t.Fatalf("child triggers must not spawn: %d", spawnCalls.Load())
	}
	// The root owner's trigger creates exactly one job and spawns once.
	if _, err := IngestEnvelope(IngestOptions{Config: cfg, Store: store, SpawnWorker: spawn}, &Envelope{
		Version: EnvelopeSchemaVersion, Harness: HarnessOpenCode,
		SessionID: "ses_owner", Trigger: true,
	}); err != nil {
		t.Fatal(err)
	}
	if job, err := store.ReadDrainJob("ses_owner"); err != nil || job.Status != JobPending {
		t.Fatalf("root job: %+v %v", job, err)
	}
	if spawnCalls.Load() != 1 {
		t.Fatalf("worker spawns = %d (want 1)", spawnCalls.Load())
	}
	// A second trigger while pending does not re-spawn the worker.
	if _, err := IngestEnvelope(IngestOptions{Config: cfg, Store: store, SpawnWorker: spawn}, &Envelope{
		Version: EnvelopeSchemaVersion, Harness: HarnessOpenCode,
		SessionID: "ses_owner", Trigger: true,
	}); err != nil {
		t.Fatal(err)
	}
	if spawnCalls.Load() != 1 {
		t.Fatalf("pending triggers must not respawn: %d", spawnCalls.Load())
	}
	// An idle blip without any user turn must not create jobs.
	if err := dropSpool(store, "ses_owner"); err != nil {
		t.Fatal(err)
	}
	if err := dropJob(store, "ses_owner"); err != nil {
		t.Fatal(err)
	}
	empty := &Envelope{Version: EnvelopeSchemaVersion, Harness: HarnessOpenCode, SessionID: "ses_owner2", Trigger: true}
	if _, err := IngestEnvelope(IngestOptions{Config: cfg, Store: store, SpawnWorker: spawn}, empty); err != nil {
		t.Fatal(err)
	}
	if job, err := store.ReadDrainJob("ses_owner2"); err == nil {
		t.Fatalf("degenerate trigger created a job: %+v", job)
	}
}

// dropJob removes one persisted job (test helper).
// Spec: docs/specs/task-retrospective.md
func dropJob(store *Store, sessionID string) error {
	if err := os.Remove(jobsPath(store, sessionID)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func TestIngestRedactsStagedObservables(t *testing.T) {
	store := testStore(t)
	envelope := &Envelope{
		Version: EnvelopeSchemaVersion, Harness: HarnessOpenCode, SessionID: "ses_redact",
		Records: []EnvelopeRecord{{
			Kind: "part", RecordID: "prt_secret", MessageID: "msg_u", PartType: "tool",
			Tool: "bash", ToolStatus: "completed",
			Output: "setting Api-Key: sk-CANARY-key-value",
		}},
	}
	if _, err := IngestEnvelope(IngestOptions{Config: DefaultConfig(), Store: store}, envelope); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(spoolPath(store, "ses_redact"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "sk-CANARY-key-value") {
		t.Fatalf("staged spool leaked a secret pattern: %s", string(body))
	}
	if !strings.Contains(string(body), "[REDACTED") {
		t.Fatalf("staged spool did not redact: %s", string(body))
	}
}

func TestInstructionLikePathFilter(t *testing.T) {
	cases := map[string]bool{
		"/repo/AGENTS.md":                   true,
		"/repo/docs/README.md":              true,
		"/repo/docs/intro.md":               true,
		"/home/x/.agents/skills/n/SKILL.md": true,
		"/repo/src/main.go":                 false,
		"/repo/CONTEXT.md":                  true,
		"/repo/notes/util.go":               false,
	}
	for path, want := range cases {
		if got := instructionLikePath(path); got != want {
			t.Fatalf("instructionLikePath(%s) = %t, want %t", path, got, want)
		}
	}
}
