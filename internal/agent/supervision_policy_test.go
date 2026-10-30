// Spec: docs/specs/supervision-and-delivery.md, docs/specs/supervision-ledger-and-qa.md — Bounded episodes and discretionary QA
package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Spec: docs/specs/supervision-and-delivery.md — Phase closure and compact planning
// These are instruction-policy regressions, not filesystem/process enforcement.
func TestPhaseClosureAndCompactPlanning(t *testing.T) {
	spec, err := os.ReadFile(filepath.Join("..", "..", "docs", "specs", "supervision-and-delivery.md"))
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
			var wants []string
			if role == "planner" {
				wants = []string{
					"with clear architectural boundaries and contracts, leaving standard coding mechanics to the implementer",
					"**Proportionality & Complexity Budget:** Scale verification and ceremony strictly to the user seed",
					"**Zero Unrequested Artifacts:** Never invent unrequested JSON Schemas (`.schema.json`), architecture DSL updates (`workspace.dsl`)",
					"**Pragmatic Preflight:** Prefer resolving live provider aliases (e.g., `-latest`) over hard-blocking on brittle version strings",
					"compact decisions and rationale in plan detail",
					"Retire task-owned temporary grilling artifacts after capturing decisions",
					"remove only clean, integrated, task-owned worktrees/branches with no active processes",
					"Never blanket reset/clean or force-remove worktrees, preserve foreign edits",
					"basic credential/secret redaction",
					"Do not require logging counters/budgets, input hashes, consumer closure inventories, or multi-format report matrices for routine tasks",
				}
				for _, forbidden := range []string{
					"Keep rationale, alternatives, review decisions, provenance, and exclusions in separate decision artifacts",
					"with no hidden design decisions left to the implementer",
				} {
					if strings.Contains(body, forbidden) {
						t.Errorf("planner retains obsolete requirement %q", forbidden)
					}
				}
			} else {
				wants = []string{"scoped commit/integration by the packet-named owner under repository workflow", "verified durable receipt", "eligible owned-resource cleanup", "Retain full raw logs for unresolved FAIL/BLOCKED/DEFERRED", "consumer-needed evidence or external retention", "traceable full-error links", "a clean tree and integrated commit ancestry", "explicit authorized discard of named content with backup/recoverable source", "retain and report the precise gap", "task-owned named hunks/paths with recorded before hash and proof of no unrelated concurrent edits", "No blanket checkout, force worktree removal, reset/clean or broad deletion", "Unassigned/no-caller resources are not obsolete", "macro-ledgers, native fixtures and budgets", "missing detail never waives these safeguards", "validate required checks on current accepted inputs → independent review → immediate scoped commit/integration", "No phase PASS with valid uncommitted code", "blocked mandatory gates do not authorize committing", "not invented test PASS", "current accepted input hash", "retained/deleted inventory", "Verify the summary before deleting source-unique evidence", "COMPLETED accepted-scope logs only after all consumer references are resolved", "IDLE is not command completion", "PID/start/boot identity"}
			}
			for _, want := range wants {
				if !strings.Contains(body, want) {
					t.Errorf("%s missing closure contract %q", role, want)
				}
			}
		})
	}
	t.Run("plan-reviewer", func(t *testing.T) {
		d, err := ParseFile(filepath.Join("..", "..", "agents", "plan-reviewer.md"))
		if err != nil {
			t.Fatal(err)
		}
		body := strings.Join(strings.Fields(d.Body), " ")
		for _, want := range []string{
			"**Enforce Proportionality (Anti-Ratchet Rule):** You are a guardian of BOTH safety and execution speed/ROI. Treat **over-engineering, unrequested infrastructure, excessive benchmark sample sizes, and compliance theater** as a `high` severity defect under category `bloat`.",
			"**Forbidden Reviewer Demands:** Never issue a `REVISE` demanding statistical significance (e.g., 100+ runs, ECE bins, large labeled corpora), synthetic fault-injection harnesses, canary secret scanners, doc-validation unit tests, or multi-viewport visual checks unless the user seed explicitly asked for them.",
			"**Subtraction Bias:** Whenever you require adding a missing safety guard or test, check if an over-engineered requirement in the candidate can be simplified or removed to keep the phase lean.",
		} {
			if !strings.Contains(body, want) {
				t.Errorf("plan-reviewer missing anti-bloat guardrail %q", want)
			}
		}
	})
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
			"standing task authority",
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
			"standing task authority",
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

// Spec: docs/specs/agent-fabric.md — Delivery ownership and checkout closure
func TestDeliveryOwnershipAndNativeContinuations(t *testing.T) {
	for _, role := range []string{"plan-supervisor", "loop-supervisor", "deploy-supervisor"} {
		d, err := ParseFile(filepath.Join("..", "..", "agents", role+".md"))
		if err != nil {
			t.Fatal(err)
		}
		body := strings.Join(strings.Fields(d.Body), " ")
		if d.Mode != "all" {
			t.Errorf("%s must support both root delivery and native child delegation", role)
		}
		for _, want := range []string{"own delivery", "authorized deployment", "current checkout", "active writers", "not dirtiness alone", "never auto-commit foreign changes", "owning checkout clean", "busy and retry", "matching dispatch/session once", "compact current delta", "original authority"} {
			if !strings.Contains(body, want) {
				t.Errorf("%s missing delivery contract %q", role, want)
			}
		}
		for _, forbidden := range []string{"until explicit user authority and a distinct recovery strategy arrive", "a direct explicit user continuation permits evaluation", "A direct explicit user instruction after exhaustion permits", "Automatic goal continuations do not authorize new episodes"} {
			if strings.Contains(body, forbidden) {
				t.Errorf("%s retains permission-renewal loop %q", role, forbidden)
			}
		}
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

// Spec: docs/specs/agent-fabric.md — compact context and provisional isolated work
func TestCompactContextAndBackgroundVerification(t *testing.T) {
	for _, role := range []string{"plan-supervisor", "loop-supervisor", "implementor"} {
		d, err := ParseFile(filepath.Join("..", "..", "agents", role+".md"))
		if err != nil {
			t.Fatal(err)
		}
		body := strings.Join(strings.Fields(d.Body), " ")
		wants := []string{"smallest functional smoke on the target runtime/producer", "implementation_ready", "background on frozen inputs"}
		if role != "implementor" {
			wants = append(wants, "one current packet", "`compact: true` only for status-only loads", "Restore the full checkpoint", "receipts are not recovery or acceptance evidence", "Do not mark DONE", "until acceptance passes", "affected dependent work")
		}
		for _, want := range wants {
			if !strings.Contains(body, want) {
				t.Errorf("%s missing compact/background contract %q", role, want)
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
