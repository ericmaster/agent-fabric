// Spec: docs/specs/task-retrospective.md
package retrospective

import (
	"strings"
	"testing"
)

// forgedClient injects forged/invalid answers on top of the scripted set to
// prove that abstention keeps counts safe.
// Spec: docs/specs/task-retrospective.md
type forgedClient struct {
	*scriptedClient
}

// TestForgedCitationsAndInvalidProbabilitiesAbstain — answers that reference
// subject/query ids outside the supplied scope, or probabilities outside
// [0,1], abstain instead of counting.
// Spec: docs/specs/task-retrospective.md
func TestForgedCitationsAndInvalidProbabilitiesAbstain(t *testing.T) {
	trace := baseFixture(4)
	// The secret-assignment-rail test fixture carries 4 calls; answer only
	// c3 Validly, and inject forged/invalid answers for the others.
	classifications := map[string]Answer{
		"c3": confidentAnswer("c3", "avoidable-error"),
	}
	client := &scriptedClient{
		outcome:         successfulOutcome(),
		classifications: classifications,
		extra: []Answer{
			// Forged subject on a valid request reference: abstains.
			{RequestID: "q-c1", Subject: "invented-c-id", Choice: "avoidable-error", Probability: f64(0.99), Confidence: f64(0.99)},
			// Valid subject, invented classification: abstains.
			{RequestID: "q-c2", Subject: "c2", Choice: "totally-forged", Probability: f64(0.99), Confidence: f64(0.99)},
			// Valid subject and choice, invalid probability: abstains.
			{RequestID: "q-c4", Subject: "c4", Choice: "wrong-direction", Probability: f64(1.5), Confidence: f64(0.95)},
		},
	}
	report := analyze(t, DefaultConfig(), trace, client)
	counts := report.Evaluation.Counts
	if counts.Avoidable.Verified != 1 {
		t.Fatalf("forged/invalid answers inflated the avoidable count: %+v", counts)
	}
	abstained := map[string]int{}
	for _, row := range report.Evaluation.Records {
		if row.Choice == "" {
			abstained[row.Subject]++
		}
	}
	if len(abstained) != 3 {
		t.Fatalf("abstained rows = %v, want c1/c2/c4 withheld", abstained)
	}
	if counts.Uncertain != 3 {
		t.Fatalf("uncertain count = %d, want 3", counts.Uncertain)
	}
	if report.Evaluation.Trigger.Eligible {
		t.Fatalf("forged answers triggered the pipeline: %+v", report.Evaluation.Trigger)
	}
	// Withheld abstentions keep the unknown rows in the denominator.
	if report.Evaluation.Rate.Denominator != 4 {
		t.Fatalf("denominator = %d", report.Evaluation.Rate.Denominator)
	}
}

// TestStageOneInvalidOutcomeAnswerAbstains — an invalid type value, a
// missing probability, or a probability outside [0,1] yields an unknown
// outcome with the reason attached, and stage two never runs.
// Spec: docs/specs/task-retrospective.md
func TestStageOneInvalidOutcomeAnswerAbstains(t *testing.T) {
	cases := []struct {
		name    string
		outcome *Answer
	}{
		{"forged outcome id", &Answer{RequestID: outcomeQuestionID, Subject: "task", Choice: "victorious", Probability: f64(0.99), Confidence: f64(0.99)}},
		{"probability outside range", &Answer{RequestID: outcomeQuestionID, Subject: "task", Choice: "succeeded", Probability: f64(1.2), Confidence: f64(0.99)}},
		{"confidence outside range", &Answer{RequestID: outcomeQuestionID, Subject: "task", Choice: "succeeded", Probability: f64(0.99), Confidence: f64(-0.5)}},
		{"wrong request id", &Answer{RequestID: "q-invented", Subject: "task", Choice: "succeeded", Probability: f64(0.99), Confidence: f64(0.99)}},
	}
	for _, tested := range cases {
		t.Run(tested.name, func(t *testing.T) {
			trace := baseFixture(3)
			client := &scriptedClient{outcome: tested.outcome}
			report := analyze(t, DefaultConfig(), trace, client)
			if report.Outcome.Choice != "unknown" || report.Outcome.Source != "model" {
				t.Fatalf("outcome = %+v", report.Outcome)
			}
			if len(report.Outcome.Reasons) == 0 {
				t.Fatal("abstention reason missing")
			}
			if len(client.requests) > 1 {
				t.Fatalf("stage two ran after an abstained stage one: %d requests", len(client.requests))
			}
		})
	}
}

