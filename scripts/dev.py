#!/usr/bin/env python3
"""Repository-local developer commands. Not shipped with any plugin package.

    python scripts/dev.py rerun daemon [--local]

`rerun daemon` builds agents-plugin-tool/cmd/ws-mcp from the working tree
(cross-built on the sys_lock remote w5090 by default, `go build` here with
--local), stages it under the dashboard daemon's bin/ directory exactly as the launcher
does, and runs `daemon-start --build-time <now>` so the daemon's own
newer-build-wins rule drains and replaces the running daemon. Session
launchers reconnect on their own.
"""
import argparse
import importlib.util
import json
import os
import shutil
import subprocess
import sys
import tempfile
import time
import urllib.request
from pathlib import Path

REPO = Path(__file__).resolve().parent.parent
TOOL_DIR = REPO / "agents-plugin-tool"
LAUNCHER = REPO / "agents-plugin" / "bin" / "ws-mcp-launcher.py"
RUNTIME_CONTRACT = REPO / "agents-plugin" / "runtime.json"

# The machine-wide lock front end, and the named remote worker builds go to.
SYS_LOCK = "D:/sys_lock.sh"
REMOTE = "w5090"
REMOTE_BUILD_MINUTES = 15
# Repo-relative directory the remote build writes into; remote only, never
# fetched wholesale.
REMOTE_DIST = "agents-plugin-tool/dist-x"

# How long to wait for a daemon to leave, and for daemon-start to finish. The
# launcher gives a replacement DefaultStartWait plus a margin; a drain adds up
# to its own 10s.
GONE_WAIT_SECONDS = 30.0
START_TIMEOUT_SECONDS = 60.0


def load_launcher():
    """Import the launcher module for its staging and cache-root helpers.

    Its main() is guarded by __name__, so importing runs no launch. Bytecode
    is not written: that would drop a __pycache__ into the shipped bin/.
    """
    sys.dont_write_bytecode = True
    spec = importlib.util.spec_from_file_location("ws_mcp_launcher", LAUNCHER)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"cannot load {LAUNCHER}")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def binary_name(launcher) -> str:
    return "ws-mcp.exe" if launcher.host_os() == "windows" else "ws-mcp"


def build_local(launcher, scratch: Path) -> Path:
    go = shutil.which("go")
    if not go:
        raise RuntimeError("go is not on PATH")
    out = scratch / binary_name(launcher)
    print(f"building locally: {TOOL_DIR / 'cmd' / 'ws-mcp'}")
    proc = subprocess.run(
        [go, "build", "-o", str(out), "./cmd/ws-mcp"],
        cwd=str(TOOL_DIR),
        env=launcher.local_devenv_build_env(launcher.host_os()),
        check=False,
    )
    if proc.returncode != 0:
        raise RuntimeError(f"go build exited {proc.returncode}")
    return out


def remote_available() -> str | None:
    """None when the sys_lock remote can build; otherwise why it cannot."""
    if os.name != "nt":
        return "sys_lock remotes are driven from the Windows host only"
    bash = shutil.which("bash")
    if not bash or not Path(SYS_LOCK).is_file():
        return f"{SYS_LOCK} or bash is not present"
    try:
        proc = subprocess.run([bash, SYS_LOCK, "remote", "list"], capture_output=True, text=True, timeout=60, check=False)
    except (OSError, subprocess.SubprocessError) as exc:
        return f"sys_lock remote list failed: {exc}"
    if proc.returncode != 0 or REMOTE not in proc.stdout.split():
        return f"remote {REMOTE} is not listed by sys_lock"
    return None


