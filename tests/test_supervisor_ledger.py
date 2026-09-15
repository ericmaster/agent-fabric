# Spec: docs/specs/agent-fabric.md — Continuity and checkpoints
import copy
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("ledger", Path(__file__).resolve().parents[1] / "hooks/supervisor/record-ledger.py")
ledger = importlib.util.module_from_spec(spec)
spec.loader.exec_module(ledger)


class LedgerTests(unittest.TestCase):
    # Spec: docs/specs/agent-fabric.md — durable task state
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.execution = Path(self.temp.name).resolve()
        self.root = self.execution / "evidence"
        self.request = {
            "tier": "micro", "task_id": "TASK-1", "expected_version": 0,
            "phase": "code_review", "status": "PASS",
            "checkpoint": {
                "scope_id": "scope-v1", "execution_root": str(self.execution),
                "revision": "abc", "worktree_state": "digest-v1", "next_stage": "qa",
                "sessions": {"implementor": "worker-1", "code-reviewer": "reviewer-1"},
                "attempts": {"mutating": 1, "review_rejections": 0, "infrastructure_failures": 0, "diagnostics": 0},
                "findings": [], "evidence": [{"command": "test", "result": "PASS", "locator": "evidence/test.log"}],
                "blockers": [], "in_flight": None,
            },
        }

    # Spec: docs/specs/agent-fabric.md via reference hook
    def run_hook(self, request):
        return ledger.operate(request, self.root, self.execution)

    # Spec: docs/specs/agent-fabric.md via resume
    def test_load_and_resume_preserve_sessions_stage_and_counters(self):
        load = {"operation": "load", "tier": "micro", "task_id": "TASK-1"}
        self.assertEqual(self.run_hook(load)["status"], "absent")
        self.assertFalse(self.root.exists())
        receipt = self.run_hook(self.request)
        restored = self.run_hook(load)
        self.assertEqual(restored["checkpoint"], self.request["checkpoint"])
        self.assertEqual(restored["version"], 1)
        self.request["expected_version"] = receipt["version"]
        self.request["checkpoint"]["next_stage"] = "reconciliation"
        self.run_hook(self.request)
        state = json.loads(Path(receipt["path"]).read_text())
        self.assertEqual(len(state["events"]), 2)
        self.assertEqual(state["checkpoint"]["sessions"]["code-reviewer"], "reviewer-1")

    # Spec: docs/specs/agent-fabric.md — structural recovery checkpoints
    def test_structural_recovery_and_aggregate_acceptance_round_trip(self):
        for stage in ("structural_recovery", "aggregate_acceptance"):
            with self.subTest(stage=stage):
                self.request["checkpoint"]["next_stage"] = stage
                self.request["checkpoint"]["replacement_scopes"] = ["TASK-1/path-a"]
                receipt = self.run_hook(self.request)
                restored = self.run_hook({"operation": "load", "tier": "micro", "task_id": "TASK-1"})
                self.assertEqual(restored["checkpoint"], self.request["checkpoint"])
                self.request["expected_version"] = receipt["version"]

    # Spec: docs/specs/agent-fabric.md via stale-write rejection
    def test_stale_writer_and_counter_reset_leave_state_unchanged(self):
        receipt = self.run_hook(self.request)
        before = Path(receipt["path"]).read_bytes()
        with self.assertRaisesRegex(ValueError, "stale"):
            self.run_hook(self.request)
        self.request["expected_version"] = 1
        self.request["checkpoint"]["scope_id"] = "scope-v2"
        self.request["checkpoint"]["attempts"]["mutating"] = 0
        with self.assertRaisesRegex(ValueError, "cannot decrease"):
            self.run_hook(self.request)
        self.assertEqual(Path(receipt["path"]).read_bytes(), before)

    # Spec: docs/specs/agent-fabric.md via failure-safe persistence
    def test_replace_failure_keeps_previous_journal_and_checkpoint(self):
        receipt = self.run_hook(self.request)
        before = Path(receipt["path"]).read_bytes()
        self.request["expected_version"] = 1
        with patch.object(ledger.os, "replace", side_effect=OSError("disk failure")):
            with self.assertRaises(OSError):
                self.run_hook(self.request)
        self.assertEqual(Path(receipt["path"]).read_bytes(), before)
        self.assertEqual(list(self.root.glob(".ledger-*")), [])

    # Spec: docs/specs/agent-fabric.md via identity and input validation
    def test_tasks_are_isolated_and_invalid_checkpoints_fail(self):
        first = self.run_hook(self.request)
        other = copy.deepcopy(self.request)
        other["task_id"] = "../../other"
        second = self.run_hook(other)
        self.assertNotEqual(first["path"], second["path"])
        self.assertEqual(Path(second["path"]).parent, self.root)
        for key, value in (("execution_root", "/wrong"), ("next_stage", "anything"), ("in_flight", {"role": "implementor"})):
            with self.subTest(key=key):
                invalid = copy.deepcopy(self.request)
                invalid["checkpoint"][key] = value
                with self.assertRaises(ValueError):
                    self.run_hook(invalid)
        Path(first["path"]).write_text("{broken")
        with self.assertRaises(ValueError):
            self.run_hook({"operation": "load", "tier": "micro", "task_id": "TASK-1"})

    # Spec: docs/specs/agent-fabric.md via in-flight checkpoint
    def test_in_flight_dispatch_and_macro_phase_states_round_trip(self):
        request = copy.deepcopy(self.request)
        request.update(tier="macro", plan_id="PLAN-1")
        request["checkpoint"]["phase_states"] = {"PHASE-1": {"status": "IN_PROGRESS"}}
        request["checkpoint"]["in_flight"] = {"dispatch_id": "dispatch-1", "role": "loop-supervisor"}
        self.run_hook(request)
        result = self.run_hook({"operation": "load", "tier": "macro", "plan_id": "PLAN-1"})
        self.assertEqual(result["checkpoint"]["in_flight"]["dispatch_id"], "dispatch-1")
        self.assertEqual(result["checkpoint"]["phase_states"], request["checkpoint"]["phase_states"])
        path = Path(result["path"])
        corrupt = json.loads(path.read_text())
        del corrupt["checkpoint"]["phase_states"]
        path.write_text(json.dumps(corrupt))
        with self.assertRaisesRegex(ValueError, "phase_states"):
            self.run_hook({"operation": "load", "tier": "macro", "plan_id": "PLAN-1"})


if __name__ == "__main__":
    unittest.main()
