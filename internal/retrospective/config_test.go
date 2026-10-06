// Spec: docs/specs/task-retrospective.md
package retrospective

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfigPassesValidation(t *testing.T) {
	cfg := DefaultConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	if cfg.RequestLimitBytes != 24_000 || cfg.StateQuestionLimitBytes != 16_000 {
		t.Fatalf("conservative byte budgets wrong: %+v", cfg)
	}
	if cfg.RequestBudget != 100 || cfg.MaxAttemptsPerRequest != 3 {
		t.Fatalf("budget/attempt defaults wrong: %+v", cfg)
	}
	if cfg.MinCountForRate != 20 || cfg.RateThreshold != 0.05 {
		t.Fatalf("threshold defaults wrong: %+v", cfg)
	}
	if cfg.MinSuccessProbability != 0.90 || cfg.MinConfidence != 0.80 {
		t.Fatalf("confidence floors wrong: %+v", cfg)
	}
	if cfg.TailSteps != 10 {
		t.Fatalf("tail steps = %d", cfg.TailSteps)
	}
}

func TestValidateRejectsBrokenConfigs(t *testing.T) {
	cases := map[string]func(*Config){
		"missing endpoint":       func(c *Config) { c.Endpoint = "" },
		"request below minimum":  func(c *Config) { c.RequestLimitBytes = 512 },
		"state above request":    func(c *Config) { c.StateQuestionLimitBytes = 30_000 },
		"zero budget":            func(c *Config) { c.RequestBudget = 0 },
		"invalid rate threshold": func(c *Config) { c.RateThreshold = 1.5 },
		"invalid probability":    func(c *Config) { c.MinSuccessProbability = -1 },
		"timeout missing":        func(c *Config) { c.HTTPTimeout = 0 },
		"empty redact pattern":   func(c *Config) { c.RedactPatterns = []string{""} },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := DefaultConfig()
			mutate(cfg)
			if err := cfg.Validate(); err == nil {
				t.Fatalf("%s was accepted", name)
			}
		})
	}
}

func TestLoadConfigRejectsUnknownAndInvalidKeys(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte(`{"endpoint":"https://x","model":"other-model"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(bad); err == nil {
		t.Fatal("unknown key was accepted")
	}
	invalid := filepath.Join(dir, "invalid.json")
	if err := os.WriteFile(invalid, []byte(`{"request_budget":"ten"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(invalid); err == nil {
		t.Fatal("mis-typed value was accepted")
	}
	good := filepath.Join(dir, "good.json")
	if err := os.WriteFile(good, []byte(`{"endpoint":"https://x","redact_patterns":["CANARY-SECRET-[A-Za-z0-9-]+"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(good)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Endpoint != "https://x" || len(cfg.RedactPatterns) != 1 {
		t.Fatalf("config not loaded: %+v", cfg)
	}
	// Pinned model stays untouched by config.
	if JevModel != "jev-1.13.0" {
		t.Fatalf("model pin moved: %q", JevModel)
	}
}

func TestLoadConfigDurationRequiresUnitSuffix(t *testing.T) {
	dir := t.TempDir()
	bare := filepath.Join(dir, "bare.json")
	if err := os.WriteFile(bare, []byte(`{"http_timeout":30}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(bare); err == nil {
		t.Fatal("bare http_timeout number was accepted as nanoseconds")
	}
	good := filepath.Join(dir, "good.json")
	if err := os.WriteFile(good, []byte(`{"http_timeout":"30s","backoff_base":"300ms","backoff_max":"4s"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(good)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPTimeout.String() != "30s" || cfg.BackoffBase.String() != "300ms" || cfg.BackoffMax.String() != "4s" {
		t.Fatalf("durations = %s %s %s", cfg.HTTPTimeout, cfg.BackoffBase, cfg.BackoffMax)
	}
	if cfg.RequestBudget != 100 {
		t.Fatalf("omitted budget was not preserved: %d", cfg.RequestBudget)
	}
}

func TestLoadConfigRejectsTrailingData(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "trailing.json")
	if err := os.WriteFile(path, []byte(`{"endpoint":"https://x"}{"endpoint":"https://y"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(path); err == nil {
		t.Fatal("trailing config object was accepted")
	}
}

func TestLoadConfigMissingFileFailsClosed(t *testing.T) {
	if _, err := LoadConfig(filepath.Join(t.TempDir(), "absent.json")); err == nil {
		t.Fatal("missing config file was accepted")
	}
	if _, err := LoadConfig(""); err != nil {
		t.Fatalf("no-config default failed: %v", err)
	}
}
