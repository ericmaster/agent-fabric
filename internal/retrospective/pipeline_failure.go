// Spec: docs/specs/task-retrospective.md — Retrospective failure feedback.
package retrospective

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// recordPipelineFailure reuses the durable incident/native review path, not the
// incomplete original task's eligibility. Its ONLY outcome is reviewed advice;
// it never enters a text/code repair sink or starts another retrospective.
func recordPipelineFailure(ctx context.Context, opts DrainOptions, store *Store, job *DrainJob) error {
	code := "retrospective_job_failed"
	for _, item := range []struct{ match, code string }{
		{"startup incomplete", "missing_episode_intake"},
		{"evidence incomplete", "incomplete_episode_evidence"},
		{"dispatch:", "retrospective_dispatch_failed"},
		{"analyze episode:", "retrospective_analysis_failed"},
		{"interpretation unavailable", "retrospective_interpretation_failed"},
	} {
		if strings.Contains(job.Error, item.match) {
			code = item.code
			break
		}
	}
	// One incident per failure class and installed source root, not one root
	// per failed task or model attempt. No task content enters the key/prompt.
	identity, _ := json.Marshal([]string{"pipeline-failure-v1", opts.SourceRoot, code})
	id := "pipeline-" + sha256hex(identity)
	job.MaintenanceIncident = id
	receipt, err := store.ReadReceipt(id)
	if err == nil {
		return nil
	} // including ambiguous/pending: never auto-retry creation
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	stamp := time.Now().UTC().Format(time.RFC3339)
	marker := learningOrigin(id)
	report := &Report{SignalReport: SignalReport{SchemaVersion: ReportSchemaVersion, Mode: "pipeline_failure",
		Trace:    ReportTrace{Identities: Identities{TaskID: id, ExecutionRoot: opts.SourceRoot}, InputSHA256: sha256hex(identity)},
		Evidence: []evidenceRecord{{ID: "pipeline-diagnostic", Kind: RecordSyntheticContext, SourceKind: "agf.pipeline_failure", Text: "Original report locator: " + job.ReportPath + "\nObserved job locator: " + jobsPath(store, job.SessionID)}}},
		Status: ReportStatusIncomplete, IncompleteReasons: []string{code}, GeneratedAt: stamp}
	path := filepath.Join(store.Dir, "reports", id+".json")
	if err := WriteReport(path, report); err != nil {
		return err
	}
	raw, _ := json.Marshal(report)
	receipt = &Receipt{SchemaVersion: 1, IncidentID: id, TaskID: id, LearningOrigin: marker,
		Directory: opts.SourceRoot, Title: marker + " retrospective pipeline failure",
		Status: StatusQueued, ReportLocator: path, ReportSHA256: sha256hex(raw), CreatedAt: stamp, UpdatedAt: stamp}
	if err := store.WriteReceipt(receipt); err != nil {
		return err
	}
	cfg := opts.Config
	// No new model selection, fallback target, or authority to change runtime
	// hooks/executables. Missing explicit capability leaves a visible queue.
	if cfg == nil || cfg.DispatchServer == "" || cfg.DispatchModel == "" || !filepath.IsAbs(opts.SourceRoot) || !withinAny(cfg.CanonicalRoots, opts.SourceRoot) {
		return nil
	}
	enabled, err := Enabled(opts.ConfigPath)
	if err != nil || !enabled {
		return err
	}
	client := OpenSessionClient(cfg)
	dispatch := DispatchOptions{Config: cfg, SettingsPath: opts.ConfigPath}
	stageCtx, done := context.WithTimeout(ctx, cfg.DispatchTimeout)
	defer done()
	objective := "Investigate this retrospective PIPELINE FAILURE, not the incomplete original task. Read the local diagnostic/report locator, identify a concrete minimal correction and runnable regression check in summary. Do NOT edit any file, settings, hooks, executable or permissions. Do NOT classify missing original evidence as complete. Return ONLY {\"findings\":[],\"summary\":\"actionable diagnosis, affected path and proposed check\"}. This is advisory maintenance intake; implementation needs ordinary scoped change/review authority."
	text, err := nativeStage(stageCtx, dispatch, store, receipt, report, marker, LearningCorrectorID, "generate", nativePacket(dispatch, receipt, report, objective, nil, ""), client)
	if err != nil {
		receipt.Status, receipt.Error = StatusPending, "pipeline diagnosis unavailable; inspect existing receipt/root, no automatic retry"
		_, saveErr := save(store, receipt)
		return saveErr
	}
	redactor, err := NewRedactor(cfg)
	if err != nil {
		return err
	}
	text = redactor.Text(text)
	var proposal NativeProposal
	if nativeDecode(text, &proposal) != nil || proposal.Summary == "" || len(proposal.Findings) != 0 {
		receipt.Status, receipt.Error = StatusPending, "pipeline diagnosis must be advice only; no edits accepted"
		_, err = save(store, receipt)
		return err
	}
	receipt.CorrectorResult = text
	body, _ := json.Marshal(proposal)
	digest := sha256hex(body)
	verdict, err := nativeStage(stageCtx, dispatch, store, receipt, report, marker, cfg.Reviewer, "review", nativePacket(dispatch, receipt, report, "Independently verify the pipeline diagnosis and proposed regression check. No edits, repair authority or invented coverage. Return only {\"approved\":true|false,\"candidate_sha256\":\"supplied digest\",\"reason\":\"evidence-backed verdict\"}.", proposal, digest), client)
	verdict = redactor.Text(verdict)
	var review nativeDecision
	if err != nil || nativeDecode(verdict, &review) != nil || !review.Approved || review.CandidateSHA != digest || review.Reason == "" {
		receipt.Status, receipt.Error = StatusPending, "pipeline diagnosis lacks independent acceptance"
	} else {
		receipt.ReviewVerdict, receipt.Status, receipt.NativeCompleted = verdict, StatusSuggested, true
	}
	_, err = save(store, receipt)
	return err
}
