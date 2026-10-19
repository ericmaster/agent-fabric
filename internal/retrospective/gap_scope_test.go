// Spec: docs/specs/task-retrospective.md — explicit interval/gap coverage.
package retrospective

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Public replay cannot discard unscoped gaps, but exact old ranges must not
// poison a genuinely explicit later task. No prompt semantics infer boundaries.
// Spec: docs/specs/task-retrospective.md via Analyze
func TestScopedGapReplayCoverage(t *testing.T) {
	for _, mode := range []string{"historical", "current", "current-source", "unknown", "descendant", "outside_interval", "malformed", "no-boundary", "implicit-tail", "accepted-tail"} {
		t.Run(mode, func(t *testing.T) {
			doc := map[string]any{
				"schema_version": 1, "identities": map[string]any{"root_session_id": "root", "session_id": "root", "execution_root": "/fixture"},
				"task":     map[string]any{"id": "explicit-new-task", "start_seq": 10, "end_seq": 20},
				"messages": []map[string]any{{"id": "old-user", "seq": 1, "role": "user", "text": "old task"}, {"id": "new-user", "seq": 11, "role": "user", "text": "Check expected error"}, {"id": "final", "seq": 14, "role": "assistant", "kind": "final", "text": "DONE"}},
				"calls":    []map[string]any{{"id": "old-call", "seq": 2, "tool": "read", "status": "error", "input": "old"}, {"id": "new-call", "seq": 12, "tool": "read", "status": "error", "input": "current", "result": []map[string]any{{"id": "result", "text": "Expected failure"}}}},
			}
			gap := map[string]any{"kind": "partial_stream", "detail": "old gap password=CANARY-gap-detail", "session_id": "root", "record_id": "old-call", "start_seq": 2, "end_seq": 2}
			switch mode {
			case "current", "current-source":
				gap["record_id"] = "new-call"
				gap["start_seq"] = 12
				gap["end_seq"] = 12
				if mode == "current-source" {
					gap["kind"] = "missing_source"
				}
			case "unknown":
				delete(gap, "start_seq")
				delete(gap, "end_seq")
			case "descendant":
				gap["session_id"] = "child"
			case "outside_interval":
				gap["start_seq"] = 25
				gap["end_seq"] = 25
			case "malformed":
				gap["start_seq"] = 9
				gap["end_seq"] = 2
			case "no-boundary":
				delete(doc, "task")
			case "implicit-tail", "accepted-tail":
				delete(doc, "task")
				gap["start_seq"], gap["end_seq"] = 25, 25
				if mode == "accepted-tail" {
					doc["boundaries"] = []map[string]any{{"kind": "accepted_success", "after_seq": 10}}
				}
			}
			doc["gaps"] = []any{gap, gap}
			raw, _ := json.Marshal(doc)
			var trace Trace
			if err := json.Unmarshal(raw, &trace); err != nil {
				t.Fatal(err)
			}
			f := newLoopFixture(t, unknownScript(t))
			report, err := Analyze(context.Background(), f.cfg, &trace, raw, NewJevClient(f.cfg.Endpoint, "synthetic-key", f.cfg))
			if mode == "malformed" {
				if err == nil {
					t.Fatal("malformed gap scope accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			complete := mode == "historical" || mode == "outside_interval"
			if report.Coverage.EvidenceComplete != complete || (len(f.jev.recorded()) == 1) != complete || report.RepairSafe || report.Dispatch.Eligible {
				t.Fatalf("wrong scoped coverage: complete=%t requests=%d", report.Coverage.EvidenceComplete, len(f.jev.recorded()))
			}
			if complete && (report.Counts.RawFailures != 1 || len(report.Trace.SourceFiles) != 0) {
				t.Fatal("old failures leaked into metrics or provenance was fabricated")
			}
			body, _ := json.Marshal(report)
			if strings.Contains(string(body), "CANARY-") {
				t.Fatal("gap detail redaction failed")
			}
			var projected struct {
				Trace struct {
					Gaps []map[string]any `json:"gaps"`
				} `json:"trace"`
			}
			json.Unmarshal(body, &projected)
			if len(projected.Trace.Gaps) != 1 {
				t.Fatal("same identified gap repeated in one report")
			}
			if report.Coverage.DeduplicatedGaps != 1 {
				t.Fatal("duplicate gap count lost")
			}
			want := mode
			if mode == "current-source" {
				want = "current"
			}
			if mode == "descendant" {
				want = "unknown"
			}
			if mode == "no-boundary" || mode == "implicit-tail" || mode == "accepted-tail" {
				want = "current"
			}
			row := projected.Trace.Gaps[0]
			if row["scope"] != want || len(row["id"].(string)) != 64 {
				t.Fatal("missing honest gap scope/identity")
			}
			// A later report for the same observation keeps one stable identity;
			// a projection must count these IDs distinctly, not sum repeated rows.
			second, err := Analyze(context.Background(), f.cfg, &trace, raw, nil)
			if err != nil {
				t.Fatal(err)
			}
			secondBytes, _ := json.Marshal(second)
			var again struct {
				Trace struct {
					Gaps []map[string]any `json:"gaps"`
				} `json:"trace"`
			}
			json.Unmarshal(secondBytes, &again)
			if again.Trace.Gaps[0]["id"] != row["id"] {
				t.Fatal("gap identity changed across reports")
			}
		})
	}
}

// A genuine accepted boundary is established by the existing public drain on
// complete prior work, not fabricated to escape an incomplete task. Later capture
// of an old part gap must not poison the new accepted-success interval.
// Spec: docs/specs/task-retrospective.md via RunDrain
func TestScopedGapAcrossAcceptedDrainBoundary(t *testing.T) {
	for _, mode := range []string{"historical", "current", "unknown", "unknown-flood", "current-flood", "overflow"} {
		t.Run(mode, func(t *testing.T) {
			f := newLoopFixture(t, unknownScript(t))
			f.setEnabledFor(t, t.TempDir(), true)
			f.cfg.DispatchServer = ""
			admit := func(records []EnvelopeRecord) *Report {
				t.Helper()
				raw, _ := json.Marshal(&Envelope{Version: 1, Harness: HarnessOpenCode, SessionID: "root", Trigger: true, Records: records})
				envelope, err := DecodeEnvelope(bytes.NewReader(raw))
				if err != nil {
					t.Fatal(err)
				}
				if _, err := IngestEnvelope(IngestOptions{Config: f.cfg, Store: f.store}, envelope); err != nil {
					t.Fatal(err)
				}
				if _, err := RunDrain(context.Background(), DrainOptions{Config: f.cfg, ConfigPath: f.configPath, EnvKey: "synthetic-key"}); err != nil {
					t.Fatal(err)
				}
				job, err := f.store.ReadDrainJob("root")
				if err != nil {
					t.Fatal(err)
				}
				report, _, err := LoadReport(job.ReportPath)
				if err != nil {
					t.Fatal(err)
				}
				return report
			}
			old := admit([]EnvelopeRecord{
				{Kind: "session", SessionID: "root", Directory: f.root},
				{Kind: "message", RecordID: "old-user", Role: "user"},
				{Kind: "part", RecordID: "old-user:text", MessageID: "old-user", PartType: "text", Text: "old work"},
				{Kind: "message", RecordID: "old-final", Role: "assistant"},
				{Kind: "part", RecordID: "old-tool", MessageID: "old-final", PartType: "tool", Tool: "read", ToolStatus: "completed", Input: "old", Output: "okay"},
				{Kind: "part", RecordID: "old-final:text", MessageID: "old-final", PartType: "text", Text: "OLD DONE"},
			})
			spool, err := loadSpool(f.store, "root")
			if err != nil {
				t.Fatal(err)
			}
			accepted := spool.AcceptedAfterSeq
			if !old.Coverage.EvidenceComplete || accepted == 0 {
				t.Fatal("fixture did not establish a real accepted boundary")
			}
			newTool := EnvelopeRecord{Kind: "part", RecordID: "new-tool", MessageID: "new-final", PartType: "tool", Tool: "read", ToolStatus: "error", Input: "new", Error: "expected failure"}
			if mode == "current" || mode == "current-flood" {
				newTool.Detail = "current result missing"
			}
			records := []EnvelopeRecord{
				{Kind: "message", RecordID: "new-user", Role: "user"},
				{Kind: "part", RecordID: "new-user:text", MessageID: "new-user", PartType: "text", Text: "new work"},
				{Kind: "message", RecordID: "new-final", Role: "assistant"}, newTool,
				{Kind: "part", RecordID: "new-final:text", MessageID: "new-final", PartType: "text", Text: "DONE"},
			}
			if mode == "historical" {
				records = append(records, EnvelopeRecord{Kind: "part", RecordID: "old-tool", MessageID: "old-final", PartType: "tool", Detail: "old result missing"})
			}
			if mode == "unknown" || mode == "unknown-flood" {
				records = append(records, EnvelopeRecord{Kind: "removed", RecordID: "unknown-old-part"})
			}
			if mode == "overflow" {
				for i := 0; i < 801; i++ {
					records = append(records, EnvelopeRecord{Kind: "removed", RecordID: fmt.Sprintf("lost-%d", i)})
				}
			}
			if strings.HasSuffix(mode, "-flood") || mode == "overflow" {
				for i := 0; i < 800; i++ {
					records = append(records, EnvelopeRecord{Kind: "part", RecordID: "old-tool", MessageID: "old-final", PartType: "tool", Detail: "old result missing"})
				}
			}
			current := admit(records)
			if current.Trace.BoundarySource != "accepted-success" || current.Trace.Interval.StartSeq != accepted {
				t.Fatal("accepted interval mapping changed")
			}
			complete := mode == "historical"
			if current.Coverage.EvidenceComplete != complete || (len(f.jev.recorded()) == 1) != complete || current.Counts.RawFailures != 1 || current.RepairSafe {
				t.Fatal("wrong current-task gap routing")
			}
			spool, err = loadSpool(f.store, "root")
			if err != nil {
				t.Fatal(err)
			}
			if !complete && (spool.AcceptedAfterSeq != accepted || current.Dispatch.Eligible) {
				t.Fatal("incomplete task advanced accepted boundary")
			}
			if strings.HasSuffix(mode, "-flood") {
				if len(spool.Gaps) != 2 || current.Coverage.DeduplicatedGaps != 799 || current.Dispatch.Eligible {
					t.Fatal("duplicates consumed retention slots or counts were lost")
				}
			}
			if !strings.HasSuffix(mode, "-flood") && mode != "overflow" && (len(current.Trace.Gaps) != 1 || current.Trace.Gaps[0].Scope != mode) {
				t.Fatal("gap classification lost")
			}
			if mode == "overflow" {
				found := false
				for _, gap := range current.Trace.Gaps {
					if gap.Kind == "coverage_overflow" && gap.Scope == "unknown" {
						found = true
					}
				}
				if !found {
					t.Fatal("distinct gap eviction lacks sticky structured overflow")
				}
				// Reload via public ingest/drain; further historical duplicates cannot
				// erase loss, permit interpretation, or advance the accepted boundary.
				again := admit([]EnvelopeRecord{{Kind: "part", RecordID: "old-tool", MessageID: "old-final", PartType: "tool", Detail: "old result missing"}})
				if again.Coverage.EvidenceComplete || again.Dispatch.Eligible || len(f.jev.recorded()) != 0 {
					t.Fatal("overflow lost after restart")
				}
			}
		})
	}
}

// Legacy over-cap input, byte eviction and namespace collisions all traverse the
// same retention/normalization seams. No detail parsing or invented task start.
// Spec: docs/specs/task-retrospective.md via Spool.appendGap and resolveGaps
func TestGapRetentionLegacyLossAndNamespaces(t *testing.T) {
	store := testStore(t)
	cfg := observationConfig(store)
	start, end := 1, 1
	old := Gap{Kind: "partial_stream", SessionID: "root", RecordID: "old", StartSeq: &start, EndSeq: &end}
	s := &Spool{Schema: SpoolSchemaVersion, SessionID: "root"}
	s.Gaps = append(s.Gaps, Gap{Kind: "partial_stream", RecordID: "unknown"})
	for i := 0; i < 900; i++ {
		s.Gaps = append(s.Gaps, old)
	}
	// Simulate a legacy document that bypassed the new append seam.
	body, _ := json.Marshal(s)
	if err := writePrivate(spoolPath(store, "root"), body); err != nil {
		t.Fatal(err)
	}
	s, err := loadSpool(store, "root")
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Gaps) != 2 || s.Gaps[1].Duplicates != 899 || s.GapOmissions != 0 {
		t.Fatal("legacy duplicates evicted unknown evidence")
	}
	for i := 0; i < 801; i++ {
		s.appendGap(Gap{Kind: "partial_stream", RecordID: fmt.Sprintf("unique-%d", i)})
	}
	if s.GapOmissions != 3 || len(s.Gaps) != 800 {
		t.Fatal("distinct eviction not counted")
	}
	if err := writeSpoolBounded(cfg, store, s); err != nil {
		t.Fatal(err)
	}
	s, err = loadSpool(store, "root")
	if err != nil {
		t.Fatal(err)
	}
	trace := assembleTrace(store, s, "root")
	trace.TaskInterval = &TaskInterval{ID: "explicit-later", StartSeq: 1000, EndSeq: 1001}
	n, err := Normalize(trace)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, g := range n.Gaps {
		if g.Kind == "coverage_overflow" {
			found = g.Scope == "unknown" && g.Omitted == 3
		}
	}
	if !found {
		t.Fatal("overflow was lost or classified historical after explicit boundary")
	}
	legacy := &Spool{Schema: 1, SessionID: "legacy"}
	for i := 0; i < 801; i++ {
		g := old
		g.SessionID = "legacy"
		g.RecordID = fmt.Sprintf("legacy-%d", i)
		legacy.Gaps = append(legacy.Gaps, g)
	}
	legacyTrace := assembleTrace(store, legacy, "legacy")
	if len(legacy.Gaps) != 801 || len(legacyTrace.Gaps) != 801 || legacyTrace.Gaps[800].Kind != "coverage_overflow" || legacyTrace.Gaps[800].Omitted != 1 {
		t.Fatal("direct legacy assembly discarded distinct evidence without marker")
	}
	if err := writeSpoolBounded(cfg, store, legacy); err != nil {
		t.Fatal(err)
	}
	legacy, err = loadSpool(store, "legacy")
	if err != nil || legacy.GapOmissions != 1 {
		t.Fatal("legacy loss counter not serialized", err)
	}
	// Even a supplied range cannot downgrade the reserved overflow kind.
	trace.Gaps = []Gap{{Kind: "coverage_overflow", SessionID: "root", StartSeq: &start, EndSeq: &end, Omitted: 1}}
	n, err = Normalize(trace)
	if err != nil || n.Gaps[0].Scope != "unknown" {
		t.Fatal("overflow accepted a historical range", err)
	}
	// Root IDs cannot collide with a child that claims root ownership; local
	// sequence spaces are never interchangeable.
	trace.Gaps = []Gap{old}
	trace.Descendants = []*Trace{{SchemaVersion: 1, Identities: Identities{RootSessionID: "root", SessionID: "child", ExecutionRoot: "/fixture"}, Gaps: []Gap{old}}}
	n, err = Normalize(trace)
	if err != nil {
		t.Fatal(err)
	}
	if len(n.Gaps) != 2 || n.Gaps[0].ID == n.Gaps[1].ID || n.Gaps[1].Scope != "unknown" {
		t.Fatal("descendant aliased root gap identity")
	}
	childGap := old
	childGap.SessionID = "child"
	childGap.Duplicates = 5
	trace.Gaps = []Gap{childGap}
	trace.Descendants[0].Gaps = []Gap{childGap}
	n, err = Normalize(trace)
	if err != nil || len(n.Gaps) != 1 || n.Gaps[0].Scope != "unknown" || n.DeduplicatedGaps != 5 {
		t.Fatal("public child projection double-counted observations", err)
	}
	grandchild := *trace.Descendants[0]
	grandchild.Identities.SessionID = "grandchild"
	grandchildGap := childGap
	grandchildGap.SessionID = "grandchild"
	grandchild.Gaps = []Gap{grandchildGap}
	trace.Gaps = []Gap{grandchildGap}
	trace.Descendants[0].Gaps = []Gap{grandchildGap}
	trace.Descendants[0].Descendants = []*Trace{&grandchild}
	n, err = Normalize(trace)
	if err != nil || len(n.Gaps) != 1 || n.DeduplicatedGaps != 5 || n.Gaps[0].Scope != "unknown" {
		t.Fatal("subtree projections double-counted observations", err)
	}
	trace.Descendants[0].Descendants = nil
	trace.Gaps = nil
	trace.Descendants[0].Gaps = []Gap{{Kind: "coverage_overflow", Omitted: 2}}
	n, err = Normalize(trace)
	if err != nil || len(n.Gaps) != 1 || n.Gaps[0].Scope != "unknown" || n.Gaps[0].Omitted != 2 {
		t.Fatal("descendant loss marker disappeared", err)
	}
	events := &Spool{Schema: 1, SessionID: "events"}
	events.appendEvent(SpoolEvent{SessionID: "events", RecordID: "compaction", Event: "session.compacted"})
	for i := 0; i < SpoolEventsRetained; i++ {
		events.appendEvent(SpoolEvent{SessionID: "events", Event: "session.idle"})
	}
	if events.GapOmissions != 1 {
		t.Fatal("compaction eviction lost coverage evidence")
	}
	if gaps := assembleTrace(store, events, "events").Gaps; len(gaps) != 1 || gaps[0].Kind != "coverage_overflow" {
		t.Fatal("evicted compaction silently became complete")
	}
	// A tiny byte limit can evict all diagnostic rows but not fixed loss evidence.
	s = &Spool{Schema: 1, SessionID: "bytes", Gaps: []Gap{{Kind: "partial_stream", Detail: strings.Repeat("x", 2048)}}}
	cfg.SpoolMaxBytes = 256
	if err := writeSpoolBounded(cfg, store, s); err != nil {
		t.Fatal(err)
	}
	s, err = loadSpool(store, "bytes")
	if err != nil {
		t.Fatal(err)
	}
	if s.GapOmissions != 1 || len(s.Gaps) != 0 {
		t.Fatal("byte eviction lost structured counter")
	}
	if gaps := assembleTrace(store, s, "bytes").Gaps; len(gaps) != 1 || gaps[0].Omitted != 1 {
		t.Fatal("byte overflow not projected")
	}
	// A bound unable to fit the fixed counters refuses without replacing disk.
	before, _ := os.ReadFile(spoolPath(store, "bytes"))
	cfg.SpoolMaxBytes = 1
	if err := writeSpoolBounded(cfg, store, s); err == nil {
		t.Fatal("loss counter silently discarded to fit")
	}
	after, _ := os.ReadFile(spoolPath(store, "bytes"))
	if !bytes.Equal(before, after) {
		t.Fatal("failed write changed prior state")
	}
}

// Identified observations are distinct by producer/sequence, while anonymous
// legacy diagnostics must never be merged by free-text equality.
// Spec: docs/specs/task-retrospective.md via resolveGaps
func TestGapIdentityPreservesDistinctAndAnonymousIssues(t *testing.T) {
	trace := signalTrace(nil)
	start, end := 1, 1
	a := Gap{Kind: "partial_stream", SessionID: "root", RecordID: "a", StartSeq: &start, EndSeq: &end}
	b := a
	b.RecordID = "b"
	legacy := Gap{Kind: "partial_stream", Detail: "same unscoped legacy text"}
	trace.Gaps = []Gap{a, a, b, legacy, legacy}
	n, err := Normalize(trace)
	if err != nil {
		t.Fatal(err)
	}
	if len(n.Gaps) != 4 || n.DeduplicatedGaps != 1 || n.Gaps[0].ID == n.Gaps[1].ID || n.Gaps[2].ID != "" || n.Gaps[3].ID != "" {
		t.Fatal("distinct or unknown gaps merged")
	}
	stable := n.Gaps[0].ID
	trace.TaskInterval = &TaskInterval{ID: "explicit", StartSeq: 10, EndSeq: 20}
	current := a
	currentStart, currentEnd := 12, 12
	current.StartSeq, current.EndSeq = &currentStart, &currentEnd
	trace.Gaps = []Gap{a, current}
	n, err = Normalize(trace)
	if err != nil || len(n.Gaps) != 2 || n.Gaps[0].ID != stable || n.Gaps[0].Scope != "historical" || n.Gaps[1].Scope != "current" || n.Gaps[0].ID == n.Gaps[1].ID {
		t.Fatal("changed sequence observation merged with historical identity", err)
	}
}

// Public ingestion/drain keeps old bad work incomplete. Only a separately
// supplied authoritative host interval can narrow the assembled trace for replay.
// Spec: docs/specs/task-retrospective.md via IngestEnvelope, RunDrain and Analyze
func TestGapIngestDrainRequiresExplicitBoundary(t *testing.T) {
	f := newLoopFixture(t, unknownScript(t))
	f.setEnabledFor(t, t.TempDir(), true)
	f.cfg.DispatchServer = "" // no history transport or real service
	admit := func(records []EnvelopeRecord) {
		t.Helper()
		raw, _ := json.Marshal(&Envelope{Version: 1, Harness: HarnessOpenCode, SessionID: "root", Trigger: true, Records: records})
		envelope, err := DecodeEnvelope(bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		_, err = IngestEnvelope(IngestOptions{Config: f.cfg, Store: f.store}, envelope)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := RunDrain(context.Background(), DrainOptions{Config: f.cfg, ConfigPath: f.configPath, EnvKey: "synthetic-key"}); err != nil {
			t.Fatal(err)
		}
	}
	admit([]EnvelopeRecord{
		{Kind: "session", SessionID: "root", Directory: f.root},
		{Kind: "message", RecordID: "old-user", Role: "user"},
		{Kind: "part", RecordID: "old-user:text", MessageID: "old-user", PartType: "text", Text: "old task"},
		{Kind: "message", RecordID: "old-final", Role: "assistant"},
		{Kind: "part", RecordID: "old-tool", MessageID: "old-final", PartType: "tool", Tool: "read", ToolStatus: "error", Input: "old", Error: "expected", Detail: "unsupported tool result content withheld"},
		{Kind: "part", RecordID: "old-final:text", MessageID: "old-final", PartType: "text", Text: "OLD"},
		{Kind: "event", RecordID: "old-compaction", Event: "session.compacted"},
	})
	job, err := f.store.ReadDrainJob("root")
	if err != nil {
		t.Fatal(err)
	}
	oldPath := job.ReportPath
	oldBytes, err := os.ReadFile(oldPath)
	if err != nil {
		t.Fatal(err)
	}
	oldReport, _, err := LoadReport(oldPath)
	if err != nil || oldReport.Coverage.EvidenceComplete {
		t.Fatal("old gap was not material", err)
	}
	admit([]EnvelopeRecord{
		{Kind: "message", RecordID: "new-user", Role: "user"},
		{Kind: "part", RecordID: "new-user:text", MessageID: "new-user", PartType: "text", Text: "New work explicitly supplied later by coordinator"},
		{Kind: "message", RecordID: "new-final", Role: "assistant"},
		{Kind: "part", RecordID: "new-tool", MessageID: "new-final", PartType: "tool", Tool: "read", ToolStatus: "error", Input: "new", Error: "expected failure"},
		{Kind: "part", RecordID: "new-final:text", MessageID: "new-final", PartType: "text", Text: "DONE"},
		{Kind: "event", RecordID: "old-compaction", Event: "session.compacted"},
	})
	spool, err := loadSpool(f.store, "root")
	if err != nil {
		t.Fatal(err)
	}
	if spool.AcceptedAfterSeq != 0 || len(spool.Events) != 1 {
		t.Fatal("incomplete task gained success boundary or repeated known compaction")
	}
	if spool.LastSeq != spool.Parts["new-final:text"].Seq {
		t.Fatal("duplicate native event consumed another sequence")
	}
	trace := assembleTrace(f.store, spool, "root")
	raw, _ := json.Marshal(trace)
	whole, err := Analyze(context.Background(), f.cfg, trace, raw, nil)
	if err != nil || whole.Coverage.EvidenceComplete || whole.Trace.BoundarySource != "session-start" {
		t.Fatal("follow-up inferred a new task", err)
	}
	// This is explicit test-host authority through the existing trace schema,
	// not an inferred boundary persisted by ingest/drain or a fabricated success.
	trace.TaskInterval = &TaskInterval{ID: "coordinator-new-task", StartSeq: spool.Parts["new-user:text"].Seq}
	raw, _ = json.Marshal(trace)
	current, err := Analyze(context.Background(), f.cfg, trace, raw, NewJevClient(f.cfg.Endpoint, "synthetic-key", f.cfg))
	if err != nil || !current.Coverage.EvidenceComplete || len(f.jev.recorded()) != 1 || current.Counts.RawFailures != 1 {
		t.Fatal("known historical gaps poisoned explicit current task", err)
	}
	if len(current.Trace.SourceFiles) != 0 || current.RepairSafe {
		t.Fatal("invented source/write authority")
	}
	file := filepath.Join(t.TempDir(), "current.json")
	if err := WriteReport(file, current); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadReport(file); err != nil {
		t.Fatal(err)
	}
	unchanged, err := os.ReadFile(oldPath)
	if err != nil || string(unchanged) != string(oldBytes) {
		t.Fatal("old report bytes were rewritten", err)
	}
}
