// Spec: docs/specs/task-retrospective.md
package retrospective

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"
)

// GLiDE is the verified default. Jev profile identifiers are reserved, but
// validation rejects Jev until its current pinned-model contract is verified.
// Model ids are not config-overridable.
const (
	// EvaluatorGlide is the default profile: Fastino GLiDE.
	EvaluatorGlide = "glide"
	// EvaluatorJev selects the TypeSafe Jev profile.
	EvaluatorJev = "jev"
	// GlideModel is the pinned Fastino GLiDE model identifier.
	GlideModel = "fastino/GLiDE"
	// JevModel is the pinned Jev model identifier for retrospective choices.
	JevModel = "jev-1.13.0"
	// SystemOnePath is the shared decision-engine route appended to the
	// configured endpoint.
	SystemOnePath = "/v1/systemone"
)

// EvaluatorModel returns the pinned model id of one profile ("" unknown).
// Spec: docs/specs/task-retrospective.md via Config
func EvaluatorModel(evaluator string) string {
	switch evaluator {
	case EvaluatorGlide:
		return GlideModel
	case EvaluatorJev:
		return JevModel
	}
	return ""
}

// EvaluatorEndpoint returns the default endpoint of one profile ("" unknown).
// Spec: docs/specs/task-retrospective.md via Config
func EvaluatorEndpoint(evaluator string) string {
	switch evaluator {
	case EvaluatorGlide:
		return "https://api.fastino.ai"
	case EvaluatorJev:
		return "https://api.typesafe.ai"
	}
	return ""
}

// EvaluatorAPIKeyEnv returns the host environment variable holding the
// profile's API key (read at request time; never persisted or logged).
// Spec: docs/specs/task-retrospective.md via Config
func EvaluatorAPIKeyEnv(evaluator string) string {
	switch evaluator {
	case EvaluatorGlide:
		return "FASTINO_API_KEY"
	case EvaluatorJev:
		return "TYPESAFE_API_KEY"
	}
	return ""
}

// EvaluatorAPIKey reads the profile's host-supplied key from the environment
// at request time. Empty means the evaluator abstains as unavailable.
// Spec: docs/specs/task-retrospective.md via Config
func EvaluatorAPIKey(evaluator string) string {
	env := EvaluatorAPIKeyEnv(evaluator)
	if env == "" {
		return ""
	}
	return os.Getenv(env)
}

// OpenCode session transport generations (dispatch_endpoint).
const (
	// EndpointPrompt is the <=1.18.x transport: POST /session/{id}/prompt.
	EndpointPrompt = "prompt"
	// EndpointMessage is the verified live 2.x transport: /api prefix,
	// Basic auth, data envelopes, synchronous flat-text prompt delivery.
	EndpointMessage = "message"
)

// RubricVersion identifies the deterministic counting rubric of the report.
const RubricVersion = SignalDetectorVersion

