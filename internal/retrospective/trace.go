// Spec: docs/specs/task-retrospective.md
package retrospective

import (
	"fmt"
	"sort"
)

// TraceSchemaVersion is the only accepted input trace schema version.
const TraceSchemaVersion = 1

// MaxDescendantDepth bounds nested descendant traces (malformed loops fail closed).
const MaxDescendantDepth = 16

// Tool call statuses. An empty status is treated as an unresolved running tool.
const (
	StatusSuccess   = "success"
	StatusError     = "error"
	StatusAborted   = "aborted"
	StatusFailed    = "failed"
	StatusCancelled = "cancelled"
	StatusRunning   = "running"
)

// Record kinds within the task interval. Requirements are explicit constraint
// records and carry no session sequence; they are ordered after all records.
const (
	RecordUserMessage      = "user_message"
	RecordAssistantMessage = "assistant_message"
	RecordAssistantFinal   = "assistant_final"
	RecordSyntheticContext = "synthetic_context"
	RecordToolCall         = "tool_call"
	RecordRequirement      = "requirement"
)

// Trace-level terminal statuses.
const (
	TaskStatusOK      = "ok"
	TaskStatusAborted = "aborted"
	TaskStatusError   = "error"
)

// Identities identifies the root session, the recorded session, the host task
// (when the host supplies one) and the execution root/source revision.
// Spec: docs/specs/task-retrospective.md via Trace
type Identities struct {
	RootSessionID  string `json:"root_session_id"`
	SessionID      string `json:"session_id"`
	TaskID         string `json:"task_id,omitempty"`
	ExecutionRoot  string `json:"execution_root"`
	SourceRevision string `json:"source_revision,omitempty"`
}

// TaskInterval is the host task block; it is preferred over boundary
// derivation. Interval = records with start_seq <= seq (and seq <= end_seq
// when end_seq > 0).
// Spec: docs/specs/task-retrospective.md via Trace
type TaskInterval struct {
	ID       string `json:"id"`
	StartSeq int    `json:"start_seq"`
	EndSeq   int    `json:"end_seq,omitempty"`
}

// Boundary is an explicit task boundary marker of the root session.
// accepted_success entries are positional (after_seq): the resolved interval
// starts after the last accepted success. task_start entries mark where a new
// task started; more than one anchored inside the resolved interval is an
// ambiguous multi-task interval.
// Spec: docs/specs/task-retrospective.md via Trace
type Boundary struct {
	Kind     string `json:"kind"` // accepted_success | task_start
	AfterSeq int    `json:"after_seq"`
	Detail   string `json:"detail,omitempty"`
}

// Fragment is one ordered result fragment of a call. Binary fragments carry a
// digest instead of decodable text and are excluded from transmitted evidence.
// Spec: docs/specs/task-retrospective.md via Trace
type Fragment struct {
	ID     string `json:"id"`
	Text   string `json:"text,omitempty"`
	Binary bool   `json:"binary,omitempty"`
}

// SourceRef references a source file the task read or wrote, by exact path,
// content SHA-256, and an optional recorded excerpt. secret_file bodies are
// redacted before evidence assembly and never persisted or transmitted.
// Spec: docs/specs/task-retrospective.md via Trace
type SourceRef struct {
	Path       string `json:"path"`
	SHA256     string `json:"sha256"`
	Excerpt    string `json:"excerpt,omitempty"`
	SecretFile bool   `json:"secret_file,omitempty"`
}

// Message is one normalized observable message. kind "reasoning" and
// "unsupported" messages are excluded from transmitted evidence with a
// coverage reason but remain visible in coverage accounting.
// Spec: docs/specs/task-retrospective.md via Trace
type Message struct {
	ID     string            `json:"id"`
	Seq    int               `json:"seq"`
	Role   string            `json:"role"`           // user | assistant | context (only kind synthetic)
	Kind   string            `json:"kind,omitempty"` // "" | final | action | reasoning | unsupported | synthetic
	Text   string            `json:"text,omitempty"`
	Hidden bool              `json:"hidden,omitempty"`
	Binary bool              `json:"binary,omitempty"`
	Env    map[string]string `json:"env,omitempty"`
}

