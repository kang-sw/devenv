"""Behavior tests for the session holder script (holder.py).

Each test runs a Holder in-process on a thread with shortened periods and
talks to it over a real loopback socket, using small Python test children so
the tests run on POSIX and on Windows alike.
"""

import base64
import importlib.util
import json
import os
import shutil
import socket
import sys
import tempfile
import threading
import time
import unittest
from pathlib import Path
from unittest import mock

HOLDER_PATH = Path(__file__).resolve().parent.parent / "holder.py"
_spec = importlib.util.spec_from_file_location("ws_session_holder", HOLDER_PATH)
holder_mod = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(holder_mod)

WINDOWS = os.name == "nt"
PY = sys.executable
TIMEOUT = 10.0


def py_child(code: str) -> list:
    return [PY, "-u", "-c", code]


# A child that runs until it is signalled.
LONG_SLEEPER = py_child("import time\nprint('ready', flush=True)\ntime.sleep(60)\n")
# A child that runs until its input is closed.
STDIN_EXIT = py_child("import sys\nprint('ready', flush=True)\nsys.stdin.read()\n")


def wait_until(pred, timeout=TIMEOUT, interval=0.02) -> bool:
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        if pred():
            return True
        time.sleep(interval)
    return pred()


def retry_sharing(action):
    """Run a file action, retrying a short while on PermissionError: on
    Windows another process (a file scanner) may briefly hold a file the
    holder has just written."""
    deadline = time.monotonic() + 5.0
    while True:
        try:
            return action()
        except PermissionError:
            if not WINDOWS or time.monotonic() >= deadline:
                raise
            time.sleep(0.05)


def read_json(path: str):
    def read():
        with open(path, encoding="utf-8") as f:
            return json.load(f)

    return retry_sharing(read)


def decode(msg: dict) -> bytes:
    return base64.b64decode(msg.get("data", ""))


def is_output(msg, needle: bytes, stream=None) -> bool:
    return (
        msg is not None
        and msg.get("event") == "output"
        and (stream is None or msg.get("stream") == stream)
        and needle in decode(msg)
    )


class Client:
    """A line-JSON client for one attach connection."""

    def __init__(self, port: int) -> None:
        self.sock = socket.create_connection(("127.0.0.1", port), timeout=TIMEOUT)
        self.reader = self.sock.makefile("rb")
        self.pending = []
        self.ids = 0
        self.hello = self._read()

    def _read(self):
        line = self.reader.readline()
        if not line:
            return None
        return json.loads(line)

    def send(self, message: dict) -> None:
        self.sock.sendall(json.dumps(message).encode("utf-8") + b"\n")

    def _next_id(self) -> int:
        self.ids += 1
        return self.ids

    def attach(self, token, protocol=None) -> dict:
        rid = self._next_id()
        msg = {"op": "attach", "id": rid, "token": token}
        msg["protocol"] = holder_mod.PROTOCOL_VERSION if protocol is None else protocol
        self.send(msg)
        return self.wait_reply(rid)

    def request(self, op: str, fields=None) -> dict:
        rid = self._next_id()
        self.send({"op": op, "id": rid, **(fields or {})})
        return self.wait_reply(rid)

    def wait_reply(self, rid) -> dict:
        while True:
            msg = self._read()
            if msg is None:
                raise EOFError("connection closed before the reply")
            if msg.get("reply") == rid:
                return msg
            self.pending.append(msg)

    def next_event(self):
        if self.pending:
            return self.pending.pop(0)
        return self._read()

    def events_until(self, pred) -> list:
        """Messages up to and including the first one pred accepts."""
        out = []
        while True:
            msg = self.next_event()
            if msg is None:
                raise EOFError(f"connection closed; got {out!r}")
            out.append(msg)
            if pred(msg):
                return out

    def at_eof(self) -> bool:
        return not self.pending and self._read() is None

    def close(self) -> None:
        for closer in (self.reader.close, lambda: self.sock.shutdown(socket.SHUT_RDWR), self.sock.close):
            try:
                closer()
            except OSError:
                pass


