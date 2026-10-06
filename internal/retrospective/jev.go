// Spec: docs/specs/task-retrospective.md
package retrospective

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// Sentinel client errors. The evaluator maps them onto status outcomes;
// ErrSizeRejected is handled by re-packing/splitting, never by omission.
var (
	// ErrBudgetExhausted is returned when the run's request budget is used up.
	ErrBudgetExhausted = errors.New("jev: request budget exhausted")
	// ErrSizeRejected is returned on HTTP 413 size rejection.
	ErrSizeRejected = errors.New("jev: request size rejected")
	// ErrRateLimited is returned when HTTP 429 persists across all attempts.
	ErrRateLimited = errors.New("jev: rate limited")
	// ErrUnavailable is returned when the service/transport stays unavailable.
	ErrUnavailable = errors.New("jev: evaluator unavailable")
	// ErrPermanent is returned for non-recoverable HTTP client errors.
	ErrPermanent = errors.New("jev: permanent rejection")
	// ErrMalformedResponse is returned when a 200 response is not parseable
	// into the choice contract across all attempts.
	ErrMalformedResponse = errors.New("jev: malformed response")
)

// choiceEndpointPath is appended to the configured endpoint URL.
const choiceEndpointPath = "/v1/choices"

// Question is one Jev choice request over supplied record ids. The evidence
// payload carries the record content the question asks about (redacted).
// Spec: docs/specs/task-retrospective.md via ChoiceRequest
type Question struct {
	ID       string          `json:"id"`
	Subject  string          `json:"subject"`
	Scope    string          `json:"scope"`
	Text     string          `json:"text"`
	Choices  []string        `json:"choices"`
	Evidence json.RawMessage `json:"evidence,omitempty"`
}

// ChoiceRequest is one serialized Jev request. The evaluator measures the
// serialized bytes against the conservative config budgets before any send.
// Spec: docs/specs/task-retrospective.md via JevClient
type ChoiceRequest struct {
	Model    string          `json:"model"`
	State    json.RawMessage `json:"state"`
	Requests []Question      `json:"requests"`
	// Overlaps are the previous page's tail records re-presented for context
	// on the next page (each with an explicit overlap marker; evaluator-side
	// accounting deduplicates by record id).
	Overlaps []evidenceRecord `json:"overlap,omitempty"`
}

// Answer is one returned choice over a supplied request id.
// Spec: docs/specs/task-retrospective.md via ChoiceResponse
type Answer struct {
	RequestID   string   `json:"request_id"`
	Subject     string   `json:"subject"`
	Choice      string   `json:"choice"`
	Labels      []string `json:"labels,omitempty"`
	Probability float64  `json:"probability"`
	Confidence  float64  `json:"confidence"`
}

// ChoiceResponse is the decoded Jev response document.
// Spec: docs/specs/task-retrospective.md via JevClient
type ChoiceResponse struct {
	Responses []Answer `json:"responses"`
}

// ChoiceClient is the small client interface all Jev HTTP interaction goes
// through. Deterministic tests implement this directly; HTTP-recording tests
// construct JevClient against httptest mock servers.
// Spec: docs/specs/task-retrospective.md
type ChoiceClient interface {
	Choice(ctx context.Context, request *ChoiceRequest) (*ChoiceResponse, error)
	// Usage reports the request accounting of this client (requests sent,
	// retries executed, size rejections handled).
	Usage() JevUsage
}

// JevUsage is the request accounting of one analysis run.
// Spec: docs/specs/task-retrospective.md via JevUsage
type JevUsage struct {
	Requests       int  `json:"requests"`
	Retries        int  `json:"retries"`
	SizeRejections int  `json:"size_rejections"`
	Budget         int  `json:"budget"`
	Exhausted      bool `json:"exhausted"`
}

// JevClient is the pinned HTTP choice client.
// Spec: docs/specs/task-retrospective.md
type JevClient struct {
	endpoint string
	apiKey   string
	cfg      *Config

	client *http.Client
	sleep  func(time.Duration)
	usage  JevUsage
}

// NewJevClient builds the client for one analysis run. The API key is the
// host-supplied TYPESAFE_API_KEY value held only in process memory; it is
// never logged, persisted, or included in any report.
// Spec: docs/specs/task-retrospective.md via JevClient
func NewJevClient(endpoint, apiKey string, cfg *Config) *JevClient {
	if endpoint != "" {
		endpoint = trimEndpoint(endpoint) + choiceEndpointPath
	}
	return &JevClient{
		endpoint: endpoint,
		apiKey:   apiKey,
		cfg:      cfg,
		client:   &http.Client{Timeout: cfg.HTTPTimeout},
		sleep:    time.Sleep,
	}
}

// trimEndpoint removes a trailing slash so the request path stays canonical.
// Spec: docs/specs/task-retrospective.md via JevClient
func trimEndpoint(endpoint string) string {
	for len(endpoint) > 0 && endpoint[len(endpoint)-1] == '/' {
		endpoint = endpoint[:len(endpoint)-1]
	}
	return endpoint
}

