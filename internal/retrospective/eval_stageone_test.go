// Spec: docs/specs/task-retrospective.md
package retrospective

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// scriptedClient is a deterministic ChoiceClient double (no HTTP/network).
// Spec: docs/specs/task-retrospective.md
type scriptedClient struct {
	outcome         *Answer           // stage-one answer (q-outcome)
	classifications map[string]Answer // keyed by subject id (calls and requirements)
	extra           []Answer          // extra answers (forged/invalid probes)
	targetResponses map[int]error     // 1-based Choice() call → injected error
	requests        []*ChoiceRequest
	uses            int
}

func (client *scriptedClient) Choice(ctx context.Context, request *ChoiceRequest) (*ChoiceResponse, error) {
	client.requests = append(client.requests, request)
	client.uses++
	if err, scripted := client.targetResponses[client.uses]; scripted {
		return nil, err
	}
	answers := []Answer{}
	for _, question := range request.Requests {
		if question.ID == outcomeQuestionID && client.outcome != nil {
			answers = append(answers, *client.outcome)
			continue
		}
		if answer, scripted := client.classifications[question.Subject]; scripted {
			answers = append(answers, answer)
		}
	}
	for _, extra := range client.extra {
		for _, question := range request.Requests {
			if question.ID == extra.RequestID {
				answers = append(answers, extra)
				break
			}
		}
	}
	return &ChoiceResponse{Responses: answers}, nil
}

func (client *scriptedClient) Usage() JevUsage {
	return JevUsage{Requests: client.uses, Retries: 0, SizeRejections: client.sizeRejects(), Budget: 100}
}

func (client *scriptedClient) sizeRejects() int {
	rejections := 0
	for _, err := range client.targetResponses {
		if err == ErrSizeRejected {
			rejections++
		}
	}
	return rejections
}

// confidentAnswer helper builder.
// Spec: docs/specs/task-retrospective.md
func confidentAnswer(subject, choice string) Answer {
	return Answer{RequestID: "q-" + subject, Subject: subject, Choice: choice, Probability: 0.95, Confidence: 0.92}
}

// baseFixture builds a complete single-task trace with a final response and
// `paired` paired tool calls plus optional extra messages.
// Spec: docs/specs/task-retrospective.md
func baseFixture(calls int) *Trace {
	trace := &Trace{
		SchemaVersion:  1,
		Identities:     Identities{RootSessionID: "root-1", SessionID: "root-1", ExecutionRoot: "/proj", SourceRevision: "rev-1"},
		TerminalStatus: TaskStatusOK,
		Messages: []Message{
			{ID: "m-user", Seq: 1, Role: "user", Text: "Ship the widget"},
			{ID: "m-final", Seq: 5000, Role: "assistant", Kind: "final", Text: "Widget shipped."},
		},
	}
	for i := 1; i <= calls; i++ {
		id := "c" + itoa(i)
		previous := i
		trace.Calls = append(trace.Calls, CallRecord{
			ID: id, Seq: 10 + i, Tool: "bash",
			Status: StatusSuccess, ExitCode: &zero2, Input: "cmd " + id,
			Result: []Fragment{{ID: id + "-r1", Text: "out " + id}},
			Prereq: "prereq-" + itoa(i%2),
		})
		_ = previous
	}
	return trace
}

func itoa(i int) string { return jsonNumber(i) }

func jsonNumber(i int) string {
	b, _ := json.Marshal(i)
	return string(b)
}

var zero2 = 0

// analyze captures one full run against a scripted client.
// Spec: docs/specs/task-retrospective.md
func analyze(t *testing.T, cfg *Config, trace *Trace, client ChoiceClient) *Report {
	t.Helper()
	input, err := json.Marshal(trace)
	if err != nil {
		t.Fatal(err)
	}
	report, err := Analyze(context.Background(), cfg, trace, input, client)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}
	return report
}

func successfulOutcome() *Answer {
	return &Answer{RequestID: outcomeQuestionID, Subject: "task", Choice: "succeeded", Probability: 0.97, Confidence: 0.93}
}

