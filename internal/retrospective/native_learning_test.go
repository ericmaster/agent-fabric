// Spec: docs/specs/task-retrospective.md
package retrospective

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeDispatchRequiresExecutedIndependentStages(t *testing.T) {
	for _, fault := range []string{"", "wrong-role", "review-reject", "qa-reject"} {
		t.Run(fault, func(t *testing.T) {
			root := t.TempDir()
			target := filepath.Join(root, "docs", "runbooks", "native.md")
			original := "Use wrong/docs.\n"
			hash := writeText(t, target, original)
			finding := Finding{ID: "native-direction", SourcePath: target, SourceSHA256: hash, InstructionVersion: hash, Cause: CauseStale, FailedBehavior: "wrong/docs attempts", Recovery: "used project docs", Excerpt: "wrong/docs", Replacement: "project/docs"}
			report := eligibleReport(root, "native-direction", nil)
			report.Trace.SourceFiles = []ReportSourceFile{{Path: target, SHA256: hash}}
			report.TaskEvidence = []evidenceRecord{{ID: "source-recovery", Text: "Read canonical instruction; wrong/docs failed; project/docs succeeded"}}
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
