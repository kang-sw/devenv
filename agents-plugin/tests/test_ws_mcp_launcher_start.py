"""Starter coordination and cold CLI contracts; all executable spawns mocked."""
from contextlib import contextmanager, ExitStack
import importlib.util
import io
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import threading
import time
import unittest
from unittest import mock

LAUNCHER = Path(__file__).resolve().parents[1] / "bin" / "ws-mcp-launcher.py"


def load():
    spec = importlib.util.spec_from_file_location("launcher_start_contract", LAUNCHER)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


class LauncherStartContractTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix="launcher-start.")
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.m = load()
        self.m.DAEMON_SWAP_GRACE_SECONDS = 0.01
        self.m.DAEMON_RETRY_SECONDS = 1.0
        self.binary = self.root / "ws-mcp.exe"
        self.binary.write_bytes(b"not an executable fixture")
        self.m.write_runtime_metadata(self.binary, {"version": "0.50.1"})
        self.env = {"WS_CACHE_HOME": str(self.root), "HOME": str(self.root)}
        self.link = self.new_link()
        self.endpoint = {"port": 1, "token": "t", "pid": 12345}
        self.health = {"version": "0.50.1"}
        self.spawns = {}
        for name in ("run", "call", "Popen"):
            patcher = mock.patch.object(subprocess, name, side_effect=AssertionError("unexpected executable spawn"))
            self.spawns[name] = patcher.start()
            self.addCleanup(patcher.stop)

    def new_link(self):
        return self.m.DaemonLink(self.binary, "0.50.1", "windows", self.env, answer_budget=0.01, start_budget=0.1)

    @contextmanager
    def daemon_lock(self):
        """Hold the Go singleton's byte-zero lock, not the launcher lock."""
        self.link.daemon_dir.mkdir(parents=True, exist_ok=True)
        with (self.link.daemon_dir / "dashboard.lock").open("a+b") as file:
            if os.name == "nt":
                import msvcrt
                file.seek(0)
                msvcrt.locking(file.fileno(), msvcrt.LK_NBLCK, 1)
            else:
                import fcntl
                fcntl.flock(file, fcntl.LOCK_EX | fcntl.LOCK_NB)
            yield

    def test_lock_is_persistent_noninheritable_and_exclusive(self):
        a = self.m.LauncherStartLock(self.link.daemon_dir)
        b = self.m.LauncherStartLock(self.link.daemon_dir)
        try:
            self.assertFalse(os.get_inheritable(a.fd))
            self.assertFalse(os.get_inheritable(b.fd))
            self.assertTrue(a.acquire())
            self.assertFalse(b.acquire())
            path = self.link.daemon_dir / "launcher-start.lock"
            identity = path.stat().st_ino
            # Windows byte locks also refuse reads of the locked byte.
            a.close()
            a = None
            self.assertEqual(path.read_bytes(), b"\0")
            self.assertTrue(b.acquire())
            self.assertEqual(path.stat().st_ino, identity)
        finally:
            if a is not None:
                a.close()
            b.close()
        self.assertTrue(path.exists())
        self.assertFalse((self.link.daemon_dir / "dashboard.lock").exists())

    def test_windows_first_use_initialization_is_inside_lock_ownership(self):
        import errno
        from types import SimpleNamespace
        owner = [None]
        writes = []
        def locking(fd, mode, length):
            self.assertEqual(length, 1)
            if mode == 1:
                if owner[0] is not None:
                    raise OSError(errno.EACCES, "winner owns byte zero")
                owner[0] = fd
            else:
                self.assertEqual(owner[0], fd)
                owner[0] = None
        def write(fd, data):
            if owner[0] != fd:
                raise PermissionError("contender wrote before owning the Windows byte lock")
            writes.append(fd)
            return len(data)
        windows = SimpleNamespace(LK_NBLCK=1, LK_UNLCK=2, locking=locking)
        # Both descriptors see a fresh empty file. An old constructor write
        # before acquire fails deterministically; no native process is spawned.
        with mock.patch.object(self.m.os, "name", "nt"), \
             mock.patch.dict(sys.modules, {"msvcrt": windows}), \
             mock.patch.object(self.m.os, "fstat", return_value=SimpleNamespace(st_size=0)) as stat, \
             mock.patch.object(self.m.os, "write", side_effect=write):
            a = self.m.LauncherStartLock(self.link.daemon_dir)
            b = self.m.LauncherStartLock(self.link.daemon_dir)
            try:
                stat.assert_not_called()
                self.assertEqual(writes, [])
                self.assertTrue(a.acquire())
                self.assertFalse(b.acquire())
                self.assertEqual(writes, [a.fd])
                a.close()
                a = None
                self.assertTrue(b.acquire())
                self.assertEqual(writes[-1], b.fd)
            finally:
                if a is not None:
                    a.close()
                b.close()

    def test_pid_native_conversion_overflow_is_unknown_on_both_platforms(self):
        from types import SimpleNamespace
        for pid in (None, 0, -1, True, "123"):
            self.assertIsNone(self.m.endpoint_process_alive({"pid": pid}))
        for platform_name in ("posix", "nt"):
            with self.subTest(platform=platform_name), \
                 mock.patch.object(self.m.os, "name", platform_name), \
                 mock.patch.object(self.m.os, "kill", side_effect=OverflowError("native PID conversion")) as kill, \
                 mock.patch.dict(sys.modules, {"_winapi": SimpleNamespace(OpenProcess=mock.Mock(side_effect=OverflowError("native PID conversion")))}):
                self.assertIsNone(self.m.endpoint_process_alive({"pid": 10**100}))
                if platform_name == "posix":
                    kill.assert_called_once_with(10**100, 0)
                else:
                    kill.assert_not_called()

    def test_huge_pid_mcp_path_refuses_start_with_controlled_unavailable(self):
        from types import SimpleNamespace
        endpoint = dict(self.endpoint, pid=10**100)
        with self.daemon_lock(), \
             mock.patch.object(self.link, "probe", return_value=(endpoint, None)), \
             mock.patch.object(self.link, "wait_healthy", return_value=(endpoint, None)), \
             mock.patch.object(self.m.os, "kill", side_effect=OverflowError("native PID conversion")), \
             mock.patch.dict(sys.modules, {"_winapi": SimpleNamespace(OpenProcess=mock.Mock(side_effect=OverflowError("native PID conversion")))}), \
             mock.patch.object(self.link, "start_daemon") as starter:
            err = io.StringIO()
            self.assertEqual(self.m.proxy_serve(self.link, ["serve", "--stdio"], io.BytesIO(), io.BytesIO(), err), 1)
            self.assertIn("refusing absence startup", err.getvalue())
            starter.assert_not_called()
        self.assertFalse((self.link.daemon_dir / "bin").exists())

    def test_swap_silence_longer_than_old_grace_waits_for_successor(self):
        for grace in (0, 20):  # initial connect and reconnect
            now = [0.0]
            def probe(timeout):
                return self.endpoint, self.health if now[0] >= 4 else None
            with self.subTest(grace=grace), self.daemon_lock(), \
                 mock.patch.object(self.m.time, "monotonic", side_effect=lambda: now[0]), \
                 mock.patch.object(self.m.time, "sleep", side_effect=lambda seconds: now.__setitem__(0, now[0] + seconds)), \
                 mock.patch.object(self.link, "probe", side_effect=probe), \
                 mock.patch.object(self.link, "start_daemon") as start:
                self.m.DAEMON_RETRY_SECONDS = 75
                self.m.DAEMON_SWAP_GRACE_SECONDS = 20
                self.assertEqual(self.link.ensure_healthy(grace=grace), (self.endpoint, self.health))
                start.assert_not_called()
            self.assertGreaterEqual(now[0], 4)
        self.assertFalse((self.link.daemon_dir / "bin").exists())

    def test_no_successor_starts_once_only_after_the_full_health_window(self):
        now = [0.0]
        started_at = []
        held = ExitStack()
        self.addCleanup(held.close)
        held.enter_context(self.daemon_lock())
        def probe(timeout):
            return self.endpoint, self.health if started_at else None
        def sleep(seconds):
            now[0] += seconds
            if now[0] >= 20:
                held.close()
        def start(*args, **kwargs):
            started_at.append(now[0])
            self.assertGreaterEqual(now[0], 20)
            return True
        with mock.patch.object(self.m.time, "monotonic", side_effect=lambda: now[0]), \
             mock.patch.object(self.m.time, "sleep", side_effect=sleep), \
             mock.patch.object(self.link, "probe", side_effect=probe), \
             mock.patch.object(self.link, "start_daemon", side_effect=start) as starter:
            self.m.DAEMON_RETRY_SECONDS = 75
            self.m.DAEMON_SWAP_GRACE_SECONDS = 20
            self.link.ensure_healthy(grace=20)
            starter.assert_called_once()
        self.assertEqual(started_at, [20])

    def test_cold_start_without_endpoint_skips_swap_grace(self):
        for grace in (0, 20):
            with self.subTest(grace=grace), \
                 mock.patch.object(self.link, "probe", return_value=(None, None)), \
                 mock.patch.object(self.link, "wait_healthy", return_value=(self.endpoint, self.health)) as wait, \
                 mock.patch.object(self.link, "start_daemon", return_value=True) as start:
                self.assertEqual(self.link.ensure_healthy(grace=grace), (self.endpoint, self.health))
                start.assert_called_once()
                wait.assert_called_once_with(self.link.start_budget)

    def test_stale_endpoint_lock_proves_absence_despite_reused_or_unknown_pid(self):
        import errno
        from types import SimpleNamespace
        endpoint = dict(self.endpoint, pid=os.getpid())
        self.link.daemon_dir.mkdir()
        path = self.link.daemon_dir / "endpoint.json"
        path.write_text(json.dumps(endpoint), encoding="utf-8")
        for platform_name, unknown in (("posix", False), ("posix", True), ("nt", True)):
            with self.subTest(platform=platform_name, unknown=unknown), \
                 mock.patch.object(self.m.os, "name", platform_name), \
                 mock.patch.object(self.m.os, "kill", side_effect=PermissionError(errno.EPERM, "denied") if unknown else None), \
                 mock.patch.dict(sys.modules, {"_winapi": SimpleNamespace(OpenProcess=mock.Mock(side_effect=PermissionError("access denied")))}):
                self.assertIs(self.m.endpoint_process_alive(endpoint), None if unknown else True)
            # Use the host's actual file locking, independent of PID-query API.
            started = [False]
            with mock.patch.object(self.link, "probe", return_value=(endpoint, None)), \
                 mock.patch.object(self.m, "endpoint_process_alive", return_value=None if unknown else True), \
                 mock.patch.object(self.link, "wait_healthy", side_effect=lambda budget: (endpoint, self.health if started[0] else None)) as wait:
                def start(*args, **kwargs):
                    with self.daemon_lock():  # probe must release before starter
                        pass
                    started[0] = True
                    return True
                with mock.patch.object(self.link, "start_daemon", side_effect=start) as starter:
                    self.assertEqual(self.link.ensure_healthy(), (endpoint, self.health))
                    starter.assert_called_once()
                wait.assert_called_once_with(self.link.start_budget)
                self.assertEqual(json.loads(path.read_text()), endpoint)

    def test_absence_is_rechecked_under_starter_lock_and_probe_errors_fail_closed(self):
        with mock.patch.object(self.m.os, "open", side_effect=PermissionError("lock denied")):
            self.assertFalse(self.m.daemon_absence_proven(self.link.daemon_dir))
        with mock.patch.object(self.link, "probe", return_value=(self.endpoint, None)), \
             mock.patch.object(self.m, "daemon_absence_proven", side_effect=[True, False]) as absence, \
             mock.patch.object(self.link, "wait_healthy") as wait, \
             mock.patch.object(self.link, "start_daemon") as start:
            with self.assertRaisesRegex(self.m.DaemonUnavailable, "refusing absence startup"):
                self.link.ensure_healthy()
            self.assertEqual(absence.call_count, 2)
            wait.assert_not_called()
            start.assert_not_called()

    def test_malformed_stale_endpoint_does_not_block_start_with_free_singleton(self):
        self.link.daemon_dir.mkdir()
        path = self.link.daemon_dir / "endpoint.json"
        path.write_text("invalid endpoint", encoding="utf-8")
        with mock.patch.object(self.link, "wait_healthy", return_value=(self.endpoint, self.health)) as wait, \
             mock.patch.object(self.link, "start_daemon", return_value=True) as start:
            self.assertEqual(self.link.ensure_healthy(), (self.endpoint, self.health))
            start.assert_called_once()
            wait.assert_called_once_with(self.link.start_budget)
        self.assertEqual(path.read_text(), "invalid endpoint", "only the real starter owns endpoint publication")

    def test_connect_timeout_is_unreachable_but_established_timeout_is_hung(self):
        import http.client
        import socket
        for connected in (False, True):
            with self.subTest(connected=connected):
                conn = mock.Mock()
                if connected:
                    conn.getresponse.side_effect = socket.timeout("accepted but unanswering")
                else:
                    conn.connect.side_effect = socket.timeout("handshake timed out")
                with mock.patch.object(http.client, "HTTPConnection", return_value=conn):
                    if connected:
                        with self.assertRaises(self.m.DaemonHung):
                            self.m.daemon_health(1, 0.02)
                    else:
                        self.assertIsNone(self.m.daemon_health(1, 0.02))
                        conn.request.assert_not_called()
                conn.close.assert_called_once()
        # Other post-connect errors keep their old unreachable result; that
        # result alone never bypasses the singleton lock's absence check.
        conn = mock.Mock()
        conn.getresponse.side_effect = ConnectionResetError("peer reset")
        with mock.patch.object(http.client, "HTTPConnection", return_value=conn):
            self.assertIsNone(self.m.daemon_health(1, 0.02))

    def test_live_unknown_or_malformed_endpoint_never_authorizes_start(self):
        self.link.daemon_dir.mkdir()
        path = self.link.daemon_dir / "endpoint.json"
        for alive in (True, None):
            with self.subTest(alive=alive), self.daemon_lock(), \
                 mock.patch.object(self.link, "probe", return_value=(self.endpoint, None)), \
                 mock.patch.object(self.link, "wait_healthy", return_value=(self.endpoint, None)), \
                 mock.patch.object(self.m, "endpoint_process_alive", return_value=alive), \
                 mock.patch.object(self.link, "start_daemon") as start:
                path.write_text(json.dumps(self.endpoint), encoding="utf-8")
                before = path.read_bytes()
                with self.assertRaisesRegex(self.m.DaemonUnavailable, "refusing absence startup"):
                    self.link.ensure_healthy()
                self.assertEqual(path.read_bytes(), before)
                start.assert_not_called()
        path.write_text("invalid endpoint", encoding="utf-8")
        with self.daemon_lock(), \
             mock.patch.object(self.link, "probe", return_value=(None, None)), \
             mock.patch.object(self.link, "wait_healthy", return_value=(None, None)):
            with self.assertRaisesRegex(self.m.DaemonUnavailable, "refusing absence startup"):
                self.link.ensure_healthy()
        self.assertFalse((self.link.daemon_dir / "bin").exists())

    def test_dead_pid_start_holds_lock_through_healthy_successor(self):
        state = [False]
        def probe(timeout):
            return self.endpoint, self.health if state[0] else None
        def start(argv=None, **kwargs):
            other = self.m.LauncherStartLock(self.link.daemon_dir)
            try:
                self.assertFalse(other.acquire())
            finally:
                other.close()
            state[0] = True
            return True
        with mock.patch.object(self.link, "probe", side_effect=probe), \
             mock.patch.object(self.m, "endpoint_process_alive", return_value=False), \
             mock.patch.object(self.link, "start_daemon", side_effect=start) as starter:
            self.assertEqual(self.link.ensure_healthy(), (self.endpoint, self.health))
            starter.assert_called_once()
        other = self.m.LauncherStartLock(self.link.daemon_dir)
        try:
            self.assertTrue(other.acquire())
        finally:
            other.close()

    def test_lock_recheck_and_losers_use_network_only(self):
        held = self.m.LauncherStartLock(self.link.daemon_dir)
        self.assertTrue(held.acquire())
        calls = [0]
        def probe(timeout):
            calls[0] += 1
            return (self.endpoint, self.health) if calls[0] >= 3 else (None, None)
        try:
            with mock.patch.object(self.link, "probe", side_effect=probe), \
                 mock.patch.object(self.link, "wait_healthy", return_value=(None, None)), \
                 mock.patch.object(self.link, "start_daemon") as start:
                self.assertEqual(self.link.ensure_healthy(), (self.endpoint, self.health))
                start.assert_not_called()
        finally:
            held.close()
        # Even when the lock is available, its health recheck precedes staging.
        with mock.patch.object(self.link, "probe", side_effect=[(None, None), (self.endpoint, self.health)]), \
             mock.patch.object(self.link, "wait_healthy", return_value=(None, None)), \
             mock.patch.object(self.link, "start_daemon") as start:
            self.link.ensure_healthy()
            start.assert_not_called()

    def test_hung_grace_is_not_converted_into_absence(self):
        with mock.patch.object(self.link, "probe", side_effect=self.m.DaemonHung("wedged")), \
             mock.patch.object(self.link, "start_daemon") as start:
            for grace in (0, 0.01):
                with self.assertRaisesRegex(self.m.DaemonHung, "wedged"):
                    self.link.ensure_healthy(grace=grace)
            start.assert_not_called()

    def test_failure_releases_lock_and_never_touches_singleton_or_endpoint(self):
        with mock.patch.object(self.link, "probe", return_value=(None, None)), \
             mock.patch.object(self.link, "wait_healthy", return_value=(None, None)), \
             mock.patch.object(self.link, "start_daemon", return_value=False):
            with self.assertRaises(self.m.DaemonUnavailable):
                self.link.ensure_healthy()
        lock = self.m.LauncherStartLock(self.link.daemon_dir)
        try:
            self.assertTrue(lock.acquire())
        finally:
            lock.close()
        self.assertFalse((self.link.daemon_dir / "dashboard.lock").exists())
        self.assertFalse((self.link.daemon_dir / "endpoint.json").exists())

    def test_cli_initial_and_retry_have_no_start_authority_even_on_start_capable_link(self):
        self.m.DAEMON_RETRY_SECONDS = 0.01
        self.m.DAEMON_RETRY_POLL_SECONDS = 0.001
        for argv in (["mailbox", "wait"], ["mailbox", "wait", "--stream"], ["tools"], ["call", "tickets.create_empty", "{}"]):
            with self.subTest(argv=argv), \
                 mock.patch.object(self.link, "probe", return_value=(None, None)), \
                 mock.patch.object(self.link, "start_daemon") as start, \
                 mock.patch.object(self.m, "stage_daemon_binary") as stage:
                err = io.BytesIO()
                self.assertEqual(self.m.proxy_cli(self.link, argv, io.BytesIO(), err), 1)
                self.assertIn(b"daemon not running", err.getvalue())
                start.assert_not_called()
                stage.assert_not_called()
        self.assertFalse(self.link.daemon_dir.exists())

    def test_failed_copy_removes_only_its_own_temp(self):
        directory = self.root / "bin"
        directory.mkdir()
        foreign = directory / ".ws-mcp-dashboard-other.tmp"
        foreign.write_bytes(b"another attempt")
        for operation in ("copyfile", "chmod", "replace"):
            with self.subTest(operation=operation):
                owner = self.m.shutil if operation == "copyfile" else self.m.os
                with mock.patch.object(owner, operation, side_effect=OSError("copy failed")):
                    with self.assertRaises(OSError):
                        self.m.stage_daemon_binary(self.binary, directory)
                self.assertEqual(list(directory.iterdir()), [foreign])

    def test_replacement_only_for_newer_core_or_explicit_equal_core(self):
        for health, argv, starts in (({"version": "0.50.1-dev", "content_hash": "other"}, None, False),
                                     ({"version": "0.50.2"}, ["daemon-start", "--replace"], False),
                                     ({"version": "0.50.1"}, ["daemon-start", "--replace"], True),
                                     ({"version": "0.50.0"}, None, True)):
            with self.subTest(health=health, argv=argv), \
                 mock.patch.object(self.link, "probe", return_value=(self.endpoint, health)), \
                 mock.patch.object(self.link, "wait_healthy", return_value=(self.endpoint, self.health)), \
                 mock.patch.object(self.link, "start_daemon", return_value=True) as start:
                self.link.ensure_healthy(lifecycle_argv=argv)
                self.assertEqual(start.called, starts)


