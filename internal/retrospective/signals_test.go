// Spec: docs/specs/task-retrospective.md
package retrospective

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// signalTrace supplies explicit intent/final records to boundary tests.
// Spec: docs/specs/task-retrospective.md via DetectSignals
func signalTrace(calls []CallRecord) *Trace {
	return &Trace{SchemaVersion: 1, Identities: Identities{RootSessionID: "root", SessionID: "root", ExecutionRoot: "/project"}, Calls: calls,
		Messages: []Message{{ID: "intent", Seq: 0, Role: "user", Text: "Research legitimately; expected red tests and polling are controls"}, {ID: "final", Seq: 10000, Role: "assistant", Kind: "final", Text: "done"}}}
}

// Spec: docs/specs/task-retrospective.md via DetectSignals
func detectForTest(t *testing.T, trace *Trace, cfg *SignalConfig) *SignalReport {
	t.Helper()
	input, err := json.Marshal(trace)
	if err != nil {
		t.Fatal(err)
	}
	r, err := DetectSignals(trace, input, cfg)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// TestSignalsFrozenFixtures pins observable evidence, not live provider choices.
// Spec: docs/specs/task-retrospective.md via DetectSignals
func TestSignalsFrozenFixtures(t *testing.T) {
	body, err := os.ReadFile("../../fixtures/retrospective/signals/frozen.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		Name          string                `json:"name"`
		Trace         Trace                 `json:"trace"`
		Expected      map[string][][]string `json:"expected"`
		TerminalCalls int                   `json:"terminal_calls"`
		RawFailures   int                   `json:"raw_failures"`
	}
	if err = json.Unmarshal(body, &fixtures); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range fixtures {
		t.Run(fixture.Name, func(t *testing.T) {
			r := detectForTest(t, &fixture.Trace, nil)
			actual := map[string][][]string{}
			for _, signal := range r.Signals {
				actual[signal.RuleID] = append(actual[signal.RuleID], signal.CallIDs)
				if signal.RuleVersion != 1 || len(signal.ID) != 64 {
					t.Fatal("unstable signal identity")
				}
			}
			if !reflect.DeepEqual(actual, fixture.Expected) || r.Counts.TerminalCalls != fixture.TerminalCalls || r.Counts.RawFailures != fixture.RawFailures {
				t.Fatalf("observed=%v calls=%d failures=%d", actual, r.Counts.TerminalCalls, r.Counts.RawFailures)
			}
			if r.RepairSafe || r.Dispatch.Eligible || r.Interpretation.Requests != 0 {
				t.Fatal("detector granted model/write authority")
			}
			if fixture.Name == "unknown-schemas-and-absent-prerequisites-never-invent-metrics" && (r.Abstentions["search-burst"]["unsupported_input_schema"] != 2 || r.Abstentions["helper-recreate"]["unsupported_input_schema"] != 1) {
				t.Fatalf("missing abstentions: %v", r.Abstentions)
			}
		})
	}
}

// TestSignalsThresholdsWindowsAndStableIDs checks below/exact/outside-window edges.
// Spec: docs/specs/task-retrospective.md via DetectSignals
func TestSignalsThresholdsWindowsAndStableIDs(t *testing.T) {
	calls := []CallRecord{}
	for i := 0; i < 4; i++ {
		calls = append(calls, CallRecord{ID: string(rune('a' + i)), Seq: i + 1, Tool: "poll", Status: StatusSuccess, Input: "same", Prereq: "known"})
	}
	below := detectForTest(t, signalTrace(calls[:2]), nil)
	if len(below.Signals) != 0 {
		t.Fatal("below threshold hit")
	}
	exact := detectForTest(t, signalTrace(calls[:3]), nil)
	later := detectForTest(t, signalTrace(calls), nil)
	if len(exact.Signals) != 1 || later.Signals[0].ID != exact.Signals[0].ID || later.Signals[0].EvidenceCalls != 4 || later.Signals[0].MatchingWindows != 2 {
		t.Fatal("unstable/incorrect window aggregation")
	}
	if exact.Signals[0].PrerequisiteFingerprint != "known" || exact.Signals[0].WindowStart != 1 || exact.Signals[0].WindowEnd != 3 {
		t.Fatal("missing exact prerequisite/window evidence")
	}
	cfg := DefaultSignalConfig()
	cfg.RepeatWindowCalls = 3
	calls[1].Input = "other"
	if len(detectForTest(t, signalTrace(calls), &cfg).Signals) != 0 {
		t.Fatal("outside-window match")
	}
	for _, fingerprint := range []string{"", "changed"} {
		calls[1].Input = "same"
		calls[1].Prereq = fingerprint
		if len(detectForTest(t, signalTrace(calls), nil).Signals) != 1 {
			t.Fatal("remaining three equal calls should still match")
		}
	}
	// Empty input, omitted input and changed prereqs cannot establish a triple.
	for _, field := range []string{"empty", "omitted", "changed"} {
		sample := append([]CallRecord{}, calls[:3]...)
		sample[1].Prereq = "known"
		switch field {
		case "empty":
			sample[1].Input = ""
		case "omitted":
			sample[1].InputOmitted = true
		case "changed":
			sample[1].Prereq = "other"
		}
		if len(detectForTest(t, signalTrace(sample), nil).Signals) != 0 {
			t.Fatal(field)
		}
	}
	bad := DefaultSignalConfig()
	bad.SignalCap = 101
	if _, err := DetectSignals(signalTrace(nil), nil, &bad); err == nil {
		t.Fatal("raised cap accepted")
	}
}

// TestSignalsEarlyContextRequiresObservedProvenance includes successful early read,
// failed read, unlinked provenance and unknown schema controls.
// Spec: docs/specs/task-retrospective.md via DetectSignals
func TestSignalsEarlyContextRequiresObservedProvenance(t *testing.T) {
	read := CallRecord{ID: "read", Seq: 1, Tool: "read", Status: StatusSuccess, Input: `{"filePath":"/project/AGENTS.md"}`, SourceRefs: []SourceRef{{Path: "/project/AGENTS.md", SHA256: strings.Repeat("a", 64)}}}
	calls := []CallRecord{read}
	for i := 0; i < 9; i++ {
		calls = append(calls, CallRecord{ID: string(rune('a' + i)), Seq: i + 2, Tool: "grep", Status: StatusSuccess, Input: `{"pattern":"research","path":"/project"}`})
	}
	for _, mode := range []string{"observed", "failed", "unlinked", "conflicting"} {
		sample := append([]CallRecord{}, calls...)
		switch mode {
		case "failed":
			sample[0].Status = StatusError
		case "unlinked":
			sample[0].SourceRefs = nil
		case "conflicting":
			sample[0].Input = `{"filePath":"/project/AGENTS.md","path":"/other"}`
		}
		r := detectForTest(t, signalTrace(sample), nil)
		found := false
		for _, s := range r.Signals {
			if s.RuleID == "late-context" {
				found = true
			}
		}
		if found != (mode != "observed") {
			t.Fatal(mode)
		}
	}
}

// TestSignalsCapsPrivacyAndInterval exercises report persistence, not just labels.
// Spec: docs/specs/task-retrospective.md via WriteSignalReport
func TestSignalsCapsPrivacyAndInterval(t *testing.T) {
	calls := []CallRecord{}
	for i := 0; i < 25; i++ {
		calls = append(calls, CallRecord{ID: string(rune('a' + i)), Seq: i + 1, Tool: "bash", Status: StatusFailed, Input: "TOKEN=CANARY-ASSIGNMENT go test", Result: []Fragment{{ID: "result", Text: "CANARY-ENV"}}, Env: map[string]string{"API_KEY": "CANARY-ENV"}})
	}
	trace := signalTrace(calls)
	trace.Messages = append(trace.Messages, Message{ID: "private", Seq: 4, Role: "assistant", Kind: "reasoning", Text: "CANARY-PRIVATE"})
	trace.Calls = append(trace.Calls, CallRecord{ID: "secret", Seq: 26, Tool: "read", Status: StatusError, Input: `{"filePath":"/project/.env"}`, Result: []Fragment{{ID: "secret-result", Text: "CANARY-FILE"}}}, CallRecord{ID: "binary", Seq: 27, Tool: "read", Status: StatusSuccess, Input: `{"filePath":"/project/data"}`, Result: []Fragment{{ID: "binary-result", Binary: true, Text: "CANARY-BINARY"}}})
	cfg := DefaultSignalConfig()
	cfg.SignalCap = 2
	cfg.EvidenceLimitBytes = 600
	r := detectForTest(t, trace, &cfg)
	if len(r.Signals) != 2 || r.Counts.Signals != 26 || r.Coverage.OmittedSignals != 24 || r.Coverage.OmittedContextRecords == 0 || r.Coverage.EvidenceComplete || r.RepairSafe {
		t.Fatalf("cap accounting %+v", r.Coverage)
	}
	file := filepath.Join(t.TempDir(), "report.json")
	if err := WriteSignalReport(file, r); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"CANARY-ENV", "CANARY-ASSIGNMENT", "CANARY-FILE", "CANARY-BINARY", "CANARY-PRIVATE"} {
		if strings.Contains(string(body), secret) {
			t.Fatal("leak: " + secret)
		}
	}
	// Call evidence cap preserves total counts without changing frozen identity.
	for i := range calls {
		calls[i].Status = StatusSuccess
		calls[i].Input = "poll"
		calls[i].Prereq = "same"
	}
	cfg = DefaultSignalConfig()
	cfg.SignalEvidenceCap = 2
	r = detectForTest(t, signalTrace(calls), &cfg)
	if r.Signals[0].EvidenceCalls != 25 || r.Signals[0].OmittedEvidenceCalls != 23 || r.Coverage.EvidenceComplete {
		t.Fatal("evidence cap hidden")
	}
	// Only resolved task interval contributes metrics, not a prior task's failure.
	trace = signalTrace([]CallRecord{{ID: "old", Seq: 1, Tool: "bash", Status: StatusFailed}, {ID: "new", Seq: 6, Tool: "bash", Status: StatusSuccess}})
	trace.TaskInterval = &TaskInterval{ID: "task", StartSeq: 5, EndSeq: 9}
	r = detectForTest(t, trace, nil)
	if r.Counts.TerminalCalls != 1 || r.Counts.RawFailures != 0 {
		t.Fatal("out-of-interval calls counted")
	}
}