// Choice performs the request with up to MaxAttemptsPerRequest attempts.
// ErrSizeRejected surfaces immediately so the evaluator can split the page.
// Spec: docs/specs/task-retrospective.md via JevClient
func (j *JevClient) Choice(ctx context.Context, request *ChoiceRequest) (*ChoiceResponse, error) {
	if j.cfg == nil {
		return nil, fmt.Errorf("jev client requires a config")
	}
	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("encode jev request: %w", err)
	}
	var lastErr error
	for attempt := 1; attempt <= j.cfg.MaxAttemptsPerRequest; attempt++ {
		if j.usage.Requests >= j.cfg.RequestBudget {
			j.usage.Exhausted = true
			return nil, ErrBudgetExhausted
		}
		j.usage.Requests++
		if attempt > 1 {
			j.usage.Retries++
		}
		response, retryable, err := j.attempt(ctx, body)
		if err == nil {
			return response, nil
		}
		if !retryable {
			return nil, err
		}
		lastErr = err
		if attempt < j.cfg.MaxAttemptsPerRequest {
			j.sleep(j.backoff(attempt, err))
		}
	}
	if errors.Is(lastErr, ErrRateLimited) || errors.Is(lastErr, ErrMalformedResponse) {
		return nil, lastErr
	}
	return nil, ErrUnavailable
}

// attempt performs one HTTP attempt and maps the response/transport outcome.
// Spec: docs/specs/task-retrospective.md via JevClient
func (j *JevClient) attempt(ctx context.Context, body []byte) (*ChoiceResponse, bool, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, j.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, false, fmt.Errorf("jev request endpoint: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	if j.apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+j.apiKey)
	}
	response, err := j.client.Do(request)
	if err != nil {
		return nil, true, ErrUnavailable
	}
	defer response.Body.Close()
	switch {
	case response.StatusCode == http.StatusRequestEntityTooLarge:
		j.usage.SizeRejections++
		return nil, false, ErrSizeRejected
	case response.StatusCode == http.StatusTooManyRequests:
		return nil, true, j.rateLimited(response)
	case response.StatusCode >= 500:
		return nil, true, ErrUnavailable
	case response.StatusCode >= 400:
		return nil, false, fmt.Errorf("%w: http %d", ErrPermanent, response.StatusCode)
	case response.StatusCode/100 != 2:
		return nil, true, ErrUnavailable
	}
	payload, err := io.ReadAll(io.LimitReader(response.Body, 1<<22))
	if err != nil {
		return nil, true, ErrMalformedResponse
	}
	decoded := &ChoiceResponse{}
	if err := json.Unmarshal(payload, decoded); err != nil {
		return nil, true, ErrMalformedResponse
	}
	return decoded, false, nil
}

// rateLimited interprets Retry-After into an honored wait, carried through
// the retryable error chain.
// Spec: docs/specs/task-retrospective.md via JevClient
func (j *JevClient) rateLimited(response *http.Response) error {
	header := response.Header.Get("Retry-After")
	seconds := -1
	if header != "" {
		if parsed, err := strconv.Atoi(header); err == nil && parsed >= 0 {
			seconds = parsed
		} else if when, err := http.ParseTime(header); err == nil {
			if wait := time.Until(when); wait > 0 {
				seconds = int(wait.Seconds())
			} else {
				seconds = 0
			}
		}
	}
	if seconds < 0 {
		return ErrRateLimited
	}
	if seconds > j.cfg.RetryAfterMaxSeconds {
		seconds = j.cfg.RetryAfterMaxSeconds
	}
	return &retryAfterError{err: ErrRateLimited, seconds: seconds}
}

// retryAfterError carries an honored Retry-After delay.
// Spec: docs/specs/task-retrospective.md via JevClient
type retryAfterError struct {
	err     error
	seconds int
}

func (e *retryAfterError) Error() string { return e.err.Error() }
func (e *retryAfterError) Unwrap() error { return e.err }

// backoff returns the delay before the next attempt: an honored Retry-After
// when present, otherwise capped exponential backoff (base 300ms doubled per
// attempt, capped at the configured maximum).
// Spec: docs/specs/task-retrospective.md via JevClient
func (j *JevClient) backoff(attempt int, err error) time.Duration {
	var after *retryAfterError
	if errors.As(err, &after) {
		return time.Duration(after.seconds) * time.Second
	}
	delay := j.cfg.BackoffBase << (attempt - 1)
	if delay > j.cfg.BackoffMax {
		delay = j.cfg.BackoffMax
	}
	return delay
}

// Usage exposes the run's request accounting.
// Spec: docs/specs/task-retrospective.md via JevClient
func (j *JevClient) Usage() JevUsage { return j.usage }

// buildRequest assembles one serialized request to send.
// Spec: docs/specs/task-retrospective.md via JevClient
func buildRequest(model string, state []byte, overlap []evidenceRecord, requests []Question) *ChoiceRequest {
	request := &ChoiceRequest{Model: model, State: state, Requests: requests}
	if len(overlap) > 0 {
		request.Overlaps = overlap
	}
	return request
}

// measureRequest measures the serialized request bytes for the budget gates.
// Spec: docs/specs/task-retrospective.md
func measureRequest(request *ChoiceRequest) ReportBytes {
	body, err := json.Marshal(request)
	if err != nil {
		return ReportBytes{}
	}
	state := len(request.State)
	longest := 0
	for _, question := range request.Requests {
		encoded, err := json.Marshal(question)
		if err == nil && len(encoded) > longest {
			longest = len(encoded)
		}
	}
	return ReportBytes{Request: len(body), State: state, LongestQuestion: longest}
}
