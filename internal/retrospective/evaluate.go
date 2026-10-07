// Spec: docs/specs/task-retrospective.md
package retrospective

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// ---------------------------------------------------------------------------
// Vocabulary
// ---------------------------------------------------------------------------

// Question scopes.
const (
	ScopeOutcome             = "outcome"
	ScopeCallClassification  = "call-classification"
	ScopeRequirementOmission = "requirement-omission"
)

// outcomeQuestionID is the stable stage-one question id.
const outcomeQuestionID = "q-outcome"

// stageReserveBytes is the conservative reserve for the request envelope and
// the state framing subtracted from page budgets during packing; serialized
// bytes are measured exactly again before dispatch.
const stageReserveBytes = 1024

// overlapMarkerBytes is the small constant added to a page's cost when its
// content carries overlap entries.
const overlapMarkerBytes = 24

// Typed outcome choices of stage one, in stable order.
var outcomeChoices = []string{"succeeded", "incomplete", "failed", "cancelled", "unknown"}

// Typed per-call classification choices of stage two, in stable order.
var callClassificationChoices = []string{
	"productive", "necessary-exploration", "expected-failure",
	"avoidable-error", "wrong-direction", "redundant", "uncertain",
}

// Typed per-requirement omission choices of stage two, in stable order.
var requirementChoices = []string{"omitted-high-impact", "omitted", "present", "uncertain"}

// Stage-two choices counted as confidently avoidable when verified.
var wasteChoices = []string{"avoidable-error", "wrong-direction", "redundant"}

// Fixed deterministic question wordings.
const (
	stageOneQuestionText    = "Did the bounded task evidence end in a complete, successful task outcome? Reply with exactly one of the listed choice ids."
	callQuestionText        = "Classify the effect of this terminal tool attempt on the task. Reply with exactly one of the listed choice ids."
	requirementQuestionText = "Classify whether this explicit requirement was omitted by the task. Reply with exactly one of the listed choice ids."
)

// oneMessage is one redacted stage-one message evidence entry.
// Spec: docs/specs/task-retrospective.md via oneEvidence
type oneMessage struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

// oneFragment is one redacted result fragment of a stage-one step.
// Spec: docs/specs/task-retrospective.md via oneEvidence
type oneFragment struct {
	ID     string `json:"id"`
	Text   string `json:"text,omitempty"`
	Binary bool   `json:"binary,omitempty"`
}

// oneStep is one redacted stage-one step entry of the bounded payload.
// Spec: docs/specs/task-retrospective.md via oneEvidence
type oneStep struct {
	ID        string        `json:"id"`
	Kind      string        `json:"kind"`
	SessionID string        `json:"session_id"`
	Tool      string        `json:"tool,omitempty"`
	Status    string        `json:"status,omitempty"`
	ExitCode  *int          `json:"exit_code,omitempty"`
	Input     string        `json:"input,omitempty"`
	Text      string        `json:"text,omitempty"`
	Result    []oneFragment `json:"result,omitempty"`
}

// oneEvidence is the exact bounded stage-one payload: the three prescribed
// fields and nothing else, all redacted.
// Spec: docs/specs/task-retrospective.md
type oneEvidence struct {
	UserMessages  []oneMessage `json:"user_messages"`
	FinalResponse *oneMessage  `json:"final_assistant_response"`
	Steps         []oneStep    `json:"last_ten_steps"`
}

// ---------------------------------------------------------------------------
// Analysis state
// ---------------------------------------------------------------------------

// analysis is the running state of one Analyze invocation.
// Spec: docs/specs/task-retrospective.md via Analyze
type analysis struct {
	cfg        *Config
	redactor   *Redactor
	client     ChoiceClient
	normalized *Normalized
	inputSHA   string
	status     string

	incompleteReasons []string
	outcome           ReportOutcome
	gate              ReportGate
	stageOne          ReportStageOne
	firstOneSent      bool

	pages           []PageDelivery
	rowByRecord     map[string]*RecordDisposition
	recordRows      []ReportRecordEval
	requirementRows []ReportRequirementEval
	avoidableCalls  []string
	coverage        ReportCoverage
	evaluation      ReportEvaluation
	dispatch        ReportDispatch
	questionScope   map[string]*Question // sent question metadata by question id
	pageOfAnswer    map[string]int       // dispatched page of each answered question id
	answers         map[string]*Answer
	redaction       ReportRedaction
	contentSplits   int // records split into id-linked pieces
	sizeRepacks     int // page re-packs after size rejection
	usage           JevUsage
}

// Analyze runs the Phase 1 replay pipeline: normalization, local redaction,
// the stage-one success gate, the conditional paginated stage-two
// classification, the counting and trigger decisions, and the report.
// Spec: docs/specs/task-retrospective.md
func Analyze(ctx context.Context, cfg *Config, trace *Trace, input []byte, client ChoiceClient) (*Report, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	normalized, err := Normalize(trace)
	if err != nil {
		return nil, err
	}
	redactor, err := NewRedactor(cfg)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(input)
	run := &analysis{
		cfg:           cfg,
		redactor:      redactor,
		client:        client,
		normalized:    normalized,
		inputSHA:      hex.EncodeToString(digest[:]),
		status:        ReportStatusComplete,
		answers:       map[string]*Answer{},
		questionScope: map[string]*Question{},
		pageOfAnswer:  map[string]int{},
		rowByRecord:   map[string]*RecordDisposition{},
		usage:         JevUsage{Budget: cfg.RequestBudget},
	}
	run.runStageOne(ctx)
	run.runStageTwo(ctx)
	run.finishEvaluation()
	return run.report(), nil
}

// ---------------------------------------------------------------------------
// Stage one
// ---------------------------------------------------------------------------

// runStageOne applies the local gates and, when permitted, dispatches one
// outcome question over the exact three-field bounded evidence.
// Spec: docs/specs/task-retrospective.md via Analyze
func (a *analysis) runStageOne(ctx context.Context) {
	if !a.normalized.Complete() {
		a.outcome = ReportOutcome{
			Source:  "local",
			Choice:  localOutcomeChoice(a.normalized),
			Reasons: a.outcomeReasons(),
		}
		a.gate = a.gateState(false)
		return
	}
	payload := a.stageOnePayload()
	a.stageOne = ReportStageOne{
		Requested:    true,
		UserMessages: len(payload.UserMessages),
		Steps:        a.stageOneStepReport(),
	}
	request := buildRequest(
		EvaluatorModel(a.cfg.Evaluator),
		[]byte(`{}`),
		nil,
		[]Question{a.stageOneQuestion(payload)},
	)
	measure := measureRequest(request)
	a.stageOne.Bytes = &measure
	if measure.Request > a.cfg.RequestLimitBytes ||
		measure.State+measure.LongestQuestion > a.cfg.StateQuestionLimitBytes {
		a.outcome = ReportOutcome{Source: "local", Choice: "unknown", Reasons: []string{ReasonStageOneOversized}}
		a.gate = a.gateState(false)
		a.stageOne.Requested = false
		return
	}
	if a.client == nil {
		a.outcome = ReportOutcome{Source: "local", Choice: "unknown", Reasons: []string{"evaluator unavailable"}}
		a.gate = a.gateState(false)
		a.stageOne.Requested = false
		a.status = ReportStatusIncomplete
		a.incompleteReasons = append(a.incompleteReasons, "evaluator unavailable")
		return
	}
	a.firstOneSent = true
	response, err := a.client.Choice(ctx, request)
	if err != nil {
		a.halt("stage one dispatch failed: " + err.Error())
		a.outcome = ReportOutcome{
			Source:  "model",
			Choice:  "unknown",
			Reasons: []string{"stage one dispatch failed: " + err.Error()},
		}
		return
	}
	a.outcome = outcomeFromResponse(response)
	a.gate = a.gateState(a.stageOneSuccess())
}

