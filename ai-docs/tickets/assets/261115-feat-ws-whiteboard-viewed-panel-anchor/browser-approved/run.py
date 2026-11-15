"""ONE approved batch. Static-authored; parent inspection/start is required.

python3 .../browser-approved/run.py --result <full frozen commit> --batch <new name>
No checkout/ref mutation, retries, repairs, installs, or prior-artifact deletion.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import signal
import subprocess
import sys
import tarfile
import time

BASELINE = "5585844a794a031a770ce4171c5e05daf1bd3353"
PLAYWRIGHT = "/Users/kang-sw/.npm/_npx/705bc6b22212b352/node_modules/playwright"
HERE = Path(__file__).resolve().parent
ROOT = next(p for p in HERE.parents if (p / "agents-plugin-tool").is_dir())
HELPER = "agents-plugin-tool/internal/wsdashboard/board_viewed_browser_test.go"
DRIVER = "agents-plugin-tool/internal/wsdashboard/testdata/viewed-panels.browser.cjs"


def digest(path):
    h = hashlib.sha256()
    with path.open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            h.update(block)
    return h.hexdigest()


def save(path, data):
    tmp = path.with_name(path.name + ".tmp")
    tmp.write_text(json.dumps(data, indent=2) + "\n")
    tmp.replace(path)


def process_rows():
    # lstart distinguishes recycled PIDs; only this identity is ever signaled.
    text = subprocess.check_output(
        ["ps", "-axo", "pid=,ppid=,lstart=,command="], text=True, timeout=2)
    rows = []
    for line in text.splitlines():
        p = line.strip().split(None, 7)
        if len(p) == 8:
            rows.append(dict(pid=int(p[0]), ppid=int(p[1]),
                             started=" ".join(p[2:7]), command=p[7][:800]))
    return rows


class Batch:
    def __init__(self, out, result):
        self.out, self.result = out, result
        self.started = time.monotonic()
        self.deadline = self.started + 600
        self.work_deadline = self.deadline - 30
        self.owned, self.children, self.stops = {}, [], []
        self.record = dict(baseline=BASELINE, result=result, overall_cap_seconds=600,
                           build_cap_seconds=180, browser_cap_seconds=180,
                           cleanup_reserve_seconds=30, steps=[], completed=False)
        # Explicit offline environment, not the caller's provider/dev-asset config.
        self.env = {k: os.environ[k] for k in
                    ["PATH", "HOME", "TMPDIR", "USER", "LOGNAME", "LANG", "SystemRoot"]
                    if k in os.environ}
        self.env.update(GOTOOLCHAIN="local", GOPROXY="off", GOSUMDB="off",
                        GOWORK="off", GOENV="off", WS_PLAYWRIGHT_MODULE=PLAYWRIGHT)
        save(out / "manifest.json", self.record)

    def checkpoint(self):
        self.record["elapsed_seconds"] = round(time.monotonic() - self.started, 3)
        save(self.out / "manifest.json", self.record)

    def track(self):
        rows = process_rows()
        live_file = self.out / "browser-owned-live.json"
        if live_file.exists():
            # Node atomically publishes Chromium's launchServer root and children.
            for row in json.loads(live_file.read_text()).get("tracked", []):
                self.owned[(row["pid"], row["started"])] = row
        known = {r["pid"] for r in rows if (r["pid"], r["started"]) in self.owned}
        known.update(child.pid for child, _ in self.children if child.poll() is None)
        for _ in range(16):
            for row in rows:
                if row["ppid"] in known:
                    known.add(row["pid"])
        for row in rows:
            if row["pid"] in known:
                self.owned[(row["pid"], row["started"])] = row
        save(self.out / "owned-live.json", dict(tracked=list(self.owned.values())))
        return rows

    def start(self, label, command, cwd, extra=None):
        if time.monotonic() >= self.work_deadline:
            raise TimeoutError("overall budget exhausted before " + label)
        if any(server.poll() is not None for server in getattr(self, "servers", [])):
            raise RuntimeError("fixture exited before " + label)
        log = (self.out / (label + ".txt")).open("wb")
        env = dict(self.env)
        if extra:
            env.update(extra)
        child = subprocess.Popen(command, cwd=cwd, env=env, stdout=log,
                                 stderr=subprocess.STDOUT, start_new_session=True)
        self.children.append((child, log))
        step = dict(label=label, command=[str(s) for s in command], cwd=str(cwd),
                    environment=env, pid=child.pid, started_elapsed=time.monotonic() - self.started)
        self.record["steps"].append(step)
        self.track()
        self.checkpoint()
        return child, step

    def wait(self, child, step, cap):
        until = min(self.work_deadline, time.monotonic() + cap)
        while child.poll() is None:
            self.track()
            # A fixture dying during build/browser is a first failure, too.
            for server in getattr(self, "servers", []):
                if server is not child and server.poll() is not None:
                    raise RuntimeError("fixture exited early: " + str(server.pid))
            if time.monotonic() >= until:
                raise TimeoutError(step["label"] + " cap reached")
            time.sleep(.1)
        self.track()
        step["returncode"] = child.returncode
        step["ended_elapsed"] = time.monotonic() - self.started
        self.checkpoint()
        if child.returncode:
            raise RuntimeError(step["label"] + " failed; retained raw output")

    def command(self, label, command, cwd, cap=20):
        child, step = self.start(label, command, cwd)
        self.wait(child, step, cap)

    def snapshot(self, label, pin):
        archive = self.out / (label + ".tar")
        self.command(label + "-archive", ["git", "archive", "--format=tar",
                     "--output=" + str(archive), pin, "agents-plugin-tool"], ROOT)
        snapshot = self.out / (label + "-source")
        snapshot.mkdir()
        # No ref/worktree operation. Reject archive paths or symlinks escaping it.
        with tarfile.open(archive) as source:
            for member in source.getmembers():
                dest = (snapshot / member.name).resolve()
                if not dest.is_relative_to(snapshot):
                    raise RuntimeError("unsafe archive path")
                if member.issym() and not (dest.parent / member.linkname).resolve().is_relative_to(snapshot):
                    raise RuntimeError("unsafe archive symlink")
                if not (member.isfile() or member.isdir() or member.issym()):
                    raise RuntimeError("unexpected non-source archive member")
            # Members/links were validated above; avoid requiring Python's
            # newer extraction-filter argument on an otherwise stdlib runner.
            source.extractall(snapshot)
        # Fixture and driver come from the frozen RESULT commit, not a live file.
        # Reading them by git show is logged, pinned and performed just once.
        shutil.copyfile(self.out / "fixture.go", snapshot / HELPER)
        shutil.copyfile(self.out / "driver.cjs", snapshot / DRIVER)
        files = []
        for file in sorted(snapshot.rglob("*")):
            if file.is_file():
                rel = file.relative_to(snapshot).as_posix()
                files.append(dict(path=rel, sha256=digest(file), size=file.stat().st_size,
                                  asset_or_template=("/assets/" in rel or file.suffix in (".tmpl", ".html", ".css", ".js"))))
        # Retain an immutable archive of EXACT build inputs, including overlay.
        inputs = self.out / (label + "-build-inputs.tar")
        with tarfile.open(inputs, "w") as target:
            target.add(snapshot / "agents-plugin-tool", arcname="agents-plugin-tool")
        for file in snapshot.rglob("*"):
            if file.is_file() and not file.is_symlink():
                file.chmod(file.stat().st_mode & ~0o222)
        save(self.out / (label + "-sources.json"), dict(source_pin=pin, files=files,
             git_archive_sha256=digest(archive), build_inputs_sha256=digest(inputs),
             fixture_sha256=digest(self.out / "fixture.go"), driver_sha256=digest(self.out / "driver.cjs")))
        self.record[label + "_sources_manifest_sha256"] = digest(self.out / (label + "-sources.json"))
        for artifact in [archive, inputs, self.out / (label + "-sources.json")]:
            artifact.chmod(0o444)
        self.checkpoint()
        return snapshot

    def ready(self, label, pin, binary):
        ready, stop = self.out / (label + "-ready.json"), self.out / (label + ".stop")
        self.stops.append(stop)
        child, step = self.start(label + "-fixture", [str(binary), "-test.run=^TestBoardViewedFixture$",
                                  "-test.timeout=245s", "-test.v"], binary.parent,
                                 dict(WS_VIEWED_READY=str(ready), WS_VIEWED_STOP=str(stop), WS_VIEWED_SOURCE_PIN=pin))
        self.servers.append(child)
        until = min(self.work_deadline, time.monotonic() + 15)
        while not ready.exists():
            self.track()
            if any(server.poll() is not None for server in self.servers):
                raise RuntimeError(label + " fixture pair failed before ready")
            if time.monotonic() >= until:
                raise TimeoutError(label + " ready cap")
            time.sleep(.05)
        info = json.loads(ready.read_text())
        if info["source_pin"] != pin or info["pid"] != child.pid or not info["asset_id"]:
            raise RuntimeError("ready source/PID/asset identity mismatch")
        self.record[label + "_ready"] = info
        self.checkpoint()
        return ready

    def cleanup(self):
        # This phase reserves the last 30s of the single 600s budget.
        errors = []
        for stop in self.stops:
            try:
                stop.touch()
            except OSError as exc:
                errors.append(str(exc))
        actions = []
        for sig, seconds in [(None, 3), (signal.SIGTERM, 4), (signal.SIGKILL, 3)]:
            try:
                rows = self.track()
                live = [r for r in rows if (r["pid"], r["started"]) in self.owned]
                if sig:
                    for row in live:
                        try:
                            os.kill(row["pid"], sig)
                            actions.append(dict(pid=row["pid"], started=row["started"], signal=sig.name))
                        except ProcessLookupError:
                            pass
                until = min(self.deadline - 3, time.monotonic() + seconds)
                while time.monotonic() < until:
                    for child, _ in self.children:
                        child.poll()  # Reap direct children; exit alone is not cleanup proof.
                    rows = self.track()
                    if not any((r["pid"], r["started"]) in self.owned for r in rows):
                        break
                    time.sleep(.1)
            except Exception as exc:
                errors.append(str(exc))
        for child, log in self.children:
            child.poll()
            for step in self.record["steps"]:
                if step["pid"] == child.pid:
                    step["final_returncode"] = child.returncode
            if child in getattr(self, "servers", []) and child.returncode != 0:
                errors.append("fixture final exit: " + str(child.pid) + " = " + str(child.returncode))
            log.close()
        observed = True
        try:
            final = process_rows()  # independent fresh ps, not a parent-exit inference
        except Exception as exc:
            final, observed = [], False
            errors.append("final ps unavailable: " + str(exc))
        survivors = [r for r in final if (r["pid"], r["started"]) in self.owned]
        save(self.out / "cleanup.json", dict(tracked=list(self.owned.values()),
             actions=actions, errors=errors, final_ps=survivors if observed else None,
             elapsed_seconds=round(time.monotonic() - self.started, 3)))
        if survivors or errors:
            self.record["cleanup_failure"] = dict(survivors=survivors, errors=errors)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--result", required=True)
    parser.add_argument("--batch", required=True)
    gate = parser.add_mutually_exclusive_group(required=True)
    gate.add_argument("--preflight", action="store_true",
                      help="compile both binaries and vet both fixture packages; no servers/browser")
    gate.add_argument("--preflight-receipt",
                      help="green compile/vet manifest for these exact frozen inputs/toolchain")
    args = parser.parse_args()
    if not re.fullmatch(r"[0-9a-f]{40}", args.result):
        parser.error("--result must be the parent's frozen FULL commit hash")
    if not re.fullmatch(r"[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}", args.batch):
        parser.error("--batch must be a new simple artifact-directory name")
    out = HERE / args.batch
    out.mkdir()  # Refuse reuse. Never erase or overwrite an earlier batch.
    batch = Batch(out, args.result)
    batch.servers = []
    batch.record["mode"] = "compile-vet-preflight" if args.preflight else "browser-batch"
    def interrupted(signum, _frame):
        raise InterruptedError("outer signal " + str(signum))
    signal.signal(signal.SIGINT, interrupted)
    signal.signal(signal.SIGTERM, interrupted)
    signal.signal(signal.SIGALRM, interrupted)
    # Include Python extraction/hashing and startup, not just subprocess waits.
    signal.setitimer(signal.ITIMER_REAL, max(1, batch.work_deadline - time.monotonic()))
    try:
        # Resolve the previously used toolchain from retained receipts, never
        # from a new version probe, PATH selection, installation or download.
        authority = HERE / "approved-001" / "cleanup.json"
        receipt = json.loads(authority.read_text())
        compiler = next(Path(row["command"].split(" ", 1)[0])
                        for row in receipt["tracked"]
                        if "/go/1.26.2/libexec/pkg/tool/darwin_arm64/compile " in row["command"])
        goroot = compiler.parents[3]
        go = goroot / "bin" / "go"
        batch.record["toolchain"] = dict(authority=str(authority), authority_sha256=digest(authority),
            goroot=str(goroot), go=str(go), go_sha256=digest(go),
            compile_sha256=digest(compiler), vet_sha256=digest(compiler.with_name("vet")))
        batch.env["GOROOT"] = str(goroot)
        if not (Path(PLAYWRIGHT) / "package.json").is_file():
            raise RuntimeError("approved cached Playwright module absent; do not install")
        batch.record["cached_playwright_package_sha256"] = digest(Path(PLAYWRIGHT) / "package.json")
        for name, source in [("fixture.go", HELPER), ("driver.cjs", DRIVER),
                             ("runner.py", Path(__file__).relative_to(ROOT).as_posix())]:
            child, step = batch.start("frozen-" + name, ["git", "show", args.result + ":" + source], ROOT)
            batch.wait(child, step, 10)
            # Raw stdout is exactly the immutable fixture source, retained.
            shutil.copyfile(out / ("frozen-" + name + ".txt"), out / name)
        if digest(Path(__file__)) != digest(out / "runner.py"):
            raise RuntimeError("runner differs from frozen result commit")
        batch.record["runner_sha256"] = digest(out / "runner.py")
        batch.record["fixture_sha256"] = digest(out / "fixture.go")
        batch.record["driver_sha256"] = digest(out / "driver.cjs")
        for name in ["fixture.go", "driver.cjs", "runner.py"]:
            (out / name).chmod(0o444)
        if not args.preflight:
            prior = json.loads(Path(args.preflight_receipt).read_text())
            fields = ["baseline", "result", "fixture_sha256", "driver_sha256", "runner_sha256", "toolchain"]
            if prior.get("mode") != "compile-vet-preflight" or not prior.get("completed") or prior.get("failure") or prior.get("cleanup_failure") or any(prior.get(k) != batch.record.get(k) for k in fields):
                raise RuntimeError("green compile/vet receipt does not match frozen inputs/toolchain")
            batch.record["preflight_receipt"] = str(Path(args.preflight_receipt).resolve())
            batch.record["preflight_receipt_sha256"] = digest(Path(args.preflight_receipt))
        ready_files = []
        binaries = []
        snapshots = []
        for label, pin in [("baseline", BASELINE), ("result", args.result)]:
            snapshot = batch.snapshot(label, pin)
            binary = out / (label + ".test")
            batch.command(label + "-build", [str(go), "test", "-c", "./internal/wsdashboard", "-o", str(binary)],
                          snapshot / "agents-plugin-tool", 180)
            source_manifest = json.loads((out / (label + "-sources.json")).read_text())
            if any(digest(snapshot / file["path"]) != file["sha256"] for file in source_manifest["files"]):
                raise RuntimeError(label + " source snapshot mutated during build")
            batch.record[label + "_binary_sha256"] = digest(binary)
            binary.chmod(0o555)
            binaries.append((label, pin, binary))
            snapshots.append((label, snapshot))
            batch.checkpoint()
        if args.preflight:
            for label, snapshot in snapshots:
                batch.command(label + "-vet", [str(go), "vet", "./internal/wsdashboard"],
                              snapshot / "agents-plugin-tool", 90)
        else:
            for label, pin, binary in binaries:
                ready_files.append(batch.ready(label, pin, binary))
            batch.command("browser", ["node", str(out / "driver.cjs"),
                          str(ready_files[0]), str(ready_files[1]), str(out)], out, 180)
        batch.record["completed"] = True
    except BaseException as exc:
        batch.record["failure"] = dict(type=type(exc).__name__, message=str(exc))
        # No progression, retry, repair or second browser execution after failure.
    finally:
        signal.setitimer(signal.ITIMER_REAL, 0)
        batch.checkpoint()
        try:
            batch.cleanup()
        except BaseException as exc:
            batch.record["cleanup_failure"] = dict(type=type(exc).__name__, message=str(exc))
        batch.checkpoint()
    print(json.dumps(dict(artifacts=str(out), completed=batch.record["completed"],
                         failure=batch.record.get("failure"), cleanup_failure=batch.record.get("cleanup_failure"))))
    return 0 if batch.record["completed"] and not batch.record.get("cleanup_failure") else 1


if __name__ == "__main__":
    sys.exit(main())
