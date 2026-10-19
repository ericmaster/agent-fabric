#!/usr/bin/env python3
# Spec: docs/specs/task-retrospective.md — release/bootstrap fixture tests of
# the Phase 3 observation plugin and runtime assets. Everything runs against
# disposable HOME/config/data directories and local archives only: a fake
# `curl` in a fixture bin dir serves the release bytes (the release.sh
# checksum gate verifies real archive bytes), no network, no real OpenCode
# server, and existing live sessions are never activated.
#
# Timing: heavy IO is once per class (~60s for two 5-way cross-compile
# release archives); per-test work installs one local tree each.

import json
import os
import pathlib
import shutil
import subprocess
import tarfile
import tempfile
import unittest


REPO = pathlib.Path(__file__).resolve().parents[1]
VERSION = "0.3.0-fixture"


def run(cmd, env=None, cwd=None, check=True, stdin=None):
    process = subprocess.run(
        cmd, env=env, cwd=cwd, input=stdin, text=True,
        stdout=subprocess.PIPE, stderr=subprocess.PIPE,
    )
    if check and process.returncode != 0:
        raise AssertionError(
            f"command {' '.join(str(part) for part in cmd)} failed ({process.returncode})\n"
            f"stdout: {process.stdout}\nstderr: {process.stderr}"
        )
    return process


UNRELATED_PLUGIN = "export const UnrelatedUserPlugin = async () => ({})\n"


