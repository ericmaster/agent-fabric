// Spec: docs/specs/task-retrospective.md
package retrospective

import (
	"testing"
)

// normalizeOK is a helper that fails the test on any normalization error.
// Spec: docs/specs/task-retrospective.md via Normalize
func normalizeOK(t *testing.T, trace *Trace) *Normalized {
	t.Helper()
	normalized, err := Normalize(trace)
	if err != nil {
		t.Fatalf("Normalize failed: %v", err)
	}
	return normalized
}

func TestNormalizeRejectsUnsupportedSchemaVersion(t *testing.T) {
	if _, err := Normalize(&Trace{SchemaVersion: 2}); err == nil {
		t.Fatal("unsupported schema version was accepted")
	}
}

func TestNormalizeRejectsIncompleteIdentities(t *testing.T) {
	trace := &Trace{SchemaVersion: 1}
	if _, err := Normalize(trace); err == nil {
		t.Fatal("missing identities were accepted")
	}
	trace = &Trace{
		SchemaVersion: 1,
		Identities:    Identities{RootSessionID: "root", SessionID: "other", ExecutionRoot: "/x"},
	}
	if _, err := Normalize(trace); err == nil {
		t.Fatal("non-root trace was accepted")
	}
}

func TestNormalizeRejectsInvalidRecordStatuses(t *testing.T) {
	trace := &Trace{
		SchemaVersion: 1,
		Identities:    Identities{RootSessionID: "root", SessionID: "root", ExecutionRoot: "/x"},
		Calls:         []CallRecord{{ID: "c1", Seq: 1, Tool: "bash", Status: "exploded"}},
	}
	if _, err := Normalize(trace); err == nil {
		t.Fatal("invalid call status was accepted")
	}
}

func TestNormalizeRejectsIdKindCollisions(t *testing.T) {
	trace := &Trace{
		SchemaVersion: 1,
		Identities:    Identities{RootSessionID: "root", SessionID: "root", ExecutionRoot: "/x"},
		Messages:      []Message{{ID: "x", Seq: 1, Role: "user"}},
		Calls:         []CallRecord{{ID: "x", Seq: 2, Tool: "bash", Status: StatusSuccess}},
	}
	if _, err := Normalize(trace); err == nil {
		t.Fatal("conflicting id kinds were accepted")
	}
}

func TestNormalizeOrdersStablyAndDeduplicates(t *testing.T) {
	trace := &Trace{
		SchemaVersion: 1,
		Identities:    Identities{RootSessionID: "root", SessionID: "root", ExecutionRoot: "/x"},
		Messages: []Message{
			{ID: "m3", Seq: 4, Role: "user", Text: "third"},
			{ID: "m1", Seq: 2, Role: "user", Text: "first"},
			{ID: "m1", Seq: 2, Role: "user", Text: "dup of first"},
		},
		Calls: []CallRecord{
			{ID: "c2", Seq: 3, Tool: "bash", Status: StatusSuccess, Input: "echo hi", Result: []Fragment{{ID: "c2-r1", Text: "hi"}}},
		},
	}
	normalized := normalizeOK(t, trace)
	if got := []string{normalized.Records[0].ID, normalized.Records[1].ID, normalized.Records[2].ID}; got[0] != "m1" || got[1] != "c2" || got[2] != "m3" {
		t.Fatalf("stable order violated: %v", got)
	}
	if normalized.Deduplicated != 1 {
		t.Fatalf("dedup count = %d, want 1", normalized.Deduplicated)
	}
}

