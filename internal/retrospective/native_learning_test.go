// Spec: docs/specs/task-retrospective.md
package retrospective

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNativeDispatchRequiresExecutedIndependentStages(t *testing.T) {
	for _, fault := range []string{"", "wrong-role", "review-reject", "qa-reject"} {
		t.Run(fault, func(t *testing.T) {
			root := t.TempDir()
			target := filepath.Join(root, "docs", "runbooks", "native.md")
			original := "Use wrong/docs.\n"
			hash := writeText(t, target, original)
			finding := Finding{ID: "native-direction", SourcePath: target, SourceSHA256: hash, InstructionVersion: hash, Cause: CauseStale, FailedBehavior: "wrong/docs attempts", Recovery: "used project docs", Excerpt: "wrong/docs", Replacement: "project/docs"}
			report := eligibleReport(root, "native-direction", []Finding{finding})
			report.Findings = nil
			report.RepairSafe = false
			report.Trace.SourceFiles = []ReportSourceFile{{Path: target, SHA256: hash}}
			// Actual detector/interpretation evidence is supplied by eligibleReport.
			reportPath := writeReport(t, t.TempDir(), report)
			serverState := &loopSessionServer{directory: root, proposal: NativeProposal{Summary: "causal investigation", Findings: []Finding{finding}}, fault: fault}
			server := httptest.NewServer(serverState.handler())
			defer server.Close()
			cfg := testConfig(root, t.TempDir(), server.URL)
			cfg.DispatchEndpoint = EndpointMessage
			cfg.DispatchModel = "fixture/worker"
			cfg.DispatchServerPassword = "synthetic-native-key"
			cfg.Reviewer = "code-reviewer"
			options := dispatchOpts(cfg, reportPath, OpenSessionClient(cfg))
			receipt, err := Dispatch(context.Background(), options)
			if err != nil {
				t.Fatal(err)
			}
			after, _ := os.ReadFile(target)
			switch fault {
			case "":
				if receipt.Status != StatusRepaired || !receipt.NativeCompleted || receipt.ReviewerRootID == "" || receipt.QARootID == "" || string(after) != "Use project/docs.\n" {
					t.Fatalf("native flow failed: %+v", receipt)
				}
			case "qa-reject":
				if receipt.Status != StatusPending || string(after) != original || len(receipt.Findings) != 1 || !receipt.Findings[0].Restored {
					t.Fatalf("QA did not roll back: %+v", receipt)
				}
			default:
				if receipt.Status != StatusPending || string(after) != original || receipt.NativeCompleted {
					t.Fatalf("unexecuted/rejected review authorized repair: %+v", receipt)
				}
			}
			creates, prompts, _ := serverState.snapshot()
			if fault == "" {
				// Simulate the durable postimage journal after QA answered but
				// before its local acceptance write. Resume must not reapply.
				store, err := OpenStore(cfg.StateDir)
				if err != nil {
					t.Fatal(err)
				}
				receipt.NativeCompleted = false
				receipt.Status = StatusPending
				receipt.Findings[0].Action = StatusPending
				if err := store.WriteReceipt(receipt); err != nil {
					t.Fatal(err)
				}
			}
			_, err = Dispatch(context.Background(), options)
			if err != nil {
				t.Fatal(err)
			}
			creates2, prompts2, _ := serverState.snapshot()
			if len(creates) != len(creates2) || len(prompts) != len(prompts2) {
				t.Fatal("resume duplicated native stage")
			}
		})
	}
}

