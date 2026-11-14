#!/Library/Developer/CommandLineTools/Library/Frameworks/Python3.framework/Versions/3.9/bin/python3.9
"""Static source artifact. MAIN has not authorized execution.

Reviewed invocation, only after a new scoped START:
  /usr/bin/env -i HOME=/var/empty PATH=/usr/bin:/bin TMPDIR=/tmp LANG=C LC_ALL=C /Library/Developer/CommandLineTools/Library/Frameworks/Python3.framework/Versions/3.9/bin/python3.9 -I -S -B /Users/kang-sw/.ws-worktrees/devenv/safari-manicotti-litter/ai-docs/.plans/2026-11/14-2341-261114-live-summary-bounded-supervisor.py --run-reviewed-batch

Filesystem lookup only selected this existing interpreter; it has not run.
The parent and its sealed same-source anchor are the only Python processes.
/bin/ps is a bounded, separately retained direct diagnostic child. Metadata Git
children read branch/HEAD/literal blobs, never working-tree diff filters. Input
inventory/hashes and branch/HEAD checks are preparation, not extra tests.

Inheritance evidence: selected wslive historical exec.CommandContext calls;
mcp's initGit/ordinary ExecRunner Git calls (no SysProcAttr); fakeHolders and
preparePiLeadLaunch replacement in wsagent/lead_fake_test.go; in-process daemon
fixtures in wsdashboard/index_test.go and pages_test.go. Installed Go 1.26.2
src/cmd/go/internal contains no SysProcAttr/Setpgid/Setsid override. Python 3.9's
subprocess.py defaults start_new_session=False and retains strongly referenced
Popen instances outside its orphan _active cleanup list. Only this parent's
anchor launch selects start_new_session=True. No arbitrary workload is admitted.

Darwin/arm64, this one worktree, these 21 non-detached fixture tests only.
The parent retains its direct session-leader child without poll/wait/reaping
until group cleanup and its absence receipt finish. That reserved PID anchors
PGID ownership even when process-list diagnostics fail or the anchor exits.
This is not a supervisor for arbitrary, hostile, or detached workloads.
"""

import hashlib
import json
import os
from pathlib import Path
import select
import signal
import subprocess
import sys
import time
import uuid

