// Spec: docs/specs/task-retrospective.md
package retrospective

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGlideRejectsInvalidDistributionOnPublicAnalyzePath(t *testing.T) {
	valid := `{"model":"glide","answers":{"q-outcome":{"type":"choice","choice":"succeeded","confidence":0.95,"probabilities":{"succeeded":0.96,"incomplete":0.01,"failed":0.01,"cancelled":0.01,"unknown":0.01}}},"usage":{"input_tokens":123},"token_usage":{"input_tokens":123}}`
	for name, body := range map[string]string{
		"missing":               strings.Replace(valid, `"probabilities":`, `"absent":`, 1),
		"null-entry":            strings.Replace(valid, `"unknown":0.01`, `"unknown":null`, 1),
		"null-map":              strings.Replace(valid, `{"succeeded":0.96,"incomplete":0.01,"failed":0.01,"cancelled":0.01,"unknown":0.01}`, `null`, 1),
		"missing-key":           strings.Replace(valid, `,"unknown":0.01`, ``, 1),
		"extra-key":             strings.Replace(valid, `"unknown":0.01`, `"unknown":0.01,"forged":0`, 1),
		"bad-total":             strings.Replace(valid, `"unknown":0.01`, `"unknown":0.5`, 1),
		"bad-range":             strings.Replace(valid, `"unknown":0.01`, `"unknown":-0.01`, 1),
		"wrong-model":           strings.Replace(valid, `"glide"`, `"forged"`, 1),
		"wrong-type":            strings.Replace(valid, `"choice","choice"`, `"noul","choice"`, 1),
		"wrong-id":              strings.Replace(valid, `"q-outcome"`, `"forged"`, 1),
		"wrong-winner":          strings.Replace(valid, `"choice":"succeeded"`, `"choice":"unknown"`, 1),
		"null-answers":          `{"model":"glide","answers":null}`,
		"null-confidence":       strings.Replace(valid, `"confidence":0.95`, `"confidence":null`, 1),
		"bad-confidence":        strings.Replace(valid, `"confidence":0.95`, `"confidence":1.1`, 1),
		"overflow-probability":  strings.Replace(valid, `"unknown":0.01`, `"unknown":1e999`, 1),
		"extra-id":              strings.Replace(valid, `"answers":{`, `"answers":{"forged":{"type":"choice"},`, 1),
		"duplicate-probability": strings.Replace(valid, `"succeeded":0.96`, `"succeeded":0.1,"succeeded":0.96`, 1),
		"duplicate-id":          strings.Replace(valid, `"answers":{`, `"answers":{"q-outcome":null,`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			mock, cfg := newMockServer(t, map[int]mockResponse{1: {status: 200, body: body}})
			cfg.MaxAttemptsPerRequest = 1
			trace := baseFixture(20)
			input, _ := json.Marshal(trace)
			report, err := Analyze(context.Background(), cfg, trace, input, newJev(t, cfg, nil))
			if err != nil {
				t.Fatal(err)
			}
			if report.Status != ReportStatusIncomplete || report.Outcome.Choice != "unknown" || report.Dispatch.Eligible || report.Evaluation.StageTwoRan || len(mock.recorded()) != 1 {
				t.Fatalf("invalid provider answer accepted: %+v", report)
			}
		})
	}
	mock, cfg := newMockServer(t, map[int]mockResponse{1: {status: 200, body: valid}})
	if _, err := newJev(t, cfg, nil).Choice(context.Background(), stageSampleRequest()); err != nil {
		t.Fatal(err)
	}
	if len(mock.recorded()) != 1 {
		t.Fatal("valid documented metadata rejected")
	}
}