func TestHostTaskIntervalPreferredOverBoundaries(t *testing.T) {
	trace := &Trace{
		SchemaVersion: 1,
		Identities:    Identities{RootSessionID: "root", SessionID: "root", ExecutionRoot: "/x"},
		TaskInterval:  &TaskInterval{ID: "host-task-1", StartSeq: 3, EndSeq: 6},
		Boundaries:    []Boundary{{Kind: "accepted_success", AfterSeq: 8}},
		Messages: []Message{
			{ID: "m1", Seq: 1, Role: "user", Text: "old"},
			{ID: "m2", Seq: 4, Role: "user", Text: "task"},
			{ID: "m3", Seq: 6, Role: "assistant", Kind: "final", Text: "done"},
			{ID: "m4", Seq: 9, Role: "user", Text: "next"},
		},
	}
	normalized := normalizeOK(t, trace)
	if normalized.BoundarySource != "host" {
		t.Fatalf("boundary source = %q", normalized.BoundarySource)
	}
	var ids []string
	for _, record := range normalized.Records {
		if normalized.InInterval(record) {
			ids = append(ids, record.ID)
		}
	}
	if len(ids) != 2 || ids[0] != "m2" || ids[1] != "m3" {
		t.Fatalf("host interval = %v, want [m2 m3]", ids)
	}
}

func TestAcceptedSuccessBoundaryResolvesInterval(t *testing.T) {
	trace := &Trace{
		SchemaVersion: 1,
		Identities:    Identities{RootSessionID: "root", SessionID: "root", ExecutionRoot: "/x"},
		Boundaries:    []Boundary{{Kind: "accepted_success", AfterSeq: 1}},
		Messages: []Message{
			{ID: "m1", Seq: 1, Role: "user", Text: "previous task"},
			{ID: "m2", Seq: 2, Role: "user", Text: "current task"},
			{ID: "m3", Seq: 3, Role: "assistant", Kind: "final", Text: "done"},
		},
	}
	normalized := normalizeOK(t, trace)
	if normalized.BoundarySource != "accepted-success" {
		t.Fatalf("boundary source = %q", normalized.BoundarySource)
	}
}

func TestNewestAcceptedSuccessBoundaryWins(t *testing.T) {
	trace := &Trace{
		SchemaVersion: 1,
		Identities:    Identities{RootSessionID: "root", SessionID: "root", ExecutionRoot: "/x"},
		Boundaries: []Boundary{
			{Kind: "accepted_success", AfterSeq: 1},
			{Kind: "accepted_success", AfterSeq: 4},
		},
		Messages: []Message{
			{ID: "m1", Seq: 1, Role: "user", Text: "a"},
			{ID: "m2", Seq: 3, Role: "user", Text: "b"},
			{ID: "m3", Seq: 4, Role: "user", Text: "c"},
			{ID: "m4", Seq: 5, Role: "assistant", Kind: "final", Text: "done"},
		},
	}
	normalized := normalizeOK(t, trace)
	if normalized.FinalResponse().ID != "m4" {
		t.Fatal("wrong interval after newest boundary")
	}
	if normalized.InInterval(&Record{ID: "m2", Seq: 3}) {
		t.Fatal("record before newest accepted-success boundary stayed in interval")
	}
}

func TestAmbiguousMultiTaskBoundaryIsMaterial(t *testing.T) {
	trace := &Trace{
		SchemaVersion: 1,
		Identities:    Identities{RootSessionID: "root", SessionID: "root", ExecutionRoot: "/x"},
		Boundaries: []Boundary{
			{Kind: "task_start", AfterSeq: 2},
			{Kind: "task_start", AfterSeq: 5},
		},
		Messages: []Message{
			{ID: "m1", Seq: 1, Role: "user"},
			{ID: "m2", Seq: 3, Role: "assistant", Kind: "final"},
			{ID: "m3", Seq: 6, Role: "assistant", Kind: "final"},
		},
	}
	normalized := normalizeOK(t, trace)
	material := false
	for _, reason := range normalized.Reasons {
		if reason.Kind == ReasonAmbiguousBoundary {
			material = true
		}
	}
	if !material {
		t.Fatalf("ambiguous multi-task interval not flagged: %+v", normalized.Reasons)
	}
}

