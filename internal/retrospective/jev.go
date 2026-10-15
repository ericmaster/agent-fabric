// Spec: docs/specs/task-retrospective.md
package retrospective

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
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
	// ErrContentOverBudget is returned when a locally measured page still
	// exceeds its serialized budgets at dispatch; the caller re-packs it
	// with halved content budgets instead of halting.
	ErrContentOverBudget = errors.New("jev: page exceeds the serialized budget")
	// ErrRateLimited is returned when HTTP 429 persists across all attempts.
	ErrRateLimited = errors.New("jev: rate limited")
	// ErrUnavailable is returned when the service/transport stays unavailable.
	ErrUnavailable = errors.New("jev: evaluator unavailable")
	// ErrPermanent is returned for non-recoverable HTTP client errors.
	ErrPermanent = errors.New("jev: permanent rejection")
	// ErrMalformedResponse is returned when a 200 response is not parseable
	// into the choice contract across all attempts.
	ErrMalformedResponse = errors.New("jev: malformed response")
	// ErrInvalidRequest is an encoding/validation failure, never a size rejection.
	ErrInvalidRequest = errors.New("evaluator: invalid request")
)

// systemOnePath is the shared decision-engine route appended to the endpoint.
const systemOnePath = SystemOnePath

// GLiDE requires a non-empty question map; empty requests fail before transport.
// Spec: docs/specs/task-retrospective.md via systemOneWireRequest
func providerQuestions(request *ChoiceRequest) []Question {
	return request.Requests
}

// systemOneQuestion is one typed choice question of the System One wire
// contract of Fastino GLiDE. The internal question id
// becomes the map key; the instructions carry the question text; criteria map
// every supplied choice label onto itself (the retrospective choice labels
// are self-describing).
type systemOneQuestion struct {
	Type         string            `json:"type"`
	Instructions string            `json:"instructions"`
	Criteria     map[string]string `json:"criteria"`
}

// systemOneRequest is the serialized wire request: one shared state plus the
// page's named questions.
type systemOneRequest struct {
	Model     string                       `json:"model"`
	State     json.RawMessage              `json:"state"`
	Questions map[string]systemOneQuestion `json:"questions"`
}

// systemOneAnswer is one typed choice answer: the winning choice, its
// probability inside the full distribution, and the confidence margin.
type systemOneAnswer struct {
	Type          string              `json:"type"`
	Choice        string              `json:"choice"`
	Confidence    *float64            `json:"confidence"`
	Probabilities map[string]*float64 `json:"probabilities"`
}

// systemOneResponse is the decoded wire response document. Usage is accepted
// (and ignored); unknown extra fields still abstain as malformed.
type systemOneResponse struct {
	Model      string                     `json:"model"`
	Answers    map[string]systemOneAnswer `json:"answers"`
	Usage      json.RawMessage            `json:"usage,omitempty"`
	TokenUsage json.RawMessage            `json:"token_usage,omitempty"`
}

// Question is one choice request over supplied record ids. The evidence
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

// ChoiceRequest is one internal evaluation request. The wire translation to
// the System One contract happens inside the client; the evaluator measures
// the serialized bytes against the conservative config budgets before any
// send, including folded evidence, overlap and criteria maps.
// Spec: docs/specs/task-retrospective.md via JevClient
type ChoiceRequest struct {
	Model    string          `json:"model"`
	State    json.RawMessage `json:"state"`
	Requests []Question      `json:"requests"`
}

