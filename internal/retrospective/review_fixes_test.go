// Spec: docs/specs/task-retrospective.md
package retrospective

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"unicode/utf8"
)

// TestStageTwoCarriesRedactedRecordContent records every stage-two HTTP body
// and requires each classified record's redacted content on the page that
// sends its question, and only as a marked overlap elsewhere.
// Spec: docs/specs/task-retrospective.md
func TestStageTwoCarriesRedactedRecordContent(t *testing.T) {
	trace := baseFixture(6)
	trace.Messages[0].Text = "USER-MESSAGE-MARKER"
	trace.Requirements = []Requirement{{ID: "req-1", Text: "REQUIREMENT-MARKER-OMISSION"}}
	for i := range trace.Calls {
		trace.Calls[i].Input = "PAYLOAD-" + trace.Calls[i].ID + "-END"
	}
	report, calls := runEndToEnd(t, nil, trace, nil)
	if report.Outcome.Choice != "succeeded" || !report.Evaluation.StageTwoRan {
		t.Fatalf("stage two did not run: outcome=%+v stage_two_ran=%t", report.Outcome, report.Evaluation.StageTwoRan)
	}
	stageTwo := stageTwoWires(t, calls)
	if len(stageTwo) == 0 {
		t.Fatal("no stage-two request recorded")
	}
	for _, id := range []string{"c1", "c2", "c3", "c4", "c5", "c6"} {
		assertPayloadOnQuestionPage(t, stageTwo, id, "PAYLOAD-"+id+"-END")
	}
	assertPayloadOnQuestionPage(t, stageTwo, "req-1", "REQUIREMENT-MARKER-OMISSION")
	foundUser := false
	for _, page := range stageTwo {
		for _, record := range page.State.Records {
			if strings.Contains(record.Text, "USER-MESSAGE-MARKER") && !record.Overlapped {
				foundUser = true
			}
		}
	}
	if !foundUser {
		t.Fatal("user message content missing from stage-two state.records")
	}
}

// TestManySmallCallsDeliverWithinBothLimits is the 120-call probe: question
// envelopes are counted, pages split instead of halting, and every record is
// delivered inside both byte limits.
// Spec: docs/specs/task-retrospective.md
func TestManySmallCallsDeliverWithinBothLimits(t *testing.T) {
	const calls = 120
	trace := baseFixture(0)
	trace.Messages[0].Text = "ship one hundred twenty"
	for i := 1; i <= calls; i++ {
		id := "c" + itoa(i)
		trace.Calls = append(trace.Calls, CallRecord{
			ID: id, Seq: 10 + i, Tool: "bash", Status: StatusSuccess,
			Input: "PAYLOAD-" + id + "-END", Result: []Fragment{{ID: id + "-r", Text: "ok"}},
		})
	}
	report, recorded := runEndToEnd(t, nil, trace, nil)
	if report.Status != ReportStatusComplete {
		t.Fatalf("status=%s reasons=%v undelivered=%d", report.Status, report.IncompleteReasons, report.Coverage.UndeliveredRecords)
	}
	if report.Coverage.UndeliveredRecords != 0 || report.Coverage.CoveredRecords != report.Coverage.TotalRecords {
		t.Fatalf("coverage = %+v", report.Coverage)
	}
	if len(report.Coverage.Pages) < 2 {
		t.Fatalf("expected multiple pages, got %d", len(report.Coverage.Pages))
	}
	if !report.Evaluation.StageTwoRan {
		t.Fatal("stage_two_ran not set")
	}
	stageTwo := stageTwoWires(t, recorded)
	if len(stageTwo) < 2 {
		t.Fatalf("stage-two requests = %d", len(stageTwo))
	}
	for _, call := range recorded {
		var request ChoiceRequest
		if err := json.Unmarshal(call.Body, &request); err != nil {
			t.Fatal(err)
		}
		measure := measureRequest(&request)
		if measure.Request > 24_000 || measure.State+measure.LongestQuestion > 16_000 {
			t.Fatalf("request exceeded a limit: %+v", measure)
		}
	}
	for i := 1; i <= calls; i++ {
		id := "c" + itoa(i)
		assertPayloadOnQuestionPage(t, stageTwo, id, "PAYLOAD-"+id+"-END")
	}
}