PYTHON = "/Library/Developer/CommandLineTools/Library/Frameworks/Python3.framework/Versions/3.9/bin/python3.9"
GO_ROOT = "/opt/homebrew/Cellar/go/1.26.2/libexec"
GO = GO_ROOT + "/bin/go"
GOFMT = GO_ROOT + "/bin/gofmt"
GIT = "/Library/Developer/CommandLineTools/usr/bin/git"
PS = "/bin/ps"
ROOT = Path("/Users/kang-sw/.ws-worktrees/devenv/safari-manicotti-litter")
BRANCH = "impl/feat/live-reasoning-summary-675/fruit-annex-hunk"
SOURCE = "5d018785c8caddc237cf24bb5229c871aac8783b"
BATCH = Path("/tmp/ws-live-summary-675-receiver-01")
BOOT_ENV = {"HOME": "/var/empty", "PATH": "/usr/bin:/bin", "TMPDIR": "/tmp", "LANG": "C", "LC_ALL": "C"}
WORK_SECONDS = 630
CLEANUP_SECONDS = 90
FORMAT = [
    "internal/mcp/intake_order.go", "internal/mcp/intake_order_test.go",
    "internal/mcp/live_snapshot.go", "internal/mcp/server.go",
    "internal/mcp/summary_intake_test.go", "internal/mcp/summary_owner.go",
    "internal/wsagent/backend.go", "internal/wsagent/pi.go",
    "internal/wsagent/record.go", "internal/wsagent/runner.go",
    "internal/wsagent/runner_summary.go", "internal/wsagent/summary_hosted_test.go",
    "internal/wsagent/wake.go", "internal/wsdashboard/live_hosted.go",
    "internal/wsdashboard/live_hosted_test.go", "internal/wslive/reasoning_summary.go",
    "internal/wslive/reasoning_summary_test.go", "internal/wslive/ring.go",
    "internal/wslive/snapshot.go", "internal/wslive/state.go",
    "internal/wslive/summary_commit.go", "internal/wslive/summary_commit_test.go",
    "internal/wslive/summary_compat_test.go",
]
TESTS = [
    ("wslive", 240, "210s", [
        "TestReasoningSummaryValidation", "TestReasoningSummaryFreshnessAndRealEnd",
        "TestReasoningSummaryLeadOnlyAndAttemptPreservation",
        "TestSummaryCommitFreezesRetriesAndNeverRecounts",
        "TestSummaryCommitCapacityNeverEvictsAcceptedCandidate",
        "TestSummaryImmutableCommitRefusesOverwrite",
        "TestSummaryHistoricalReaderWriterPreservesOriginalBytes",
        "TestSummaryPagingPruningAndNoResurrection",
    ]),
    ("mcp", 150, "120s", [
        "TestSummaryIngressTracksAbandonedLegacyAndActualDeferredWrite",
        "TestSummaryIngressFailsClosedWithinAdmissionBudget",
        "TestSummaryIngressFIFOAndUnrelatedToolsRemainIndependent",
        "TestSummaryQuiescenceWaitsForRunningSnapshotCallback",
        "TestSummaryIntakeNegotiatesOnlyOwnedDaemonReceiver",
        "TestSummaryIntakeFreezesRealEndBeforeAcceptanceAndRetriesOnlyCandidate",
        "TestSummaryIntakeRetiredConnectionAndHostedTakeoverCannotWrite",
    ]),
    ("wsagent", 150, "120s", [
        "TestSummaryHostedWidgetIsProjectionNotLifecycleAuthority",
        "TestSummaryHostedFinalPrecedesNativeEndAndQuietReplayNeverWrites",
        "TestSummaryHostedNativeAbortReasonIsActualFact",
    ]),
    ("wsdashboard", 60, "40s", [
        "TestHostedSlotRendersLiveOnly", "TestHostedSlotOwnerFencesActualGeneration",
        "TestHostedSlotTakeover",
    ]),
]
COMMANDS = [("format", 30, str(ROOT), [GOFMT, "-w"] + [
    "agents-plugin-tool/" + p for p in FORMAT
])]
for package, ceiling, timeout, names in TESTS:
    COMMANDS.append((package, ceiling, str(ROOT / "agents-plugin-tool"), [
        GO, "test", "./internal/" + package, "-run", "^(" + "|".join(names) + ")$",
        "-count=1", "-vet=off", "-p=1", "-parallel=1", "-timeout=" + timeout,
    ]))
INPUT_PREFIXES = ["agents-plugin-tool", "agents-plugin/rsrc", "agents-plugin/config-manifest.json",
                  "agents-plugin/dashboard-config-manifest.json", "agents-plugin-pi/package.json"]
GIT_META = [GIT, "-c", "core.fsmonitor=false", "-c", "core.hooksPath=/dev/null"]
META = {
    "branch": GIT_META + ["symbolic-ref", "--quiet", "--short", "HEAD"],
    "head": GIT_META + ["rev-parse", "--verify", "HEAD"],
    # Read literal committed blobs; do not invoke working-tree diff filters,
    # textconv, hooks, fsmonitor, credential helpers, or installed launchers.
    "tree": GIT_META + ["ls-tree", "-r", "-z", SOURCE, "--"] + INPUT_PREFIXES,
}


class Block(Exception):
    pass


class Ledger:
    def __init__(self, path):
        self.fd = os.open(str(path), os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_APPEND, 0o600)
        self.failed = False

    def event(self, kind, **fields):
        raw = (json.dumps(dict(kind=kind, monotonic=time.monotonic(), **fields),
                          ensure_ascii=True, sort_keys=True) + "\n").encode("ascii")
        try:
            view = memoryview(raw)
            while view:
                written = os.write(self.fd, view)
                if written <= 0:
                    raise OSError("receipt write made no progress")
                view = view[written:]
            os.fsync(self.fd)
        except OSError:
            self.failed = True
            raise

    def close(self):
        os.close(self.fd)


