// Spec: docs/specs/task-retrospective.md
package retrospective

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

// Replay the actual sanitized response through Analyze and the production HTTP
// client. Only the legacy control's model is changed, never the live choices.
// Spec: docs/specs/task-retrospective.md via Analyze
func TestObservedGlideModelCompatibility(t *testing.T) {
	body, err := os.ReadFile("../../fixtures/retrospective/glide/observed-v2.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, model := range []string{"glide_v2", "glide", "glide_v3", "glide_v20", "glide_other", "GLIDE", "fastino/GLiDE", "unrelated-CANARY", "", "null", "123", "true", "[]", "{}", "missing"} {
		t.Run(model, func(t *testing.T) {
			value, _ := json.Marshal(model)
			if model == "null" || model == "123" || model == "true" || model == "[]" || model == "{}" {
				value = []byte(model)
			}
			payload := strings.Replace(string(body), `"model":"glide_v2"`, `"model":`+string(value), 1)
			if model == "missing" {
				payload = strings.Replace(string(body), `"model":"glide_v2",`, "", 1)
			}
			mock, cfg := newMockServer(t, map[int]mockResponse{1: {status: 200, body: payload}})
			cfg.MaxAttemptsPerRequest = 1
			calls := []CallRecord{
				{ID: "c:msg_140640a7f001sqorwYNed69bVe:call-bbe14613-b168-4c06-b149-b3e720da4a48", Seq: 1, Tool: "read", Status: StatusSuccess, Input: `{"path":"docs/catalog.md"}`},
				{ID: "c:msg_14064105a0014ArnLLLOu51zSW:call-454d9cc9-44cf-47cc-b8f5-afaf0f4c85d8", Seq: 2, Tool: "read", Status: StatusError, Input: `{"path":"archive/catalog.json"}`},
				{ID: "c:msg_14064135b0017k0Ypgi25AM8vb:call-c0cc59c9-c3bb-4cf0-b900-054e96842dfc", Seq: 3, Tool: "read", Status: StatusSuccess, Input: `{"path":"data/catalog.json"}`},
			}
			trace := signalTrace(calls)
			input, _ := json.Marshal(trace)
			report, err := Analyze(context.Background(), cfg, trace, input, NewJevClient(mock.server.URL, "synthetic-key", cfg))
			if err != nil {
				t.Fatal(err)
			}
			if len(report.Interpretation.Rows) != 1 || report.Usage.Requests != 1 {
				t.Fatalf("expected one real client interpretation: %+v", report)
			}
			row := report.Interpretation.Rows[0]
			if model == "glide" || model == "glide_v2" {
				if row.Status != "interpreted" || !row.Delivered || row.Choice != "justified" || row.Cause != "instruction" || row.SupportCallID != calls[1].ID || row.Probability != 0.9104179151666885 || row.Confidence != 0.8517571237742433 || report.Interpretation.Status != "complete" {
					t.Fatalf("captured response not interpreted: %+v", row)
				}
			} else if row.Status != "uncertain" || row.Delivered || row.Choice != "uncertain" {
				t.Fatalf("invalid model accepted: %+v", row)
			}
			if model == "glide_v3" && row.Reason != "unknown_provider_model" {
				t.Fatalf("missing safe model diagnostic: %+v", row)
			}
			if report.Model.Model != GlideModel || report.Routing.Actionable || report.Dispatch.Eligible || report.RepairSafe || ineligible(report) == "" {
				t.Fatal("compatibility changed model pin or authorized repair")
			}
			encoded, _ := json.Marshal(report)
			if strings.Contains(string(encoded), "CANARY") || strings.Contains(string(encoded), "synthetic-key") {
				t.Fatal("provider model/key echoed")
			}
			mock.mu.Lock()
			defer mock.mu.Unlock()
			call := mock.calls[0]
			var wire systemOneRequest
			if json.Unmarshal(call.Body, &wire) != nil || wire.Model != GlideModel || call.Path != SystemOnePath || call.XAPIKey != "synthetic-key" || call.Auth != "" {
				t.Fatal("request pin/path/auth changed")
			}
		})
	}
}

// In-memory transport keeps this replay entirely offline while exercising Choice.
// Spec: docs/specs/task-retrospective.md via JevClient
type compatTransport func(*http.Request) (*http.Response, error)

func (f compatTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// The new model must not bypass any strict wire-answer check.
// Spec: docs/specs/task-retrospective.md via JevClient
func TestObservedGlideStrictNegatives(t *testing.T) {
	body, err := os.ReadFile("../../fixtures/retrospective/glide/observed-v2.json")
	if err != nil {
		t.Fatal(err)
	}
	var wire systemOneResponse
	if err := json.Unmarshal(body, &wire); err != nil {
		t.Fatal(err)
	}
	request := &ChoiceRequest{State: json.RawMessage(`{}`)}
	for id, answer := range wire.Answers {
		q := Question{ID: id, Subject: "signal", Text: "Synthetic strict control"}
		for choice := range answer.Probabilities {
			q.Choices = append(q.Choices, choice)
		}
		request.Requests = append(request.Requests, q)
	}
	for name, payload := range map[string]string{
		"duplicate model":  strings.Replace(string(body), `"model":"glide_v2"`, `"model":"glide_v2","model":"glide_v2"`, 1),
		"unknown field":    strings.Replace(string(body), `"model":`, `"extra":true,"model":`, 1),
		"wrong type":       strings.Replace(string(body), `"type":"choice"`, `"type":"text"`, 1),
		"unknown id":       strings.Replace(string(body), `"action:`, `"forged:`, 1),
		"forged citation":  strings.ReplaceAll(string(body), "call-454d9cc9", "call-forged"),
		"confidence null":  strings.Replace(string(body), `0.8517571237742433`, `null`, 1),
		"confidence range": strings.Replace(string(body), `0.8517571237742433`, `1.5`, 1),
		"probability null": strings.Replace(string(body), `0.9104179151666885`, `null`, 1),
		"sum":              strings.Replace(string(body), `0.9104179151666885`, `0.8`, 1),
		"winner":           strings.Replace(string(body), `"choice":"justified"`, `"choice":"actionable"`, 1),
		"nonfinite":        strings.Replace(string(body), `0.9104179151666885`, `1e999`, 1),
		"missing answers":  `{"model":"glide_v2","answers":{}}`,
	} {
		t.Run(name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.MaxAttemptsPerRequest = 1
			client := NewJevClient("https://offline.invalid", "", cfg)
			client.client.Transport = compatTransport(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(payload)), Header: http.Header{}}, nil
			})
			if response, err := client.Choice(context.Background(), request); err == nil || response != nil {
				t.Fatal("malformed captured-response mutation accepted")
			}
		})
	}
}