def build_remote(launcher, scratch: Path) -> Path:
    """Cross-build for this host on the sys_lock remote and fetch the binary back.

    The remote is Linux, so GOOS/GOARCH are this host's. The flags match the
    launcher's own source build (no ldflags): the daemon then reports the
    same version string a launcher-built daemon does, "0.49.0-dev".
    """
    name = binary_name(launcher)
    goos, goarch = launcher.host_os(), launcher.host_arch()
    remote_cmd = (
        "export PATH=$HOME/sdk/go/bin:$PATH; "
        f"cd agents-plugin-tool && mkdir -p dist-x && "
        f"CGO_ENABLED=0 GOOS={goos} GOARCH={goarch} go build -o dist-x/{name} ./cmd/ws-mcp"
    )
    argv = [
        shutil.which("bash") or "bash", SYS_LOCK, "remote", REMOTE, "run", "dev-rerun-daemon", str(REMOTE_BUILD_MINUTES),
        # --sync wants the repo root as a Windows-style path; a subdirectory
        # fails with "sync source missing".
        "--sync", REPO.as_posix(),
        "--fetch", f"{REMOTE_DIST}/{name}={scratch.as_posix()}",
        "--", remote_cmd,
    ]
    print(f"building on {REMOTE} for {goos}/{goarch}")
    proc = subprocess.run(argv, check=False)
    if proc.returncode != 0:
        raise RuntimeError(f"remote build on {REMOTE} exited {proc.returncode}")
    out = scratch / name
    if not out.is_file():
        raise RuntimeError(f"remote build succeeded but {out} was not fetched")
    return out


def build(launcher, scratch: Path, local: bool) -> Path:
    if not local:
        reason = remote_available()
        if reason is None:
            return build_remote(launcher, scratch)
        print(f"notice: building locally; {reason}")
    return build_local(launcher, scratch)


def check_built(launcher, built: Path) -> None:
    """Print the build's version and whether the launcher's gate would accept it.

    A failed gate is a warning, not an error: the daemon does not apply it,
    but a launcher would refuse the same binary as a runtime.
    """
    proc = subprocess.run([str(built), "version"], capture_output=True, text=True, timeout=30, check=False)
    if proc.returncode != 0:
        raise RuntimeError(f"the built binary's `version` exited {proc.returncode}: {proc.stderr.strip()}")
    print(f"built version: {proc.stdout.strip()}")
    try:
        contract = launcher.read_runtime_contract(RUNTIME_CONTRACT)
        accepted = launcher.runtime_capabilities_compatible(built, contract, allow_same_minor=True)
    except (Exception, SystemExit) as exc:
        print(f"warning: launcher compatibility gate could not be evaluated: {exc}", file=sys.stderr)
        return
    print("launcher compatibility gate: " + ("accepted" if accepted else "REFUSED (launchers would reject this build as a runtime)"))


def describe(health: dict | None) -> str:
    if health is None:
        return "none running"
    return (
        f"version={health.get('version')} pid={health.get('pid')} "
        f"build_time={health.get('build_time')} content_hash={str(health.get('content_hash', ''))[:12]}"
    )


def current_health(launcher, daemon_dir: Path) -> tuple[dict | None, dict | None]:
    endpoint = launcher.read_daemon_endpoint(daemon_dir)
    if endpoint is None:
        return None, None
    try:
        return endpoint, launcher.daemon_health(int(endpoint["port"]), 3.0)
    except launcher.DaemonHung as exc:
        raise RuntimeError(f"the running daemon is wedged: {exc}") from exc


def shutdown_and_wait(launcher, daemon_dir: Path, endpoint: dict, old_pid) -> None:
    """Ask the running daemon to drain and exit through /shutdown, then wait for it to go.

    Used only when daemon-start would keep a daemon whose content equals the
    new build, which it never replaces by itself.
    """
    req = urllib.request.Request(
        f"http://127.0.0.1:{int(endpoint['port'])}/shutdown",
        method="POST",
        headers={"Authorization": f"Bearer {endpoint['token']}"},
    )
    with urllib.request.urlopen(req, timeout=10) as resp:
        if resp.status != 202:
            raise RuntimeError(f"/shutdown answered {resp.status}")
    deadline = time.monotonic() + GONE_WAIT_SECONDS
    while time.monotonic() < deadline:
        _, health = current_health(launcher, daemon_dir)
        if health is None or health.get("pid") != old_pid:
            return
        time.sleep(0.3)
    raise RuntimeError(f"daemon pid {old_pid} did not exit within {GONE_WAIT_SECONDS:.0f}s of /shutdown")


