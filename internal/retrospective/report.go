// Spec: docs/specs/task-retrospective.md
package retrospective

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// ReportSchemaVersion is the emitted report schema version.
const ReportSchemaVersion = 1

// Report statuses.
const (
	ReportStatusComplete   = "complete"
	ReportStatusIncomplete = "incomplete"
)

// ModelInfo carries the pinned model and rubric versions of the report.
// Spec: docs/specs/task-retrospective.md via Report
type ModelInfo struct {
	Model         string `json:"model"`
	RubricVersion string `json:"rubric_version"`
}

// ReportGap persists one trace-collection gap with redacted detail.
// Spec: docs/specs/task-retrospective.md via Report
type ReportGap struct {
	Kind   string `json:"kind"`
	Detail string `json:"detail,omitempty"`
}

// ReportSourceFile carries exact source file provenance (path + SHA-256);
// secret-file excerpts are redacted before persistence.
// Spec: docs/specs/task-retrospective.md via Report
type ReportSourceFile struct {
	Path       string `json:"path"`
	SHA256     string `json:"sha256"`
	SecretFile bool   `json:"secret_file,omitempty"`
}

// ReportInterval is the resolved task interval sequence space.
// Spec: docs/specs/task-retrospective.md via Report
type ReportInterval struct {
	StartSeq int `json:"start_seq"` // exclusive bound (records with seq > start)
	EndSeq   int `json:"end_seq"`   // inclusive; non-positive = end of trace
}

// ReportTrace is the normalized trace identity block of the report.
// Spec: docs/specs/task-retrospective.md via Report
type ReportTrace struct {
	SchemaVersion      int                `json:"schema_version"`
	InputSHA256        string             `json:"input_sha256"`
	Identities         Identities         `json:"identities"`
	TerminalStatus     string             `json:"terminal_status"`
	BoundarySource     string             `json:"boundary_source"`
	Interval           ReportInterval     `json:"interval"`
	Gaps               []ReportGap        `json:"gaps,omitempty"`
	SourceFiles        []ReportSourceFile `json:"source_files,omitempty"`
	ChildRefs          []string           `json:"child_refs,omitempty"`
	DescendantSessions []string           `json:"descendant_sessions,omitempty"`
	Deduplicated       int                `json:"deduplicated_records"`
	Records            int                `json:"records"`
	Steps              int                `json:"steps"`
}

// ReportOutcome is the typed outcome of the run with its provenance; the
// probability and confidence stay absent for unknown/abstained outcomes.
// Spec: docs/specs/task-retrospective.md via Report
type ReportOutcome struct {
	Source      string   `json:"source"` // model | local
	Choice      string   `json:"choice"`
	Probability *float64 `json:"probability,omitempty"`
	Confidence  *float64 `json:"confidence,omitempty"`
	Reasons     []string `json:"reasons,omitempty"`
}

// ReportGate carries the four stage-two eligibility conditions explicitly.
// Spec: docs/specs/task-retrospective.md via Report
type ReportGate struct {
	StageTwoRequested    bool `json:"stage_two_requested"`
	ModelSuccess         bool `json:"model_success"`
	CompleteBoundedInput bool `json:"complete_bounded_input"`
	NoTerminalAbortError bool `json:"no_terminal_abort_error"`
	EvaluatorAvailable   bool `json:"evaluator_available"`
}

// StageOneStep is one last-ten step reference of the bounded payload.
// Spec: docs/specs/task-retrospective.md via Report
type StageOneStep struct {
	Subject   string   `json:"subject"`
	Kind      StepKind `json:"kind"`
	SessionID string   `json:"session_id"`
	Tool      string   `json:"tool,omitempty"`
	Status    string   `json:"status,omitempty"`
	ExitCode  *int     `json:"exit_code,omitempty"`
	Paired    bool     `json:"paired"`
	Fragments int      `json:"result_fragments,omitempty"`
}

// ReportBytes records one request's measured byte totals (a conservative
// approximation of the token limits; bytes do not claim to be tokens).
// Spec: docs/specs/task-retrospective.md via Report
type ReportBytes struct {
	Request         int `json:"request"`
	State           int `json:"state"`
	LongestQuestion int `json:"longest_question"`
}

