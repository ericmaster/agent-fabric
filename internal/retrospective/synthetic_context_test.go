// Spec: docs/specs/task-retrospective.md — native durable-input context contract.
package retrospective

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

// Normalization, retention and packet selection share one typed source kind.
// Spec: docs/specs/task-retrospective.md via Analyze
func TestSyntheticContextIntervalDedupCapsAndCompatibility(t *testing.T) {
	for _, mode := range []string{"valid", "duplicate", "interval", "cap", "hidden", "missing-intent", "bad-context-kind", "bad-human-kind", "legacy"} {
		t.Run(mode, func(t *testing.T) {
			trace := &Trace{SchemaVersion: 1, Identities: Identities{RootSessionID: "root", SessionID: "root", ExecutionRoot: "/fixture"}, Messages: []Message{
				{ID: "user", Seq: 1, Role: "user", Text: "Check the expected error"},
				{ID: "context", Seq: 2, Role: "context", Kind: "synthetic", Text: "This error is an expected negative control"},
				{ID: "final", Seq: 4, Role: "assistant", Kind: "final", Text: "DONE"},
				// Newer context must not select a final or become productive work.
				{ID: "tail-context", Seq: 5, Role: "context", Kind: "synthetic", Text: "No repair requested"},
			}, Calls: []CallRecord{{ID: "call", Seq: 3, Tool: "read", Status: StatusError, Input: `{"path":"wrong/docs"}`, Result: []Fragment{{ID: "result", Text: "File not found"}}}}}
			switch mode {
			case "duplicate":
				trace.Messages = append(trace.Messages, trace.Messages[1])
			case "interval":
				trace.Messages = append(trace.Messages, Message{ID: "old-context", Seq: 0, Role: "context", Kind: "synthetic", Text: "OUTSIDE-INTERVAL"})
				trace.TaskInterval = &TaskInterval{ID: "task", StartSeq: 1}
			case "cap":
				trace.Messages[1].Text = strings.Repeat("x", DefaultSignalConfig().EvidenceLimitBytes+1)
			case "hidden":
				trace.Messages[1].Hidden = true
			case "missing-intent":
				trace.Messages = trace.Messages[1:]
			case "bad-context-kind":
				trace.Messages[1].Kind = "final"
			case "bad-human-kind":
				trace.Messages[1].Role = "user"
			case "legacy":
				trace.Messages = []Message{trace.Messages[0], trace.Messages[2]}
			}
			n, err := Normalize(trace)
			if mode == "bad-context-kind" || mode == "bad-human-kind" {
				if err == nil {
					t.Fatal("context was coerced into human intent/final")
				}
				return
			}
			if err != nil || n.FinalResponse() == nil || n.FinalResponse().ID != "final" || len(n.Steps) != 1 {
				t.Fatalf("context changed final/productive steps: %v", err)
			}
			if mode == "duplicate" && n.Deduplicated != 1 {
				t.Fatal("context deduplication lost")
			}
			raw, _ := json.Marshal(trace)
			report, err := Analyze(context.Background(), DefaultConfig(), trace, raw, nil)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "cap" || mode == "hidden" || mode == "missing-intent" {
				if report.Coverage.EvidenceComplete || report.Status != ReportStatusIncomplete || report.Usage.Requests != 0 {
					t.Fatal("missing/withheld context became complete or semantic authority")
				}
				return
			}
			if !report.Coverage.EvidenceComplete || len(report.Signals) != 1 {
				t.Fatal("valid legacy/context trace lost coverage")
			}
			packet, _ := packetFor(n, report, report.Signals[0], 0)
			body, _ := json.Marshal(packet)
			if strings.Contains(string(body), "OUTSIDE-INTERVAL") {
				t.Fatal("out-of-interval context delivered")
			}
			count := 0
			for _, record := range packet.Records {
				if record.Kind != "synthetic_context" {
					continue
				}
				count++
				if record.SourceKind != "opencode.synthetic" || record.SessionID != "root" || len(record.SourceRefs) != 0 {
					t.Fatal("context provenance was lost/fabricated")
				}
				// Prior report decoding is strict. Its evidence shape did not
				// know source_kind, so it cannot silently accept new context.
				var legacy struct {
					ID        string `json:"id"`
					Kind      string `json:"kind"`
					SessionID string `json:"session_id"`
					Text      string `json:"text"`
				}
				encoded, _ := json.Marshal(record)
				decoder := json.NewDecoder(bytes.NewReader(encoded))
				decoder.DisallowUnknownFields()
				if decoder.Decode(&legacy) == nil {
					t.Fatal("pre-extension report shape silently accepted context provenance")
				}
			}
			if (mode == "legacy" && count != 0) || (mode != "legacy" && count != 2) {
				t.Fatal("context outside tool neighborhood was omitted/duplicated")
			}
		})
	}
}

