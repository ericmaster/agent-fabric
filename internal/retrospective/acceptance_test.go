// Spec: docs/specs/task-retrospective.md
package retrospective

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// catalogTask repeats the original fixture task using ONLY observed file reads.
// It does not edit, run commands or infer a replacement. A bad instruction fails
// an actual data lookup, not an assertion that a string was replaced.
// Spec: docs/specs/task-retrospective.md via TestControlledNativeCatalogRepair
func catalogTask(root, instruction, sku string, want int) (string, error) {
	body, err := os.ReadFile(instruction)
	if err != nil {
		return "", err
	}
	var catalog string
	for _, line := range strings.Split(string(body), "\n") {
		if strings.HasPrefix(line, "Catalog: ") {
			catalog = strings.TrimPrefix(line, "Catalog: ")
		}
	}
	if catalog == "" || filepath.IsAbs(catalog) || strings.Contains(catalog, "..") {
		return "", fmt.Errorf("invalid catalog locator")
	}
	body, err = os.ReadFile(filepath.Join(root, catalog))
	if err != nil {
		return "", err
	}
	var prices map[string]int
	if err = json.Unmarshal(body, &prices); err != nil {
		return "", err
	}
	price, ok := prices[sku]
	if !ok {
		return "", fmt.Errorf("SKU %s absent", sku)
	}
	if price != want {
		return "", fmt.Errorf("SKU %s price %d != %d", sku, price, want)
	}
	return fmt.Sprintf("%s=%d", sku, price), nil
}