// TestSizeRejectionRepacksInsteadOfHalting returns HTTP 413 for the first
// stage-two page and requires a smaller re-pack plus full delivery.
// Spec: docs/specs/task-retrospective.md
func TestSizeRejectionRepacksInsteadOfHalting(t *testing.T) {
	report, calls := runEndToEnd(t, map[int]mockResponse{
		2: {status: http.StatusRequestEntityTooLarge},
	}, bigFixture(12, 0), nil)
	if report.Status != ReportStatusComplete {
		t.Fatalf("413 halted the run: status=%s reasons=%v", report.Status, report.IncompleteReasons)
	}
	if report.Coverage.UndeliveredRecords != 0 || report.Coverage.CoveredRecords != report.Coverage.TotalRecords {
		t.Fatalf("re-pack dropped records: %+v", report.Coverage)
	}
	if report.Usage.SizeRejections < 1 || report.Usage.Splits < 1 {
		t.Fatalf("size re-pack not accounted: %+v", report.Usage)
	}
	if len(calls) < 3 {
		t.Fatalf("expected a rejected page plus a re-pack, calls=%d", len(calls))
	}
	if len(calls[2].Body) >= len(calls[1].Body) {
		t.Fatalf("re-packed page was not smaller: rejected=%d retried=%d", len(calls[1].Body), len(calls[2].Body))
	}
	for _, call := range calls[2:] {
		var request ChoiceRequest
		if err := json.Unmarshal(call.Body, &request); err != nil {
			t.Fatal(err)
		}
		measure := measureRequest(&request)
		if measure.Request > 24_000 || measure.State+measure.LongestQuestion > 16_000 {
			t.Fatalf("re-packed request still over budget: %+v", measure)
		}
	}
}

// TestOutOfIntervalFinalDoesNotSatisfyOrTransmit rejects a previous task's
// assistant final and sends no stage-one request.
// Spec: docs/specs/task-retrospective.md
func TestOutOfIntervalFinalDoesNotSatisfyOrTransmit(t *testing.T) {
	trace := &Trace{
		SchemaVersion:  1,
		Identities:     Identities{RootSessionID: "root-1", SessionID: "root-1", ExecutionRoot: "/proj"},
		TerminalStatus: TaskStatusOK,
		TaskInterval:   &TaskInterval{ID: "task-2", StartSeq: 20},
		Messages: []Message{
			{ID: "old-final", Seq: 5, Role: "assistant", Kind: "final", Text: "PREVIOUS-TASK-FINAL"},
			{ID: "m-user", Seq: 20, Role: "user", Text: "do the new task"},
		},
		Calls: []CallRecord{{
			ID: "c1", Seq: 21, Tool: "bash", Status: StatusSuccess,
			Input: "cmd", Result: []Fragment{{ID: "c1-r", Text: "ok"}},
		}},
	}
	client := &scriptedClient{outcome: successfulOutcome()}
	report := analyze(t, DefaultConfig(), trace, client)
	if len(client.requests) != 0 {
		t.Fatalf("out-of-interval final triggered a request: %d", len(client.requests))
	}
	if report.StageOne.Requested {
		t.Fatal("stage one was requested without an in-interval final")
	}
	if !containsString(report.Outcome.Reasons, ReasonMissingFinal) && !reasonHasKind(report, ReasonMissingFinal) {
		t.Fatalf("missing-final reason absent: %+v", report.Outcome.Reasons)
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "PREVIOUS-TASK-FINAL") {
		t.Fatal("previous-task final persisted into the report evidence")
	}
}

