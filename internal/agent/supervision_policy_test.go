// Spec: docs/specs/agent-fabric.md — Bounded episodes and discretionary QA
package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Spec: docs/specs/agent-fabric.md — Phase closure and compact planning
// These are instruction-policy regressions, not filesystem/process enforcement.
func TestPhaseClosureAndCompactPlanning(t *testing.T) {
	spec, err := os.ReadFile(filepath.Join("..", "..", "docs", "specs", "agent-fabric.md"))
	if err != nil {
		t.Fatal(err)
	}
	body := strings.Join(strings.Fields(string(spec)), " ")
	for name, wants := range map[string][]string{
		"acceptance_order": {"validate required checks on current accepted inputs, independent source review, immediate scoped commit/integration", "verified compact durable receipt, then owned-resource cleanup", "Never end phase acceptance PASS with valid uncommitted code", "Blocked mandatory gates do not authorize a commit", "docs-only work records applicable checks, not invented test PASS"},
		"receipt":          {"current accepted input hash", "source/reviewed/integration revisions", "exact commands and results", "independent review disposition", "retained/deleted inventory", "consumer closure and cleanup eligibility evidence", "Verify summary completeness against source evidence", "cumulative counters, episodes and budgets/hard caps"},
		"logs":             {"Successful COMPLETED accepted-scope raw logs", "after all consumer references are resolved and the summary is verified", "log digest/locator", "never leave dangling proof locators", "Retain full raw logs for unresolved FAIL/BLOCKED/DEFERRED", "incomplete acceptance or required external retention", "traceable full-error links", "IDLE is not command completion"},
		"resources":        {"task-owned, with no active handles or live commands, a clean tree and integrated commit ancestry", "explicit authorized discard of named content with backup/recoverable source", "Unknown ownership, dirty or unmerged work is retained", "PID/start/boot identity", "named hunks/paths", "recorded before hash", "no unrelated concurrent edits", "Never use blanket checkout, force worktree removal, reset/clean or broad deletion", "absence of callers do not establish obsolescence", "Do not erase current resources, macro-ledgers, native fixtures or budgets"},
		"planning":         {"put compact grilling decisions, rationale, important rejected alternatives and unresolved conditions in plan detail", "After verifying the complete summary, retire task-owned temporary grilling questionnaires, diagrams and raw Q&A artifacts", "no full grill-session export or per-question archive is required", "does not cover operational failure logs, third-party materials, foreign artifacts"},
	} {
		t.Run(name, func(t *testing.T) {
			for _, want := range wants {
				if !strings.Contains(body, want) {
					t.Errorf("missing shared closure invariant %q", want)
				}
			}
		})
	}
	for _, role := range []string{"loop-supervisor", "plan-supervisor", "planner"} {
		t.Run(role, func(t *testing.T) {
			d, err := ParseFile(filepath.Join("..", "..", "agents", role+".md"))
			if err != nil {
				t.Fatal(err)
			}
			body := strings.Join(strings.Fields(d.Body), " ")
			if strings.Contains(body, "docs/specs/agent-fabric.md") {
				t.Error("PORTABILITY: exported role depends on Fabric source checkout")
			}
			wants := []string{"scoped commit/integration by the packet-named owner under repository workflow", "verified durable receipt", "eligible owned-resource cleanup", "Retain full raw logs for unresolved FAIL/BLOCKED/DEFERRED", "consumer-needed evidence or external retention", "traceable full-error links", "a clean tree and integrated commit ancestry", "explicit authorized discard of named content with backup/recoverable source", "retain and report the precise gap", "task-owned named hunks/paths with recorded before hash and proof of no unrelated concurrent edits", "No blanket checkout, force worktree removal, reset/clean or broad deletion", "Unassigned/no-caller resources are not obsolete", "macro-ledgers, native fixtures and budgets", "missing detail never waives these safeguards"}
			if role == "planner" {
				wants = append(wants, "compact decisions and rationale in plan detail", "verify the summary before retiring task-owned temporary grilling artifacts", "Resolve all consumer references first", "retain incomplete/foreign artifacts, third-party materials and operational failure evidence", "Blocked mandatory gates do not authorize committing")
				if strings.Contains(body, "Keep rationale, alternatives, review decisions, provenance, and exclusions in separate decision artifacts") {
					t.Error("obsolete separate-decision-artifact requirement")
				}
			} else {
				wants = append(wants, "validate required checks on current accepted inputs → independent review → immediate scoped commit/integration", "No phase PASS with valid uncommitted code", "blocked mandatory gates do not authorize committing", "not invented test PASS", "current accepted input hash", "retained/deleted inventory", "Verify the summary before deleting source-unique evidence", "COMPLETED accepted-scope logs only after all consumer references are resolved", "IDLE is not command completion", "PID/start/boot identity")
			}
			for _, want := range wants {
				if !strings.Contains(body, want) {
					t.Errorf("%s missing closure contract %q", role, want)
				}
			}
		})
	}
}

