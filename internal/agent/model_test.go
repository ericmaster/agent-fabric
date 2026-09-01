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
	for _, id := range []string{"planner", "plan-supervisor", "loop-supervisor", "implementor", "code-reviewer", "expert-debugger", "plan-reviewer", "qa-runner"} {
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
			"second substantive code or specification rejection",
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
