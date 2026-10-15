// Spec: docs/specs/task-retrospective.md
package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// signalsNoNetwork makes any accidental provider request fail the public-path test.
// Spec: docs/specs/task-retrospective.md via runRetrospectiveAnalyze
type signalsNoNetwork struct{ t *testing.T }

// Spec: docs/specs/task-retrospective.md via signalsNoNetwork
func (s signalsNoNetwork) RoundTrip(*http.Request) (*http.Response, error) {
	s.t.Fatal("signals-only attempted network")
	return nil, nil
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
				if err := runRetrospective([]string{"analyze", "--signals-only", "--input", input, "--output", output}); err != nil {
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