// Config is the single contract for retrospective defaults and thresholds.
// Later phases (dispatch, OpenCode decoding) read the same struct; adapters
// must not duplicate the values.
type Config struct {
	SignalConfig
	InterpretationSignalCap     int     `json:"interpretation_signal_cap,omitempty"`
	MinActionabilityProbability float64 `json:"minimum_actionability_probability,omitempty"`
	MinActionabilityConfidence  float64 `json:"minimum_actionability_confidence,omitempty"`
	// Endpoint is the evaluator base URL. Defaults to the Fastino API host; tests
	// must override it with a local mock endpoint.
	Endpoint string `json:"endpoint,omitempty"`
	// RequestLimitBytes is the conservative serialized-request byte budget
	// (conservative GLiDE transport bound, not an exact token count).
	RequestLimitBytes int `json:"request_limit_bytes,omitempty"`
	// StateQuestionLimitBytes is the conservative state+longest-question
	// byte budget (conservative GLiDE state+question bound).
	StateQuestionLimitBytes int `json:"state_question_limit_bytes,omitempty"`
	// RequestBudget is the maximum number of HTTP requests per task run,
	// counted across candidate packets and retries.
	RequestBudget int `json:"request_budget,omitempty"`
	// HTTPTimeout is the per-request timeout. JSON config accepts a
	// unit-suffixed string ("30s"); a bare number is rejected so 30 cannot
	// silently become 30ns.
	HTTPTimeout time.Duration `json:"http_timeout,omitempty"`
	// MaxAttemptsPerRequest caps attempts (initial + retries) per request.
	MaxAttemptsPerRequest int `json:"max_attempts_per_request,omitempty"`
	// BackoffBase is the first backoff delay for retryable transport errors.
	// JSON config accepts a unit-suffixed string ("300ms").
	BackoffBase time.Duration `json:"backoff_base,omitempty"`
	// BackoffMax caps the exponential backoff delay. JSON config accepts a
	// unit-suffixed string ("4s").
	BackoffMax time.Duration `json:"backoff_max,omitempty"`
	// RetryAfterMaxSeconds caps an honored Retry-After seconds value.
	RetryAfterMaxSeconds int `json:"retry_after_max_seconds,omitempty"`
	// RedactPatterns are additional configured sensitive patterns (regular
	// expressions) beyond the built-in rails. Defaults cannot be removed.
	RedactPatterns []string `json:"redact_patterns,omitempty"`
	// DispatchServer is the OpenCode server base URL, not the evaluator endpoint.
	// Empty means the correcting-session capability is missing.
	DispatchServer string `json:"dispatch_server,omitempty"`
	// DispatchUITemplate is an http(s) URL template containing exactly one {id}.
	DispatchUITemplate string `json:"dispatch_ui_template,omitempty"`
	// DispatchModel is an optional host-owned model id sent to the server.
	// The JSON key is not "model"; that name stays unknown.
	DispatchModel string `json:"dispatch_model,omitempty"`
	// DispatchDirectory is the directory recorded on the correcting root.
	DispatchDirectory string `json:"dispatch_directory,omitempty"`
	// DispatchTimeout bounds one session create, read, list, or prompt call.
	DispatchTimeout time.Duration `json:"dispatch_timeout,omitempty"`
	// CanonicalRoots are absolute editable roots. There is no enrollment list.
	CanonicalRoots []string `json:"canonical_roots,omitempty"`
	// Reviewer is the coordinator-supplied independent reviewer identity.
	Reviewer string `json:"reviewer,omitempty"`
	// StateDir overrides the user state directory.
	StateDir string `json:"state_dir,omitempty"`
	// MaxRepairAttempts caps one initial run plus one repair per source version.
	MaxRepairAttempts int `json:"max_repair_attempts,omitempty"`
	// SpoolMaxBytes bounds each staged per-session observation spool.
	SpoolMaxBytes int `json:"spool_max_bytes,omitempty"`
	// DrainMaxJobs caps the jobs one drain worker pass may process.
	DrainMaxJobs int `json:"drain_max_jobs,omitempty"`
	// DispatchEndpoint selects the OpenCode session transport generation:
	// "prompt" (default, <=1.18.x) or "message" (the verified live 2.x
	// contract: /api prefix, Basic auth, data envelopes, sync prompt).
	// Empty means the default.
	DispatchEndpoint string `json:"dispatch_endpoint,omitempty"`
	// Evaluator selects the decision-engine provider profile: "glide"
	// (default; Fastino GLiDE, FASTINO_API_KEY). "jev" is reserved but rejected
	// until its current contract is verified. Models are pinned.
	Evaluator string `json:"evaluator,omitempty"`
	// DispatchServerUsername is the HTTP Basic username for the session
	// transport. Empty falls back to OPENCODE_SERVER_USERNAME, then the
	// documented server default "opencode".
	DispatchServerUsername string `json:"dispatch_server_username,omitempty"`
	// DispatchServerPassword is the HTTP Basic password for the session
	// transport. Empty falls back to the server process env
	// (OPENCODE_PASSWORD, then OPENCODE_SERVER_PASSWORD). It is only ever
	// sent as the Authorization header — never logged or persisted.
	DispatchServerPassword string `json:"dispatch_server_password,omitempty"`
}

// DefaultConfig returns the conservative defaults of the config contract.
// Spec: docs/specs/task-retrospective.md via Config
func DefaultConfig() *Config {
	return &Config{
		SignalConfig:                DefaultSignalConfig(),
		InterpretationSignalCap:     10,
		MinActionabilityProbability: .90,
		MinActionabilityConfidence:  .80,
		Evaluator:                   EvaluatorGlide,
		Endpoint:                    EvaluatorEndpoint(EvaluatorGlide),
		RequestLimitBytes:           24_000,
		StateQuestionLimitBytes:     16_000,
		RequestBudget:               30,
		HTTPTimeout:                 30 * time.Second,
		MaxAttemptsPerRequest:       3,
		BackoffBase:                 300 * time.Millisecond,
		BackoffMax:                  4 * time.Second,
		RetryAfterMaxSeconds:        30,
		RedactPatterns:              nil,
		DispatchTimeout:             30 * time.Second,
		MaxRepairAttempts:           2,
		SpoolMaxBytes:               2_000_000,
		DrainMaxJobs:                10,
		DispatchEndpoint:            EndpointPrompt,
	}
}