// Answer is one returned choice over a supplied request id. Probability and
// confidence are pointers: an absent value stays distinguishable from a zero
// value and abstains.
// Spec: docs/specs/task-retrospective.md via ChoiceResponse
type Answer struct {
	RequestID   string   `json:"request_id"`
	Subject     string   `json:"subject"`
	Choice      string   `json:"choice"`
	Labels      []string `json:"labels,omitempty"`
	Probability *float64 `json:"probability"`
	Confidence  *float64 `json:"confidence"`
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

// JevClient retains its legacy Go name; it sends the verified GLiDE System
// One contract. Unverified Jev selection fails closed without network.
// Spec: docs/specs/task-retrospective.md
type JevClient struct {
	endpoint string
	apiKey   string
	cfg      *Config
	profile  string
	model    string

	client *http.Client
	sleep  func(time.Duration)
	usage  JevUsage
}

// NewJevClient builds the client for one analysis run. The API key is the
// host-supplied profile key (FASTINO_API_KEY or TYPESAFE_API_KEY) held only
// in process memory; it is never logged, persisted, or included in any report.
// Spec: docs/specs/task-retrospective.md via JevClient
func NewJevClient(endpoint, apiKey string, cfg *Config) *JevClient {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	if endpoint != "" {
		endpoint = trimEndpoint(endpoint) + systemOnePath
	}
	profile := EvaluatorGlide
	if cfg != nil && cfg.Evaluator != "" {
		profile = cfg.Evaluator
	}
	return &JevClient{
		endpoint: endpoint,
		apiKey:   apiKey,
		cfg:      cfg,
		profile:  profile,
		model:    EvaluatorModel(profile),
		client:   &http.Client{Timeout: cfg.HTTPTimeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
		sleep:    time.Sleep,
	}
}

// wireRequest translates one internal request onto the System One contract.
// The question id becomes the map key; choices become criteria labels. The
// System One contract folds optional question evidence into shared state.
// The integrated analyzer instead sends one compact signal packet referenced
// by all three questions. Encoding errors fail closed, never drop evidence.
// Spec: docs/specs/task-retrospective.md via JevClient
func systemOneWireRequest(request *ChoiceRequest) (*systemOneRequest, error) {
	if request == nil {
		return nil, ErrInvalidRequest
	}
	if len(request.Requests) == 0 {
		return nil, ErrInvalidRequest
	}
	state := request.State
	var stateMap map[string]json.RawMessage
	if err := json.Unmarshal(state, &stateMap); err != nil || stateMap == nil {
		return nil, ErrInvalidRequest
	}
	var evidence map[string]json.RawMessage
	for _, question := range providerQuestions(request) {
		if len(question.Evidence) == 0 {
			continue
		}
		if !json.Valid(question.Evidence) || bytes.Equal(bytes.TrimSpace(question.Evidence), []byte("null")) {
			return nil, ErrInvalidRequest
		}
		if evidence == nil {
			evidence = make(map[string]json.RawMessage)
		}
		evidence[question.ID] = question.Evidence
	}
	if len(evidence) > 0 {
		if len(evidence) > 0 {
			encoded, err := json.Marshal(evidence)
			if err != nil {
				return nil, err
			}
			stateMap["evidence"] = encoded
		}
		merged, err := json.Marshal(stateMap)
		if err != nil {
			return nil, err
		}
		state = merged
	}
	wire := &systemOneRequest{
		Model:     request.Model,
		State:     state,
		Questions: make(map[string]systemOneQuestion, len(providerQuestions(request))),
	}
	for _, question := range providerQuestions(request) {
		if question.ID == "" || question.Subject == "" || question.Text == "" || len(question.Choices) < 2 || len(question.Choices) > 255 {
			return nil, ErrInvalidRequest
		}
		if _, exists := wire.Questions[question.ID]; exists {
			return nil, ErrInvalidRequest
		}
		criteria := make(map[string]string, len(question.Choices))
		for _, choice := range question.Choices {
			if choice == "" {
				return nil, ErrInvalidRequest
			}
			if _, exists := criteria[choice]; exists {
				return nil, ErrInvalidRequest
			}
			criteria[choice] = choice
		}
		wire.Questions[question.ID] = systemOneQuestion{
			Type:         "choice",
			Instructions: fmt.Sprintf("Subject ID: %s (scope: %s). %s", question.Subject, question.Scope, question.Text),
			Criteria:     criteria,
		}
	}
	return wire, nil
}

// wireAnswer maps one System One answer back onto the internal contract. The
// winning choice's probability comes from the distribution; an absent
// probability or confidence stays nil so the evaluator abstains.
// Spec: docs/specs/task-retrospective.md via JevClient
func wireAnswer(request *ChoiceRequest, id string, answer systemOneAnswer) Answer {
	internal := Answer{RequestID: id, Choice: answer.Choice, Confidence: answer.Confidence}
	for _, question := range providerQuestions(request) {
		if question.ID == id {
			internal.Subject = question.Subject
			break
		}
	}
	if answer.Probabilities != nil {
		if probability, ok := answer.Probabilities[answer.Choice]; ok {
			internal.Probability = probability
		}
	}
	return internal
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
	if request == nil {
		return nil, ErrInvalidRequest
	}
	if j.profile != EvaluatorGlide {
		return nil, fmt.Errorf("%w: Jev contract is not verified", ErrPermanent)
	}
	copyRequest := *request
	copyRequest.Model = j.model
	request = &copyRequest
	wire, err := systemOneWireRequest(request)
	if err != nil {
		return nil, fmt.Errorf("encode evaluator request: %w", err)
	}
	body, err := json.Marshal(wire)
	if err != nil {
		return nil, fmt.Errorf("encode jev request: %w", err)
	}
	measure := measureRequest(request)
	if measure.Request > j.cfg.RequestLimitBytes || measure.State+measure.LongestQuestion > j.cfg.StateQuestionLimitBytes {
		return nil, ErrContentOverBudget
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
		response, retryable, err := j.attempt(ctx, body, request)
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
func (j *JevClient) attempt(ctx context.Context, body []byte, request *ChoiceRequest) (*ChoiceResponse, bool, error) {
	posted, err := http.NewRequestWithContext(ctx, http.MethodPost, j.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, false, fmt.Errorf("jev request endpoint: %w", err)
	}
	posted.Header.Set("Content-Type", "application/json")
	if j.apiKey != "" {
		if j.profile == EvaluatorGlide {
			// Fastino GLiDE authenticates with the X-API-Key header.
			posted.Header.Set("X-API-Key", j.apiKey)
		} else {
			posted.Header.Set("Authorization", "Bearer "+j.apiKey)
		}
	}
	response, err := j.client.Do(posted)
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
	case response.StatusCode == 425:
		// GLiDE: deployment warming — retryable.
		return nil, true, j.retryAfter(response, ErrUnavailable)
	case response.StatusCode >= 500:
		// 503 (GLiDE serving) and 529 (Jev busy) included.
		return nil, true, j.retryAfter(response, ErrUnavailable)
	case response.StatusCode >= 400:
		return nil, false, fmt.Errorf("%w: http %d", ErrPermanent, response.StatusCode)
	case response.StatusCode/100 != 2:
		return nil, false, ErrPermanent
	}
	payload, err := io.ReadAll(io.LimitReader(response.Body, (1<<22)+1))
	if err != nil || len(payload) > 1<<22 {
		return nil, true, ErrMalformedResponse
	}
	if !uniqueJSONKeys(payload) {
		return nil, true, ErrMalformedResponse
	}
	wire := &systemOneResponse{}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(wire); err != nil {
		// Unknown/extra response fields or wrong types abstain as malformed.
		return nil, true, ErrMalformedResponse
	}
	var trailing struct{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, true, ErrMalformedResponse
	}
	internal := &ChoiceResponse{}
	if wire.Model != "glide" || wire.Answers == nil || len(wire.Answers) != len(providerQuestions(request)) {
		return nil, true, ErrMalformedResponse
	}
	for _, question := range providerQuestions(request) {
		answer, ok := wire.Answers[question.ID]
		if !ok || !validSystemOneAnswer(question, answer) {
			return nil, true, ErrMalformedResponse
		}
		internal.Responses = append(internal.Responses, wireAnswer(request, question.ID, answer))
	}
	return internal, false, nil
}

// rateLimited interprets Retry-After into an honored wait, carried through
// the retryable error chain.
// Spec: docs/specs/task-retrospective.md via JevClient
func (j *JevClient) rateLimited(response *http.Response) error {
	return j.retryAfter(response, ErrRateLimited)
}

// Spec: docs/specs/task-retrospective.md via attempt
func (j *JevClient) retryAfter(response *http.Response, cause error) error {
	header := response.Header.Get("Retry-After")
	seconds := -1
	if header != "" {
		if parsed, err := strconv.Atoi(header); err == nil && parsed >= 0 {
			seconds = parsed
		} else if when, err := http.ParseTime(header); err == nil {
			if wait := time.Until(when); wait > 0 {
				seconds = int(math.Ceil(wait.Seconds()))
			} else {
				seconds = 0
			}
		}
	}
	if seconds < 0 {
		return cause
	}
	if seconds > j.cfg.RetryAfterMaxSeconds {
		seconds = j.cfg.RetryAfterMaxSeconds
	}
	return &retryAfterError{err: cause, seconds: seconds}
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
func buildRequest(model string, state []byte, requests []Question) *ChoiceRequest {
	return &ChoiceRequest{Model: model, State: state, Requests: requests}
}

// measureRequest measures the serialized request bytes for the budget gates.
// Spec: docs/specs/task-retrospective.md
func measureRequest(request *ChoiceRequest) ReportBytes {
	wire, err := systemOneWireRequest(request)
	if err != nil {
		return ReportBytes{Request: math.MaxInt / 4, State: math.MaxInt / 4}
	}
	body, err := json.Marshal(wire)
	if err != nil {
		return ReportBytes{Request: math.MaxInt / 4, State: math.MaxInt / 4}
	}
	state := len(wire.State)
	longest := 0
	for id, question := range wire.Questions {
		encoded, err := json.Marshal(map[string]systemOneQuestion{id: question})
		if err == nil && len(encoded) > longest {
			longest = len(encoded)
		}
	}
	return ReportBytes{Request: len(body), State: state, LongestQuestion: longest}
}

// validSystemOneAnswer validates the full distribution, not just its selected cell.
// Spec: docs/specs/task-retrospective.md via attempt
func validSystemOneAnswer(question Question, answer systemOneAnswer) bool {
	if answer.Type != "choice" || !containsString(question.Choices, answer.Choice) || answer.Confidence == nil || !probabilityInRange(*answer.Confidence) || len(answer.Probabilities) != len(question.Choices) {
		return false
	}
	total, highest := 0.0, 0.0
	for _, choice := range question.Choices {
		p, ok := answer.Probabilities[choice]
		if !ok || p == nil || !probabilityInRange(*p) {
			return false
		}
		total += *p
		if *p > highest {
			highest = *p
		}
	}
	return math.Abs(total-1) <= 0.00001 && *answer.Probabilities[answer.Choice] >= highest
}

// uniqueJSONKeys refuses duplicate object keys before map decoding can hide them.
// Spec: docs/specs/task-retrospective.md via attempt
func uniqueJSONKeys(payload []byte) bool {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	var value func(int) bool
	value = func(depth int) bool {
		if depth > 128 {
			return false
		}
		token, err := decoder.Token()
		if err != nil {
			return false
		}
		delim, composite := token.(json.Delim)
		if !composite {
			return true
		}
		if delim != '{' && delim != '[' {
			return false
		}
		seen := map[string]bool{}
		for decoder.More() {
			if delim == '{' {
				key, err := decoder.Token()
				if err != nil {
					return false
				}
				name, ok := key.(string)
				if !ok || seen[name] {
					return false
				}
				seen[name] = true
			}
			if !value(depth + 1) {
				return false
			}
		}
		_, err = decoder.Token()
		return err == nil
	}
	if !value(0) {
		return false
	}
	_, err := decoder.Token()
	return err == io.EOF
}
