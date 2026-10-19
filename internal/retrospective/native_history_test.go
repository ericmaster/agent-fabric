// Spec: docs/specs/task-retrospective.md — literal native control/history regression.
package retrospective

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func nativeHistoryFixture(t *testing.T) []json.RawMessage {
	t.Helper()
	body, err := os.ReadFile("../../fixtures/retrospective/opencode/native-history-2.0.24.json")
	if err != nil {
		t.Fatal(err)
	}
	var rows []json.RawMessage
	if err := json.Unmarshal(body, &rows); err != nil {
		t.Fatal(err)
	}
	return rows
}

// Actual isolated 2.0.27 session/history (only private reasoning text removed).
// Spec: docs/specs/task-retrospective.md via HTTPSessionClient.Messages
func TestNative2027RecordedSessionAndHistory(t *testing.T) {
	history, err := os.ReadFile("../../fixtures/retrospective/opencode/native-history-2.0.27.json")
	if err != nil {
		t.Fatal(err)
	}
	session, err := os.ReadFile("../../fixtures/retrospective/opencode/native-session-2.0.27.json")
	if err != nil {
		t.Fatal(err)
	}
	id := "ses_ebfbbab22ffesmWLNKHirpohXz"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/session/" + id:
			w.Write(session)
		case "/api/session/" + id + "/message":
			if r.URL.Query().Get("cursor") != "" {
				w.Write([]byte(`{"data":[],"cursor":{"next":null}}`))
			} else {
				w.Write(history)
			}
		default:
			t.Errorf("unexpected route %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := &HTTPSessionClient{BaseURL: server.URL, Generation: EndpointMessage}
	root, err := client.Read(context.Background(), id)
	if err != nil || root.ID != id || root.ParentID != "" || root.Directory != "/tmp/opencode/agf1-native-2027-runtime3/proj" {
		t.Fatalf("native session changed: %+v %v", root, err)
	}
	rows, err := client.Messages(context.Background(), id)
	if err != nil || len(rows) != 4 {
		t.Fatalf("native history changed: %+v %v", rows, err)
	}
	if rows[0].Info.Role != "user" || rows[3].Info.Role != "" || rows[3].Control != "idle" {
		t.Fatal("history role/control coercion")
	}
	tools, final := 0, false
	for _, row := range rows {
		for _, part := range row.Parts {
			if part.Tool == "read" && part.CallID != "" {
				tools++
			}
			if part.Text == "QA_OBSERVER_DONE" && !part.NonFinal {
				final = true
			}
		}
	}
	if tools != 1 || !final {
		t.Fatal("lost native tool/final evidence")
	}
}

// 2.0.30 public responses are from a no-provider import of a synthetic transcript;
// role/model controls were produced by actual switch endpoints, not generation.
// Spec: docs/specs/task-retrospective.md via HTTPSessionClient.Messages
func TestNative2030RecordedSessionHistoryAndControls(t *testing.T) {
	load := func(name string) []byte {
		t.Helper()
		b, err := os.ReadFile("../../fixtures/retrospective/opencode/" + name + "-2.0.30.json")
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	session, history, controls := load("native-session"), load("native-history"), load("native-controls")
	for _, mode := range []string{"valid", "controls-only", "unknown-field", "malformed-model", "malformed-agent"} {
		t.Run(mode, func(t *testing.T) {
			var page struct {
				Data []map[string]any `json:"data"`
			}
			if err := json.Unmarshal(controls, &page); err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "unknown-field":
				page.Data[0]["future"] = true
			case "malformed-model":
				page.Data[0]["previous"] = map[string]any{"id": "worker"}
			case "malformed-agent":
				page.Data[1]["previous"] = 7
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/session/ses_compat_import_2030" {
					w.Write(session)
					return
				}
				if r.URL.Path != "/api/session/ses_compat_import_2030/message" {
					t.Error(r.URL.Path)
					http.NotFound(w, r)
					return
				}
				switch r.URL.Query().Get("cursor") {
				case "":
					if mode != "controls-only" {
						w.Write(history)
						return
					}
				case "controls-end":
					w.Write([]byte(`{"data":[],"cursor":{"next":null}}`))
					return
				}
				json.NewEncoder(w).Encode(map[string]any{"data": page.Data, "cursor": map[string]any{"next": "controls-end"}})
			}))
			defer server.Close()
			client := &HTTPSessionClient{BaseURL: server.URL, Generation: EndpointMessage}
			root, err := client.Read(context.Background(), "ses_compat_import_2030")
			if err != nil || root.Directory != "/tmp/opencode/agf1-2030-3tecoigc/project" || root.ParentID != "" {
				t.Fatalf("session: %+v %v", root, err)
			}
			rows, err := client.Messages(context.Background(), root.ID)
			if mode != "valid" {
				if err == nil || rows != nil {
					t.Fatal("incomplete/malformed controls accepted")
				}
				return
			}
			if err != nil || len(rows) != 7 {
				t.Fatalf("history: %d %v", len(rows), err)
			}
			if rows[0].Info.Role != "user" || rows[3].Control != "idle" {
				t.Fatal("history role coercion")
			}
			for _, row := range rows[4:] {
				if row.Info.Role != "" || row.Control == "" || len(row.Parts) != 0 {
					t.Fatal("selection became task evidence")
				}
			}
		})
	}
}

