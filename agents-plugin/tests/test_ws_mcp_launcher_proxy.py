"""The launcher as a proxy to the daemon-hosted MCP server.

A fake daemon speaks the daemon's side of the MCP route (the /health answer,
the HTTP Upgrade, the hello and accept lines, then newline JSON-RPC) and a
fake runtime binary plays `daemon-start`, direct `serve --stdio`, and direct
`mailbox wait`, so each path the launcher takes is observable without a Go
build: daemon present, absent, unanswering, cross-minor, replaced under a
live session, and gone with no successor.
"""

import importlib.util
import io
import json
import os
import socket
import stat
import sys
import tempfile
import threading
import time
import unittest
from unittest import mock
from pathlib import Path

LAUNCHER_PATH = Path(__file__).resolve().parents[1] / "bin" / "ws-mcp-launcher.py"

# The other launcher test modules replace time.sleep on the shared module and
# never restore it; these tests need real sleeps (the fake daemon's slow call,
# the proxy's polling), so the one captured at import is put back per test.
_REAL_SLEEP = time.sleep

FAKE_RUNTIME = """#!/usr/bin/env python3
import json, os, sys, time
args = sys.argv[1:]
cache = os.environ["WS_CACHE_HOME"]
daemon_dir = os.path.join(cache, "dashboard")
with open(os.path.join(cache, "fake-runtime.log"), "a", encoding="utf-8") as log:
    log.write(json.dumps({"exe": sys.argv[0], "args": args, "env_marker": os.environ.get("FAKE_MARKER", ""), "holder_python": os.environ.get("WS_MCP_HOLDER_PYTHON")}) + "\\n")
if args[:1] == ["daemon-start"]:
    try:
        target = json.load(open(os.path.join(cache, "fake-daemon.json"), encoding="utf-8"))
    except OSError:
        print("daemon-start: no fake daemon to point at", file=sys.stderr)
        sys.exit(1)
    if target.get("fail"):
        print("daemon-start: told to fail", file=sys.stderr)
        sys.exit(1)
    os.makedirs(daemon_dir, exist_ok=True)
    with open(os.path.join(daemon_dir, "endpoint.json"), "w", encoding="utf-8") as fh:
        json.dump({"port": target["port"], "pid": 4242, "version": target["version"], "token": target["token"], "started_at": "2026-11-10T00:00:00Z"}, fh)
    print(json.dumps({"outcome": "started", "port": target["port"]}))
    sys.exit(0)
if args == ["serve", "--stdio"]:
    for line in sys.stdin:
        msg = json.loads(line)
        with open(os.path.join(cache, "fake-runtime.log"), "a", encoding="utf-8") as log:
            log.write(json.dumps({"recv": msg.get("method"), "id": msg.get("id")}) + "\\n")
        if "id" in msg and "method" in msg:
            print(json.dumps({"jsonrpc": "2.0", "id": msg["id"], "result": {"served_by": "direct", "method": msg["method"]}}), flush=True)
    sys.exit(0)
if args[:2] == ["mailbox", "wait"]:
    print(json.dumps({"served_by": "direct", "args": args[2:]}))
    sys.exit(3)
sys.exit(2)
"""


def load_launcher():
    spec = importlib.util.spec_from_file_location("ws_mcp_launcher", LAUNCHER_PATH)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


class FakeDaemon:
    """The daemon's side of /health and the MCP route, on a loopback port."""

    def __init__(self, *, version="0.48.16", build_time=2_000_000_000_000, name="daemon", refuse_reason="", slow_methods=(), slow_seconds=30.0, lan=False, cli_exit_code=7, content_hash="", drop_mcp=False, health_status=200, health_version=True):
        self.version = version
        self.health_status = health_status
        self.health_version = health_version
        self.build_time = build_time
        # content_hash: reported in /health when set. drop_mcp: an accepted
        # MCP stream is closed at once, as a daemon in a crash loop would.
        self.content_hash = content_hash
        self.drop_mcp = drop_mcp
        self.slow_seconds = slow_seconds
        self.name = name
        self.refuse_reason = refuse_reason
        self.slow_methods = set(slow_methods)
        self.lan = lan
        # None: the final frame carries no exit_code at all.
        self.cli_exit_code = cli_exit_code
        self.token = "f" * 64
        self.hellos = []
        self.requests = []  # (method, id) per hosted request
        self.conns = []
        self.lock = threading.Lock()
        self.listener = socket.socket()
        self.listener.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        self.listener.bind(("127.0.0.1", 0))
        self.listener.listen(8)
        self.port = self.listener.getsockname()[1]
        self.closed = False
        self.accept_thread = threading.Thread(target=self.accept_loop, daemon=True)
        self.accept_thread.start()

    def endpoint(self):
        return {"port": self.port, "pid": 4242, "version": self.version, "token": self.token, "started_at": "2026-11-10T00:00:00Z"}

    def write_endpoint(self, daemon_dir: Path):
        daemon_dir.mkdir(parents=True, exist_ok=True)
        (daemon_dir / "endpoint.json").write_text(json.dumps(self.endpoint()), encoding="utf-8")

    def accept_loop(self):
        while not self.closed:
            try:
                conn, _ = self.listener.accept()
            except OSError:
                return
            with self.lock:
                self.conns.append(conn)
            threading.Thread(target=self.handle, args=(conn,), daemon=True).start()

    def handle(self, conn):
        reader = conn.makefile("rb")
        try:
            head = reader.readline().decode()
            headers = {}
            while True:
                line = reader.readline()
                if not line or line == b"\r\n":
                    break
                key, _, value = line.decode().partition(":")
                headers[key.strip().lower()] = value.strip()
            parts = head.split()
            path = parts[1] if len(parts) > 1 else ""
            if path == "/health":
                health = {"service": "ws-dashboard", "version": self.version, "pid": 4242, "port": self.port, "started_at": "x", "lan": self.lan, "build_time": self.build_time}
                if self.content_hash:
                    health["content_hash"] = self.content_hash
                if not self.health_version:
                    health.pop("version")
                body = json.dumps(health).encode()
                conn.sendall(f"HTTP/1.1 {self.health_status} Test\r\n".encode() + b"Content-Type: application/json\r\nContent-Length: " + str(len(body)).encode() + b"\r\nConnection: close\r\n\r\n" + body)
                return
            if path != "/mcp":
                conn.sendall(b"HTTP/1.1 404 Not Found\r\nContent-Length: 0\r\n\r\n")
                return
            if headers.get("authorization") != "Bearer " + self.token:
                conn.sendall(b"HTTP/1.1 401 Unauthorized\r\nContent-Length: 0\r\n\r\n")
                return
            if headers.get("upgrade") != "ws-mcp":
                conn.sendall(b"HTTP/1.1 426 Upgrade Required\r\nContent-Length: 0\r\n\r\n")
                return
            conn.sendall(b"HTTP/1.1 101 Switching Protocols\r\nUpgrade: ws-mcp\r\nConnection: Upgrade\r\n\r\n")
            hello = json.loads(reader.readline())
            with self.lock:
                self.hellos.append(hello)
            if self.refuse_reason:
                conn.sendall(json.dumps({"ok": False, "reason": self.refuse_reason}).encode() + b"\n")
                return
            conn.sendall(json.dumps({"ok": True, "version": self.version, "pid": 4242}).encode() + b"\n")
            if hello.get("mode") == "cli":
                self.serve_cli(conn, hello)
                return
            if self.drop_mcp:
                return
            self.serve_mcp(conn, reader, hello)
        except (OSError, ValueError):
            return
        finally:
            try:
                conn.close()
            except OSError:
                pass

    def serve_cli(self, conn, hello):
        conn.sendall(json.dumps({"stream": "stderr", "data": "fake: waiting\n"}).encode() + b"\n")
        result = {"served_by": self.name, "args": hello["argv"][2:], "owner_pid": hello["owner_pid"]}
        final = {"final": True, "data": json.dumps(result) + "\n"}
        if self.cli_exit_code is not None:
            final["exit_code"] = self.cli_exit_code
        conn.sendall(json.dumps(final).encode() + b"\n")

    def serve_mcp(self, conn, reader, hello):
        while True:
            line = reader.readline()
            if not line:
                return
            msg = json.loads(line)
            method = msg.get("method")
            with self.lock:
                self.requests.append((method, msg.get("id")))
            if method == "notify-me":
                conn.sendall(json.dumps({"jsonrpc": "2.0", "method": "notifications/message", "params": {"from": self.name}}).encode() + b"\n")
            if "id" not in msg or method is None:
                continue
            if method in self.slow_methods:
                time.sleep(self.slow_seconds)
            result = {"served_by": self.name, "method": method}
            if method == "initialize":
                result["serverInfo"] = {"name": "ws", "version": self.version}
            if method == "tools/list":
                result["tools"] = [{"name": "runtime.read"}] + ([{"name": "extra.tool"}] if self.version != "0.48.16" else [])
            if method == "whoami":
                result["allowed"] = hello["env"].get("WS_MCP_ALLOWED_TOOLS", "")
            conn.sendall(json.dumps({"jsonrpc": "2.0", "id": msg["id"], "result": result}).encode() + b"\n")

    def close(self):
        self.closed = True
        # shutdown wakes the blocked accept, so the port really closes now
        # rather than when that accept returns.
        for closer in (lambda: self.listener.shutdown(socket.SHUT_RDWR), self.listener.close):
            try:
                closer()
            except OSError:
                pass
        self.accept_thread.join(2)
        with self.lock:
            conns = list(self.conns)
        for conn in conns:
            try:
                conn.shutdown(socket.SHUT_RDWR)
            except OSError:
                pass
            try:
                conn.close()
            except OSError:
                pass


