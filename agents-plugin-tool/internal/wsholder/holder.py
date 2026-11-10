#!/usr/bin/env python3
"""ws session holder: owns one child process and outlives the dashboard daemon.

A holder is started detached by the dashboard daemon (one holder per hosted
agent process or terminal). It owns the child (a process over pipes, or a
shell over a PTY), keeps the child's output in a bounded ring, and serves one
attach connection at a time on a loopback TCP port. The daemon may exit and
restart; its successor reads the holder's descriptor file, attaches, and
replays the ring, so an in-flight turn or shell continues.

Standard library only, Python 3.10 or newer.

State directory (one per holder, mode 0700): <dir>/descriptor.json (0600)

    {"schema": 1, "id": "<holder id>", "pid": <holder pid>,
     "port": <loopback TCP port>, "token": "<hex>", "protocol": 1,
     "started_at": <wall-clock start, epoch seconds>,
     "start_ticks": <start in clock ticks since boot; Linux only>,
     "script_hash": "<sha256 of this script>", "restart_safe": true,
     "session": {...}}

The holder rewrites the descriptor when it goes missing (checked every 60 s)
and removes the directory when it exits on its own.

Attach protocol, version 1. Newline-delimited JSON objects, UTF-8, both ways.

  holder -> client, on connect:  {"hello": "ws-holder", "protocol": 1}
  client -> holder, first line:  {"op": "attach", "id": <any>, "token": "...",
                                  "protocol": <version the client speaks>}
  holder -> client:  {"reply": <id>, "ok": true, "protocol": <spoken>,
                      "holder": {"id", "pid", "started_at", "script_hash",
                                 "restart_safe", "session"},
                      "child": <child status>, "first_seq": n, "next_seq": n}
      or {"reply": <id>, "ok": false, "error": "token" | "protocol" |
          "bad_request", "message": "..."} and the connection closes.

  The holder accepts the protocol version it reports and the one before it.
  The newest authenticated attach takes over: the previous connection gets
  {"event": "detached", "reason": "replaced"} and is closed, and only the
  new one may send operations.

  Operations (each answered {"reply": <id>, "ok": true, ...} or
  {"reply": <id>, "ok": false, "error": "<code>", "message": "..."}):

    spawn   {"argv": [...], "cwd": "...", "env": {...}, "mode": "pipe" |
             "pty", "rows": 24, "cols": 80} -> {"child": <status>}
            One child per holder. env, when given, is the whole environment.
    input   {"data": "<base64>", "eof": false}   eof closes a pipe child's
            stdin after the data.
    resize  {"rows": n, "cols": n}               PTY children only.
    signal  {"name": "INT" | "TERM" | "KILL" | "HUP" | "QUIT"}
    stop    {}   ends the child (TERM, then KILL after a grace period) and
            exits the holder.
    status  {} -> {"child": <status>, "first_seq": n, "next_seq": n}
    replay  {"from": n} -> {"first_seq": n, "next_seq": n}
            Starts the record stream at sequence n; n <= 0 means the whole
            ring. When n has already left the ring the stream begins with
            {"event": "gap", "from": n, "to": <first held sequence>}.

  Child status: {"state": "none"} | {"state": "running", "pid", "mode"} |
  {"state": "exited", "pid", "mode", "exit_code", "signal"}.

  Records (sequenced from 1, kept in a 4 MiB ring, streamed after replay):

    {"event": "output", "seq": n, "stream": "stdout" | "stderr" | "pty",
     "data": "<base64>"}   plus "partial": true when a pipe line was cut at
                           the record limit and continues in the next record.
    {"event": "exit", "seq": n, "exit_code": <int|null>, "signal": <int|null>}

  A pipe child's output is one record per line (the newline included); a PTY
  child's output is one record per read.

Self-exit: once the child has exited (or none was ever spawned) and nobody
has been attached for the idle period (24 hours), the holder exits. A live
child never triggers it.
"""

import time

# The wall-clock start, taken before the remaining imports so it sits close
# to the OS creation time the daemon compares it with before a guarded kill.
STARTED_AT = time.time()

import argparse
import base64
import hashlib
import hmac
import json
import os
import secrets
import shutil
import signal
import socket
import subprocess
import sys
import threading

PROTOCOL_VERSION = 1
SCHEMA_VERSION = 1
DESCRIPTOR_NAME = "descriptor.json"
LOG_NAME = "holder.log"


def _start_ticks():
    """This process's start in clock ticks since boot, where the OS has it.

    Linux only (/proc/self/stat, field 22). The daemon derives a Linux
    process's creation time from the boot clock, so the wall-clock start
    stops matching it once the wall clock steps; the ticks never change.
    """
    try:
        with open("/proc/self/stat", encoding="ascii", errors="replace") as f:
            text = f.read()
        # The command name, in parentheses, may hold spaces and parentheses.
        fields = text[text.rindex(")") + 1 :].split()
        return int(fields[22 - 3])
    except (OSError, ValueError, IndexError):
        return None


START_TICKS = _start_ticks() if sys.platform.startswith("linux") else None

RING_BYTES = 4 << 20
MAX_RECORD_BYTES = 1 << 20
IDLE_EXIT_SECONDS = 24 * 60 * 60.0
DESCRIPTOR_CHECK_SECONDS = 60.0
STOP_GRACE_SECONDS = 5.0
AUTH_TIMEOUT_SECONDS = 10.0
READ_CHUNK = 65536
# One request line: an input operation carries at most this much base64.
MAX_LINE_BYTES = 4 << 20
# Input the child has not read yet; beyond it an input operation is refused.
MAX_PENDING_INPUT = 8 << 20
# One writer pass sends at most this many records and bytes.
SEND_BATCH_RECORDS = 256
SEND_BATCH_BYTES = 256 << 10