// TestHiddenReasoningFinalIsNeverTransmitted keeps a hidden reasoning record
// out of every HTTP body, including when it is the last assistant record.
// Spec: docs/specs/task-retrospective.md
func TestHiddenReasoningFinalIsNeverTransmitted(t *testing.T) {
	t.Run("only hidden reasoning", func(t *testing.T) {
		trace := baseFixture(1)
		trace.Messages = []Message{
			{ID: "m-user", Seq: 1, Role: "user", Text: "ship"},
			{ID: "m-hidden", Seq: 50, Role: "assistant", Kind: "reasoning", Hidden: true, Text: "HIDDEN-REASONING-CANARY"},
		}
		_, calls := runEndToEnd(t, nil, trace, nil)
		if len(calls) != 0 {
			t.Fatal("hidden-reasoning-only trace sent a request")
		}
	})
	t.Run("hidden reasoning after a visible final", func(t *testing.T) {
		trace := baseFixture(2)
		trace.Messages = append(trace.Messages, Message{
			ID: "m-hidden", Seq: 6000, Role: "assistant", Kind: "reasoning", Hidden: true,
			Text: "HIDDEN-REASONING-CANARY",
		})
		report, calls := runEndToEnd(t, nil, trace, nil)
		if report.Outcome.Choice != "succeeded" {
			t.Fatalf("outcome = %+v", report.Outcome)
		}
		for _, call := range calls {
			if strings.Contains(string(call.Body), "HIDDEN-REASONING-CANARY") {
				t.Fatal("hidden reasoning was transmitted")
			}
		}
		if len(calls) == 0 {
			t.Fatal("visible final did not produce a stage-one request")
		}
		var first wireRequest
		if err := json.Unmarshal(calls[0].Body, &first); err != nil {
			t.Fatal(err)
		}
		payload := oneEvidence{}
		if err := json.Unmarshal(first.State.Evidence[outcomeQuestionID], &payload); err != nil {
			t.Fatal(err)
		}
		if payload.FinalResponse == nil || payload.FinalResponse.Text != "Widget shipped." {
			t.Fatalf("transmitted final = %+v", payload.FinalResponse)
		}
	})
}

// TestMateriallyIncompleteRedactionBlocksEligibleDispatch requires a met
// trigger plus a binary exclusion to refuse dispatch with an explicit reason.
// Spec: docs/specs/task-retrospective.md
func TestMateriallyIncompleteRedactionBlocksEligibleDispatch(t *testing.T) {
	trace := baseFixture(3)
	trace.Requirements = []Requirement{{ID: "r1", Text: "never touch prod"}}
	trace.Calls = append(trace.Calls, CallRecord{
		ID: "c-bin", Seq: 40, Tool: "bash", Status: StatusSuccess,
		Input: "dump", Result: []Fragment{{ID: "bin", Binary: true}},
	})
	client := &scriptedClient{
		outcome:         successfulOutcome(),
		classifications: map[string]Answer{"r1": confidentAnswer("r1", "omitted-high-impact")},
	}
	report := analyze(t, DefaultConfig(), trace, client)
	if !report.Evaluation.Trigger.Eligible {
		t.Fatalf("fixture did not meet a trigger: %+v", report.Evaluation.Trigger)
	}
	if !report.Redaction.MateriallyIncomplete || report.Dispatch.Eligible {
		t.Fatalf("binary exclusion did not block dispatch: redaction=%+v dispatch=%+v", report.Redaction, report.Dispatch)
	}
	joined := strings.Join(report.Dispatch.Reasons, " ")
	if !strings.Contains(joined, "materially incomplete redaction") || !strings.Contains(joined, "binary") {
		t.Fatalf("dispatch reason missing: %v", report.Dispatch.Reasons)
	}
}