// Replay does not pass through native stageRecords: verify the shared redactor
// against actual mocked HTTP and persisted detector/analyzer reports, descendants too.
// Spec: docs/specs/task-retrospective.md via Analyze
func TestSyntheticContextReplayPrivacy(t *testing.T) {
	raw, err := os.ReadFile("../../fixtures/retrospective/opencode/synthetic-replay-privacy.json")
	if err != nil {
		t.Fatal(err)
	}
	var trace Trace
	if err := json.Unmarshal(raw, &trace); err != nil {
		t.Fatal(err)
	}
	f := newLoopFixture(t, unknownScript(t))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), "CANARY-") {
			t.Error("direct Analyze semantic HTTP request leaked context secret")
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		f.jevAPI.Config.Handler.ServeHTTP(w, r)
	}))
	defer server.Close()
	f.cfg.Endpoint = server.URL
	detected, err := DetectSignals(&trace, raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	report, err := Analyze(context.Background(), f.cfg, &trace, raw, NewJevClient(f.cfg.Endpoint, "synthetic-test-key", f.cfg))
	if err != nil {
		t.Fatal(err)
	}
	if !report.Coverage.EvidenceComplete || len(f.jev.recorded()) != 1 {
		t.Fatal("public replay did not reach semantic client")
	}
	for name, value := range map[string]any{"detector": detected, "report": report, "http": f.jev.recorded()} {
		body, _ := json.Marshal(value)
		if strings.Contains(string(body), "CANARY-") || !strings.Contains(string(body), "Public root context") || !strings.Contains(string(body), "Public child context") || !utf8.Valid(body) {
			t.Errorf("%s leaked secrets or lost material UTF-8 context", name)
		}
	}
	file := filepath.Join(t.TempDir(), "report.json")
	if err := WriteReport(file, report); err != nil {
		t.Fatal(err)
	}
	loaded, persisted, err := LoadReport(file)
	if err != nil || strings.Contains(string(persisted), "CANARY-") {
		t.Fatal("persisted replay report leaked", err)
	}
	detectorFile := filepath.Join(t.TempDir(), "detector.json")
	if err := WriteSignalReport(detectorFile, detected); err != nil {
		t.Fatal(err)
	}
	detectorBytes, err := os.ReadFile(detectorFile)
	if err != nil || strings.Contains(string(detectorBytes), "CANARY-") {
		t.Fatal("persisted detector-only replay leaked", err)
	}
	seen := map[string]string{}
	for _, record := range loaded.Evidence {
		if record.Kind == RecordSyntheticContext {
			if record.SourceKind != "opencode.synthetic" || len(record.SourceRefs) != 0 {
				t.Fatal("source provenance changed")
			}
			seen[record.ID] = record.SessionID
		}
	}
	if seen["root-context"] != "root" || seen["child-context"] != "child" {
		t.Fatal("context identity/parent session lost")
	}
}