class ProxyClient:
    """A client of proxy_serve over pipes, run on its own thread."""

    def __init__(self, launcher, link, argv=("serve", "--stdio")):
        r_in, w_in = os.pipe()
        r_out, w_out = os.pipe()
        self.stdin = os.fdopen(r_in, "rb")
        self.stdout = os.fdopen(w_out, "wb", buffering=0)
        self.writer = os.fdopen(w_in, "wb", buffering=0)
        self.reader = os.fdopen(r_out, "rb")
        self.stderr = io.StringIO()
        self.result = {}
        self.thread = threading.Thread(
            target=lambda: self.result.__setitem__("code", launcher.proxy_serve(link, list(argv), self.stdin, self.stdout, self.stderr)),
            daemon=True,
        )
        self.thread.start()
        self.next_id = 0

    def send(self, method, *, notification=False, params=None):
        msg = {"jsonrpc": "2.0", "method": method}
        if params is not None:
            msg["params"] = params
        if not notification:
            self.next_id += 1
            msg["id"] = self.next_id
        self.writer.write(json.dumps(msg).encode() + b"\n")
        return msg.get("id")

    def recv(self, timeout=5.0):
        box = {}

        def read():
            box["line"] = self.reader.readline()

        t = threading.Thread(target=read, daemon=True)
        t.start()
        t.join(timeout)
        if t.is_alive() or not box.get("line"):
            raise AssertionError("no line from the proxy within the timeout")
        return json.loads(box["line"])

    def close_stdin(self):
        try:
            self.writer.close()
        except OSError:
            pass

    def close(self, timeout=5.0):
        self.close_stdin()
        self.thread.join(timeout)
        if self.thread.is_alive():
            raise AssertionError("proxy_serve did not return after stdin closed")
        return self.result.get("code")


