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
        self.go.chmod(0o755)
        self.plugin = self.root / "pkg"
        self.plugin.mkdir()
        (self.plugin / "runtime.json").write_text(json.dumps({"plugin_version": "1.2.3"}))
        self.built = self.root / "built.exe"
        self.built.write_bytes(b"binary")
        self.launcher.write_runtime_metadata(self.built, {"version": "1.2.9-dev", "source_commit": "dev"})

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
        self.assertEqual(self.launcher.read_runtime_metadata(binary), self.launcher.read_runtime_metadata(self.built))
        stamp = json.loads(self.launcher.compatibility_stamp_path(runtime_dir).read_text())
        self.assertEqual(stamp["accepted_runtime_version"], "1.2.9-dev")
        self.assertEqual(stamp["binary_sha256"], self.launcher.sha256_file(binary))
        fingerprint = self.launcher.local_devenv_source_fingerprint(self.plugin, os_name)
        self.assertTrue(self.launcher.compatibility_stamp_current(binary, contract, contract_path, runtime_dir, fingerprint))

    def test_dev_check_built_reads_metadata_without_ws_process_spawn(self):
        with mock.patch.object(dev, "RUNTIME_CONTRACT", self.plugin / "runtime.json"), \
             mock.patch.multiple(dev.subprocess, run=mock.DEFAULT, Popen=mock.DEFAULT, call=mock.DEFAULT) as processes, \
             mock.patch("builtins.print") as output:
            dev.check_built(self.launcher, self.built)
        for spy in processes.values():
            spy.assert_not_called()
        output.assert_any_call("built version: 1.2.9-dev")
        output.assert_any_call("launcher compatibility gate: accepted")

    def test_check_built_rejects_changed_content_without_probe(self):
        self.built.write_bytes(b"changed build")
        with mock.patch.object(dev.subprocess, "run") as run:
            with self.assertRaisesRegex(RuntimeError, "no valid hash-bound metadata"):
                dev.check_built(self.launcher, self.built)
            run.assert_not_called()

    def test_seed_refuses_metadata_free_or_incompatible_build(self):
        self.write_marker()
        self.launcher.runtime_metadata_path(self.built).unlink()
        self.assertIn("metadata missing", dev.seed_package(self.launcher, self.plugin, self.built))
        self.launcher.write_runtime_metadata(self.built, {"version": "1.3.0"})
        self.assertIn("incompatible", dev.seed_package(self.launcher, self.plugin, self.built))
        self.assertFalse((self.plugin / ".runtime").exists())

    def test_seed_same_content_repairs_missing_identity(self):
        self.write_marker()
        self.assertTrue(dev.seed_package(self.launcher, self.plugin, self.built).startswith("seeded"))
        binary = next((self.plugin / ".runtime").glob("*/*metadata.json"))
        binary.unlink()
        with mock.patch.object(dev.shutil, "copyfile") as copy:
            self.assertTrue(dev.seed_package(self.launcher, self.plugin, self.built).startswith("seeded"))
            copy.assert_not_called()
        self.assertTrue(binary.is_file())

    def test_remote_build_fetches_identity_and_never_runs_ws_binary(self):
        def fake_run(argv, **kwargs):
            self.assertIn("--fetch", argv)
            self.assertIn(f"{dev.REMOTE_DIST}/{dev.binary_name(self.launcher)}.metadata.json={self.root.as_posix()}", argv)
            self.assertIn("scripts/write-runtime-metadata.py", argv[-1])
            out = self.root / dev.binary_name(self.launcher)
            out.write_bytes(b"cross-build")
            self.launcher.write_runtime_metadata(out, {"version": "1.2.9-dev"})
            return mock.Mock(returncode=0)

        with mock.patch.object(dev.subprocess, "run", side_effect=fake_run) as run:
            out = dev.build_remote(self.launcher, self.root)
        self.assertEqual(run.call_count, 1)
        self.assertEqual(self.launcher.read_runtime_metadata(out)["version"], "1.2.9-dev")

    def test_explicit_daemon_start_is_still_the_lifecycle_exception(self):
        with mock.patch.object(dev.subprocess, "run", return_value=mock.Mock(returncode=0, stdout='{"outcome":"started"}', stderr="")) as run:
            self.assertEqual(dev.run_daemon_start(self.launcher, self.built, 123), {"outcome": "started"})
        self.assertEqual(run.call_args.args[0], [str(self.built), "daemon-start", "--build-time", "123"])

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
