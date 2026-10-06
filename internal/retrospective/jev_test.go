// Spec: docs/specs/task-retrospective.md
package retrospective

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// recordedCall captures one mocked HTTP request exchange for assertions.
// Spec: docs/specs/task-retrospective.md via JevClient
type recordedCall struct {
	Path string
	Body []byte
	Auth string
}

// mockServer is a scripted recording Jev endpoint; responses map the 1-based
// call index to the scripted status/body/headers. A missing script answers
// every question with its first listed choice (then tests override it).
// Spec: docs/specs/task-retrospective.md via JevClient
type mockServer struct {
	mu          sync.Mutex
	server      *httptest.Server
	calls       []*recordedCall
	responses   map[int]mockResponse
	answersByID map[string]Answer
}

type mockResponse struct {
	status  int
	body    string
	headers map[string]string
	delay   time.Duration
}

func newMockServer(t *testing.T, responses map[int]mockResponse) (*mockServer, *Config) {
	t.Helper()
	mock := &mockServer{responses: responses, answersByID: map[string]Answer{}}
	mock.server = httptest.NewServer(mock.handler())
	t.Cleanup(mock.server.Close)
	cfg := DefaultConfig()
	cfg.Endpoint = mock.server.URL
	return mock, cfg
}

func (mock *mockServer) handler() http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		n := mock.record(&recordedCall{Path: request.URL.Path, Body: body, Auth: request.Header.Get("Authorization")})
		mock.mu.Lock()
		scripted, scriptedFound := mock.responses[n]
		mock.mu.Unlock()
		if scriptedFound && scripted.delay > 0 {
			time.Sleep(scripted.delay)
		}
		if scriptedFound && scripted.status > 0 {
			for header, value := range scripted.headers {
				writer.Header().Set(header, value)
			}
			if scripted.status/100 != 2 {
				writer.WriteHeader(scripted.status)
				return
			}
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(scripted.body))
			return
		}
		decoded := &ChoiceRequest{}
		_ = json.Unmarshal(body, decoded)
		answers := []Answer{}
		for _, question := range decoded.Requests {
			mock.mu.Lock()
			scriptedAnswer, scripted := mock.answersByID[question.ID]
			mock.mu.Unlock()
			if !scripted {
				scriptedAnswer = Answer{
					RequestID:   question.ID,
					Subject:     question.Subject,
					Choice:      question.Choices[0],
					Probability: f64(0.9),
					Confidence:  f64(0.9),
				}
			}
			answers = append(answers, scriptedAnswer)
		}
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(ChoiceResponse{Responses: answers})
	})
}

// record appends one captured call and returns its 1-based index.
// Spec: docs/specs/task-retrospective.md
func (mock *mockServer) record(call *recordedCall) int {
	mock.mu.Lock()
	defer mock.mu.Unlock()
	mock.calls = append(mock.calls, call)
	return len(mock.calls)
}

// recorded returns a stable copy of the captured calls.
// Spec: docs/specs/task-retrospective.md
func (mock *mockServer) recorded() []*recordedCall {
	mock.mu.Lock()
	defer mock.mu.Unlock()
	return append([]*recordedCall(nil), mock.calls...)
}

// newJev builds a JevClient against the mock endpoint with an injectable
// sleep function (deterministic retry-elapsed assertions).
// Spec: docs/specs/task-retrospective.md
func newJev(t *testing.T, cfg *Config, sleep func(d time.Duration)) *JevClient {
	t.Helper()
	if cfg.Endpoint == "" {
		t.Fatal("mock endpoint missing")
	}
	client := NewJevClient(cfg.Endpoint, "host-key-fixture", cfg)
	if sleep != nil {
		client.sleep = sleep
	}
	return client
}

func stageSampleRequest() *ChoiceRequest {
	return buildRequest(JevModel, []byte(`{"stage":"one"}`), nil, []Question{{
		ID: outcomeQuestionID, Subject: "task", Scope: ScopeOutcome,
		Text: "any", Choices: outcomeChoices,
	}})
}

func TestJevSendsPinnedModelAndBearerAuth(t *testing.T) {
	mock, cfg := newMockServer(t, nil)
	client := newJev(t, cfg, nil)
	if _, err := client.Choice(context.Background(), stageSampleRequest()); err != nil {
		t.Fatal(err)
	}
	calls := mock.recorded()
	if len(calls) != 1 {
		t.Fatalf("calls = %d", len(calls))
	}
	if calls[0].Path != choiceEndpointPath {
		t.Fatalf("endpoint path = %q", calls[0].Path)
	}
	if calls[0].Auth != "Bearer host-key-fixture" {
		t.Fatalf("auth header missing: %q", calls[0].Auth)
	}
	var sent ChoiceRequest
	if err := json.Unmarshal(calls[0].Body, &sent); err != nil {
		t.Fatal(err)
	}
	if sent.Model != "jev-1.13.0" {
		t.Fatalf("model = %q", sent.Model)
	}
}