// Exact-tag union audit: synthetic context is retained, material context and
// explicit shell execution are not silently discarded. Never echo private data.
// Spec: docs/specs/task-retrospective.md via HTTPSessionClient.Messages
func TestNative2030SyntheticHistoryControls(t *testing.T) {
	fixture, err := os.ReadFile("../../fixtures/retrospective/opencode/native-synthetic-2.0.30.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"valid", "description", "empty-text", "missing-text", "null-text", "wrong-text", "null-description", "wrong-description", "oversized-text", "oversized-description", "extra-field", "extra-time", "private-content", "unknown-type", "system", "skill", "shell", "synthetic-only", "missing-final"} {
		t.Run(mode, func(t *testing.T) {
			var synthetic map[string]any
			if err := json.Unmarshal(fixture, &synthetic); err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "description":
				synthetic["description"] = "CANARY-private-description"
			case "empty-text":
				synthetic["text"] = ""
			case "missing-text":
				delete(synthetic, "text")
			case "null-text":
				synthetic["text"] = nil
			case "wrong-text":
				synthetic["text"] = 7
			case "null-description":
				synthetic["description"] = nil
			case "wrong-description":
				synthetic["description"] = []string{"CANARY-private"}
			case "oversized-text":
				synthetic["text"] = strings.Repeat("x", EnvelopeRecordMaxBytes+1)
			case "oversized-description":
				synthetic["description"] = strings.Repeat("x", EnvelopeRecordMaxBytes+1)
			case "extra-field":
				synthetic["future"] = "CANARY-private"
			case "extra-time":
				synthetic["time"] = map[string]any{"created": 1, "completed": 2}
			case "private-content":
				synthetic["content"] = []any{map[string]any{"type": "reasoning", "text": "CANARY-private"}}
			case "unknown-type":
				synthetic["type"] = "CANARY-private-type"
			case "system", "skill", "shell":
				synthetic["type"] = mode
				// Extra fields on unsupported material types cannot turn them
				// into synthetic controls; diagnostics expose only the known tag.
				synthetic["private"] = "CANARY-private"
			}
			data := []any{synthetic}
			if mode != "synthetic-only" {
				assistant := map[string]any{"id": "msg_final", "type": "assistant", "time": map[string]any{"created": 2, "completed": 3}, "agent": "build", "model": map[string]any{"id": "worker", "providerID": "fixture"}, "finish": "stop", "content": []any{map[string]any{"type": "text", "text": "DONE"}}}
				if mode == "missing-final" {
					assistant["content"] = []any{map[string]any{"type": "reasoning", "text": "CANARY-private-reasoning"}}
				}
				data = append(data, assistant)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/api/session/ses_synthetic/message" {
					t.Error("unexpected public route")
				}
				json.NewEncoder(w).Encode(map[string]any{"data": data, "cursor": map[string]any{"next": nil}})
			}))
			defer server.Close()
			client := &HTTPSessionClient{BaseURL: server.URL, Generation: EndpointMessage}
			rows, err := client.Messages(context.Background(), "ses_synthetic")
			valid := mode == "valid" || mode == "description" || mode == "empty-text" || mode == "missing-final"
			if !valid {
				if err == nil || rows != nil || strings.Contains(err.Error(), "CANARY") {
					t.Fatal("malformed/material history accepted or private diagnostic leaked")
				}
				if (mode == "system" || mode == "skill" || mode == "shell") && !strings.Contains(err.Error(), "native history "+mode+" evidence unsupported") {
					t.Fatal("known material row diagnostic lost")
				}
				return
			}
			if err != nil || len(rows) != 2 {
				t.Fatalf("synthetic control refused: %v", err)
			}
			if rows[0].Control != "" || rows[0].Info.Role != "context" || len(rows[0].Parts) != 1 || rows[0].Parts[0].Type != "synthetic" || rows[0].Parts[0].Synthetic || len(rows[0].Events) != 0 {
				t.Fatal("durable context missing or misattributed")
			}
			encoded, _ := json.Marshal(rows)
			if strings.Contains(string(encoded), "CANARY-private-description") || strings.Contains(string(encoded), "CANARY-private-reasoning") {
				t.Fatal("display context/private reasoning leaked")
			}
			if mode == "missing-final" && len(rows[1].Events) != 1 {
				t.Fatal("synthetic row supplied a missing final")
			}
		})
	}
}

