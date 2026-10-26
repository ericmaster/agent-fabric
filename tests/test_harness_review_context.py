"""Spec: docs/specs/task-retrospective.md — native instruction activation."""
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]
INSTALL = ROOT / 'skills/harness-review/scripts/install_context.py'
ACTIVATION = ROOT / 'skills/harness-review/references/activation.md'


class ContextTests(unittest.TestCase):
    # Spec: docs/specs/task-retrospective.md via module
    def run_install(self, *paths, ok=True):
        # Spec: docs/specs/task-retrospective.md via ContextTests
        result = subprocess.run([sys.executable, str(INSTALL), *map(str, paths)],
                                capture_output=True, text=True, timeout=10)
        self.assertEqual(result.returncode, 0 if ok else 1, result.stderr)
        return result

    def test_existing_bytes_and_mode_survive_idempotent_install_and_remove(self):
        # Spec: docs/specs/task-retrospective.md
        with tempfile.TemporaryDirectory() as directory:
            target = Path(directory) / 'AGENTS.md'
            original = b'# Existing\r\nKeep foreign context.\r\n'
            target.write_bytes(original)
            target.chmod(0o640)
            self.run_install(target)
            installed = target.read_bytes()
            self.assertTrue(installed.startswith(original))
            self.assertIn(ACTIVATION.read_bytes().strip(), installed)
            self.assertEqual(target.stat().st_mode & 0o777, 0o640)
            self.run_install(target)
            self.assertEqual(target.read_bytes(), installed)
            self.run_install('--remove', target)
            self.assertEqual(target.read_bytes(), original)

    def test_create_and_update_only_the_owned_block(self):
        # Spec: docs/specs/task-retrospective.md
        with tempfile.TemporaryDirectory() as directory:
            target = Path(directory) / 'pi' / 'AGENTS.md'
            self.run_install(target)
            self.assertIn('/harness-review', target.read_text())
            target.write_bytes(target.read_bytes().replace(b'Improve harness', b'Old instruction') + b'Tail stays.\n')
            self.run_install(target)
            self.assertNotIn(b'Old instruction', target.read_bytes())
            self.assertTrue(target.read_bytes().endswith(b'Tail stays.\n'))

    def test_symlinks_and_malformed_or_duplicate_blocks_are_untouched(self):
        # Spec: docs/specs/task-retrospective.md
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            target = root / 'AGENTS.md'
            target.write_text('foreign\n')
            link = root / 'linked.md'
            link.symlink_to(target)
            self.run_install(link, ok=False)
            folder = root / 'linked'
            folder.symlink_to(root, target_is_directory=True)
            self.run_install(folder / 'new.md', ok=False)
            self.assertEqual(target.read_text(), 'foreign\n')
            for content in ('<!-- harness-review:start -->\n',
                            '<!-- harness-review:start -->\r\n',
                            '<!-- harness-review:start -->\n<!-- harness-review:end -->',
                            '<!-- harness-review:end -->\n<!-- harness-review:start -->\n'):
                target.write_text(content)
                self.run_install(target, ok=False)
                self.assertEqual(target.read_bytes(), content.encode())
            target.write_text('foreign\n')
            self.run_install(target)
            duplicate = target.read_bytes() * 2
            target.write_bytes(duplicate)
            self.run_install(target, ok=False)
            self.assertEqual(target.read_bytes(), duplicate)

    def test_pointer_is_small_and_does_not_load_full_workflow(self):
        # Spec: docs/specs/task-retrospective.md
        text = ACTIVATION.read_text()
        self.assertLess(len(text.split()), 100)
        for trigger in ('tool errors', 'context searches', 'ambiguous', 'stalls'):
            self.assertIn(trigger, text)
        self.assertIn('review/correction', text)
        self.assertNotIn('task-review', text)


if __name__ == '__main__':
    unittest.main()
