// Spec: docs/specs/task-retrospective.md
package retrospective

import "testing"

// TestExactlyFivePercentDoesNotTrigger — 20 calls with exactly one
// confidently avoidable call = 0.05 exactly → no rate trigger.
// Spec: docs/specs/task-retrospective.md
func TestExactlyFivePercentDoesNotTrigger(t *testing.T) {
	trace := baseFixture(20)
	classifications := map[string]Answer{"c1": confidentAnswer("c1", "wrong-direction")}
	for i := 2; i <= 20; i++ {
		id := "c" + itoa(i)
		classifications[id] = confidentAnswer(id, "productive")
	}
	client := &scriptedClient{outcome: successfulOutcome(), classifications: classifications}
	report := analyze(t, DefaultConfig(), trace, client)
	counts := report.Evaluation.Counts
	if counts.TerminalCalls != 20 || counts.Avoidable.Verified != 1 {
		t.Fatalf("counts = %+v", counts)
	}
	if counts.WrongDirection != 1 {
		t.Fatalf("wrong-direction count = %d", counts.WrongDirection)
	}
	rate := report.Evaluation.Rate
	if rate.Value != 0.05 || rate.Numerator != 1 || rate.Denominator != 20 {
		t.Fatalf("rate = %+v", rate)
	}
	if report.Evaluation.Trigger.Rate.Met {
		t.Fatal("exactly 5% triggered the rate rule")
	}
	if report.Evaluation.Trigger.Eligible {
		t.Fatalf("trigger eligible without exceptions: %+v", report.Evaluation.Trigger)
	}
}

// TestMoreThanFivePercentWithAtLeastTwentyCallsTriggers.
// Spec: docs/specs/task-retrospective.md
func TestMoreThanFivePercentWithAtLeastTwentyCallsTriggers(t *testing.T) {
	trace := baseFixture(21)
	classifications := map[string]Answer{
		"c1": confidentAnswer("c1", "wrong-direction"),
		"c2": confidentAnswer("c2", "redundant"),
	}
	for i := 3; i <= 21; i++ {
		id := "c" + itoa(i)
		classifications[id] = confidentAnswer(id, "productive")
	}
	client := &scriptedClient{outcome: successfulOutcome(), classifications: classifications}
	report := analyze(t, DefaultConfig(), trace, client)
	if report.Evaluation.Trigger.Rate.Met {
		return
	}
	t.Fatalf("rate trigger not met: %+v", report.Evaluation.Trigger)
}

// TestFewerThanTwentyCallsUsesOnlyExplicitExceptions — three verified
// redundant attempts with unchanged prereq fingerprints fire the exception,
// while fewer-than-20 never fires the rate rule.
// Spec: docs/specs/task-retrospective.md
func TestFewerThanTwentyCallsUsesOnlyExplicitExceptions(t *testing.T) {
	trace := baseFixture(5)
	classifications := map[string]Answer{
		"c1": confidentAnswer("c1", "redundant"),
		"c2": confidentAnswer("c2", "redundant"),
		"c3": confidentAnswer("c3", "redundant"),
	}
	for _, id := range []string{"c1", "c2", "c3"} {
		trace.Calls[idCall(trace, id)].Prereq = "prereq-same"
	}
	client := &scriptedClient{outcome: successfulOutcome(), classifications: classifications}
	report := analyze(t, DefaultConfig(), trace, client)
	trigger := report.Evaluation.Trigger
	if trigger.Rate.Met {
		t.Fatalf("rate rule fired under the call floor: %+v", trigger.Rate)
	}
	if !trigger.Redundant.Met || trigger.Redundant.PrereqState != "unchanged" || !trigger.Redundant.Verified {
		t.Fatalf("redundant exception missing: %+v", trigger.Redundant)
	}
	if !trigger.Eligible || !sameStrings(trigger.Via, []string{"redundant_exception"}) {
		t.Fatalf("trigger = %+v", trigger)
	}
}

// idCall returns the trace call index for one call id.
// Spec: docs/specs/task-retrospective.md
func idCall(trace *Trace, id string) int {
	for index, call := range trace.Calls {
		if call.ID == id {
			return index
		}
	}
	return -1
}

// TestRedundantExceptionNeedsUnrecordedPrereqsStaysBlocked — redundant
// attempts without recorded prerequisite fingerprints stay unverified.
// Spec: docs/specs/task-retrospective.md
func TestRedundantExceptionWithoutPrereqFingerprintsStaysBlocked(t *testing.T) {
	trace := baseFixture(5)
	for i := range trace.Calls {
		trace.Calls[i].Prereq = ""
	}
	client := &scriptedClient{outcome: successfulOutcome(), classifications: map[string]Answer{
		"c1": confidentAnswer("c1", "redundant"),
		"c2": confidentAnswer("c2", "redundant"),
		"c3": confidentAnswer("c3", "redundant"),
	}}
	report := analyze(t, DefaultConfig(), trace, client)
	trigger := report.Evaluation.Trigger
	if trigger.Redundant.Met || trigger.Redundant.PrereqState != "unrecorded" {
		t.Fatalf("unrecorded prereqs fired the exception: %+v", trigger.Redundant)
	}
	if trigger.Eligible {
		t.Fatalf("unverified exception still triggered: %+v", trigger)
	}
}