func TestIncompleteReasonSetsBlockCompleteTrace(t *testing.T) {
	base := Identities{RootSessionID: "root", SessionID: "root", ExecutionRoot: "/x"}
	cases := []struct {
		name   string
		trace  *Trace
		reason string
	}{
		{
			name: "unresolved running tool",
			trace: &Trace{
				SchemaVersion: 1,
				Identities:    base,
				Calls:         []CallRecord{{ID: "c1", Seq: 1, Tool: "bash", Status: StatusRunning}},
				Messages:      []Message{{ID: "m1", Seq: 2, Role: "assistant", Kind: "final", Text: "done"}},
			},
			reason: ReasonRunningTool,
		},
		{
			name: "missing final response",
			trace: &Trace{
				SchemaVersion: 1,
				Identities:    base,
				Calls:         []CallRecord{{ID: "c1", Seq: 1, Tool: "bash", Status: StatusSuccess, Input: "x", Result: []Fragment{{ID: "c1-r1"}}}},
				Messages:      []Message{{ID: "m1", Seq: 2, Role: "user"}},
			},
			reason: ReasonMissingFinal,
		},
		{
			name: "missing linked child trace",
			trace: &Trace{
				SchemaVersion: 1,
				Identities:    base,
				ChildRefs:     []string{"child-9"},
				Messages:      []Message{{ID: "m1", Seq: 1, Role: "assistant", Kind: "final", Text: "done"}},
			},
			reason: ReasonMissingChild,
		},
		{
			name: "compacted-away evidence",
			trace: &Trace{
				SchemaVersion: 1,
				Identities:    base,
				Gaps:          []Gap{{Kind: "compacted", Detail: "history compacted"}},
				Messages:      []Message{{ID: "m1", Seq: 1, Role: "assistant", Kind: "final", Text: "done"}},
			},
			reason: ReasonCompacted,
		},
		{
			name: "terminal abort",
			trace: &Trace{
				SchemaVersion:  1,
				Identities:     base,
				TerminalStatus: TaskStatusAborted,
				Messages:       []Message{{ID: "m1", Seq: 1, Role: "assistant", Kind: "final", Text: "done"}},
			},
			reason: ReasonTerminalAbortError,
		},
	}
	for _, tested := range cases {
		t.Run(tested.name, func(t *testing.T) {
			normalized := normalizeOK(t, tested.trace)
			if normalized.Complete() {
				t.Fatalf("incomplete trace passed the gate: %+v", normalized.Reasons)
			}
			found := false
			for _, reason := range normalized.Reasons {
				if reason.Kind == tested.reason {
					found = true
				}
			}
			if !found {
				t.Fatalf("expected reason %q missing: %+v", tested.reason, normalized.Reasons)
			}
		})
	}
}

func TestLinkedDescendantRecordsCoveredOnce(t *testing.T) {
	child := &Trace{
		SchemaVersion: 1,
		Identities:    Identities{RootSessionID: "root", SessionID: "child-1", ExecutionRoot: "/x"},
		Messages:      []Message{{ID: "cm1", Seq: 10, Role: "user", Text: "child says"}},
		Calls:         []CallRecord{{ID: "cc1", Seq: 11, Tool: "bash", Status: StatusSuccess, Input: "ls", Result: []Fragment{{ID: "cc1-r1", Text: "files"}}}},
	}
	trace := &Trace{
		SchemaVersion: 1,
		Identities:    Identities{RootSessionID: "root", SessionID: "root", ExecutionRoot: "/x"},
		ChildRefs:     []string{"child-1"},
		Descendants:   []*Trace{child},
		Messages: []Message{
			{ID: "m1", Seq: 1, Role: "user"},
			{ID: "m2", Seq: 2, Role: "assistant", Kind: "final", Text: "done"},
			{ID: "cm1", Seq: 10, Role: "assistant", Kind: "action", Text: "duplicate in root"}, // duplicate id, first occurrence is the child's? No: the root record here appears first in walk order.
		},
	}
	normalized := normalizeOK(t, trace)
	counted := map[string]int{}
	for _, record := range normalized.Records {
		counted[record.ID]++
	}
	if counted["cm1"] != 1 {
		t.Fatalf("record cm1 delivered %d times", counted["cm1"])
	}
}