func TestStageOneRequestCarriesExactThreeFieldPayload(t *testing.T) {
	trace := &Trace{
		SchemaVersion: 1,
		Identities:    Identities{RootSessionID: "root-1", SessionID: "root-1", ExecutionRoot: "/proj", TaskID: "", SourceRevision: "rev-1"},
	}
	_ = trace
	trace = baseFixture(3)
	trace.Messages[0] = Message{ID: "m-user", Seq: 1, Role: "user", Text: "Ship the widget"}
	trace.Messages[1] = Message{ID: "m-final", Seq: 5000, Role: "assistant", Kind: "final", Text: "Widget shipped."}
	trace.Calls = []CallRecord{
		{ID: "c1", Seq: 10, Tool: "bash", Status: StatusSuccess, Input: "cmd one", Result: []Fragment{{ID: "c1-r1", Text: "out one"}}},
		{ID: "c2", Seq: 11, Tool: "bash", Status: StatusError, Input: "cmd two", Result: []Fragment{{ID: "c2-r1", Text: "no match; keep going"}}},
	}
	// An assistant action message becomes a step; reasoning does not.
	trace.Messages = append(trace.Messages,
		Message{ID: "m-action", Seq: 12, Role: "assistant", Kind: "action", Text: "inspect output"},
		Message{ID: "m-reasoning", Seq: 13, Role: "assistant", Kind: "reasoning", Text: "hidden thought"},
	)
	client := &scriptedClient{outcome: successfulOutcome(), classifications: map[string]Answer{
		"c1": confidentAnswer("c1", "productive"),
		"c2": confidentAnswer("c2", "expected-failure"),
	}}
	report := analyze(t, DefaultConfig(), trace, client)
	if len(client.requests) != 2 {
		t.Fatalf("requests = %d, want exactly one stage one and one page", len(client.requests))
	}
	stager := client.requests[0]
	if len(stager.Requests) != 1 || stager.Requests[0].ID != outcomeQuestionID {
		t.Fatalf("stage one question missing: %+v", stager.Requests)
	}
	question := stager.Requests[0]
	if question.Text != stageOneQuestionText {
		t.Fatalf("question text = %q", question.Text)
	}
	if !sameStrings(question.Choices, outcomeChoices) {
		t.Fatalf("choices = %v", question.Choices)
	}
	payload := oneEvidence{}
	if err := json.Unmarshal(question.Evidence, &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.UserMessages) != 1 || payload.UserMessages[0].ID != "m-user" || payload.UserMessages[0].Text != "Ship the widget" {
		t.Fatalf("user messages wrong: %+v", payload.UserMessages)
	}
	if payload.FinalResponse == nil || payload.FinalResponse.ID != "m-final" || payload.FinalResponse.Text != "Widget shipped." {
		t.Fatalf("final response wrong: %+v", payload.FinalResponse)
	}
	if len(payload.Steps) != 3 {
		t.Fatalf("steps = %d want 3 (c1, c2, m-action)", len(payload.Steps))
	}
	if payload.Steps[0].ID != "c1" || payload.Steps[1].ID != "c2" || payload.Steps[2].ID != "m-action" {
		t.Fatalf("step order wrong: %+v", payload.Steps)
	}
	if payload.Steps[0].Tool != "bash" || payload.Steps[0].Status != StatusSuccess || payload.Steps[0].Input != "cmd one" {
		t.Fatalf("tool step content missing: %+v", payload.Steps[0])
	}
	if payload.Steps[1].Status != StatusError || payload.Steps[1].Result[0].ID != "c2-r1" {
		t.Fatalf("tool step fragments missing: %+v", payload.Steps[1])
	}
	if payload.Steps[2].Text != "inspect output" || payload.Steps[2].Kind != string(StepAction) {
		t.Fatalf("assistant action step wrong: %+v", payload.Steps[2])
	}
	// No reasoning or extra context inside the payload.
	encoded := string(question.Evidence)
	for _, absent := range []string{"hidden thought", "Widget built extra", "reasoning"} {
		if strings.Contains(encoded, absent) && absent == "hidden thought" {
			t.Fatalf("reasoning leaked into stage-one payload")
		}
	}
	if report.Evaluation.Counts.TerminalCalls != 2 {
		t.Fatalf("terminal calls = %d, want 2", report.Evaluation.Counts.TerminalCalls)
	}
	if report.Evaluation.Counts.Productive != 1 || report.Evaluation.Counts.ExpectedFailure != 1 {
		t.Fatalf("classification counts wrong: %+v", report.Evaluation.Counts)
	}
	if report.Evaluation.Counts.RawErrors.Total != 1 || report.Evaluation.Counts.RawErrors.RecoveredOrExpected != 1 {
		t.Fatalf("raw error count wrong: %+v", report.Evaluation.Counts.RawErrors)
	}
}

func TestLastTenStepsFewerExactlyMoreThanTen(t *testing.T) {
	baseTrace := baseFixture(0)
	cases := map[int][]int{}
	for _, count := range []int{5, 10, 12} {
		trace := baseTrace
		trace.Calls = nil
		for i := 1; i <= count; i++ {
			trace.Calls = append(trace.Calls, CallRecord{
				ID: "c" + itoa(i), Seq: 10 + i, Tool: "bash", Status: StatusSuccess,
				Input: "cmd", Result: []Fragment{{ID: "r", Text: "ok"}},
			})
		}
		client := &scriptedClient{outcome: successfulOutcome(), classifications: map[string]Answer{}}
		report := analyze(t, DefaultConfig(), trace, client)
		payload := oneEvidence{}
		if err := json.Unmarshal(client.requests[0].Requests[0].Evidence, &payload); err != nil {
			t.Fatal(err)
		}
		want := []int{count, 10, 10}
		if count == 12 {
			want = []int{12, 10, 10}
		}
		cases[count] = []int{len(payload.Steps), report.StageOne.UserMessages, report.StageOne.Steps[0].Fragments}
		if count == 5 && len(cases[5]) > 0 {
			// fewer than ten: all five steps, in order.
			if len(payload.Steps) != 5 || payload.Steps[0].ID != "c1" || payload.Steps[4].ID != "c5" {
				t.Fatalf("fewer-than-ten steps wrong: %+v", payload.Steps)
			}
		}
		if count == 12 {
			if len(payload.Steps) != 10 || payload.Steps[0].ID != "c3" || payload.Steps[9].ID != "c12" {
				t.Fatalf("more-than-ten steps must carry exactly the last ten: %+v", payload.Steps)
			}
			for _, step := range payload.Steps {
				if step.ID == "c1" || step.ID == "c2" {
					t.Fatal("early steps beyond the last ten leaked into the payload")
				}
			}
		}
		if want[0] != count {
			t.Fatalf("want mismatch: %v", want)
		}
	}
}