WINDOWS = os.name == "nt"

SIGNAL_NAMES = ("INT", "TERM", "KILL", "HUP", "QUIT")


class OpError(Exception):
    """An operation's refusal: a short code and a message for a person."""

    def __init__(self, code: str, message: str) -> None:
        super().__init__(message)
        self.code = code
        self.message = message


class Record:
    __slots__ = ("seq", "kind", "stream", "data", "partial", "fields")

    def __init__(self, seq, kind, stream=None, data=b"", partial=False, fields=None):
        self.seq = seq
        self.kind = kind
        self.stream = stream
        self.data = data
        self.partial = partial
        self.fields = fields

    def cost(self) -> int:
        return len(self.data) + 64

    def message(self) -> dict:
        if self.kind == "exit":
            msg = {"event": "exit", "seq": self.seq}
            msg.update(self.fields)
            return msg
        msg = {
            "event": "output",
            "seq": self.seq,
            "stream": self.stream,
            "data": base64.b64encode(self.data).decode("ascii"),
        }
        if self.partial:
            msg["partial"] = True
        return msg


class Ring:
    """Sequenced records bounded by a byte budget; the newest always stays."""

    def __init__(self, capacity: int) -> None:
        self.capacity = capacity
        self.items: list[Record] = []
        self.head = 0
        self.size = 0
        self.next_seq = 1

    def first_seq(self) -> int:
        if self.head < len(self.items):
            return self.items[self.head].seq
        return self.next_seq

    def append(self, kind, stream=None, data=b"", partial=False, fields=None) -> Record:
        rec = Record(self.next_seq, kind, stream, data, partial, fields)
        self.next_seq += 1
        self.items.append(rec)
        self.size += rec.cost()
        while self.size > self.capacity and len(self.items) - self.head > 1:
            self.size -= self.items[self.head].cost()
            self.items[self.head] = None
            self.head += 1
        if self.head > 1024 and self.head > len(self.items) // 2:
            del self.items[: self.head]
            self.head = 0
        return rec

    def batch(self, seq: int, max_records: int, max_bytes: int) -> list[Record]:
        """Records from seq on (seq is at least first_seq), bounded."""
        start = self.head + (seq - self.first_seq())
        out, total = [], 0
        for rec in self.items[start : start + max_records]:
            out.append(rec)
            total += rec.cost()
            if total >= max_bytes:
                break
        return out


class LineSplitter:
    """Cuts a pipe's bytes into line records, each at most limit bytes."""

    def __init__(self, limit: int) -> None:
        self.limit = limit
        self.buf = bytearray()

    def feed(self, chunk: bytes):
        self.buf += chunk
        while True:
            nl = self.buf.find(b"\n")
            if 0 <= nl < self.limit:
                line = bytes(self.buf[: nl + 1])
                del self.buf[: nl + 1]
                yield line, False
            elif len(self.buf) >= self.limit:
                part = bytes(self.buf[: self.limit])
                del self.buf[: self.limit]
                yield part, True
            else:
                return

    def flush(self):
        if self.buf:
            rest = bytes(self.buf)
            self.buf.clear()
            yield rest, False


# --- children ---------------------------------------------------------------


def _exit_fields(returncode: int) -> dict:
    if not WINDOWS and returncode < 0:
        return {"exit_code": None, "signal": -returncode}
    return {"exit_code": returncode, "signal": None}


def _kill_tree_windows(pid: int) -> None:
    subprocess.run(
        ["taskkill", "/PID", str(pid), "/T", "/F"],
        stdin=subprocess.DEVNULL,
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
        creationflags=getattr(subprocess, "CREATE_NO_WINDOW", 0),
        check=False,
    )


class PipeChild:
    """A child over three pipes. On POSIX it stays in the holder's process
    group, so a kill of that group (the daemon's guarded kill) ends it too."""

    mode = "pipe"

    def __init__(self, spec: dict) -> None:
        flags = subprocess.CREATE_NEW_PROCESS_GROUP if WINDOWS else 0
        self.proc = subprocess.Popen(
            spec["argv"],
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            cwd=spec["cwd"],
            env=spec["env"],
            bufsize=0,
            creationflags=flags,
        )
        self.pid = self.proc.pid

    def outputs(self):
        return [("stdout", self.proc.stdout.fileno()), ("stderr", self.proc.stderr.fileno())]

    def read(self, fd: int) -> bytes:
        try:
            return os.read(fd, READ_CHUNK)
        except OSError:
            return b""

    def write(self, data: bytes) -> None:
        view = memoryview(data)
        while view:
            n = self.proc.stdin.write(view)
            view = view[n if n else len(view) :]

    def close_input(self) -> None:
        try:
            self.proc.stdin.close()
        except OSError:
            pass

    def resize(self, rows: int, cols: int) -> None:
        raise OpError("not_pty", "the child runs over pipes, not a PTY")

    def signal(self, name: str) -> None:
        if WINDOWS:
            if name == "INT":
                os.kill(self.pid, signal.CTRL_BREAK_EVENT)
            else:
                _kill_tree_windows(self.pid)
            return
        os.kill(self.pid, getattr(signal, "SIG" + name))

    def wait(self) -> dict:
        return _exit_fields(self.proc.wait())

    def after_exit(self) -> None:
        pass

    def close(self) -> None:
        for f in (self.proc.stdin, self.proc.stdout, self.proc.stderr):
            try:
                f.close()
            except OSError:
                pass


