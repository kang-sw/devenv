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
    log.write(json.dumps({"exe": sys.argv[0], "args": args, "env_marker": os.environ.get("FAKE_MARKER", "")}) + "\\n")
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

    def __init__(self, *, version="0.48.16", build_time=2_000_000_000_000, name="daemon", refuse_reason="", slow_methods=(), lan=False):
        self.version = version
        self.build_time = build_time
        self.name = name
        self.refuse_reason = refuse_reason
        self.slow_methods = set(slow_methods)
        self.lan = lan
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
                body = json.dumps({"service": "ws-dashboard", "version": self.version, "pid": 4242, "port": self.port, "started_at": "x", "lan": self.lan, "build_time": self.build_time}).encode()
                conn.sendall(b"HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: " + str(len(body)).encode() + b"\r\nConnection: close\r\n\r\n" + body)
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
        conn.sendall(json.dumps({"final": True, "exit_code": 7, "data": json.dumps(result) + "\n"}).encode() + b"\n")

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
                time.sleep(30)
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

    def close(self, timeout=5.0):
        try:
            self.writer.close()
        except OSError:
            pass
        self.thread.join(timeout)
        if self.thread.is_alive():
            raise AssertionError("proxy_serve did not return after stdin closed")
        return self.result.get("code")


class LauncherProxyTest(unittest.TestCase):
    def setUp(self):
        time.sleep = _REAL_SLEEP
        self.launcher = load_launcher()
        self.tmp = tempfile.TemporaryDirectory(prefix="ws-launcher-proxy.")
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.cache = self.root / "cache"
        self.cache.mkdir()
        self.daemon_dir = self.cache / "dashboard"
        self.binary = self.root / "ws-mcp-0.48.16-abcdef123456"
        self.binary.write_text(FAKE_RUNTIME, encoding="utf-8")
        self.binary.chmod(self.binary.stat().st_mode | stat.S_IXUSR | stat.S_IXGRP | stat.S_IXOTH)
        os.utime(self.binary, ns=(2_000_000_000_000_000_000, 2_000_000_000_000_000_000))  # build time 2_000_000_000_000 ms
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
        self.assertEqual(sorted(p.name for p in (self.daemon_dir / "bin").iterdir()), [staged.name])

    def test_an_older_daemon_or_an_older_build_is_replaced_a_newer_one_is_used(self):
        for version, build_time, want_start in [("0.48.15", 5000, True), ("0.48.16", 1999999999999, True), ("0.48.16", 2000000000000, False), ("0.48.16-dev", 1, False), ("0.48.20", 1, False)]:
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

    def test_a_cross_minor_daemon_sends_the_launcher_to_the_direct_path(self):
        daemon = self.daemon(version="0.49.0")
        daemon.write_endpoint(self.daemon_dir)
        stderr = io.StringIO()
        code = self.launcher.proxy_serve(self.link(), ["serve", "--stdio"], io.BytesIO(), io.BytesIO(), stderr)
        self.assertIsNone(code)
        self.assertIn("outside this plugin's minor", stderr.getvalue())
        self.assertEqual(stderr.getvalue().count("\n"), 1)
        self.assertEqual(daemon.hellos, [])

    def test_a_refused_hello_sends_the_launcher_to_the_direct_path(self):
        daemon = self.daemon(refuse_reason="hello carries no WS_RSRC_ROOT")
        daemon.write_endpoint(self.daemon_dir)
        stderr = io.StringIO()
        self.assertIsNone(self.launcher.proxy_serve(self.link(), ["serve", "--stdio"], io.BytesIO(), io.BytesIO(), stderr))
        self.assertIn("hello carries no WS_RSRC_ROOT", stderr.getvalue())

    def test_an_unanswering_daemon_falls_back_within_the_budget(self):
        silent = socket.socket()
        silent.bind(("127.0.0.1", 0))
        silent.listen(1)
        self.addCleanup(silent.close)
        self.daemon_dir.mkdir(parents=True)
        (self.daemon_dir / "endpoint.json").write_text(json.dumps({"port": silent.getsockname()[1], "pid": 1, "version": "0.48.16", "token": "t" * 64}), encoding="utf-8")
        self.point_start_at(fail=True)
        stderr = io.StringIO()
        started = time.monotonic()
        self.assertIsNone(self.launcher.proxy_serve(self.link(answer_budget=0.5), ["serve", "--stdio"], io.BytesIO(), io.BytesIO(), stderr))
        self.assertLess(time.monotonic() - started, 4.0)
        self.assertIn("no daemon could be started; running the direct path", stderr.getvalue())
        self.assertEqual([r["args"][0] for r in self.runtime_log()], ["daemon-start"])

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
        # The replayed initialize answer is dropped: the next line is the ping's.
        ping_id = client.send("ping")
        answer = client.recv()
        self.assertEqual(answer["id"], ping_id)
        self.assertEqual(answer["result"]["served_by"], "new")
        self.assertEqual(client.close(), 0)

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

    def test_a_daemon_that_dies_with_no_successor_and_no_restart_hands_over_to_direct_serve(self):
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
        self.assertEqual(answer["result"]["served_by"], "direct")
        self.assertEqual(answer["result"]["method"], "ping")
        self.assertEqual(client.close(), 0)
        self.assertIn("continuing on the direct path", client.stderr.getvalue())
        self.assertEqual([r["args"] for r in self.runtime_log()], [["daemon-start", "--build-time", "2000000000000"], ["serve", "--stdio"]])

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

    # --- mailbox wait -------------------------------------------------------------

    def test_mailbox_wait_runs_in_the_daemon_with_its_frames_unpacked(self):
        daemon = self.daemon()
        daemon.write_endpoint(self.daemon_dir)
        stdout, stderr = io.StringIO(), io.StringIO()
        code = self.launcher.proxy_cli(self.link(), ["mailbox", "wait", "--session-key", "k", "--format", "json"], stdout, stderr)
        self.assertEqual(code, 7)
        self.assertEqual(json.loads(stdout.getvalue()), {"served_by": "daemon", "args": ["--session-key", "k", "--format", "json"], "owner_pid": os.getpid()})
        self.assertEqual(stderr.getvalue(), "fake: waiting\n")
        self.assertEqual(daemon.hellos[0]["mode"], "cli")
        self.assertEqual(daemon.hellos[0]["argv"], ["mailbox", "wait", "--session-key", "k", "--format", "json"])
        self.assertEqual(self.runtime_log(), [])

    def test_mailbox_wait_falls_back_to_the_direct_path(self):
        self.point_start_at(fail=True)
        stderr = io.StringIO()
        self.assertIsNone(self.launcher.proxy_cli(self.link(), ["mailbox", "wait", "--session-key", "k"], io.StringIO(), stderr))
        self.assertIn("running the direct path", stderr.getvalue())

    def test_proxy_mode_covers_only_serve_stdio_and_mailbox_wait(self):
        self.assertEqual(self.launcher.proxy_mode(["serve", "--stdio"]), "mcp")
        self.assertEqual(self.launcher.proxy_mode(["mailbox", "wait", "--session-key", "k"]), "cli")
        self.assertIsNone(self.launcher.proxy_mode(["serve"]))
        self.assertIsNone(self.launcher.proxy_mode(["version"]))
        self.assertIsNone(self.launcher.proxy_mode(["mailbox", "send"]))
        self.assertIsNone(self.launcher.proxy_mode([]))
        self.assertTrue(self.launcher.proxy_disabled({"WS_MCP_LAUNCHER_DIRECT": "1"}))
        self.assertFalse(self.launcher.proxy_disabled({}))

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

    def test_staging_writes_the_copy_once_and_clears_other_entries(self):
        bin_dir = self.daemon_dir / "bin"
        bin_dir.mkdir(parents=True)
        (bin_dir / "ws-mcp-dashboard-stale").write_text("old", encoding="utf-8")
        staged = self.launcher.stage_daemon_binary(self.binary, bin_dir)
        self.assertEqual(staged.read_bytes(), self.binary.read_bytes())
        self.assertTrue(os.access(staged, os.X_OK))
        self.assertEqual([p.name for p in bin_dir.iterdir()], [staged.name])
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
        proxy = self.launcher.McpProxy(None, {}, ["serve", "--stdio"], io.BytesIO(), io.BytesIO(), io.StringIO())
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

    def test_compat_probe_runs_the_staged_copy_on_the_daemon_path(self) -> None:
        cache = Path(self.tmp.name) / "probe-cache"
        env = {"WS_CACHE_HOME": str(cache)}
        bin_dir = self.launcher.proxy_daemon_dir(env) / self.launcher.DAEMON_BIN_DIR_NAME
        probe = self.launcher.compat_probe_binary(self.binary, env, "mcp")
        self.assertEqual(probe.parent, bin_dir, "the probe runs the staged daemon copy, not the plugin-dir binary")
        self.assertEqual(probe.read_bytes(), self.binary.read_bytes())
        self.assertEqual(self.launcher.compat_probe_binary(self.binary, env, None), self.binary, "no proxy mode: the runtime binary itself")
        self.assertEqual(self.launcher.compat_probe_binary(self.binary, {**env, "WS_MCP_LAUNCHER_DIRECT": "1"}, "mcp"), self.binary)
        missing = Path(self.tmp.name) / "missing-binary"
        self.assertEqual(self.launcher.compat_probe_binary(missing, env, "mcp"), missing, "a missing runtime is the caller's to report")

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