def environment():
    # Construct from empty: no parent credentials, launcher variables, dynamic
    # loader options, stand-in switches, Python path, or installed harness PATH.
    env = {
        "PATH": str(BATCH / "bin"), "HOME": str(BATCH / "home"),
        "TMPDIR": str(BATCH / "tmp"), "TMP": str(BATCH / "tmp"), "TEMP": str(BATCH / "tmp"),
        "LANG": "C", "LC_ALL": "C", "GOROOT": GO_ROOT,
        "GOCACHE": str(BATCH / "gocache"), "GOMODCACHE": "/Users/kang-sw/go/pkg/mod",
        "GOOS": "darwin", "GOARCH": "arm64", "GOMAXPROCS": "2", "GOTOOLCHAIN": "local",
        "GOENV": "off", "GOWORK": "off", "CGO_ENABLED": "0", "GOPROXY": "off",
        "GOSUMDB": "off", "GOVCS": "*:off", "GOTELEMETRY": "off",
        "GOFLAGS": "-mod=readonly -buildvcs=false",
        "GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": "/dev/null", "GIT_CONFIG_SYSTEM": "/dev/null",
        "GIT_NO_REPLACE_OBJECTS": "1", "GIT_ATTR_NOSYSTEM": "1", "GIT_TERMINAL_PROMPT": "0",
        "GIT_ASKPASS": "/usr/bin/false", "SSH_ASKPASS": "/usr/bin/false",
        "GIT_SSH_COMMAND": "/usr/bin/false", "WS_CACHE_HOME": str(BATCH / "cache"),
        "WS_CONFIG_HOME": str(BATCH / "config"), "WS_RSRC_ROOT": str(ROOT / "agents-plugin/rsrc"),
    }
    return env


def send(stream, record):
    raw = (json.dumps(record, ensure_ascii=True) + "\n").encode("ascii")
    if len(raw) > 4096:
        raise Block("oversized supervisor control record")
    stream.write(raw)
    stream.flush()


def anchor(nonce, parent):
    # Popen(start_new_session=True) already established this private session.
    # Keep this object/process alive between commands; spawn no detached child.
    if dict(os.environ) != environment():
        return 2
    signal.signal(signal.SIGCHLD, signal.SIG_DFL)
    pid = os.getpid()
    if os.getppid() != parent or os.getpgid(0) != pid or os.getsid(0) != pid:
        return 2
    ledger = Ledger(BATCH / "logs/anchor.jsonl")
    ledger.event("anchor_ready", nonce=nonce, pid=pid, ppid=parent, pgid=pid, sid=pid,
                 python=PYTHON, start_monotonic=time.monotonic())
    send(sys.stdout.buffer, dict(kind="ready", nonce=nonce, pid=pid, ppid=parent, pgid=pid, sid=pid))
    serial = 0
    retained = None
    try:
        for raw in sys.stdin.buffer:
            if len(raw) > 4096:
                raise Block("oversized control input")
            request = json.loads(raw)
            if request.get("nonce") != nonce:
                raise Block("control nonce mismatch")
            if request.get("op") == "close":
                ledger.event("anchor_closing", nonce=nonce)
                return 0
            if retained is not None:
                raise Block("a timed-out command remains owned")
            if request.get("op") == "meta" and request.get("name") in META:
                label, argv, cwd = request["name"], META[request["name"]], str(ROOT)
            elif request.get("op") == "command" and type(request.get("index")) is int and 0 <= request["index"] < len(COMMANDS):
                label, _, cwd, argv = COMMANDS[request["index"]]
            else:
                raise Block("unlisted operation")
            # Python 3.9/macOS monotonic reference points need not match
            # between processes. Only relative duration crosses this pipe;
            # the parent's independent absolute deadline still owns cleanup.
            local_deadline = time.monotonic() + request["timeout_seconds"]
            serial += 1
            outpath = BATCH / "logs" / ("%03d-%s.stdout" % (serial, label))
            errpath = BATCH / "logs" / ("%03d-%s.stderr" % (serial, label))
            with open(outpath, "xb", buffering=0) as out, open(errpath, "xb", buffering=0) as err:
                retained = subprocess.Popen(argv, cwd=cwd, env=environment(), stdin=subprocess.DEVNULL,
                                            stdout=out, stderr=err, close_fds=True, start_new_session=False)
                # The direct child handle remains retained until wait() below.
                # Its children inherit this anchor's session/group by audited
                # default os/exec/Popen paths in the selected fixtures/toolchain.
                env_hash = hashlib.sha256(json.dumps(environment(), sort_keys=True).encode()).hexdigest()
                ledger.event("command_launch", nonce=nonce, serial=serial, label=label, argv=argv, cwd=cwd,
                             environment_sha256=env_hash, environment_keys=sorted(environment()),
                             pid=retained.pid, ppid=pid, pgid=pid, sid=pid,
                             start_monotonic=time.monotonic(), stdout=str(outpath), stderr=str(errpath))
                send(sys.stdout.buffer, dict(kind="started", pid=retained.pid, label=label,
                                            stdout=str(outpath), stderr=str(errpath)))
                try:
                    result = retained.wait(timeout=max(0.001, local_deadline - time.monotonic()))
                except subprocess.TimeoutExpired:
                    ledger.event("command_timeout", pid=retained.pid, label=label)
                    send(sys.stdout.buffer, dict(kind="timeout", pid=retained.pid, label=label))
                    # No local retry or guessed PID signal. The parent still
                    # owns this entire session through its unreaped anchor.
                    continue
                ledger.event("command_reaped", pid=retained.pid, label=label, returncode=result)
                retained = None
                send(sys.stdout.buffer, dict(kind="done", label=label, returncode=result,
                                            stdout=str(outpath), stderr=str(errpath)))
    finally:
        ledger.close()
    return 2