@unittest.skipUnless(os.environ.get("WS_LAUNCHER_PROCESS_TESTS") == "1", "remote-only process fixture")
class LauncherStarterProcessesTest(unittest.TestCase):
    def test_concurrent_starters_and_crash_release(self):
        # Invoked only by the remote Go family. The child executes Python,
        # never a ws executable; its daemon-start subprocess API is mocked.
        with tempfile.TemporaryDirectory(prefix="launcher-processes.") as tmp:
            root = Path(tmp)
            children = [subprocess.Popen([sys.executable, __file__, "--starter-child", str(root)]) for _ in range(6)]
            try:
                for child in children:
                    self.assertEqual(child.wait(timeout=10), 0)
                self.assertEqual((root / "starts").read_text(), "start\n")
                self.assertTrue((root / "dashboard" / "launcher-start.lock").exists())
                crash = subprocess.Popen([sys.executable, __file__, "--crash-child", str(root)])
                self.assertEqual(crash.wait(timeout=10), 23)
                lock = load().LauncherStartLock(root / "dashboard")
                try:
                    self.assertTrue(lock.acquire())
                finally:
                    lock.close()
            finally:
                for child in children:
                    if child.poll() is None:
                        child.kill()
                    child.wait()


def starter_child(root, crash=False):
    m = load()
    if crash:
        lock = m.LauncherStartLock(root / "dashboard")
        assert lock.acquire()
        os._exit(23)
    binary = root / "fixture.bin"
    # All workers can create the same non-executable bytes before taking the
    # lock; only the winner's mocked starter can publish the healthy marker.
    binary.touch(exist_ok=True)
    m.DAEMON_SWAP_GRACE_SECONDS = 0.01
    link = m.DaemonLink(binary, "0.50.1", "linux", {"WS_CACHE_HOME": str(root)})
    endpoint = {"port": 1, "token": "fake", "pid": os.getpid()}
    link.probe = lambda timeout: (endpoint, {"version": "0.50.1"}) if (root / "healthy").exists() else (None, None)
    def run(*args, **kwargs):
        with (root / "starts").open("a") as f:
            f.write("start\n")
        time.sleep(0.2)
        (root / "healthy").touch()
        return subprocess.CompletedProcess([], 0, "{}", "")
    with mock.patch.object(m, "read_runtime_metadata", return_value={"version": "0.50.1"}), \
         mock.patch.object(m, "stage_daemon_binary", return_value=binary), \
         mock.patch.object(subprocess, "run", side_effect=run), \
         mock.patch.object(subprocess, "Popen", side_effect=AssertionError("unexpected spawn")), \
         mock.patch.object(subprocess, "call", side_effect=AssertionError("unexpected spawn")):
        link.ensure_healthy()


if __name__ == "__main__":
    if len(sys.argv) > 1 and sys.argv[1] in {"--starter-child", "--crash-child"}:
        starter_child(Path(sys.argv[2]), sys.argv[1] == "--crash-child")
    else:
        unittest.main()