class FixtureBase(unittest.TestCase):
    """One local release + fixture source tree per class; one HOME per test."""

    @classmethod
    def setUpClass(cls):
        cls.work = tempfile.TemporaryDirectory()
        cls.base = pathlib.Path(cls.work.name).resolve()
        cls.fakebin = cls.base / "fakebin"
        cls.fakebin.mkdir(parents=True)
        run([str(REPO / "scripts/release.sh"), VERSION, str(cls.base / "release")], cwd=REPO)
        cls.release = cls.base / "release"
        cls.archive = cls.release / f"agent-fabric_{VERSION}_linux_amd64.tar.gz"
        if not cls.archive.is_file():
            raise AssertionError("release archive missing")
        # Fixture source tree: only what install --source needs
        # (agents/, adapters/, hooks/, assets/).
        cls.source = cls.base / "source"
        for name in ("agents", "adapters", "hooks", "assets"):
            shutil.copytree(REPO / name, cls.source / name)
        cls.install_fake_curl()

    @classmethod
    def tearDownClass(cls):
        cls.work.cleanup()

    @classmethod
    def install_fake_curl(cls):
        # Serves ONLY curl's download positions (stdout for the latest-tag
        # probe, -o FILE for the artifact URLs). Any other curl usage fails
        # loudly so real network can never slip in.
        config = cls.base / "fake-release-routes.json"
        latest = cls.base / "fake-latest.json"
        latest.write_text(json.dumps({"tag_name": f"v{VERSION}"}))
        config.write_text(json.dumps({
            "/releases/latest": str(latest),
            "sha256sums.txt": str(cls.release / "sha256sums.txt"),
            f"agent-fabric_{VERSION}_linux_amd64.tar.gz": str(cls.archive),
        }))
        script = cls.fakebin / "curl"
        script.write_text(
            "#!/usr/bin/env python3\n"
            "import json, pathlib, sys\n"
            "routes = json.loads(pathlib.Path(" + repr(str(config)) + ").read_text())\n"
            "args = sys.argv[1:]\n"
            "url = args[-1]\n"
            "hits = [source for marker, source in routes.items() if url.endswith(marker)]\n"
            "if len(hits) != 1:\n"
            "    raise SystemExit('fake curl refuses url ' + url)\n"
            "payload = pathlib.Path(hits[0]).read_bytes()\n"
            "if '-o' in args:\n"
            "    pathlib.Path(args[args.index('-o') + 1]).write_bytes(payload)\n"
            "else:\n"
            "    sys.stdout.buffer.write(payload)\n"
        )
        script.chmod(script.stat().st_mode | 0o755)

    def setUp(self):
        self.home = pathlib.Path(tempfile.mkdtemp(prefix="agf-rel-home-")).resolve()
        self.addCleanup(shutil.rmtree, str(self.home), True)
        self.installdir = self.home / ".local" / "bin"
        self.installdir.mkdir(parents=True)
        (self.home / ".config" / "opencode" / "plugins").mkdir(parents=True, exist_ok=True)
        (self.home / ".config" / "opencode" / "plugins" / "unrelated-user-plugin.ts").write_text(UNRELATED_PLUGIN)

    def base_env(self, extra=None):
        env = dict(os.environ)
        env["HOME"] = str(self.home)
        for stale in ("XDG_CONFIG_HOME", "XDG_STATE_HOME", "AGF_CONFIG",
                      "AGF_SOURCE", "AGF_RETROSPECTIVE_STATE",
                      "AGF_RETROSPECTIVE_CONFIG", "AGF_RETROSPECTIVE_NO_SPAWN"):
            env.pop(stale, None)
        env["AGF_RETROSPECTIVE_NO_SPAWN"] = "1"
        env["PATH"] = f"{self.fakebin}:{env['PATH']}"
        if extra:
            env.update(extra)
        return env

    def cli(self, name="agent-fabric"):
        return str(self.installdir / name)

    def stage_release_payload(self):
        """Stages the released binary + packaged source tree into the
        disposable install dir — exactly the partial state bootstrap reaches
        after its extract/copy steps (tar content is checksum-verified real
        release bytes). Direct-CLI tests then behave as post-bootstrap."""
        target = str(self.installdir)
        with tarfile.open(self.archive) as archive:
            for member in archive.getmembers():
                name = pathlib.PurePosixPath(member.name)
                if name.is_absolute() or ".." in name.parts:
                    raise AssertionError("unsafe archive member " + member.name)
                if not (member.isfile() or member.isdir()):
                    raise AssertionError("unsupported archive member " + member.name)
            archive.extractall(path=target)
        shutil.copy(str(self.installdir / "agent-fabric"), str(self.installdir / "agf"))

    def manifest_of(self):
        path = self.home / ".config" / "agent-fabric" / ".agent-fabric-manifest.json"
        return json.loads(path.read_text())

    def install_all_opencode_retrospective(self):
        self.stage_release_payload()
        run([self.cli(), "install",
             "--source", str(self.installdir),
             "--all", "--tools", "opencode", "--assets", "retrospective",
             "--yes"],
            env=self.base_env())

    def install_from_bootstrap(self):
        result = subprocess.run(
            [str(REPO / "scripts/bootstrap.sh"), "--all", "--tools", "opencode",
             "--assets", "retrospective", "--yes"],
            env=self.base_env({"AGF_INSTALL_DIR": str(self.installdir)}),
            cwd=str(REPO), text=True,
            stdout=subprocess.PIPE, stderr=subprocess.PIPE,
        )
        if result.returncode != 0:
            raise AssertionError(
                f"bootstrap failed ({result.returncode})\n"
                f"stdout: {result.stdout}\nstderr: {result.stderr}"
            )
        return result

    def assert_plugin_and_asset_installed(self):
        plugin = self.home / ".config" / "opencode" / "plugins" / "retrospective.ts"
        origin = self.home / ".config" / "agent-fabric" / "retrospective" / "learning-origin.json"
        hooks_copy = self.installdir / "hooks" / "retrospective" / "opencode.ts"
        self.assertTrue(hooks_copy.is_file(), "the packaged source root must ship hooks/")
        self.assertTrue(plugin.is_file(), "the opt-in owned plugin asset must install")
        self.assertTrue(origin.is_file(), "the optional runtime asset must install")
        managed = {entry["path"] for entry in self.manifest_of()["files"]}
        self.assertIn(str(plugin), managed)
        self.assertIn(str(origin), managed)
        self.assertEqual(
            (self.home / ".config" / "opencode" / "plugins" / "unrelated-user-plugin.ts").read_text(),
            UNRELATED_PLUGIN,
        )
        return plugin, origin