// CallRecord is one tool call; status "" is an unresolved running tool. An
// input_omitted call was recorded by the collector with the input content
// withheld; it is never a paired step.
// Spec: docs/specs/task-retrospective.md via Trace
type CallRecord struct {
	ID           string            `json:"id"`
	Seq          int               `json:"seq"`
	Tool         string            `json:"tool"`
	Status       string            `json:"status,omitempty"`
	ExitCode     *int              `json:"exit_code,omitempty"`
	Input        string            `json:"input,omitempty"`
	InputOmitted bool              `json:"input_omitted,omitempty"`
	Result       []Fragment        `json:"result,omitempty"`
	Env          map[string]string `json:"env,omitempty"`
	Prereq       string            `json:"prereq_fingerprint,omitempty"`
	SourceRefs   []SourceRef       `json:"source_refs,omitempty"`
}

// Requirement is one explicit task requirement statement for omission
// classification. Common API-key claim: the trace always supplies the explicit
// requirement ids; the report never fabricates omissions for other records.
// Spec: docs/specs/task-retrospective.md via Trace
type Requirement struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

// Gap is a collection gap marker of the trace collector (schema stable: only
// the enumerated kinds carry defined incompleteness semantics).
// Spec: docs/specs/task-retrospective.md via Trace
type Gap struct {
	Kind   string `json:"kind"` // compacted | partial_stream | missing_source
	Detail string `json:"detail,omitempty"`
}

// Trace is the schema v1 input document.
// Spec: docs/specs/task-retrospective.md
type Trace struct {
	SchemaVersion  int           `json:"schema_version"`
	Identities     Identities    `json:"identities"`
	TaskInterval   *TaskInterval `json:"task,omitempty"`
	TerminalStatus string        `json:"terminal_status,omitempty"`
	Boundaries     []Boundary    `json:"boundaries,omitempty"`
	Messages       []Message     `json:"messages,omitempty"`
	Calls          []CallRecord  `json:"calls,omitempty"`
	Requirements   []Requirement `json:"requirements,omitempty"`
	ChildRefs      []string      `json:"child_refs,omitempty"`
	Descendants    []*Trace      `json:"descendants,omitempty"`
	Gaps           []Gap         `json:"gaps,omitempty"`
	SourceFiles    []SourceRef   `json:"source_files,omitempty"`
}

// Record is the normalized, deduplicated, stable-ordered record set within the
// task interval.
// Spec: docs/specs/task-retrospective.md via Normalized
type Record struct {
	ID        string
	Kind      string
	Seq       int
	SessionID string
	// Message fields.
	Text        string
	Hidden      bool
	Binary      bool
	Reasoning   bool
	Unsupported bool
	// Call fields.
	Tool         string
	Status       string
	ExitCode     *int
	Input        string
	InputOmitted bool
	Result       []Fragment
	Env          map[string]string
	Prereq       string
	SourceRefs   []SourceRef
}

// Paired reports whether a terminal tool call has paired input/result
// evidence; a call whose input was omitted by the collector is never a paired
// step.
// Spec: docs/specs/task-retrospective.md via Record
func (r *Record) Paired() bool {
	return r.Kind == RecordToolCall && !r.InputOmitted && len(r.Result) > 0
}

// Terminal reports whether the call has a terminal status.
// Spec: docs/specs/task-retrospective.md via Record
func (r *Record) Terminal() bool {
	return r.Kind == RecordToolCall && r.Status != StatusRunning
}

// RawError reports whether the terminal call ended in a raw error state:
// status error/aborted/failed or a recorded nonzero exit code. Raw errors are
// counted separately and their expected/recovered outcomes stay distinct from
// avoidable waste.
// Spec: docs/specs/task-retrospective.md via Record
func (r *Record) RawError() bool {
	if !r.Terminal() {
		return false
	}
	switch r.Status {
	case StatusError, StatusAborted, StatusFailed:
		return true
	}
	return r.ExitCode != nil && *r.ExitCode != 0
}

// StepKind distinguishes tool attempts from assistant action messages.
// Spec: docs/specs/task-retrospective.md via Normalized
type StepKind string

const (
	StepToolAttempt StepKind = "tool-attempt"
	StepAction      StepKind = "assistant-action"
)

// Step is one stable step of the task interval: a terminal tool attempt with
// paired input/result or a non-final visible assistant action message.
// Spec: docs/specs/task-retrospective.md via Normalized
type Step struct {
	ID     string
	Kind   StepKind
	Record *Record
}