// stageOneSuccess reports whether the stage-one answer passes the floors.
// Spec: docs/specs/task-retrospective.md via runStageOne
func (a *analysis) stageOneSuccess() bool {
	if a.outcome.Choice != "succeeded" || a.outcome.Probability == nil || a.outcome.Confidence == nil {
		return false
	}
	return *a.outcome.Probability >= a.cfg.MinSuccessProbability && *a.outcome.Confidence >= a.cfg.MinConfidence
}

// localOutcomeChoice maps local material reasons to a typed outcome.
// Spec: docs/specs/task-retrospective.md via runStageOne
func localOutcomeChoice(n *Normalized) string {
	for _, reason := range n.Reasons {
		if reason.Kind == ReasonTerminalAbortError {
			return "failed"
		}
	}
	return "incomplete"
}

// outcomeReasons renders the local material reasons with redacted detail
// text (gap details and provenance strings are collector free text).
// Spec: docs/specs/task-retrospective.md via runStageOne
func (a *analysis) outcomeReasons() []string {
	rows := []string{}
	for _, reason := range a.normalized.Reasons {
		rows = append(rows, reason.Kind+": "+a.redactor.Text(reason.Detail))
	}
	return rows
}

// stageOnePayload builds the exact three-field bounded payload, redacted.
// Spec: docs/specs/task-retrospective.md via runStageOne
func (a *analysis) stageOnePayload() *oneEvidence {
	payload := &oneEvidence{}
	for _, record := range a.normalized.Records {
		if !a.normalized.InInterval(record) || record.Kind != RecordUserMessage {
			continue
		}
		redacted := a.redactor.Record(record)
		payload.UserMessages = append(payload.UserMessages, oneMessage{ID: redacted.ID, Text: redacted.Text})
	}
	if final := a.normalized.FinalResponse(); final != nil {
		redacted := a.redactor.Record(final)
		payload.FinalResponse = &oneMessage{ID: redacted.ID, Text: redacted.Text}
	}
	for _, step := range a.normalized.LastSteps(a.cfg.TailSteps) {
		redacted := a.redactor.Record(step.Record)
		one := oneStep{ID: step.ID, Kind: string(step.Kind), SessionID: redacted.SessionID}
		if step.Kind == StepToolAttempt {
			one.Tool = redacted.Tool
			one.Status = redacted.Status
			one.ExitCode = redacted.ExitCode
			one.Input = redacted.Input
			for _, fragment := range redacted.Result {
				one.Result = append(one.Result, oneFragment{
					ID: fragment.ID, Text: fragment.Text, Binary: fragment.Binary,
				})
			}
		} else {
			one.Text = redacted.Text
		}
		payload.Steps = append(payload.Steps, one)
	}
	return payload
}

// stageOneStepReport returns the report-side step reference rows.
// Spec: docs/specs/task-retrospective.md via runStageOne
func (a *analysis) stageOneStepReport() []StageOneStep {
	rows := []StageOneStep{}
	for _, step := range a.normalized.LastSteps(a.cfg.TailSteps) {
		rows = append(rows, StageOneStep{
			Subject:   step.ID,
			Kind:      step.Kind,
			SessionID: step.Record.SessionID,
			Tool:      step.Record.Tool,
			Status:    step.Record.Status,
			ExitCode:  step.Record.ExitCode,
			Paired:    step.Record.Paired(),
			Fragments: len(step.Record.Result),
		})
	}
	return rows
}

// stageOneQuestion builds the single stage-one outcome question.
// Spec: docs/specs/task-retrospective.md via runStageOne
func (a *analysis) stageOneQuestion(payload *oneEvidence) Question {
	return Question{
		ID:       outcomeQuestionID,
		Subject:  "task",
		Scope:    ScopeOutcome,
		Text:     stageOneQuestionText,
		Choices:  outcomeChoices,
		Evidence: mustMarshal(payload),
	}
}

// stageIdentityState builds the identity state fragment.
// Spec: docs/specs/task-retrospective.md via runStageOne
func (a *analysis) stageIdentityState(extra map[string]any) []byte {
	trace := a.normalized.Trace
	fields := map[string]any{
		"root_session_id": trace.Identities.RootSessionID,
		"session_id":      trace.Identities.SessionID,
		"execution_root":  trace.Identities.ExecutionRoot,
		"boundary_source": a.normalized.BoundarySource,
	}
	if trace.Identities.TaskID != "" {
		fields["task_id"] = trace.Identities.TaskID
	}
	if trace.Identities.SourceRevision != "" {
		fields["source_revision"] = trace.Identities.SourceRevision
	}
	for field, value := range extra {
		fields[field] = value
	}
	return mustMarshal(fields)
}

// outcomeFromResponse validates the stage-one answer over the supplied
// choice ids; constraint violations — unknown choice ids, missing or
// out-of-range probability/confidence — abstain (unknown with the reason).
// Spec: docs/specs/task-retrospective.md via runStageOne
func outcomeFromResponse(response *ChoiceResponse) ReportOutcome {
	if response == nil || len(response.Responses) != 1 {
		return ReportOutcome{Source: "model", Choice: "unknown", Reasons: []string{"expected exactly one answer"}}
	}
	answer := response.Responses[0]
	if answer.RequestID != outcomeQuestionID {
		return ReportOutcome{Source: "model", Choice: "unknown", Reasons: []string{"answer references unknown question id " + answer.RequestID}}
	}
	if !containsString(outcomeChoices, answer.Choice) {
		return ReportOutcome{Source: "model", Choice: "unknown", Reasons: []string{"answer choice is outside the supplied ids"}}
	}
	if answer.Probability == nil || answer.Confidence == nil {
		return ReportOutcome{Source: "model", Choice: "unknown", Reasons: []string{"answer probability or confidence missing"}}
	}
	if !probabilityInRange(*answer.Probability) || !probabilityInRange(*answer.Confidence) {
		return ReportOutcome{Source: "model", Choice: "unknown", Reasons: []string{"answer probability or confidence outside [0,1]"}}
	}
	return ReportOutcome{
		Source:      "model",
		Choice:      answer.Choice,
		Probability: answer.Probability,
		Confidence:  answer.Confidence,
	}
}

// ---------------------------------------------------------------------------
// Stage two
// ---------------------------------------------------------------------------

// evidenceRecord is one redacted observable record (or a piece of one) in the
// page's record list.
// Spec: docs/specs/task-retrospective.md via stageItem
type evidenceRecord struct {
	ID           string        `json:"id"`
	Kind         string        `json:"kind"`
	SessionID    string        `json:"session_id,omitempty"`
	Tool         string        `json:"tool,omitempty"`
	Status       string        `json:"status,omitempty"`
	ExitCode     *int          `json:"exit_code,omitempty"`
	Input        string        `json:"input,omitempty"`
	InputOmitted bool          `json:"input_omitted,omitempty"`
	Text         string        `json:"text,omitempty"`
	Result       []oneFragment `json:"result,omitempty"`
	SourceRefs   []SourceRef   `json:"source_refs,omitempty"`
	PieceSource  string        `json:"piece_source,omitempty"`
	Piece        *fragRef      `json:"part,omitempty"`
	Overlapped   bool          `json:"overlapped,omitempty"`
}