// The child dies inside the real Dispatch -> applyOne -> source-write sink.
// Spec: docs/specs/task-retrospective.md via TestNativeRepairWriteAheadCrashWindows
func TestNativeRepairCrashChild(t *testing.T) {
	configPath := os.Getenv("AGF_TEST_CRASH_CONFIG")
	if configPath == "" {
		return
	}
	body, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	// Preserve the in-process normalized fixture config (duration integers),
	// rather than round-tripping through the CLI's unit-suffixed JSON decoder.
	type fixtureConfig Config
	var decoded fixtureConfig
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
	cfg := Config(decoded)
	opts := dispatchOpts(&cfg, os.Getenv("AGF_TEST_CRASH_REPORT"), OpenSessionClient(&cfg))
	opts.repairCheckpoint = func(window string) {
		if window == os.Getenv("AGF_TEST_CRASH_WINDOW") {
			os.Exit(73)
		}
	}
	if _, err := Dispatch(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	t.Fatal("fault checkpoint not reached")
}

func TestNativeRepairWriteAheadCrashWindows(t *testing.T) {
	for _, window := range []string{"before-write", "after-write", "before-validator", "after-validator"} {
		for _, control := range []string{"resume", "qa-reject", "unrelated-edit", "corrupt-preimage", "mode-change"} {
			t.Run(window+"/"+control, func(t *testing.T) {
				root := t.TempDir()
				target := filepath.Join(root, "docs", "runbooks", "crash.md")
				original := "Use wrong/docs.\n"
				postimage := "Use project/docs.\n"
				hash := writeText(t, target, original)
				finding := Finding{ID: "crash-direction", SourcePath: target, SourceSHA256: hash, InstructionVersion: hash, Cause: CauseStale, FailedBehavior: "wrong/docs attempts", Recovery: "used project docs", Excerpt: "wrong/docs", Replacement: "project/docs"}
				report := eligibleReport(root, "crash-direction", []Finding{finding})
				report.Findings = nil
				report.RepairSafe = false
				report.Trace.SourceFiles = []ReportSourceFile{{Path: target, SHA256: hash}}
				// Preserve the exact signal/call evidence rather than a fabricated summary.
				reportPath := writeReport(t, t.TempDir(), report)
				serverState := &loopSessionServer{directory: root, proposal: NativeProposal{Summary: "causal investigation", Findings: []Finding{finding}}}
				if control == "qa-reject" {
					serverState.fault = "qa-reject"
				}
				server := httptest.NewServer(serverState.handler())
				defer server.Close()
				cfg := testConfig(root, t.TempDir(), server.URL)
				cfg.DispatchEndpoint = EndpointMessage
				cfg.DispatchModel = "fixture/worker"
				cfg.DispatchServerPassword = "synthetic-native-key"
				cfg.Reviewer = "code-reviewer"
				configPath := filepath.Join(t.TempDir(), "config.json")
				encoded, _ := json.Marshal(cfg)
				if err := os.WriteFile(configPath, encoded, 0600); err != nil {
					t.Fatal(err)
				}
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestNativeRepairCrashChild$")
				cmd.Env = []string{"AGF_TEST_CRASH_CONFIG=" + configPath, "AGF_TEST_CRASH_REPORT=" + reportPath, "AGF_TEST_CRASH_WINDOW=" + window}
				output, err := cmd.CombinedOutput()
				var exit *exec.ExitError
				if !errors.As(err, &exit) || exit.ExitCode() != 73 {
					t.Fatalf("child did not die at sink: %v %s", err, output)
				}
				store, err := OpenStore(cfg.StateDir)
				if err != nil {
					t.Fatal(err)
				}
				receipts, err := store.ListReceipts()
				if err != nil || len(receipts) != 1 {
					t.Fatalf("receipt: %v %v", receipts, err)
				}
				receipt := receipts[0]
				if len(receipt.Findings) != 1 || receipt.Findings[0].Action != StatusPending {
					t.Fatalf("no write-ahead pending finding at %s: %+v", window, receipt)
				}
				result := receipt.Findings[0]
				proposalJSON, _ := json.Marshal(serverState.proposal)
				if result.CandidateSHA256 != sha256hex(proposalJSON) || result.ReviewSHA256 != sha256hex([]byte(receipt.ReviewVerdict)) || result.RepairAttempts != 1 || result.FileMode != 0644 {
					t.Fatalf("write-ahead reviewer/mode/budget binding absent: %+v", result)
				}
				key := findingKey(finding.ID, finding.SourcePath, finding.SourceSHA256)
				reservation, err := store.ReadFindingRecord(key)
				if err != nil || reservation.Attempts != 1 || reservation.LastAction != StatusPending {
					t.Fatalf("no durable attempt reservation: %+v %v", reservation, err)
				}
				if result.PreimageSHA256 != hash || result.PostimageSHA256 != sha256hex([]byte(postimage)) || result.CanonicalPath != target {
					t.Fatalf("journal bytes/identity absent: %+v", result)
				}
				preimage, err := store.ReadPreimage(receipt.IncidentID, finding.ID)
				if err != nil || string(preimage) != original {
					t.Fatalf("preimage: %s %v", preimage, err)
				}
				current, _ := os.ReadFile(target)
				expected := postimage
				if window == "before-write" {
					expected = original
				}
				if string(current) != expected {
					t.Fatalf("wrong crash bytes: %s", current)
				}
				if control == "unrelated-edit" {
					if err := os.WriteFile(target, []byte("unrelated user edit\n"), 0644); err != nil {
						t.Fatal(err)
					}
				}
				if control == "corrupt-preimage" {
					if err := store.WritePreimage(receipt.IncidentID, finding.ID, []byte("tampered preimage\n")); err != nil {
						t.Fatal(err)
					}
				}
				if control == "mode-change" {
					if err := os.Chmod(target, 0600); err != nil {
						t.Fatal(err)
					}
				}
				opts := dispatchOpts(cfg, reportPath, OpenSessionClient(cfg))
				resumed, err := Dispatch(context.Background(), opts)
				if err != nil {
					t.Fatal(err)
				}
				current, _ = os.ReadFile(target)
				switch control {
				case "resume":
					if resumed.Status != StatusRepaired || !resumed.NativeCompleted || string(current) != postimage {
						t.Fatalf("resume failed: %+v %s", resumed, current)
					}
				case "qa-reject":
					if string(current) != original || resumed.NativeCompleted || resumed.Status != StatusPending {
						t.Fatalf("failed QA not restored: %+v %s", resumed, current)
					}
				case "unrelated-edit":
					if string(current) != "unrelated user edit\n" || resumed.NativeCompleted || resumed.Findings[0].Action != StatusConflict {
						t.Fatalf("user edit overwritten: %+v %s", resumed, current)
					}
				case "corrupt-preimage", "mode-change":
					if string(current) != expected || resumed.NativeCompleted || resumed.Findings[0].Action != StatusConflict {
						t.Fatalf("ambiguous journal/source overwritten: %+v %s", resumed, current)
					}
					if control == "mode-change" {
						info, err := os.Stat(target)
						if err != nil || info.Mode().Perm() != 0600 {
							t.Fatal("user mode change overwritten")
						}
					}
				}
				creates, prompts, _ := serverState.snapshot()
				count := 3
				if control != "resume" && control != "qa-reject" && window != "after-validator" {
					count = 2
				}
				if len(creates) != count || len(prompts) != count {
					t.Fatalf("roots/prompts not stable: %v %v", creates, prompts)
				}
				if _, err := Dispatch(context.Background(), opts); err != nil {
					t.Fatal(err)
				}
				creates2, prompts2, _ := serverState.snapshot()
				if len(creates) != len(creates2) || len(prompts) != len(prompts2) {
					t.Fatal("duplicate worker root/prompt on repeated resume")
				}
				reservation, err = store.ReadFindingRecord(key)
				if err != nil || reservation.Attempts != 1 {
					t.Fatalf("resume consumed another repair attempt: %+v %v", reservation, err)
				}
			})
		}
	}
}

func TestRepairValidationNeverRestoresOverConcurrentEdit(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "docs", "runbooks", "native.md")
	original := "old instruction\n"
	hash := writeText(t, path, original)
	finding := Finding{ID: "concurrent", SourceSHA256: hash, Excerpt: "old instruction", Replacement: "new instruction"}
	result := applyExcerpt(testStore(t), "synthetic-incident", finding, path, func(path string, _ []byte) error {
		if err := os.WriteFile(path, []byte("unrelated user edit\n"), 0600); err != nil {
			t.Fatal(err)
		}
		return errors.New("QA rejected")
	})
	after, _ := os.ReadFile(path)
	if result.Restored || string(after) != "unrelated user edit\n" || result.Action != StatusConflict {
		t.Fatalf("concurrent user work overwritten: %+v %s", result, after)
	}
}