// Validate checks the config contract invariants; a pinned model and the
// default rails are not user-configurable.
// Spec: docs/specs/task-retrospective.md via Config
func (c *Config) Validate() error {
	if err := c.SignalConfig.validate(); err != nil {
		return err
	}
	if c.InterpretationSignalCap < 1 || c.InterpretationSignalCap > 10 {
		return fmt.Errorf("interpretation_signal_cap must be within [1,10]")
	}
	if !probabilityInRange(c.MinActionabilityProbability) || c.MinActionabilityProbability < .9 || !probabilityInRange(c.MinActionabilityConfidence) || c.MinActionabilityConfidence < .8 {
		return fmt.Errorf("semantic actionability thresholds may only be raised from .9/.8")
	}
	if c.Endpoint == "" {
		return fmt.Errorf("retrospective config: endpoint is required")
	}
	if c.Evaluator == "" {
		c.Evaluator = EvaluatorGlide
	}
	if EvaluatorModel(c.Evaluator) == "" {
		return fmt.Errorf("retrospective config: evaluator must be %q or %q", EvaluatorGlide, EvaluatorJev)
	}
	if c.Evaluator == EvaluatorJev {
		return fmt.Errorf("retrospective config: Jev is unavailable until its current provider contract is verified")
	}
	if err := validateHTTPBase(c.Endpoint); err != nil {
		return fmt.Errorf("retrospective config: endpoint: %w", err)
	}
	if c.RequestLimitBytes > 24_000 || c.StateQuestionLimitBytes > 16_000 {
		return fmt.Errorf("retrospective config: byte limits may only be lowered")
	}
	if c.RequestLimitBytes < 1 {
		return fmt.Errorf("retrospective config: request_limit_bytes must be positive")
	}
	if c.StateQuestionLimitBytes < 1 || c.StateQuestionLimitBytes > c.RequestLimitBytes {
		return fmt.Errorf("retrospective config: state_question_limit_bytes must be positive and no greater than request_limit_bytes")
	}
	if c.RequestBudget < 1 || c.RequestBudget > 30 {
		return fmt.Errorf("retrospective config: request_budget must be within [1,30]")
	}
	if c.HTTPTimeout <= 0 {
		return fmt.Errorf("retrospective config: http_timeout must be positive")
	}
	if c.MaxAttemptsPerRequest < 1 || c.MaxAttemptsPerRequest > 3 {
		return fmt.Errorf("retrospective config: max_attempts_per_request must be within [1,3]")
	}
	if c.BackoffBase <= 0 || c.BackoffMax < c.BackoffBase {
		return fmt.Errorf("retrospective config: backoff_max must be >= backoff_base > 0")
	}
	if c.RetryAfterMaxSeconds < 0 || c.RetryAfterMaxSeconds > 30 {
		return fmt.Errorf("retrospective config: retry_after_max_seconds must be within [0,30]")
	}
	for _, pattern := range c.RedactPatterns {
		if pattern == "" {
			return fmt.Errorf("retrospective config: redact_patterns entries must be non-empty")
		}
	}
	if c.MaxRepairAttempts < 1 {
		return fmt.Errorf("retrospective config: max_repair_attempts must be >= 1")
	}
	if c.DispatchTimeout <= 0 {
		return fmt.Errorf("retrospective config: dispatch_timeout must be positive")
	}
	if c.DispatchServer != "" {
		if err := validateHTTPBase(c.DispatchServer); err != nil {
			return fmt.Errorf("retrospective config: dispatch_server: %w", err)
		}
	}
	if c.DispatchUITemplate != "" {
		if err := ValidateUITemplate(c.DispatchUITemplate); err != nil {
			return fmt.Errorf("retrospective config: dispatch_ui_template: %w", err)
		}
	}
	if c.Reviewer == LearningCorrectorID {
		return fmt.Errorf("retrospective config: reviewer must not be the learning-corrector role")
	}
	for _, root := range c.CanonicalRoots {
		if err := checkCanonicalRoot(root); err != nil {
			return fmt.Errorf("retrospective config: %w", err)
		}
	}
	if c.SpoolMaxBytes < EnvelopeRecordMaxBytes {
		return fmt.Errorf("retrospective config: spool_max_bytes must be >= %d", EnvelopeRecordMaxBytes)
	}
	if c.DrainMaxJobs < 1 {
		return fmt.Errorf("retrospective config: drain_max_jobs must be >= 1")
	}
	if c.DispatchEndpoint != EndpointPrompt && c.DispatchEndpoint != EndpointMessage {
		return fmt.Errorf("retrospective config: dispatch_endpoint must be %q or %q", EndpointPrompt, EndpointMessage)
	}
	return nil
}