// CompletenessReason is one material incompleteness or gate reason of the
// trace. Reasons arrive only from the deterministic local gates; never from
// model guesses.
// Spec: docs/specs/task-retrospective.md via Normalized
type CompletenessReason struct {
	Kind   string
	Detail string
}

// Material incompleteness / gate reason kinds.
const (
	ReasonAmbiguousBoundary  = "ambiguous_multi_task_boundary"
	ReasonRunningTool        = "unresolved_running_tool"
	ReasonMissingChild       = "missing_child_trace"
	ReasonCompacted          = "compacted_evidence"
	ReasonMissingFinal       = "missing_final_response"
	ReasonTerminalAbortError = "terminal_abort_error"
	ReasonPartialRedaction   = "materially_incomplete_redaction"
	ReasonStageOneOversized  = "oversized_bounded_evidence"
)

// Normalized is the validated, ordered, deduplicated trace ready for
// evaluation.
// Spec: docs/specs/task-retrospective.md
type Normalized struct {
	Trace *Trace

	// Records contains every deduplicated observable record of the task
	// interval (root session plus linked descendants, once each), in stable
	// (seq, id) order.
	Records    []*Record
	Exclusions []*Record
	// Requirements contains the normalized explicit requirement records.
	Requirements []*Record

	// Deduplicated counts duplicate record IDs collapsed during
	// normalization (first occurrence wins).
	Deduplicated int

	// Steps lists every stable step of the interval in order.
	Steps []*Step

	// IntervalStart (exclusive) and IntervalEnd (inclusive) bound the record
	// sequence space; IntervalStart < 0 means session start.
	IntervalStart int
	IntervalEnd   int

	// BoundarySource is host | accepted-success | session-start.
	BoundarySource string

	// Reasons are the material reasons the interval is not a complete bounded
	// input (recorded only for incomplete gate states).
	Reasons []CompletenessReason
}

// Complete reports the absence of material incompleteness reasons.
// Spec: docs/specs/task-retrospective.md via Normalized
func (n *Normalized) Complete() bool { return len(n.Reasons) == 0 }

// TerminalCalls returns the unique terminal call records of the interval.
// Spec: docs/specs/task-retrospective.md via Normalized
func (n *Normalized) TerminalCalls() []*Record {
	out := make([]*Record, 0, len(n.Records))
	for _, record := range n.Records {
		if record.Terminal() {
			out = append(out, record)
		}
	}
	return out
}

// LastSteps returns the last count stable steps (the full step list when smaller).
// Spec: docs/specs/task-retrospective.md via Normalized
func (n *Normalized) LastSteps(count int) []*Step {
	if count <= 0 || len(n.Steps) == 0 {
		return nil
	}
	if len(n.Steps) <= count {
		return append([]*Step(nil), n.Steps...)
	}
	return append([]*Step(nil), n.Steps[len(n.Steps)-count:]...)
}

// FinalResponse returns the interval's final assistant response record when
// present: the last assistant message inside the resolved interval that is
// visible, decodable, and not private reasoning or unsupported content.
// Out-of-interval records and hidden/binary/reasoning/unsupported records are
// never eligible, so a previous task's final or a hidden reasoning record can
// never be transmitted as this task's final response.
// Spec: docs/specs/task-retrospective.md via Normalized
func (n *Normalized) FinalResponse() *Record {
	var last *Record
	for _, record := range n.Records {
		if !n.InInterval(record) {
			continue
		}
		if record.Kind != RecordAssistantMessage && record.Kind != RecordAssistantFinal {
			continue
		}
		if record.Hidden || record.Binary || record.Reasoning || record.Unsupported {
			continue
		}
		last = record
	}
	return last
}

// InInterval reports whether the record sequence belongs to the resolved task
// interval. Requirement records are always within scope of stage two.
// Spec: docs/specs/task-retrospective.md via Normalized
func (n *Normalized) InInterval(record *Record) bool {
	if record.Kind == RecordRequirement {
		return true
	}
	if record.Seq < 0 || record.Seq <= n.IntervalStart {
		return false
	}
	return n.IntervalEnd <= 0 || record.Seq <= n.IntervalEnd
}