// TestEmptySecretFileBodyBlocksDispatch flags an absent secret-file body,
// including a path-pattern match without the secret_file flag.
// Spec: docs/specs/task-retrospective.md
func TestEmptySecretFileBodyBlocksDispatch(t *testing.T) {
	trace := baseFixture(2)
	trace.Requirements = []Requirement{{ID: "r1", Text: "rotate credentials"}}
	trace.SourceFiles = []SourceRef{{Path: "/tmp/id_rsa", SHA256: "abc", Excerpt: "  "}}
	trace.Calls[0].SourceRefs = []SourceRef{{Path: "/proj/.env", SHA256: "def", SecretFile: true}}
	client := &scriptedClient{
		outcome:         successfulOutcome(),
		classifications: map[string]Answer{"r1": confidentAnswer("r1", "omitted-high-impact")},
	}
	report := analyze(t, DefaultConfig(), trace, client)
	if !report.Evaluation.Trigger.Eligible {
		t.Fatalf("trigger not met: %+v", report.Evaluation.Trigger)
	}
	if !report.Redaction.MateriallyIncomplete || report.Dispatch.Eligible {
		t.Fatalf("empty secret body did not block dispatch: %+v / %+v", report.Redaction, report.Dispatch)
	}
	joined := strings.Join(append(report.Redaction.Reasons, report.Dispatch.Reasons...), " ")
	if !strings.Contains(joined, "id_rsa") || !strings.Contains(joined, ".env") {
		t.Fatalf("secret-file paths missing from reasons: %s", joined)
	}
}

// TestGapDetailCanaryIsRedactedBeforePersistence keeps a secret in a gap
// detail out of the persisted report and the outcome reasons.
// Spec: docs/specs/task-retrospective.md
func TestGapDetailCanaryIsRedactedBeforePersistence(t *testing.T) {
	trace := baseFixture(1)
	trace.Gaps = []Gap{{Kind: "collector", Detail: "read failed password=CANARYSECRETVALUE"}}
	report := analyze(t, DefaultConfig(), trace, &scriptedClient{outcome: successfulOutcome()})
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "CANARYSECRETVALUE") {
		t.Fatalf("gap canary persisted: %s", encoded)
	}
	if !strings.Contains(string(encoded), "[REDACTED:secret-value]") {
		t.Fatal("gap detail was not redacted")
	}
	if len(report.Trace.Gaps) != 1 || strings.Contains(report.Trace.Gaps[0].Detail, "CANARYSECRETVALUE") {
		t.Fatalf("gap row = %+v", report.Trace.Gaps)
	}
}

// TestMissingProbabilityAbstains covers an absent probability on the wire
// (not decoded as zero) and an explicit zero, which stays in range.
// Spec: docs/specs/task-retrospective.md
func TestMissingProbabilityAbstains(t *testing.T) {
	trace := baseFixture(1)
	mock, cfg := newMockServer(t, map[int]mockResponse{
		1: {status: 200, body: `{"model":"glide","answers":{"q-outcome":{"type":"choice","choice":"succeeded","confidence":0.95}}}`},
	})
	cfg.MaxAttemptsPerRequest = 1
	client := newJev(t, cfg, nil)
	source, err := json.Marshal(trace)
	if err != nil {
		t.Fatal(err)
	}
	report, err := Analyze(context.Background(), cfg, trace, source, client)
	if err != nil {
		t.Fatal(err)
	}
	if report.Outcome.Choice != "unknown" {
		t.Fatalf("missing probability did not abstain: %+v", report.Outcome)
	}
	if len(report.Outcome.Reasons) == 0 || !strings.Contains(report.Outcome.Reasons[0], "malformed") {
		t.Fatalf("abstain reason = %v", report.Outcome.Reasons)
	}
	if report.Evaluation.StageTwoRan || len(mock.recorded()) != 1 {
		t.Fatalf("stage two ran after abstain: pages=%t calls=%d", report.Evaluation.StageTwoRan, len(mock.recorded()))
	}
	zero := 0.0
	present := outcomeFromResponse(&ChoiceResponse{Responses: []Answer{{
		RequestID: outcomeQuestionID, Subject: "task", Choice: "succeeded",
		Probability: &zero, Confidence: f64(0.91),
	}}})
	if present.Choice != "succeeded" || present.Probability == nil || *present.Probability != 0 {
		t.Fatalf("explicit zero was treated as absent: %+v", present)
	}
}