class Supervisor:
    def __init__(self):
        self.started = time.monotonic()
        self.work_end = self.started + WORK_SECONDS
        self.total_end = self.work_end + CLEANUP_SECONDS
        self.nonce = uuid.uuid4().hex
        self.anchor = None
        self.anchor_reaped = False
        self.anchor_ready = False
        self.anchor_start = None
        self.anchor_created = None
        self.diag_failed = False
        self.receipt_failed = False
        self.signal_failed = False
        self.anchor_state = ""
        self.helpers = []
        self.script = Path(__file__).read_bytes()
        self.detached = []
        self.seen = {}
        self.buffer = b""
        self.ledger = None
        self.completed = []
        self.cleaning = False

    def note(self, kind, **fields):
        try:
            self.ledger.event(kind, **fields)
        except BaseException:
            self.receipt_failed = True
            raise

    def cleanup_note(self, kind, **fields):
        try:
            self.note(kind, **fields)
        except BaseException:
            self.receipt_failed = True

    def prepare(self):
        if sys.platform != "darwin" or os.path.realpath(sys.executable) != os.path.realpath(PYTHON):
            raise Block("unreviewed platform/interpreter")
        for path in [PYTHON, GO, GOFMT, GIT, PS]:
            if not os.path.isfile(path) or not os.access(path, os.X_OK):
                raise Block("missing reviewed executable: " + path)
        os.mkdir(BATCH, 0o700)  # Existing roots are never reused or repaired.
        for name in ["bin", "home", "tmp", "gocache", "config", "cache", "logs"]:
            os.mkdir(BATCH / name, 0o700)
        os.symlink(GO, BATCH / "bin/go")
        os.symlink(GIT, BATCH / "bin/git")
        self.ledger = Ledger(BATCH / "logs/supervisor.jsonl")
        self.note("batch_start", nonce=self.nonce, source=SOURCE, branch=BRANCH,
                  supervisor_pid=os.getpid(), supervisor_ppid=os.getppid(),
                  supervisor_pgid=os.getpgid(0), supervisor_sid=os.getsid(0),
                  work_deadline=self.work_end, total_deadline=self.total_end,
                  environment=environment(), script_sha256=hashlib.sha256(self.script).hexdigest())
        sealed = BATCH / "supervisor.py"
        with open(sealed, "xb") as source:
            source.write(self.script)
        sealed.chmod(0o400)
        signal.signal(signal.SIGCHLD, signal.SIG_DFL)
        creating = time.monotonic()
        self.anchor = subprocess.Popen([PYTHON, "-I", "-S", "-B", str(sealed),
                                        "--anchor", self.nonce, str(os.getpid())],
                                       env=environment(), cwd=str(ROOT), stdin=subprocess.PIPE,
                                       stdout=subprocess.PIPE, stderr=open(BATCH / "logs/anchor.stderr", "xb"),
                                       close_fds=True, start_new_session=True)
        self.anchor_created = [creating, time.monotonic()]
        # Never poll(), wait(), communicate(), context-manage, or discard this
        # Popen before cleanup: the unreaped direct PID reserves PGID identity.
        self.note("anchor_launch", nonce=self.nonce, pid=self.anchor.pid, ppid=os.getpid(),
                  pgid=self.anchor.pid, sid=self.anchor.pid, retained_unreaped=True,
                  creation_interval_parent_monotonic=self.anchor_created,
                  start_monotonic=time.monotonic())
        ready_deadline = min(self.work_end, time.monotonic() + 5)
        ready = None
        while ready is None:
            ready = self.read_message(ready_deadline)
        if ready != dict(kind="ready", nonce=self.nonce, pid=self.anchor.pid, ppid=os.getpid(),
                         pgid=self.anchor.pid, sid=self.anchor.pid):
            raise Block("anchor handshake mismatch")
        self.anchor_ready = True
        self.snapshot(require_empty=True)

    def read_message(self, deadline):
        while b"\n" not in self.buffer:
            left = deadline - time.monotonic()
            if left <= 0:
                raise Block("control deadline")
            ready, _, _ = select.select([self.anchor.stdout], [], [], min(left, 0.2))
            if not ready:
                return None
            data = os.read(self.anchor.stdout.fileno(), 4096)
            if not data:
                raise Block("anchor control stream closed")
            self.buffer += data
            if len(self.buffer) > 8192:
                raise Block("oversized anchor reply")
        raw, self.buffer = self.buffer.split(b"\n", 1)
        return json.loads(raw)

    def process_table(self):
        if self.diag_failed:
            raise Block("diagnostics already failed; no diagnostic retry")
        # This is a separately retained direct metadata child. It never joins
        # the fixture group, and only its own unreaped handle may be signaled.
        diagnostic_deadline = self.total_end if self.cleaning else self.work_end
        if time.monotonic() >= diagnostic_deadline:
            raise Block("diagnostic budget exhausted")
        child = subprocess.Popen([PS, "-axo", "pid=,ppid=,pgid=,lstart=,stat=,comm="],
                                 env=environment(), stdin=subprocess.DEVNULL,
                                 stdout=subprocess.PIPE, stderr=subprocess.PIPE, close_fds=True)
        self.helpers.append(child)
        try:
            self.note("diagnostic_launch", pid=child.pid, ppid=os.getpid(),
                      pgid=os.getpgid(0), sid=os.getsid(0),
                      identity_basis="retained direct child; default group/session inheritance",
                      start_monotonic=time.monotonic(), argv=[PS, "-axo", "pid=,ppid=,pgid=,lstart=,stat=,comm="])
            out, err = child.communicate(timeout=max(0.001, min(2, diagnostic_deadline - time.monotonic())))
            if child.returncode != 0:
                raise Block("ps diagnostic failed: " + err.decode("utf-8", "replace")[:300])
        except BaseException:
            self.diag_failed = True
            # communicate may reap on normal completion; if it has not, this
            # exact retained direct child is still ours, not a guessed PID.
            if child.returncode is None:
                self.cleanup_note("diagnostic_signal_intent", pid=child.pid, signal=int(signal.SIGKILL),
                                  retained_unreaped=True, method="Popen.kill on exact owned direct handle")
                child.kill()
                self.cleanup_note("diagnostic_signal_requested", pid=child.pid, signal=int(signal.SIGKILL))
                child.wait(timeout=max(0.001, min(2, self.total_end - time.monotonic())))
            raise
        finally:
            self.cleanup_note("diagnostic_reaped", pid=child.pid, returncode=child.returncode)
        rows = {}
        try:
            for line in out.decode("utf-8", "strict").splitlines():
                fields = line.split(None, 9)
                if len(fields) != 10:
                    raise Block("unparseable process table")
                pid, ppid, pgid = map(int, fields[:3])
                rows[pid] = dict(pid=pid, ppid=ppid, pgid=pgid, start=" ".join(fields[3:8]),
                                 state=fields[8], executable=fields[9])
        except BaseException:
            self.diag_failed = True
            raise
        return rows

    def expected_executable(self, row):
        exe = row["executable"]
        base = os.path.basename(exe)
        if row["pid"] == self.anchor.pid:
            return os.path.realpath(exe) == os.path.realpath(PYTHON)
        if base in {"go", "gofmt", "git", "asm", "compile", "link", "preprofile"}:
            return True
        return base in {"wslive.test", "mcp.test", "wsagent.test", "wsdashboard.test"}

    def snapshot(self, require_empty=False):
        rows = self.process_table()
        group = {pid: row for pid, row in rows.items() if row["pgid"] == self.anchor.pid}
        # Follow current ancestry and previously observed identities. An
        # observed descendant outside the private group is not adopted/killed.
        family = {self.anchor.pid}
        changed = True
        while changed:
            added = {pid for pid, row in rows.items() if row["ppid"] in family}
            changed = not added.issubset(family)
            family.update(added)
        for pid, prior in self.seen.items():
            if pid in rows and rows[pid]["start"] == prior["start"]:
                family.add(pid)
        family.update(group)
        owned = [rows[pid] for pid in sorted(family) if pid in rows]
        self.note("process_snapshot", anchor_pid=self.anchor.pid, rows=owned)
        leader = rows.get(self.anchor.pid)
        if leader is None or leader["ppid"] != os.getpid() or leader["pgid"] != self.anchor.pid:
            raise Block("retained anchor identity not visible")
        self.anchor_state = leader["state"]
        if self.anchor_start is None:
            self.anchor_start = leader["start"]
            self.note("anchor_start_identity", pid=self.anchor.pid, pgid=self.anchor.pid,
                      start=self.anchor_start, retained_unreaped=True)
        elif leader["start"] != self.anchor_start:
            raise Block("anchor start identity changed")
        for row in owned:
            if row["pgid"] != self.anchor.pid:
                self.detached.append(row)
                raise Block("unknown detached lineage")
            self.seen[row["pid"]] = row
            if "Z" not in row["state"] and not self.expected_executable(row):
                raise Block("unlisted owned descendant executable")
        descendants = [row for row in group.values() if row["pid"] != self.anchor.pid]
        if require_empty:
            self.note("positive_absence", anchor_pid=self.anchor.pid, anchor_start=self.anchor_start,
                      descendants=descendants, absent=not descendants, retained_unreaped=True)
            if descendants:
                raise Block("owned descendants remain after command completion")
        return descendants

    def operation(self, request, ceiling):
        deadline = min(self.work_end, time.monotonic() + ceiling)
        if deadline <= time.monotonic():
            raise Block("work budget exhausted")
        self.note("operation_admitted", request=request, deadline=deadline)
        send(self.anchor.stdin, dict(request, nonce=self.nonce,
                                     timeout_seconds=max(0.001, deadline - time.monotonic())))
        last_snapshot = 0.0
        while True:
            message = self.read_message(deadline)
            now = time.monotonic()
            if now >= deadline:
                raise Block("operation ceiling reached")
            if now - last_snapshot >= 0.5:
                self.snapshot()
                last_snapshot = now
            if message is None:
                continue
            self.note("anchor_message", record=message)
            if message.get("kind") not in {"started", "done", "timeout"}:
                raise Block("unexpected anchor reply")
            if message.get("kind") == "timeout":
                raise Block("command deadline: " + message["label"])
            if message.get("kind") == "done":
                self.snapshot(require_empty=True)
                if message["returncode"] != 0:
                    raise Block("command failure: " + message["label"])
                return message

    def meta(self, name):
        result = self.operation(dict(op="meta", name=name), 5)
        raw = Path(result["stdout"]).read_bytes()
        if len(raw) > 16 * 1024 * 1024:
            raise Block("metadata output exceeds reviewed bound")
        return raw

    def manifest(self, paths):
        digest = hashlib.sha256()
        for raw in paths:
            relative = raw.decode("utf-8", "strict")
            path = ROOT / relative
            resolved = path.resolve(strict=True)
            if ROOT.resolve() not in resolved.parents or not path.is_file() or path.is_symlink():
                raise Block("input leaves reviewed source tree: " + relative)
            digest.update(raw + b"\0" + hashlib.sha256(path.read_bytes()).digest())
        return digest.hexdigest()

    def source_inputs(self):
        entries = {}
        for record in filter(None, self.meta("tree").split(b"\0")):
            header, path = record.split(b"\t", 1)
            mode, kind, oid = header.split()
            if mode not in {b"100644", b"100755"} or kind != b"blob" or len(oid) != 40:
                raise Block("unreviewed source object type")
            entries[path] = oid
        paths = sorted(entries)
        if not paths:
            raise Block("empty committed input inventory")
        self.check_input_inventory(paths)
        for raw in paths:
            data = (ROOT / raw.decode("utf-8", "strict")).read_bytes()
            oid = hashlib.sha1(b"blob " + str(len(data)).encode() + b"\0" + data).hexdigest().encode()
            if oid != entries[raw]:
                raise Block("source differs from reviewed receiver binding")
        return paths

    def check_input_inventory(self, paths):
        actual = set()
        for prefix in INPUT_PREFIXES:
            base = ROOT / prefix
            if base.is_file():
                actual.add(prefix.encode())
                continue
            for directory, subdirs, files in os.walk(base, followlinks=False):
                if time.monotonic() >= self.work_end:
                    raise Block("input inventory exceeded work deadline")
                if any((Path(directory) / name).is_symlink() for name in subdirs + files):
                    raise Block("unreviewed input symlink")
                for name in files:
                    actual.add(str((Path(directory) / name).relative_to(ROOT)).encode())
        if actual != set(paths):
            raise Block("untracked, ignored, missing, or unexpected build inputs")

    def run(self):
        self.prepare()
        if self.meta("branch").decode().strip() != BRANCH:
            raise Block("branch identity changed")
        head = self.meta("head").strip()
        paths = self.source_inputs()
        before = {raw: hashlib.sha256((ROOT / raw.decode()).read_bytes()).digest() for raw in paths}
        self.note("input_manifest", phase="before_format", head=head.decode(), sha256=self.manifest(paths))
        self.operation(dict(op="command", index=0), COMMANDS[0][1])
        allowed = {("agents-plugin-tool/" + p).encode() for p in FORMAT}
        for raw in paths:
            if raw not in allowed and hashlib.sha256((ROOT / raw.decode()).read_bytes()).digest() != before[raw]:
                raise Block("formatting changed an unlisted input")
        baseline = self.manifest(paths)
        self.note("input_manifest", phase="after_format", sha256=baseline)
        for index in range(1, len(COMMANDS)):
            self.check_input_inventory(paths)
            if (self.meta("branch").decode().strip() != BRANCH or self.meta("head").strip() != head
                    or self.manifest(paths) != baseline):
                raise Block("branch/source changed during batch")
            self.operation(dict(op="command", index=index), COMMANDS[index][1])
            if self.manifest(paths) != baseline:
                raise Block("test changed source inputs")
            self.completed.append(COMMANDS[index][0])

    def group_signal(self, sig):
        if self.anchor is None or self.anchor_reaped:
            raise Block("group signal without retained unreaped anchor")
        # This proof is independent of ps success: this exact direct child was
        # created with setsid, remains strongly referenced, and has NEVER been
        # polled/reaped. Its PID cannot be reassigned to an unrelated group.
        receipt = dict(anchor_pid=self.anchor.pid, pgid=self.anchor.pid, start=self.anchor_start,
                       creation_interval_parent_monotonic=self.anchor_created,
                       anchor_handshake=self.anchor_ready,
                       identity_basis="retained unreaped direct child created with start_new_session=True",
                       signal=int(sig), retained_unreaped=True, diagnostic_failed=self.diag_failed)
        self.cleanup_note("signal_intent", **receipt)
        try:
            os.killpg(self.anchor.pid, sig)
        except ProcessLookupError:
            self.cleanup_note("group_absent_at_signal", **receipt)
            return
        except OSError as error:
            self.signal_failed = True
            self.cleanup_note("signal_error", error=str(error), **receipt)
            return
        self.cleanup_note("signal_sent", **receipt)

    def cleanup(self, failed):
        if self.anchor is None:
            return False
        # The cleanup proof never depends on a fresh diagnostic succeeding.
        # A missing receipt still blocks the result, but never drops ownership.
        self.cleaning = True
        deadline = min(self.total_end, time.monotonic() + CLEANUP_SECONDS)
        absent = False
        cleanup_error = None
        try:
            # A diagnostic write/read failure cannot orphan its separately
            # retained direct helper or prevent the owned group cleanup.
            for helper in self.helpers:
                if helper.returncode is None:
                    try:
                        self.cleanup_note("helper_kill_intent", pid=helper.pid, signal=int(signal.SIGKILL),
                                          retained_unreaped=True, method="Popen.kill on exact owned direct handle")
                        helper.kill()
                        helper.wait(timeout=max(0.001, min(2, deadline - time.monotonic())))
                        self.cleanup_note("helper_reaped", pid=helper.pid, returncode=helper.returncode)
                    except BaseException as error:
                        cleanup_error = "diagnostic helper: " + str(error)
            if not failed and not self.diag_failed and not self.detached:
                try:
                    self.snapshot(require_empty=True)
                    send(self.anchor.stdin, dict(op="close", nonce=self.nonce))
                    self.anchor.stdin.close()
                except BaseException as error:
                    cleanup_error = str(error)
                    self.diag_failed = True
                    failed = True
                    self.group_signal(signal.SIGTERM)
            else:
                self.group_signal(signal.SIGTERM)
            # A normal close is allowed 10 seconds, TERM another 10, KILL up
            # to the remaining cleanup ceiling. Never signal after reaping.
            term_at = time.monotonic() + (0 if failed else 10)
            kill_at = term_at + 10
            term_sent = failed
            kill_sent = False
            while time.monotonic() < deadline - 5:
                now = time.monotonic()
                if not self.diag_failed:
                    try:
                        descendants = self.snapshot()
                        if not descendants and "Z" in self.anchor_state:
                            self.cleanup_note("positive_absence", anchor_pid=self.anchor.pid,
                                              anchor_start=self.anchor_start, descendants=[], absent=True,
                                              retained_unreaped=True, phase="cleanup")
                            absent = True
                            break
                    except BaseException as error:
                        cleanup_error = str(error)
                        self.diag_failed = True
                if not term_sent and now >= term_at:
                    self.group_signal(signal.SIGTERM)
                    term_sent = True
                if not kill_sent and now >= kill_at:
                    self.group_signal(signal.SIGKILL)
                    kill_sent = True
                    if self.diag_failed:
                        break  # No diagnostic retry or invented absence receipt.
                time.sleep(min(0.2, max(0.001, deadline - 5 - time.monotonic())))
            if not absent and not kill_sent:
                self.group_signal(signal.SIGKILL)
            # All group signals precede this sole wait/reap of the anchor.
            code = self.anchor.wait(timeout=max(0.001, min(5, deadline - time.monotonic())))
            self.anchor_reaped = True
            self.anchor.stdin.close()
            self.anchor.stdout.close()
            self.cleanup_note("anchor_reaped", pid=self.anchor.pid, returncode=code,
                              control_pipes_closed=True, positive_descendant_absence=absent,
                              detached=self.detached,
                              listener_disposition="selected listeners were in reaped fixture processes",
                              holder_disposition="selected holders were in-process fakes, not OS children")
        except BaseException as error:
            cleanup_error = str(error)
            # Ownership remains reserved if wait did not complete. No guessed
            # PID/group fallback, broad name signal, restart, or cleanup repair.
        finally:
            if self.ledger is not None:
                try:
                    self.cleanup_note("cleanup_receipt", anchor_pid=self.anchor.pid,
                              anchor_reaped=self.anchor_reaped, positive_absence=absent,
                              diagnostic_failed=self.diag_failed, detached=self.detached,
                              error=cleanup_error, retained_root=str(BATCH),
                              total_elapsed=time.monotonic() - self.started)
                except OSError:
                    pass
        return (absent and self.anchor_reaped and not self.diag_failed and not self.detached
                and not self.receipt_failed and not self.signal_failed and not cleanup_error)


