// Spec: docs/specs/task-retrospective.md — Retrospective failure feedback.
package retrospective

import (
	"context"
	"strings"
	"testing"
)

// taskReceipts excludes the explicitly non-repairing maintenance queue, while
// asserting that an unconfigured failure cannot secretly dispatch a new root.
func taskReceipts(t *testing.T, store *Store) []*Receipt {
	t.Helper()
	all, err := store.ListReceipts()
	if err != nil {
		t.Fatal(err)
	}
	var tasks []*Receipt
	for _, receipt := range all {
		if strings.HasPrefix(receipt.TaskID, "pipeline-") {
			if receipt.Status != StatusQueued || receipt.RootID != "" || receipt.NativeCompleted {
				t.Fatal("unconfigured pipeline failure gained execution/write authority")
			}
		} else {
			tasks = append(tasks, receipt)
		}
	}
	return tasks
}

func TestPipelineFailureDeduplicatesWithoutRepairAuthority(t *testing.T) {
	store := testStore(t)
	job := &DrainJob{SessionID: "source", Error: "evidence incomplete"}
	opts := DrainOptions{Config: DefaultConfig()}
	for i := 0; i < 3; i++ {
		if err := recordPipelineFailure(context.Background(), opts, store, job); err != nil {
			t.Fatal(err)
		}
	}
	receipts, err := store.ListReceipts()
	if err != nil || len(receipts) != 1 || job.MaintenanceIncident == "" || len(taskReceipts(t, store)) != 0 {
		t.Fatal("failure did not produce one maintenance incident", err)
	}
	report, _, err := LoadReport(receipts[0].ReportLocator)
	if err != nil || report.Mode != "pipeline_failure" || report.RepairSafe || report.Dispatch.Eligible || ineligible(report) == "" {
		t.Fatal("maintenance report escaped ordinary task evidence/repair gates", err)
	}
}

func TestPipelineFailureUsesNativeIndependentAdviceOnly(t *testing.T) {
	for _, writes := range []bool{false, true} {
		t.Run(map[bool]string{false: "advisory", true: "reject-write"}[writes], func(t *testing.T) {
			f := newLoopFixture(t, unknownScript(t))
			f.setEnabledFor(t, t.TempDir(), true)
			f.sessions.proposal = NativeProposal{Summary: "Inspect episode boundary and add prompt-to-idle regression"}
			if writes {
				f.sessions.proposal.Findings = []Finding{{ID: "forbidden", SourcePath: f.agentSource}}
			}
			job := &DrainJob{SessionID: "source", Error: "evidence incomplete"}
			opts := DrainOptions{Config: f.cfg, ConfigPath: f.configPath, SourceRoot: f.root}
			original := readFileText(t, f.agentSource)
			if err := recordPipelineFailure(context.Background(), opts, f.store, job); err != nil {
				t.Fatal(err)
			}
			receipt, err := f.store.ReadReceipt(job.MaintenanceIncident)
			if err != nil {
				t.Fatal(err)
			}
			if writes {
				if receipt.Status != StatusPending || receipt.ReviewerRootID != "" {
					t.Fatal("pipeline code/text change was accepted")
				}
			} else if receipt.Status != StatusSuggested || !receipt.NativeCompleted || receipt.RootID == "" || receipt.ReviewerRootID == "" || receipt.RootID == receipt.ReviewerRootID {
				t.Fatalf("independent native advice path not completed: status=%s reason=%s", receipt.Status, receipt.Error)
			}
			creates, _, _ := f.sessions.snapshot()
			if err := recordPipelineFailure(context.Background(), opts, f.store, job); err != nil {
				t.Fatal(err)
			}
			after, _, _ := f.sessions.snapshot()
			if len(after) != len(creates) || readFileText(t, f.agentSource) != original || receipt.QARootID != "" {
				t.Fatal("maintenance repeated roots or wrote source")
			}
		})
	}
}