# The PTY child starts through this trampoline: a fresh interpreter that is
# the session leader (start_new_session), takes the PTY as its controlling
# terminal, and execs the target. It keeps Python code out of the forked
# child of this threaded process, which preexec_fn would not.
_PTY_TRAMPOLINE = (
    "import fcntl,os,sys,termios\n"
    "fcntl.ioctl(0,termios.TIOCSCTTY,0)\n"
    "try:\n"
    " os.execvp(sys.argv[1],sys.argv[1:])\n"
    "except OSError as e:\n"
    " sys.stderr.write('exec failed: %s\\n'%e)\n"
    " os._exit(127)\n"
)


class PosixPtyChild:
    mode = "pty"

    def __init__(self, spec: dict) -> None:
        import pty

        env = spec["env"]
        path = (env if env is not None else os.environ).get("PATH")
        program = spec["argv"][0]
        if os.sep in program:
            base = spec["cwd"] or os.getcwd()
            if not os.access(os.path.join(base, program), os.X_OK):
                raise OSError(f"{program}: not an executable file")
        elif shutil.which(program, path=path) is None:
            raise OSError(f"{program}: not found on PATH")
        self.master, slave = pty.openpty()
        try:
            self._set_size(spec["rows"], spec["cols"])
            self.proc = subprocess.Popen(
                [sys.executable or "python3", "-I", "-c", _PTY_TRAMPOLINE, *spec["argv"]],
                stdin=slave,
                stdout=slave,
                stderr=slave,
                cwd=spec["cwd"],
                env=env,
                start_new_session=True,
            )
        except BaseException:
            os.close(self.master)
            raise
        finally:
            os.close(slave)
        self.pid = self.proc.pid

    def _set_size(self, rows: int, cols: int) -> None:
        import fcntl
        import struct
        import termios

        fcntl.ioctl(self.master, termios.TIOCSWINSZ, struct.pack("HHHH", rows, cols, 0, 0))

    def outputs(self):
        return [("pty", self.master)]

    def read(self, fd: int) -> bytes:
        try:
            return os.read(fd, READ_CHUNK)
        except OSError:  # EIO once every slave end is closed
            return b""

    def write(self, data: bytes) -> None:
        view = memoryview(data)
        while view:
            view = view[os.write(self.master, view) :]

    def close_input(self) -> None:
        self.write(b"\x04")

    def resize(self, rows: int, cols: int) -> None:
        self._set_size(rows, cols)

    def signal(self, name: str) -> None:
        # The child is its session's leader, so its pid names its group.
        os.killpg(self.pid, getattr(signal, "SIG" + name))

    def wait(self) -> dict:
        return _exit_fields(self.proc.wait())

    def after_exit(self) -> None:
        pass

    def close(self) -> None:
        try:
            os.close(self.master)
        except OSError:
            pass


