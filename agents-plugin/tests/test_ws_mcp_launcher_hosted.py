"""Hosted launchers restart retained daemon builds, never plugin runtimes."""
import importlib.util
import io
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest import mock

LAUNCHER = Path(__file__).resolve().parents[1] / "bin" / "ws-mcp-launcher.py"


class HostedLauncherTest(unittest.TestCase):
    def setUp(self):
        spec = importlib.util.spec_from_file_location("hosted_launcher", LAUNCHER)
        self.m = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(self.m)
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.env = {"WS_CACHE_HOME": str(self.root), "HOME": str(self.root), "WS_MCP_HOSTED_PACKAGE": "1"}
        self.dir = self.m.proxy_daemon_dir(self.env)
        self.link = self.m.DaemonLink(self.root / "no-plugin-runtime", "0.50.1", "linux", self.env)

    def record(self, content=b"first", version="0.50.7"):
        source = self.root / "source"
        source.write_bytes(content)
        binary = self.m.stage_daemon_binary(source, self.dir / "bin")
        payload = {"schema_version": 1, "executable": str(binary), "version": version,
                   "binary_sha256": self.m.sha256_file(binary), "build_time": 123}
        path = self.dir / self.m.DAEMON_RUNNING_EXECUTABLE_FILE
        path.write_text(json.dumps(payload), encoding="utf-8")
        source.unlink()
        return path, payload

    def test_restart_rereads_the_last_running_build_without_plugin_repair(self):
        _, first = self.record()
        completed = subprocess.CompletedProcess([], 0, "started", "")
        with mock.patch.object(self.m, "read_runtime_metadata", side_effect=AssertionError("plugin metadata read")), \
             mock.patch.object(self.m, "stage_daemon_binary", side_effect=AssertionError("restaging")), \
             mock.patch.object(self.m.subprocess, "run", return_value=completed) as run:
            self.link.repair_runtime = mock.Mock(side_effect=AssertionError("plugin repair"))
            self.assertTrue(self.link.start_daemon())
            self.assertEqual(run.call_args.args[0], [first["executable"], "daemon-start", "--build-time", "123"])
            self.assertNotIn("WS_MCP_HOSTED_PACKAGE", run.call_args.kwargs["env"])
            # Surviving launcher, newer daemon/replacement: the next cold
            # reconnect must not keep its first executable path.
            # Use the unpatched staging helper to emulate the new daemon writer.
        _, latest = self.record(b"second", "0.50.8")
        with mock.patch.object(self.m.subprocess, "run", return_value=completed) as run:
            self.assertTrue(self.link.start_daemon())
            self.assertEqual(run.call_args.args[0][0], latest["executable"])
            self.assertEqual(self.link.binary_version, "0.50.8")
        self.assertTrue(Path(first["executable"]).exists())

    def test_bad_records_refuse_without_spawning_or_downloading(self):
        path, payload = self.record()
        valid = dict(payload)
        invalid = [None, {}, dict(valid, schema_version=2), dict(valid, executable="relative"),
                   dict(valid, binary_sha256="f" * 64), dict(valid, build_time=True)]
        outside = self.root / Path(valid["executable"]).name
        outside.write_bytes(Path(valid["executable"]).read_bytes())
        invalid.append(dict(valid, executable=str(outside)))
        with mock.patch.object(self.m.subprocess, "run", side_effect=AssertionError("spawn")), \
             mock.patch.object(self.m, "install_runtime", side_effect=AssertionError("download")):
            for record in invalid:
                path.write_text(json.dumps(record), encoding="utf-8")
                self.assertFalse(self.link.start_daemon(), record)
                self.assertIn("hosted daemon restart unavailable", self.link.start_error)
            path.unlink()
            self.assertFalse(self.link.start_daemon())
            path.write_text(json.dumps(valid), encoding="utf-8")
            Path(valid["executable"]).write_bytes(b"corrupt")
            self.assertFalse(self.link.start_daemon())

    def test_healthy_daemons_never_upgrade_from_a_hosted_package_contract(self):
        for version in ("0.50.0", "0.50.99", "0.51.0"):
            self.assertFalse(self.link.needs_start({"version": version}))
        external = self.m.DaemonLink(self.root / "external", "0.50.7", "linux", {})
        self.assertTrue(external.needs_start({"version": "0.50.0"}))

    def test_main_uses_only_five_staged_inputs_and_never_repairs_or_waits_for_rsrc(self):
        staged = self.root / "hosted"
        (staged / "bin").mkdir(parents=True)
        contract = json.loads((LAUNCHER.parent.parent / "runtime.json").read_bytes())
        (staged / "runtime.json").write_text(json.dumps(contract), encoding="utf-8")
        stream = io.BytesIO()
        healthy = {"version": contract["plugin_version"]}
        with mock.patch.dict(os.environ, self.env, clear=True), \
             mock.patch.object(self.m, "__file__", str(staged / "bin" / LAUNCHER.name)), \
             mock.patch.object(sys, "argv", [str(LAUNCHER), "serve", "--stdio"]), \
             mock.patch.object(sys, "stdin", mock.Mock(buffer=stream)), \
             mock.patch.object(sys, "stdout", mock.Mock(buffer=stream)), \
             mock.patch.object(self.m.DaemonLink, "probe", return_value=({}, healthy)), \
             mock.patch.object(self.m, "detect_project_root"), \
             mock.patch.object(self.m, "wait_for_rsrc_tree", side_effect=AssertionError("rsrc wait")), \
             mock.patch.object(self.m, "install_runtime", side_effect=AssertionError("runtime repair")), \
             mock.patch.object(self.m, "runtime_fully_compatible", side_effect=AssertionError("plugin binary check")), \
             mock.patch.object(self.m, "local_devenv_runtime_enabled", side_effect=AssertionError("local source discovery")), \
             mock.patch.object(self.m, "proxy_serve", return_value=0) as proxy:
            self.assertEqual(self.m.main(), 0)
            self.assertTrue(proxy.call_args.args[0].hosted_package)
        self.assertFalse((staged / ".runtime").exists())


if __name__ == "__main__":
    unittest.main()