// Spec: docs/specs/agent-fabric.md — portable supervision instructions
func TestBoundedEpisodesAndDiscretionaryQA(t *testing.T) {
	cases := []struct {
		role    string
		wants   []string
		forbids []string
	}{
		{"loop-supervisor", []string{
			"QA is recommended, not mandatory",
			"loop-supervisor owns the QA decision",
			"DEFERRED",
			"does not stop remaining implementation",
			"Never label skipped, deferred or running checks PASS",
			"implementation_ready",
			"Automatic goal continuations do not authorize new episodes",
			"at most two mutations",
			"baseline_attempts",
			"hypothesis, difference from prior attempts, evidence, failing regression, budget and rollback",
			"Hard limits remain in force",
			"An active child or command is IN_PROGRESS, not a failed gate",
			"implemented, deferred and still-defective",
			"already deployed and verified",
		}, []string{
			"five is the absolute cap",
			"Dispatch the required `qa-runner`",
			"Exception: `QA_SETUP_APPROVAL_REQUIRED` is a policy boundary",
		}},
		{"plan-supervisor", []string{
			"QA is recommended, not mandatory",
			"loop-supervisor owns the QA decision",
			"DEFERRED",
			"implementation_ready",
			"Automatic goal continuations do not authorize new episodes",
			"at most two mutations",
			"Hard limits remain in force",
			"An active child or command is IN_PROGRESS, not a failed gate",
			"already deployed and verified",
		}, []string{
			"passes independent review and QA",
			"Exception: `QA_SETUP_APPROVAL_REQUIRED` is a policy boundary",
		}},
		{"qa-runner", []string{
			"assigned checks, not whole-plan acceptance",
			"QA_SETUP_REQUIRED",
			"DEFERRED",
			"Do not add acceptance predicates",
			"Do not implement or remediate tooling, wrappers or product code",
			"Do not turn an active command into FAIL",
		}, []string{
			"Ask the supervisor to obtain user approval",
			"For end-user UI surfaces, execute accessibility audits",
		}},
	}
	for _, tc := range cases {
		t.Run(tc.role, func(t *testing.T) {
			d, err := ParseFile(filepath.Join("..", "..", "agents", tc.role+".md"))
			if err != nil {
				t.Fatal(err)
			}
			body := strings.Join(strings.Fields(d.Body), " ")
			for _, want := range tc.wants {
				if !strings.Contains(body, want) {
					t.Errorf("missing contract %q", want)
				}
			}
			for _, forbidden := range tc.forbids {
				if strings.Contains(body, forbidden) {
					t.Errorf("obsolete contract %q", forbidden)
				}
			}
		})
	}
}

// Spec: docs/specs/agent-fabric.md — Deterministic supervision support
func TestDeterministicSupportInstructions(t *testing.T) {
	for _, role := range []string{"loop-supervisor", "plan-supervisor", "qa-runner"} {
		d, err := ParseFile(filepath.Join("..", "..", "agents", role+".md"))
		if err != nil {
			t.Fatal(err)
		}
		body := strings.Join(strings.Fields(d.Body), " ")
		for _, want := range []string{"python3 <declared-fabric-root>/hooks/supervisor/support.py", "fingerprint", "unrun checks"} {
			if !strings.Contains(body, want) {
				t.Errorf("%s missing support contract %q", role, want)
			}
		}
		if role != "qa-runner" {
			for _, want := range []string{"reconcile", "handoff", "IDENTITY_GAP", "pending stage"} {
				if !strings.Contains(body, want) {
					t.Errorf("%s missing restart contract %q", role, want)
				}
			}
		}
	}
}

// Spec: docs/specs/agent-fabric.md — long-running checks and visible delivery
func TestSupervisorLongChecksPreserveVisibleProgress(t *testing.T) {
	for _, role := range []string{"loop-supervisor", "plan-supervisor"} {
		d, err := ParseFile(filepath.Join("..", "..", "agents", role+".md"))
		if err != nil {
			t.Fatal(err)
		}
		body := strings.Join(strings.Fields(d.Body), " ")
		for _, want := range []string{"public deliverable", "frozen inputs", "progress receipt", "dependency/bootstrap smoke"} {
			if !strings.Contains(body, want) {
				t.Errorf("%s missing long-check contract %q", role, want)
			}
		}
	}
}

// Spec: docs/specs/agent-fabric.md — crash-safe continuation evidence
func TestSupervisorResumeInputsAreDurable(t *testing.T) {
	for _, role := range []string{"loop-supervisor", "plan-supervisor"} {
		d, err := ParseFile(filepath.Join("..", "..", "agents", role+".md"))
		if err != nil {
			t.Fatal(err)
		}
		body := strings.Join(strings.Fields(d.Body), " ")
		for _, want := range []string{"durable private root", "temporary storage is scratch", "missing evidence is not PASS", "no budget reset"} {
			if !strings.Contains(body, want) {
				t.Errorf("%s missing crash-safe resume contract %q", role, want)
			}
		}
	}
}
