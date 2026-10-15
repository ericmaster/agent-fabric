// Spec: docs/specs/task-retrospective.md
package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ericmaster/agent-fabric/internal/retrospective"
)

// signalsNoNetwork makes any accidental provider request fail the public-path test.
// Spec: docs/specs/task-retrospective.md via runRetrospectiveAnalyze
type signalsNoNetwork struct{ t *testing.T }

// Spec: docs/specs/task-retrospective.md via signalsNoNetwork
func (s signalsNoNetwork) RoundTrip(*http.Request) (*http.Response, error) {
	s.t.Fatal("signals-only attempted network")
	return nil, nil
}

// TestFrozenSignalsPublicCLI replays the frozen positive AND negative inputs
// unchanged through the public runtime path. Missing context is never patched
// merely to make a fixture semantically eligible.
// Spec: docs/specs/task-retrospective.md via runRetrospectiveAnalyze
func TestFrozenSignalsPublicCLI(t *testing.T) {
	body, err := os.ReadFile("../../fixtures/retrospective/signals/frozen.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		Name          string                `json:"name"`
		Trace         json.RawMessage       `json:"trace"`
		Expected      map[string][][]string `json:"expected"`
		TerminalCalls int                   `json:"terminal_calls"`
		RawFailures   int                   `json:"raw_failures"`
	}
	if err = json.Unmarshal(body, &fixtures); err != nil {
		t.Fatal(err)
	}
	previous := http.DefaultTransport
	http.DefaultTransport = signalsNoNetwork{t}
	t.Cleanup(func() { http.DefaultTransport = previous })
	t.Setenv("FASTINO_API_KEY", "synthetic-unused-key")
	for _, f := range fixtures {
		t.Run(f.Name, func(t *testing.T) {
			dir := t.TempDir()
			input, output := filepath.Join(dir, "trace.json"), filepath.Join(dir, "report.json")
			if err := os.WriteFile(input, f.Trace, 0600); err != nil {
				t.Fatal(err)
			}
			if err := runRetrospective([]string{"analyze", "--input", input, "--output", output}); err != nil {
				t.Fatal(err)
			}
			r, _, err := retrospective.LoadReport(output)
			if err != nil {
				t.Fatal(err)
			}
			actual := map[string][][]string{}
			for _, s := range r.Signals {
				actual[s.RuleID] = append(actual[s.RuleID], s.CallIDs)
			}
			if !reflect.DeepEqual(actual, f.Expected) || r.Counts.TerminalCalls != f.TerminalCalls || r.Counts.RawFailures != f.RawFailures {
				t.Fatal("public runtime changed frozen observations", actual)
			}
			var trace retrospective.Trace
			if err := json.Unmarshal(f.Trace, &trace); err != nil {
				t.Fatal(err)
			}
			detected, err := retrospective.DetectSignals(&trace, f.Trace, nil)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(r.Coverage, detected.Coverage) || !reflect.DeepEqual(r.Abstentions, detected.Abstentions) || !reflect.DeepEqual(r.Counts, detected.Counts) {
				t.Fatal("CLI lost detector accounting")
			}
			if r.Dispatch.Eligible || r.RepairSafe || r.Usage.Requests != 0 || r.Coverage.EvidenceComplete || r.Routing.Candidates != (len(f.Expected) > 0) {
				t.Fatal("frozen missing context acquired authority")
			}
			t.Logf("fixture=%s rules=%v terminal=%d failures=%d abstentions=%v coverage=%v requests=%d repair_safe=%t", f.Name, actual, r.Counts.TerminalCalls, r.Counts.RawFailures, r.Abstentions, r.Coverage.Reasons, r.Usage.Requests, r.RepairSafe)
		})
	}
}