class ConPtyChild:
    """A child on a Windows pseudoconsole, through ctypes."""

    mode = "pty"

    def __init__(self, spec: dict) -> None:
        import ctypes
        from ctypes import wintypes

        self.ctypes = ctypes
        k32 = ctypes.WinDLL("kernel32", use_last_error=True)
        self.k32 = k32
        HANDLE, DWORD, BOOL = wintypes.HANDLE, wintypes.DWORD, wintypes.BOOL
        LPVOID, SIZE_T = ctypes.c_void_p, ctypes.c_size_t

        class COORD(ctypes.Structure):
            _fields_ = [("X", ctypes.c_short), ("Y", ctypes.c_short)]

        class STARTUPINFOW(ctypes.Structure):
            _fields_ = [
                ("cb", DWORD),
                ("lpReserved", wintypes.LPWSTR),
                ("lpDesktop", wintypes.LPWSTR),
                ("lpTitle", wintypes.LPWSTR),
                ("dwX", DWORD),
                ("dwY", DWORD),
                ("dwXSize", DWORD),
                ("dwYSize", DWORD),
                ("dwXCountChars", DWORD),
                ("dwYCountChars", DWORD),
                ("dwFillAttribute", DWORD),
                ("dwFlags", DWORD),
                ("wShowWindow", wintypes.WORD),
                ("cbReserved2", wintypes.WORD),
                ("lpReserved2", LPVOID),
                ("hStdInput", HANDLE),
                ("hStdOutput", HANDLE),
                ("hStdError", HANDLE),
            ]

        class STARTUPINFOEXW(ctypes.Structure):
            _fields_ = [("StartupInfo", STARTUPINFOW), ("lpAttributeList", LPVOID)]

        class PROCESS_INFORMATION(ctypes.Structure):
            _fields_ = [
                ("hProcess", HANDLE),
                ("hThread", HANDLE),
                ("dwProcessId", DWORD),
                ("dwThreadId", DWORD),
            ]

        self.COORD = COORD
        k32.CreatePipe.argtypes = [ctypes.POINTER(HANDLE), ctypes.POINTER(HANDLE), LPVOID, DWORD]
        k32.CreatePipe.restype = BOOL
        k32.CreatePseudoConsole.argtypes = [COORD, HANDLE, HANDLE, DWORD, ctypes.POINTER(HANDLE)]
        k32.CreatePseudoConsole.restype = ctypes.c_long
        k32.ResizePseudoConsole.argtypes = [HANDLE, COORD]
        k32.ResizePseudoConsole.restype = ctypes.c_long
        k32.ClosePseudoConsole.argtypes = [HANDLE]
        k32.ClosePseudoConsole.restype = None
        k32.InitializeProcThreadAttributeList.argtypes = [LPVOID, DWORD, DWORD, ctypes.POINTER(SIZE_T)]
        k32.InitializeProcThreadAttributeList.restype = BOOL
        k32.UpdateProcThreadAttribute.argtypes = [LPVOID, DWORD, SIZE_T, LPVOID, SIZE_T, LPVOID, LPVOID]
        k32.UpdateProcThreadAttribute.restype = BOOL
        k32.DeleteProcThreadAttributeList.argtypes = [LPVOID]
        k32.DeleteProcThreadAttributeList.restype = None
        k32.CreateProcessW.argtypes = [
            wintypes.LPCWSTR,
            wintypes.LPWSTR,
            LPVOID,
            LPVOID,
            BOOL,
            DWORD,
            LPVOID,
            wintypes.LPCWSTR,
            ctypes.POINTER(STARTUPINFOEXW),
            ctypes.POINTER(PROCESS_INFORMATION),
        ]
        k32.CreateProcessW.restype = BOOL
        k32.ReadFile.argtypes = [HANDLE, LPVOID, DWORD, ctypes.POINTER(DWORD), LPVOID]
        k32.ReadFile.restype = BOOL
        k32.WriteFile.argtypes = [HANDLE, LPVOID, DWORD, ctypes.POINTER(DWORD), LPVOID]
        k32.WriteFile.restype = BOOL
        k32.WaitForSingleObject.argtypes = [HANDLE, DWORD]
        k32.WaitForSingleObject.restype = DWORD
        k32.GetExitCodeProcess.argtypes = [HANDLE, ctypes.POINTER(DWORD)]
        k32.GetExitCodeProcess.restype = BOOL
        k32.TerminateProcess.argtypes = [HANDLE, ctypes.c_uint]
        k32.TerminateProcess.restype = BOOL
        k32.CloseHandle.argtypes = [HANDLE]
        k32.CloseHandle.restype = BOOL

        def pipe():
            read, write = HANDLE(), HANDLE()
            if not k32.CreatePipe(ctypes.byref(read), ctypes.byref(write), None, 0):
                raise ctypes.WinError(ctypes.get_last_error())
            return read, write

        self.lock = threading.Lock()
        self.closed = False
        in_read, self.in_write = pipe()
        self.out_read, out_write = pipe()
        self.hpc = HANDLE()
        hr = k32.CreatePseudoConsole(COORD(spec["cols"], spec["rows"]), in_read, out_write, 0, ctypes.byref(self.hpc))
        # The pseudoconsole holds its own duplicates of the two pipe ends.
        k32.CloseHandle(in_read)
        k32.CloseHandle(out_write)
        if hr != 0:
            k32.CloseHandle(self.in_write)
            k32.CloseHandle(self.out_read)
            raise OSError(f"CreatePseudoConsole failed: HRESULT {hr & 0xFFFFFFFF:#010x}")

        size = SIZE_T(0)
        k32.InitializeProcThreadAttributeList(None, 1, 0, ctypes.byref(size))
        self.attr_buf = ctypes.create_string_buffer(size.value)
        startup = STARTUPINFOEXW()
        startup.StartupInfo.cb = ctypes.sizeof(STARTUPINFOEXW)
        # STARTF_USESTDHANDLES with null handles: without it the child takes
        # the holder's own redirected standard handles instead of the
        # pseudoconsole's on Windows builds before 19H1.
        startup.StartupInfo.dwFlags = 0x00000100
        startup.lpAttributeList = ctypes.cast(self.attr_buf, LPVOID)
        info = PROCESS_INFORMATION()
        try:
            if not k32.InitializeProcThreadAttributeList(startup.lpAttributeList, 1, 0, ctypes.byref(size)):
                raise ctypes.WinError(ctypes.get_last_error())
            # PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE takes the handle itself.
            if not k32.UpdateProcThreadAttribute(
                startup.lpAttributeList, 0, 0x00020016, self.hpc, ctypes.sizeof(HANDLE), None, None
            ):
                raise ctypes.WinError(ctypes.get_last_error())
            env_block = None
            if spec["env"] is not None:
                env_block = ctypes.create_unicode_buffer(
                    "".join(f"{k}={v}\0" for k, v in spec["env"].items()) + "\0"
                )
            cmdline = ctypes.create_unicode_buffer(subprocess.list2cmdline(spec["argv"]))
            # EXTENDED_STARTUPINFO_PRESENT | CREATE_UNICODE_ENVIRONMENT
            if not k32.CreateProcessW(
                None, cmdline, None, None, False, 0x00080000 | 0x00000400, env_block, spec["cwd"],
                ctypes.byref(startup), ctypes.byref(info),
            ):
                raise ctypes.WinError(ctypes.get_last_error())
        except BaseException:
            k32.ClosePseudoConsole(self.hpc)
            k32.CloseHandle(self.in_write)
            k32.CloseHandle(self.out_read)
            raise
        finally:
            k32.DeleteProcThreadAttributeList(startup.lpAttributeList)
        k32.CloseHandle(info.hThread)
        self.process = info.hProcess
        self.pid = info.dwProcessId

    def outputs(self):
        return [("pty", self.out_read)]

    def read(self, handle) -> bytes:
        ctypes = self.ctypes
        buf = ctypes.create_string_buffer(READ_CHUNK)
        got = ctypes.c_ulong(0)
        if not self.k32.ReadFile(handle, buf, READ_CHUNK, ctypes.byref(got), None):
            return b""
        return buf.raw[: got.value]

    def write(self, data: bytes) -> None:
        ctypes = self.ctypes
        view = bytes(data)
        while view:
            wrote = ctypes.c_ulong(0)
            if not self.k32.WriteFile(self.in_write, view, len(view), ctypes.byref(wrote), None):
                raise ctypes.WinError(ctypes.get_last_error())
            view = view[wrote.value :]

    def close_input(self) -> None:
        self.write(b"\x1a\r")

    def resize(self, rows: int, cols: int) -> None:
        with self.lock:
            if not self.closed:
                self.k32.ResizePseudoConsole(self.hpc, self.COORD(cols, rows))

    def signal(self, name: str) -> None:
        if name == "INT":
            self.write(b"\x03")
        else:
            _kill_tree_windows(self.pid)

    def wait(self) -> dict:
        ctypes = self.ctypes
        self.k32.WaitForSingleObject(self.process, 0xFFFFFFFF)
        code = ctypes.c_ulong(0)
        self.k32.GetExitCodeProcess(self.process, ctypes.byref(code))
        return {"exit_code": code.value, "signal": None}

    def after_exit(self) -> None:
        # The output pipe reaches its end only once the pseudoconsole is
        # closed; the reader thread keeps draining meanwhile, which closing
        # needs on older Windows builds.
        with self.lock:
            if not self.closed:
                self.closed = True
                self.k32.ClosePseudoConsole(self.hpc)

    def close(self) -> None:
        self.after_exit()
        for handle in (self.in_write, self.out_read, self.process):
            self.k32.CloseHandle(handle)