// ReportStageOne records the stage-one evidence and dispatch accounting.
// Spec: docs/specs/task-retrospective.md via Report
type ReportStageOne struct {
	Requested    bool           `json:"requested"`
	UserMessages int            `json:"user_messages"`
	Steps        []StageOneStep `json:"last_ten_steps"`
	Bytes        *ReportBytes   `json:"bytes,omitempty"`
}

// ReportRecordEval is one per-record classification verdict.
// Spec: docs/specs/task-retrospective.md via Report
type ReportRecordEval struct {
	Subject     string   `json:"subject"`
	Kind        string   `json:"kind"`
	Page        int      `json:"page,omitempty"`
	Choice      string   `json:"choice,omitempty"`
	Labels      []string `json:"labels,omitempty"`
	Probability *float64 `json:"probability,omitempty"`
	Confidence  *float64 `json:"confidence,omitempty"`
	Counted     bool     `json:"counted"`
	Reason      string   `json:"reason,omitempty"`
}

// ReportRequirementEval is one per-requirement omission verdict.
// Spec: docs/specs/task-retrospective.md via Report
type ReportRequirementEval struct {
	Subject     string   `json:"subject"`
	Page        int      `json:"page,omitempty"`
	Choice      string   `json:"choice,omitempty"`
	Probability *float64 `json:"probability,omitempty"`
	Confidence  *float64 `json:"confidence,omitempty"`
	Counted     bool     `json:"counted"`
	Reason      string   `json:"reason,omitempty"`
}

// VerifiedCount wraps a verified/withheld pair.
// Spec: docs/specs/task-retrospective.md via Report
type VerifiedCount struct {
	Verified int `json:"verified"`
	Withheld int `json:"withheld"`
}

// RawErrors splits raw terminal errors from avoidable waste.
// Spec: docs/specs/task-retrospective.md via Report
type RawErrors struct {
	Total               int `json:"total"`
	RecoveredOrExpected int `json:"recovered_or_expected"`
	Avoidable           int `json:"avoidable"`
	Unclassified        int `json:"unclassified"`
}

// EvaluationCounts is the exact counting block of the report. One unique tool
// attempt per call id; overlapping deliveries, waste labels, and requirement
// omissions are deduplicated to their unique record ids; required-step
// omissions are counted separately, never folded into the call denominator.
// Spec: docs/specs/task-retrospective.md via Report
type EvaluationCounts struct {
	TerminalCalls        int           `json:"terminal_calls"`
	Avoidable            VerifiedCount `json:"avoidable"`
	Productive           int           `json:"productive"`
	NecessaryExploration int           `json:"necessary_exploration"`
	ExpectedFailure      int           `json:"expected_failure"`
	AvoidableErrors      int           `json:"avoidable_error"`
	WrongDirection       int           `json:"wrong_direction"`
	Redundant            int           `json:"redundant"`
	Uncertain            int           `json:"uncertain"`
	Requirements         VerifiedCount `json:"requirements"`
	RawErrors            RawErrors     `json:"raw_errors"`
}

// RateReport is the avoidable rate over all terminal calls; unknown calls
// remain in the denominator, so the value is an explicit lower bound.
// Spec: docs/specs/task-retrospective.md via Report
type RateReport struct {
	Value       float64 `json:"value"`
	Numerator   int     `json:"numerator"`
	Denominator int     `json:"denominator"`
	LowerBound  bool    `json:"lower_bound"`
}

// RateTriggerState is the rate trigger evaluation against the thresholds.
// Spec: docs/specs/task-retrospective.md via Report
type RateTriggerState struct {
	Met       bool    `json:"met"`
	Value     float64 `json:"value"`
	Calls     int     `json:"calls"`
	Avoidable int     `json:"avoidable"`
	MinCalls  int     `json:"min_calls"`
	MinRate   float64 `json:"min_rate"`
}