class LauncherProxyTest(unittest.TestCase):
    def setUp(self):
        time.sleep = _REAL_SLEEP
        self.launcher = load_launcher()
        # Exercise exhaustion without spending the production 75s per test.
        self.launcher.DAEMON_RETRY_SECONDS = 0.3
        self.launcher.DAEMON_RETRY_POLL_SECONDS = 0.05
        self.tmp = tempfile.TemporaryDirectory(prefix="ws-launcher-proxy.")
        self.addCleanup(self.tmp.cleanup)
        # Resolved: the launcher reports real paths, and macOS's temp dir
        # is a /var symlink to /private/var.
        self.root = Path(self.tmp.name).resolve()
        self.cache = self.root / "cache"
        self.cache.mkdir()
        self.daemon_dir = self.cache / "dashboard"
        self.binary = self.root / "ws-mcp-0.48.16-abcdef123456"
        self.binary.write_text(FAKE_RUNTIME, encoding="utf-8")
        self.binary.chmod(self.binary.stat().st_mode | stat.S_IXUSR | stat.S_IXGRP | stat.S_IXOTH)
        os.utime(self.binary, ns=(2_000_000_000_000_000_000, 2_000_000_000_000_000_000))  # build time 2_000_000_000_000 ms
        self.launcher.write_runtime_metadata(self.binary, {"version": "0.48.16"})
        self.env = {
            "PATH": os.environ.get("PATH", ""),
            "HOME": str(self.root),
            "WS_CACHE_HOME": str(self.cache),
            "WS_MCP_ALLOWED_TOOLS": "runtime.read,todo.list",
            "WS_RSRC_ROOT": str(self.root / "rsrc"),
            "WS_MCP_PROJECT_ROOT": str(self.root / "project"),
            "FAKE_MARKER": "from-launcher",
        }
        self.daemons = []

    def tearDown(self):
        for daemon in self.daemons:
            daemon.close()

    def daemon(self, **kwargs):
        daemon = FakeDaemon(**kwargs)
        self.daemons.append(daemon)
        return daemon

    def point_start_at(self, daemon=None, *, fail=False):
        """What the fake daemon-start writes into endpoint.json."""
        payload = {"fail": True} if fail else {"port": daemon.port, "token": daemon.token, "version": daemon.version}
        (self.cache / "fake-daemon.json").write_text(json.dumps(payload), encoding="utf-8")

    def link(self, **kwargs):
        kwargs.setdefault("answer_budget", 1.0)
        kwargs.setdefault("start_budget", 3.0)
        return self.launcher.DaemonLink(self.binary, "0.48.16", "linux", self.env, **kwargs)

    def wait_for_hello(self, daemon, timeout=5.0):
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            if daemon.hellos:
                return
            time.sleep(0.02)
        raise AssertionError("the proxy never reconnected to the successor")

    def runtime_log(self):
        path = self.cache / "fake-runtime.log"
        if not path.is_file():
            return []
        return [json.loads(line) for line in path.read_text(encoding="utf-8").splitlines() if line]

    # --- the paths before traffic -------------------------------------------------

    def test_a_present_daemon_hosts_the_session_without_running_the_binary(self):
        daemon = self.daemon()
        daemon.write_endpoint(self.daemon_dir)
        client = ProxyClient(self.launcher, self.link())
        init_id = client.send("initialize", params={"clientInfo": {"name": "t"}})
        self.assertEqual(client.recv()["id"], init_id)
        client.send("notifications/initialized", notification=True)
        client.send("whoami")
        answer = client.recv()
        self.assertEqual(answer["result"]["served_by"], "daemon")
        self.assertEqual(answer["result"]["allowed"], "runtime.read,todo.list")
        client.send("notify-me")
        self.assertEqual(client.recv()["method"], "notifications/message")
        self.assertEqual(client.recv()["id"], 3)
        self.assertEqual(client.close(), 0)
        hello = daemon.hellos[0]
        self.assertEqual(hello["protocol"], 1)
        self.assertEqual(hello["mode"], "mcp")
        self.assertEqual(hello["argv"], ["serve", "--stdio"])
        self.assertEqual(hello["owner_pid"], os.getpid())
        self.assertEqual(hello["project_root"], str(self.root / "project"))
        self.assertEqual(hello["env"]["FAKE_MARKER"], "from-launcher")
        self.assertEqual(hello["version"], "0.48.16")
        self.assertEqual(hello["cwd"], os.getcwd())
        self.assertEqual(self.runtime_log(), [], "the runtime binary ran although a daemon was present")
        self.assertEqual(client.stderr.getvalue(), "")

    def test_an_absent_daemon_is_started_from_the_staged_copy_then_hosts(self):
        daemon = self.daemon()
        self.point_start_at(daemon)
        client = ProxyClient(self.launcher, self.link())
        client.send("initialize")
        self.assertEqual(client.recv()["result"]["served_by"], "daemon")
        self.assertEqual(client.close(), 0)
        runs = self.runtime_log()
        self.assertEqual(len(runs), 1)
        staged = self.daemon_dir / "bin" / self.launcher.staged_daemon_name(self.binary)
        self.assertEqual(runs[0]["exe"], str(staged), "daemon-start must run from the staged copy, not the plugin binary")
        self.assertEqual(runs[0]["args"], ["daemon-start", "--build-time", "2000000000000"])
        self.assertEqual(runs[0]["env_marker"], "from-launcher")
        self.assertEqual(self.launcher.read_runtime_metadata(staged)["version"], "0.48.16")
        self.assertEqual(sorted(p.name for p in (self.daemon_dir / "bin").iterdir()), sorted([staged.name, staged.name + ".metadata.json"]))

    def test_an_older_daemon_or_an_older_build_is_replaced_a_newer_one_is_used(self):
        for version, build_time, want_start in [("0.48.15", 5000, True), ("0.48.16", 1999999999999, True), ("0.48.16", 2000000000000, False), ("0.48.16-dev", 1, True), ("0.48.16-dev", 2000000000000, False), ("0.48.20", 1, False)]:
            with self.subTest(version=version, build_time=build_time):
                health = {"version": version, "build_time": build_time}
                self.assertEqual(self.link().needs_start(health), want_start)

    def test_a_newer_same_minor_daemon_with_extra_tools_is_accepted(self):
        daemon = self.daemon(version="0.48.20")
        daemon.write_endpoint(self.daemon_dir)
        client = ProxyClient(self.launcher, self.link())
        client.send("tools/list")
        self.assertEqual([t["name"] for t in client.recv()["result"]["tools"]], ["runtime.read", "extra.tool"])
        self.assertEqual(client.close(), 0)
        self.assertEqual(self.runtime_log(), [])

    def test_a_cross_minor_daemon_errors_without_a_second_server(self):
        daemon = self.daemon(version="0.49.0")
        daemon.write_endpoint(self.daemon_dir)
        stderr = io.StringIO()
        code = self.launcher.proxy_serve(self.link(), ["serve", "--stdio"], io.BytesIO(), io.BytesIO(), stderr)
        self.assertEqual(code, 1)
        self.assertIn("dashboard daemon not reachable", stderr.getvalue())
        self.assertIn("outside this plugin's minor", stderr.getvalue())
        self.assertEqual(self.runtime_log(), [])
        self.assertEqual(stderr.getvalue().count("\n"), 1)
        self.assertEqual(daemon.hellos, [])

    def test_a_refused_hello_errors_without_exe_fallback(self):
        daemon = self.daemon(refuse_reason="hello carries no WS_RSRC_ROOT")
        daemon.write_endpoint(self.daemon_dir)
        stderr = io.StringIO()
        self.assertEqual(self.launcher.proxy_serve(self.link(), ["serve", "--stdio"], io.BytesIO(), io.BytesIO(), stderr), 1)
        self.assertIn("hello carries no WS_RSRC_ROOT", stderr.getvalue())
        self.assertEqual(self.runtime_log(), [])

    def test_an_unanswering_daemon_errors_within_the_budget(self):
        silent = socket.socket()
        silent.bind(("127.0.0.1", 0))
        silent.listen(1)
        self.addCleanup(silent.close)
        self.daemon_dir.mkdir(parents=True)
        (self.daemon_dir / "endpoint.json").write_text(json.dumps({"port": silent.getsockname()[1], "pid": 1, "version": "0.48.16", "token": "t" * 64}), encoding="utf-8")
        self.point_start_at(fail=True)
        stderr = io.StringIO()
        started = time.monotonic()
        self.assertEqual(self.launcher.proxy_serve(self.link(answer_budget=0.5), ["serve", "--stdio"], io.BytesIO(), io.BytesIO(), stderr), 1)
        # The answer budget alone: a wedged daemon holds the lock, so a start
        # would only add the whole start budget before the same fallback.
        self.assertLess(time.monotonic() - started, 2.0)
        self.assertIn("dashboard daemon not reachable", stderr.getvalue())
        self.assertIn("did not answer within 0s", stderr.getvalue())
        self.assertEqual(self.runtime_log(), [], "no daemon-start against a daemon that is alive but wedged")

    def test_a_start_that_leaves_the_older_daemon_answering_errors(self):
        # M1: daemon-start reports success, but the daemon answering is still
        # the older build (it refused to stop, or won the lock race). The
        # launcher must not settle on it.
        old = self.daemon(build_time=1_000_000_000_000)
        old.write_endpoint(self.daemon_dir)
        self.point_start_at(old)
        stderr = io.StringIO()
        self.assertEqual(self.launcher.proxy_serve(self.link(), ["serve", "--stdio"], io.BytesIO(), io.BytesIO(), stderr), 1)
        self.assertIn("was not replaced", stderr.getvalue())
        self.assertEqual([r["args"][0] for r in self.runtime_log()], ["daemon-start"])
        self.assertEqual(old.hellos, [], "the older daemon never hosted the session")

    def test_a_same_version_daemon_of_the_same_content_is_kept_whatever_its_build_time(self):
        # L1: Claude, Codex, and Pi install separate copies of one binary,
        # so their mtimes differ; only a different content hash lets the
        # build time replace the daemon.
        digest = self.launcher.sha256_file(self.binary)
        same = self.daemon(build_time=1_000_000_000_000, content_hash=digest)
        same.write_endpoint(self.daemon_dir)
        client = ProxyClient(self.launcher, self.link())
        client.send("initialize")
        self.assertEqual(client.recv()["result"]["served_by"], "daemon")
        self.assertEqual(client.close(), 0)
        self.assertEqual(self.runtime_log(), [], "same content: no replacement")
        link = self.link()
        self.assertFalse(link.needs_start({"version": "0.48.16", "build_time": 1_000_000_000_000, "content_hash": digest}))
        self.assertTrue(link.needs_start({"version": "0.48.16", "build_time": 1_000_000_000_000, "content_hash": "0" * 64}), "another content, older build: replaced")
        self.assertTrue(link.needs_start({"version": "0.48.16", "build_time": 1_000_000_000_000}), "a daemon that reports no hash: the build time decides")
        self.assertFalse(link.needs_start({"version": "0.48.16", "build_time": 2_000_000_000_000, "content_hash": "0" * 64}), "an equal build is never replaced")
        self.assertFalse(link.needs_start({"version": "0.48.17", "build_time": 1, "content_hash": "0" * 64}), "a newer version is never replaced")

    def test_a_dev_daemon_at_the_contract_core_is_replaced_by_a_newer_build(self):
        # The contract's plugin_version carries no suffix while a local-devenv
        # build reports "-dev"; the suffix order alone would read every such
        # daemon as newer and never let a reinstall at that core replace it.
        digest = self.launcher.sha256_file(self.binary)
        link = self.link()
        self.assertTrue(link.needs_start({"version": "0.48.16-dev", "build_time": 1_000_000_000_000, "content_hash": "0" * 64}), "-dev daemon, older build of other content: replaced")
        self.assertFalse(link.needs_start({"version": "0.48.16-dev", "build_time": 1_000_000_000_000, "content_hash": digest}), "-dev daemon of the same content: kept")
        self.assertFalse(link.needs_start({"version": "0.48.16", "build_time": 1_000_000_000_000, "content_hash": digest}), "same version, same content: kept")
        self.assertTrue(link.needs_start({"version": "0.48.15-dev", "build_time": 3_000_000_000_000, "content_hash": "0" * 64}), "an older core: replaced")
        self.assertFalse(link.needs_start({"version": "0.48.17-dev", "build_time": 1, "content_hash": "0" * 64}), "a newer core: kept")

    def test_a_dev_daemon_that_daemon_start_keeps_is_used_not_fallen_back_from(self):
        # A bare release runtime asks to replace an older-built "-dev" daemon
        # of its core, but daemon-start (wsdashboard.keepRunning) ranks the
        # "-dev" daemon newer and keeps it; the launcher then uses it.
        dev = self.daemon(version="0.48.16-dev", build_time=1_000_000_000_000, content_hash="0" * 64)
        dev.write_endpoint(self.daemon_dir)
        self.point_start_at(dev)
        client = ProxyClient(self.launcher, self.link())
        client.send("initialize")
        self.assertEqual(client.recv()["result"]["served_by"], "daemon")
        self.assertEqual(client.close(), 0)
        self.assertEqual([r["args"][0] for r in self.runtime_log()], ["daemon-start"])
        self.assertEqual(len(dev.hellos), 1)

    # --- the paths under traffic --------------------------------------------------

    def test_a_replaced_daemon_is_reconnected_with_initialize_replayed(self):
        old = self.daemon(name="old")
        old.write_endpoint(self.daemon_dir)
        client = ProxyClient(self.launcher, self.link())
        init_id = client.send("initialize", params={"capabilities": {}})
        self.assertEqual(client.recv()["id"], init_id)
        client.send("notifications/initialized", notification=True)
        ping_id = client.send("ping")
        self.assertEqual(client.recv()["id"], ping_id)
        # The successor is up before the old daemon drops the connection.
        new = self.daemon(name="new", build_time=2_000_000_000_001)
        new.write_endpoint(self.daemon_dir)
        old.close()
        self.wait_for_hello(new)
        ping_id = client.send("ping")
        answer = client.recv()
        self.assertEqual(answer["id"], ping_id)
        self.assertEqual(answer["result"]["served_by"], "new")
        self.assertEqual(client.close(), 0)
        replayed = [m for m, _ in new.requests]
        self.assertEqual(replayed[:2], ["initialize", "notifications/initialized"])
        self.assertTrue(str(new.requests[0][1]).startswith("ws-mcp-launcher-replay-"), new.requests[0])
        self.assertEqual(new.hellos[0]["owner_pid"], os.getpid())
        self.assertEqual(self.runtime_log(), [], "a reconnect to a successor must not run the binary")

    def test_a_replaced_daemon_errors_the_requests_in_flight(self):
        old = self.daemon(name="old", slow_methods={"slow"})
        old.write_endpoint(self.daemon_dir)
        client = ProxyClient(self.launcher, self.link())
        client.send("initialize")
        client.recv()
        slow_id = client.send("slow")
        time.sleep(0.2)
        new = self.daemon(name="new", build_time=2_000_000_000_001)
        new.write_endpoint(self.daemon_dir)
        old.close()
        answer = client.recv()
        self.assertEqual(answer["id"], slow_id)
        self.assertEqual(answer["error"]["code"], -32000)
        self.assertEqual(answer["error"]["message"], "dashboard daemon not reachable; the request did not complete")
        # Lost calls fail immediately, before recovery. Wait for the successor
        # before expecting the ping to succeed; unsent calls wait during the gap.
        self.wait_for_hello(new)
        # The replayed initialize answer is dropped: the next line is the ping's.
        ping_id = client.send("ping")
        answer = client.recv()
        self.assertEqual(answer["id"], ping_id)
        self.assertEqual(answer["result"]["served_by"], "new")
        self.assertEqual(client.close(), 0)
        self.assertEqual([m for m, _ in old.requests].count("slow"), 1)
        self.assertNotIn("slow", [m for m, _ in new.requests])

    def test_a_daemon_that_dies_with_no_successor_is_restarted(self):
        old = self.daemon(name="old")
        old.write_endpoint(self.daemon_dir)
        client = ProxyClient(self.launcher, self.link())
        client.send("initialize")
        client.recv()
        new = self.daemon(name="new", build_time=2_000_000_000_001)
        self.point_start_at(new)  # daemon-start will point the endpoint at it
        old.close()
        self.wait_for_hello(new)
        client.send("ping")
        self.assertEqual(client.recv()["result"]["served_by"], "new")
        self.assertEqual(client.close(), 0)
        self.assertEqual([r["args"][0] for r in self.runtime_log()], ["daemon-start"])
        self.assertEqual([m for m, _ in new.requests][:1], ["initialize"])

    def test_three_quick_closes_never_spawn_exe_and_can_recover(self):
        daemon = self.daemon(drop_mcp=True)
        daemon.write_endpoint(self.daemon_dir)
        client = ProxyClient(self.launcher, self.link())
        init_id = client.send("initialize")
        lost = client.recv(timeout=10.0)
        self.assertEqual(lost["id"], init_id)
        self.assertEqual(lost["error"]["code"], -32000)
        deadline = time.monotonic() + 12
        while len(daemon.hellos) < 3 and time.monotonic() < deadline:
            ping_id = client.send("side-effect")
            answer = client.recv(timeout=3.0)
            self.assertEqual(answer["id"], ping_id)
            self.assertIn("dashboard daemon not reachable", answer["error"]["message"])
            time.sleep(0.1)
        self.assertGreaterEqual(len(daemon.hellos), 3)
        self.assertEqual(self.runtime_log(), [])
        healthy = self.daemon(name="recovered")
        healthy.write_endpoint(self.daemon_dir)
        self.wait_for_hello(healthy)
        ping_id = client.send("ping")
        self.assertEqual(client.recv()["id"], ping_id)
        self.assertEqual(client.close(), 0)
        self.assertNotIn("side-effect", [method for method, _ in healthy.requests])
        self.assertEqual(self.runtime_log(), [])

    def test_reconnect_failure_fails_requests_without_replaying_side_effects(self):
        old = self.daemon(name="old", slow_methods={"slow"})
        old.write_endpoint(self.daemon_dir)
        self.point_start_at(fail=True)
        client = ProxyClient(self.launcher, self.link(start_budget=1.0))
        client.send("initialize")
        client.recv()
        client.send("notifications/initialized", notification=True)
        slow_id = client.send("slow")
        time.sleep(0.2)
        old.close()
        lost = client.recv(timeout=10.0)
        self.assertEqual(lost["id"], slow_id)
        self.assertIn("error", lost)
        client.send("ping")
        answer = client.recv(timeout=10.0)
        self.assertEqual(answer["id"], client.next_id)
        self.assertIn("dashboard daemon not reachable", answer["error"]["message"])
        self.assertEqual(client.close(), 1)
        self.assertTrue(all(r["args"][0] == "daemon-start" for r in self.runtime_log() if "args" in r))
        self.assertEqual([r for r in self.runtime_log() if "recv" in r], [])

    def test_unsent_tools_list_waits_for_endpoint_after_three_attempts(self):
        self.launcher.DAEMON_RETRY_SECONDS = 3.0
        old = self.daemon(name="old")
        old.write_endpoint(self.daemon_dir)
        client = ProxyClient(self.launcher, self.link(allow_start=False))
        client.send("initialize")
        client.recv()
        client.send("notifications/initialized", notification=True)
        (self.daemon_dir / "endpoint.json").unlink()
        successor = self.daemon(name="successor")
        attempts = []
        original = self.launcher.read_daemon_endpoint
        def discover(directory):
            value = original(directory)
            if value is None:
                attempts.append(True)
                if len(attempts) == 3:
                    successor.write_endpoint(directory)
            return value
        with mock.patch.object(self.launcher, "read_daemon_endpoint", side_effect=discover):
            old.close()
            time.sleep(0.1)
            list_id = client.send("tools/list")
            call_id = client.send("tools/call")
            answer = client.recv()
            call_answer = client.recv()
        self.assertEqual(call_answer["id"], call_id)
        self.assertEqual(call_answer["result"]["served_by"], "successor")
        self.assertEqual(answer["id"], list_id)
        self.assertIn("tools", answer["result"])
        self.assertGreaterEqual(len(attempts), 3)
        self.assertEqual([r for r in successor.requests if r[0] == "tools/call"], [("tools/call", call_id)])
        self.assertEqual(client.close(), 0)

    def test_initial_initialize_waits_without_replaying_the_unsent_original(self):
        self.launcher.DAEMON_RETRY_SECONDS = 3.0
        daemon = self.daemon()
        attempts = []
        original = self.launcher.read_daemon_endpoint
        def discover(directory):
            attempts.append(True)
            if len(attempts) == 3:
                daemon.write_endpoint(directory)
            return original(directory)
        with mock.patch.object(self.launcher, "read_daemon_endpoint", side_effect=discover):
            client = ProxyClient(self.launcher, self.link(allow_start=False))
            init_id = client.send("initialize")
            answer = client.recv()
        self.assertEqual(answer["id"], init_id)
        self.assertIn("serverInfo", answer["result"])
        client.send("tools/list")
        self.assertIn("tools", client.recv()["result"])
        self.assertEqual(client.close(), 0)
        self.assertEqual([r for r in daemon.requests if r[0] == "initialize"], [("initialize", init_id)])

    def test_tools_list_exhaustion_keeps_stdio_alive_for_later_reconnect(self):
        old = self.daemon()
        old.write_endpoint(self.daemon_dir)
        client = ProxyClient(self.launcher, self.link(allow_start=False))
        client.send("initialize")
        client.recv()
        (self.daemon_dir / "endpoint.json").unlink()
        old.close()
        time.sleep(0.1)
        started = time.monotonic()
        request_id = client.send("tools/list")
        call_id = client.send("tools/call")
        answers = {reply["id"]: reply for reply in (client.recv(), client.recv())}
        self.assertLess(time.monotonic() - started, 0.55, "concurrent calls must not wait two budgets")
        self.assertIn("waited 0.3s", answers[call_id]["error"]["message"])
        answer = answers[request_id]
        self.assertEqual(answer["id"], request_id)
        self.assertEqual(answer["error"]["message"],
                         "dashboard daemon not reachable; request was not sent (waited 0.3s for the dashboard daemon)")
        self.assertTrue(client.thread.is_alive())
        successor = self.daemon(name="successor")
        successor.write_endpoint(self.daemon_dir)
        self.wait_for_hello(successor)
        client.send("tools/list")
        self.assertIn("tools", client.recv()["result"])
        self.assertEqual(client.close(), 0)

    def test_cli_retries_missing_and_stale_endpoints_with_one_deadline(self):
        self.launcher.DAEMON_RETRY_SECONDS = 10.0
        self.launcher.DAEMON_RETRY_POLL_SECONDS = 1.0
        for stale in (False, True):
            with self.subTest(stale=stale):
                now = [0.0]
                reads = []
                daemon = self.daemon(cli_exit_code=0)
                link = self.link(allow_start=False)
                def discover(directory):
                    reads.append(directory)
                    if len(reads) >= 3:
                        return daemon.endpoint()
                    return {"port": 1, "token": "stale"} if stale else None
                stdout = io.BytesIO()
                with mock.patch.object(self.launcher.time, "monotonic", side_effect=lambda: now[0]), \
                     mock.patch.object(self.launcher.time, "sleep", side_effect=lambda seconds: now.__setitem__(0, now[0] + seconds)), \
                     mock.patch.object(self.launcher, "read_daemon_endpoint", side_effect=discover):
                    code = self.launcher.proxy_cli(link, ["tools"], stdout, io.BytesIO())
                self.assertEqual(code, 0)
                self.assertEqual(json.loads(stdout.getvalue())["served_by"], "daemon")
                self.assertEqual(len(reads), 3)
                self.assertEqual(len(daemon.hellos), 1)

    def test_cli_never_appears_exhausts_budget(self):
        now = [0.0]
        stderr = io.BytesIO()
        with mock.patch.object(self.launcher.time, "monotonic", side_effect=lambda: now[0]), \
             mock.patch.object(self.launcher.time, "sleep", side_effect=lambda seconds: now.__setitem__(0, now[0] + seconds)):
            code = self.launcher.proxy_cli(self.link(allow_start=False), ["tools"], io.BytesIO(), stderr)
        self.assertEqual(code, 1)
        self.assertEqual(now[0], 0.3)
        self.assertIn(b"(waited 0.3s for the dashboard daemon)", stderr.getvalue())

    def test_cli_failure_after_hello_is_not_retried(self):
        for refuse in (False, True):
            with self.subTest(refuse=refuse):
                daemon = self.daemon(refuse_reason="closing after hello" if refuse else "")
                daemon.write_endpoint(self.daemon_dir)
                # Close after acceptance without a final frame.
                daemon.serve_cli = lambda conn, hello: None
                stderr = io.BytesIO()
                self.assertEqual(self.launcher.proxy_cli(self.link(), ["tools"], io.BytesIO(), stderr), 1)
                self.assertEqual(len(daemon.hellos), 1)
                self.assertNotIn(b"waited", stderr.getvalue())

    def test_stdin_close_ends_the_session(self):
        daemon = self.daemon()
        daemon.write_endpoint(self.daemon_dir)
        client = ProxyClient(self.launcher, self.link())
        client.send("initialize")
        client.recv()
        self.assertEqual(client.close(), 0)
        deadline = time.monotonic() + 3
        while time.monotonic() < deadline and any(not c._closed for c in daemon.conns):
            time.sleep(0.05)
        self.assertTrue(all(c._closed for c in daemon.conns), "the daemon saw the session end")
        self.assertEqual(len(daemon.conns), 2, "one /health probe and one session")

    def test_stdin_close_still_delivers_the_answers_in_flight(self):
        # M6: the client closes its side while a call runs; the proxy
        # half-closes, so the answer still comes through before the end.
        daemon = self.daemon(slow_methods={"slow"}, slow_seconds=0.5)
        daemon.write_endpoint(self.daemon_dir)
        client = ProxyClient(self.launcher, self.link())
        client.send("initialize")
        client.recv()
        slow_id = client.send("slow")
        client.close_stdin()
        answer = client.recv(timeout=5.0)
        self.assertEqual(answer["id"], slow_id)
        self.assertEqual(answer["result"]["served_by"], "daemon")
        client.thread.join(5.0)
        self.assertFalse(client.thread.is_alive())
        self.assertEqual(client.result.get("code"), 0)

    # --- mailbox wait -------------------------------------------------------------

    def test_mailbox_wait_runs_in_the_daemon_with_its_frames_unpacked(self):
        daemon = self.daemon()
        daemon.write_endpoint(self.daemon_dir)
        stdout, stderr = io.BytesIO(), io.BytesIO()
        code = self.launcher.proxy_cli(self.link(), ["mailbox", "wait", "--session-key", "k", "--format", "json"], stdout, stderr)
        self.assertEqual(code, 7)
        self.assertEqual(json.loads(stdout.getvalue()), {"served_by": "daemon", "args": ["--session-key", "k", "--format", "json"], "owner_pid": os.getpid()})
        # Bytes, UTF-8, the daemon's newlines as sent: no console code page
        # and no newline translation on any platform.
        self.assertEqual(stderr.getvalue(), b"fake: waiting\n")
        self.assertEqual(daemon.hellos[0]["mode"], "cli")
        self.assertEqual(daemon.hellos[0]["argv"], ["mailbox", "wait", "--session-key", "k", "--format", "json"])
        self.assertEqual(self.runtime_log(), [])

    def test_mailbox_stream_flushes_each_arrival_before_the_next_frame(self):
        stdout, stderr = io.BytesIO(), io.BytesIO()
        arrivals = ['{"kinds":["mail"],"unread":1}\n', '{"kinds":["board"],"board":1}\n']
        stream = mock.Mock()

        def frames():
            yield json.dumps({"stream": "stdout", "data": arrivals[0]}).encode() + b"\n"
            self.assertEqual(stdout.getvalue(), arrivals[0].encode())
            yield json.dumps({"stream": "stderr", "data": "diagnostic\n"}).encode() + b"\n"
            yield json.dumps({"stream": "stdout", "data": arrivals[1]}).encode() + b"\n"
            self.assertEqual(stdout.getvalue(), "".join(arrivals).encode())
            yield json.dumps({"final": True, "exit_code": 130, "data": ""}).encode() + b"\n"

        stream.read_line.side_effect = frames()
        link = mock.Mock(env=self.env, runtime_version="0.48.16")
        link.connect.return_value = stream
        argv = ["mailbox", "wait", "--session-key", "k", "--stream", "--timeout", "10m"]
        with mock.patch.object(self.launcher, "watch_parent_exit") as watch:
            code = self.launcher.proxy_cli(link, argv, stdout, stderr)
        self.assertEqual(code, 130)
        self.assertEqual(stderr.getvalue(), b"diagnostic\n")
        self.assertEqual(link.connect.call_count, 1, "a stream must not relaunch per arrival")
        self.assertEqual(link.connect.call_args.args[0]["argv"], argv)
        watch.assert_called_once_with(stream.close, os.getppid())
        stream.close.assert_called_once()

    def test_parent_watch_closes_an_already_exited_windows_parent(self):
        api = mock.Mock(WAIT_OBJECT_0=0, INFINITE=0xFFFFFFFF, SYNCHRONIZE=0x00100000)
        api.WaitForSingleObject.return_value = 0
        ended = threading.Event()
        with mock.patch.object(self.launcher.os, "name", "nt"), \
             mock.patch.dict(sys.modules, {"_winapi": api}):
            self.launcher.watch_parent_exit(ended.set)
            self.assertTrue(ended.wait(2), "already-dead parent left the wait attached")
        api.WaitForSingleObject.assert_called_once_with(api.OpenProcess.return_value, api.INFINITE)

    def test_parent_watch_closes_when_parent_disappeared_before_open(self):
        api = mock.Mock()
        gone = OSError("parent gone")
        gone.winerror = 87
        api.OpenProcess.side_effect = gone
        ended = threading.Event()
        with mock.patch.object(self.launcher.os, "name", "nt"), \
             mock.patch.dict(sys.modules, {"_winapi": api}):
            self.launcher.watch_parent_exit(ended.set)
            self.assertTrue(ended.wait(2))

    def test_mailbox_wait_success_reaches_the_caller_as_exit_zero(self):
        # A wait that a mail wakes exits 0; the mod and the Pi waiter treat
        # any code but 0 and 3 as an error and never push the mail. A final
        # frame with the code written out, and one from a daemon that
        # omitted the zero, both read as success.
        for daemon in (self.daemon(cli_exit_code=0), self.daemon(cli_exit_code=None)):
            daemon.write_endpoint(self.daemon_dir)
            stdout = io.BytesIO()
            code = self.launcher.proxy_cli(self.link(), ["mailbox", "wait", "--session-key", "k", "--format", "json"], stdout, io.BytesIO())
            self.assertEqual(code, 0, f"exit code with cli_exit_code={daemon.cli_exit_code}")
            self.assertEqual(json.loads(stdout.getvalue())["served_by"], "daemon")

    def test_mailbox_wait_errors_without_direct_execution(self):
        self.point_start_at(fail=True)
        stderr = io.BytesIO()
        argv = ["mailbox", "wait", "--session-key", "k"]
        self.assertEqual(self.launcher.proxy_cli(self.link(), argv, io.BytesIO(), stderr), 1)
        self.assertIn(b"dashboard daemon not reachable", stderr.getvalue())
        self.assertEqual([r["args"][0] for r in self.runtime_log()], ["daemon-start"])

    def test_proxy_mode_has_no_catch_all_executable_escape(self):
        self.assertEqual(self.launcher.proxy_mode(["serve", "--stdio"]), "mcp")
        self.assertEqual(self.launcher.proxy_mode(["serve", "--stdio", "--root", "project"]), "mcp")
        for argv in (["mailbox", "wait"], ["mailbox", "codex-stop-hook"], ["version"], ["tools"], ["call", "runtime.read", "{}"], ["unknown"], []):
            self.assertEqual(self.launcher.proxy_mode(argv), "cli")
        self.assertEqual(self.launcher.proxy_mode(["daemon-start"]), "lifecycle")
        self.assertFalse(hasattr(self.launcher, "ProcessStream"))
        self.assertFalse(hasattr(self.launcher, "proxy_disabled"))

    def run_main(self, argv, *, input_text="", input_encoding="utf-8", forbid_install=False, allow_build=False):
        stdout = io.TextIOWrapper(io.BytesIO(), encoding="utf-8")
        stderr = io.TextIOWrapper(io.BytesIO(), encoding="utf-8")
        env = dict(self.env, WS_MCP_RUNTIME_DIR=str(self.root), WS_MCP_LAUNCHER_DIRECT="1")
        with mock.patch.dict(os.environ, env, clear=True), \
             mock.patch.object(sys, "argv", [str(LAUNCHER_PATH), *argv]), \
             mock.patch.object(sys, "stdin", io.TextIOWrapper(io.BytesIO(input_text.encode("utf-8")), encoding=input_encoding)), \
             mock.patch.object(sys, "stdout", stdout), mock.patch.object(sys, "stderr", stderr), \
             mock.patch.object(self.launcher, "detect_project_root"), \
             mock.patch.object(self.launcher, "read_runtime_contract", return_value={"plugin_version": "0.48.16"}), \
             mock.patch.object(self.launcher, "runtime_binary_name", return_value=self.binary.name), \
             mock.patch.multiple(self.launcher.subprocess, run=mock.DEFAULT, Popen=mock.DEFAULT, call=mock.DEFAULT) as processes, \
             mock.patch.multiple(self.launcher.os, execv=mock.DEFAULT, execve=mock.DEFAULT, execvpe=mock.DEFAULT) as execs, \
             mock.patch.object(self.launcher, "install_runtime", side_effect=AssertionError("unexpected repair")) if forbid_install else mock.patch.object(self.launcher, "note"):
            if allow_build:
                processes["run"].side_effect = self.launcher.subprocess.TimeoutExpired(["fake-go", "build"], 0.05)
            code = self.launcher.main()
            for name, process in processes.items():
                if allow_build and name == "run":
                    self.assertTrue(process.called)
                    for call in process.call_args_list:
                        self.assertEqual(call.args[0][0], "fake-go")
                        self.assertEqual(call.kwargs["timeout"], self.launcher.RUNTIME_BUILD_TIMEOUT)
                else:
                    process.assert_not_called()
            for process in execs.values():
                process.assert_not_called()
        stdout.flush()
        stderr.flush()
        return code, stdout.buffer.getvalue(), stderr.buffer.getvalue()

    def test_main_never_restores_direct_bypass_or_catch_all_exec(self):
        daemon = self.daemon(cli_exit_code=0)
        daemon.write_endpoint(self.daemon_dir)
        for argv in (["tools"], ["call", "runtime.read", "{}"], ["git", "status"], ["unknown"]):
            with self.subTest(argv=argv):
                code, stdout, _ = self.run_main(argv, forbid_install=True)
                self.assertEqual(code, 0)
                self.assertEqual(json.loads(stdout)["served_by"], "daemon")
        hello = daemon.hellos[-1]
        self.assertEqual(hello["env"]["WS_MCP_HARNESS_PID"], str(os.getppid()))
        self.assertIn("ws-mcp-launcher.py", hello["env"]["WS_MCP_CLI_NAME"])
        self.assertNotEqual(hello["env"]["WS_MCP_CLI_NAME"], str(self.binary))

    def test_compatible_older_daemon_needs_no_local_binary_or_metadata(self):
        daemon = self.daemon(cli_exit_code=0)
        daemon.write_endpoint(self.daemon_dir)
        self.binary.unlink()
        self.launcher.runtime_metadata_path(self.binary).unlink()
        code, stdout, _ = self.run_main(["tools"], forbid_install=True)
        self.assertEqual(code, 0)
        self.assertEqual(json.loads(stdout)["served_by"], "daemon")

    def test_version_inspection_never_starts_or_probes_a_ws_process(self):
        code, stdout, _ = self.run_main(["version"], forbid_install=True)
        self.assertEqual((code, stdout.strip()), (0, b"0.48.16"))
        self.assertFalse((self.daemon_dir / "bin").exists())
        daemon = self.daemon(version="0.48.20")
        daemon.write_endpoint(self.daemon_dir)
        self.binary.unlink()
        code, stdout, _ = self.run_main(["version"], forbid_install=True)
        self.assertEqual((code, stdout.strip()), (0, b"0.48.20"))

    def test_new_launcher_uses_older_daemon_hello_without_version_endpoint(self):
        for options in ({"health_status": 404}, {"health_version": False}):
            with self.subTest(options=options):
                daemon = self.daemon(**options)
                daemon.write_endpoint(self.daemon_dir)
                client = ProxyClient(self.launcher, self.link())
                client.send("tools/list")
                self.assertIn("tools", client.recv()["result"])
                self.assertEqual(client.close(), 0)
                self.assertGreaterEqual(len(daemon.hellos), 2)
        self.assertEqual(self.runtime_log(), [])

    def test_codex_stop_hook_uses_daemon_and_optional_stdin(self):
        manifest = json.loads((LAUNCHER_PATH.parents[1] / ".codex-plugin" / "hooks.json").read_text(encoding="utf-8"))
        self.assertIn("ws-mcp-launcher.py", json.dumps(manifest))
        self.assertIn("mailbox codex-stop-hook", json.dumps(manifest))
        daemon = self.daemon(cli_exit_code=0)
        daemon.write_endpoint(self.daemon_dir)
        payload = '{"cwd":"my-worktree","stop_hook_active":true}'
        code, _, _ = self.run_main(["mailbox", "codex-stop-hook"], input_text=payload, forbid_install=True)
        self.assertEqual(code, 0)
        self.assertEqual(daemon.hellos[-1]["stdin"], payload)
        self.assertEqual(daemon.hellos[-1]["argv"], ["mailbox", "codex-stop-hook"])
        daemon.close()
        self.point_start_at(fail=True)
        stderr = io.BytesIO()
        self.assertEqual(self.launcher.proxy_cli(self.link(), ["mailbox", "codex-stop-hook"], io.BytesIO(), stderr, payload), 0)
        self.assertIn(b"dashboard daemon not reachable", stderr.getvalue())
        self.assertTrue(all(row["args"][0] == "daemon-start" for row in self.runtime_log()))

    def test_daemon_down_answers_each_request_with_original_id_without_exe_child(self):
        stderr, stdout = io.StringIO(), io.BytesIO()
        requests = b'{"jsonrpc":"2.0","id":"first","method":"initialize"}\n{"jsonrpc":"2.0","id":7,"method":"tools/call"}\n'
        failed = self.launcher.subprocess.CompletedProcess([], 1, "", "starter refused")
        with mock.patch.object(self.launcher.subprocess, "run", return_value=failed) as starter, \
             mock.patch.object(self.launcher.subprocess, "Popen") as child:
            self.assertEqual(self.launcher.proxy_serve(self.link(), ["serve", "--stdio"], io.BytesIO(requests), stdout, stderr), 1)
            child.assert_not_called()
            self.assertTrue(all(call.args[0][1] == "daemon-start" for call in starter.call_args_list))
        replies = [json.loads(line) for line in stdout.getvalue().splitlines()]
        self.assertCountEqual([reply["id"] for reply in replies], ["first", 7])
        self.assertTrue(all("dashboard daemon not reachable" in reply["error"]["message"] for reply in replies))
        self.assertIn("starter refused", stderr.getvalue())
        self.assertEqual(self.runtime_log(), [])

    def test_all_valid_stdio_flag_shapes_preserve_mcp_ids_and_root(self):
        daemon = self.daemon()
        daemon.write_endpoint(self.daemon_dir)
        root = str(self.root / "explicit-root")
        payload = '{"jsonrpc":"2.0","id":"init","method":"initialize"}\n{"jsonrpc":"2.0","id":22,"method":"tools/list"}\n'
        for argv in (["serve", "--root", root, "--stdio"], ["serve", "-stdio", "-root", root],
                     ["serve", "--stdio=true", "--root=" + root], ["serve", "-root=" + root, "-stdio=1"]):
            with self.subTest(argv=argv):
                code, output, _ = self.run_main(argv, input_text=payload, forbid_install=True)
                self.assertEqual(code, 0)
                replies = [json.loads(line) for line in output.splitlines()]
                self.assertEqual([reply["id"] for reply in replies], ["init", 22])
                self.assertIn("tools", replies[1]["result"])
                self.assertEqual(daemon.hellos[-1]["project_root"], root)
        for argv in (["serve", "--stdio=false"], ["serve", "--stdio=invalid"], ["serve", "--unknown", "--stdio"]):
            self.assertEqual(self.launcher.proxy_mode(argv), "cli")

    def test_cli_stdin_is_utf8_even_under_a_non_utf8_windows_wrapper(self):
        daemon = self.daemon(cli_exit_code=0)
        daemon.write_endpoint(self.daemon_dir)
        for argv, payload in ((["git", "commit", "--description-file", "-"], "한글 commit body — café"),
                              (["mailbox", "codex-stop-hook"], '{"cwd":"D:/작업/저장소","stop_hook_active":true}')):
            with self.subTest(argv=argv):
                code, _, _ = self.run_main(argv, input_text=payload, input_encoding="cp1252", forbid_install=True)
                self.assertEqual(code, 0)
                self.assertEqual(daemon.hellos[-1]["stdin"], payload)

    def test_cold_stalled_download_reaches_framed_mcp_errors(self):
        silent = socket.socket()
        silent.bind(("127.0.0.1", 0))
        silent.listen(8)
        self.addCleanup(silent.close)
        self.env["WS_MCP_BOOTSTRAP_URL"] = f"http://127.0.0.1:{silent.getsockname()[1]}/runtime"
        self.binary.unlink()
        self.launcher.runtime_metadata_path(self.binary).unlink()
        payload = '{"jsonrpc":"2.0","id":"cold","method":"initialize"}\n{"jsonrpc":"2.0","id":3,"method":"tools/call"}\n'
        started = time.monotonic()
        with mock.patch.object(self.launcher, "RUNTIME_DOWNLOAD_TIMEOUT", 0.05):
            code, output, stderr = self.run_main(["serve", "--stdio"], input_text=payload)
        self.assertLess(time.monotonic() - started, 2)
        self.assertEqual(code, 1)
        replies = [json.loads(line) for line in output.splitlines()]
        self.assertCountEqual([reply["id"] for reply in replies], ["cold", 3])
        self.assertTrue(all("dashboard daemon not reachable" in reply["error"]["message"] for reply in replies))
        self.assertIn(b"timed out", stderr)

    def test_cold_build_timeout_reaches_framed_mcp_errors(self):
        self.binary.unlink()
        self.launcher.runtime_metadata_path(self.binary).unlink()
        def build(*args, **kwargs):
            self.launcher.build_local_devenv_runtime(self.root, self.binary, {"plugin_version": "0.48.16"},
                {"tool_dir": self.root, "go": Path("fake-go")}, "linux")
        payload = '{"jsonrpc":"2.0","id":"build","method":"initialize"}\n'
        with mock.patch.object(self.launcher, "install_runtime", side_effect=build), \
             mock.patch.object(self.launcher, "source_runtime_metadata", return_value={"version": "0.48.16"}):
            code, output, stderr = self.run_main(["serve", "--stdio"], input_text=payload, allow_build=True)
        self.assertEqual(code, 1)
        self.assertEqual(json.loads(output)["id"], "build")
        self.assertIn("dashboard daemon not reachable", json.loads(output)["error"]["message"])
        self.assertIn(b"timed out", stderr)

    def test_recovery_repairs_missing_runtime_only_when_start_becomes_needed(self):
        source = self.root / "repair-source"
        self.launcher.copy_runtime(self.binary, source)
        self.binary.unlink()
        self.launcher.runtime_metadata_path(self.binary).unlink()
        for transient_initial_failure in (False, True):
            with self.subTest(transient_initial_failure=transient_initial_failure):
                attempts = []
                def repair():
                    attempts.append(True)
                    if transient_initial_failure and len(attempts) == 1:
                        return False
                    self.launcher.copy_runtime(source, self.binary)
                    return True
                old = self.daemon(name="old", slow_methods={"side-effect"})
                successor = self.daemon(name="repaired")
                self.point_start_at(successor)
                if not transient_initial_failure:
                    old.write_endpoint(self.daemon_dir)
                else:
                    (self.daemon_dir / "endpoint.json").unlink(missing_ok=True)
                client = ProxyClient(self.launcher, self.link(repair_runtime=repair))
                client.send("initialize")
                client.recv()
                if not transient_initial_failure:
                    self.assertEqual(attempts, [], "healthy old daemon needs no local upgrade")
                    lost_id = client.send("side-effect")
                    time.sleep(0.1)
                    old.close()
                    self.assertEqual(client.recv()["id"], lost_id)
                self.wait_for_hello(successor, timeout=8)
                ping_id = client.send("ping")
                reply = client.recv()
                self.assertEqual(reply["id"], ping_id)
                self.assertEqual(reply["result"]["served_by"], "repaired")
                self.assertEqual(client.close(), 0)
                self.assertTrue(attempts)
                self.assertNotIn("side-effect", [method for method, _ in successor.requests])
                self.assertTrue(all(row["args"][0] == "daemon-start" for row in self.runtime_log()))
                self.binary.unlink()
                self.launcher.runtime_metadata_path(self.binary).unlink()
                successor.close()

    # --- parity with Go -----------------------------------------------------------

    def test_versions_compare_as_the_daemon_compares_them(self):
        cmp = self.launcher.compare_daemon_versions
        self.assertEqual(cmp("0.48.16", "0.48.16"), 0)
        self.assertEqual(cmp("0.48.16", "0.48.17"), -1)
        self.assertEqual(cmp("0.48.16-dev", "0.48.16"), 1)
        self.assertEqual(cmp("0.48.16", "0.48.16-dev"), -1)
        self.assertEqual(cmp("v0.49.0", "0.48.99"), 1)
        self.assertEqual(cmp("junk", "0.48.16"), -1)
        self.assertEqual(cmp("junk", "more junk"), 0)
        self.assertTrue(self.launcher.same_minor("0.48.16", "0.48.99-dev"))
        self.assertFalse(self.launcher.same_minor("0.48.16", "0.49.0"))
        self.assertFalse(self.launcher.same_minor("", "0.49.0"))

    def test_cache_root_and_staged_name_follow_the_go_rules(self):
        link_root = self.root / "link"
        os.symlink(self.cache, link_root)
        self.assertEqual(self.launcher.proxy_cache_root({"WS_CACHE_HOME": str(link_root)}), Path(os.path.realpath(self.cache)))
        self.assertEqual(self.launcher.proxy_cache_root({"WS_CACHE_HOME": str(self.root / "missing")}), self.root / "missing")
        self.assertEqual(self.launcher.proxy_cache_root({"HOME": "/nowhere"}).name, "ws@kang-sw-devenv")
        self.assertEqual(self.launcher.proxy_daemon_dir(self.env), Path(os.path.realpath(self.cache)) / "dashboard")
        digest = self.launcher.sha256_file(self.binary)
        self.assertEqual(self.launcher.staged_daemon_name(self.binary), f"ws-mcp-dashboard-{digest[:16]}.16-abcdef123456")
        self.assertEqual(self.launcher.staged_daemon_name(Path("ws-mcp-0.48.16-abc.EXE"), "a" * 64), "ws-mcp-dashboard-aaaaaaaaaaaaaaaa.exe")
        # filepath.Ext, not Path.suffix: a trailing dot is ".", a dot-file is
        # its whole name, and the directory never takes part.
        go_ext = self.launcher.go_ext
        self.assertEqual(go_ext("ws-mcp."), ".")
        self.assertEqual(go_ext(".bashrc"), ".bashrc")
        self.assertEqual(go_ext("noext"), "")
        self.assertEqual(go_ext(os.path.join("dir.d", "noext")), "")
        self.assertEqual(self.launcher.staged_daemon_name(Path("ws-mcp."), "b" * 64), "ws-mcp-dashboard-bbbbbbbbbbbbbbbb.")

    def test_the_default_start_wait_matches_the_go_constant(self):
        tool = Path(__file__).resolve().parents[2] / "agents-plugin-tool" / "internal" / "wsdashboard"
        daemon_src = (tool / "daemon.go").read_text(encoding="utf-8")
        ensure_src = (tool / "ensure.go").read_text(encoding="utf-8")
        import re

        lock = re.search(r"DefaultLockWait = (\d+) \* time\.Second", daemon_src)
        extra = re.search(r"DefaultStartWait = DefaultLockWait \+ (\d+)\*time\.Second", ensure_src)
        self.assertIsNotNone(lock, "DefaultLockWait not found in daemon.go")
        self.assertIsNotNone(extra, "DefaultStartWait not found in ensure.go")
        self.assertEqual(self.launcher.DAEMON_DEFAULT_START_WAIT, float(int(lock.group(1)) + int(extra.group(1))))

    def daemon_start_entries(self):
        log = self.cache / "fake-runtime.log"
        entries = [json.loads(line) for line in log.read_text(encoding="utf-8").splitlines()]
        return [e for e in entries if e.get("args", [])[:1] == ["daemon-start"]]

    def test_daemon_start_gets_the_launcher_interpreter_for_session_holders(self):
        daemon = self.daemon()
        self.point_start_at(daemon)
        before = dict(self.env)
        self.assertTrue(self.link().start_daemon())
        [entry] = self.daemon_start_entries()
        self.assertEqual(entry["holder_python"], sys.executable)
        self.assertEqual(entry["env_marker"], "from-launcher")
        self.assertEqual(self.env, before, "only the daemon-start environment carries the interpreter")

    def test_daemon_start_keeps_an_interpreter_already_chosen(self):
        daemon = self.daemon()
        self.point_start_at(daemon)
        self.env["WS_MCP_HOLDER_PYTHON"] = "/opt/chosen/python3"
        self.assertTrue(self.link().start_daemon())
        [entry] = self.daemon_start_entries()
        self.assertEqual(entry["holder_python"], "/opt/chosen/python3")

    def test_staging_writes_the_copy_once_and_leaves_other_entries_alone(self):
        # The other entry may be the running daemon's own executable, which
        # it respawns itself from on a LAN change; only the daemon that holds
        # the lock prunes the directory.
        bin_dir = self.daemon_dir / "bin"
        bin_dir.mkdir(parents=True)
        (bin_dir / "ws-mcp-dashboard-running").write_text("old", encoding="utf-8")
        staged = self.launcher.stage_daemon_binary(self.binary, bin_dir)
        self.assertEqual(staged.read_bytes(), self.binary.read_bytes())
        self.assertTrue(os.access(staged, os.X_OK))
        self.assertEqual(sorted(p.name for p in bin_dir.iterdir()), sorted([staged.name, staged.name + ".metadata.json", "ws-mcp-dashboard-running"]))
        self.assertEqual(self.launcher.read_runtime_metadata(staged)["binary_sha256"], self.launcher.sha256_file(self.binary))
        first = staged.stat().st_mtime_ns
        self.assertEqual(self.launcher.stage_daemon_binary(self.binary, bin_dir), staged)
        self.assertEqual(staged.stat().st_mtime_ns, first, "an up-to-date staged copy is left alone")


