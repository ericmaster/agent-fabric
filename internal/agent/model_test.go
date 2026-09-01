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
		"planner":         {"## Initial Understanding", "## Design", "## Independent Review", "## Final Plan And Proposal"},
		"plan-supervisor": {"## Execution Model", "## Evidence Contract", "## Failure And Recovery"},
		"loop-supervisor": {"## Outcome Contract", "## Atomic Execution Loop", "## Bounded Recovery"},
		"implementor":     {"Before editing", "Run the exact required checks", "## Execution And Handoff"},
		"code-reviewer":   {"adversarial, read-only gate", "mental mutation test", "## Review Protocol", "ACCEPT|REJECT"},
		"expert-debugger": {"at least three distinct hypotheses", "bounded remediation brief", "## Recovery Protocol", "failure_classification"},
		"plan-reviewer":   {"vertical-slice shape", "## Review Rubric And Output", "PASS|REVISE"},
		"qa-runner":       {"Read the original DoD", "## Verification Discipline", "PASS|FAIL|BLOCKED"},
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