// TestControlledNativeCatalogRepair uses the real Analyze -> report -> Dispatch
// -> native role protocol -> Go WAL/apply -> QA path. All model/role replies are
// explicitly synthetic; no live independent reviewer claim is made here.
// Spec: docs/specs/task-retrospective.md
func TestControlledNativeCatalogRepair(t *testing.T) {
	for _, control := range []string{"repair", "review-reject", "wrong-review-digest", "qa-reject", "dirty-source", "dirty-after-review", "no-provenance", "unknown-cause", "incomplete-evidence", "protected-target"} {
		t.Run(control, func(t *testing.T) {
			root := t.TempDir()
			target := filepath.Join(root, "docs", "catalog.md")
			original := "# Catalog lookup\nCatalog: archive/catalog.json\nNever invent missing SKUs.\n"
			post := strings.Replace(original, "archive/catalog.json", "data/catalog.json", 1)
			hash := writeText(t, target, original)
			dataPath := filepath.Join(root, "data", "catalog.json")
			dataHash := writeText(t, dataPath, `{"widget":1250,"spare":250}`)
			neighbor := filepath.Join(root, "docs", "unrelated.md")
			neighborHash := writeText(t, neighbor, "Unrelated user-owned instruction.\n")
			before, beforeErr := catalogTask(root, target, "widget", 1250)
			if beforeErr == nil || !os.IsNotExist(beforeErr) || before != "" {
				t.Fatal("preimage did not fail specifically at stale catalog", beforeErr)
			}
			recovered, err := os.ReadFile(dataPath)
			if err != nil || string(recovered) != `{"widget":1250,"spare":250}` {
				t.Fatal("fixture data is not healthy", err)
			}
			trace := signalTrace([]CallRecord{
				{ID: "instruction-read", Seq: 1, Tool: "read", Status: StatusSuccess, Input: `{"filePath":` + fmt.Sprintf("%q", target) + `}`, Result: []Fragment{{ID: "instruction-body", Text: original}}, SourceRefs: []SourceRef{{Path: target, SHA256: hash}}},
				{ID: "stale-lookup", Seq: 2, Tool: "read", Status: StatusError, Input: fmt.Sprintf(`{"filePath":%q}`, filepath.Join(root, "archive", "catalog.json")), Result: []Fragment{{ID: "lookup-error", Text: beforeErr.Error()}}},
				{ID: "catalog-recovery", Seq: 3, Tool: "read", Status: StatusSuccess, Input: fmt.Sprintf(`{"filePath":%q}`, dataPath), Result: []Fragment{{ID: "catalog-data", Text: string(recovered)}}},
			})
			trace.Identities.ExecutionRoot = root
			trace.Identities.TaskID = "catalog-fixture"
			trace.TerminalStatus = TaskStatusError
			trace.Messages[0].Text = "Look up widget at 1250 cents using docs/catalog.md; unknown SKU and wrong-price checks must reject. Do not modify original task result."
			trace.Messages[1].Text = "Lookup failed using the documented path. Direct observed recovery read found the valid catalog at data/catalog.json."
			trace.SourceFiles = []SourceRef{{Path: target, SHA256: hash}}
			if control == "no-provenance" {
				trace.SourceFiles = nil
				trace.Calls[0].SourceRefs = nil
			}
			if control == "incomplete-evidence" {
				trace.Gaps = []Gap{{Kind: "partial_stream", Detail: "explicit missing fixture context"}}
			}
			// The existing seam is test-only. No report flags/findings are patched.
			report := analyze(t, DefaultConfig(), trace, &fixtureSignalClient{choice: "actionable"})
			if report.RepairSafe || len(report.Findings) != 0 || report.Dispatch.Eligible != (control != "incomplete-evidence") || len(report.Signals) != 1 {
				t.Fatal("Analyze authority was bypassed")
			}
			reportPath := writeReport(t, t.TempDir(), report)
			reportBytes, _ := os.ReadFile(reportPath)
			finding := Finding{ID: "catalog-location", SourcePath: target, SourceSHA256: hash, InstructionVersion: hash, Cause: CauseStale, FailedBehavior: beforeErr.Error(), Recovery: "catalog-recovery read data/catalog.json with widget=1250", Excerpt: "archive/catalog.json", Replacement: "data/catalog.json"}
			if control == "unknown-cause" {
				finding.Cause = "unknown"
			}
			proposal := NativeProposal{Summary: "TEST FIXTURE: observed stale catalog path; minimal owning-doc correction", Findings: []Finding{finding}}
			proposalBytes, _ := json.Marshal(proposal)
			candidateSHA := sha256hex(proposalBytes)
			state := &loopSessionServer{directory: root, proposal: proposal}
			base := state.handler()
			reviewed, qaChecked := false, false
			// Extend only the existing mock response seam. The real client still
			// creates roots, waits, checks effective role/model and decodes finals.
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/prompt") {
					base.ServeHTTP(w, r)
					return
				}
				body, _ := io.ReadAll(r.Body)
				r.Body = io.NopCloser(bytes.NewReader(body))
				base.ServeHTTP(w, r)
				var request struct {
					Text string `json:"text"`
				}
				json.Unmarshal(body, &request)
				var packet struct {
					Report    Report          `json:"report"`
					Candidate json.RawMessage `json:"candidate"`
					SHA       string          `json:"candidate_sha256"`
				}
				at := strings.Index(request.Text, "\n")
				if at < 0 || json.Unmarshal([]byte(request.Text[at+1:]), &packet) != nil {
					t.Error("invalid self-contained native packet")
					return
				}
				id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/session/"), "/prompt")
				state.mu.Lock()
				defer state.mu.Unlock()
				role := state.roles[id]
				if packet.Report.TerminalOutcome.Status != TaskStatusError || packet.Report.RepairSafe || len(packet.Report.Signals) != 1 {
					t.Error("original source outcome/authority changed")
				}
				switch role {
				case "code-reviewer":
					current, _ := os.ReadFile(target)
					var proposed NativeProposal
					valid := nativeDecode(string(packet.Candidate), &proposed) == nil && len(proposed.Findings) == 1 && packet.SHA == candidateSHA && sha256hex(packet.Candidate) == candidateSHA && string(current) == original && proposed.Findings[0] == finding
					if !valid && control != "dirty-source" {
						t.Error("mock reviewer lacked exact original/proposal/digest")
					}
					reviewed = true
					decision := nativeDecision{Approved: valid && control != "review-reject", CandidateSHA: packet.SHA, Reason: "TEST MOCK: read exact source, observed failure/recovery and minimal candidate; not a live reviewer"}
					if control == "wrong-review-digest" {
						decision.CandidateSHA = strings.Repeat("0", 64)
					}
					encoded, _ := json.Marshal(decision)
					state.finals[id] = string(encoded)
					t.Logf("TEST MOCK role=%s root=%s typed_verdict=%s", role, id, encoded)
					if control == "dirty-after-review" {
						if err := os.WriteFile(target, []byte(original+"User changed this instruction.\n"), 0644); err != nil {
							t.Error(err)
						}
					}
				case "qa-runner":
					current, _ := os.ReadFile(target)
					output, checkErr := catalogTask(root, target, "widget", 1250)
					_, missingErr := catalogTask(root, target, "unknown", 1250)
					_, priceErr := catalogTask(root, target, "widget", 1)
					valid := reviewed && packet.SHA == candidateSHA && string(current) == post && checkErr == nil && output == "widget=1250" && missingErr != nil && priceErr != nil
					if control == "unknown-cause" {
						valid = reviewed && packet.SHA == candidateSHA && string(current) == original && os.IsNotExist(checkErr)
					}
					if !valid {
						t.Error("mock QA did not observe real postimage/task/control success")
					}
					qaChecked = true
					encoded, _ := json.Marshal(nativeDecision{Passed: valid && control != "qa-reject", CandidateSHA: packet.SHA, Reason: "TEST MOCK: same read-only catalog task passed, unknown SKU/wrong price rejected; not live QA"})
					state.finals[id] = string(encoded)
					t.Logf("TEST MOCK role=%s root=%s task_output=%q typed_verdict=%s", role, id, output, encoded)
				}
			}))
			defer server.Close()
			cfg := testConfig(root, t.TempDir(), server.URL)
			cfg.DispatchEndpoint = EndpointMessage
			cfg.DispatchModel = "fixture/worker"
			cfg.DispatchServerPassword = "synthetic-native-key"
			cfg.Reviewer = "code-reviewer"
			if control == "dirty-source" {
				os.WriteFile(target, []byte(original+"User changed this instruction.\n"), 0644)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			opts := dispatchOpts(cfg, reportPath, OpenSessionClient(cfg))
			if control == "protected-target" {
				opts.SettingsPath = target
			}
			receipt, err := Dispatch(ctx, opts)
			if control == "incomplete-evidence" {
				current, _ := os.ReadFile(target)
				creates, _, _ := state.snapshot()
				if err == nil || !strings.Contains(err.Error(), "not eligible") || len(creates) != 0 || string(current) != original {
					t.Fatal("incomplete report reached native write authority", err)
				}
				t.Log("TEST MOCK incomplete evidence retained signal, no native roots and unchanged failed task preimage")
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			current, _ := os.ReadFile(target)
			after, afterErr := catalogTask(root, target, "widget", 1250)
			if control == "repair" {
				if !reviewed || !qaChecked || receipt.Status != StatusRepaired || !receipt.NativeCompleted || string(current) != post || afterErr != nil || after != "widget=1250" {
					t.Fatalf("controlled repair did not complete: %+v %v", receipt, afterErr)
				}
				if receipt.RootID == receipt.ReviewerRootID || receipt.RootID == receipt.QARootID || receipt.ReviewerRootID == receipt.QARootID {
					t.Fatal("protocol roots were not distinct")
				}
				if len(receipt.Findings) != 1 || receipt.Findings[0].CandidateSHA256 != candidateSHA || receipt.Findings[0].PreimageSHA256 != hash || receipt.Findings[0].PostimageSHA256 != sha256hex([]byte(post)) {
					t.Fatal("exact candidate/WAL binding lost")
				}
			} else {
				want := original
				if control == "dirty-source" || control == "dirty-after-review" {
					want += "User changed this instruction.\n"
				}
				if string(current) != want || afterErr == nil || receipt.Status == StatusRepaired {
					t.Fatal("negative control changed source or certified repair")
				}
				if control == "qa-reject" && (!qaChecked || len(receipt.Findings) != 1 || !receipt.Findings[0].Restored) {
					t.Fatal("actual QA rejection did not roll back")
				}
				if control == "dirty-after-review" && (!reviewed || qaChecked || len(receipt.Findings) != 1 || receipt.Findings[0].Action != StatusConflict) {
					t.Fatal("Go source-hash sink did not reject post-review edit")
				}
			}
			for path, want := range map[string]string{dataPath: dataHash, neighbor: neighborHash} {
				body, _ := os.ReadFile(path)
				if sha256hex(body) != want {
					t.Fatal("unrelated fixture bytes changed", path)
				}
			}
			stillReport, _ := os.ReadFile(reportPath)
			if !bytes.Equal(stillReport, reportBytes) {
				t.Fatal("source report/output was rewritten")
			}
			creates, prompts, _ := state.snapshot()
			// Successful and rejected attempts do not create duplicate roots on reconnect.
			if _, err = Dispatch(ctx, opts); err != nil {
				t.Fatal(err)
			}
			againCreates, againPrompts, _ := state.snapshot()
			if len(creates) != len(againCreates) || len(prompts) != len(againPrompts) {
				t.Fatal("reconnect duplicated native work")
			}
			t.Logf("TEST MOCK control=%s before_error=%q after=%q after_error=%v source_sha=%s candidate_sha=%s receipt=%s roots=%v reviewer_checked=%t qa_checked=%t", control, beforeErr, after, afterErr, hash, candidateSHA, receipt.Status, creates, reviewed, qaChecked)
			receiptJSON, _ := json.Marshal(receipt)
			t.Logf("TEST MOCK exact_preimage=%q planned_postimage=%q proposal=%s durable_receipt=%s", original, post, proposalBytes, receiptJSON)
		})
	}
}