// Normalize validates the trace and resolves the task interval, the record
// and step ordering (with deduplication), and the completeness reasons.
// Spec: docs/specs/task-retrospective.md
func Normalize(trace *Trace) (*Normalized, error) {
	if trace == nil {
		return nil, fmt.Errorf("trace is required")
	}
	if trace.SchemaVersion != TraceSchemaVersion {
		return nil, fmt.Errorf("unsupported trace schema_version %d (want %d)", trace.SchemaVersion, TraceSchemaVersion)
	}
	ids := trace.Identities
	if ids.RootSessionID == "" || ids.SessionID == "" || ids.ExecutionRoot == "" {
		return nil, fmt.Errorf("trace identities root_session_id, session_id and execution_root are required")
	}
	if ids.RootSessionID != ids.SessionID {
		return nil, fmt.Errorf("trace must carry the root session (root_session_id %q != session_id %q)", ids.RootSessionID, ids.SessionID)
	}
	switch trace.TerminalStatus {
	case "", TaskStatusOK, TaskStatusAborted, TaskStatusError:
	default:
		return nil, fmt.Errorf("invalid trace terminal_status %q", trace.TerminalStatus)
	}
	normalized := &Normalized{Trace: trace, IntervalStart: -1}
	state := &normalization{normalized: normalized, seen: map[string]string{}}
	if err := collect(trace, 0, state); err != nil {
		return nil, err
	}
	sort.SliceStable(normalized.Records, func(i, j int) bool {
		if normalized.Records[i].Seq != normalized.Records[j].Seq {
			return normalized.Records[i].Seq < normalized.Records[j].Seq
		}
		return normalized.Records[i].ID < normalized.Records[j].ID
	})
	if err := resolveInterval(normalized); err != nil {
		return nil, err
	}
	resolveCompleteness(normalized)
	buildSteps(normalized)
	return normalized, nil
}

// normalization accumulates records while walking the trace tree.
// Spec: docs/specs/task-retrospective.md via Normalize
type normalization struct {
	normalized *Normalized
	seen       map[string]string // stable record id -> kind
}

// register records the first occurrence of an id; a repeated id with the same
// kind deduplicates, a collision with a different kind fails closed.
// Spec: docs/specs/task-retrospective.md via Normalize
func (state *normalization) register(id, kind string) (isNew bool, err error) {
	if id == "" {
		return false, fmt.Errorf("record without stable id")
	}
	previous, seen := state.seen[id]
	if seen {
		if previous != kind {
			return false, fmt.Errorf("record id %q declared with conflicting kinds %q and %q", id, previous, kind)
		}
		state.normalized.Deduplicated++
		return false, nil
	}
	state.seen[id] = kind
	return true, nil
}

// walk appends one trace's records with the declared_root collected session ids.
// Spec: docs/specs/task-retrospective.md via Normalize
func walk(trace *Trace, depth int, sessionID string, state *normalization) error {
	if depth > MaxDescendantDepth {
		return fmt.Errorf("descendant trace nesting exceeds %d", MaxDescendantDepth)
	}
	for i := range trace.Messages {
		m := &trace.Messages[i]
		if err := state.validateMessage(m); err != nil {
			return err
		}
		// Messages share one registration kind so a repeated id deduplicates
		// across roles; the record itself still carries the role-specific kind.
		// A call or requirement reusing the id remains a kind collision.
		isNew, err := state.register(m.ID, RecordUserMessage)
		if err != nil {
			return err
		}
		if !isNew {
			continue
		}
		state.normalized.Records = append(state.normalized.Records, messageRecord(m, sessionID))
	}
	for i := range trace.Calls {
		c := &trace.Calls[i]
		if err := state.validateCall(c); err != nil {
			return err
		}
		isNew, err := state.register(c.ID, RecordToolCall)
		if err != nil {
			return err
		}
		if !isNew {
			continue
		}
		state.normalized.Records = append(state.normalized.Records, callRecord(c, sessionID))
	}
	if sessionID == state.normalized.Trace.Identities.RootSessionID {
		for i := range trace.Requirements {
			r := &trace.Requirements[i]
			if r.ID == "" {
				return fmt.Errorf("requirement without stable id")
			}
			isNew, err := state.register(r.ID, RecordRequirement)
			if err != nil {
				return err
			}
			if !isNew {
				continue
			}
			state.normalized.Requirements = append(state.normalized.Requirements, &Record{
				ID:        r.ID,
				Kind:      RecordRequirement,
				SessionID: sessionID,
				Text:      r.Text,
			})
		}
	}
	for _, child := range trace.Descendants {
		if child == nil {
			return fmt.Errorf("nil descendant trace in session %q", sessionID)
		}
		if child.Identities.SessionID == state.normalized.Trace.Identities.RootSessionID {
			return fmt.Errorf("descendant trace %q duplicate root session id", child.Identities.SessionID)
		}
		if err := walk(child, depth+1, child.Identities.SessionID, state); err != nil {
			return err
		}
	}
	return nil
}

