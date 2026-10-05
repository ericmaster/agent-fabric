# Spec: docs/specs/agent-fabric.md
"""Regression guard for dashboard-visible canonical model defaults."""
import json
from pathlib import Path
import unittest


class ModelDefaultsTest(unittest.TestCase):
    # Spec: docs/specs/agent-fabric.md — dashboard-visible model defaults
    def test_opencode_and_kilo_do_not_restore_retired_gpt_5_6(self):
        # Spec: docs/specs/agent-fabric.md — dashboard-visible model defaults
        root = Path(__file__).resolve().parents[1]
        for target in ("opencode", "kilo"):
            with self.subTest(target=target):
                adapter = json.loads((root / "adapters" / f"{target}.json").read_text())
                for profile, mapping in adapter["profiles"].items():
                    self.assertFalse(mapping["model"].startswith("openai/gpt-5.6"), profile)
                self.assertEqual(adapter["profiles"]["solver"]["model"], "openai/gpt-6.1-sol")

    def test_codex_reviewed_profiles_keep_current_model(self):
        # Spec: docs/specs/agent-fabric.md — adapter model defaults
        root = Path(__file__).resolve().parents[1]
        adapter = json.loads((root / "adapters" / "codex.json").read_text())
        for profile in ("readonly", "worker", "reviewer", "supervisor", "qa"):
            with self.subTest(profile=profile):
                self.assertEqual(adapter["profiles"][profile]["model"], "gpt-6.1-sol")
