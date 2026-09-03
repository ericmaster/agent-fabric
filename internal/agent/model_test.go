// Spec: docs/specs/agent-fabric.md
package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseFrontmatterHooksAndRootKeysAfterFabricBlock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.md")
	content := "---\nx-agent-fabric:\n  schema: 1\n  profile: worker\n  effort: high\n  visibility: hidden\n  isolation: sandbox\n  hooks: [pre-plan]\ndescription: Demo\nmode: subagent\n---\nbody\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	d, err := ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if d.Description != "Demo" || len(d.Fabric.Hooks) != 1 || d.Fabric.Hooks[0] != "pre-plan" {
		t.Fatalf("parsed definition lost root or hook fields: %+v", d)
	}
}

func TestCanonicalAgentsRetainWorkflowContracts(t *testing.T) {
	anchors := map[string][]string{
		"planner":           {"## Initial Understanding", "## Design", "## Independent Review", "## Final Plan And Proposal"},
		"plan-supervisor":   {"## Execution Model", "## Evidence Contract", "## Failure And Recovery"},
		"loop-supervisor":   {"## Outcome Contract", "## Atomic Execution Loop", "## Bounded Recovery"},
		"implementor":       {"Before editing", "Run the exact required checks", "## Execution And Handoff"},
		"code-reviewer":     {"adversarial, read-only gate", "mental mutation test", "## Review Protocol", "ACCEPT|REJECT"},
		"expert-debugger":   {"at least three distinct hypotheses", "bounded remediation brief", "## Recovery Protocol", "failure_classification"},
		"plan-reviewer":     {"vertical-slice shape", "## Review Rubric And Output", "PASS|REVISE"},
		"qa-runner":         {"Read the original DoD", "## Verification Discipline", "PASS|FAIL|BLOCKED"},
		"deploy-supervisor": {"## Operating Invariant & Human Gate", "## Release Execution Sequence", "## Output Contract"},
	}
	for id, required := range anchors {
		d, err := ParseFile(filepath.Join("..", "..", "agents", id+".md"))
		if err != nil {
			t.Fatalf("parse %s: %v", id, err)
		}
		for _, anchor := range required {
			if !strings.Contains(d.Body, anchor) {
				t.Errorf("%s lost workflow anchor %q", id, anchor)
			}
		}
	}
}

func TestExpertDebuggerUsesSolverProfile(t *testing.T) {
	d, err := ParseFile(filepath.Join("..", "..", "agents", "expert-debugger.md"))
	if err != nil {
		t.Fatalf("parse expert-debugger: %v", err)
	}
	if d.Fabric.Profile != "solver" {
		t.Errorf("expert-debugger profile = %q, want solver", d.Fabric.Profile)
	}
	if d.Fabric.Effort != "max" {
		t.Errorf("expert-debugger effort = %q, want max", d.Fabric.Effort)
	}
}

