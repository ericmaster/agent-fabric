// Spec: docs/specs/task-retrospective.md
package retrospective

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// The full-distribution trust-boundary matrix now tests semantic Choice wire,
// independent of the removed outcome/per-call evaluator.
// Spec: docs/specs/task-retrospective.md via JevClient
func TestGlideRejectsInvalidDistribution(t *testing.T) {
	valid := `{"model":"glide","answers":{"action:signal":{"type":"choice","choice":"actionable","confidence":0.95,"probabilities":{"actionable":0.98,"justified":0.01,"uncertain":0.01}}},"usage":{"input_tokens":123},"token_usage":{"input_tokens":123}}`
	cases := map[string]string{
		"missing": strings.Replace(valid, `"probabilities":`, `"absent":`, 1), "null-entry": strings.Replace(valid, `"uncertain":0.01`, `"uncertain":null`, 1),
		"null-map": strings.Replace(valid, `{"actionable":0.98,"justified":0.01,"uncertain":0.01}`, `null`, 1), "missing-key": strings.Replace(valid, `,"uncertain":0.01`, "", 1),
		"extra-key": strings.Replace(valid, `"uncertain":0.01`, `"uncertain":0.01,"forged":0`, 1), "bad-total": strings.Replace(valid, `"uncertain":0.01`, `"uncertain":0.5`, 1),
		"bad-range": strings.Replace(valid, `"uncertain":0.01`, `"uncertain":-0.01`, 1), "wrong-model": strings.Replace(valid, `"glide"`, `"forged"`, 1),
		"wrong-type": strings.Replace(valid, `"type":"choice"`, `"type":"other"`, 1), "wrong-id": strings.Replace(valid, `"action:signal"`, `"forged"`, 1),
		"wrong-winner": strings.Replace(valid, `"choice":"actionable"`, `"choice":"uncertain"`, 1), "null-answers": `{"model":"glide","answers":null}`,
		"null-confidence": strings.Replace(valid, `"confidence":0.95`, `"confidence":null`, 1), "bad-confidence": strings.Replace(valid, `"confidence":0.95`, `"confidence":1.1`, 1),
		"overflow-probability": strings.Replace(valid, `"uncertain":0.01`, `"uncertain":1e999`, 1), "extra-id": strings.Replace(valid, `"answers":{`, `"answers":{"forged":{"type":"choice"},`, 1),
		"duplicate-probability": strings.Replace(valid, `"actionable":0.98`, `"actionable":0.1,"actionable":0.98`, 1), "duplicate-id": strings.Replace(valid, `"answers":{`, `"answers":{"action:signal":null,`, 1),
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			mock, cfg := newMockServer(t, map[int]mockResponse{1: {status: 200, body: body}})
			cfg.MaxAttemptsPerRequest = 1
			if _, err := newJev(t, cfg, nil).Choice(context.Background(), stageSampleRequest()); !errors.Is(err, ErrMalformedResponse) || len(mock.recorded()) != 1 {
				t.Fatal("invalid wire response accepted", err)
			}
		})
	}
	mock, cfg := newMockServer(t, map[int]mockResponse{1: {status: 200, body: valid}})
	if _, err := newJev(t, cfg, nil).Choice(context.Background(), stageSampleRequest()); err != nil || len(mock.recorded()) != 1 {
		t.Fatal("documented metadata rejected", err)
	}
}

// Spec: docs/specs/task-retrospective.md via JevClient
func TestGlideActualWireBudgetPreventsSend(t *testing.T) {
	mock, cfg := newMockServer(t, nil)
	q := stageSampleRequest()
	q.Requests[0].Evidence = json.RawMessage(`{"payload":"` + strings.Repeat("x", 1800) + `"}`)
	cfg.StateQuestionLimitBytes = 2000
	if _, err := newJev(t, cfg, nil).Choice(context.Background(), q); !errors.Is(err, ErrContentOverBudget) || len(mock.recorded()) != 0 {
		t.Fatal("folded wire escaped limit", err)
	}
}

// Spec: docs/specs/task-retrospective.md via JevClient
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
	c := NewJevClient(source.URL, "CANARY-API-KEY", cfg)
	if _, err := c.Choice(context.Background(), stageSampleRequest()); !errors.Is(err, ErrPermanent) || c.Usage().Requests != 1 || c.Usage().Retries != 0 || hits != 0 {
		t.Fatal("redirect credentials replayed", err)
	}
}

// Spec: docs/specs/task-retrospective.md via JevClient
func TestGlideRejectsResponseBeyondCapWithoutTruncation(t *testing.T) {
	_, cfg := newMockServer(t, map[int]mockResponse{1: {status: 200, body: strings.Repeat(" ", 4*1024*1024+1)}})
	cfg.MaxAttemptsPerRequest = 1
	if _, err := newJev(t, cfg, nil).Choice(context.Background(), stageSampleRequest()); !errors.Is(err, ErrMalformedResponse) {
		t.Fatal("oversized response accepted", err)
	}
}

// Spec: docs/specs/task-retrospective.md via JevClient
func TestGlideHonorsRetryAfterForWarmingAndUnavailable(t *testing.T) {
	for _, status := range []int{425, 429, 503} {
		_, cfg := newMockServer(t, map[int]mockResponse{1: {status: status, headers: map[string]string{"Retry-After": "120"}}})
		delays := []time.Duration{}
		if _, err := newJev(t, cfg, func(d time.Duration) { delays = append(delays, d) }).Choice(context.Background(), stageSampleRequest()); err != nil || len(delays) != 1 || delays[0] != 30*time.Second {
			t.Fatal("Retry-After rail lost", status, err)
		}
	}
}

// Spec: docs/specs/task-retrospective.md via JevClient
func TestGlideProfileAndUnverifiedJevFailClosed(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Evaluator = EvaluatorJev
	if cfg.Validate() == nil {
		t.Fatal("unverified Jev accepted")
	}
	if _, err := NewJevClient("http://127.0.0.1", "synthetic", cfg).Choice(context.Background(), stageSampleRequest()); !errors.Is(err, ErrPermanent) {
		t.Fatal("unverified request sent")
	}
}

// Spec: docs/specs/task-retrospective.md via JevClient
func TestGlideNoSilentEvidenceFallback(t *testing.T) {
	mock, cfg := newMockServer(t, nil)
	for _, mutate := range []func(*ChoiceRequest){func(q *ChoiceRequest) { q.Requests = nil }, func(q *ChoiceRequest) { q.State = json.RawMessage(`bad`) }, func(q *ChoiceRequest) { q.Requests[0].Evidence = json.RawMessage(`bad`) }, func(q *ChoiceRequest) { q.Requests[0].Choices = []string{"only"} }} {
		q := stageSampleRequest()
		mutate(q)
		if _, err := newJev(t, cfg, nil).Choice(context.Background(), q); !errors.Is(err, ErrInvalidRequest) || len(mock.recorded()) != 0 {
			t.Fatal("invalid request sent", err)
		}
	}
}