func TestGlideActualWireBudgetPreventsSend(t *testing.T) {
	mock, cfg := newMockServer(t, nil)
	request := stageSampleRequest()
	request.Requests[0].Evidence = json.RawMessage(`{"payload":"` + strings.Repeat("x", 1800) + `"}`)
	cfg.StateQuestionLimitBytes = 2000
	client := newJev(t, cfg, nil)
	if _, err := client.Choice(context.Background(), request); !errors.Is(err, ErrContentOverBudget) {
		t.Fatalf("folded evidence escaped budget: %v", err)
	}
	if len(mock.recorded()) != 0 {
		t.Fatal("over-budget wire sent")
	}
	request = stageSampleRequest()
	request.Requests = nil
	for i := 0; i < 100; i++ {
		request.Requests = append(request.Requests, Question{ID: fmt.Sprintf("q-c%d", i), Subject: fmt.Sprintf("c%d", i), Scope: ScopeCallClassification, Text: callQuestionText, Choices: callClassificationChoices})
	}
	cfg.StateQuestionLimitBytes = 16_000
	if _, err := client.Choice(context.Background(), request); !errors.Is(err, ErrContentOverBudget) {
		t.Fatalf("criteria overhead escaped body budget: %v", err)
	}
	if len(mock.recorded()) != 0 {
		t.Fatal("oversized criteria maps sent")
	}
}

func TestGlideRedirectNeverTransmitsKey(t *testing.T) {
	hits := 0
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits++ }))
	defer sink.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, sink.URL, http.StatusTemporaryRedirect)
	}))
	defer source.Close()
	cfg := DefaultConfig()
	cfg.Endpoint = source.URL
	cfg.MaxAttemptsPerRequest = 1
	if _, err := NewJevClient(cfg.Endpoint, "CANARY-API-KEY", cfg).Choice(context.Background(), stageSampleRequest()); err == nil {
		t.Fatal("redirect accepted")
	}
	if hits != 0 {
		t.Fatal("redirect followed with sensitive header")
	}
}

func TestGlideExactFirstStageWireAndSubjectInstructions(t *testing.T) {
	trace := bigFixture(40, 0)
	trace.Identities.ExecutionRoot = "/NOT-TASK-EVIDENCE"
	trace.Identities.SourceRevision = "NOT-TASK-REVISION"
	report, calls := runEndToEnd(t, nil, trace, nil)
	if report.Status != ReportStatusComplete || report.Coverage.CoveredRecords != 42 {
		t.Fatalf("coverage lost: %+v", report.Coverage)
	}
	var first systemOneRequest
	if err := json.Unmarshal(calls[0].Body, &first); err != nil {
		t.Fatal(err)
	}
	var state map[string]json.RawMessage
	if err := json.Unmarshal(first.State, &state); err != nil {
		t.Fatal(err)
	}
	if len(state) != 1 || state["evidence"] == nil {
		t.Fatalf("foreign stage-one state: %s", first.State)
	}
	var evidence map[string]json.RawMessage
	if err := json.Unmarshal(state["evidence"], &evidence); err != nil {
		t.Fatal(err)
	}
	if len(evidence) != 1 || evidence[outcomeQuestionID] == nil {
		t.Fatal("wrong stage-one evidence IDs")
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(evidence[outcomeQuestionID], &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload) != 3 || payload["user_messages"] == nil || payload["final_assistant_response"] == nil || payload["last_ten_steps"] == nil {
		t.Fatal("wrong three-part success evidence")
	}
	if strings.Contains(string(calls[0].Body), "NOT-TASK-") {
		t.Fatal("root metadata transmitted at stage one")
	}
	for _, call := range calls {
		var wire systemOneRequest
		if err := json.Unmarshal(call.Body, &wire); err != nil {
			t.Fatal(err)
		}
		internal := decodeWireChoiceRequest(call.Body)
		measure := measureRequest(internal)
		if measure.Request != len(call.Body) || measure.State != len(wire.State) {
			t.Fatalf("measurement differs from wire: %+v body=%d state=%d", measure, len(call.Body), len(wire.State))
		}
		for _, question := range internal.Requests {
			if !strings.Contains(wire.Questions[question.ID].Instructions, "Subject ID: "+question.Subject+" (scope: ") {
				t.Fatal("question identifies only ID, not subject")
			}
		}
	}
}

func TestGlideRejectsResponseBeyondCapWithoutTruncation(t *testing.T) {
	body := `{"model":"glide","answers":{"q-outcome":{"type":"choice","choice":"succeeded","confidence":0.95,"probabilities":{"succeeded":0.96,"incomplete":0.01,"failed":0.01,"cancelled":0.01,"unknown":0.01}}}}`
	body += strings.Repeat(" ", (1<<22)+1-len(body))
	_, cfg := newMockServer(t, map[int]mockResponse{1: {status: 200, body: body}})
	cfg.MaxAttemptsPerRequest = 1
	if _, err := newJev(t, cfg, nil).Choice(context.Background(), stageSampleRequest()); !errors.Is(err, ErrMalformedResponse) {
		t.Fatalf("oversized response truncated to valid prefix: %v", err)
	}
}

func TestGlideHonorsRetryAfterForWarmingAndUnavailable(t *testing.T) {
	for _, status := range []int{425, 429, 503} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			mock, cfg := newMockServer(t, map[int]mockResponse{1: {status: status, headers: map[string]string{"Retry-After": "2"}}})
			var delay time.Duration
			client := newJev(t, cfg, func(d time.Duration) { delay += d })
			if _, err := client.Choice(context.Background(), stageSampleRequest()); err != nil {
				t.Fatal(err)
			}
			if delay != 2*time.Second || len(mock.recorded()) != 2 {
				t.Fatalf("status %d delay=%s requests=%d", status, delay, len(mock.recorded()))
			}
		})
	}
}