// Optional base metadata is a JSON object, not arbitrary raw JSON or a route.
// Spec: docs/specs/task-retrospective.md via HTTPSessionClient.Messages
func TestNativeSyntheticMetadataObjectContract(t *testing.T) {
	for index, metadata := range []string{"", `{}`, `{"private_key":"CANARY-metadata","nested":{"reasoning":"CANARY-private"}}`, `7`, `[]`, `null`, `"CANARY-private"`, `true`, `{"a":1,"a":2}`, `{"nested":{"a":1,"a":2}}`, `{"bad":}`, `{"deep":` + strings.Repeat(`[`, 130) + `0` + strings.Repeat(`]`, 130) + `}`} {
		t.Run(fmt.Sprint(index), func(t *testing.T) {
			field := ""
			if metadata != "" {
				field = `,"metadata":` + metadata
			}
			body := `{"data":[{"id":"msg_context","time":{"created":1},"type":"synthetic","text":"public context"` + field + `},{"id":"msg_final","time":{"created":2,"completed":3},"type":"assistant","agent":"build","model":{"id":"worker","providerID":"fixture"},"finish":"stop","content":[{"type":"text","text":"DONE"}]}],"cursor":{"next":null}}`
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) }))
			defer server.Close()
			client := &HTTPSessionClient{BaseURL: server.URL, Generation: EndpointMessage}
			rows, err := client.Messages(context.Background(), "ses_metadata")
			valid := metadata == "" || metadata == `{}` || strings.Contains(metadata, `"private_key"`)
			if !valid {
				if err == nil || rows != nil || strings.Contains(err.Error(), "CANARY") || strings.Contains(err.Error(), "private_key") {
					t.Fatal("invalid metadata accepted or echoed")
				}
				return
			}
			if err != nil || len(rows) != 2 {
				t.Fatalf("valid metadata refused: %v", err)
			}
			encoded, _ := json.Marshal(rows)
			if strings.Contains(string(encoded), "CANARY") || strings.Contains(string(encoded), "private_key") {
				t.Fatal("opaque metadata copied into normalized output")
			}
		})
	}
}

// Literal 2.0.27 schema addition retained for 2.0.30's identical history schema.
// Spec: docs/specs/task-retrospective.md via HTTPSessionClient.Messages
func TestNative2027IdleErrorLiteralWire(t *testing.T) {
	for _, errorJSON := range []string{`{"type":"ProviderError","message":"CANARY-private","status":503,"response":{"body":"CANARY-private"}}`, `{"type":"ProviderError","message":""}`, `null`, `{"message":"missing type"}`, `{"type":"x","message":"x","status":99}`, `{"type":"x","message":"x","extra":true}`} {
		t.Run(errorJSON, func(t *testing.T) {
			wire := `{"data":[{"id":"msg_final","type":"assistant","agent":"build","model":{"providerID":"fixture","id":"worker"},"time":{"created":1,"completed":2},"finish":"stop","content":[{"type":"text","text":"DONE"}]},{"id":"msg_idle","type":"idle","time":{"created":3},"outcome":"failed","error":` + errorJSON + `}],"cursor":{"next":null}}`
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(wire)) }))
			defer server.Close()
			client := &HTTPSessionClient{BaseURL: server.URL, Generation: EndpointMessage}
			rows, err := client.Messages(context.Background(), "ses_2027")
			valid := strings.HasPrefix(errorJSON, `{"type":"ProviderError"`)
			if !valid {
				if err == nil || rows != nil {
					t.Fatal("malformed idle error accepted")
				}
				return
			}
			if err != nil || len(rows) != 2 {
				t.Fatalf("2.0.27 history refused: %v", err)
			}
			if rows[1].Control != "idle" || rows[1].Info.Role != "" || len(rows[1].Events) != 2 || rows[1].Events[0].Event != "session.error" {
				t.Fatalf("control coerced/lost: %+v", rows[1])
			}
			body, _ := json.Marshal(rows)
			if strings.Contains(string(body), "CANARY-private") {
				t.Fatal("private idle error body exposed")
			}
		})
	}
}