func TestCanonicalFreshContextDelegationContracts(t *testing.T) {
	dispatchers := []string{
		"loop-supervisor",
		"plan-supervisor",
		"planner",
	}
	childRoles := []string{
		"implementor",
		"code-reviewer",
		"qa-runner",
		"expert-debugger",
		"plan-reviewer",
	}
	ids := append(append([]string{}, dispatchers...), childRoles...)
	bodies := map[string]string{}
	for _, id := range ids {
		d, err := ParseFile(filepath.Join("..", "..", "agents", id+".md"))
		if err != nil {
			t.Fatalf("parse %s: %v", id, err)
		}
		bodies[id] = d.Body
	}

	t.Run("every explicit fresh edge is packet guarded", func(t *testing.T) {
		tests := []struct {
			name   string
			agent  string
			anchor string
		}{
			{"loop initial implementor", "loop-supervisor", "packet, then dispatch `implementor` in a fresh context"},
			{"loop code reviewer", "loop-supervisor", "packet, then dispatch `code-reviewer` in a separate fresh context"},
			{"loop QA runner", "loop-supervisor", "packet, then dispatch `qa-runner` in a fresh context"},
			{"loop expert debugger", "loop-supervisor", "packet, then dispatch `expert-debugger` in a fresh diagnostic context"},
			{"loop retries and remediation", "loop-supervisor", "Every retry, remediation, or idle-child redispatch repeats the applicable hook and immediate packet validation"},
			{"plan phase loop supervisor", "plan-supervisor", "initial phase `loop-supervisor`"},
			{"plan retry loop supervisor", "plan-supervisor", "retried phase `loop-supervisor`"},
			{"plan recovery diagnostic", "plan-supervisor", "recovery diagnostic"},
			{"plan recovery remediation", "plan-supervisor", "recovery remediation"},
			{"planner discovery child", "planner", "Any fresh discovery or design child receives a self-locating Delegation Packet"},
			{"planner initial review", "planner", "Every plan-reviewer pass receives a self-locating Delegation Packet"},
			{"planner revised review", "planner", "This includes every revised-candidate pass"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				if !strings.Contains(bodies[tt.agent], tt.anchor) {
					t.Errorf("%s missing fresh-edge safeguard %q", tt.agent, tt.anchor)
				}
			})
		}
	})

	packetFields := []string{
		"declared execution root",
		"workspace ownership/isolation",
		"VCS revision",
		"working-tree state",
		"bounded objective",
		"explicit non-goals",
		"authoritative input inline",
		"unambiguous locator anchored",
		"named declared root",
		"permitted source and evidence paths",
		"exact required commands",
		"observable DoD",
		"required evidence",
		"rollback boundary",
		"explicit unresolved-locator behavior",
		"bare name without a declared base",
		"missing, unreadable, or ambiguous required input",
		"A context gap can never yield",
		"Never search ambient roots",
		"stays within declared and permitted paths",
		"Hooks may enrich or validate the packet",
		"never reconstruct a location known to its producer",
	}

	t.Run("each canonical role carries a self-contained portable packet", func(t *testing.T) {
		for _, id := range ids {
			body := bodies[id]
			if got := strings.Count(body, "## Delegation Packet"); got != 1 {
				t.Errorf("%s delegation packet definitions = %d, want 1", id, got)
			}
			if strings.Contains(body, "docs/specs/agent-fabric.md") {
				t.Errorf("%s depends on the source-checkout packet specification", id)
			}
			if strings.Contains(body, "Delegation Packet defined by") {
				t.Errorf("%s makes another agent body a packet dependency", id)
			}
			for _, anchor := range packetFields {
				if !strings.Contains(body, anchor) {
					t.Errorf("%s packet missing %q", id, anchor)
				}
			}
		}
	})

	const unconditionalOldGuard = "Before any fresh-context dispatch or substantive work"
	t.Run("dispatcher entrypoints distinguish direct intake and child dispatch", func(t *testing.T) {
		for _, id := range dispatchers {
			body := bodies[id]
			for _, anchor := range []string{
				"Direct user invocation is not a fresh-child handoff; a Delegation Packet is optional",
				"When invoked as a fresh child, validate the intake Delegation Packet before substantive work",
				"Before every fresh child dispatch, construct and validate a separate self-contained Delegation Packet immediately before dispatch",
				"For fresh-child intake and outgoing packet validation, resolve required inputs only from packet content",
				"Fail closed before substantive child work",
				"Normal repository inspection for fresh-child intake begins only after all required packet inputs resolve",
				"before child dispatch, every outgoing packet locator must resolve within its declared and permitted paths",
			} {
				if !strings.Contains(body, anchor) {
					t.Errorf("%s missing direct/fresh-child distinction %q", id, anchor)
				}
			}
			if strings.Contains(body, unconditionalOldGuard) {
				t.Errorf("%s retains unconditional packet guard %q", id, unconditionalOldGuard)
			}
		}
	})

	t.Run("child-only roles remain fail closed before substantive work", func(t *testing.T) {
		for _, id := range childRoles {
			for _, anchor := range []string{
				unconditionalOldGuard,
				"Fail closed before substantive work",
				"Normal repository inspection begins only after all required packet inputs resolve",
			} {
				if !strings.Contains(bodies[id], anchor) {
					t.Errorf("%s lost child-only intake guard %q", id, anchor)
				}
			}
		}
	})

	t.Run("roles preserve schema-compatible context-gap results", func(t *testing.T) {
		tests := []struct {
			agent         string
			failureAnchor string
		}{
			{"loop-supervisor", "stop with `BLOCKED` and name the exact gap"},
			{"plan-supervisor", "return `BLOCKED` for fresh-child intake, or keep the affected phase `BLOCKED`, and name the exact gap"},
			{"planner", "stop fresh-child intake or the affected child dispatch and report the exact gap"},
			{"implementor", "return `BLOCKED` naming the exact gap"},
			{"code-reviewer", "return `REJECT` with a `scope_blocker` finding naming the exact gap"},
			{"qa-runner", "return `BLOCKED` naming the exact gap"},
			{"expert-debugger", "return the existing schema with the exact gap in `root_cause_analysis.blockers`"},
			{"plan-reviewer", "return `REVISE` with a critical finding naming the exact gap"},
		}
		for _, tt := range tests {
			t.Run(tt.agent, func(t *testing.T) {
				body := bodies[tt.agent]
				for _, anchor := range []string{"A context gap can never yield `PASS` or `ACCEPT`", tt.failureAnchor} {
					if !strings.Contains(body, anchor) {
						t.Errorf("%s missing fail-closed intake %q", tt.agent, anchor)
					}
				}
			})
		}
	})

	t.Run("artifact resolution is distinct from repository inspection", func(t *testing.T) {
		spec, err := os.ReadFile(filepath.Join("..", "..", "docs", "specs", "agent-fabric.md"))
		if err != nil {
			t.Fatal(err)
		}
		tests := []struct {
			name       string
			body       string
			resolution string
			inspection string
		}{
			{"normative spec", string(spec), "For fresh-child intake and outgoing packet validation, required task and evidence artifacts are resolved only from packet content", "Normal repository inspection for fresh-child intake begins only after all required packet inputs resolve"},
			{"loop supervisor", bodies["loop-supervisor"], "For fresh-child intake and outgoing packet validation, resolve required inputs only from packet content", "Normal repository inspection for fresh-child intake begins only after all required packet inputs resolve"},
			{"implementor", bodies["implementor"], "Resolve required inputs only from packet content", "Normal repository inspection begins only after all required packet inputs resolve"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				if !strings.Contains(tt.body, tt.resolution) || !strings.Contains(tt.body, tt.inspection) {
					t.Errorf("missing resolution/inspection distinction: resolution=%q inspection=%q", tt.resolution, tt.inspection)
				}
			})
		}
	})

	t.Run("normative contract permits direct dispatcher invocation", func(t *testing.T) {
		spec, err := os.ReadFile(filepath.Join("..", "..", "docs", "specs", "agent-fabric.md"))
		if err != nil {
			t.Fatal(err)
		}
		for _, anchor := range []string{
			"Direct user invocation of a dispatcher-capable primary or all-mode role is not a fresh-child handoff",
			"does not require an intake Delegation Packet",
			"If that role is invoked as a fresh child, it validates the intake packet before substantive work",
			"Every outgoing fresh-child dispatch requires a separately constructed and validated Delegation Packet",
			"ordinary direct invocation in the user-selected execution context",
		} {
			if !strings.Contains(string(spec), anchor) {
				t.Errorf("normative spec missing direct-invocation compatibility %q", anchor)
			}
		}
	})

	t.Run("dispatcher architecture keeps direct entry separate from packet-labelled fresh edges", func(t *testing.T) {
		tests := []struct {
			agent   string
			anchors []string
		}{
			{"planner", []string{
				"Direct user invocation is not a fresh-child handoff, so its intake packet is optional.",
				`FRESH -->|"intake Delegation Packet"| INTAKE`,
				`MAP -->|"validated discovery packet"| GAPS`,
				`WRITE -- "No · validated review packet" --> REVIEWER`,
				`CAP -- "No · refreshed review packet" --> REVIEWER`,
			}},
			{"plan-supervisor", []string{
				"Direct user invocation is not a fresh-child handoff, so its intake packet is optional.",
				`FRESH -->|"intake Delegation Packet"| INTAKE`,
				`BRIEF -->|"validated phase packet"| DISPATCH`,
				`FAIL_CLASS -->|"validated recovery packet"| DIAG`,
				`EXHAUST -- "No · refreshed phase packet" --> DISPATCH`,
			}},
			{"loop-supervisor", []string{
				"Direct user invocation is not a fresh-child handoff, so its intake packet is optional.",
				`FRESH -->|"intake Delegation Packet"| LT`,
				`BRIEF -->|"validated implementor packet"| IMPL`,
				`IMPL -->|"validated review packet"| REV`,
				`REV -->|"validated QA packet"| QA`,
				`PRESERVE -->|"validated recovery packet"| DIAG_CTX`,
				`BUDGET -- "Yes · validated retry packet" --> IMPL`,
			}},
		}
		for _, tt := range tests {
			t.Run(tt.agent, func(t *testing.T) {
				architecture, err := os.ReadFile(filepath.Join("..", "..", "docs", "architecture", tt.agent+".md"))
				if err != nil {
					t.Fatal(err)
				}
				for _, anchor := range tt.anchors {
					if !strings.Contains(string(architecture), anchor) {
						t.Errorf("%s architecture missing %q", tt.agent, anchor)
					}
				}
			})
		}
	})

	t.Run("portable locator and proof boundaries are explicit", func(t *testing.T) {
		spec, err := os.ReadFile(filepath.Join("..", "..", "docs", "specs", "agent-fabric.md"))
		if err != nil {
			t.Fatal(err)
		}
		for _, anchor := range []string{
			"execution, artifact, or evidence base",
			"relative locator explicitly names the Declared Root or base",
			"absolute, Declared-Root-relative",
			"URI-like, or harness-supported",
			"bare artifact name is insufficient",
			"ambient home or temporary",
			"No hook can reconstruct a location",
			"do not prove model compliance",
		} {
			if !strings.Contains(string(spec), anchor) {
				t.Errorf("normative spec missing portable boundary %q", anchor)
			}
		}
		for _, tt := range []struct {
			agent string
			ban   string
		}{
			{"implementor", "Use absolute paths"},
			{"qa-runner", "Use absolute paths"},
		} {
			if strings.Contains(bodies[tt.agent], tt.ban) {
				t.Errorf("%s retains non-portable mandate %q", tt.agent, tt.ban)
			}
		}
	})

	t.Run("system overview routes child results and dispatches through loop supervisor", func(t *testing.T) {
		overview, err := os.ReadFile(filepath.Join("..", "..", "docs", "architecture", "system-overview.md"))
		if err != nil {
			t.Fatal(err)
		}
		body := string(overview)
		for _, edge := range []string{
			`IMPL -->|"implementation result"| LSUP`,
			`LSUP -->|"validated review packet dispatch"| CREV`,
			`CREV -->|"review result"| LSUP`,
			`LSUP -->|"validated QA packet dispatch"| QAR`,
			`QAR -->|"QA result"| LSUP`,
		} {
			if !strings.Contains(body, edge) {
				t.Errorf("system overview missing supervisor-owned edge %q", edge)
			}
		}
		for _, line := range strings.Split(body, "\n") {
			edge := strings.ReplaceAll(strings.TrimSpace(line), " ", "")
			if strings.HasPrefix(edge, "IMPL-->") && strings.Contains(edge, "CREV") {
				t.Errorf("implementor must return to loop supervisor, not dispatch reviewer: %s", line)
			}
			if strings.HasPrefix(edge, "CREV-->") && strings.Contains(edge, "QAR") {
				t.Errorf("code reviewer must return to loop supervisor, not dispatch QA: %s", line)
			}
		}
	})
}