class HolderTestCase(unittest.TestCase):
    def setUp(self) -> None:
        self.tmp = tempfile.mkdtemp(prefix="ws-holder-test-")
        self.addCleanup(shutil.rmtree, self.tmp, ignore_errors=True)

    def start_holder(self, name="h1", **kwargs):
        kwargs.setdefault("idle_exit", 60.0)
        kwargs.setdefault("descriptor_check", 60.0)
        kwargs.setdefault("stop_grace", 1.0)
        directory = os.path.join(self.tmp, name)
        holder = holder_mod.Holder(directory, holder_id=name, session={"kind": "test"}, **kwargs)
        holder.result = {}
        holder.thread = threading.Thread(target=lambda: holder.result.setdefault("code", holder.run()), daemon=True)
        holder.thread.start()
        self.addCleanup(self._end_holder, holder)
        self.assertTrue(wait_until(lambda: holder.port != 0 and os.path.exists(self.descriptor_path(holder))))
        return holder

    def _end_holder(self, holder) -> None:
        holder.request_stop()
        holder.thread.join(timeout=15.0)
        child = holder.child
        if child is not None:
            proc = getattr(child, "proc", None)
            if proc is not None and proc.poll() is None:
                proc.kill()
                proc.wait(timeout=5.0)
            process = getattr(child, "process", None)
            if process is not None and child.k32.WaitForSingleObject(process, 0) != 0:
                child.k32.TerminateProcess(process, 1)
        self.assertFalse(holder.thread.is_alive(), "holder did not end")

    def descriptor_path(self, holder) -> str:
        return os.path.join(holder.dir, holder_mod.DESCRIPTOR_NAME)

    def connect(self, holder) -> Client:
        client = Client(holder.port)
        self.addCleanup(client.close)
        return client

    def attached(self, holder, stream=True) -> Client:
        client = self.connect(holder)
        reply = client.attach(holder.token)
        self.assertTrue(reply["ok"], reply)
        if stream:
            self.assertTrue(client.request("replay", {"from": 0})["ok"])
        return client

    def spawn(self, client, argv, **fields) -> dict:
        reply = client.request("spawn", {"argv": argv, **fields})
        self.assertTrue(reply["ok"], reply)
        return reply

    def wait_exit_record(self, client) -> dict:
        return client.events_until(lambda m: m.get("event") == "exit")[-1]


class ProtocolVersionTest(HolderTestCase):
    def test_banner_attach_reply_and_descriptor_report_the_version(self):
        holder = self.start_holder()
        client = self.connect(holder)
        self.assertEqual(client.hello, {"hello": "ws-holder", "protocol": holder_mod.PROTOCOL_VERSION})
        reply = client.attach(holder.token)
        self.assertTrue(reply["ok"], reply)
        self.assertEqual(reply["protocol"], holder_mod.PROTOCOL_VERSION)
        self.assertEqual(reply["holder"]["id"], "h1")
        self.assertEqual(reply["holder"]["session"], {"kind": "test"})
        self.assertEqual(reply["child"], {"state": "none"})
        self.assertEqual((reply["first_seq"], reply["next_seq"]), (1, 1))
        desc = read_json(self.descriptor_path(holder))
        self.assertEqual(desc["protocol"], holder_mod.PROTOCOL_VERSION)
        self.assertEqual(desc["token"], holder.token)
        self.assertEqual(desc["port"], holder.port)
        self.assertEqual(desc["pid"], os.getpid())

    def test_accepts_its_version_and_the_one_before_and_refuses_others(self):
        # Only one protocol version exists today, so raise the holder's
        # version to make "the one before it" a real accepted version.
        with mock.patch.object(holder_mod, "PROTOCOL_VERSION", 2):
            holder = self.start_holder()
            for spoken in (2, 1):
                client = self.connect(holder)
                self.assertEqual(client.hello["protocol"], 2)
                reply = client.attach(holder.token, protocol=spoken)
                self.assertTrue(reply["ok"], reply)
                self.assertEqual(reply["protocol"], spoken)
            for spoken in (0, 3, "2", True):
                client = self.connect(holder)
                reply = client.attach(holder.token, protocol=spoken)
                self.assertFalse(reply["ok"], reply)
                self.assertEqual(reply["error"], "protocol")
                self.assertEqual(reply["protocol"], 2)
                self.assertTrue(client.at_eof())

    def test_version_one_has_no_version_before_it(self):
        with mock.patch.object(holder_mod, "PROTOCOL_VERSION", 1):
            holder = self.start_holder()
            self.assertTrue(self.connect(holder).attach(holder.token, protocol=1)["ok"])
            for spoken in (0, 2):
                reply = self.connect(holder).attach(holder.token, protocol=spoken)
                self.assertEqual(reply["error"], "protocol", reply)