// Public ingest -> drain -> authenticated paginated native backfill -> mock Choice.
func TestNativeHistoryControlsDrainLiteralWire(t *testing.T) {
	f := newLoopFixture(t, unknownScript(t))
	rows := nativeHistoryFixture(t)
	// Live structural shape, anonymized ID; timestamp/text are synthetic canaries.
	// The actual row body was never retained. Test text is material nonhuman input.
	synthetic, err := os.ReadFile("../../fixtures/retrospective/opencode/native-synthetic-2.0.30.json")
	if err != nil {
		t.Fatal(err)
	}
	rows = append(rows, json.RawMessage(synthetic))
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/api/session/ses_history/message" {
			t.Errorf("unexpected route %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		u, p, ok := r.BasicAuth()
		if !ok || u != "opencode" || p != "synthetic-loop-key" {
			t.Error("auth boundary lost")
		}
		if r.URL.Query().Get("cursor") == "older" {
			json.NewEncoder(w).Encode(map[string]any{"data": rows[2:], "cursor": map[string]any{"next": nil}})
		} else {
			json.NewEncoder(w).Encode(map[string]any{"data": rows[:2], "cursor": map[string]any{"next": "older"}})
		}
	}))
	defer server.Close()
	f.cfg.DispatchServer = server.URL
	if err := SetEnabled(f.configPath, true); err != nil {
		t.Fatal(err)
	}
	// Intake is observed; all tools/final come from real-shape history backfill.
	_, err = IngestEnvelope(IngestOptions{Config: f.cfg, Store: f.store}, &Envelope{Version: 1, Harness: HarnessOpenCode, SessionID: "ses_history", ServerURL: server.URL, Trigger: true, Records: []EnvelopeRecord{
		{Kind: "session", SessionID: "ses_history", Directory: f.root},
		{Kind: "message", SessionID: "ses_history", RecordID: "msg_11816a0f1001Lpag8sItprEyak", Role: "user"},
		{Kind: "part", SessionID: "ses_history", RecordID: "msg_11816a0f1001Lpag8sItprEyak:text", MessageID: "msg_11816a0f1001Lpag8sItprEyak", PartType: "text", Text: "Actual synthetic opening"},
		{Kind: "source", SessionID: "ses_history", Path: f.agentSource, SHA256: f.sourceHash},
		{Kind: "event", SessionID: "ses_history", Event: "session.idle"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	// Exact persisted artifact of the previous native history adapter. It is
	// control metadata, not a genuine observable message/tool/text to discard.
	old, err := loadSpool(f.store, "ses_history")
	if err != nil {
		t.Fatal(err)
	}
	old.upsert(&EnvelopeRecord{Kind: "message", SessionID: "ses_history", RecordID: "msg_118177345001d7T96LucDphgOL", Role: "idle"})
	old.upsert(&EnvelopeRecord{Kind: "part", SessionID: "ses_history", RecordID: "msg_118177345001d7T96LucDphgOL", MessageID: "msg_118177345001d7T96LucDphgOL", PartType: "text"})
	if err := writeSpoolBounded(f.cfg, f.store, old); err != nil {
		t.Fatal(err)
	}
	summary, err := RunDrain(context.Background(), DrainOptions{Config: f.cfg, ConfigPath: f.configPath, EnvKey: "CANARY-mock-history-key"})
	if err != nil || summary.Processed != 1 {
		t.Fatalf("drain: %+v %v", summary, err)
	}
	job, err := f.store.ReadDrainJob("ses_history")
	if err != nil || job.ReportPath == "" {
		t.Fatalf("no report: %+v %v", job, err)
	}
	body, err := os.ReadFile(job.ReportPath)
	if err != nil {
		t.Fatal(err)
	}
	var report Report
	if err := json.Unmarshal(body, &report); err != nil {
		t.Fatal(err)
	}
	if report.Status != ReportStatusComplete || requests != 2 || len(f.jev.recorded()) != 1 || len(report.Signals) != 1 {
		t.Fatalf("history never reached mock evaluation: %+v requests=%d reasons=%v", job, requests, report.IncompleteReasons)
	}
	spool, err := loadSpool(f.store, "ses_history")
	if err != nil {
		t.Fatal(err)
	}
	trace := assembleTrace(f.store, spool, "ses_history")
	for _, value := range []any{spool, trace, report, f.jev.recorded()} {
		encoded, err := json.Marshal(value)
		if err != nil || strings.Contains(string(encoded), "CANARY-context-secret") || !strings.Contains(string(encoded), "expected negative control") || !strings.Contains(string(encoded), "msg_0c003f2bca58eb41") {
			t.Fatal("material context disappeared or redaction failed")
		}
	}
	contextRows := 0
	for _, record := range report.Evidence {
		if strings.Contains(record.ID, "msg_0c003f2bca58eb41") {
			if record.Kind != "synthetic_context" || record.SessionID != "ses_history" || len(record.SourceRefs) != 0 {
				t.Fatal("nonhuman context became human/assistant/file provenance")
			}
			contextRows++
		}
	}
	if contextRows != 1 {
		t.Fatal("material context absent/duplicated in report")
	}
	loaded, _, err := LoadReport(job.ReportPath)
	if err != nil {
		t.Fatal("context report cannot be read back", err)
	}
	normalized, err := Normalize(trace)
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range normalized.Steps {
		if strings.Contains(step.ID, "msg_0c003f2bca58eb41") {
			t.Fatal("context counted as productive work")
		}
	}
	for _, boundary := range trace.Boundaries {
		if boundary.Kind == "task_start" {
			t.Fatal("nonhuman context invented a user task boundary")
		}
	}
	if spool.Messages["msg_0c003f2bca58eb41"].ParentID != "" || loaded.Evidence[0].Kind == "" {
		t.Fatal("context invented human ownership or lost report evidence")
	}
	// Counterfactual packet selection: removing the actual collected context
	// changes the same signal's packet; it cannot disappear behind complete=true.
	packet, _ := packetFor(normalized, loaded, loaded.Signals[0], f.cfg.ContextNeighborCalls)
	withContext, _ := json.Marshal(packet)
	without := *loaded
	without.Evidence = nil
	for _, record := range loaded.Evidence {
		if record.Kind != "synthetic_context" {
			without.Evidence = append(without.Evidence, record)
		} else if record.SourceKind != "opencode.synthetic" {
			t.Fatal("context lost source-kind provenance")
		}
	}
	packetWithout, _ := packetFor(normalized, &without, loaded.Signals[0], f.cfg.ContextNeighborCalls)
	withoutContext, _ := json.Marshal(packetWithout)
	if string(withContext) == string(withoutContext) || !strings.Contains(string(withContext), "expected negative control") || strings.Contains(string(withoutContext), "expected negative control") {
		t.Fatal("candidate selection ignores material context")
	}
	if len(trace.Calls) != 2 {
		t.Fatalf("lost native attempts: %+v", trace.Calls)
	}
	final := ""
	for _, message := range trace.Messages {
		if message.Role == "context" && message.Kind == "synthetic" {
			continue
		}
		if message.Role != "user" && message.Role != "assistant" {
			t.Fatal("fake control role")
		}
		if message.Kind == "final" {
			final = message.Text
		}
	}
	if !strings.Contains(final, "QA_STALE_PATH_AUDIT_DONE") || len(trace.Gaps) != 0 {
		t.Fatalf("genuine final lost: %q gaps=%+v", final, trace.Gaps)
	}
	if len(spool.SourceRefs) != 1 || spool.SourceRefs[0].SHA256 != f.sourceHash {
		t.Fatal("at-use source provenance changed")
	}
	if len(trace.Calls[0].Result) == 0 || len(trace.Calls[1].Result) == 0 {
		t.Fatal("tool results lost")
	}
	if !strings.Contains(trace.Calls[1].Result[0].Text, "File not found") {
		t.Fatal("native public error evidence lost")
	}
	if _, err := os.Stat(filepath.Join(f.store.Dir, "receipts")); err == nil {
		receipts, _ := f.store.ListReceipts()
		if len(receipts) != 0 {
			t.Fatal("small fixture forced a repair")
		}
	}
}

func TestNativeHistoryResponseAndAggregateByteBudgets(t *testing.T) {
	for _, aggregate := range []bool{false, true} {
		calls := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			rows := nativeHistoryFixture(t)
			var next any
			pad := strings.Repeat(" ", exchangeLimit+1)
			if aggregate {
				next = fmt.Sprint(calls)
				pad = strings.Repeat(" ", exchangeLimit/2)
			}
			body, _ := json.Marshal(map[string]any{"data": rows, "cursor": map[string]any{"next": next}})
			w.Write(append(body, []byte(pad)...))
		}))
		client := &HTTPSessionClient{BaseURL: server.URL, Generation: EndpointMessage}
		if rows, err := client.Messages(context.Background(), "ses_byte_budget"); err == nil || rows != nil {
			t.Fatal("byte truncation fabricated complete history")
		}
		server.Close()
		if calls > 2 {
			t.Fatal("aggregate byte budget unbounded")
		}
	}
}