func TestJevRetriesUnavailableAndHonorsRetryAfterThenSucceeds(t *testing.T) {
	mock, cfg := newMockServer(t, map[int]mockResponse{
		1: {status: http.StatusServiceUnavailable},
		2: {status: http.StatusTooManyRequests, headers: map[string]string{"Retry-After": "1"}},
	})
	var waited time.Duration
	client := newJev(t, cfg, func(delay time.Duration) { waited += delay })
	if _, err := client.Choice(context.Background(), stageSampleRequest()); err != nil {
		t.Fatal(err)
	}
	usage := client.Usage()
	if usage.Requests != 3 || usage.Retries != 2 {
		t.Fatalf("usage = %+v", usage)
	}
	if waited < time.Second {
		t.Fatalf("Retry-After was not honored: waited %s", waited)
	}
	if waited > 2*time.Second {
		t.Fatalf("Retry-After overshoot: waited %s", waited)
	}
	if len(mock.recorded()) != 3 {
		t.Fatalf("calls = %d", len(mock.recorded()))
	}
}

func TestJevRateLimitedExhaustsAttempts(t *testing.T) {
	mock, cfg := newMockServer(t, map[int]mockResponse{
		1: {status: http.StatusTooManyRequests, headers: map[string]string{"Retry-After": "0"}},
		2: {status: http.StatusTooManyRequests, headers: map[string]string{"Retry-After": "0"}},
		3: {status: http.StatusTooManyRequests, headers: map[string]string{"Retry-After": "0"}},
	})
	client := newJev(t, cfg, nil)
	if _, err := client.Choice(context.Background(), stageSampleRequest()); err == nil {
		t.Fatal("persistent 429 was accepted")
	} else if err.Error() != "jev: rate limited" {
		t.Fatalf("error = %v", err)
	}
	if len(mock.recorded()) != 3 {
		t.Fatalf("attempts = %d", len(mock.recorded()))
	}
}

func TestJevMalformedResponseRetriesAndFails(t *testing.T) {
	mock, cfg := newMockServer(t, map[int]mockResponse{
		1: {status: 200, body: "not json"},
		2: {status: 200, body: "still not json"},
		3: {status: 200, body: `{`},
	})
	client := newJev(t, cfg, nil)
	if _, err := client.Choice(context.Background(), stageSampleRequest()); err == nil || err.Error() != "jev: malformed response" {
		t.Fatalf("malformed response handling = %v", err)
	}
	if usage := client.Usage(); usage.Retries != 2 {
		t.Fatalf("usage = %+v", usage)
	}
	if len(mock.recorded()) != 3 {
		t.Fatalf("calls = %d", len(mock.recorded()))
	}
}

func TestJevSizeRejectedIsNeverRetriedAtHTTPLevel(t *testing.T) {
	mock, cfg := newMockServer(t, map[int]mockResponse{1: {status: http.StatusRequestEntityTooLarge}})
	client := newJev(t, cfg, nil)
	_, err := client.Choice(context.Background(), stageSampleRequest())
	if err == nil || err.Error() != "jev: request size rejected" {
		t.Fatalf("413 handling = %v", err)
	}
	if len(mock.recorded()) != 1 {
		t.Fatalf("413 was retried at HTTP level: %d calls", len(mock.recorded()))
	}
	if usage := client.Usage(); usage.SizeRejections != 1 {
		t.Fatalf("size rejection not accounted: %+v", usage)
	}
}

func TestJevPermanentRejectStopsWithoutRetries(t *testing.T) {
	mock, cfg := newMockServer(t, map[int]mockResponse{1: {status: http.StatusUnauthorized}})
	client := newJev(t, cfg, nil)
	_, err := client.Choice(context.Background(), stageSampleRequest())
	if err == nil {
		t.Fatal("401 was accepted")
	}
	if len(mock.recorded()) != 1 {
		t.Fatalf("401 was retried: %d calls", len(mock.recorded()))
	}
}

func TestJevBudgetExhaustedBlocksFurtherRequests(t *testing.T) {
	mock, cfg := newMockServer(t, nil)
	cfg.RequestBudget = 1
	client := newJev(t, cfg, nil)
	if _, err := client.Choice(context.Background(), stageSampleRequest()); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Choice(context.Background(), stageSampleRequest()); err != ErrBudgetExhausted {
		t.Fatalf("second call error = %v", err)
	}
	if usage := client.Usage(); !usage.Exhausted || usage.Requests != 1 {
		t.Fatalf("usage = %+v", usage)
	}
	if len(mock.recorded()) != 1 {
		t.Fatalf("budget exhausted still sent requests: %d", len(mock.recorded()))
	}
}