class AuthenticationTest(HolderTestCase):
    def test_attach_without_or_with_a_wrong_token_is_refused_and_closed(self):
        holder = self.start_holder()
        for token in (None, "", "0" * 64, holder.token[:-1]):
            client = self.connect(holder)
            rid = 7
            msg = {"op": "attach", "id": rid, "protocol": holder_mod.PROTOCOL_VERSION}
            if token is not None:
                msg["token"] = token
            client.send(msg)
            reply = client.wait_reply(rid)
            self.assertEqual(reply["ok"], False)
            self.assertEqual(reply["error"], "token")
            self.assertTrue(client.at_eof())
        # A refused attach does not displace the holder's state: a right
        # token still works.
        self.assertTrue(self.connect(holder).attach(holder.token)["ok"])

    def test_first_line_must_be_an_attach(self):
        holder = self.start_holder()
        client = self.connect(holder)
        client.send({"op": "status", "id": 1, "token": holder.token})
        reply = client.wait_reply(1)
        self.assertEqual(reply["error"], "bad_request")
        self.assertTrue(client.at_eof())

    @unittest.skipIf(WINDOWS, "POSIX file modes do not apply on Windows")
    def test_descriptor_is_private_to_the_owner(self):
        holder = self.start_holder()
        self.assertEqual(os.stat(self.descriptor_path(holder)).st_mode & 0o777, 0o600)
        self.assertEqual(os.stat(holder.dir).st_mode & 0o777, 0o700)


class RingReplayTest(HolderTestCase):
    def _filled_holder(self):
        # 200 lines of 10 bytes cost far more than the 2000-byte ring holds.
        holder = self.start_holder(ring_bytes=2000)
        client = self.attached(holder, stream=False)
        self.spawn(client, py_child("import sys\nfor i in range(200): sys.stdout.buffer.write(b'line %04d\\n' % i)\n"))
        self.assertTrue(
            wait_until(lambda: client.request("status")["child"]["state"] == "exited"),
        )
        status = client.request("status")
        self.assertGreater(status["first_seq"], 1)
        self.assertEqual(status["next_seq"], 202)  # 200 lines and the exit record
        client.close()
        return holder, status["first_seq"]

    def _replay(self, holder, from_seq):
        client = self.attached(holder, stream=False)
        reply = client.request("replay", {"from": from_seq})
        self.assertTrue(reply["ok"], reply)
        events = client.events_until(lambda m: m.get("event") == "exit")
        client.close()
        return reply, events

    def test_replay_reports_a_gap_when_the_sequence_left_the_ring(self):
        holder, first = self._filled_holder()
        reply, events = self._replay(holder, 1)
        self.assertEqual(reply["first_seq"], first)
        self.assertEqual(events[0], {"event": "gap", "from": 1, "to": first})
        seqs = [m["seq"] for m in events[1:]]
        self.assertEqual(seqs, list(range(first, 202)))
        self.assertEqual(decode(events[-2]), b"line 0199\n")
        self.assertEqual(events[-1]["exit_code"], 0)

    def test_replay_from_zero_or_below_sends_the_whole_ring_without_a_gap(self):
        holder, first = self._filled_holder()
        for from_seq in (0, -5):
            _, events = self._replay(holder, from_seq)
            self.assertEqual([m["seq"] for m in events], list(range(first, 202)))

    def test_replay_from_a_held_sequence_starts_there(self):
        holder, first = self._filled_holder()
        _, events = self._replay(holder, first + 5)
        self.assertEqual([m["seq"] for m in events], list(range(first + 5, 202)))
        self.assertEqual(decode(events[0]), b"line %04d\n" % (first + 5 - 1))


