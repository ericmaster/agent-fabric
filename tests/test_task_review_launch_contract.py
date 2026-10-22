"""Spec: docs/specs/task-retrospective.md — discretionary native review launch."""
import json
from pathlib import Path
import re
import unittest

ROOT = Path(__file__).resolve().parents[1]
SKILL = ROOT / "skills/task-review/SKILL.md"
RECIPE = ROOT / "skills/task-review/references/pichamber.md"


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