// RedundantTriggerState is the verified redundant-attempts exception.
// Spec: docs/specs/task-retrospective.md via Report
type RedundantTriggerState struct {
	Met          bool     `json:"met"`
	Verified     bool     `json:"verified"`
	AttemptCalls []string `json:"attempt_calls,omitempty"`
	PrereqState  string   `json:"prereq_state"` // unchanged | varied | unrecorded
	Required     int      `json:"required"`
}

// BreachTriggerState is the verified high-impact explicit-requirement breach.
// Spec: docs/specs/task-retrospective.md via Report
type BreachTriggerState struct {
	Met         bool   `json:"met"`
	Verified    bool   `json:"verified"`
	Requirement string `json:"requirement,omitempty"`
	Reason      string `json:"reason,omitempty"`
}

// ThresholdsReport exposes the exact thresholds this run used.
// Spec: docs/specs/task-retrospective.md via Report
type ThresholdsReport struct {
	MinCountForRate           int     `json:"min_count_for_rate"`
	RateThreshold             float64 `json:"rate_threshold"`
	MinProbability            float64 `json:"min_probability"`
	MinConfidence             float64 `json:"min_confidence"`
	RedundantAttemptsRequired int     `json:"redundant_attempts_required"`
}

// TriggerReport is the trigger decision with every rule state.
// Spec: docs/specs/task-retrospective.md via Report
type TriggerReport struct {
	Eligible   bool                  `json:"eligible"`
	Via        []string              `json:"via,omitempty"`
	Rate       RateTriggerState      `json:"rate"`
	Redundant  RedundantTriggerState `json:"redundant_exception"`
	Breach     BreachTriggerState    `json:"high_impact_breach"`
	Thresholds ThresholdsReport      `json:"thresholds"`
}

// ReportEvaluation is the complete stage-two accounting.
// Spec: docs/specs/task-retrospective.md via Report
type ReportEvaluation struct {
	StageTwoRan  bool                    `json:"stage_two_ran"`
	Records      []ReportRecordEval      `json:"records,omitempty"`
	Requirements []ReportRequirementEval `json:"requirements,omitempty"`
	Counts       EvaluationCounts        `json:"counts"`
	Rate         RateReport              `json:"rate"`
	Trigger      TriggerReport           `json:"trigger"`
}

// PageDelivery records one delivered stage-two page with byte accounting.
// Spec: docs/specs/task-retrospective.md via Report
type PageDelivery struct {
	Index   int         `json:"index"`
	Records []string    `json:"records"`           // recordgram subject ids delivered on this page (new content)
	Overlap []string    `json:"overlap,omitempty"` // subject ids re-presented from the previous page
	Bytes   ReportBytes `json:"bytes"`
	Answers []string    `json:"answers,omitempty"` // answered request ids
}

// RecordDisposition is one record's coverage accounting row.
// Spec: docs/specs/task-retrospective.md via Report
type RecordDisposition struct {
	Subject     string `json:"subject"`
	Kind        string `json:"kind"`
	Covered     bool   `json:"covered"`
	Deliveries  int    `json:"deliveries"`      // page deliveries incl. overlap repeats; coverage counts the record once
	Pieces      int    `json:"pieces"`          // fragment pieces when split across pages
	Disposition string `json:"disposition"`     // covered | excluded:<reason> | unknown:<reason> | undelivered
	Pages       []int  `json:"pages,omitempty"` // pages the record was delivered on
}

// ReportCoverage is the complete coverage accounting of both stages.
// Spec: docs/specs/task-retrospective.md via Report
type ReportCoverage struct {
	StageOneDelivered  bool                `json:"stage_one_delivered"`
	Pages              []PageDelivery      `json:"stage_two_pages,omitempty"`
	Records            []RecordDisposition `json:"record_dispositions"`
	TotalRecords       int                 `json:"total_records"`
	CoveredRecords     int                 `json:"covered_records"`
	UndeliveredRecords int                 `json:"undelivered_records"`
	ExcludedRecords    int                 `json:"excluded_records"`
	OverlapDeliveries  int                 `json:"overlap_deliveries"`
	SplitRecords       int                 `json:"split_records"`
}