class IdleExitTest(HolderTestCase):
    def test_never_idles_out_while_the_child_lives_then_exits_after_it(self):
        holder = self.start_holder(idle_exit=0.4)
        client = self.attached(holder)
        self.spawn(client, STDIN_EXIT)
        client.events_until(lambda m: is_output(m, b"ready"))
        client.close()
        # Nobody attached and the child alive: well past the idle period the
        # holder is still up.
        time.sleep(2.0)
        self.assertTrue(holder.thread.is_alive())
        client = self.attached(holder)
        self.assertTrue(client.request("input", {"data": "", "eof": True})["ok"])
        self.wait_exit_record(client)
        # Attached, with the child gone: still up.
        time.sleep(1.5)
        self.assertTrue(holder.thread.is_alive())
        client.close()
        holder.thread.join(timeout=TIMEOUT)
        self.assertFalse(holder.thread.is_alive())
        self.assertEqual(holder.result.get("code"), 0)
        self.assertFalse(os.path.exists(holder.dir))

    def test_idles_out_when_no_child_was_ever_spawned(self):
        holder = self.start_holder(idle_exit=0.3)
        holder.thread.join(timeout=TIMEOUT)
        self.assertFalse(holder.thread.is_alive())
        self.assertFalse(os.path.exists(holder.dir))

    def test_an_attachment_keeps_a_childless_holder_up(self):
        holder = self.start_holder(idle_exit=0.3)
        client = self.attached(holder)
        time.sleep(1.5)
        self.assertTrue(holder.thread.is_alive())
        self.assertTrue(client.request("status")["ok"])


class TakeoverTest(HolderTestCase):
    def test_newer_authenticated_attach_closes_the_previous_one(self):
        holder = self.start_holder()
        old = self.attached(holder, stream=False)
        new = self.connect(holder)
        self.assertTrue(new.attach(holder.token)["ok"])
        self.assertEqual(old.next_event(), {"event": "detached", "reason": "replaced"})
        self.assertTrue(old.at_eof())
        self.assertTrue(new.request("status")["ok"])
        self.spawn(new, STDIN_EXIT)

    def test_refused_attach_leaves_the_current_one_in_place(self):
        holder = self.start_holder()
        current = self.attached(holder, stream=False)
        self.assertFalse(self.connect(holder).attach("bad")["ok"])
        self.assertTrue(current.request("status")["ok"])