// collect walks the root trace.
// Spec: docs/specs/task-retrospective.md via Normalize
func collect(trace *Trace, depth int, state *normalization) error {
	return walk(trace, depth, trace.Identities.SessionID, state)
}

// validateMessage validates one message record.
// Spec: docs/specs/task-retrospective.md via Normalize
func (state *normalization) validateMessage(m *Message) error {
	if m.Seq < 0 {
		return fmt.Errorf("message %q has invalid seq %d", m.ID, m.Seq)
	}
	if m.Role == "context" {
		if m.Kind != "synthetic" {
			return fmt.Errorf("context message has unsupported source kind")
		}
		return nil
	}
	switch m.Role {
	case "user", "assistant":
	default:
		return fmt.Errorf("message %q has invalid role %q", m.ID, m.Role)
	}
	switch m.Kind {
	case "", "final", "action", "reasoning", "unsupported":
	default:
		return fmt.Errorf("message %q has invalid kind %q", m.ID, m.Kind)
	}
	if m.Role == "user" && m.Kind != "" && m.Kind != "message" {
		return fmt.Errorf("user message %q carries assistant-only kind %q", m.ID, m.Kind)
	}
	return nil
}

// messageRecord converts one message to a normalized record.
// Spec: docs/specs/task-retrospective.md via Normalize
func messageRecord(m *Message, sessionID string) *Record {
	kind := m.Kind
	if kind == "" {
		kind = "message"
	}
	record := &Record{
		ID: m.ID, Seq: m.Seq, SessionID: sessionID,
		Text: m.Text, Hidden: m.Hidden, Binary: m.Binary, Env: m.Env,
	}
	switch {
	case m.Role == "context":
		record.Kind = RecordSyntheticContext
	case m.Role == "user":
		record.Kind = RecordUserMessage
	case kind == "final":
		record.Kind = RecordAssistantFinal
	case kind == "reasoning":
		record.Kind = RecordAssistantMessage
		record.Reasoning = true
	case kind == "unsupported":
		record.Kind = RecordAssistantMessage
		record.Unsupported = true
	default:
		record.Kind = RecordAssistantMessage
	}
	return record
}

// validateCall validates one call record.
// Spec: docs/specs/task-retrospective.md via Normalize
func (state *normalization) validateCall(c *CallRecord) error {
	if c.Seq < 0 {
		return fmt.Errorf("call %q has invalid seq %d", c.ID, c.Seq)
	}
	if c.Tool == "" {
		return fmt.Errorf("call %q has empty tool", c.ID)
	}
	switch c.Status {
	case "", StatusSuccess, StatusError, StatusAborted, StatusFailed, StatusCancelled, StatusRunning:
	default:
		return fmt.Errorf("call %q has invalid status %q", c.ID, c.Status)
	}
	return nil
}

// callRecord converts one tool call to a normalized record.
// Spec: docs/specs/task-retrospective.md via Normalize
func callRecord(c *CallRecord, sessionID string) *Record {
	status := c.Status
	if status == "" {
		status = StatusRunning
	}
	return &Record{
		ID: c.ID, Kind: RecordToolCall, Seq: c.Seq, SessionID: sessionID,
		Tool: c.Tool, Status: status, ExitCode: c.ExitCode,
		Input: c.Input, InputOmitted: c.InputOmitted, Result: c.Result, Env: c.Env,
		Prereq: c.Prereq, SourceRefs: c.SourceRefs,
	}
}