def spawn_child(spec: dict):
    if spec["mode"] == "pipe":
        return PipeChild(spec)
    if WINDOWS:
        return ConPtyChild(spec)
    return PosixPtyChild(spec)


def parse_spawn_spec(req: dict) -> dict:
    argv = req.get("argv")
    if not isinstance(argv, list) or not argv or not all(isinstance(a, str) and a for a in argv[:1]):
        raise OpError("bad_request", "spawn needs a non-empty argv list of strings")
    if not all(isinstance(a, str) for a in argv):
        raise OpError("bad_request", "spawn needs a non-empty argv list of strings")
    cwd = req.get("cwd") or None
    if cwd is not None and not isinstance(cwd, str):
        raise OpError("bad_request", "spawn cwd must be a string")
    env = req.get("env")
    if env is not None and not (
        isinstance(env, dict) and all(isinstance(k, str) and isinstance(v, str) for k, v in env.items())
    ):
        raise OpError("bad_request", "spawn env must map strings to strings")
    mode = req.get("mode", "pipe")
    if mode not in ("pipe", "pty"):
        raise OpError("bad_request", 'spawn mode must be "pipe" or "pty"')
    rows, cols = parse_size(req, default=(24, 80))
    return {"argv": argv, "cwd": cwd, "env": env, "mode": mode, "rows": rows, "cols": cols}


def parse_size(req: dict, default=None) -> tuple[int, int]:
    rows, cols = req.get("rows"), req.get("cols")
    if rows is None and cols is None and default is not None:
        return default
    for value in (rows, cols):
        if not isinstance(value, int) or isinstance(value, bool) or not 1 <= value <= 32767:
            raise OpError("bad_request", "rows and cols must be integers from 1 to 32767")
    return rows, cols


# --- the attach connection ---------------------------------------------------


class Attachment:
    """One authenticated connection. Its writer thread sends the queued
    control messages, then the ring's records from its cursor on; a client
    that falls behind the ring is told the gap, so nothing accumulates."""

    def __init__(self, holder: "Holder", sock: socket.socket) -> None:
        self.holder = holder
        self.sock = sock
        self.control: list[dict] = []
        self.streaming = False
        self.cursor = 0
        self.closing = False
        self.drain = False
        self.dead = False
        self.thread = threading.Thread(target=self._write_loop, daemon=True)

    # Callers hold holder.cond for push, start_stream, and finish.

    def push(self, message: dict) -> None:
        self.control.append(message)
        self.holder.cond.notify_all()

    def start_stream(self, from_seq: int) -> None:
        ring = self.holder.ring
        if from_seq <= 0:
            from_seq = ring.first_seq()
        self.cursor = min(from_seq, ring.next_seq)
        self.streaming = True
        self.holder.cond.notify_all()

    def finish(self, drain: bool = False) -> None:
        """Send what is queued (with drain, the ring's remaining records
        too), then close."""
        self.closing = True
        self.drain = drain
        self.holder.cond.notify_all()
        timer = threading.Timer(5.0, self.kill)
        timer.daemon = True
        timer.start()

    def kill(self) -> None:
        self.dead = True
        try:
            self.sock.shutdown(socket.SHUT_RDWR)
        except OSError:
            pass
        try:
            self.sock.close()
        except OSError:
            pass
        with self.holder.cond:
            self.holder.cond.notify_all()

    def _write_loop(self) -> None:
        holder = self.holder
        ring = holder.ring
        while True:
            with holder.cond:
                while True:
                    if self.dead:
                        return
                    pending = self.streaming and self.cursor < ring.next_seq and (self.drain or not self.closing)
                    if self.control or pending or self.closing:
                        break
                    holder.cond.wait()
                messages = self.control
                self.control = []
                if pending:
                    first = ring.first_seq()
                    if self.cursor < first:
                        messages.append({"event": "gap", "from": self.cursor, "to": first})
                        self.cursor = first
                    records = ring.batch(self.cursor, SEND_BATCH_RECORDS, SEND_BATCH_BYTES)
                    self.cursor += len(records)
                else:
                    records = []
                last = self.closing and not (self.drain and self.streaming and self.cursor < ring.next_seq)
            try:
                payload = b"".join(encode_line(m) for m in messages)
                payload += b"".join(encode_line(r.message()) for r in records)
                if payload:
                    self.sock.sendall(payload)
            except OSError:
                self.kill()
                return
            if last:
                self.kill()
                return


def encode_line(message: dict) -> bytes:
    return json.dumps(message, separators=(",", ":")).encode("utf-8") + b"\n"


# --- the holder ---------------------------------------------------------------