def run_daemon_start(launcher, staged: Path, build_time: int) -> dict:
    env = dict(os.environ)
    # The daemon starts session holders with the interpreter that started it.
    if not env.get("WS_MCP_HOLDER_PYTHON") and sys.executable:
        env["WS_MCP_HOLDER_PYTHON"] = sys.executable
    argv = [str(staged), "daemon-start", "--build-time", str(build_time)]
    print("running: " + " ".join(argv))
    proc = subprocess.run(
        argv,
        env=env,
        stdin=subprocess.DEVNULL,
        capture_output=True,
        text=True,
        timeout=START_TIMEOUT_SECONDS,
        check=False,
    )
    for line in proc.stderr.splitlines():
        print(f"daemon-start: {line}")
    if proc.returncode != 0:
        raise RuntimeError(f"daemon-start exited {proc.returncode}: {proc.stdout.strip()}")
    print(f"daemon-start: {proc.stdout.strip()}")
    try:
        return json.loads(proc.stdout.strip().splitlines()[-1])
    except (ValueError, IndexError) as exc:
        raise RuntimeError(f"daemon-start printed no report line: {proc.stdout!r}") from exc


def rerun_daemon(local: bool) -> int:
    launcher = load_launcher()
    env = dict(os.environ)
    daemon_dir = launcher.proxy_daemon_dir(env)
    bin_dir = daemon_dir / launcher.DAEMON_BIN_DIR_NAME

    with tempfile.TemporaryDirectory(prefix="ws-mcp-dev-") as scratch:
        built = build(launcher, Path(scratch), local)
        check_built(launcher, built)
        # The build output is only the staging source: the daemon runs from
        # the content-hash-named copy, so the scratch file is never held open.
        digest = launcher.sha256_file(built)
        staged = launcher.stage_daemon_binary(built, bin_dir)
    print(f"staged: {staged}")

    endpoint, before = current_health(launcher, daemon_dir)
    print(f"daemon before: {describe(before)}")
    if before is not None and before.get("content_hash") == digest:
        # The same content is the same build to the newer-build-wins rule;
        # a rerun still means replace, so drain it through /shutdown.
        print("running daemon has identical content; replacing it through /shutdown")
        shutdown_and_wait(launcher, daemon_dir, endpoint, before.get("pid"))

    report = run_daemon_start(launcher, staged, int(time.time() * 1000))
    _, after = current_health(launcher, daemon_dir)
    print(f"daemon after:  {describe(after)}")
    if after is None:
        print("error: no daemon answers /health after daemon-start", file=sys.stderr)
        return 1
    if after.get("content_hash") != digest:
        print(
            f"error: the running daemon is not the new build (outcome {report.get('outcome')}); "
            "a newer-version daemon keeps running over an older source build",
            file=sys.stderr,
        )
        return 1
    return 0


def main(argv: list[str]) -> int:
    parser = argparse.ArgumentParser(prog="dev", description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = parser.add_subparsers(dest="command", required=True)
    rerun = sub.add_parser("rerun", help="replace a running component with the working-tree build")
    rerun_sub = rerun.add_subparsers(dest="target", required=True)
    daemon = rerun_sub.add_parser("daemon", help="replace the dashboard daemon")
    daemon.add_argument("--local", action="store_true", help=f"build with the local go instead of on the {REMOTE} sys_lock remote")
    args = parser.parse_args(argv)
    try:
        if args.command == "rerun" and args.target == "daemon":
            return rerun_daemon(args.local)
    except (RuntimeError, OSError, subprocess.SubprocessError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1
    return 2


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