// fragRef marks one piece of a split record (part/total, id-linked).
// Spec: docs/specs/task-retrospective.md via evidenceRecord
type fragRef struct {
	Part  int `json:"part"`
	Total int `json:"total"`
}

// stageItem is one packable stage-two unit.
// Spec: docs/specs/task-retrospective.md via packStageTwoPages
type stageItem struct {
	subject  string
	kind     string
	grams    []evidenceRecord
	question *Question
}

// stageTwoPage is one packed page.
// Spec: docs/specs/task-retrospective.md via runStageTwo
type stageTwoPage struct {
	items     []stageItem
	overlap   []evidenceRecord
	questions []Question
	continues []string
	stats     ReportBytes
}

// runStageTwo dispatches the paginated stage-two classification when the
// success gate held, recording coverage accounting and validated answers.
// Pages are packed under the conservative byte budgets (question envelopes
// included) and delivered in order. A locally over-budget page or an HTTP
// 413 size rejection re-packs that page with a halved content budget and
// retries. Halving repeats until the page fits or a single record cannot;
// that record is unknown (oversized-indivisible) and the rest of the run
// continues. Other failures mark the run incomplete. Budgets are restored
// after a successful page so one rejection does not permanently shrink
// later pages.
// Spec: docs/specs/task-retrospective.md via Analyze
func (a *analysis) runStageTwo(ctx context.Context) {
	if !a.gate.StageTwoRequested {
		return
	}
	items, excluded := a.stageTwoItems()
	for _, record := range excluded {
		a.setExcluded(record.ID, record.Kind, exclusionReason(record))
	}
	pending := append([]stageItem(nil), items...)
	requestBudget := a.cfg.RequestLimitBytes
	stateBudget := a.cfg.StateQuestionLimitBytes
	pageNumber := 0
	forceSingle := false
	var previousTail []evidenceRecord
	// Bound the re-pack loop. Each record can be halved a handful of times
	// and split into pieces; past that the packer is not making progress.
	guard := 0
	limit := len(items)*16 + 64
	for len(pending) > 0 {
		guard++
		if guard > limit {
			a.halt("stage two packing did not converge")
			a.markUndelivered(pending)
			return
		}
		page, remaining := a.packNextPage(pending, previousTail, requestBudget, stateBudget, forceSingle)
		if len(page.items) == 0 && len(remaining) == len(pending) {
			a.halt("stage two packing stalled")
			a.markUndelivered(pending)
			return
		}
		pending = remaining
		if len(page.items) == 0 {
			forceSingle = false
			requestBudget = a.cfg.RequestLimitBytes
			stateBudget = a.cfg.StateQuestionLimitBytes
			continue
		}
		pageNumber++
		err := a.dispatchPage(ctx, pageNumber, page)
		if isRepackableSizeError(err) {
			a.sizeRepacks++
			pageNumber--
			nextRequest, nextState := requestBudget/2, stateBudget/2
			indivisible := forceSingle || len(page.items) == 1 && (nextRequest < a.cfg.MinFragmentBytes || nextState < a.cfg.MinFragmentBytes)
			if indivisible {
				// A single record still cannot fit: name it unknown and keep
				// going. Splitting never omits the rest of the page.
				head := page.items[0]
				a.setUnknown(head.subject, head.kind, "oversized-indivisible")
				if len(page.items) > 1 {
					pending = append(append([]stageItem(nil), page.items[1:]...), pending...)
				}
				forceSingle = false
				requestBudget = a.cfg.RequestLimitBytes
				stateBudget = a.cfg.StateQuestionLimitBytes
				continue
			}
			if nextRequest < a.cfg.MinFragmentBytes || nextState < a.cfg.MinFragmentBytes {
				forceSingle = true
				requestBudget = a.cfg.MinFragmentBytes
				stateBudget = a.cfg.MinFragmentBytes
			} else {
				requestBudget, stateBudget = nextRequest, nextState
			}
			pending = append(append([]stageItem(nil), page.items...), pending...)
			continue
		}
		if err != nil {
			a.halt("stage two dispatch failed: " + err.Error())
			// The failed page's own records stay undelivered along with any
			// further pages, so the accounting names every uncovered record.
			a.markUndelivered(page.items)
			a.markUndelivered(pending)
			return
		}
		forceSingle = false
		requestBudget = a.cfg.RequestLimitBytes
		stateBudget = a.cfg.StateQuestionLimitBytes
		if last := lastItem(page); last != nil {
			previousTail = overlappedGramsOf(*last)
		}
	}
	// Every dispatch was size-rejected and nothing was delivered. Per-record
	// unknowns stay; the run is incomplete so a persistent server fault is not
	// reported as a successful local size decision.
	if len(a.pages) == 0 && a.sizeRepacks > 0 {
		a.halt("persistent size rejection")
	}
}

// overlappedGramsOf re-renders one item's recordgrams as the next page's
// overlap context entries (explicitly flagged so evaluator-side and local
// accounting deduplicate them).
// Spec: docs/specs/task-retrospective.md via runStageTwo
func overlappedGramsOf(item stageItem) []evidenceRecord {
	tail := make([]evidenceRecord, 0, len(item.grams))
	for _, gram := range item.grams {
		repeated := gram
		repeated.Overlapped = true
		tail = append(tail, repeated)
	}
	return tail
}

// lastItem returns the page's last packed item.
// Spec: docs/specs/task-retrospective.md via runStageTwo
func lastItem(page *stageTwoPage) *stageItem {
	if len(page.items) == 0 {
		return nil
	}
	return &page.items[len(page.items)-1]
}

// isRepackableSizeError reports whether a dispatch failure is a size
// rejection that the packer must handle by re-packing (splitting) instead of
// halting the run.
// Spec: docs/specs/task-retrospective.md via runStageTwo
func isRepackableSizeError(err error) bool {
	return errors.Is(err, ErrSizeRejected) || errors.Is(err, ErrContentOverBudget)
}

// stageTwoItems lists the redacted observable records (the interval's records
// including linked descendants, once each) plus every explicit requirement.
// Spec: docs/specs/task-retrospective.md via runStageTwo
func (a *analysis) stageTwoItems() ([]stageItem, []*Record) {
	items := []stageItem{}
	excluded := []*Record{}
	for _, record := range a.normalized.Records {
		if !a.normalized.InInterval(record) {
			continue
		}
		if reason := exclusionReason(record); reason != "" {
			excluded = append(excluded, record)
			continue
		}
		redacted := a.redactor.Record(record)
		item := stageItem{subject: redacted.ID, kind: redacted.Kind, grams: []evidenceRecord{gramOf(redacted)}}
		if redacted.Kind == RecordToolCall {
			item.question = a.callQuestion(redacted)
		}
		items = append(items, item)
	}
	for _, requirement := range a.normalized.Requirements {
		redacted := a.redactor.Record(requirement)
		items = append(items, stageItem{
			subject:  redacted.ID,
			kind:     redacted.Kind,
			grams:    []evidenceRecord{gramOf(redacted)},
			question: a.requirementQuestion(redacted),
		})
	}
	return items, excluded
}