// Unknown or malformed wire never becomes invented user/assistant evidence.
func TestNativeHistoryRejectsMalformedRows(t *testing.T) {
	for _, wire := range []string{
		`{"data":[{"id":"bad","time":{"created":1},"type":"idle"}],"cursor":{"next":null}}`,
		`{"data":[{"id":"bad","time":{"created":1},"type":"idle","outcome":"succeeded","content":null}],"cursor":{"next":null}}`,
		`{"data":[{"id":"bad","time":{"created":1},"type":"status","text":"CANARY-private"}],"cursor":{"next":null}}`,
		`{"data":[{"id":"bad","time":{"created":1},"type":"location-switched","location":{}}],"cursor":{"next":null}}`,
		`{"data":[{"id":"bad","time":{"created":1},"type":"user"}],"cursor":{"next":null}}`,
		`{"data":[{"id":"bad","type":"user","text":"missing time"}],"cursor":{"next":null}}`,
		`{"data":[{"id":"bad","time":{"created":1},"type":"assistant","content":[]}],"cursor":{"next":null}}`,
		`{"data":[],"cursor":{}}`, `{"cursor":{"next":null}}`,
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(wire)) }))
		client := &HTTPSessionClient{BaseURL: server.URL, Generation: EndpointMessage}
		if rows, err := client.Messages(context.Background(), "ses_malformed"); err == nil || rows != nil {
			t.Fatalf("malformed history accepted: %s", wire)
		}
		server.Close()
	}
	for _, content := range []string{
		`{"type":"future","text":"CANARY-private"}`,
		`{"type":"text","text":"fake final","name":"lost-tool"}`,
		`{"type":"reasoning"}`,
		`{"type":"tool","id":"call","name":"read","time":{"created":1},"state":{"status":"completed","input":{},"content":[]}}`,
		`{"type":"tool","id":"call","name":"read","time":{"created":1},"state":{"status":"completed","input":null,"content":[{"type":"text","text":"bad"}]}}`,
	} {
		row := nativeHistoryRow{ID: "msg_bad", Time: nativeHistoryTime{Created: 1, Completed: 2}, Type: "assistant", Agent: "build", Model: nativeHistoryModel{ID: "mock", ProviderID: "fixture"}, Content: []json.RawMessage{json.RawMessage(content)}, Finish: "stop"}
		if _, err := normalizeNativeHistory(row, "ses_malformed"); err == nil {
			t.Fatalf("malformed content accepted: %s", content)
		}
	}
}