// TestOneVerifiedHighImpactBreachTriggersWithoutCallFloor.
// Spec: docs/specs/task-retrospective.md
func TestOneVerifiedHighImpactBreachTriggersWithoutCallFloor(t *testing.T) {
	trace := baseFixture(3)
	trace.Requirements = []Requirement{{ID: "r1", Text: "never touch the prod config"}}
	client := &scriptedClient{
		outcome:         successfulOutcome(),
		classifications: map[string]Answer{"r1": confidentAnswer("r1", "omitted-high-impact")},
	}
	report := analyze(t, DefaultConfig(), trace, client)
	trigger := report.Evaluation.Trigger
	if !trigger.Breach.Met || !trigger.Breach.Verified || trigger.Breach.Requirement != "r1" {
		t.Fatalf("breach exception missing: %+v", trigger.Breach)
	}
	if !trigger.Eligible || !sameStrings(trigger.Via, []string{"high_impact_breach"}) {
		t.Fatalf("trigger = %+v", trigger)
	}
	// The omission must never surface inside the call denominator.
	counts := report.Evaluation.Counts
	if counts.Requirements.Verified != 1 || counts.Avoidable.Verified != 0 {
		t.Fatalf("omission folded into the call denominator: %+v", counts)
	}
}

// TestDuplicateEventsDoNotInflateCounts — a repeated call id stays one
// unique terminal attempt; duplicates must not push an exactly-five-percent
// run over the threshold.
// Spec: docs/specs/task-retrospective.md
func TestDuplicateEventsDoNotInflateCounts(t *testing.T) {
	trace := baseFixture(20)
	// The same call appears twice in the trace (first occurrence wins).
	trace.Calls = append(trace.Calls, CallRecord{
		ID: "c1", Seq: 10, Tool: "bash", Status: StatusSuccess,
		Input: "cmd c1", Result: []Fragment{{ID: "c1-r1", Text: "out c1"}},
	})
	classifications := map[string]Answer{"c1": confidentAnswer("c1", "redundant")}
	for i := 2; i <= 20; i++ {
		id := "c" + itoa(i)
		classifications[id] = confidentAnswer(id, "productive")
	}
	// The redundant exception must NOT fire with a single attempt.
	client := &scriptedClient{outcome: successfulOutcome(), classifications: classifications}
	report := analyze(t, DefaultConfig(), trace, client)
	counts := report.Evaluation.Counts
	if counts.TerminalCalls != 20 || counts.Redundant != 1 {
		t.Fatalf("duplicate event inflated counts: %+v", counts)
	}
	if report.Evaluation.Trigger.Eligible {
		t.Fatalf("duplicates triggered the plan: %+v", report.Evaluation.Trigger)
	}
}

// TestSuccessfulWrongDirectionCountsAndExpectedFailureDoesNot.
// Spec: docs/specs/task-retrospective.md
func TestSuccessfulWrongDirectionCountsAndExpectedFailureDoesNot(t *testing.T) {
	trace := baseFixture(4)
	trace.Calls = append(trace.Calls, CallRecord{
		ID: "cmisfire", Seq: 40, Tool: "bash", Status: StatusSuccess,
		Input: "deploy wrong dir", Result: []Fragment{{ID: "cmisfire-r1", Text: "deployed"}},
	})
	nonzero := 1
	trace.Calls = append(trace.Calls, CallRecord{
		ID: "cred", Seq: 41, Tool: "bash", Status: StatusSuccess, ExitCode: &nonzero,
		Input: "npm test", Result: []Fragment{{ID: "cred-r1", Text: "expected no-match"}},
	})
	classifications := map[string]Answer{
		"cmisfire": confidentAnswer("cmisfire", "wrong-direction"),
		"cred":     confidentAnswer("cred", "necessary-exploration"),
	}
	client := &scriptedClient{outcome: successfulOutcome(), classifications: classifications}
	report := analyze(t, DefaultConfig(), trace, client)
	counts := report.Evaluation.Counts
	if counts.TerminalCalls != 6 {
		t.Fatalf("terminal calls = %d", counts.TerminalCalls)
	}
	if counts.WrongDirection != 1 || counts.NecessaryExploration != 1 {
		t.Fatalf("per-choice counts wrong: %+v", counts)
	}
	if counts.Avoidable.Verified != 1 {
		t.Fatalf("successful wrong-direction call not counted: %+v", counts)
	}
	// The wrong-direction row records its exact subject/served evidence.
	if report.Evaluation.Records[4].Subject != "cmisfire" || !report.Evaluation.Records[4].Counted {
		t.Fatalf("wrong-direction row not present: %+v", report.Evaluation.Records)
	}
	if counts.RawErrors.Total != 1 || counts.RawErrors.RecoveredOrExpected != 1 {
		t.Fatalf("expected/exploration outcomes leaked into avoidable waste: %+v", counts.RawErrors)
	}
}

// TestNoCallsMeansNoRateTrigger.
// Spec: docs/specs/task-retrospective.md
func TestNoCallsMeansNoRateTrigger(t *testing.T) {
	trace := &Trace{
		SchemaVersion: 1,
		Identities:    Identities{RootSessionID: "root-1", SessionID: "root-1", ExecutionRoot: "/proj"},
		Messages: []Message{
			{ID: "m-user", Seq: 1, Role: "user", Text: "did nothing"},
			{ID: "m-final", Seq: 2, Role: "assistant", Kind: "final", Text: "noop done"},
		},
	}
	client := &scriptedClient{outcome: successfulOutcome()}
	report := analyze(t, DefaultConfig(), trace, client)
	counts := report.Evaluation.Counts
	if counts.TerminalCalls != 0 || report.Evaluation.Rate.Value != 0 {
		t.Fatalf("empty call set produced counts: %+v", counts)
	}
	if report.Evaluation.Trigger.Rate.Met || report.Evaluation.Trigger.Eligible {
		t.Fatalf("zero calls triggered: %+v", report.Evaluation.Trigger)
	}
}