// exclusionReason names why a record's content is never transmitted. A call
// whose every result fragment is binary is excluded wholesale; mixed records
// deliver with the binary fragments withheld as undecodable markers.
// Spec: docs/specs/task-retrospective.md via stageTwoItems
func exclusionReason(record *Record) string {
	switch {
	case record.Hidden:
		return "hidden-record"
	case record.Binary:
		return "binary-record"
	case record.Reasoning:
		return "private-reasoning"
	case record.Unsupported:
		return "unsupported-record"
	}
	if record.Kind == RecordToolCall && len(record.Result) > 0 {
		allBinary := true
		for _, fragment := range record.Result {
			if !fragment.Binary {
				allBinary = false
				break
			}
		}
		if allBinary {
			return "binary-record"
		}
	}
	return ""
}

// gramOf renders one redacted record as its evidence recordgram.
// Spec: docs/specs/task-retrospective.md via stageTwoItems
func gramOf(record *Record) evidenceRecord {
	gram := evidenceRecord{
		ID: record.ID, Kind: record.Kind, SessionID: record.SessionID,
		Tool: record.Tool, Status: record.Status, ExitCode: record.ExitCode,
		Input: record.Input, InputOmitted: record.InputOmitted, Text: record.Text,
	}
	for _, fragment := range record.Result {
		gram.Result = append(gram.Result, oneFragment{
			ID: fragment.ID, Text: fragment.Text, Binary: fragment.Binary,
		})
	}
	for _, ref := range record.SourceRefs {
		gram.SourceRefs = append(gram.SourceRefs, ref)
	}
	if gram.Result == nil {
		gram.Result = []oneFragment{}
	}
	if gram.SourceRefs == nil {
		gram.SourceRefs = []SourceRef{}
	}
	return gram
}

// callQuestion builds the per-call classification question.
// Spec: docs/specs/task-retrospective.md via stageTwoItems
func (a *analysis) callQuestion(record *Record) *Question {
	question := &Question{
		ID:      "q-" + record.ID,
		Subject: record.ID,
		Scope:   ScopeCallClassification,
		Text:    callQuestionText,
		Choices: callClassificationChoices,
	}
	return question
}

// requirementQuestion builds the per-requirement omission question.
// Spec: docs/specs/task-retrospective.md via stageTwoItems
func (a *analysis) requirementQuestion(record *Record) *Question {
	return &Question{
		ID:      "q-" + record.ID,
		Subject: record.ID,
		Scope:   ScopeRequirementOmission,
		Text:    requirementQuestionText,
		Choices: requirementChoices,
	}
}

// gramBytes measures the serialized bytes of one item's recordgram list.
// Spec: docs/specs/task-retrospective.md via packNextPage
func gramBytes(grams []evidenceRecord) int {
	encoded, err := json.Marshal(grams)
	if err != nil {
		return 0
	}
	return len(encoded)
}

// fitsPage checks both serialized constraints with a tentative new item by
// assembling the trial request exactly as dispatch would (recordgrams in
// state, question envelopes included) and measuring the serialized bytes.
// Spec: docs/specs/task-retrospective.md via packNextPage
func (a *analysis) fitsPage(page *stageTwoPage, item stageItem, requestBudget, stateBudget int) bool {
	trial := &stageTwoPage{}
	*trial = *page
	trial.items = append(append([]stageItem(nil), page.items...), item)
	trial.questions = page.questions
	if item.question != nil {
		trial.questions = append(append([]Question(nil), page.questions...), *item.question)
	}
	measure := a.measureTrialRequest(trial)
	if measure.Request > requestBudget {
		return false
	}
	return measure.State+measure.LongestQuestion <= stateBudget
}

// measureTrialRequest measures one tentative page's serialized request bytes
// for the budget gates (state includes the page's recordgrams). The
// provisional page number is stable; the reserve covers digit growth of the
// real dispatch numbers.
// Spec: docs/specs/task-retrospective.md via fitsPage
func (a *analysis) measureTrialRequest(page *stageTwoPage) ReportBytes {
	return measureRequest(a.buildPageRequest(1, page))
}

// packNextPage greedily packs ONE page from the head of the pending items
// under the current budgets, re-presenting the previous page's tail as the
// page's overlap. An item too large for a fresh page is split into ID-linked
// fragments (replacing its pending entry, so the caller's remaining list
// always reflects the split); a record that can never fit is recorded unknown
// (never omitted silently). forceSingle stops after one item so a size
// rejection that can no longer be halved still makes progress. It returns the
// packed page (possibly empty when every consumed item was recorded unknown)
// and the remaining pending items.
// Spec: docs/specs/task-retrospective.md via runStageTwo
func (a *analysis) packNextPage(pending []stageItem, previousTail []evidenceRecord, requestBudget, stateBudget int, forceSingle bool) (*stageTwoPage, []stageItem) {
	page := &stageTwoPage{overlap: previousTail}
	requestBudget -= pageBudgetOverhead(page)
	stateBudget -= pageBudgetOverhead(page)
	consumed := 0
	for consumed < len(pending) {
		item := pending[consumed]
		if a.fitsPage(page, item, requestBudget, stateBudget) {
			page.items = append(page.items, item)
			if item.question != nil {
				page.questions = append(page.questions, *item.question)
			}
			consumed++
			if forceSingle {
				break
			}
			continue
		}
		if len(page.items) == 0 && len(page.overlap) > 0 {
			// A previous-page tail must not block a record that fits alone.
			// Drop the overlap for this page rather than trim or omit the record.
			bare := &stageTwoPage{}
			if a.fitsPage(bare, item, requestBudget, stateBudget) {
				page.overlap = nil
				page.items = append(page.items, item)
				if item.question != nil {
					page.questions = append(page.questions, *item.question)
				}
				consumed++
				break
			}
		}
		if len(page.items) == 0 {
			// The record cannot fit an empty page: split into id-linked
			// pieces or record it unknown (never omitted silently).
			overlapBytes := 0
			if len(page.overlap) > 0 {
				overlapBytes = gramBytes(page.overlap)
			}
			pieces, reason := a.splitItem(item, requestBudget, stateBudget, overlapBytes)
			if len(pieces) == 0 {
				a.setUnknown(item.subject, item.kind, reason)
				consumed++
				continue
			}
			a.contentSplits++
			pending = append(pieces, pending[consumed+1:]...)
			consumed = 0
			continue
		}
		break // The page is full; the caller re-presents this page's tail as
		// the next page's overlap and keeps packing the same item.
	}
	return page, pending[consumed:]
}

// pageBudgetOverhead reserves the request envelope, the state framing keys,
// and small page-index growth from the content budgets. Exact bytes are
// measured again on every fit trial and before dispatch.
// Spec: docs/specs/task-retrospective.md via packNextPage
func pageBudgetOverhead(page *stageTwoPage) int {
	reserved := stageReserveBytes
	if len(page.overlap) > 0 {
		reserved += overlapMarkerBytes
	}
	return reserved
}