func TestNativeHistoryGapsNoGuessedSuccessOrPrivateText(t *testing.T) {
	for _, mode := range []string{"private-reasoning", "old-idle-error", "interrupted", "missing-final", "unknown-control", "malformed-tool", "incomplete-final"} {
		t.Run(mode, func(t *testing.T) {
			f := newLoopFixture(t, successScript(t))
			raw := nativeHistoryFixture(t)
			var rows []map[string]any
			for _, body := range raw {
				var row map[string]any
				json.Unmarshal(body, &row)
				rows = append(rows, row)
			}
			switch mode {
			case "private-reasoning":
				content := rows[3]["content"].([]any)
				rows[3]["content"] = append([]any{map[string]any{"type": "reasoning", "text": "CANARY-private-history"}}, content...)
			case "old-idle-error":
				rows = append(rows, map[string]any{"id": "msg_old_idle", "time": map[string]any{"created": 1791405498700}, "type": "idle", "outcome": "failed"})
			case "interrupted":
				rows[0]["outcome"] = "interrupted"
			case "missing-final":
				rows = append(rows[:1], rows[2:]...)
			case "unknown-control":
				rows[0]["type"] = "future-status"
				rows[0]["text"] = "CANARY-private-history"
			case "malformed-tool":
				rows[3]["content"].([]any)[0].(map[string]any)["type"] = "future-tool"
			case "incomplete-final":
				delete(rows[1]["time"].(map[string]any), "completed")
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				json.NewEncoder(w).Encode(map[string]any{"data": rows, "cursor": map[string]any{"next": nil}})
			}))
			defer server.Close()
			f.cfg.DispatchServer = server.URL
			SetEnabled(f.configPath, true)
			_, err := IngestEnvelope(IngestOptions{Config: f.cfg, Store: f.store}, &Envelope{Version: 1, Harness: HarnessOpenCode, SessionID: "ses_gaps", ServerURL: server.URL, Trigger: true, Records: []EnvelopeRecord{
				{Kind: "session", SessionID: "ses_gaps", Directory: f.root},
				{Kind: "message", SessionID: "ses_gaps", RecordID: "msg_11816a0f1001Lpag8sItprEyak", Role: "user"},
				{Kind: "part", SessionID: "ses_gaps", RecordID: "msg_11816a0f1001Lpag8sItprEyak:text", MessageID: "msg_11816a0f1001Lpag8sItprEyak", PartType: "text", Text: "Actual opening"},
				{Kind: "message", SessionID: "ses_gaps", RecordID: "msg_1181766b5001Fus9hPD6jJMcj9", Role: "assistant"},
				{Kind: "part", SessionID: "ses_gaps", RecordID: "msg_1181766b5001Fus9hPD6jJMcj9:text:0", MessageID: "msg_1181766b5001Fus9hPD6jJMcj9", PartType: "text", Text: "Streamed completed answer"},
				{Kind: "event", SessionID: "ses_gaps", Event: "session.idle"},
			}})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := RunDrain(context.Background(), DrainOptions{Config: f.cfg, ConfigPath: f.configPath, EnvKey: "CANARY-mock-history-key"}); err != nil {
				t.Fatal(err)
			}
			job, err := f.store.ReadDrainJob("ses_gaps")
			if err != nil || job.ReportPath == "" {
				t.Fatalf("no safe report: %+v %v", job, err)
			}
			body, err := os.ReadFile(job.ReportPath)
			if err != nil {
				t.Fatal(err)
			}
			var report Report
			json.Unmarshal(body, &report)
			success := mode == "private-reasoning" || mode == "old-idle-error" || mode == "interrupted"
			if success != (report.Status == ReportStatusComplete) {
				t.Fatalf("wrong coverage: %+v", report.IncompleteReasons)
			}
			if !success && len(f.jev.recorded()) != 0 {
				t.Fatal("gap/status guessed terminal success")
			}
			if mode == "interrupted" && report.TerminalOutcome.Status != TaskStatusAborted {
				t.Fatal("interruption reinterpreted as success")
			}
			if strings.Contains(string(body), "CANARY-private-history") {
				t.Fatal("private history leaked to report")
			}
			for _, req := range f.jev.recorded() {
				data, _ := json.Marshal(req)
				if strings.Contains(string(data), "CANARY-private-history") {
					t.Fatal("private history leaked to mock evaluator")
				}
			}
			spool, _ := loadSpool(f.store, "ses_gaps")
			if !success && spool.AcceptedAfterSeq != 0 {
				t.Fatal("incomplete native history advanced accepted task boundary")
			}
			if mode == "unknown-control" || mode == "malformed-tool" {
				if len(spool.Parts) < 2 {
					t.Fatal("malformed history discarded already captured evidence")
				}
			}
		})
	}
}