// configJSONKeys are the only keys a config file may set. Duration keys are
// decoded as unit-suffixed strings, never as bare nanosecond integers.
var configJSONKeys = map[string]struct{}{
	"endpoint": {}, "request_limit_bytes": {}, "state_question_limit_bytes": {},
	"request_budget": {}, "http_timeout": {}, "max_attempts_per_request": {},
	"backoff_base": {}, "backoff_max": {}, "retry_after_max_seconds": {}, "redact_patterns": {},
	"repeat_min_calls": {}, "repeat_window_calls": {}, "helper_min_calls": {}, "helper_window_calls": {}, "search_min_calls": {}, "search_window_calls": {}, "broad_search_min_calls": {}, "early_context_window_calls": {}, "early_context_search_min_calls": {}, "signal_cap": {}, "signal_evidence_cap": {}, "interpretation_signal_cap": {}, "context_neighbor_calls": {}, "evidence_limit_bytes": {}, "minimum_actionability_probability": {}, "minimum_actionability_confidence": {},
	"dispatch_server": {}, "dispatch_ui_template": {}, "dispatch_model": {},
	"dispatch_directory": {}, "dispatch_timeout": {}, "canonical_roots": {},
	"reviewer": {}, "state_dir": {}, "max_repair_attempts": {},
	"spool_max_bytes": {}, "drain_max_jobs": {}, "dispatch_endpoint": {},
	"dispatch_server_username": {}, "dispatch_server_password": {},
	"evaluator": {},
}