// TestSyntheticReplayProducesSchemaValidReport — the full pipeline replay of
// one synthetic trace against recorded mock HTTP yields a schema-valid
// versioned report with exact call/source references, raw-error and avoidable
// counts, coverage, and model metadata.
// Spec: docs/specs/task-retrospective.md
func TestSyntheticReplayProducesSchemaValidReport(t *testing.T) {
	trace := bigFixture(23, 0)
	trace.Calls[len(trace.Calls)-1].Result = []Fragment{{ID: "c23-r1", Text: "final output bytes"}}
	recovered := 3
	trace.Calls[2].ExitCode = &recovered
	trace.Calls[2].Status = StatusError
	trace.Calls[2].Result = []Fragment{{ID: "c3-r1", Text: "red test no-match; next attempt green"}}
	trace.Calls[4].Status = StatusSuccess
	trace.SourceFiles = []SourceRef{{Path: "docs/runbook.md", SHA256: "runbook-digest"}}
	trace.Requirements = []Requirement{{ID: "req-post", Text: "the fix must update the runbook"}}
	classifications := map[string]Answer{
		"c1":       confidentAnswer("c1", "productive"),
		"c2":       confidentAnswer("c2", "redundant"),
		"c3":       confidentAnswer("c3", "expected-failure"),
		"c4":       confidentAnswer("c4", "necessary-exploration"),
		"req-post": confidentAnswer("req-post", "present"),
	}
	for i := 5; i <= 23; i++ {
		id := "c" + itoa(i)
		classifications[id] = confidentAnswer(id, "productive")
	}
	client := &scriptedClient{outcome: successfulOutcome(), classifications: classifications}
	report := analyze(t, DefaultConfig(), trace, client)
	// Schema validity: every top-level block present with the exact versioned fields.
	if report.SchemaVersion != 1 || report.Outcome.Choice != "succeeded" {
		t.Fatalf("report invalid: %+v", report)
	}
	if report.Model.Model != "jev-1.13.0" || report.Model.RubricVersion != RubricVersion {
		t.Fatalf("model metadata missing: %+v", report.Model)
	}
	counts := report.Evaluation.Counts
	if counts.TerminalCalls != 23 {
		t.Fatalf("terminal calls = %d", counts.TerminalCalls)
	}
	if counts.Avoidable.Verified != 1 || counts.AvoidableErrors != 0 || counts.Redundant != 1 || counts.ExpectedFailure != 1 {
		t.Fatalf("exact counts wrong: %+v", counts)
	}
	if counts.RawErrors.Total != 1 || counts.RawErrors.RecoveredOrExpected != 1 {
		t.Fatalf("raw-error accounting wrong: %+v", counts.RawErrors)
	}
	if counts.Requirements.Verified != 0 {
		t.Fatalf("present requirement counted as omission: %+v", counts.Requirements)
	}
	// Exact call/source reference: the redundant classification names the exact
	// subject id and page, and the trace provenance keeps the hash.
	var redundantRow, c3Row bool
	for _, row := range report.Evaluation.Records {
		if row.Subject == "c2" && row.Choice == "redundant" && row.Page > 0 && row.Counted {
			redundantRow = true
		}
		if row.Subject == "c3" && row.Choice == "expected-failure" {
			c3Row = true
		}
	}
	if !redundantRow || !c3Row {
		t.Fatalf("exact call references missing: %+v", report.Evaluation.Records)
	}
	if len(report.Trace.SourceFiles) != 1 || report.Trace.SourceFiles[0].SHA256 != "runbook-digest" {
		t.Fatalf("source provenance missing: %+v", report.Trace.SourceFiles)
	}
	if report.Trace.InputSHA256 == "" {
		t.Fatal("input digest missing")
	}
	// Coverage complete; one page per state boundary list.
	if report.Coverage.CoveredRecords != report.Coverage.TotalRecords {
		t.Fatalf("coverage incomplete: %+v", report.Coverage)
	}
}

