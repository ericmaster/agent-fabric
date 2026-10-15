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

// Public ingest -> drain -> authenticated paginated native backfill -> mock Choice.
func TestNativeHistoryControlsDrainLiteralWire(t *testing.T) {
	f := newLoopFixture(t, unknownScript(t))
	rows := nativeHistoryFixture(t)
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
	_, err := IngestEnvelope(IngestOptions{Config: f.cfg, Store: f.store}, &Envelope{Version: 1, Harness: HarnessOpenCode, SessionID: "ses_history", ServerURL: server.URL, Trigger: true, Records: []EnvelopeRecord{
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
	if len(trace.Calls) != 2 {
		t.Fatalf("lost native attempts: %+v", trace.Calls)
	}
	final := ""
	for _, message := range trace.Messages {
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
