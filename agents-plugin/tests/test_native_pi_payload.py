"""Offline distribution, corruption, drift and transactional snapshot regressions."""
import importlib.util
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

REPO = Path(__file__).resolve().parents[2]
sys.dont_write_bytecode = True


def load(path, name):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


PAYLOAD = load(REPO / "agents-plugin/bin/ws-pi-payload.py", "payload")
PRODUCER = load(REPO / "agents-plugin-pi/scripts/prepare-native-payload.py", "producer")


class NativePiPayloadTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="ws-native-test-")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name).resolve()
        self.env = dict(os.environ)
        for key, name in (("HOME", "home"), ("USERPROFILE", "home"),
                          ("WS_CACHE_HOME", "cache"), ("WS_CONFIG_HOME", "config"),
                          ("npm_config_cache", "npm-cache")):
            folder = self.root / name
            folder.mkdir(exist_ok=True)
            self.env[key] = str(folder)
        self.env["PYTHONDONTWRITEBYTECODE"] = "1"

    def owner(self, product="agents-plugin"):
        dest = self.root / product
        for name in ("native-pi", "bin", "rsrc", "skills", "runtime.json", "dashboard-config-manifest.json", ".claude-plugin"):
            source = REPO / product / name
            target = dest / name
            target.parent.mkdir(parents=True, exist_ok=True)
            if source.is_dir():
                shutil.copytree(source, target, ignore=shutil.ignore_patterns("__pycache__"))
            else:
                shutil.copyfile(source, target)
        return dest

    def test_both_archives_and_materializers_agree(self):
        for name in ("native-pi/manifest.json", "native-pi/payload.zip", "bin/ws-pi-payload.py"):
            self.assertEqual((REPO / "agents-plugin" / name).read_bytes(), (REPO / "agents-plugin-wsflow" / name).read_bytes())
        for product in ("agents-plugin", "agents-plugin-wsflow"):
            with self.subTest(product=product):
                manifest = PAYLOAD.check_source(REPO / product, REPO)
                self.assertEqual(manifest["dependencies"], json.loads((REPO / "agents-plugin-pi/package.json").read_text())["dependencies"])

    def test_materialized_owner_bytes_and_offline_reuse(self):
        for product, identity in (("agents-plugin", "ws"), ("agents-plugin-wsflow", "wsflow")):
            owner = self.owner(product)
            # Text translation on Windows must not change the hashed marker.
            original = Path.write_text
            def windows_text(path, data, encoding=None, errors=None, newline=None):
                return original(path, data, encoding=encoding, errors=errors, newline="\r\n" if newline is None else newline)
            with patch.object(Path, "write_text", windows_text):
                control = self.root / "control"
                control.write_text("control\n")
                self.assertEqual(control.read_bytes(), b"control\r\n")
                first = PAYLOAD.materialize(owner, self.root / "native-cache")
                self.assertEqual(first, PAYLOAD.materialize(owner, self.root / "native-cache"))
            self.assertEqual((first / "workflow-owner.json").read_bytes(), ('{"plugin": "' + identity + '"}\n').encode())
            self.assertEqual(PAYLOAD.tree(first / "skills"), PAYLOAD.tree(owner / "skills"))
            self.assertEqual((first / "dist/eff_large_wordlist.txt").read_bytes(), (REPO / "agents-plugin-pi/src/eff_large_wordlist.txt").read_bytes())
            self.assertEqual(json.loads((first / "runtime.json").read_text())["plugin"], identity)
            (first / "dist/index.js").write_bytes(b"corrupt")
            with self.assertRaisesRegex(ValueError, "corrupt"):
                PAYLOAD.materialize(owner, self.root / "native-cache")

    def source_fixture(self):
        repo = self.root / "repo"
        manifest = json.loads((REPO / "agents-plugin/native-pi/manifest.json").read_text())
        for name in manifest["inputs"]:
            target = repo / name
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(REPO / name, target)
        owner = self.owner()
        shutil.copytree(owner, repo / "agents-plugin", dirs_exist_ok=True)
        return repo

    def test_source_dependency_generator_and_skill_drift_rejected(self):
        repo = self.source_fixture()
        owner = repo / "agents-plugin"
        PAYLOAD.check_source(owner, repo)
        for name in ("agents-plugin-pi/src/index.ts", "agents-plugin-pi/package.json",
                     "agents-plugin-pi/package-lock.json", "agents-plugin-pi/scripts/prepare-native-payload.py",
                     "agents-plugin/skills/lead-run/SKILL.md", "agents-plugin-wsflow/skills/lead-run/SKILL.md"):
            path = repo / name
            original = path.read_bytes()
            path.write_bytes(original + b"\n")
            with self.subTest(input=name), self.assertRaisesRegex(ValueError, "stale"):
                PAYLOAD.check_source(owner, repo)
            path.write_bytes(original)
        added = repo / "agents-plugin-pi/src/new-input.ts"
        added.write_text("// new input\n")
        with self.assertRaisesRegex(ValueError, "input added"):
            PAYLOAD.check_source(owner, repo)

    def test_producer_rejects_embedded_checkout_paths_before_preparation(self):
        repo = self.source_fixture()
        bundle = repo / "agents-plugin-pi/dist/index.js"
        bundle.write_bytes(bundle.read_bytes() + str(repo).encode())
        before = PAYLOAD.tree(repo / "agents-plugin/native-pi")
        with patch.object(PRODUCER.subprocess, "run") as npm:
            with self.assertRaisesRegex(ValueError, "producer path"):
                PRODUCER.generate(repo)
            npm.assert_not_called()
        self.assertEqual(PAYLOAD.tree(repo / "agents-plugin/native-pi"), before)

    def test_failed_producer_preserves_both_usable_artifacts(self):
        repo = self.source_fixture()
        shutil.copytree(REPO / "agents-plugin-wsflow/native-pi", repo / "agents-plugin-wsflow/native-pi")
        before = {p: PAYLOAD.tree(repo / p / "native-pi") for p in ("agents-plugin", "agents-plugin-wsflow")}
        # No npm process is executed by this failure injection.
        with patch.object(PRODUCER.subprocess, "run", side_effect=RuntimeError("preparation failed")):
            with self.assertRaisesRegex(RuntimeError, "preparation failed"):
                PRODUCER.generate(repo)
        for product, inventory in before.items():
            self.assertEqual(inventory, PAYLOAD.tree(repo / product / "native-pi"))

    def test_snapshot_bodies_two_updates_and_invalid_input_rollback(self):
        shell = (REPO / "install.sh").read_text().split("<<'SNAPSHOTEOF'\n", 1)[1].split("\nSNAPSHOTEOF", 1)[0]
        windows = (REPO / "scripts/install-claude-plugin.ps1").read_text().split("$snapshotCode = @'\n", 1)[1].split("\n'@", 1)[0]
        repo = self.source_fixture()
        marker = b'{"schema_version":1,"source_root":"fixture-only"}\n'
        (repo / "agents-plugin/.local-devenv-runtime").write_bytes(marker)
        for kind, body in (("shell", shell), ("windows-python", windows)):
            with self.subTest(kind=kind):
                cache = self.root / kind / "snapshot"
                argv = [sys.executable, "-I", "-B", "-c", body, str(repo), str(cache)]
                if kind == "windows-python":
                    argv.append(str(self.root / "fixture-go.exe"))
                for _ in range(2):
                    result = subprocess.run(argv, env=self.env, capture_output=True, text=True, timeout=60)
                    self.assertEqual(result.returncode, 0, result.stderr)
                    self.assertFalse((cache / "wsflow").exists())
                    self.assertEqual(json.loads((cache / ".claude-plugin/marketplace.json").read_text())["plugins"][0]["source"], "./ws")
                    PAYLOAD.validate(cache / "ws")
                    self.assertFalse((cache / "ws/stale-destination-file").exists())
                    actual = (cache / "ws/.local-devenv-runtime").read_bytes()
                    self.assertFalse(actual.startswith(b"\xef\xbb\xbf"))
                    if kind == "shell":
                        self.assertEqual(actual, marker)
                    else:
                        self.assertEqual(json.loads(actual)["go"], argv[-1])
                    (cache / "ws/stale-destination-file").write_text("stale")
                before = PAYLOAD.tree(cache)
                archive = repo / "agents-plugin/native-pi/payload.zip"
                original = archive.read_bytes()
                archive.write_bytes(b"broken")
                result = subprocess.run(argv, env=self.env, capture_output=True, text=True, timeout=60)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn("checksum mismatch", result.stderr)
                self.assertEqual(PAYLOAD.tree(cache), before)
                archive.write_bytes(original)
                # Repeated update removed the stale file from the first pass.
                (cache / "ws/stale-destination-file").unlink()
                self.assertEqual(list(cache.parent.glob(".ws-snapshot-*")), [])

    def test_windows_dry_run_keeps_all_python_and_cli_execution_gated(self):
        script = (REPO / "scripts/install-claude-plugin.ps1").read_text()
        self.assertIn("if ($DryRun) { return }", script)
        self.assertIn("Invoke-Py $snapshotCode @($RepoDir, $PluginCache, $GoPath)", script)
        self.assertIn("if (-not $DryRun) {\n        & claude plugin install", script)
        self.assertNotIn("Copy-Item", script)


if __name__ == "__main__":
    unittest.main()