// TestPersistentSizeRejectionMarksRunIncomplete records an incomplete reason
// when every stage-two dispatch is size-rejected, and keeps the per-record
// unknowns.
// Spec: docs/specs/task-retrospective.md
func TestPersistentSizeRejectionMarksRunIncomplete(t *testing.T) {
	responses := map[int]mockResponse{}
	for index := 2; index <= 80; index++ {
		responses[index] = mockResponse{status: http.StatusRequestEntityTooLarge}
	}
	report, _ := runEndToEnd(t, responses, baseFixture(2), nil)
	if report.Status != ReportStatusIncomplete {
		t.Fatalf("status = %q, reasons = %v", report.Status, report.IncompleteReasons)
	}
	if !containsString(report.IncompleteReasons, "persistent size rejection") {
		t.Fatalf("persistent size rejection missing: %v", report.IncompleteReasons)
	}
	unknowns := 0
	for _, row := range report.Coverage.Records {
		if strings.HasPrefix(row.Disposition, "unknown:") {
			unknowns++
		}
	}
	if unknowns == 0 {
		t.Fatal("per-record unknowns were dropped")
	}
}

// TestUnknownResponseFieldIsMalformed rejects extra response fields.
// Spec: docs/specs/task-retrospective.md
func TestUnknownResponseFieldIsMalformed(t *testing.T) {
	_, cfg := newMockServer(t, map[int]mockResponse{
		1: {status: 200, body: `{"responses":[{"request_id":"q-outcome","subject":"task","choice":"succeeded","probability":0.9,"confidence":0.9}],"extra":true}`},
	})
	cfg.MaxAttemptsPerRequest = 1
	client := newJev(t, cfg, nil)
	_, err := client.Choice(context.Background(), stageSampleRequest())
	if err != ErrMalformedResponse {
		t.Fatalf("unknown field error = %v", err)
	}
}

// TestInputOmittedCallIsNotAStep and message-env redaction close the schema drift.
// Spec: docs/specs/task-retrospective.md
func TestInputOmittedCallIsNotAStep(t *testing.T) {
	trace := baseFixture(0)
	trace.Calls = []CallRecord{{
		ID: "c-omit", Seq: 3, Tool: "bash", Status: StatusSuccess,
		Input: "secret command", InputOmitted: true,
		Result: []Fragment{{ID: "r", Text: "ok"}},
	}}
	normalized := normalizeOK(t, trace)
	for _, step := range normalized.Steps {
		if step.ID == "c-omit" {
			t.Fatal("input_omitted call was treated as a paired step")
		}
	}
	if normalized.Records[len(normalized.Records)-1].Paired() {
		t.Fatal("Paired() accepted an omitted input")
	}
}