// splitItem cuts an oversized item's record into ID-linked pieces whose
// serialized grams each fit inside both budgets; the input (if any) and each
// result fragment's text are cut rune-safely and re-linked with part/total.
// Each piece's piece_source is the source segment covering the piece's start
// offset, so a source that spans several cuts keeps its own provenance
// instead of the cut index. overlapBytes reserves the previous page's tail
// so a piece still fits beside that overlap. Already-split pieces (content
// only in Text) are cut again from that text, keeping their piece_source.
// Spec: docs/specs/task-retrospective.md via packNextPage
func (a *analysis) splitItem(item stageItem, requestBudget, stateBudget, overlapBytes int) ([]stageItem, string) {
	gram := item.grams[0]
	base := evidenceRecord{
		ID: gram.ID, Kind: gram.Kind, SessionID: gram.SessionID,
		Tool: gram.Tool, Status: gram.Status, ExitCode: gram.ExitCode,
	}
	baseBytes, err := json.Marshal([]evidenceRecord{base})
	if err != nil {
		return nil, "oversized-indivisible"
	}
	questionBytes := 0
	if item.question != nil {
		questionBytes = len(mustMarshal(*item.question))
	}
	budget := stateBudget
	if requestBudget < budget {
		budget = requestBudget
	}
	headroom := budget - len(baseBytes) - questionBytes - pieceMarginBytes - overlapBytes
	if headroom < a.cfg.MinFragmentBytes {
		return nil, "oversized-indivisible"
	}
	content := ""
	type segment struct {
		src        string
		start, end int
	}
	segments := []segment{}
	addSource := func(src, text string) {
		if content != "" {
			content += "\u0000"
		}
		start := len(content)
		content += text
		segments = append(segments, segment{src: src, start: start, end: len(content)})
	}
	added := false
	if !gram.InputOmitted && gram.Input != "" {
		addSource("input", gram.Input)
		added = true
	}
	for _, fragment := range gram.Result {
		if fragment.Binary || fragment.Text == "" {
			continue
		}
		addSource("result:"+fragment.ID, fragment.Text)
		added = true
	}
	if !added && gram.Text != "" {
		src := gram.PieceSource
		if src == "" {
			src = "text"
		}
		addSource(src, gram.Text)
	}
	cuts := utf8Slices(content, headroom)
	if len(cuts) < 2 {
		return nil, "oversized-indivisible"
	}
	var pieces []stageItem
	offset := 0
	segmentAt := 0
	for index, piece := range cuts {
		start, end := offset, offset+len(piece)
		offset = end
		for segmentAt < len(segments) && segments[segmentAt].end <= start {
			segmentAt++
		}
		nextGram := base
		nextGram.Piece = &fragRef{Part: index + 1, Total: len(cuts)}
		if segmentAt < len(segments) {
			nextGram.PieceSource = segments[segmentAt].src
		}
		nextGram.Text = piece
		nextGram.InputOmitted = gram.InputOmitted
		question := item.question
		if index+1 < len(cuts) {
			question = nil
		}
		pieces = append(pieces, stageItem{
			subject: item.subject, kind: item.kind,
			grams:    []evidenceRecord{nextGram},
			question: question,
		})
	}
	return pieces, ""
}

// pieceMarginBytes reserves the piece's JSON field names and markers when
// computing the per-piece content headroom.
// Spec: docs/specs/task-retrospective.md via splitItem
const pieceMarginBytes = 96

// utf8Slices cuts content into at most limit-byte rune-boundary slices.
// Spec: docs/specs/task-retrospective.md via splitItem
func utf8Slices(content string, limit int) []string {
	if limit < 1 {
		limit = 1
	}
	var pieces []string
	for start := 0; start < len(content); {
		end := start + limit
		if end > len(content) {
			end = len(content)
		} else if !utf8.RuneStart(content[end]) {
			for end > start && !utf8.RuneStart(content[end]) {
				end--
			}
		}
		if end == start {
			end = start + 1
		}
		pieces = append(pieces, content[start:end])
		start = end
	}
	return pieces
}

// ---------------------------------------------------------------------------
// Coveragedispatch/accounting helpers
// ---------------------------------------------------------------------------

// setExcluded records a coverage exclusion row.
// Spec: docs/specs/task-retrospective.md via runStageTwo
func (a *analysis) setExcluded(subject, kind, reason string) {
	a.rowByRecord[subject] = &RecordDisposition{
		Subject:     subject,
		Kind:        kind,
		Disposition: "excluded:" + reason,
	}
}

// setUnknown records an unknown record (never delivered, never omitted silently).
// Spec: docs/specs/task-retrospective.md via packStageTwoPages
func (a *analysis) setUnknown(subject, kind, reason string) {
	a.rowByRecord[subject] = &RecordDisposition{
		Subject:     subject,
		Kind:        kind,
		Disposition: "unknown:" + reason,
	}
}

// rowFor returns/creates one record's coverage row (rows live in the map so
// the reported order is derived deterministically at finish time).
// Spec: docs/specs/task-retrospective.md via pageSetup
func (a *analysis) rowFor(subject, kind string) *RecordDisposition {
	if row, exists := a.rowByRecord[subject]; exists {
		return row
	}
	row := &RecordDisposition{Subject: subject, Kind: kind}
	a.rowByRecord[subject] = row
	return row
}

// buildPageRequest assembles one page's serialized request: the state
// carries the page's redacted recordgrams (`records`) explicitly, so every
// classified record's redacted content is transmitted on the page where its
// question is sent.
// Spec: docs/specs/task-retrospective.md
func (a *analysis) buildPageRequest(index int, page *stageTwoPage) *ChoiceRequest {
	fields := map[string]any{
		"page":    index,
		"overlap": overlapSubjects(page.overlap),
	}
	grams := []evidenceRecord{}
	for _, item := range page.items {
		grams = append(grams, item.grams...)
	}
	fields["records"] = grams
	return buildRequest(EvaluatorModel(a.cfg.Evaluator), a.stageIdentityState(fields), page.overlap, page.questions)
}

// overlapSubjects lists the overlap record ids of one page.
// Spec: docs/specs/task-retrospective.md via buildPageRequest
func overlapSubjects(overlap []evidenceRecord) []string {
	seen := map[string]bool{}
	ids := []string{}
	for _, record := range overlap {
		if !seen[record.ID] {
			seen[record.ID] = true
			ids = append(ids, record.ID)
		}
	}
	return ids
}

// dispatchPage serializes, measures, and delivers one page. A local
// over-budget measurement or an HTTP 413 size rejection surfaces as a
// repackable size error (ErrContentOverBudget / ErrSizeRejected); the caller
// re-packs with halved budgets instead of trimming or halting.
// Spec: docs/specs/task-retrospective.md via runStageTwo
func (a *analysis) dispatchPage(ctx context.Context, index int, page *stageTwoPage) error {
	request := a.buildPageRequest(index, page)
	measure := measureRequest(request)
	page.stats = measure
	if measure.Request > a.cfg.RequestLimitBytes || measure.State+measure.LongestQuestion > a.cfg.StateQuestionLimitBytes {
		return ErrContentOverBudget
	}
	response, err := a.client.Choice(ctx, request)
	if err != nil {
		return err
	}
	a.recordPage(index, page, response)
	return nil
}

// collectAnswer validates one answer against the known dispatched questions
// and stores the first valid occurrence; forged/invalid/duplicate deliveries
// abstain. A missing probability or confidence value also abstains (it never
// decodes as zero and is never counted).
// Spec: docs/specs/task-retrospective.md via recordPage
func (a *analysis) collectAnswer(answer Answer) string {
	question, known := a.questionScope[answer.RequestID]
	if !known {
		return ""
	}
	if _, duplicate := a.answers[answer.RequestID]; duplicate {
		return ""
	}
	if answer.Subject != question.Subject || !containsString(question.Choices, answer.Choice) ||
		answer.Probability == nil || answer.Confidence == nil ||
		!probabilityInRange(*answer.Probability) || !probabilityInRange(*answer.Confidence) {
		return ""
	}
	stored := answer
	a.answers[answer.RequestID] = &stored
	return answer.RequestID
}