// resolveInterval resolves the task interval from the host task id, the last
// accepted-success boundary, or the session start.
// Spec: docs/specs/task-retrospective.md via Normalize
func resolveInterval(n *Normalized) error {
	if task := n.Trace.TaskInterval; task != nil {
		if task.ID == "" {
			return fmt.Errorf("trace task.id is required when a task interval is supplied")
		}
		n.IntervalStart = task.StartSeq - 1
		n.IntervalEnd = task.EndSeq
		n.BoundarySource = "host"
		return nil
	}
	accepted := -1
	for _, boundary := range n.Trace.Boundaries {
		switch boundary.Kind {
		case "accepted_success":
			if boundary.AfterSeq > accepted {
				accepted = boundary.AfterSeq
			}
		case "task_start":
		default:
			return fmt.Errorf("invalid trace boundary kind %q", boundary.Kind)
		}
	}
	maxSeq := n.IntervalEnd
	for _, record := range n.Records {
		if record.Seq > maxSeq {
			maxSeq = record.Seq
		}
	}
	if accepted >= 0 {
		n.IntervalStart, n.IntervalEnd = accepted, maxSeq
		n.BoundarySource = "accepted-success"
		return nil
	}
	n.IntervalStart, n.IntervalEnd = -1, maxSeq
	n.BoundarySource = "session-start"
	return nil
}

// resolveCompleteness computes the material incompleteness reasons of the
// interval. Every listed reason makes the interval an incomplete trace that is
// never a successful complete trace.
// Spec: docs/specs/task-retrospective.md via Normalize
func resolveCompleteness(n *Normalized) {
	ambiguous := 0
	for _, boundary := range n.Trace.Boundaries {
		if boundary.Kind != "task_start" {
			continue
		}
		if boundary.AfterSeq > n.IntervalStart && (n.IntervalEnd <= 0 || boundary.AfterSeq <= n.IntervalEnd) {
			ambiguous++
		}
	}
	if ambiguous > 1 {
		n.Reasons = append(n.Reasons, CompletenessReason{Kind: ReasonAmbiguousBoundary, Detail: "multiple task_start boundaries inside the resolved task interval"})
	}
	sessions := map[string]bool{}
	var visit func(trace *Trace)
	visit = func(trace *Trace) {
		sessions[trace.Identities.SessionID] = true
		for _, child := range trace.Descendants {
			visit(child)
		}
	}
	visit(n.Trace)
	for _, ref := range n.Trace.ChildRefs {
		if !sessions[ref] {
			n.Reasons = append(n.Reasons, CompletenessReason{Kind: ReasonMissingChild, Detail: "linked child trace not available: " + ref})
		}
	}
	for _, gap := range n.Trace.Gaps {
		switch gap.Kind {
		case "compacted":
			n.Reasons = append(n.Reasons, CompletenessReason{Kind: ReasonCompacted, Detail: gap.Detail})
		default:
			n.Reasons = append(n.Reasons, CompletenessReason{Kind: "collection_gap", Detail: gap.Kind + ": " + gap.Detail})
		}
	}
	running := 0
	for _, record := range n.Records {
		if n.InInterval(record) && record.Status == StatusRunning {
			running++
		}
	}
	if running > 0 {
		n.Reasons = append(n.Reasons, CompletenessReason{Kind: ReasonRunningTool, Detail: "unresolved running tool calls inside the task interval"})
	}
	if n.FinalResponse() == nil {
		n.Reasons = append(n.Reasons, CompletenessReason{Kind: ReasonMissingFinal, Detail: "no assistant response inside the resolved task interval"})
	}
	if status := n.Trace.TerminalStatus; status == TaskStatusAborted || status == TaskStatusError {
		n.Reasons = append(n.Reasons, CompletenessReason{Kind: ReasonTerminalAbortError, Detail: "task terminated with status " + status})
	}
}

// buildSteps derives the stable ordered step list from the interval records:
// terminal tool attempts with paired input/result or non-final visible
// assistant action messages; excluding user messages, the final response,
// hidden reasoning, binary and unsupported records.
// Spec: docs/specs/task-retrospective.md via Normalize
func buildSteps(n *Normalized) {
	for _, record := range n.Records {
		if !n.InInterval(record) {
			continue
		}
		switch {
		case record.Kind == RecordToolCall && record.Terminal() && record.Paired():
			n.Steps = append(n.Steps, &Step{ID: record.ID, Kind: StepToolAttempt, Record: record})
		case record.Kind == RecordAssistantMessage && !record.Reasoning && !record.Hidden && !record.Binary && !record.Unsupported:
			n.Steps = append(n.Steps, &Step{ID: record.ID, Kind: StepAction, Record: record})
		}
	}
}