func TestLegacyHistoryRolesRemainStrict(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/session/ses_legacy/message" {
			t.Errorf("legacy route changed: %s", r.URL.Path)
		}
		w.Write([]byte(`[{"info":{"id":"legacy_idle","sessionID":"ses_legacy","role":"idle"},"parts":[{"id":"legacy_idle","messageID":"legacy_idle","type":"text","text":"not a message"}]}]`))
	}))
	defer server.Close()
	rows, err := (&HTTPSessionClient{BaseURL: server.URL, Generation: EndpointPrompt}).Messages(context.Background(), "ses_legacy")
	if err != nil || len(rows) != 1 || rows[0].Info.Role != "idle" {
		t.Fatal("native role coercion leaked into legacy history")
	}
	trace := &Trace{SchemaVersion: 1, Identities: Identities{RootSessionID: "ses_legacy", SessionID: "ses_legacy", ExecutionRoot: t.TempDir()}, Messages: []Message{{ID: "legacy_idle", Seq: 1, Role: rows[0].Info.Role, Text: "not a real user/assistant"}}}
	if _, err := Normalize(trace); err == nil {
		t.Fatal("core role validator was relaxed")
	}
}

func TestNativeHistoryVisibleRecordBoundsNeverSilentlyClip(t *testing.T) {
	text := strings.Repeat("x", EnvelopeRecordMaxBytes+1)
	row := nativeHistoryRow{ID: "msg_bound", Type: "assistant", Agent: "build", Model: nativeHistoryModel{ID: "worker", ProviderID: "fixture"}, Time: nativeHistoryTime{Created: 1, Completed: 2}, Finish: "stop"}
	for _, part := range []any{
		map[string]any{"type": "text", "text": text},
		map[string]any{"type": "tool", "id": "call", "name": "read", "time": map[string]int{"created": 1}, "state": map[string]any{"status": "completed", "input": map[string]string{"path": text}, "content": []any{map[string]string{"type": "text", "text": "result"}}}},
		map[string]any{"type": "tool", "id": "call", "name": "read", "time": map[string]int{"created": 1}, "state": map[string]any{"status": "completed", "input": map[string]string{"path": "normal"}, "content": []any{map[string]string{"type": "text", "text": text}}}},
	} {
		body, _ := json.Marshal(part)
		row.Content = []json.RawMessage{body}
		if _, err := normalizeNativeHistory(row, "ses_bound"); err == nil {
			t.Fatal("oversized visible field clipped without gap")
		}
	}
	row.Type = "user"
	row.Text = &text
	row.Content = nil
	if _, err := normalizeNativeHistory(row, "ses_bound"); err == nil {
		t.Fatal("oversized user text clipped without gap")
	}
}

func TestNativeHistoryControlsCannotBypassPaginationBudgets(t *testing.T) {
	for _, mode := range []string{"repeated-cursor", "page-budget", "row-budget", "controls-only"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var data []any
				count := 1
				if mode == "row-budget" {
					count = EnvelopeMaxRecords + 1
				}
				for n := 0; n < count; n++ {
					data = append(data, map[string]any{"id": fmt.Sprintf("idle_%d_%d", calls, n), "type": "idle", "time": map[string]int{"created": calls}, "outcome": "succeeded"})
				}
				var next any
				if mode == "repeated-cursor" {
					next = "same"
				} else if mode == "page-budget" {
					next = fmt.Sprint(calls)
				}
				json.NewEncoder(w).Encode(map[string]any{"data": data, "cursor": map[string]any{"next": next}})
			}))
			defer server.Close()
			client := &HTTPSessionClient{BaseURL: server.URL, Generation: EndpointMessage}
			if rows, err := client.Messages(context.Background(), "ses_budget"); err == nil || rows != nil {
				t.Fatal("controls forged complete coverage")
			}
			if calls > messagePageLimit {
				t.Fatal("unbounded pagination")
			}
		})
	}
}