// markUndelivered marks the given items as explicitly undelivered so the
// coverage accounting names every uncovered record.
// Spec: docs/specs/task-retrospective.md via runStageTwo
func (a *analysis) markUndelivered(items []stageItem) {
	for _, item := range items {
		row := a.rowFor(item.subject, item.kind)
		if row.Disposition == "" {
			row.Disposition = "undelivered"
		}
	}
}

// recordPage commits one delivered page's coverage and answer captures; only
// answers for this page's own dispatched questions are eligible.
// Spec: docs/specs/task-retrospective.md via dispatchPage
func (a *analysis) recordPage(index int, page *stageTwoPage, response *ChoiceResponse) {
	for _, question := range page.questions {
		copied := question
		a.questionScope[copied.ID] = &copied
	}
	delivered := []string{}
	for _, item := range page.items {
		row := a.rowFor(item.subject, item.kind)
		row.Covered = true
		row.Deliveries++
		row.Pieces++
		row.Disposition = "covered"
		if !containsInt(row.Pages, index) {
			row.Pages = append(row.Pages, index)
		}
		delivered = append(delivered, item.subject)
	}
	overlapped := []string{}
	for _, record := range page.overlap {
		overlapped = append(overlapped, record.ID)
		if row := a.rowByRecord[record.ID]; row != nil {
			row.Deliveries++
		}
	}
	answers := []string{}
	if response != nil {
		for _, answer := range response.Responses {
			if !containsString(pageQuestionIDs(page), answer.RequestID) {
				continue // forged request reference: never attributed
			}
			if accepted := a.collectAnswer(answer); accepted != "" {
				answers = append(answers, accepted)
				a.pageOfAnswer[accepted] = index
			}
		}
	}
	a.pages = append(a.pages, PageDelivery{
		Index:   index,
		Records: delivered,
		Overlap: overlapSubjects(page.overlap),
		Bytes:   page.stats,
		Answers: answers,
	})
}

// pageQuestionIDs lists the dispatched question ids of one page.
// Spec: docs/specs/task-retrospective.md via recordPage
func pageQuestionIDs(page *stageTwoPage) []string {
	ids := []string{}
	for _, question := range page.questions {
		ids = append(ids, question.ID)
	}
	return ids
}

func containsInt(values []int, needle int) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Accounting, triggers, coverage, and the report
// ---------------------------------------------------------------------------

// finishEvaluation computes the record/requirement rows, the counting block,
// the rate and trigger state, the coverage totals, the redaction summary, and
// dispatch gating.
// Spec: docs/specs/task-retrospective.md via Analyze
func (a *analysis) finishEvaluation() {
	a.finishRecordRows()
	a.finishRequirementRows()
	a.finishCounts()
	a.finishRateTrigger()
	a.finishCoverage()
	a.finishRedaction()
	a.finishDispatch()
}

// stageTwoRan reports whether at least one page was delivered.
// Spec: docs/specs/task-retrospective.md via finishRecordRows
func (a *analysis) stageTwoRan() bool { return len(a.pages) > 0 }

// finishRecordRows builds the per-call rows over the answer sheets: one
// classification per unique preparation call; unanswered or invalid answers
// abstain with a reason, uncertain and never counted.
// Spec: docs/specs/task-retrospective.md via finishEvaluation
func (a *analysis) finishRecordRows() {
	a.recordRows = nil
	for _, record := range a.normalized.Records {
		if !a.normalized.InInterval(record) || record.Kind != RecordToolCall {
			continue
		}
		row := ReportRecordEval{Subject: record.ID, Kind: record.Kind}
		if !a.stageTwoRan() {
			row.Reason = "stage two not requested"
		} else {
			answer := a.answers["q-"+record.ID]
			if answer == nil {
				// Unanswered, forged, or invalid answers never classify.
				row.Reason = "classification abstained"
			} else {
				row.Choice = answer.Choice
				row.Labels = answer.Labels
				row.Probability = answer.Probability
				row.Confidence = answer.Confidence
				row.Page = a.pageOfAnswer["q-"+record.ID]
				row.Counted = containsString(wasteChoices, answer.Choice) &&
					*answer.Probability >= a.cfg.MinSuccessProbability &&
					*answer.Confidence >= a.cfg.MinConfidence
				if !row.Counted && containsString(wasteChoices, answer.Choice) {
					row.Reason = "waste classification below the confident floors"
				}
			}
		}
		a.recordRows = append(a.recordRows, row)
	}
}

// finishRequirementRows builds the requirement omission verdict rows.
// Spec: docs/specs/task-retrospective.md via finishEvaluation
func (a *analysis) finishRequirementRows() {
	a.requirementRows = nil
	for _, requirement := range a.normalized.Requirements {
		row := ReportRequirementEval{Subject: requirement.ID}
		if !a.stageTwoRan() {
			row.Reason = "stage two not requested"
		} else {
			answer := a.answers["q-"+requirement.ID]
			if answer == nil {
				// Unanswered, forged, or invalid answers never classify.
				row.Reason = "classification abstained"
			} else {
				row.Choice = answer.Choice
				row.Probability = answer.Probability
				row.Confidence = answer.Confidence
				row.Page = a.pageOfAnswer["q-"+requirement.ID]
				row.Counted = (answer.Choice == "omitted" || answer.Choice == "omitted-high-impact") &&
					*answer.Probability >= a.cfg.MinSuccessProbability &&
					*answer.Confidence >= a.cfg.MinConfidence
				if !row.Counted && (answer.Choice == "omitted" || answer.Choice == "omitted-high-impact") {
					row.Reason = "omission below the confident floors"
				}
			}
		}
		a.requirementRows = append(a.requirementRows, row)
	}
}

// finishCounts computes the counting block of the report.
// Spec: docs/specs/task-retrospective.md via finishEvaluation
func (a *analysis) finishCounts() {
	var counts EvaluationCounts
	for _, record := range a.normalized.Records {
		if !a.normalized.InInterval(record) || record.Kind != RecordToolCall || !record.Terminal() {
			continue
		}
		counts.TerminalCalls++
		if record.RawError() {
			counts.RawErrors.Total++
		}
	}
	avoidableOnce := map[string]bool{}
	for _, row := range a.recordRows {
		if row.Choice == "" {
			// Unanswered/invalid/abstained classification: uncertain, never counted.
			counts.Uncertain++
			continue
		}
		switch row.Choice {
		case "productive":
			counts.Productive++
		case "necessary-exploration":
			counts.NecessaryExploration++
		case "expected-failure":
			counts.ExpectedFailure++
		case "avoidable-error":
			counts.AvoidableErrors++
		case "wrong-direction":
			counts.WrongDirection++
		case "redundant":
			counts.Redundant++
		}
		if avoidableOnce == nil {
			avoidableOnce = map[string]bool{}
		}
		if row.Counted {
			if !avoidableOnce[row.Subject] {
				avoidableOnce[row.Subject] = true
				counts.Avoidable.Verified++
				a.avoidableCalls = append(a.avoidableCalls, row.Subject)
			}
		} else if containsString(wasteChoices, row.Choice) {
			// Waste class present but without confident probability/confidence.
			counts.Avoidable.Withheld++
		}
	}
	for _, row := range a.requirementRows {
		if row.Counted {
			counts.Requirements.Verified++
		} else {
			counts.Requirements.Withheld++
		}
	}
	for _, record := range a.normalized.Records {
		if !a.normalized.InInterval(record) || record.Kind != RecordToolCall || !record.RawError() {
			continue
		}
		row := a.classificationRow(record.ID)
		switch {
		case row.Counted:
			counts.RawErrors.Avoidable++
		case row.Choice == "productive" || row.Choice == "necessary-exploration" || row.Choice == "expected-failure":
			counts.RawErrors.RecoveredOrExpected++
		default:
			counts.RawErrors.Unclassified++
		}
	}
	a.evaluation.Counts = counts
}