def main():
    if len(sys.argv) == 4 and sys.argv[1] == "--anchor":
        return anchor(sys.argv[2], int(sys.argv[3]))
    if sys.argv[1:] != ["--run-reviewed-batch"]:
        print("BLOCK: static source artifact; a new MAIN scoped START is required.", file=sys.stderr)
        return 2
    if dict(os.environ) != BOOT_ENV:
        print("BLOCK: bootstrap environment differs from the explicit empty-derived allowlist.", file=sys.stderr)
        return 2
    supervisor = Supervisor()
    failure = None
    def interrupted(signum, _frame):
        raise Block("supervisor interrupted by signal %d" % signum)
    signal.signal(signal.SIGTERM, interrupted)
    signal.signal(signal.SIGINT, interrupted)
    try:
        supervisor.run()
    except BaseException as error:
        failure = str(error)
    finally:
        # An interrupt requests cleanup; another one does not discard the
        # reserved ownership handle while bounded cleanup is in progress.
        signal.signal(signal.SIGTERM, lambda _sig, _frame: None)
        signal.signal(signal.SIGINT, lambda _sig, _frame: None)
        clean = supervisor.cleanup(failure is not None)
    result = dict(status="BLOCK" if failure or not clean else "completed_pending_full_log_read",
                  failure=failure, completed=supervisor.completed, cleanup_proven=clean,
                  retained_root=str(BATCH), receipts=str(BATCH / "logs"),
                  omitted="Pi/full suites/review/visual captures and every shared daemon/provider action")
    try:
        if supervisor.ledger is not None:
            supervisor.note("batch_result", **result)
            supervisor.ledger.close()
    finally:
        print(json.dumps(result, sort_keys=True))
    return 2 if result["status"] == "BLOCK" else 0


if __name__ == "__main__":
    sys.exit(main())