// TestSignalsHelperPrivacyAndExactEvidence avoids fuzzy similarity or shell parsing.
// Spec: docs/specs/task-retrospective.md via DetectSignals
func TestSignalsHelperPrivacyAndExactEvidence(t *testing.T) {
	for _, input := range []string{`{"filePath":"/tmp/h.py","content":"TOKEN=CANARY-SECRET"}`, `{"filePath":"/tmp/../project/h.py","content":"print(1)"}`, `{"filePath":"/tmp/h.py","content":1}`, `{"filePath":"/tmp/h.py","content":null}`} {
		calls := []CallRecord{}
		for i := 0; i < 3; i++ {
			calls = append(calls, CallRecord{ID: string(rune('a' + i)), Seq: i, Tool: "write", Status: StatusSuccess, Input: input})
		}
		r := detectForTest(t, signalTrace(calls), nil)
		if len(r.Signals) != 0 {
			t.Fatal("unobserved/secret helper matched")
		}
	}
	trace := signalTrace(nil)
	trace.SchemaVersion = 2
	if _, err := DetectSignals(trace, nil, nil); err == nil {
		t.Fatal("invalid trace accepted")
	}
}

// TestSignalsEvidenceBoundsAndSchemaControls verifies actual serialized evidence,
// independent report identity and negative-schema abstentions without provider calls.
// Spec: docs/specs/task-retrospective.md via DetectSignals
func TestSignalsEvidenceBoundsAndSchemaControls(t *testing.T) {
	for _, input := range []string{`{"pattern":"x","path":null}`, `{"pattern":"x","path":4}`, `{"pattern":"x","include":false}`, `{"pattern":"x"} trailing`, `{"pattern":"x","pattern":"y"}`} {
		calls := []CallRecord{}
		for i := 0; i < 10; i++ {
			calls = append(calls, CallRecord{ID: string(rune('a' + i)), Seq: i + 1, Tool: "grep", Status: StatusSuccess, Input: input})
		}
		r := detectForTest(t, signalTrace(calls), nil)
		if len(r.Signals) != 0 || r.Abstentions["broad-search"]["unsupported_input_schema"] != 10 {
			t.Fatal("unknown search schema invented a metric")
		}
	}
	// Equal helper content at different observed paths is not recreation.
	calls := []CallRecord{}
	for i := 0; i < 3; i++ {
		input := `{"filePath":"/tmp/` + string(rune('a'+i)) + `.py","content":"print(1)"}`
		calls = append(calls, CallRecord{ID: string(rune('a' + i)), Seq: i + 1, Tool: "write", Status: StatusSuccess, Input: input})
	}
	if len(detectForTest(t, signalTrace(calls), nil).Signals) != 0 {
		t.Fatal("path evidence ignored")
	}
	for i := range calls {
		calls[i].Status = StatusError
		calls[i].Input = strings.Repeat("x", 500)
	}
	cfg := DefaultSignalConfig()
	cfg.EvidenceLimitBytes = 500
	r := detectForTest(t, signalTrace(calls), &cfg)
	body, _ := json.Marshal(r.Evidence)
	if len(body) > cfg.EvidenceLimitBytes || r.Coverage.OmittedContextRecords != 3 {
		t.Fatal("actual evidence byte cap not enforced")
	}
	file := filepath.Join(t.TempDir(), "signals.json")
	if err := WriteSignalReport(file, r); err != nil {
		t.Fatal(err)
	}
	if preview, _, err := LoadReport(file); err == nil && ineligible(preview) == "" {
		t.Fatal("preview can enter dispatch")
	}
	// Expected private-reasoning exclusion preserves complete observable evidence.
	trace := signalTrace(nil)
	trace.Messages = append(trace.Messages, Message{ID: "reasoning", Seq: 2, Role: "assistant", Kind: "reasoning", Text: "CANARY-PRIVATE"})
	r = detectForTest(t, trace, nil)
	if !r.Coverage.EvidenceComplete || r.Coverage.ExcludedRecords != 1 || r.Interpretation.Status != "skipped_no_candidates" {
		t.Fatal("reasoning exclusion fabricated an observable gap")
	}
	trace.Messages = trace.Messages[1:]
	r = detectForTest(t, trace, nil)
	if r.Coverage.EvidenceComplete {
		t.Fatal("missing intent treated as complete")
	}
}