func TestStepDerivationLeavesOutExcludedRecords(t *testing.T) {
	trace := &Trace{
		SchemaVersion: 1,
		Identities:    Identities{RootSessionID: "root", SessionID: "root", ExecutionRoot: "/x"},
		Messages: []Message{
			{ID: "m1", Seq: 1, Role: "user", Text: "todo"},
			{ID: "m2", Seq: 2, Role: "assistant", Kind: "reasoning", Text: "hidden chain"},
			{ID: "m3", Seq: 3, Role: "assistant", Kind: "action", Text: "action one"},
			{ID: "m4", Seq: 4, Role: "assistant", Kind: "final", Text: "done"},
		},
		Calls: []CallRecord{
			{ID: "c1", Seq: 3, Tool: "bash", Status: StatusSuccess, Input: "steps", Result: []Fragment{{ID: "c1-r1", Text: "x"}}},
			// unpaired call: terminal but without paired result — not a step.
			{ID: "c2", Seq: 4, Tool: "bash", Status: StatusSuccess, Input: "naked"},
		},
	}
	normalized := normalizeOK(t, trace)
	var stepIDs []string
	for _, step := range normalized.Steps {
		stepIDs = append(stepIDs, step.ID)
	}
	if len(stepIDs) != 2 || stepIDs[0] != "c1" || stepIDs[1] != "m3" {
		t.Fatalf("steps = %v, want [c1 m3] (excludes user/final/reasoning/unpaired)", stepIDs)
	}
}

func TestLastTenStepsStableSliceOfSize(t *testing.T) {
	trace := &Trace{
		SchemaVersion: 1,
		Identities:    Identities{RootSessionID: "root", SessionID: "root", ExecutionRoot: "/x"},
		Messages:      []Message{{ID: "m99", Seq: 99, Role: "assistant", Kind: "final", Text: "done"}},
	}
	for i := 0; i < 14; i++ {
		id := "c" + string(rune('a'+i))
		trace.Calls = append(trace.Calls, CallRecord{
			ID: id, Seq: 10 + i, Tool: "bash", Status: StatusSuccess,
			Input: "make " + id, Result: []Fragment{{ID: id + "-r1", Text: "ok"}},
		})
	}
	normalized := normalizeOK(t, trace)
	lastTen := normalized.LastSteps(10)
	if len(lastTen) != 10 {
		t.Fatalf("last steps = %d", len(lastTen))
	}
	if lastTen[0].ID != "ce" || lastTen[9].ID != "cn" {
		t.Fatalf("wrong tail slice: %q and %q", lastTen[0].ID, lastTen[9].ID)
	}
	fewer := normalized.LastSteps(3)
	if len(fewer) != 3 || fewer[0].ID != "cl" || fewer[2].ID != "cn" {
		t.Fatalf("fewer slice wrong: %+v", fewer)
	}
}

func TestTerminalAbortedCallIsNotStepButRawError(t *testing.T) {
	nonzero := 2
	trace := &Trace{
		SchemaVersion: 1,
		Identities:    Identities{RootSessionID: "root", SessionID: "root", ExecutionRoot: "/x"},
		Messages:      []Message{{ID: "m1", Seq: 2, Role: "assistant", Kind: "final", Text: "recovered"}},
		Calls: []CallRecord{
			{ID: "c1", Seq: 1, Tool: "bash", Status: StatusSuccess, ExitCode: &nonzero, Input: "cmd", Result: []Fragment{{ID: "c1-r1", Text: "noise"}}},
		},
	}
	normalized := normalizeOK(t, trace)
	if !normalized.Records[0].RawError() {
		t.Fatal("nonzero exit code was not recognized as a raw error")
	}
	if len(normalized.Steps) != 1 || normalized.Steps[0].ID != "c1" {
		t.Fatal("paired tool call should stay a step")
	}
	if len(normalized.Reasons) != 0 {
		t.Fatalf("raw terminal errors are counted, never a completeness gate: %+v", normalized.Reasons)
	}
}

func TestAbortedTerminalStatusRemainsObservedAbort(t *testing.T) {
	trace := signalTrace(nil)
	trace.TerminalStatus = TaskStatusAborted
	r := detectForTest(t, trace, nil)
	if r.TerminalOutcome.Status != TaskStatusAborted || !r.TerminalOutcome.Observed {
		t.Fatal("aborted task was reinterpreted")
	}
}