// TestMessageEnvIsRedacted applies the env rail to message records and keeps
// the secret out of any transmitted or persisted document.
// Spec: docs/specs/task-retrospective.md
func TestMessageEnvIsRedacted(t *testing.T) {
	redactor, err := NewRedactor(DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	record := &Record{
		ID: "m1", Kind: RecordUserMessage, Text: "hello",
		Env: map[string]string{"API_TOKEN": "super-secret-value", "COLOR": "blue"},
	}
	redacted := redactor.Record(record)
	if redacted.Env["API_TOKEN"] != redactEnvValue || redacted.Env["COLOR"] != "blue" {
		t.Fatalf("message env redaction = %+v", redacted.Env)
	}
	if record.Env["API_TOKEN"] != "super-secret-value" {
		t.Fatal("redactor mutated the original message env")
	}
	trace := baseFixture(1)
	trace.Messages[0].Env = map[string]string{"API_TOKEN": "super-secret-value"}
	report, calls := runEndToEnd(t, nil, trace, nil)
	for _, call := range calls {
		if strings.Contains(string(call.Body), "super-secret-value") {
			t.Fatal("message env secret transmitted")
		}
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "super-secret-value") {
		t.Fatal("message env secret persisted")
	}
}

// TestPieceSourceFollowsSourceSegment keeps provenance on the source that
// the piece's bytes came from when one source spans several cuts.
// Spec: docs/specs/task-retrospective.md
func TestPieceSourceFollowsSourceSegment(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MinFragmentBytes = 64
	cfg.RequestLimitBytes = 4_000
	cfg.StateQuestionLimitBytes = 800
	run := &analysis{cfg: cfg}
	item := stageItem{
		subject: "c1",
		kind:    RecordToolCall,
		grams: []evidenceRecord{{
			ID: "c1", Kind: RecordToolCall, Tool: "bash", Status: StatusSuccess,
			Input:  strings.Repeat("A", 3000),
			Result: []oneFragment{{ID: "r1", Text: strings.Repeat("B", 3000)}},
		}},
		question: &Question{
			ID: "q-c1", Subject: "c1", Scope: ScopeCallClassification,
			Text: "q", Choices: callClassificationChoices,
		},
	}
	pieces, reason := run.splitItem(item, 4_000, 800, 0)
	if reason != "" || len(pieces) < 3 {
		t.Fatalf("split failed: reason=%s pieces=%d", reason, len(pieces))
	}
	inputPieces := 0
	resultPieces := 0
	for _, piece := range pieces {
		text := piece.grams[0].Text
		source := piece.grams[0].PieceSource
		switch {
		case strings.Contains(text, "AAAA") && !strings.Contains(text, "BBBB"):
			if source != "input" {
				sample := text
				if len(sample) > 24 {
					sample = sample[:24]
				}
				t.Fatalf("input piece labeled %q: %q", source, sample)
			}
			inputPieces++
		case strings.Contains(text, "BBBB") && !strings.Contains(text, "AAAA"):
			if source != "result:r1" {
				t.Fatalf("result piece labeled %q", source)
			}
			resultPieces++
		}
		if !utf8.ValidString(text) {
			t.Fatal("split piece is not valid UTF-8")
		}
	}
	if inputPieces < 2 || resultPieces < 1 {
		t.Fatalf("provenance coverage input=%d result=%d pieces=%d", inputPieces, resultPieces, len(pieces))
	}
}

func stageTwoWires(t *testing.T, calls []*recordedCall) []wireRequest {
	t.Helper()
	out := []wireRequest{}
	for _, call := range calls {
		decoded := decodeWire(t, call.Body)
		if decoded.stageTwo() {
			out = append(out, decoded)
		}
	}
	return out
}

// assertPayloadOnQuestionPage requires the full marker on the page that sends
// the question. It would not match a marker split across pieces; callers use
// short unsplit records.
// Spec: docs/specs/task-retrospective.md
func assertPayloadOnQuestionPage(t *testing.T, pages []wireRequest, subject, payload string) {
	t.Helper()
	primary := 0
	for i, page := range pages {
		hasQuestion := false
		for id := range page.Questions {
			if strings.TrimPrefix(id, "q-") == subject {
				hasQuestion = true
			}
		}
		inRecords := false
		for _, record := range page.State.Records {
			if record.ID == subject && strings.Contains(record.Input+record.Text+fragmentText(record), payload) {
				inRecords = true
				if record.Overlapped {
					t.Fatalf("%s primary gram marked overlapped on page %d", subject, page.State.Page)
				}
			}
		}
		inOverlap := false
		for _, record := range page.State.Overlap {
			blob := record.Input + record.Text + fragmentText(record)
			if !strings.Contains(blob, payload) {
				continue
			}
			if !record.Overlapped {
				t.Fatalf("%s overlap content not marked on page %d", subject, page.State.Page)
			}
			inOverlap = true
		}
		if hasQuestion {
			if !inRecords {
				t.Fatalf("%s content missing from the page that sent its question (page index %d)", subject, i)
			}
			primary++
		} else if inRecords {
			t.Fatalf("%s content appeared unmarked on a page that did not send its question", subject)
		}
		_ = inOverlap
	}
	if primary != 1 {
		t.Fatalf("%s question sent on %d pages, want 1", subject, primary)
	}
}

func fragmentText(record evidenceRecord) string {
	parts := make([]string, 0, len(record.Result))
	for _, fragment := range record.Result {
		parts = append(parts, fragment.Text)
	}
	return strings.Join(parts, "")
}

func reasonHasKind(report *Report, kind string) bool {
	for _, reason := range report.Outcome.Reasons {
		if strings.HasPrefix(reason, kind) {
			return true
		}
	}
	return false
}
