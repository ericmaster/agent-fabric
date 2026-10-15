// Spec: docs/specs/task-retrospective.md
package retrospective

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"
)

const ReportSchemaVersion = 2
const (
	ReportStatusComplete   = "complete"
	ReportStatusIncomplete = "incomplete"
)

// Spec: docs/specs/task-retrospective.md via Report
type ModelInfo struct {
	Model         string `json:"model"`
	RubricVersion string `json:"rubric_version"`
}

// Spec: docs/specs/task-retrospective.md via Report
type ReportGap struct {
	Kind   string `json:"kind"`
	Detail string `json:"detail,omitempty"`
}

// Spec: docs/specs/task-retrospective.md via Report
type ReportSourceFile struct {
	Path       string `json:"path"`
	SHA256     string `json:"sha256"`
	SecretFile bool   `json:"secret_file,omitempty"`
}

// Spec: docs/specs/task-retrospective.md via Report
type ReportInterval struct {
	StartSeq int `json:"start_seq"`
	EndSeq   int `json:"end_seq"`
}

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
}

// Spec: docs/specs/task-retrospective.md via Report
type ReportBytes struct {
	Request         int `json:"request"`
	State           int `json:"state"`
	LongestQuestion int `json:"longest_question"`
}

// Spec: docs/specs/task-retrospective.md via Report
type ReportUsage struct {
	Requests       int  `json:"requests"`
	Retries        int  `json:"retries"`
	SizeRejections int  `json:"size_rejections"`
	Budget         int  `json:"budget"`
	Exhausted      bool `json:"exhausted"`
}

// Spec: docs/specs/task-retrospective.md via Report
type ReportDispatch struct {
	Eligible bool     `json:"eligible"`
	Reasons  []string `json:"reasons,omitempty"`
}

// Finding is causal evidence verified by independent native review, not a model hit.
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

// Spec: docs/specs/task-retrospective.md via Report
type ObservedOutcome struct {
	Status   string `json:"status"`
	Observed bool   `json:"observed"`
}

// Spec: docs/specs/task-retrospective.md via Report
type SignalInterpretation struct {
	SignalID       string      `json:"signal_id"`
	Status         string      `json:"status"`
	Choice         string      `json:"choice"`
	Cause          string      `json:"cause"`
	SupportCallID  string      `json:"support_call_id,omitempty"`
	Probability    float64     `json:"probability"`
	Confidence     float64     `json:"confidence"`
	Actionable     bool        `json:"actionable"`
	Delivered      bool        `json:"delivered"`
	Reason         string      `json:"reason,omitempty"`
	EvidenceIDs    []string    `json:"evidence_ids"`
	EvidenceSHA256 string      `json:"evidence_sha256"`
	Bytes          ReportBytes `json:"bytes"`
}

// Spec: docs/specs/task-retrospective.md via Report
type InterpretationReport struct {
	Status         string                 `json:"status"`
	Requests       int                    `json:"requests"`
	Rows           []SignalInterpretation `json:"rows"`
	OmittedSignals int                    `json:"omitted_signals"`
}

// Spec: docs/specs/task-retrospective.md via Report
type RoutingReport struct {
	Candidates     bool    `json:"candidates"`
	Actionable     bool    `json:"actionable"`
	MinProbability float64 `json:"minimum_actionability_probability"`
	MinConfidence  float64 `json:"minimum_actionability_confidence"`
}

// Report shares the detector's report instead of maintaining a second evaluator.
// Spec: docs/specs/task-retrospective.md
type Report struct {
	SignalReport
	GeneratedAt       string        `json:"generated_at"`
	Status            string        `json:"status"`
	IncompleteReasons []string      `json:"incomplete_reasons,omitempty"`
	Model             ModelInfo     `json:"model"`
	Routing           RoutingReport `json:"routing"`
	Usage             ReportUsage   `json:"usage"`
	Findings          []Finding     `json:"findings,omitempty"`
}

// Spec: docs/specs/task-retrospective.md via Report
func WriteReport(filename string, r *Report) error {
	if r.SchemaVersion != 2 {
		return fmt.Errorf("legacy report is read-only; re-analyze original evidence for v2")
	}
	// The common atomic writer takes only the data to serialize, never a client.
	body, err := json.Marshal(r)
	if err != nil {
		return err
	}
	return writeSignalDocument(filename, body)
}

var now = func() time.Time { return time.Now().UTC() }

// LoadReport keeps legacy bytes inspectable without translating historical
// success/trigger verdicts into v2 authority. Only an already journalled exact
// native repair can resume through Dispatch's separate fail-closed receipt check.
// Spec: docs/specs/task-retrospective.md via Report
func LoadReport(filename string) (*Report, []byte, error) {
	body, err := os.ReadFile(filename)
	if err != nil {
		return nil, nil, err
	}
	if !uniqueJSONKeys(body) {
		return nil, nil, fmt.Errorf("invalid or duplicate-key report JSON")
	}
	var header struct {
		SchemaVersion int `json:"schema_version"`
	}
	if err = json.Unmarshal(body, &header); err != nil {
		return nil, nil, err
	}
	if header.SchemaVersion == 1 {
		var legacy struct {
			Trace    ReportTrace      `json:"trace"`
			Evidence []evidenceRecord `json:"task_evidence"`
			Findings []Finding        `json:"findings"`
		}
		if err = json.Unmarshal(body, &legacy); err != nil {
			return nil, nil, err
		}
		return &Report{SignalReport: SignalReport{SchemaVersion: 1, Mode: "legacy_v1", Trace: legacy.Trace, Evidence: legacy.Evidence}, Findings: legacy.Findings}, body, nil
	}
	r := &Report{}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(r); err != nil {
		return nil, nil, err
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		return nil, nil, fmt.Errorf("trailing report data")
	}
	return r, body, nil
}