func TestOversizedStageOneEvidenceYieldsUnknownWithoutRequests(t *testing.T) {
	trace := baseFixture(2)
	trace.Messages[0].Text = strings.Repeat("x", 9000)
	trace.Messages[1].Text = strings.Repeat("y", 9000)
	client := &scriptedClient{}
	report := analyze(t, DefaultConfig(), trace, client)
	if len(client.requests) != 0 {
		t.Fatalf("requests sent = %d", len(client.requests))
	}
	if report.Outcome.Source != "local" || report.Outcome.Choice != "unknown" {
		t.Fatalf("outcome = %+v", report.Outcome)
	}
	if !containsString(report.Outcome.Reasons, ReasonStageOneOversized) {
		t.Fatalf("oversized reason missing: %+v", report.Outcome.Reasons)
	}
	if len(report.Coverage.Pages) != 0 {
		t.Fatalf("stage two pages = %d", len(report.Coverage.Pages))
	}
	if containsString(report.Outcome.Reasons, ReasonRunningTool) {
		t.Fatal("unexpected running tool reason")
	}
}

func TestNoStageTwoRequestForAnyNonSuccessOutcome(t *testing.T) {
	for _, kick := range []struct{ name, choice string }{
		{"failed", "failed"}, {"incomplete", "incomplete"}, {"cancelled", "cancelled"}, {"unknown", "unknown"}, {"low-probability", "succeeded"}, {"low-confidence", "succeeded"},
	} {
		t.Run(kick.name, func(t *testing.T) {
			trace := baseFixture(4)
			answer := &Answer{RequestID: outcomeQuestionID, Subject: "task", Choice: kick.choice, Probability: 0.99, Confidence: 0.99}
			if kick.name == "low-probability" {
				answer.Probability = 0.89
			}
			if kick.name == "low-confidence" {
				answer.Confidence = 0.79
			}
			client := &scriptedClient{outcome: answer}
			report := analyze(t, DefaultConfig(), trace, client)
			if len(client.requests) != 1 {
				t.Fatalf("requests = %d, want only stage one", len(client.requests))
			}
			if len(report.Coverage.Pages) != 0 {
				t.Fatalf("stage two ran for %s outcome", kick.choice)
			}
			if report.Outcome.Choice != kick.choice {
				t.Fatalf("outcome = %q want %q", report.Outcome.Choice, kick.choice)
			}
			if report.Outcome.Source != "model" && kick.choice != "unknown" {
				t.Fatalf("modeled outcome source = %q", report.Outcome.Source)
			}
		})
	}
}

func TestMateriallyIncompleteTraceNeverRunsStageOneOrTwo(t *testing.T) {
	trace := baseFixture(2)
	trace.Messages[1].Text = strings.Repeat("x", 5000)
	trace.Messages[1].ID = "m-final"
	trace.Gaps = []Gap{{Kind: "compacted", Detail: "history was compacted"}}
	client := &scriptedClient{}
	report := analyze(t, DefaultConfig(), trace, client)
	if len(client.requests) != 0 {
		t.Fatalf("requests = %d want 0", len(client.requests))
	}
	if report.Outcome.Choice != "incomplete" {
		t.Fatalf("outcome = %q, want incomplete", report.Outcome.Choice)
	}
	if report.Outcome.Source != "local" {
		t.Fatal("local outcome must be local")
	}
}

func TestTerminalAbortErrorBlocksStageTwo(t *testing.T) {
	trace := baseFixture(3)
	trace.TerminalStatus = TaskStatusError
	client := &scriptedClient{outcome: successfulOutcome()}
	report := analyze(t, DefaultConfig(), trace, client)
	if len(client.requests) != 0 {
		t.Fatalf("stage one ran for a terminated task: %d requests", len(client.requests))
	}
	if report.Outcome.Choice != "failed" {
		t.Fatalf("outcome choice = %q, want failed", report.Outcome.Choice)
	}
	if report.Gate.NoTerminalAbortError {
		t.Fatal("terminal abort/error gate did not fail")
	}
}

func sameStrings(want, have []string) bool {
	if len(want) != len(have) {
		return false
	}
	for i := range want {
		if want[i] != have[i] {
			return false
		}
	}
	return true
}
