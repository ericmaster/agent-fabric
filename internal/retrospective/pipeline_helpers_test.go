// Spec: docs/specs/task-retrospective.md
package retrospective

import (
	"context"
	"encoding/json"
	"testing"
)

// Spec: docs/specs/task-retrospective.md via pipeline fixtures
func f64(v float64) *float64 { return &v }

// fixtureSignalClient is a test-only interpreted-issue fixture, not a production
// bypass. Real wire decoding has separate httptest coverage.
// Spec: docs/specs/task-retrospective.md via Analyze
type fixtureSignalClient struct {
	choice string
	calls  int
	mutate func(*ChoiceRequest, *ChoiceResponse)
}

// Spec: docs/specs/task-retrospective.md via fixtureSignalClient
func (c *fixtureSignalClient) Choice(_ context.Context, q *ChoiceRequest) (*ChoiceResponse, error) {
	c.calls++
	r := &ChoiceResponse{}
	for _, question := range q.Requests {
		choice := c.choice
		switch question.Scope {
		case ScopeCause:
			choice = "instruction"
		case ScopeSupport:
			choice = question.Choices[1]
		}
		r.Responses = append(r.Responses, Answer{RequestID: question.ID, Subject: question.Subject, Choice: choice, Probability: f64(.99), Confidence: f64(.99)})
	}
	if c.mutate != nil {
		c.mutate(q, r)
	}
	return r, nil
}

// Spec: docs/specs/task-retrospective.md via fixtureSignalClient
func (c *fixtureSignalClient) Usage() JevUsage { return JevUsage{Requests: c.calls} }

// Spec: docs/specs/task-retrospective.md via Analyze
func analyze(t *testing.T, cfg *Config, trace *Trace, client ChoiceClient) *Report {
	t.Helper()
	raw, _ := json.Marshal(trace)
	r, err := Analyze(context.Background(), cfg, trace, raw, client)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
