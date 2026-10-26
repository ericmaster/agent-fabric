"""Spec: docs/specs/task-retrospective.md — discretionary native review launch."""
import json
from pathlib import Path
import re
import unittest

ROOT = Path(__file__).resolve().parents[1]
SKILL = ROOT / "skills/harness-review/SKILL.md"
RECIPE = ROOT / "skills/harness-review/references/pichamber.md"


class TaskReviewLaunchContractTests(unittest.TestCase):
    # Spec: docs/specs/task-retrospective.md via module
    def test_discretion_covers_completion_and_execution_trouble(self):
        # Spec: docs/specs/task-retrospective.md via module
        skill = SKILL.read_text()
        description = next(line for line in skill.splitlines() if line.startswith("description:"))
        for trigger in ("completion", "roadblocks", "errors", "stalled"):
            self.assertIn(trigger, description)
        self.assertIn("## When to invoke", skill)
        self.assertIn("status-only turns without new evidence", skill)
        self.assertIn("checkpoint as incomplete", skill)
        self.assertIn("never trigger recursive reviews", skill)

    def test_review_targets_execution_not_delivery_certification(self):
        # Spec: docs/specs/task-retrospective.md — task success cannot erase execution waste.
        skill = SKILL.read_text()
        description = next(line for line in skill.splitlines() if line.startswith("description:"))
        for concept in ('harness/tool use', 'orchestration', 'skills and context', 'not task-delivery QA'):
            self.assertIn(concept, description)
        for row in ('execution_efficiency', 'context_alignment', 'improvement_actions'):
            self.assertIn(f'| `{row}` |', skill)
            self.assertIn(f'"{row}":', skill)
        for guard in ('Task\nsuccess is context, not the verdict',
                      'Recovery closes the immediate error, not a repeated misuse',
                      'clear guidance\nignored', 'maintenance authority',
                      'installed/generated copy is not durable delivery'):
            self.assertIn(guard, skill)
        recipe = RECIPE.read_text()
        self.assertIn('execution-quality', recipe)

    def test_quoting_failures_require_scoped_ssot_scripts(self):
        # Spec: docs/specs/task-retrospective.md — durable executable prevention.
        skill = SKILL.read_text()
        for requirement in ('### Durable remediation', 'project `scripts/` or `tools/`',
                            "corresponding skill's `scripts/`", 'Reuse an existing helper',
                            'arguments or structured stdin', 'regression test',
                            'secret-management/human-pull rules'):
            self.assertIn(requirement, skill)

    def test_resolved_detours_require_discoverable_guidance(self):
        # Spec: docs/specs/task-retrospective.md — recovery must leave reusable learning.
        skill = SKILL.read_text()
        for requirement in ('Every resolved broad/repeated search or tool-call detour',
                            'project context or corresponding skill', 'verified entry point',
                            'Audit-only notes are not durable learning',
                            'link adequate existing guidance'):
            self.assertIn(requirement, skill)
        self.assertIn('durable remediation', skill.split('| `improvement_actions` |')[1].split('\n')[0])

    def test_unknown_causes_get_one_bounded_diagnosis_then_disposition(self):
        # Spec: docs/specs/task-retrospective.md — one diagnosis, not serial investigation.
        skill = SKILL.read_text()
        for requirement in ('exactly one diagnostic subagent', 'per original source and distinct issue',
                            'before dispatch', 'native child ID', 'read-only',
                            'must not delegate', 'no replacement diagnosis',
                            'supported fix path', 'proceed with authorized correction',
                            '`not remediated`', 'does not reset', 'two cumulative correction starts'):
            self.assertIn(requirement, skill)

    def test_persisted_worker_and_supervisor_blocker_verification(self):
        # Spec: docs/specs/task-retrospective.md — incomplete reading is pending.
        skill = ' '.join(SKILL.read_text().split())
        for requirement in ('supervises one fresh read-only native worker',
                            'persisted session/history locator',
                            'worker may report **pending**',
                            'supervisor independently inspects',
                            'reduce page size', 'character offset',
                            'checkpoint and resume the same review',
                            'cancellation alone does not prove termination',
                            'worker_session_id', 'supervisor probe evidence'):
            self.assertIn(requirement, skill)

    def test_old_name_is_only_a_model_disabled_deprecation(self):
        # Spec: docs/specs/task-retrospective.md
        deprecated = ROOT / 'skills/task-review'
        text = (deprecated / 'SKILL.md').read_text()
        self.assertIn('disable-model-invocation: true', text)
        self.assertIn('Deprecated', text)
        self.assertIn('/harness-review', text)
        self.assertFalse((deprecated / 'scripts').exists())
        self.assertEqual(SKILL.read_text().splitlines()[1], 'name: harness-review')

    def test_asynchronous_launch_keeps_audit_and_independence(self):
        # Spec: docs/specs/task-retrospective.md via module
        skill = SKILL.read_text()
        self.assertNotIn("returnResult: true", skill)
        self.assertIn("without `returnResult`", skill)
        self.assertIn("do not wait for its verdict", skill)
        self.assertIn("review_requested", skill)
        self.assertIn("review_started", skill)
        self.assertIn("two cumulative correction starts", skill)
        self.assertIn("target HEAD and working-tree state", skill)
        self.assertIn("references/pichamber.md", skill)
        self.assertTrue(RECIPE.is_file())

    def test_native_api_recipe_has_required_create_and_send_identity(self):
        # Spec: docs/specs/task-retrospective.md via module
        recipe = RECIPE.read_text()
        example = re.search(r"```json\n(.*?)\n\s*```", recipe, re.S)
        payload = json.loads(example.group(1))
        self.assertEqual(payload["engine"], "pi")
        self.assertTrue(payload["cwd"].startswith("/"))
        self.assertEqual(set(payload["model"]), {"providerId", "modelId"})
        self.assertNotIn("parentId", payload)
        for requirement in ("POST /api/pi/sessions", "review_requested",
                            "/prompt", "streamEpoch", "operationId", "HTTP 202",
                            "send-receipt", "beforeCursor", "hasMoreBefore",
                            "Pi JSONL session file"):
            self.assertIn(requirement, recipe)
        self.assertIn("If recording fails, do not submit", recipe)
        self.assertIn("do not create another session", recipe.lower())


if __name__ == "__main__":
    unittest.main()