// UnmarshalJSON decodes a config document over the receiver so omitted keys
// keep their defaults. Duration fields must be unit-suffixed strings
// ("30s", "300ms"); a bare number is rejected.
// Spec: docs/specs/task-retrospective.md via Config
func (c *Config) UnmarshalJSON(data []byte) error {
	if !uniqueJSONKeys(data) {
		return fmt.Errorf("invalid or duplicate-key retrospective config JSON")
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var raw map[string]json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		return err
	}
	for key := range raw {
		if bytes.Equal(bytes.TrimSpace(raw[key]), []byte("null")) {
			return fmt.Errorf("retrospective config key %q cannot be null; omit it to retain defaults", key)
		}
		if containsString([]string{"min_success_probability", "min_confidence", "min_count_for_rate", "rate_threshold", "redundant_attempts_required", "tail_steps", "min_fragment_bytes"}, key) {
			return fmt.Errorf("retired retrospective config key %q: remove old success/rate/pagination settings; use signal thresholds and minimum_actionability_probability/confidence", key)
		}
		if _, ok := configJSONKeys[key]; !ok {
			return fmt.Errorf("json: unknown field %q", key)
		}
	}
	assign := func(key string, dst any) error {
		value, ok := raw[key]
		if !ok {
			return nil
		}
		if err := json.Unmarshal(value, dst); err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
		return nil
	}
	if err := assign("evaluator", &c.Evaluator); err != nil {
		return err
	}
	// The profile default endpoint follows the selected evaluator unless the
	// document sets an explicit endpoint.
	_, endpointSet := raw["endpoint"]
	if !endpointSet {
		c.Endpoint = EvaluatorEndpoint(c.Evaluator)
	}
	if err := assign("endpoint", &c.Endpoint); err != nil {
		return err
	}
	if err := assign("request_limit_bytes", &c.RequestLimitBytes); err != nil {
		return err
	}
	if err := assign("state_question_limit_bytes", &c.StateQuestionLimitBytes); err != nil {
		return err
	}
	if err := assign("request_budget", &c.RequestBudget); err != nil {
		return err
	}
	if err := assign("max_attempts_per_request", &c.MaxAttemptsPerRequest); err != nil {
		return err
	}
	if err := assign("retry_after_max_seconds", &c.RetryAfterMaxSeconds); err != nil {
		return err
	}
	if err := json.Unmarshal(data, &c.SignalConfig); err != nil {
		return err
	}
	if err := assign("interpretation_signal_cap", &c.InterpretationSignalCap); err != nil {
		return err
	}
	if err := assign("minimum_actionability_probability", &c.MinActionabilityProbability); err != nil {
		return err
	}
	if err := assign("minimum_actionability_confidence", &c.MinActionabilityConfidence); err != nil {
		return err
	}
	if err := assign("redact_patterns", &c.RedactPatterns); err != nil {
		return err
	}
	if err := assign("dispatch_server", &c.DispatchServer); err != nil {
		return err
	}
	if err := assign("dispatch_ui_template", &c.DispatchUITemplate); err != nil {
		return err
	}
	if err := assign("dispatch_model", &c.DispatchModel); err != nil {
		return err
	}
	if err := assign("dispatch_directory", &c.DispatchDirectory); err != nil {
		return err
	}
	if err := assign("dispatch_server_username", &c.DispatchServerUsername); err != nil {
		return err
	}
	if err := assign("dispatch_server_password", &c.DispatchServerPassword); err != nil {
		return err
	}
	if err := assign("canonical_roots", &c.CanonicalRoots); err != nil {
		return err
	}
	if err := assign("reviewer", &c.Reviewer); err != nil {
		return err
	}
	if err := assign("state_dir", &c.StateDir); err != nil {
		return err
	}
	if err := assign("max_repair_attempts", &c.MaxRepairAttempts); err != nil {
		return err
	}
	if err := assign("spool_max_bytes", &c.SpoolMaxBytes); err != nil {
		return err
	}
	if err := assign("drain_max_jobs", &c.DrainMaxJobs); err != nil {
		return err
	}
	if err := assign("dispatch_endpoint", &c.DispatchEndpoint); err != nil {
		return err
	}
	if value, ok := raw["http_timeout"]; ok {
		parsed, err := parseConfigDuration("http_timeout", value)
		if err != nil {
			return err
		}
		c.HTTPTimeout = parsed
	}
	if value, ok := raw["backoff_base"]; ok {
		parsed, err := parseConfigDuration("backoff_base", value)
		if err != nil {
			return err
		}
		c.BackoffBase = parsed
	}
	if value, ok := raw["backoff_max"]; ok {
		parsed, err := parseConfigDuration("backoff_max", value)
		if err != nil {
			return err
		}
		c.BackoffMax = parsed
	}
	if value, ok := raw["dispatch_timeout"]; ok {
		parsed, err := parseConfigDuration("dispatch_timeout", value)
		if err != nil {
			return err
		}
		c.DispatchTimeout = parsed
	}
	return nil
}

// parseConfigDuration accepts a JSON string such as "30s" and rejects bare
// numbers, which encoding/json would otherwise treat as nanoseconds.
// Spec: docs/specs/task-retrospective.md via Config
func parseConfigDuration(name string, raw json.RawMessage) (time.Duration, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '"' {
		return 0, fmt.Errorf("%s must be a unit-suffixed string such as \"30s\", not %s", name, raw)
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return 0, fmt.Errorf("%s: %w", name, err)
	}
	parsed, err := time.ParseDuration(text)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", name, err)
	}
	if parsed <= 0 {
		return 0, fmt.Errorf("%s must be positive", name)
	}
	return parsed, nil
}

// LoadConfig reads an optional JSON config file over DefaultConfig. Unknown
// or mis-typed keys fail closed; credentials are accepted only from the host
// environment, never from a config file. Duration fields are unit-suffixed
// strings.
// Spec: docs/specs/task-retrospective.md via Config
func LoadConfig(path string) (*Config, error) {
	cfg := DefaultConfig()
	if path == "" {
		return cfg, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read retrospective config: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(b))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(cfg); err != nil {
		return nil, fmt.Errorf("decode retrospective config %s: %w", path, err)
	}
	var trailing struct{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, fmt.Errorf("decode retrospective config %s: trailing data", path)
	}
	return cfg, cfg.Validate()
}