class Holder:
    def __init__(
        self,
        directory: str,
        *,
        holder_id: str | None = None,
        session: dict | None = None,
        idle_exit: float = IDLE_EXIT_SECONDS,
        descriptor_check: float = DESCRIPTOR_CHECK_SECONDS,
        ring_bytes: int = RING_BYTES,
        stop_grace: float = STOP_GRACE_SECONDS,
        restart_safe: bool = True,
        script_path: str | None = None,
    ) -> None:
        self.dir = os.path.abspath(directory)
        self.id = holder_id or os.path.basename(self.dir)
        self.session = session if session is not None else {}
        self.idle_exit = idle_exit
        self.descriptor_check = descriptor_check
        self.stop_grace = stop_grace
        self.restart_safe = restart_safe
        self.token = secrets.token_hex(32)
        self.script_hash = _file_sha256(script_path or __file__)
        self.max_record = max(1, min(MAX_RECORD_BYTES, ring_bytes))
        self.ring = Ring(ring_bytes)
        self.cond = threading.Condition()
        self.attachment: Attachment | None = None
        self.child = None
        self.child_status: dict = {"state": "none"}
        self.input_queue: list = []
        self.input_pending = 0
        self.idle_since = time.monotonic()
        self.stopping = False
        self.exit_event = threading.Event()
        # Guards the log against the removal of the state directory: a line
        # written while shutdown() removes the directory would leave it behind.
        self.log_lock = threading.Lock()
        self.log_closed = False
        self.exit_code = 0
        self.listener: socket.socket | None = None
        self.port = 0

    # -- lifecycle --

    def start(self) -> None:
        """Bind the attach port, write the descriptor, and start accepting."""
        self.listener = socket.create_server(("127.0.0.1", 0))
        self.port = self.listener.getsockname()[1]
        self.write_descriptor()
        self.log(f"holder {self.id} pid {os.getpid()} listening on 127.0.0.1:{self.port}")
        threading.Thread(target=self._accept_loop, daemon=True).start()

    def run(self) -> int:
        """Serve until a stop or the idle exit; returns the exit code."""
        self.start()
        tick = max(0.02, min(1.0, self.descriptor_check, self.idle_exit / 4 if self.idle_exit > 0 else 1.0))
        next_check = time.monotonic() + self.descriptor_check
        while not self.exit_event.wait(tick):
            now = time.monotonic()
            if now >= next_check:
                next_check = now + self.descriptor_check
                self.ensure_descriptor()
            with self.cond:
                idle = (
                    self.attachment is None
                    and self.child_status["state"] != "running"
                    and now - self.idle_since >= self.idle_exit
                )
            if idle:
                self.log("idle exit: no child running and nobody attached for the idle period")
                break
        self.shutdown()
        return self.exit_code

    def shutdown(self) -> None:
        """Close the port and the attachment, and remove the state directory."""
        self.exit_event.set()
        if self.listener is not None:
            try:
                self.listener.close()
            except OSError:
                pass
        with self.cond:
            att, self.attachment = self.attachment, None
            if att is not None:
                att.finish(drain=True)
        if att is not None:
            att.thread.join(timeout=5.0)
        child = self.child
        if child is not None and self.child_status["state"] == "exited":
            child.close()
        with self.log_lock:
            self.log_closed = True
            shutil.rmtree(self.dir, ignore_errors=True)

    # -- descriptor --

    def descriptor(self) -> dict:
        doc = {
            "schema": SCHEMA_VERSION,
            "id": self.id,
            "pid": os.getpid(),
            "port": self.port,
            "token": self.token,
            "protocol": PROTOCOL_VERSION,
            "started_at": STARTED_AT,
            "script_hash": self.script_hash,
            "restart_safe": self.restart_safe,
            "session": self.session,
        }
        if START_TICKS:
            doc["start_ticks"] = START_TICKS
        return doc

    def write_descriptor(self) -> None:
        os.makedirs(self.dir, mode=0o700, exist_ok=True)
        path = os.path.join(self.dir, DESCRIPTOR_NAME)
        tmp = f"{path}.{os.getpid()}.tmp"
        try:
            os.unlink(tmp)
        except OSError:
            pass
        fd = os.open(tmp, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        with os.fdopen(fd, "w", encoding="utf-8") as f:
            json.dump(self.descriptor(), f)
            f.write("\n")
        os.replace(tmp, path)

    def ensure_descriptor(self) -> None:
        if os.path.exists(os.path.join(self.dir, DESCRIPTOR_NAME)):
            return
        try:
            self.write_descriptor()
            self.log("descriptor was missing; rewritten")
        except OSError as exc:
            self.log(f"descriptor rewrite failed: {exc}")

    def log(self, text: str) -> None:
        with self.log_lock:
            if self.log_closed:
                return
            try:
                with open(os.path.join(self.dir, LOG_NAME), "a", encoding="utf-8") as f:
                    f.write(time.strftime("%Y-%m-%dT%H:%M:%S%z") + " " + text + "\n")
            except OSError:
                pass

    # -- connections --

    def _accept_loop(self) -> None:
        while not self.exit_event.is_set():
            try:
                sock, _ = self.listener.accept()
            except OSError:
                return
            threading.Thread(target=self._serve, args=(sock,), daemon=True).start()

    def _serve(self, sock: socket.socket) -> None:
        att = None
        try:
            sock.setsockopt(socket.IPPROTO_TCP, socket.TCP_NODELAY, 1)
            sock.settimeout(AUTH_TIMEOUT_SECONDS)
            sock.sendall(encode_line({"hello": "ws-holder", "protocol": PROTOCOL_VERSION}))
            reader = sock.makefile("rb")
            att = self._authenticate(sock, reader)
            if att is None:
                return
            sock.settimeout(None)
            while True:
                line = reader.readline(MAX_LINE_BYTES + 1)
                if not line or len(line) > MAX_LINE_BYTES:
                    return
                self._handle_line(att, line)
        except (OSError, ValueError):
            return
        finally:
            if att is None:
                try:
                    sock.close()
                except OSError:
                    pass
            else:
                self._detach(att)

    def _authenticate(self, sock: socket.socket, reader) -> Attachment | None:
        line = reader.readline(65536)
        refusal = None
        req: dict = {}
        try:
            req = json.loads(line)
            if not isinstance(req, dict):
                raise ValueError
        except ValueError:
            req, refusal = {}, ("bad_request", "the first line must be a JSON attach request")
        accepted = {v for v in (PROTOCOL_VERSION, PROTOCOL_VERSION - 1) if v >= 1}
        token = req.get("token")
        spoken = req.get("protocol")
        if refusal is None and req.get("op") != "attach":
            refusal = ("bad_request", "the first operation must be attach")
        if refusal is None and not (
            isinstance(token, str) and hmac.compare_digest(token.encode("utf-8"), self.token.encode("utf-8"))
        ):
            refusal = ("token", "the attach token was refused")
        if refusal is None and (isinstance(spoken, bool) or spoken not in accepted):
            refusal = ("protocol", f"this holder speaks protocol {PROTOCOL_VERSION} and the one before it")
        if refusal is not None:
            sock.sendall(
                encode_line(
                    {
                        "reply": req.get("id"),
                        "ok": False,
                        "error": refusal[0],
                        "message": refusal[1],
                        "protocol": PROTOCOL_VERSION,
                    }
                )
            )
            return None
        att = Attachment(self, sock)
        with self.cond:
            if self.exit_event.is_set():
                return None
            old, self.attachment = self.attachment, att
            if old is not None:
                old.push({"event": "detached", "reason": "replaced"})
                old.finish()
            att.push(
                {
                    "reply": req.get("id"),
                    "ok": True,
                    "protocol": spoken,
                    "holder": {
                        "id": self.id,
                        "pid": os.getpid(),
                        "started_at": STARTED_AT,
                        "script_hash": self.script_hash,
                        "restart_safe": self.restart_safe,
                        "session": self.session,
                    },
                    "child": dict(self.child_status),
                    "first_seq": self.ring.first_seq(),
                    "next_seq": self.ring.next_seq,
                }
            )
        att.thread.start()
        return att

    def _detach(self, att: Attachment) -> None:
        att.kill()
        with self.cond:
            if self.attachment is att:
                self.attachment = None
                self.idle_since = time.monotonic()

    def _handle_line(self, att: Attachment, line: bytes) -> None:
        try:
            req = json.loads(line)
            if not isinstance(req, dict):
                raise ValueError
        except ValueError:
            req = {"op": None}
        reply = {"reply": req.get("id"), "ok": True}
        # One critical section: the reply is queued before a stop the
        # operation asked for can close the connection.
        with self.cond:
            if self.attachment is not att:
                return
            try:
                handler = self._OPS.get(req.get("op"))
                if handler is None:
                    raise OpError("unknown_op", f"unknown operation {req.get('op')!r}")
                reply.update(handler(self, att, req) or {})
            except OpError as err:
                reply = {"reply": req.get("id"), "ok": False, "error": err.code, "message": err.message}
            att.push(reply)

    # -- operations (called with self.cond held) --

    def _op_spawn(self, att: Attachment, req: dict) -> dict:
        if self.child is not None:
            raise OpError("already_spawned", "this holder already has a child")
        if self.stopping:
            raise OpError("stopping", "the holder is stopping")
        spec = parse_spawn_spec(req)
        try:
            child = spawn_child(spec)
        except OpError:
            raise
        except Exception as exc:  # OSError, a ctypes failure, a bad argument
            raise OpError("spawn_failed", f"{type(exc).__name__}: {exc}") from None
        self.child = child
        self.child_status = {"state": "running", "pid": child.pid, "mode": child.mode}
        self.log(f"spawned child pid {child.pid} ({child.mode}): {spec['argv'][0]}")
        readers = []
        for stream, source in child.outputs():
            t = threading.Thread(target=self._read_output, args=(child, stream, source), daemon=True)
            t.start()
            readers.append(t)
        threading.Thread(target=self._write_input, args=(child,), daemon=True).start()
        threading.Thread(target=self._watch_child, args=(child, readers), daemon=True).start()
        return {"child": dict(self.child_status)}

    def _running_child(self):
        if self.child is None or self.child_status["state"] != "running":
            raise OpError("no_child", "no child is running")
        return self.child

    def _op_input(self, att: Attachment, req: dict) -> None:
        self._running_child()
        try:
            data = base64.b64decode(req.get("data") or "", validate=True)
        except (ValueError, TypeError):
            raise OpError("bad_request", "input data must be base64") from None
        if self.input_pending + len(data) > MAX_PENDING_INPUT:
            raise OpError("input_overflow", "the child is not reading its input")
        if data:
            self.input_queue.append(data)
            self.input_pending += len(data)
        if req.get("eof") is True:
            self.input_queue.append(None)
        self.cond.notify_all()

    def _op_resize(self, att: Attachment, req: dict) -> None:
        child = self._running_child()
        rows, cols = parse_size(req)
        try:
            child.resize(rows, cols)
        except OSError as exc:
            raise OpError("resize_failed", str(exc)) from None

    def _op_signal(self, att: Attachment, req: dict) -> None:
        child = self._running_child()
        name = req.get("name")
        if name not in SIGNAL_NAMES:
            raise OpError("bad_request", "signal name must be one of " + ", ".join(SIGNAL_NAMES))
        try:
            child.signal(name)
        except (OSError, ValueError) as exc:
            raise OpError("signal_failed", str(exc)) from None

    def _op_stop(self, att: Attachment, req: dict) -> None:
        self.request_stop()

    def _op_status(self, att: Attachment, req: dict) -> dict:
        return {
            "child": dict(self.child_status),
            "first_seq": self.ring.first_seq(),
            "next_seq": self.ring.next_seq,
        }

    def _op_replay(self, att: Attachment, req: dict) -> dict:
        from_seq = req.get("from", 0)
        if not isinstance(from_seq, int) or isinstance(from_seq, bool):
            raise OpError("bad_request", "replay from must be an integer")
        att.start_stream(from_seq)
        return {"first_seq": self.ring.first_seq(), "next_seq": self.ring.next_seq}

    _OPS = {
        "spawn": _op_spawn,
        "input": _op_input,
        "resize": _op_resize,
        "signal": _op_signal,
        "stop": _op_stop,
        "status": _op_status,
        "replay": _op_replay,
    }

    # -- the child's threads --

    def _read_output(self, child, stream: str, source) -> None:
        splitter = LineSplitter(self.max_record) if child.mode == "pipe" else None
        while True:
            chunk = child.read(source)
            if not chunk:
                break
            if splitter is None:
                pieces = [(chunk[i : i + self.max_record], False) for i in range(0, len(chunk), self.max_record)]
            else:
                pieces = list(splitter.feed(chunk))
            self._append_output(stream, pieces)
        if splitter is not None:
            self._append_output(stream, list(splitter.flush()))

    def _append_output(self, stream: str, pieces) -> None:
        if not pieces:
            return
        with self.cond:
            for data, partial in pieces:
                self.ring.append("output", stream, data, partial)
            self.cond.notify_all()

    def _write_input(self, child) -> None:
        while True:
            with self.cond:
                while not self.input_queue:
                    if self.child_status["state"] != "running":
                        return
                    self.cond.wait()
                data = self.input_queue.pop(0)
            try:
                if data is None:
                    child.close_input()
                else:
                    child.write(data)
            except (OSError, ValueError):
                pass
            if data is not None:
                with self.cond:
                    self.input_pending -= len(data)

    def _watch_child(self, child, readers) -> None:
        fields = child.wait()
        child.after_exit()
        # The output still in the pipes comes before the exit record; a
        # grandchild holding a pipe open does not delay it past this wait.
        deadline = time.monotonic() + 2.0
        for t in readers:
            t.join(timeout=max(0.0, deadline - time.monotonic()))
        with self.cond:
            self.child_status = {"state": "exited", "pid": child.pid, "mode": child.mode, **fields}
            self.ring.append("exit", fields=fields)
            self.idle_since = time.monotonic()
            self.input_queue.clear()
            self.input_pending = 0
            self.cond.notify_all()
        self.log(f"child pid {child.pid} exited: {fields}")

    # -- stop --

    def request_stop(self) -> None:
        """End the child and exit; safe from any thread, and idempotent."""
        with self.cond:
            if self.stopping:
                return
            self.stopping = True
        threading.Thread(target=self._stop, daemon=True).start()

    def _stop(self) -> None:
        self.log("stop requested")
        with self.cond:
            child = self.child if self.child_status["state"] == "running" else None
        if child is not None:
            # An interactive shell ignores TERM; a hangup is what ends it.
            first = "HUP" if child.mode == "pty" and not WINDOWS else "TERM"
            for name, wait in ((first, self.stop_grace), ("KILL", 5.0)):
                try:
                    child.signal(name)
                except (OSError, ValueError, OpError):
                    pass
                with self.cond:
                    if self.cond.wait_for(lambda: self.child_status["state"] != "running", timeout=wait):
                        break
        self.exit_event.set()


def _file_sha256(path: str) -> str:
    try:
        with open(path, "rb") as f:
            return hashlib.sha256(f.read()).hexdigest()
    except OSError:
        return ""


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="ws session holder")
    parser.add_argument("--dir", required=True, help="this holder's state directory")
    parser.add_argument("--id", default=None, help="holder id; defaults to the directory name")
    parser.add_argument("--session", default="{}", help="session descriptor, a JSON object")
    parser.add_argument("--idle-exit-seconds", type=float, default=IDLE_EXIT_SECONDS)
    parser.add_argument("--descriptor-check-seconds", type=float, default=DESCRIPTOR_CHECK_SECONDS)
    parser.add_argument("--ring-bytes", type=int, default=RING_BYTES)
    parser.add_argument("--stop-grace-seconds", type=float, default=STOP_GRACE_SECONDS)
    parser.add_argument("--restart-unsafe", action="store_true", help="record that this holder may end with its starter")
    args = parser.parse_args(argv)
    try:
        session = json.loads(args.session)
        if not isinstance(session, dict):
            raise ValueError("not an object")
    except ValueError as exc:
        parser.error(f"--session: {exc}")
    holder = Holder(
        args.dir,
        holder_id=args.id,
        session=session,
        idle_exit=args.idle_exit_seconds,
        descriptor_check=args.descriptor_check_seconds,
        ring_bytes=args.ring_bytes,
        stop_grace=args.stop_grace_seconds,
        restart_safe=not args.restart_unsafe,
    )
    for name in ("SIGTERM", "SIGINT", "SIGHUP"):
        sig = getattr(signal, name, None)
        if sig is not None:
            try:
                signal.signal(sig, lambda *_: holder.request_stop())
            except (OSError, ValueError):
                pass
    try:
        code = holder.run()
    except Exception as exc:
        holder.log(f"fatal: {type(exc).__name__}: {exc}")
        raise
    if holder.stopping and not WINDOWS and os.getpgrp() == os.getpid():
        # Started detached, the holder leads its own process group, which a
        # pipe child and its descendants share: nothing of it outlives a stop.
        signal.signal(signal.SIGTERM, signal.SIG_IGN)
        try:
            os.killpg(0, signal.SIGTERM)
            time.sleep(0.2)
            os.killpg(0, signal.SIGKILL)
        except OSError:
            pass
    return code


if __name__ == "__main__":
    sys.exit(main())
