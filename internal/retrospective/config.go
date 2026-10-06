// Spec: docs/specs/task-retrospective.md
package retrospective

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// JevModel is the pinned Jev model identifier for retrospective choices.
const JevModel = "jev-1.13.0"

// RubricVersion identifies the deterministic counting rubric of the report.
const RubricVersion = "task-retrospective-rubric-1"

// Config is the single contract for retrospective defaults and thresholds.
// Later phases (dispatch, OpenCode decoding) read the same struct; adapters
// must not duplicate the values.
type Config struct {
	// Endpoint is the Jev base URL. Defaults to the TypeSafe API host; tests
	// must override it with a local mock endpoint.
	Endpoint string `json:"endpoint,omitempty"`
	// RequestLimitBytes is the conservative serialized-request byte budget
	// (approximation of the Jev all-input limit).
	RequestLimitBytes int `json:"request_limit_bytes,omitempty"`
	// StateQuestionLimitBytes is the conservative state+longest-question
	// byte budget (approximation of the Jev state+longest-question limit).
	StateQuestionLimitBytes int `json:"state_question_limit_bytes,omitempty"`
	// RequestBudget is the maximum number of HTTP requests per task run,
	// counted across both stages, retries, and size-rejection splits.
	RequestBudget int `json:"request_budget,omitempty"`
	// HTTPTimeout is the per-request timeout.
	HTTPTimeout time.Duration `json:"http_timeout,omitempty"`
	// MaxAttemptsPerRequest caps attempts (initial + retries) per request.
	MaxAttemptsPerRequest int `json:"max_attempts_per_request,omitempty"`
	// MinFragmentBytes is the smallest permitted split fragment size.
	MinFragmentBytes int `json:"min_fragment_bytes,omitempty"`
	// BackoffBase is the first backoff delay for retryable transport errors.
	BackoffBase time.Duration `json:"backoff_base,omitempty"`
	// BackoffMax caps the exponential backoff delay.
	BackoffMax time.Duration `json:"backoff_max,omitempty"`
	// RetryAfterMaxSeconds caps an honored Retry-After seconds value.
	RetryAfterMaxSeconds int `json:"retry_after_max_seconds,omitempty"`
	// MinSuccessProbability is the confident success/attribution probability floor.
	MinSuccessProbability float64 `json:"min_success_probability,omitempty"`
	// MinConfidence is the confident judgment floor.
	MinConfidence float64 `json:"min_confidence,omitempty"`
	// MinCountForRate is the minimum terminal-call count for the rate trigger.
	MinCountForRate int `json:"min_count_for_rate,omitempty"`
	// RateThreshold is the exclusive avoidable-rate threshold.
	RateThreshold float64 `json:"rate_threshold,omitempty"`
	// RedundantAttemptsRequired is the verified-redundant-attempts exception count.
	RedundantAttemptsRequired int `json:"redundant_attempts_required,omitempty"`
	// TailSteps is the stage-one last-N observable steps size.
	TailSteps int `json:"tail_steps,omitempty"`
	// RedactPatterns are additional configured sensitive patterns (regular
	// expressions) beyond the built-in rails. Defaults cannot be removed.
	RedactPatterns []string `json:"redact_patterns,omitempty"`
}

// DefaultConfig returns the conservative defaults of the config contract.
// Spec: docs/specs/task-retrospective.md via Config
func DefaultConfig() *Config {
	return &Config{
		Endpoint:                  "https://api.typesafe.ai",
		RequestLimitBytes:         24_000,
		StateQuestionLimitBytes:   16_000,
		RequestBudget:             100,
		HTTPTimeout:               30 * time.Second,
		MaxAttemptsPerRequest:     3,
		MinFragmentBytes:          1024,
		BackoffBase:               300 * time.Millisecond,
		BackoffMax:                4 * time.Second,
		RetryAfterMaxSeconds:      30,
		MinSuccessProbability:     0.90,
		MinConfidence:             0.80,
		MinCountForRate:           20,
		RateThreshold:             0.05,
		RedundantAttemptsRequired: 3,
		TailSteps:                 10,
		RedactPatterns:            nil,
	}
}

// Validate checks the config contract invariants; a pinned model and the
// default rails are not user-configurable.
// Spec: docs/specs/task-retrospective.md via Config
func (c *Config) Validate() error {
	if c.Endpoint == "" {
		return fmt.Errorf("retrospective config: endpoint is required")
	}
	if c.RequestLimitBytes < c.MinFragmentBytes {
		return fmt.Errorf("retrospective config: request_limit_bytes must be >= min_fragment_bytes (%d)", c.MinFragmentBytes)
	}
	if c.StateQuestionLimitBytes < c.MinFragmentBytes || c.StateQuestionLimitBytes > c.RequestLimitBytes {
		return fmt.Errorf("retrospective config: state_question_limit_bytes must be within [min_fragment_bytes, request_limit_bytes]")
	}
	if c.RequestBudget < 1 {
		return fmt.Errorf("retrospective config: request_budget must be >= 1")
	}
	if c.HTTPTimeout <= 0 {
		return fmt.Errorf("retrospective config: http_timeout must be positive")
	}
	if c.MaxAttemptsPerRequest < 1 {
		return fmt.Errorf("retrospective config: max_attempts_per_request must be >= 1")
	}
	if c.MinFragmentBytes < 1 {
		return fmt.Errorf("retrospective config: min_fragment_bytes must be >= 1")
	}
	if c.BackoffBase <= 0 || c.BackoffMax < c.BackoffBase {
		return fmt.Errorf("retrospective config: backoff_max must be >= backoff_base > 0")
	}
	if c.RetryAfterMaxSeconds < 0 {
		return fmt.Errorf("retrospective config: retry_after_max_seconds must be >= 0")
	}
	if c.MinSuccessProbability < 0 || c.MinSuccessProbability > 1 {
		return fmt.Errorf("retrospective config: min_success_probability must be within [0,1]")
	}
	if c.MinConfidence < 0 || c.MinConfidence > 1 {
		return fmt.Errorf("retrospective config: min_confidence must be within [0,1]")
	}
	if c.MinCountForRate < 0 {
		return fmt.Errorf("retrospective config: min_count_for_rate must be >= 0")
	}
	if c.RateThreshold <= 0 || c.RateThreshold >= 1 {
		return fmt.Errorf("retrospective config: rate_threshold must be within (0,1)")
	}
	if c.RedundantAttemptsRequired < 0 {
		return fmt.Errorf("retrospective config: redundant_attempts_required must be >= 0")
	}
	if c.TailSteps < 0 {
		return fmt.Errorf("retrospective config: tail_steps must be >= 0")
	}
	for _, pattern := range c.RedactPatterns {
		if pattern == "" {
			return fmt.Errorf("retrospective config: redact_patterns entries must be non-empty")
		}
	}
	return nil
}

// LoadConfig reads an optional JSON config file over DefaultConfig. Unknown
// or mis-typed keys fail closed; credentials are accepted only from the host
// environment, never from a config file.
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
	decoder := json.NewDecoder(strings.NewReader(string(b)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(cfg); err != nil {
		return nil, fmt.Errorf("decode retrospective config %s: %w", path, err)
	}
	return cfg, cfg.Validate()
}