func TestNativeResultDecodingAbstainsOnExtraOrTrailingData(t *testing.T) {
	for _, input := range []string{`{"findings":[],"summary":"done","approved":true}`, `{"findings":[],"summary":"done"} {}`, `not a result`} {
		var proposal NativeProposal
		if nativeDecode(input, &proposal) == nil {
			t.Fatal("unsafe native result accepted")
		}
	}
	for _, input := range []string{"", "implicit", "provider/model#variant", "provider/model?secret=CANARY"} {
		if _, err := nativeModel(input); err == nil || strings.Contains(err.Error(), "CANARY") {
			t.Fatal("unbounded/unsafe model accepted")
		}
	}
	encoded, _ := json.Marshal(NativeProposal{Summary: "advisory", Findings: []Finding{}})
	var proposal NativeProposal
	if nativeDecode(string(encoded), &proposal) != nil {
		t.Fatal("valid native advisory rejected")
	}
}

func TestNativeCreationPermissionsAndOpaqueModelReference(t *testing.T) {
	root := t.TempDir()
	protected := filepath.Join(root, ".config", "opencode", "policy.json")
	writeText(t, protected, "unrelated protected config\n")
	var captured struct {
		Model       map[string]string   `json:"model"`
		Permissions []map[string]string `json:"permissions"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/session" {
			t.Error("unexpected endpoint/permission approval")
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Error(err)
		}
		json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": "ses_permissions", "title": "fixture", "location": map[string]string{"directory": root}}})
	}))
	defer server.Close()
	cfg := testConfig(root, t.TempDir(), server.URL)
	cfg.DispatchEndpoint = EndpointMessage
	cfg.DispatchModel = "fixture/" + protected
	if _, err := OpenSessionClient(cfg).CreateRoot(context.Background(), CreateRootRequest{Title: "fixture", Directory: root}); err != nil {
		t.Fatal(err)
	}
	if len(captured.Model) != 2 || captured.Model["providerID"] != "fixture" || captured.Model["id"] != protected {
		t.Fatalf("model reference interpreted as a file: %v", captured.Model)
	}
	if body, _ := os.ReadFile(protected); string(body) != "unrelated protected config\n" {
		t.Fatal("protected model-looking file mutated")
	}
	if len(captured.Permissions) == 0 || captured.Permissions[0]["action"] != "*" || captured.Permissions[0]["resource"] != "*" || captured.Permissions[0]["effect"] != "deny" {
		t.Fatal("default-deny missing")
	}
	readOnly := map[string]bool{"read": true, "glob": true, "grep": true, "external_directory": true}
	for _, rule := range captured.Permissions {
		if rule["effect"] == "allow" && !readOnly[rule["action"]] {
			t.Fatalf("write/execute/approval channel enabled: %v", rule)
		}
	}
}

func TestPreimageJournalIdentifiersCannotEscapeOwnedState(t *testing.T) {
	store := testStore(t)
	for _, id := range []string{"../escape", "/absolute", "..", `..\escape`, "C:escape"} {
		if err := store.WritePreimage("incident", id, []byte("preimage")); err == nil {
			t.Fatalf("unsafe finding locator accepted: %q", id)
		}
		if err := store.WritePreimage(id, "finding", []byte("preimage")); err == nil {
			t.Fatalf("unsafe incident locator accepted: %q", id)
		}
	}
}