// classificationRow returns the classification row for one record id.
// Spec: docs/specs/task-retrospective.md via finishCounts
func (a *analysis) classificationRow(subject string) ReportRecordEval {
	for _, row := range a.recordRows {
		if row.Subject == subject {
			return row
		}
	}
	return ReportRecordEval{}
}

// ---------------------------------------------------------------------------
// Remaining helpers and assembly
// ---------------------------------------------------------------------------

// halt marks the run incomplete with a reason; no further dispatch happens.
// Spec: docs/specs/task-retrospective.md via Analyze
func (a *analysis) halt(reason string) {
	a.status = ReportStatusIncomplete
	a.incompleteReasons = append(a.incompleteReasons, reason)
	a.incompleteReasons = dedupeStrings(a.incompleteReasons)
}

// containsString reports membership in a stable list.
// Spec: docs/specs/task-retrospective.md
func containsString(list []string, needle string) bool {
	for _, item := range list {
		if item == needle {
			return true
		}
	}
	return false
}

// dedupeStrings keeps first occurrences in order.
// Spec: docs/specs/task-retrospective.md
func dedupeStrings(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	return out
}

// mustMarshal encodes a value, falling back to JSON null on failure.
// Spec: docs/specs/task-retrospective.md
func mustMarshal(value any) json.RawMessage {
	encoded, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage("null")
	}
	return encoded
}

// probabilityInRange validates one probability/confidence value.
// Spec: docs/specs/task-retrospective.md
func probabilityInRange(value float64) bool { return value >= 0 && value <= 1 }

// finishRateTrigger computes the avoidable rate and the trigger decision.
// Rate = confident avoidable unique calls / all terminal calls (unknown
// classifications stay in the denominator, so the value is an explicit lower
// bound); rate trigger: N>=20 with rate>0.05 (exactly 5% does not trigger);
// exceptions: three verified redundant attempts with unchanged prerequisite
// fingerprints, or one verified high-impact explicit-requirement breach.
// Spec: docs/specs/task-retrospective.md via finishEvaluation
func (a *analysis) finishRateTrigger() {
	counts := a.evaluation.Counts
	rate := RateReport{
		Numerator:   counts.Avoidable.Verified,
		Denominator: counts.TerminalCalls,
		LowerBound:  counts.Avoidable.Withheld > 0 || counts.Uncertain > 0,
	}
	if counts.TerminalCalls > 0 {
		rate.Value = float64(counts.Avoidable.Verified) / float64(counts.TerminalCalls)
	}
	a.evaluation.Rate = rate

	rateTrigger := RateTriggerState{
		Value:     rate.Value,
		Calls:     counts.TerminalCalls,
		Avoidable: counts.Avoidable.Verified,
		MinCalls:  a.cfg.MinCountForRate,
		MinRate:   a.cfg.RateThreshold,
	}
	rateTrigger.Met = counts.TerminalCalls >= a.cfg.MinCountForRate && rate.Value > a.cfg.RateThreshold

	redundant := RedundantTriggerState{Required: a.cfg.RedundantAttemptsRequired, PrereqState: "unrecorded"}
	var redundantCalls []string
	var redundantPrereqs []string
	for _, row := range a.recordRows {
		if row.Counted && row.Choice == "redundant" {
			redundantCalls = append(redundantCalls, row.Subject)
		}
	}
	if len(redundantCalls) >= a.cfg.RedundantAttemptsRequired {
		for _, id := range redundantCalls {
			redundantPrereqs = append(redundantPrereqs, a.prereqOf(id))
		}
		unchanged := true
		unrecorded := false
		for _, prereq := range redundantPrereqs {
			if prereq == "" {
				unrecorded = true
				unchanged = false
			} else if prereq != redundantPrereqs[0] {
				unchanged = false
			}
		}
		switch {
		case unrecorded:
			redundant.PrereqState = "unrecorded"
		case unchanged:
			redundant.PrereqState = "unchanged"
			redundant.Verified = true
			redundant.Met = true
		default:
			redundant.PrereqState = "varied"
		}
		redundant.AttemptCalls = redundantCalls
	}

	breach := BreachTriggerState{}
	for _, row := range a.requirementRows {
		if row.Choice == "omitted-high-impact" && row.Counted {
			breach.Met = true
			breach.Verified = true
			breach.Requirement = row.Subject
		}
	}

	var via []string
	if rateTrigger.Met {
		via = append(via, "rate")
	}
	if redundant.Met {
		via = append(via, "redundant_exception")
	}
	if breach.Met {
		via = append(via, "high_impact_breach")
	}
	a.evaluation.Trigger = TriggerReport{
		Eligible:   len(via) > 0,
		Via:        via,
		Rate:       rateTrigger,
		Redundant:  redundant,
		Breach:     breach,
		Thresholds: a.thresholdsReport(),
	}
}

// thresholdsReport returns the exact thresholds this run used.
// Spec: docs/specs/task-retrospective.md via finishRateTrigger
func (a *analysis) thresholdsReport() ThresholdsReport {
	return ThresholdsReport{
		MinCountForRate:           a.cfg.MinCountForRate,
		RateThreshold:             a.cfg.RateThreshold,
		MinProbability:            a.cfg.MinSuccessProbability,
		MinConfidence:             a.cfg.MinConfidence,
		RedundantAttemptsRequired: a.cfg.RedundantAttemptsRequired,
	}
}

// prereqOf returns the recorded prerequisite fingerprint of one call id.
// Spec: docs/specs/task-retrospective.md via finishRateTrigger
func (a *analysis) prereqOf(id string) string {
	for _, record := range a.normalized.Records {
		if record.ID == id {
			return record.Prereq
		}
	}
	return ""
}

// finishCoverage settles the coverage totals and the ordered disposition
// rows of the report (rows built by walking the normalized records once, so
// the persisted order is deterministic).
// Spec: docs/specs/task-retrospective.md via finishEvaluation
func (a *analysis) finishCoverage() {
	rows := []RecordDisposition{}
	appendRow := func(record *Record) {
		if row, exists := a.rowByRecord[record.ID]; exists {
			rows = append(rows, *row)
		}
	}
	inInterval := 0
	for _, record := range a.normalized.Records {
		if !a.normalized.InInterval(record) {
			continue
		}
		inInterval++
		appendRow(record)
	}
	for _, requirement := range a.normalized.Requirements {
		appendRow(requirement)
	}
	coverage := ReportCoverage{
		StageOneDelivered: a.firstOneSent,
		Pages:             a.pages,
		Records:           rows,
		TotalRecords:      inInterval + len(a.normalized.Requirements),
	}
	for _, row := range rows {
		switch {
		case row.Covered:
			coverage.CoveredRecords++
		case strings.HasPrefix(row.Disposition, "excluded:"):
			coverage.ExcludedRecords++
		default:
			coverage.UndeliveredRecords++
		}
		if row.Deliveries > 1 {
			coverage.OverlapDeliveries += row.Deliveries - 1
		}
		if row.Pieces > 1 {
			coverage.SplitRecords++
		}
	}
	a.evaluation.Records = a.recordRows
	a.evaluation.Requirements = a.requirementRows
	a.evaluation.StageTwoRan = a.stageTwoRan()
	a.coverage = coverage
}