func TestCanonicalAgentsUseInstallTimeHookPlaceholders(t *testing.T) {
	for _, id := range []string{"planner", "plan-supervisor", "loop-supervisor", "implementor", "code-reviewer", "expert-debugger", "plan-reviewer", "qa-runner", "deploy-supervisor"} {
		d, err := ParseFile(filepath.Join("..", "..", "agents", id+".md"))
		if err != nil {
			t.Fatalf("parse %s: %v", id, err)
		}
		if strings.Count(d.Body, "<agent-hooks:list-available>") != 1 {
			t.Errorf("%s must declare one hook list placeholder", id)
		}
		for _, hook := range d.Fabric.Hooks {
			marker := "<agent-hooks:invoke:" + hook + ">"
			if strings.Count(d.Body, marker) != 1 {
				t.Errorf("%s must declare %q exactly once", id, marker)
			}
		}
		if strings.Contains(d.Body, "AGENT_TRACE_ROOT") || strings.Contains(d.Body, "TRACE_ROOT") || strings.Contains(d.Body, ".agent-hooks/") {
			t.Errorf("%s leaks hook implementation details into the canonical body", id)
		}
	}
}

func TestPlannerDelegatesOptionalValidationToPrePlanHook(t *testing.T) {
	d, err := ParseFile(filepath.Join("..", "..", "agents", "planner.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"## Candidate Validation", "Validation requires:", "local validator"} {
		if strings.Contains(d.Body, forbidden) {
			t.Errorf("planner must not define validation policy: %q", forbidden)
		}
	}
}

func TestPlannerCapsReviewAndHonorsPublishInstruction(t *testing.T) {
	d, err := ParseFile(filepath.Join("..", "..", "agents", "planner.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"**Publish:**",
		"two `plan-reviewer` passes",
		"explicit write instruction",
		"Skip remaining reviews",
	} {
		if !strings.Contains(d.Body, want) {
			t.Errorf("planner missing %q", want)
		}
	}
	for _, forbidden := range []string{
		"Proceed only when review passes",
		"independent review prevents proposal",
	} {
		if strings.Contains(d.Body, forbidden) {
			t.Errorf("planner still blocks on review: %q", forbidden)
		}
	}
}

func TestSupervisionRecoveryContracts(t *testing.T) {
	tests := []struct {
		agent   string
		wants   []string
		forbids []string
	}{
		{"plan-supervisor", []string{
			"cumulative mutating-attempt",
			"`BLOCKED` phase is eligible for redispatch only when new",
			"For a `BLOCKED`\n   phase, retain its status and do not rebrief or redispatch",
			"second substantive code or specification rejection",
			"Environment and harness failures do not count",
		}, []string{"Re-brief the same phase with the diagnostic artifact and cumulative counters,\n   then dispatch again in fresh context."}},
		{"loop-supervisor", []string{
			"Rebriefing, resuming, and fresh sessions never reset",
			"a `scope_blocker` immediately returns `BLOCKED`",
			"second substantive code review rejection or QA failure",
			"earliest shared enforcement boundary",
			"\"attempts\":",
		}, nil},
		{"code-reviewer", []string{
			"findings may produce `REJECT`",
			"\"classification\":\"new|repeat|scope_blocker\"",
			"\"breached_contract\"",
		}, nil},
	}

	for _, tt := range tests {
		t.Run(tt.agent, func(t *testing.T) {
			d, err := ParseFile(filepath.Join("..", "..", "agents", tt.agent+".md"))
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range tt.wants {
				if !strings.Contains(d.Body, want) {
					t.Errorf("%s missing recovery contract %q", tt.agent, want)
				}
			}
			for _, forbidden := range tt.forbids {
				if strings.Contains(d.Body, forbidden) {
					t.Errorf("%s has unconditional recovery dispatch %q", tt.agent, forbidden)
				}
			}
		})
	}
}

func TestReviewerAndQARunnerScopeBoundaries(t *testing.T) {
	cr, err := ParseFile(filepath.Join("..", "..", "agents", "code-reviewer.md"))
	if err != nil {
		t.Fatalf("parse code-reviewer: %v", err)
	}

	// Code reviewer must statically trace (not dynamically test) exploits and focus on code-level evidence.
	crWants := []string{
		"statically trace the exploit matrix through\nthe call path",
		"Reject missing code-level evidence",
		"dynamic verification and live command\nexecution are the exclusive authority of `qa-runner`",
		"dynamic DoD\nitems must be omitted from reviewer rejections and deferred to QA",
		"compilation_status: NOT_AVAILABLE",
		"Syntax, import, or type failures",
		"untrusted input",
		"unsafe file paths",
		"hollow tests",
		"swallowed errors",
		"scope drift",
		"unsupported scope expansion",
	}
	for _, want := range crWants {
		if !strings.Contains(cr.Body, want) {
			t.Errorf("code-reviewer missing scope boundary assertion: %q", want)
		}
	}

	// Code reviewer must not instruct dynamic testing.
	if strings.Contains(cr.Body, "test an exploit matrix") {
		t.Errorf("code-reviewer contains dynamic exploit testing instruction")
	}

	qa, err := ParseFile(filepath.Join("..", "..", "agents", "qa-runner.md"))
	if err != nil {
		t.Fatalf("parse qa-runner: %v", err)
	}
	qaWants := []string{
		"without offering code-quality judgments",
		"deployment evidence is evaluated against\nthe DoD during supervisor reconciliation",
		"runtime, persistence, payload, and visual checks",
	}
	for _, want := range qaWants {
		if !strings.Contains(qa.Body, want) {
			t.Errorf("qa-runner missing symmetric boundary assertion: %q", want)
		}
	}

	ls, err := ParseFile(filepath.Join("..", "..", "agents", "loop-supervisor.md"))
	if err != nil {
		t.Fatalf("parse loop-supervisor: %v", err)
	}
	lsWants := []string{
		"Reclassify review findings grounded solely on absent dynamic evidence",
		"out-of-authority",
		"authoritative for runtime and visual claims",
		"authoritative for code-level contracts",
		"qa-runner` `FAIL` counts equivalently toward the two-rejection diagnostic trigger",
	}
	for _, want := range lsWants {
		if !strings.Contains(ls.Body, want) {
			t.Errorf("loop-supervisor missing authority filtering or precedence assertion: %q", want)
		}
	}
}

func TestParseFrontmatterRejectsInvalidSchema(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "invalid-schema.md")
	content := "---\ndescription: Demo\nmode: subagent\nx-agent-fabric:\n  schema: 1.0\n  profile: worker\n  effort: high\n  visibility: hidden\n  isolation: sandbox\n---\nbody\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseFile(path); err == nil {
		t.Fatal("expected error on non-integer schema version '1.0'")
	}
}

func TestParseFrontmatterIgnoresUnrelatedIndentedSections(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "extra-section.md")
	content := "---\ndescription: Demo\nmode: subagent\ncustom-meta:\n  edit: allow\n  profile: planner\nx-agent-fabric:\n  schema: 1\n  profile: worker\n  effort: high\n  visibility: hidden\n  isolation: sandbox\n  permissions:\n    edit: deny\n---\nbody\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	d, err := ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if d.Fabric.Profile != "worker" {
		t.Fatalf("custom-meta contaminated profile: %q", d.Fabric.Profile)
	}
	if d.Fabric.Permissions["edit"] != "deny" {
		t.Fatalf("custom-meta contaminated permissions: %q", d.Fabric.Permissions["edit"])
	}
}
