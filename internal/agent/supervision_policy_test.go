// Spec: docs/specs/agent-fabric.md — Bounded episodes and discretionary QA
package agent

import (
	"path/filepath"
	"strings"
	"testing"
)

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