// ReportRedaction is the redaction accounting block.
// Spec: docs/specs/task-retrospective.md via Report
type ReportRedaction struct {
	SubstitutionsTotal   int            `json:"substitutions_total"`
	Substitutions        map[string]int `json:"substitutions"`
	PatternNames         []string       `json:"pattern_names"`
	ExcludedRecords      int            `json:"excluded_records"`
	MateriallyIncomplete bool           `json:"materially_incomplete"`
	Reasons              []string       `json:"reasons,omitempty"`
}

// ReportUsage retains the run's request accounting and budget.
// Spec: docs/specs/task-retrospective.md via Report
type ReportUsage struct {
	Requests       int  `json:"requests"`
	Retries        int  `json:"retries"`
	SizeRejections int  `json:"size_rejections"`
	Splits         int  `json:"splits"`
	Budget         int  `json:"budget"`
	Exhausted      bool `json:"exhausted"`
}

// ReportDispatch is the phase-two gating block: eligible requires the
// trigger, complete coverage, complete redaction, and the run's delivery.
// Spec: docs/specs/task-retrospective.md via Report
type ReportDispatch struct {
	Eligible bool     `json:"eligible"`
	Reasons  []string `json:"reasons,omitempty"`
}

// Finding is optional dispatch evidence. Analyze does not emit it. A source
// that was merely read is not causation.
// Spec: docs/specs/task-retrospective.md via Report
type Finding struct {
	ID                 string `json:"id"`
	SourcePath         string `json:"source_path"`
	SourceSHA256       string `json:"source_sha256"`
	Cause              string `json:"cause"`
	InstructionVersion string `json:"instruction_version,omitempty"`
	FailedBehavior     string `json:"failed_behavior,omitempty"`
	Recovery           string `json:"recovery,omitempty"`
	MerelyRead         bool   `json:"merely_read,omitempty"`
	Excerpt            string `json:"excerpt,omitempty"`
	Replacement        string `json:"replacement,omitempty"`
}

// Report is the versioned deliverable written by `agf retrospective analyze`.
// Spec: docs/specs/task-retrospective.md
type Report struct {
	TaskEvidence      []evidenceRecord `json:"task_evidence,omitempty"`
	SchemaVersion     int              `json:"schema_version"`
	Status            string           `json:"status"`
	IncompleteReasons []string         `json:"incomplete_reasons,omitempty"`
	GeneratedAt       string           `json:"generated_at"`
	Model             ModelInfo        `json:"model"`
	Trace             ReportTrace      `json:"trace"`
	Outcome           ReportOutcome    `json:"outcome"`
	Gate              ReportGate       `json:"success_gate"`
	StageOne          ReportStageOne   `json:"stage_one"`
	Evaluation        ReportEvaluation `json:"evaluation"`
	Coverage          ReportCoverage   `json:"coverage"`
	Redaction         ReportRedaction  `json:"redaction"`
	Usage             ReportUsage      `json:"usage"`
	Dispatch          ReportDispatch   `json:"dispatch"`
	Findings          []Finding        `json:"findings,omitempty"`
}

// WriteReport atomically persists the report (temp file inside the output
// directory, mode 0600) so partial writes are never visible.
// Spec: docs/specs/task-retrospective.md via Report
func WriteReport(path string, report *Report) error {
	body, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("encode report: %w", err)
	}
	body = append(body, '\n')
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fmt.Errorf("prepare report directory: %w", err)
	}
	tmp, err := os.CreateTemp(directory, ".retrospective-report-*")
	if err != nil {
		return fmt.Errorf("stage report file: %w", err)
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err = tmp.Chmod(0o600); err == nil {
		_, err = tmp.Write(body)
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(name, path)
}

// now is the wall clock used by Analyze (injectable for tests).
var now = func() time.Time { return time.Now().UTC() }

// LoadReport strict-decodes a report file. Unknown fields and trailing values fail.
// Spec: docs/specs/task-retrospective.md via Report
func LoadReport(path string) (*Report, []byte, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read report: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	report := &Report{}
	if err := decoder.Decode(report); err != nil {
		return nil, nil, fmt.Errorf("decode report %s: %w", path, err)
	}
	var trailing struct{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, nil, fmt.Errorf("decode report %s: trailing data", path)
	}
	return report, body, nil
}