class RecordingStream:
    """A stream stub that records writes and whether it was closed."""

    def __init__(self) -> None:
        self.writes: list[bytes] = []
        self.closed = False

    def write(self, data: bytes) -> None:
        self.writes.append(data)

    def close(self) -> None:
        self.closed = True


class LauncherProxyReviewTest(unittest.TestCase):
    """Fixes from the lite review: relative paths in the hello, a client that
    leaves during a reconnect, and the compatibility probe's executable."""

    def test_hello_env_makes_path_values_absolute_against_the_cwd(self) -> None:
        cwd = os.path.abspath(os.path.join(os.sep, "work", "plugin"))
        env = {
            "WS_MCP_CONFIG_MANIFESTS": "./config-manifest.json;" + os.pathsep.join(["dash/d.json", ""]) + ";",
            "WS_CACHE_HOME": "cache",
            "WS_CONFIG_HOME": "",
            "WS_RSRC_ROOT": os.path.join(cwd, "rsrc"),
            "HOME": "relative-home-is-not-ours",
        }
        out = self.launcher.hello_env(env, cwd)
        self.assertEqual(out["WS_MCP_CONFIG_MANIFESTS"], ";".join([os.path.join(cwd, "config-manifest.json"), os.path.join(cwd, "dash", "d.json")]))
        self.assertEqual(out["WS_CACHE_HOME"], os.path.join(cwd, "cache"))
        self.assertEqual(out["WS_CONFIG_HOME"], "", "an empty value stays empty")
        self.assertEqual(out["WS_RSRC_ROOT"], os.path.join(cwd, "rsrc"), "an absolute value is unchanged")
        self.assertEqual(out["HOME"], "relative-home-is-not-ours", "only ws path keys are touched")
        self.assertEqual(env["WS_CACHE_HOME"], "cache", "the launcher's own environment is not modified")
        hello = self.launcher.proxy_hello("mcp", ["serve", "--stdio"], {"WS_MCP_CONFIG_MANIFESTS": "./m.json"}, "0.48.16")
        self.assertEqual(hello["env"]["WS_MCP_CONFIG_MANIFESTS"], os.path.join(os.getcwd(), "m.json"))

    def test_replay_into_ends_the_session_when_the_client_already_left(self) -> None:
        proxy = self.launcher.McpProxy(None, {}, io.BytesIO(), io.BytesIO(), io.StringIO())
        proxy.initialize = {"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {}}
        proxy.initialized = b'{"jsonrpc":"2.0","method":"notifications/initialized"}\n'
        proxy.inflight = {"n:7": 7}
        proxy.stdin_closed = True
        stream = RecordingStream()
        proxy.replay_into(stream)
        self.assertTrue(stream.closed, "nobody else will close the successor stream")
        self.assertEqual(stream.writes, [], "no replay into a session the client left")
        self.assertEqual(proxy.inflight, {})
        self.assertIs(proxy.stream, stream)

        # With the client still attached the replay goes through.
        proxy.stdin_closed = False
        proxy.inflight = {"n:7": 7}
        live = RecordingStream()
        proxy.replay_into(live)
        self.assertFalse(live.closed)
        self.assertEqual(len(live.writes), 2)
        self.assertIn(self.launcher.PROXY_REPLAY_ID_PREFIX.encode(), live.writes[0])

    def test_no_executable_probe_or_direct_stream_remains(self) -> None:
        for name in ("compat_probe_binary", "run_binary", "ProcessStream", "proxy_disabled"):
            self.assertFalse(hasattr(self.launcher, name), name)

    def setUp(self) -> None:
        time.sleep = _REAL_SLEEP
        self.launcher = load_launcher()
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.binary = Path(self.tmp.name) / "ws-mcp-0.48.16-abcdef123456"
        self.binary.write_bytes(b"#!/bin/sh\nexit 0\n")
        self.binary.chmod(0o755)


if __name__ == "__main__":
    unittest.main()
