"""Tests for scripts/dev.py runtime seeding. Run: python -m unittest scripts/test_dev.py"""
import json
import os
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock

sys.path.insert(0, str(Path(__file__).resolve().parent))
import dev  # noqa: E402


class SeedPackageTest(unittest.TestCase):
    def setUp(self):
        self.launcher = dev.load_launcher()
        self._tmp = tempfile.TemporaryDirectory(prefix="dev-seed-", dir=os.environ.get("DEV_TEST_TMP"))
        self.addCleanup(self._tmp.cleanup)
        self.root = Path(self._tmp.name).resolve()
        (self.root / "tool" / "cmd" / "ws-mcp").mkdir(parents=True)
        (self.root / "tool" / "cmd" / "ws-mcp" / "main.go").write_text("package main\n")
        self.go = self.root / "go.exe"
        self.go.write_text("")
        self.plugin = self.root / "pkg"
        self.plugin.mkdir()
        (self.plugin / "runtime.json").write_text(json.dumps({"plugin_version": "1.2.3"}))
        self.built = self.root / "built.exe"
        self.built.write_bytes(b"binary")

    def write_marker(self):
        (self.plugin / ".local-devenv-runtime").write_text(json.dumps({
            "schema_version": 1,
            "source_root": str(self.root),
            "tool_dir": str(self.root / "tool"),
            "go": str(self.go),
        }))

    def test_seed_makes_stamp_current(self):
        self.write_marker()
        result = dev.seed_package(self.launcher, self.plugin, self.built)
        self.assertTrue(result.startswith("seeded"), result)
        os_name = self.launcher.host_os()
        contract_path = self.plugin / "runtime.json"
        contract = self.launcher.read_runtime_contract(contract_path)
        runtime_dir = self.plugin / ".runtime" / f"{os_name}-{self.launcher.host_arch()}"
        binary = runtime_dir / self.launcher.runtime_binary_name(contract, contract_path, os_name)
        self.assertEqual(binary.read_bytes(), b"binary")
        fingerprint = self.launcher.local_devenv_source_fingerprint(self.plugin, os_name)
        self.assertTrue(self.launcher.compatibility_stamp_current(binary, contract, contract_path, runtime_dir, fingerprint))

    def test_no_marker_untouched(self):
        result = dev.seed_package(self.launcher, self.plugin, self.built)
        self.assertEqual(result, "not local-devenv")
        self.assertFalse((self.plugin / ".runtime").exists())

    def test_locked_target_is_skipped(self):
        self.write_marker()
        with mock.patch.object(dev.os, "replace", side_effect=PermissionError("in use")):
            result = dev.seed_package(self.launcher, self.plugin, self.built)
        self.assertTrue(result.startswith("skipped"), result)
        runtime_dir = next((self.plugin / ".runtime").iterdir())
        self.assertEqual([p.name for p in runtime_dir.iterdir()], [])


if __name__ == "__main__":
    unittest.main()