// TestReportRecordsThresholdsAndUsage — the report retains thresholds, usage,
// and uncertainty content per the plan contract.
// Spec: docs/specs/task-retrospective.md
func TestReportRecordsThresholdsAndUsage(t *testing.T) {
	trace := baseFixture(4)
	client := &scriptedClient{outcome: successfulOutcome(), classifications: map[string]Answer{
		"c1": confidentAnswer("c1", "redundant"),
	}}
	report := analyze(t, DefaultConfig(), trace, client)
	if report.Evaluation.Trigger.Thresholds.MinProbability != 0.90 || report.Evaluation.Trigger.Thresholds.MinConfidence != 0.80 {
		t.Fatalf("thresholds missing: %+v", report.Evaluation.Trigger.Thresholds)
	}
	if report.Usage.Requests == 0 {
		t.Fatalf("usage missing: %+v", report.Usage)
	}
	if len(report.Coverage.Records) == 0 {
		t.Fatal("coverage rows missing")
	}
	if !strings.Contains(report.Status, "complete") {
		t.Fatalf("status = %q", report.Status)
	}
}

// TestEvaluatorUnavailableProducesReportNotCorrection — a nil client (no
// host key / endpoint) yields an unknown outcome, an incomplete status, and
// zero requests (unmet capability, never a silent pass).
// Spec: docs/specs/task-retrospective.md
func TestEvaluatorUnavailableProducesReportNotCorrection(t *testing.T) {
	trace := baseFixture(3)
	report := analyze(t, DefaultConfig(), trace, nil)
	if report.Status != ReportStatusIncomplete {
		t.Fatalf("status = %q", report.Status)
	}
	if !containsString(report.IncompleteReasons, "evaluator unavailable") {
		t.Fatalf("reasons = %v", report.IncompleteReasons)
	}
	if report.Dispatch.Eligible {
		t.Fatal("dispatch eligible without evaluator")
	}
	if report.Usage.Requests != 0 {
		t.Fatalf("requests = %d", report.Usage.Requests)
	}
}

// TestBinaryRecordExclusionBlocksCorrection — an all-binary call record is
// excluded with a coverage reason and its undecodable content marks the
// redaction materially incomplete (blocking automatic correction).
// Spec: docs/specs/task-retrospective.md
func TestBinaryRecordExclusionBlocksCorrection(t *testing.T) {
	trace := baseFixture(2)
	trace.Calls[0].Result = []Fragment{{ID: "c1-r1", Binary: true}}
	trace.Calls[1].Result = []Fragment{{ID: "c2-r1", Binary: true}}
	client := &scriptedClient{outcome: successfulOutcome()}
	report := analyze(t, DefaultConfig(), trace, client)
	if report.Coverage.ExcludedRecords != 2 {
		t.Fatalf("excluded records = %d", report.Coverage.ExcludedRecords)
	}
	if !report.Redaction.MateriallyIncomplete || len(report.Redaction.Reasons) == 0 {
		t.Fatalf("materially incomplete redaction not flagged: %+v", report.Redaction)
	}
	for _, row := range report.Coverage.Records {
		if row.Subject == "c1" && row.Disposition != "excluded:binary-record" {
			t.Fatalf("binary row disposition = %q", row.Disposition)
		}
	}
	// The excluded content never reaches the evaluation denominator silently.
	if report.Evaluation.Counts.TerminalCalls != 2 {
		t.Fatalf("terminal calls = %d", report.Evaluation.Counts.TerminalCalls)
	}
}