func TestGlideProfileAndUnverifiedJevFailClosed(t *testing.T) {
	t.Setenv("FASTINO_API_KEY", "CANARY-FASTINO")
	t.Setenv("TYPESAFE_API_KEY", "CANARY-TYPESAFE")
	cfg := DefaultConfig()
	if cfg.Evaluator != EvaluatorGlide || cfg.Endpoint != "https://api.fastino.ai" || EvaluatorAPIKey(cfg.Evaluator) != "CANARY-FASTINO" {
		t.Fatal("wrong provider defaults/env")
	}
	if err := json.Unmarshal([]byte(`{"evaluator":"jev"}`), cfg); err != nil {
		t.Fatal(err)
	}
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "verified") {
		t.Fatal("unverified Jev enabled")
	}
}

func TestGlideNoSilentEvidenceFallback(t *testing.T) {
	for _, broken := range []string{"state", "evidence", "null-evidence", "choices"} {
		t.Run(broken, func(t *testing.T) {
			mock, cfg := newMockServer(t, nil)
			request := stageSampleRequest()
			switch broken {
			case "state":
				request.State = json.RawMessage(`null`)
			case "evidence":
				request.Requests[0].Evidence = json.RawMessage(`{`)
			case "null-evidence":
				request.Requests[0].Evidence = json.RawMessage(`null`)
			case "choices":
				request.Requests[0].Choices = make([]string, 256)
			}
			if _, err := newJev(t, cfg, nil).Choice(context.Background(), request); err == nil {
				t.Fatal("invalid request accepted")
			}
			if len(mock.recorded()) != 0 {
				t.Fatal("invalid request sent")
			}
		})
	}
}

func TestGlideFragmentOnlyPagesUseNonActionableAcknowledgement(t *testing.T) {
	report, calls := runEndToEnd(t, nil, bigFixture(25, 2000), nil)
	if report.Status != ReportStatusComplete || report.Coverage.UndeliveredRecords != 0 || report.Coverage.SplitRecords == 0 {
		t.Fatalf("fragment coverage: %+v", report.Coverage)
	}
	contexts := 0
	for _, call := range calls {
		var wire systemOneRequest
		if err := json.Unmarshal(call.Body, &wire); err != nil {
			t.Fatal(err)
		}
		if len(wire.Questions) == 0 {
			t.Fatal("provider-invalid empty question map")
		}
		if question, present := wire.Questions["q-context-page"]; present {
			contexts++
			if len(question.Criteria) != 1 || question.Criteria["context-only"] != "context-only" {
				t.Fatal("context acknowledgement can classify waste")
			}
		}
	}
	if contexts == 0 {
		t.Fatal("fixture missed context-only pages")
	}
	if report.Evaluation.Counts.TerminalCalls != 25 || report.Evaluation.Counts.Avoidable.Verified != 0 || report.Dispatch.Eligible {
		t.Fatal("context answers altered core counting")
	}
}