// Reasoning-marked synthetic input stays private through native admission,
// monotonic upserts, older stored state, descendant loading, Analyze and persistence.
// Spec: docs/specs/task-retrospective.md via IngestEnvelope and assembleTrace
func TestSyntheticContextNativeReasoningPrivacy(t *testing.T) {
	for _, child := range []bool{false, true} {
		for _, mode := range []string{"private-first", "late-private", "legacy-store"} {
			t.Run(mode+map[bool]string{false: "/root", true: "/child"}[child], func(t *testing.T) {
				f := newLoopFixture(t, unknownScript(t))
				owner := "root"
				if child {
					owner = "child"
				}
				root := &Envelope{Version: 1, Harness: HarnessOpenCode, SessionID: "root", Records: []EnvelopeRecord{
					{Kind: "session", SessionID: "root", Directory: f.root},
					{Kind: "message", RecordID: "intent", Role: "user"},
					{Kind: "part", RecordID: "intent:text", MessageID: "intent", PartType: "text", Text: "Check expected negative control"},
					{Kind: "message", RecordID: "final", Role: "assistant"},
					{Kind: "part", RecordID: "final:text", MessageID: "final", PartType: "text", Text: "DONE"},
					{Kind: "part", RecordID: "fail", MessageID: "final", PartType: "tool", Tool: "read", ToolStatus: "error", Input: `{"path":"wrong/docs"}`, Error: "File not found"},
				}}
				if child {
					root.Records = append(root.Records, EnvelopeRecord{Kind: "session", SessionID: "child", ParentID: "root", Directory: f.root})
				}
				if _, err := IngestEnvelope(IngestOptions{Config: f.cfg, Store: f.store}, root); err != nil {
					t.Fatal(err)
				}
				input := &Envelope{Version: 1, Harness: HarnessOpenCode, SessionID: owner, Records: []EnvelopeRecord{
					{Kind: "session", SessionID: owner, Directory: f.root},
					{Kind: "message", RecordID: "private-context", Role: "context"},
					{Kind: "part", RecordID: "private-context:synthetic", MessageID: "private-context", PartType: "synthetic", Reasoning: mode != "late-private", Text: "CANARY-private-reasoning-body"},
				}}
				admit := func() {
					t.Helper()
					if _, err := IngestEnvelope(IngestOptions{Config: f.cfg, Store: f.store}, input); err != nil {
						t.Fatal(err)
					}
				}
				admit()
				if mode == "late-private" {
					input.Records[2].Reasoning = true
					admit()
				}
				input.Records[2].Reasoning = false
				admit() // A false/unset flag cannot republish an already private part.
				for _, relabel := range []string{"text", "tool"} {
					input.Records[2].PartType = relabel
					input.Records[2].Input = "CANARY-private-input"
					input.Records[2].Output = "CANARY-private-output"
					input.Records[2].Tool = "read"
					admit()
				}
				spool, err := loadSpool(f.store, owner)
				if err != nil {
					t.Fatal(err)
				}
				part := spool.Parts["private-context:synthetic"]
				body, _ := json.Marshal(spool)
				if !part.Reasoning || part.PartType != "synthetic" || part.MessageID != "private-context" || strings.Contains(string(body), "CANARY-private") {
					t.Error("private part persisted or privacy flag regressed")
				}
				if mode == "legacy-store" {
					// Synthetic canary models an old persisted row; never real reasoning.
					part.Text = "CANARY-private-reasoning-body"
					if err := writeSpoolBounded(f.cfg, f.store, spool); err != nil {
						t.Fatal(err)
					}
				}
				spool, err = loadSpool(f.store, "root")
				if err != nil {
					t.Fatal(err)
				}
				trace := assembleTrace(f.store, spool, "root")
				raw, _ := json.Marshal(trace)
				if strings.Contains(string(raw), "CANARY-private") {
					t.Error("reasoning body copied into assembled trace")
				}
				report, err := Analyze(context.Background(), f.cfg, trace, raw, NewJevClient(f.cfg.Endpoint, "synthetic-key", f.cfg))
				if err != nil {
					t.Fatal(err)
				}
				if report.Coverage.EvidenceComplete || len(trace.Gaps) == 0 || len(f.jev.recorded()) != 0 {
					t.Error("private context became complete or reached HTTP")
				}
				file := filepath.Join(t.TempDir(), "report.json")
				if err := WriteReport(file, report); err != nil {
					t.Fatal(err)
				}
				persisted, _ := os.ReadFile(file)
				if strings.Contains(string(persisted), "CANARY-private") {
					t.Error("reasoning body persisted in derived report/diagnostics")
				}
			})
		}
	}
}

// Secret removal precedes UTF-8 JSON evidence sizing; shrinking a secret is not
// truncation, while genuinely oversized public context remains covered by cap tests.
// Spec: docs/specs/task-retrospective.md via DetectSignals.retainEvidence
func TestSyntheticContextRedactionBeforeByteBudget(t *testing.T) {
	raw, err := os.ReadFile("../../fixtures/retrospective/opencode/synthetic-replay-privacy.json")
	if err != nil {
		t.Fatal(err)
	}
	var trace Trace
	if err := json.Unmarshal(raw, &trace); err != nil {
		t.Fatal(err)
	}
	trace.Messages[1].Text = "Public café 🧭 password=" + strings.Repeat("CANARY-budget-", 3000)
	raw, _ = json.Marshal(trace)
	report, err := Analyze(context.Background(), DefaultConfig(), &trace, raw, nil)
	if err != nil || !report.Coverage.EvidenceComplete || report.Coverage.OmittedContextRecords != 0 {
		t.Fatal("context was capped before shared redaction", err)
	}
	body, _ := json.Marshal(report.Evidence)
	if strings.Contains(string(body), "CANARY-") || !strings.Contains(string(body), "Public café 🧭") || !utf8.Valid(body) || len(body) > DefaultSignalConfig().EvidenceLimitBytes {
		t.Fatal("redacted UTF-8 context byte budget violated")
	}
}

// A legacy ignored flag cannot erase a native durable-input row without a gap.
// Spec: docs/specs/task-retrospective.md via assembleTrace
func TestSyntheticContextIncompatibleSpoolIsIncomplete(t *testing.T) {
	for _, ignored := range []bool{false, true} {
		spool := &Spool{SessionID: "root", Messages: map[string]*SpoolMessage{"context": {Role: "context"}}, Parts: map[string]*SpoolPart{"context:synthetic": {SessionID: "root", MessageID: "context", Seq: 2, PartType: "synthetic", Text: "material", Synthetic: ignored}}}
		if !ignored {
			spool.Messages["context"].Role = "user"
		}
		trace := assembleTrace(testStore(t), spool, "root")
		if len(trace.Gaps) == 0 || len(trace.Messages) != 0 {
			t.Fatal("incompatible context was silently dropped or misattributed")
		}
	}
}