// TestSignalsOnlyPublicReplay is the counterfactual regression: the original
// analyzer cannot produce provider-independent signals on an aborted task.
// Spec: docs/specs/task-retrospective.md via runRetrospectiveAnalyze
func TestSignalsOnlyPublicReplay(t *testing.T) {
	previous := http.DefaultTransport
	http.DefaultTransport = signalsNoNetwork{t}
	t.Cleanup(func() { http.DefaultTransport = previous })
	for _, outcome := range []string{"aborted", "error", ""} {
		for _, key := range []string{"", "synthetic-unused-key"} {
			t.Run(outcome+key, func(t *testing.T) {
				t.Setenv("FASTINO_API_KEY", key)
				dir := t.TempDir()
				input := filepath.Join(dir, "trace.json")
				output := filepath.Join(dir, "signals.json")
				body := `{"schema_version":1,"identities":{"root_session_id":"s","session_id":"s","execution_root":"/project"},"terminal_status":"` + outcome + `","calls":[{"id":"red-test","seq":1,"tool":"bash","status":"success","exit_code":1,"input":"go test"}]}`
				if err := os.WriteFile(input, []byte(body), 0600); err != nil {
					t.Fatal(err)
				}
				if err := runRetrospective([]string{"analyze", "--input", input, "--output", output}); err != nil {
					t.Fatal(err)
				}
				data, err := os.ReadFile(output)
				if err != nil {
					t.Fatal(err)
				}
				var report struct {
					SchemaVersion   int `json:"schema_version"`
					TerminalOutcome struct {
						Status   string `json:"status"`
						Observed bool   `json:"observed"`
					} `json:"terminal_outcome"`
					Signals []struct {
						Rule  string   `json:"rule_id"`
						Calls []string `json:"call_ids"`
					} `json:"signals"`
					RepairSafe bool `json:"repair_safe"`
					Dispatch   struct {
						Eligible bool `json:"eligible"`
					} `json:"dispatch"`
				}
				if err := json.Unmarshal(data, &report); err != nil {
					t.Fatal(err)
				}
				if report.SchemaVersion != 2 || len(report.Signals) != 1 || report.Signals[0].Rule != "terminal-failure" || report.Signals[0].Calls[0] != "red-test" {
					t.Fatalf("missing observable signal: %s", data)
				}
				want := outcome
				if want == "" {
					want = "ok"
				}
				if report.TerminalOutcome.Status != want || report.TerminalOutcome.Observed != (outcome != "") || report.RepairSafe || report.Dispatch.Eligible {
					t.Fatalf("outcome/write authority changed: %s", data)
				}
				if strings.Contains(string(data), "synthetic-unused-key") {
					t.Fatal("provider key persisted")
				}
				info, _ := os.Stat(output)
				if info.Mode().Perm() != 0600 {
					t.Fatal("report not private")
				}
			})
		}
	}
}

// TestSignalsCLIConfigMigration exercises real flag/config ordering and output
// non-mutation, not just the internal config decoder.
// Spec: docs/specs/task-retrospective.md via runRetrospectiveAnalyze
func TestSignalsCLIConfigMigration(t *testing.T) {
	dir := t.TempDir()
	input := e2eTrace(t, dir)
	output := filepath.Join(dir, "report.json")
	config := filepath.Join(dir, "config.json")
	for _, body := range []string{`{"tail_steps":10}`, `{"rate_threshold":0.05}`, `{"min_success_probability":0.9}`, `{"signal_cap":101}`, `{"search_min_calls":11}`, `{"minimum_actionability_confidence":0.7}`, `{"model":"not-configurable"}`} {
		if err := os.WriteFile(config, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		if err := runRetrospective([]string{"analyze", "--input", input, "--output", output, "--config", config}); err == nil {
			t.Fatal("bad migration config accepted", body)
		}
		if _, err := os.Stat(output); !os.IsNotExist(err) {
			t.Fatal("invalid config wrote output")
		}
	}
	if err := runRetrospective([]string{"analyze", "--signals-only", "--input", input, "--output", output}); err == nil || !strings.Contains(err.Error(), "retired") {
		t.Fatal("old flag silently accepted", err)
	}
	if err := runRetrospective([]string{"analyze", "--signals-only=false", "--input", input, "--output", output}); err == nil || !strings.Contains(err.Error(), "retired") {
		t.Fatal("explicit false retired flag silently accepted", err)
	}
	if err := os.WriteFile(config, []byte(`{"repeat_min_calls":4,"interpretation_signal_cap":2,"minimum_actionability_probability":0.95}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := runRetrospective([]string{"analyze", "--input", input, "--output", output, "--config", config}); err != nil {
		t.Fatal("valid flat signal config rejected", err)
	}
	body, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	var r struct {
		Thresholds struct {
			Repeat int `json:"repeat_min_calls"`
		} `json:"thresholds"`
		Interpretation struct {
			Status string `json:"status"`
		} `json:"interpretation"`
	}
	if json.Unmarshal(body, &r) != nil || r.Thresholds.Repeat != 4 || r.Interpretation.Status != "skipped_no_candidates" {
		t.Fatal("new CLI keys/default routing drift")
	}
}
