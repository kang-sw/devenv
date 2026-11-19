"""Tests for scripts/dev.py seeding and mocked explicit lifecycle coordination."""
from contextlib import ExitStack
import io
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
        # Every process API, not only subprocess: the identity comes from metadata alone.
        os_spawns = {name: mock.DEFAULT for name in (
            "system", "popen", "posix_spawn", "posix_spawnp", "execv", "execve", "execvp", "execvpe",
            "spawnv", "spawnve", "spawnvp", "spawnvpe", "startfile") if hasattr(dev.os, name)}
        with mock.patch.object(dev, "RUNTIME_CONTRACT", self.plugin / "runtime.json"), \
             mock.patch.multiple(dev.subprocess, run=mock.DEFAULT, Popen=mock.DEFAULT, call=mock.DEFAULT,
                                 check_call=mock.DEFAULT, check_output=mock.DEFAULT) as processes, \
             mock.patch.multiple(dev.os, **os_spawns) as os_processes, \
             mock.patch("builtins.print") as output:
            dev.check_built(self.launcher, self.built)
        for spy in [*processes.values(), *os_processes.values()]:
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

    def test_local_build_generates_before_go_compilation(self):
        events = []
        launcher = mock.Mock()
        launcher.host_os.return_value = 'linux'
        launcher.generate_package_assets.side_effect = lambda tool: events.append('generate')
        launcher.source_runtime_metadata.return_value = {'version': '1.2.3'}
        launcher.local_devenv_build_env.return_value = {}
        def fake_build(argv, **kwargs):
            self.assertEqual(events, ['generate'])
            self.assertEqual(argv[1], 'build')
            events.append('build')
            return mock.Mock(returncode=0)
        with mock.patch.object(dev.shutil, 'which', return_value='fake-go'), \
             mock.patch.object(dev.subprocess, 'run', side_effect=fake_build):
            dev.build_local(launcher, self.root)
        self.assertEqual(events, ['generate', 'build'])
        launcher.generate_package_assets.assert_called_once_with(dev.TOOL_DIR)
        launcher.write_runtime_metadata.assert_called_once()

    def test_remote_build_fetches_identity_and_never_runs_ws_binary(self):
        def fake_run(argv, **kwargs):
            self.assertIn("--fetch", argv)
            self.assertIn(f"{dev.REMOTE_DIST}/{dev.binary_name(self.launcher)}.metadata.json={self.root.as_posix()}", argv)
            self.assertIn("scripts/write-runtime-metadata.py", argv[-1])
            self.assertLess(argv[-1].index('scripts/generate-package-assets.py'), argv[-1].index('go build'))
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
        self.assertEqual(run.call_args.args[0], [str(self.built), "daemon-start", "--replace", "--build-time", "123"])

    def test_rerun_identical_or_changed_content_uses_only_daemon_start_for_the_drain(self):
        digest = self.launcher.sha256_file(self.built)
        for old_hash in (digest, "other-content"):
            with self.subTest(old_hash=old_hash), \
                 mock.patch.dict(os.environ, {"WS_CACHE_HOME": str(self.root / "cache"), "WS_CONFIG_HOME": str(self.root / "config")}), \
                 mock.patch.object(dev, "load_launcher", return_value=self.launcher), \
                 mock.patch.object(dev, "build", return_value=self.built), \
                 mock.patch.object(dev, "check_built"), \
                 mock.patch.object(dev, "seed_packages") as seed, \
                 mock.patch.object(self.launcher, "stage_daemon_binary", return_value=self.built) as stage, \
                 mock.patch.object(dev, "current_health", side_effect=[({"token": "fake"}, {"pid": 10, "content_hash": old_hash}), ({"token": "new"}, {"pid": 11, "content_hash": digest})]) as health, \
                 mock.patch.object(dev.subprocess, "run", return_value=mock.Mock(returncode=0, stdout='{"outcome":"replaced"}', stderr="")) as run, \
                 mock.patch("urllib.request.urlopen", side_effect=AssertionError("no separate HTTP shutdown")):
                self.assertEqual(dev.rerun_daemon(local=True), 0)
                seed.assert_called_once()
                stage.assert_called_once()
                self.assertEqual(health.call_count, 2)
                run.assert_called_once()
                self.assertEqual(run.call_args.args[0][:3], [str(self.built), "daemon-start", "--replace"])
                self.assertIn("--build-time", run.call_args.args[0])

    def test_daemon_start_points_rsrc_root_at_checkout_package(self):
        # The daemon resolves the Pi worker launcher as <dir(WS_RSRC_ROOT)>/bin/ws-mcp-launcher.py.
        stale = {"WS_RSRC_ROOT": "/stale/cache/rsrc", "WS_SKILLS_ROOT": "/stale/cache/skills"}
        with mock.patch.dict(dev.os.environ, stale),                 mock.patch.object(dev.subprocess, "run", return_value=mock.Mock(returncode=0, stdout='{"outcome":"started"}', stderr="")) as run:
            dev.run_daemon_start(self.launcher, self.built, 123)
        env = run.call_args.kwargs["env"]
        self.assertEqual(env["WS_RSRC_ROOT"], str(dev.DAEMON_PACKAGE / "rsrc"))
        self.assertEqual(env["WS_SKILLS_ROOT"], str(dev.DAEMON_PACKAGE / "skills"))
        self.assertTrue((Path(env["WS_RSRC_ROOT"]).parent / "bin" / "ws-mcp-launcher.py").is_file())
        self.assertTrue(Path(env["WS_SKILLS_ROOT"]).is_dir())

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