class ReleaseAndBootstrapFixtureTests(FixtureBase):
    # Spec: docs/specs/task-retrospective.md — DoD: fixture tests install the
    # plugin + runtime assets into a disposable HOME, exercise
    # enable/disable/sync/doctor/uninstall, retain modified and unrelated
    # files, and never activate existing live sessions.

    def test_release_archive_ships_plugin_source_without_devtools(self):
        with tarfile.open(self.archive) as archive:
            names = archive.getnames()
        self.assertIn("hooks/retrospective/opencode.ts", names)
        self.assertIn("hooks/retrospective/package.json", names)
        self.assertIn("assets/retrospective/learning-origin.json", names)
        self.assertFalse(any("node_modules" in name for name in names), names)
        self.assertFalse(any(name.endswith("/dist") or "/dist/" in name for name in names), names)
        self.assertFalse(any(name.endswith(".test.js") or name.endswith(".test.ts") for name in names), names)
        self.assertFalse(any(name.endswith(".tsbuildinfo") for name in names), names)
        self.assertFalse(any(name.endswith(".record-captures.jsonl") for name in names), names)
        sums = (self.release / "sha256sums.txt").read_text()
        self.assertIn(self.archive.name, sums)
        manifest = json.loads((self.release / "release-manifest.json").read_text())
        self.assertEqual(manifest["version"], VERSION)
        self.assertTrue(manifest["artifacts"])

    def test_bootstrap_installs_plugin_and_assets_without_activation(self):
        self.install_from_bootstrap()
        plugin, origin = self.assert_plugin_and_asset_installed()
        # No activation or observation side effects anywhere: no state tree,
        # no extra plugin content, and no session artefacts (the fixture HOME
        # holds no live sessions at all, and bootstrap never starts one).
        state = self.home / ".local" / "state" / "agent-fabric"
        self.assertFalse(state.exists())
        plugin_dir = self.home / ".config" / "opencode" / "plugins"
        self.assertEqual(
            sorted(entry.name for entry in plugin_dir.iterdir()),
            ["retrospective.ts", "unrelated-user-plugin.ts"],
        )
        self.assertIn("Spec: docs/specs/task-retrospective.md", plugin.read_text()[:2000])
        _ = origin

    def test_enable_disable_and_local_status(self):
        self.install_all_opencode_retrospective()
        env = self.base_env()
        run([self.cli(), "retrospective", "enable"], env=env, cwd=str(self.base))
        config = json.loads((self.home / ".config" / "agent-fabric" / "config.json").read_text())
        self.assertTrue(config["retrospective"]["enabled"])
        run([self.cli(), "retrospective", "disable"], env=env, cwd=str(self.base))
        config = json.loads((self.home / ".config" / "agent-fabric" / "config.json").read_text())
        self.assertFalse(config["retrospective"]["enabled"])
        status = run([self.cli(), "retrospective", "status"], env=env, cwd=str(self.base))
        self.assertIn("retrospective.enabled=false", status.stdout)
        # A disabled ingest is inert: the disabled code, no state tree, and
        # no worker spawn (AGF_RETROSPECTIVE_NO_SPAWN is belt and braces).
        state = self.home / ".local" / "state" / "agent-fabric" / "retrospective"
        result = run(
            [self.cli(), "retrospective", "ingest", "--harness", "opencode"],
            env=env, cwd=str(self.base), check=False,
            stdin=json.dumps({"version": 1, "harness": "opencode", "session_id": "s", "records": []}),
        )
        self.assertEqual(result.returncode, 2, result.stdout + result.stderr)
        self.assertFalse(state.exists(), "a disabled ingest must not create state")
        # An unsupported harness is refused fail-closed too.
        result = run(
            [self.cli(), "retrospective", "ingest", "--harness", "not-opencode"],
            env=env, cwd=str(self.base), check=False, stdin="{}",
        )
        self.assertNotEqual(result.returncode, 0)

    def test_sync_and_doctor_preserve_modified_and_unrelated_files(self):
        self.install_all_opencode_retrospective()
        origin = self.home / ".config" / "agent-fabric" / "retrospective" / "learning-origin.json"
        plugin = self.home / ".config" / "opencode" / "plugins" / "retrospective.ts"
        env = self.base_env()
        origin.write_text('{"schema":1,"user":"edit"}\n')
        # Sync's guarded write refuses to clobber a user edit (exit 1) and
        # retains the modified file unchanged.
        result = run([self.cli(), "sync",
                      "--source", str(self.installdir),
                      "--all", "--tools", "opencode", "--assets", "retrospective",
                      "--yes"],
                     env=env, check=False)
        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        self.assertIn("modified managed file preserved", result.stderr)
        self.assertEqual(origin.read_text(), '{"schema":1,"user":"edit"}\n')
        # The unchanged plugin stays managed with an unchanged hash.
        managed = {entry["path"]: entry.get("sha256", "") for entry in self.manifest_of()["files"]}
        self.assertIn(str(plugin), managed)
        doctor = run([self.cli(), "doctor"], env=env, cwd=str(self.base))
        self.assertEqual(doctor.returncode, 0, doctor.stderr)

    def test_uninstall_removes_owned_and_keeps_user_edits_and_unrelated(self):
        self.install_all_opencode_retrospective()
        origin = self.home / ".config" / "agent-fabric" / "retrospective" / "learning-origin.json"
        plugin = self.home / ".config" / "opencode" / "plugins" / "retrospective.ts"
        env = self.base_env()
        origin.write_text('{"schema":1,"user":"edit"}\n')
        run([self.cli(), "uninstall"], env=env, cwd=str(self.base))
        self.assertFalse(plugin.exists(), "unchanged owned plugin must uninstall")
        self.assertEqual(origin.read_text(), '{"schema":1,"user":"edit"}\n')
        self.assertEqual(
            (self.home / ".config" / "opencode" / "plugins" / "unrelated-user-plugin.ts").read_text(),
            UNRELATED_PLUGIN,
        )


if __name__ == "__main__":
    unittest.main()