class ChildTest(HolderTestCase):
    def test_pipe_child_streams_echoes_input_and_reports_its_exit_code(self):
        holder = self.start_holder()
        client = self.attached(holder)
        code = (
            "import sys\n"
            "sys.stderr.buffer.write(b'err line\\n'); sys.stderr.flush()\n"
            "for line in sys.stdin.buffer:\n"
            "    sys.stdout.buffer.write(b'got:' + line); sys.stdout.flush()\n"
            "sys.exit(3)\n"
        )
        reply = self.spawn(client, py_child(code), mode="pipe")
        self.assertEqual(reply["child"]["state"], "running")
        self.assertEqual(reply["child"]["mode"], "pipe")
        data = base64.b64encode(b"hello\nworld\n").decode("ascii")
        self.assertTrue(client.request("input", {"data": data, "eof": True})["ok"])
        events = client.events_until(lambda m: m.get("event") == "exit")
        stdout = [decode(m) for m in events if m.get("stream") == "stdout"]
        stderr = [decode(m) for m in events if m.get("stream") == "stderr"]
        self.assertEqual(stdout, [b"got:hello\n", b"got:world\n"])
        self.assertEqual(stderr, [b"err line\n"])
        seqs = [m["seq"] for m in events]
        self.assertEqual(seqs, sorted(seqs))
        self.assertEqual((events[-1]["exit_code"], events[-1]["signal"]), (3, None))
        status = client.request("status")["child"]
        self.assertEqual(status["state"], "exited")
        self.assertEqual(status["exit_code"], 3)
        # The child is gone, so child operations are refused.
        self.assertEqual(client.request("input", {"data": data})["error"], "no_child")

    def test_second_spawn_is_refused(self):
        holder = self.start_holder()
        client = self.attached(holder)
        self.spawn(client, STDIN_EXIT)
        reply = client.request("spawn", {"argv": STDIN_EXIT})
        self.assertEqual(reply["error"], "already_spawned")

    def test_bad_requests_are_refused_without_ending_the_connection(self):
        holder = self.start_holder()
        client = self.attached(holder)
        self.assertEqual(client.request("spawn", {"argv": []})["error"], "bad_request")
        self.assertEqual(client.request("frobnicate")["error"], "unknown_op")
        self.assertEqual(client.request("input", {"data": "aGk="})["error"], "no_child")
        self.spawn(client, STDIN_EXIT)
        self.assertEqual(client.request("resize", {"rows": 10, "cols": 10})["error"], "not_pty")
        self.assertEqual(client.request("signal", {"name": "USR1"})["error"], "bad_request")
        self.assertEqual(client.request("input", {"data": "!!"})["error"], "bad_request")

    def test_signal_by_name_ends_the_child(self):
        holder = self.start_holder()
        client = self.attached(holder)
        self.spawn(client, LONG_SLEEPER)
        client.events_until(lambda m: is_output(m, b"ready"))
        self.assertTrue(client.request("signal", {"name": "TERM"})["ok"])
        exit_rec = self.wait_exit_record(client)
        status = client.request("status")["child"]
        if WINDOWS:
            # Windows has no signals: TERM ends the child's process tree.
            self.assertIsNone(exit_rec["signal"])
            self.assertNotEqual(exit_rec["exit_code"], 0)
        else:
            self.assertEqual((exit_rec["exit_code"], exit_rec["signal"]), (None, 15))
            self.assertEqual(status["signal"], 15)
        self.assertEqual(status["state"], "exited")

    def test_stop_ends_the_child_and_the_holder(self):
        holder = self.start_holder(stop_grace=1.0)
        client = self.attached(holder)
        self.spawn(client, LONG_SLEEPER)
        client.events_until(lambda m: is_output(m, b"ready"))
        reply = client.request("stop")
        self.assertTrue(reply["ok"], reply)
        holder.thread.join(timeout=TIMEOUT)
        self.assertFalse(holder.thread.is_alive())
        self.assertIsNotNone(holder.child.proc.poll())
        self.assertFalse(os.path.exists(holder.dir))
        # The connection drains the remaining records, then closes.
        events = client.events_until(lambda m: m.get("event") == "exit")
        self.assertEqual(events[-1]["event"], "exit")
        self.assertTrue(client.at_eof())

    def test_pty_child_sees_its_size_and_a_resize(self):
        holder = self.start_holder()
        client = self.attached(holder)
        code = (
            "import os,sys\n"
            "def size():\n"
            "    s = os.get_terminal_size(1)\n"
            "    return 'SZ=%dx%d' % (s.lines, s.columns)\n"
            "print(size(), flush=True)\n"
            "sys.stdin.readline()\n"
            "print(size(), flush=True)\n"
            "sys.exit(5)\n"
        )
        reply = self.spawn(client, py_child(code), mode="pty", rows=24, cols=80)
        self.assertEqual(reply["child"]["mode"], "pty")
        seen = bytearray()

        def saw(needle):
            def pred(msg):
                if msg.get("event") == "output":
                    self.assertEqual(msg["stream"], "pty")
                    seen.extend(decode(msg))
                return needle in seen or msg.get("event") == "exit"

            return pred

        client.events_until(saw(b"SZ=24x80"))
        self.assertIn(b"SZ=24x80", seen)
        self.assertTrue(client.request("resize", {"rows": 40, "cols": 100})["ok"])
        self.assertEqual(client.request("resize", {"rows": 0, "cols": 100})["error"], "bad_request")
        data = base64.b64encode(b"go\r").decode("ascii")
        self.assertTrue(client.request("input", {"data": data})["ok"])
        client.events_until(saw(b"SZ=40x100"))
        self.assertIn(b"SZ=40x100", seen)
        exit_rec = self.wait_exit_record(client)
        self.assertEqual(exit_rec["exit_code"], 5)


class DescriptorTest(HolderTestCase):
    def test_deleted_descriptor_is_rewritten(self):
        holder = self.start_holder(descriptor_check=0.1)
        path = self.descriptor_path(holder)
        before = read_json(path)
        retry_sharing(lambda: os.unlink(path))
        self.assertTrue(wait_until(lambda: os.path.exists(path)))
        after = read_json(path)
        self.assertEqual(after, before)
        if not WINDOWS:
            self.assertEqual(os.stat(path).st_mode & 0o777, 0o600)


if __name__ == "__main__":
    unittest.main()