class DevLifecycleTest(unittest.TestCase):
    """No executable fixtures, native PID queries, builds or live endpoints."""

    def setUp(self):
        self.launcher = dev.load_launcher()
        self.tmp = tempfile.TemporaryDirectory(prefix="dev-lifecycle-", dir=os.environ.get("DEV_TEST_TMP"))
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.binary = self.root / "candidate.bin"  # File IO and identity are mocked too.
        self.digest = "expected-content-hash"
        self.endpoint = {"port": 1, "token": "test-only", "pid": 12345}
        self.expected = {"version": "0.50.1-dev", "content_hash": self.digest, "pid": 12345}
        self.old = dict(self.expected, content_hash="old-build")
        self.now = 0.0
        self.health = self.old
        self.out, self.err = io.StringIO(), io.StringIO()
        self.stack = ExitStack()
        self.addCleanup(self.stack.close)
        patch = lambda *args, **kwargs: self.stack.enter_context(mock.patch(*args, **kwargs))
        self.stack.enter_context(mock.patch.dict(os.environ, {
            "WS_CACHE_HOME": str(self.root / "cache"), "WS_CONFIG_HOME": str(self.root / "config"),
        }))
        patch("dev.load_launcher", return_value=self.launcher)
        patch("dev.time.monotonic", side_effect=lambda: self.now)
        self.sleep = patch("dev.time.sleep", side_effect=self.advance)
        patch("sys.stdout", self.out)
        patch("sys.stderr", self.err)
        self.Lock = self.launcher.LauncherStartLock
        self.factory = self.stack.enter_context(mock.patch.object(self.launcher, "LauncherStartLock", side_effect=self.Lock))
        self.build = patch("dev.build", side_effect=self.fake_build)
        patch("dev.check_built")
        self.seed = patch("dev.seed_packages")
        self.stack.enter_context(mock.patch.object(self.launcher, "sha256_file", return_value=self.digest))
        self.stage = self.stack.enter_context(mock.patch.object(self.launcher, "stage_daemon_binary", return_value=self.binary))
        self.stack.enter_context(mock.patch.object(self.launcher, "read_daemon_endpoint", return_value=self.endpoint))
        self.rest = self.stack.enter_context(mock.patch.object(self.launcher, "daemon_health", side_effect=self.fake_health))
        self.stack.enter_context(mock.patch.object(self.launcher, "endpoint_process_alive", side_effect=AssertionError("no PID query")))
        for name in ("run", "Popen", "call", "check_call", "check_output"):
            spy = patch(f"subprocess.{name}", side_effect=AssertionError("unexpected executable spawn"))
            if name == "run":
                self.run = spy
        for name in ("kill", "system", "popen", "posix_spawn", "posix_spawnp", "execv", "execve", "execvp",
                     "execvpe", "spawnv", "spawnve", "spawnvp", "spawnvpe", "startfile"):
            if hasattr(os, name):
                patch(f"os.{name}", side_effect=AssertionError("no native PID/process API"))
        if os.name == "nt":
            patch("_winapi.OpenProcess", side_effect=AssertionError("no native PID query"))
        patch("http.client.HTTPConnection", side_effect=AssertionError("no live HTTP"))
        patch("urllib.request.urlopen", side_effect=AssertionError("no live HTTP"))
        self.run.side_effect = self.successful_start
        self.daemon_dir = self.launcher.proxy_daemon_dir(dict(os.environ))

    def advance(self, seconds):
        self.assertGreater(seconds, 0)
        self.now += seconds

    def fake_build(self, *args):
        self.factory.assert_not_called()  # Builds finish before the gate/deadline.
        return self.binary

    def fake_health(self, port, timeout):
        self.assertEqual(port, 1)
        self.assertGreater(timeout, 0)
        self.assertLessEqual(timeout, 3)
        self.assert_gate_held()
        return self.health

    def result(self, code=0, stdout='{"outcome":"replaced"}', stderr=""):
        return mock.Mock(returncode=code, stdout=stdout, stderr=stderr)

    def successful_start(self, *args, **kwargs):
        self.assert_gate_held()
        self.health = self.expected
        return self.result()

    def assert_gate_held(self):
        contender = self.Lock(self.daemon_dir)
        try:
            self.assertFalse(contender.acquire())
        finally:
            contender.close()

    def assert_gate_released(self):
        contender = self.Lock(self.daemon_dir)
        try:
            self.assertTrue(contender.acquire())
        finally:
            contender.close()
        self.assertFalse((self.daemon_dir / "dashboard.lock").exists())
        self.assertFalse((self.daemon_dir / "endpoint.json").exists())

    def test_explicit_replace_build_outside_gate_and_one_starter(self):
        self.assertEqual(dev.rerun_daemon(local=False), 0)
        self.run.assert_called_once()
        self.assertEqual(self.run.call_args.args[0][:3], [str(self.binary), "daemon-start", "--replace"])
        self.assertIn("--build-time", self.run.call_args.args[0])
        self.assertEqual(self.run.call_args.kwargs["timeout"], 60)
        env = self.run.call_args.kwargs["env"]
        self.assertEqual(env["WS_RSRC_ROOT"], str(dev.DAEMON_PACKAGE / "rsrc"))
        self.assertEqual(env["WS_SKILLS_ROOT"], str(dev.DAEMON_PACKAGE / "skills"))
        self.seed.assert_called_once()
        self.assertEqual(self.rest.call_count, 2)
        self.assert_gate_released()

    def test_unavailable_recovers_only_expected_hash_and_keeps_diagnostic(self):
        def start(*args, **kwargs):
            self.assert_gate_held()
            self.advance(12)
            return self.result(1, 'notice\n{"outcome":"unavailable","pid":0}', "successor not yet ready")
        self.run.side_effect = start
        self.rest.side_effect = lambda port, timeout: self.fake_health(port, timeout) if self.now < 25 else self.expected_while_held()
        self.assertEqual(dev.rerun_daemon(local=True), 0)
        self.assertGreaterEqual(self.now, 25)
        self.assertLess(self.now, 60)
        self.run.assert_called_once()
        self.assertIn('daemon-start exited 1: notice\n{"outcome":"unavailable","pid":0}', self.out.getvalue())
        self.assertIn("successor not yet ready", self.out.getvalue())
        self.assertIn("expected build confirmed over REST", self.out.getvalue())
        self.assert_gate_released()

    def expected_while_held(self):
        self.assert_gate_held()
        return self.expected

    def test_unavailable_old_hash_no_health_and_pid_only_exhaust_original_budget(self):
        for health in (self.old, None, {"pid": 12345}):
            with self.subTest(health=health):
                self.now = 0
                self.health = health
                self.run.reset_mock()
                self.run.side_effect = lambda *a, **kw: self.result(1, '{"outcome":"unavailable"}', "original diagnostic")
                self.assertEqual(dev.rerun_daemon(local=True), 1)
                self.assertEqual(self.now, 60)
                self.run.assert_called_once()
                self.assertIn("lifecycle deadline", self.err.getvalue())
                self.assertIn("original diagnostic", self.out.getvalue())
                self.assert_gate_released()
                self.factory.reset_mock()

    def test_malformed_or_other_failure_never_reconciles_even_if_expected_build_answers(self):
        cases = ((1, "not-json"), (1, "[]"), (1, "null"), (1, "{}"), (1, '{"outcome":1}'),
                 (1, '{"outcome":"unavailable"}\ntrailing junk'), (1, '{"outcome":"replaced"}'),
                 (2, '{"outcome":"unavailable"}'), (-1, '{"outcome":"unavailable"}'), (0, "not-json"))
        for code, stdout in cases:
            with self.subTest(code=code, stdout=stdout):
                self.run.reset_mock()
                self.rest.reset_mock()
                def start(*args, **kwargs):
                    self.health = self.expected
                    return self.result(code, stdout, "preserved failure")
                self.run.side_effect = start
                with self.assertRaisesRegex(RuntimeError, "daemon-start") as error:
                    dev.rerun_daemon(local=True)
                self.assertIn(stdout, str(error.exception))
                self.assertEqual(self.rest.call_count, 1)  # Only the pre-start observation.
                self.run.assert_called_once()
                self.assert_gate_released()
                self.factory.reset_mock()

    def test_starter_timeout_or_exception_releases_gate_without_retry(self):
        for error in (dev.subprocess.TimeoutExpired("mock starter", 60), OSError("starter failed")):
            with self.subTest(error=error):
                self.run.reset_mock()
                self.run.side_effect = error
                with self.assertRaises(type(error)):
                    dev.rerun_daemon(local=True)
                self.run.assert_called_once()
                self.assert_gate_released()
                self.factory.reset_mock()

    def test_hung_before_or_after_start_is_not_absence_or_recovery(self):
        for before_start in (True, False):
            with self.subTest(before_start=before_start):
                self.run.reset_mock()
                self.rest.side_effect = [self.launcher.DaemonHung("wedged")] if before_start else [self.old, self.launcher.DaemonHung("wedged")]
                with self.assertRaisesRegex(RuntimeError, "running daemon is wedged"):
                    dev.rerun_daemon(local=True)
                self.assertEqual(self.run.call_count, 0 if before_start else 1)
                self.assert_gate_released()
                self.factory.reset_mock()

    def test_gate_contention_times_out_without_any_starter(self):
        owner = self.Lock(self.daemon_dir)
        self.assertTrue(owner.acquire())
        try:
            with self.assertRaisesRegex(RuntimeError, "deadline expired waiting.*starter lock"):
                dev.rerun_daemon(local=True)
            self.assertEqual(self.now, 60)
            self.run.assert_not_called()
            self.rest.assert_not_called()
            self.assert_gate_held()  # A loser must not release the owner's lock.
        finally:
            owner.close()
        self.assert_gate_released()

    def test_contention_and_unavailable_confirmation_share_one_deadline(self):
        owner = self.Lock(self.daemon_dir)
        self.assertTrue(owner.acquire())
        def sleep(seconds):
            self.advance(seconds)
            if self.now >= 5 and owner.fd is not None:
                owner.close()
                owner.fd = None
        def start(*args, **kwargs):
            self.assertLessEqual(kwargs["timeout"], 55)
            self.advance(50)
            return self.result(1, '{"outcome":"unavailable"}')
        self.sleep.side_effect = sleep
        self.run.side_effect = start
        try:
            self.assertEqual(dev.rerun_daemon(local=True), 1)
            self.assertEqual(self.now, 60)  # Not 65 (lock) or 115 (new post-start budget).
            self.run.assert_called_once()
        finally:
            if owner.fd is not None:
                owner.close()
        self.assert_gate_released()

    def test_expired_starter_budget_cannot_accept_even_expected_health(self):
        def start(*args, **kwargs):
            self.health = self.expected
            self.advance(kwargs["timeout"])
            return self.result(1, '{"outcome":"unavailable"}')
        self.run.side_effect = start
        self.assertEqual(dev.rerun_daemon(local=True), 1)
        self.assertEqual(self.now, 60)
        self.assertEqual(self.rest.call_count, 1)
        self.run.assert_called_once()
        self.assert_gate_released()

    def test_health_returning_at_deadline_is_not_late_success(self):
        def health(port, timeout):
            self.fake_health(port, timeout)
            if self.run.called:
                self.advance(60 - self.now)
                return self.expected
            return self.old
        self.rest.side_effect = health
        self.assertEqual(dev.rerun_daemon(local=True), 1)
        self.assertEqual(self.now, 60)
        self.run.assert_called_once()
        self.assert_gate_released()

    def test_slow_pre_start_health_cannot_reset_deadline_or_invoke_starter(self):
        def health(port, timeout):
            self.advance(60)
            return self.old
        self.rest.side_effect = health
        with self.assertRaisesRegex(RuntimeError, "deadline expired before daemon-start"):
            dev.rerun_daemon(local=True)
        self.run.assert_not_called()
        self.assert_gate_released()

    def test_dev_held_gate_blocks_mcp_past_full_grace_until_successor_publication(self):
        self.launcher.DAEMON_SWAP_GRACE_SECONDS = 20  # Production grace, not shortened.
        self.launcher.DAEMON_RETRY_SECONDS = 75
        link = self.launcher.DaemonLink(self.binary, "0.50.1", "windows", dict(os.environ))
        observed = []
        def probe(timeout):
            self.assert_gate_held()
            observed.append(self.now)
            if self.now >= 25:
                self.health = self.expected
                return self.endpoint, self.expected
            return None, None
        def start(*args, **kwargs):
            with mock.patch.object(link, "probe", side_effect=probe), \
                 mock.patch.object(link, "start_daemon", side_effect=AssertionError("no contender starter")) as extra:
                self.assertEqual(link.ensure_healthy(grace=20), (self.endpoint, self.expected))
                extra.assert_not_called()
            self.assert_gate_held()  # Dev still owns it at publication and report parsing.
            return self.result(1, '{"outcome":"unavailable"}')
        self.run.side_effect = start
        self.assertEqual(dev.rerun_daemon(local=True), 0)
        self.assertTrue(any(20 < seen < 25 for seen in observed))
        self.assertGreaterEqual(self.now, 25)
        self.run.assert_called_once()
        self.stage.assert_called_once()  # Dev staging only, no contender staging.
        self.assert_gate_released()


if __name__ == "__main__":
    unittest.main()