// finishRedaction computes the redaction accounting block. Redaction is
// materially incomplete (and blocks automatic correction) when binary
// records were excluded — their content cannot be certified redacted — or
// when a secret-file body is absent/unreadable (an empty excerpt from the
// collector).
// Spec: docs/specs/task-retrospective.md via finishEvaluation
func (a *analysis) finishRedaction() {
	names := []string{}
	for key := range a.redactor.Counts() {
		names = append(names, key)
	}
	sort.Strings(names)
	incomplete := false
	reasons := []string{}
	for _, row := range a.coverage.Records {
		if row.Disposition == "excluded:binary-record" {
			incomplete = true
			reasons = append(reasons, "binary record excluded: redaction cannot certify its content ("+row.Subject+")")
		}
	}
	seenSecret := map[string]bool{}
	noteSecret := func(ref SourceRef) {
		if !SecretFileBodyAbsent(ref) {
			return
		}
		key := ref.Path + "\x00" + ref.SHA256
		if seenSecret[key] {
			return
		}
		seenSecret[key] = true
		incomplete = true
		reasons = append(reasons, "secret-file body absent or unreadable: "+ref.Path)
	}
	for _, ref := range a.normalized.Trace.SourceFiles {
		noteSecret(ref)
	}
	for _, record := range a.normalized.Records {
		for _, ref := range record.SourceRefs {
			noteSecret(ref)
		}
	}
	a.redaction = ReportRedaction{
		SubstitutionsTotal:   a.redactor.TotalSubstitutions(),
		Substitutions:        a.redactor.Counts(),
		PatternNames:         names,
		ExcludedRecords:      a.coverage.ExcludedRecords,
		MateriallyIncomplete: incomplete,
		Reasons:              reasons,
	}
}

// finishDispatch computes the phase-two gating block. Eligible needs the
// trigger, complete coverage (every page delivered, every record accounted
// for), a complete run, and materially complete redaction.
// Spec: docs/specs/task-retrospective.md via finishEvaluation
func (a *analysis) finishDispatch() {
	var reasons []string
	if !a.evaluation.Trigger.Eligible {
		reasons = append(reasons, "trigger not met")
	}
	if a.status != ReportStatusComplete {
		reasons = append(reasons, "incomplete evaluation: "+strings.Join(a.incompleteReasons, "; "))
	}
	if a.coverage.UndeliveredRecords > 0 {
		reasons = append(reasons, fmt.Sprintf("%d records not delivered", a.coverage.UndeliveredRecords))
	}
	if len(a.coverage.Records) < a.coverage.TotalRecords {
		reasons = append(reasons, fmt.Sprintf("coverage incomplete: %d of %d records accounted", len(a.coverage.Records), a.coverage.TotalRecords))
	}
	if a.redaction.MateriallyIncomplete {
		reasons = append(reasons, "materially incomplete redaction blocks automatic correction: "+strings.Join(a.redaction.Reasons, "; "))
	}
	a.dispatch = ReportDispatch{Eligible: len(reasons) == 0, Reasons: reasons}
}

// report assembles the versioned report document.
// Spec: docs/specs/task-retrospective.md via Analyze
func (a *analysis) report() *Report {
	normalized := a.normalized
	trace := normalized.Trace
	evidence := []evidenceRecord{}
	for _, record := range normalized.Records {
		if normalized.InInterval(record) && exclusionReason(record) == "" {
			evidence = append(evidence, gramOf(a.redactor.Record(record)))
		}
	}
	for _, record := range normalized.Requirements {
		evidence = append(evidence, gramOf(a.redactor.Record(record)))
	}
	a.finishRedaction()
	model := ModelInfo{Model: EvaluatorModel(a.cfg.Evaluator), RubricVersion: RubricVersion}
	sourceFiles := []ReportSourceFile{}
	for _, ref := range trace.SourceFiles {
		sourceFiles = append(sourceFiles, ReportSourceFile{
			Path:       ref.Path,
			SHA256:     ref.SHA256,
			SecretFile: ref.SecretFile,
		})
	}
	gaps := []ReportGap{}
	for _, gap := range trace.Gaps {
		// Gap details are collector-supplied free text: redact before any
		// persistence so secrets in gap markers never reach the report.
		gaps = append(gaps, ReportGap{Kind: gap.Kind, Detail: a.redactor.Text(gap.Detail)})
	}
	descendants := []string{}
	var walk func(trace *Trace)
	walk = func(trace *Trace) {
		if trace.Identities.SessionID != trace.Identities.RootSessionID {
			descendants = append(descendants, trace.Identities.SessionID)
		}
		for _, child := range trace.Descendants {
			walk(child)
		}
	}
	walk(trace)
	usage := a.usage
	if a.client != nil {
		usage = a.client.Usage()
		usage.Budget = a.cfg.RequestBudget
	}
	recordRows := a.recordRows
	if recordRows == nil {
		recordRows = []ReportRecordEval{}
	}
	pages := a.pages
	if pages == nil {
		pages = []PageDelivery{}
	}
	return &Report{
		TaskEvidence:      evidence,
		SchemaVersion:     ReportSchemaVersion,
		Status:            a.status,
		IncompleteReasons: a.incompleteReasons,
		GeneratedAt:       now().UTC().Format(time.RFC3339),
		Model:             model,
		Trace: ReportTrace{
			SchemaVersion:      TraceSchemaVersion,
			InputSHA256:        a.inputSHA,
			Identities:         trace.Identities,
			TerminalStatus:     trace.TerminalStatus,
			BoundarySource:     normalized.BoundarySource,
			Interval:           ReportInterval{StartSeq: normalized.IntervalStart, EndSeq: normalized.IntervalEnd},
			Gaps:               gaps,
			SourceFiles:        sourceFiles,
			ChildRefs:          trace.ChildRefs,
			DescendantSessions: descendants,
			Deduplicated:       normalized.Deduplicated,
			Records:            len(normalized.Records),
			Steps:              len(normalized.Steps),
		},
		Outcome:    a.outcome,
		Gate:       a.gate,
		StageOne:   a.stageOne,
		Evaluation: a.evaluation,
		Coverage:   a.coverage,
		Redaction:  a.redaction,
		Usage: ReportUsage{
			Requests:       usage.Requests,
			Retries:        usage.Retries,
			SizeRejections: usage.SizeRejections,
			Splits:         a.contentSplits + a.sizeRepacks,
			Budget:         usage.Budget,
			Exhausted:      usage.Exhausted,
		},
		Dispatch: a.dispatch,
	}
}

// ---------------------------------------------------------------------------
// Gate construction
// ---------------------------------------------------------------------------

// gateState materializes the stage-two eligibility with explicit conditions:
// the model verdict, the complete bounded input, the terminal abort/error
// gate, and the evaluator availability.
// Spec: docs/specs/task-retrospective.md via runStageOne
func (a *analysis) gateState(modelSuccess bool) ReportGate {
	complete := true
	abort := true
	for _, reason := range a.normalized.Reasons {
		switch reason.Kind {
		case ReasonTerminalAbortError:
			abort = false
		default:
			complete = false
		}
	}
	return ReportGate{
		StageTwoRequested:    modelSuccess && complete && abort && a.client != nil,
		ModelSuccess:         modelSuccess,
		CompleteBoundedInput: complete,
		NoTerminalAbortError: abort,
		EvaluatorAvailable:   a.client != nil,
	}
}
