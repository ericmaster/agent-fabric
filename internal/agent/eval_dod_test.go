// Spec: docs/specs/agent-fabric.md — Pre-implementation evaluation baseline
package agent

import (
	"path/filepath"
	"strings"
	"testing"
)

// These lexical policy checks guard distinct obligations, not live model obedience.
// Keep role guards separate: a declaration cannot substitute for dispatch/refusal.
func evalDoDObligations(role string) []string {
	common := []string{
		"nonempty evaluation baseline",
		"criterion IDs", "observable pass/fail predicates with expected results",
		"reproducible inspection procedures with expected observations",
		"required evidence types and locations", "Command-free criteria still require reproducible inspection",
		"inspectable immutable version/digest", "authoritative locator or inline snapshot",
		"before the first implementation mutation", "baseline identity and criteria",
		"criterion IDs to actual outcomes", "executed commands/procedures",
		"inspectable evidence locators", "evaluated candidate revision/digest",
		"authoritative predeclared inputs", "never derive acceptance from a finished patch",
		"explicit revised baseline", "traceable reason and authority",
		"applicable plan re-review before further implementation",
		"Retain original criteria and change history", "invalidate affected evidence",
		"forbid retroactive weakening to turn failure into PASS",
		"Retry preserves baseline identity and budgets",
		"Discover ordinary setup within authorized roots, not new acceptance criteria",
	}
	switch role {
	case "planner":
		return append(common,
			"During design, before review, projection or dispatch, declare",
			"in implementation packets and final reconciliation requirements",
			"Missing, empty, ambiguous or unresolvable criteria block implementation dispatch and edits, never PASS",
			"Publication in Publish mode or after the review cap does not waive these execution gates",
			"Direct atomic work may establish its baseline without a parent planner")
	case "implementor":
		// Worker reports use singular fields per result, not the supervisor's plural map.
		for i, text := range common {
			switch text {
			case "criterion IDs to actual outcomes":
				common[i] = "report each criterion ID with actual outcome"
			case "executed commands/procedures":
				common[i] = "executed command/procedure"
			case "inspectable evidence locators":
				common[i] = "inspectable evidence locator"
			case "authoritative predeclared inputs":
				common[i] = "authoritative task inputs"
			case "never derive acceptance from a finished patch":
				common[i] = "do not manufacture criteria or derive acceptance from a finished patch"
			}
		}
		return append(common,
			"Before editing, validate the supplied",
			"Missing, empty, ambiguous or unresolvable criteria: return the precise gap before editing",
			"Carry baseline identity and criteria into the handoff",
			"For direct atomic invocations, establish the baseline",
			"before mutation without requiring a parent planner",
			"Assertions or command names alone are not execution evidence",
			"Report absent, failed, stale, contradictory, skipped, deferred or unrun mandatory evidence as incomplete, never PASS",
			"QA discretion and `implementation_ready` do not bypass mandatory evidence")
	default:
		guard, acceptance := "Before phase dispatch, validate", "Before phase PASS, independently reconcile"
		if role == "loop-supervisor" {
			guard = "Before initial, retry and remediation implementation dispatch, validate"
			acceptance = "Before final PASS, independently reconcile"
		}
		return append(common, guard, acceptance,
			"Missing, empty, ambiguous or unresolvable criteria block implementation dispatch and edits, never PASS",
			"in implementation packets and final reconciliation",
			"every declared criterion and existing mandatory gate",
			"validate evidence existence, content and candidate freshness, not merely report text",
			"assertions or command names alone are not execution evidence",
			"Absent, failed, stale, contradictory, skipped, deferred or unrun mandatory evidence cannot PASS",
			"QA remains discretionary; `implementation_ready` is provisional, not acceptance, and neither bypasses mandatory evidence",
			"full failing product tests are not required before every docs/prompt edit")
	}
}

func evalDoDMissing(body string, obligations []string) []string {
	body = strings.Join(strings.Fields(body), " ")
	var missing []string
	for _, want := range obligations {
		if !strings.Contains(body, want) {
			missing = append(missing, want)
		}
	}
	return missing
}

func TestEvalDoDCanonicalRoleContracts(t *testing.T) {
	for _, role := range []string{"planner", "plan-supervisor", "loop-supervisor", "implementor"} {
		t.Run(role, func(t *testing.T) {
			d, err := ParseFile(filepath.Join("..", "..", "agents", role+".md"))
			if err != nil {
				t.Fatal(err)
			}
			// Scope to the policy section so unrelated workflow text cannot satisfy it.
			_, section, ok := strings.Cut(d.Body, "## Evaluation Baseline\n")
			if !ok {
				t.Fatal("missing evaluation baseline policy")
			}
			section, _, _ = strings.Cut(section, "\n## ")
			section = strings.Join(strings.Fields(section), " ")
			obligations := evalDoDObligations(role)
			for _, missing := range evalDoDMissing(section, obligations) {
				t.Errorf("missing EvalDoD obligation %q", missing)
			}
			for _, forbidden := range []string{
				"derive acceptance from a finished patch and proceed",
				"missing baseline permits implementation",
				"skipped mandatory evidence can PASS",
				"retry resets baseline or budgets",
			} {
				if strings.Contains(section, forbidden) {
					t.Errorf("unsafe EvalDoD exception %q", forbidden)
				}
			}
			// Negative expectations: removal of any individual obligation must be
			// detected, including each role's distinct preimplementation/PASS guard.
			for _, obligation := range obligations {
				mutated := strings.ReplaceAll(section, obligation, "[removed obligation]")
				if len(evalDoDMissing(mutated, obligations)) == 0 {
					t.Errorf("guard removal went undetected: %q", obligation)
				}
			}
		})
	}
}
