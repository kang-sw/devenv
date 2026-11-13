#!/usr/bin/env python3
"""Repository-local developer commands. Not shipped with any plugin package.

    python scripts/dev.py rerun daemon [--local] [--no-seed]

`rerun daemon` builds agents-plugin-tool/cmd/ws-mcp from the working tree
(cross-built on the sys_lock remote w5090 by default, `go build` here with
--local), stages it under the dashboard daemon's bin/ directory exactly as the launcher
does, and runs `daemon-start --build-time <now>` so the daemon's own
newer-build-wins rule drains and replaces the running daemon. Session
launchers reconnect on their own.

It then seeds the same build into each local-devenv package dir of this checkout
(a valid .local-devenv-runtime marker) with the launcher's own runtime name and
compatibility stamp, so the next session start does not rebuild the same source
locally. --no-seed skips that.

The daemon is started with WS_RSRC_ROOT/WS_SKILLS_ROOT pointing at this
checkout's agents-plugin-wsflow package, as its launcher would set them: the
daemon resolves the Python launcher it hands to Pi workers as
<dir(WS_RSRC_ROOT)>/bin/ws-mcp-launcher.py, and the staged exe has no package
beside it.
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

# Package dirs of this checkout whose launcher runtime gets seeded.
SEED_PACKAGES = ("agents-plugin", "agents-plugin-wsflow")

# Package dir whose rsrc/ and skills/ the daemon is started against, as the
# launcher of that package would; its bin/ws-mcp-launcher.py is what Pi
# workers run.
DAEMON_PACKAGE = REPO / "agents-plugin-wsflow"

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
    identity = launcher.source_runtime_metadata(TOOL_DIR)
    proc = subprocess.run(
        [go, "build", "-o", str(out), "./cmd/ws-mcp"],
        cwd=str(TOOL_DIR),
        env=launcher.local_devenv_build_env(launcher.host_os()),
        check=False,
    )
    if proc.returncode != 0:
        raise RuntimeError(f"go build exited {proc.returncode}")
    launcher.write_runtime_metadata(out, identity)
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
        f"CGO_ENABLED=0 GOOS={goos} GOARCH={goarch} go build -o dist-x/{name} ./cmd/ws-mcp && "
        f"python3 scripts/write-runtime-metadata.py dist-x/{name}"
    )
    argv = [
        shutil.which("bash") or "bash", SYS_LOCK, "remote", REMOTE, "run", "dev-rerun-daemon", str(REMOTE_BUILD_MINUTES),
        # --sync wants the repo root as a Windows-style path; a subdirectory
        # fails with "sync source missing".
        "--sync", REPO.as_posix(),
        "--fetch", f"{REMOTE_DIST}/{name}={scratch.as_posix()}",
        "--fetch", f"{REMOTE_DIST}/{name}.metadata.json={scratch.as_posix()}",
        "--", remote_cmd,
    ]
    print(f"building on {REMOTE} for {goos}/{goarch}")
    proc = subprocess.run(argv, check=False)
    if proc.returncode != 0:
        raise RuntimeError(f"remote build on {REMOTE} exited {proc.returncode}")
    out = scratch / name
    if not out.is_file() or launcher.read_runtime_metadata(out) is None:
        raise RuntimeError(f"remote build succeeded but {out} and its valid metadata were not fetched")
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
    identity = launcher.read_runtime_metadata(built)
    if identity is None:
        raise RuntimeError(f"built binary has no valid hash-bound metadata: {built}")
    print(f"built version: {identity['version']}")
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


def run_daemon_start(launcher, staged: Path, build_time: int, plugin_dir: Path = DAEMON_PACKAGE) -> dict:
    env = dict(os.environ)
    # Set, not defaulted: a shell inside a harness session can inherit a
    # versioned plugin-cache path, and this command means this checkout.
    env["WS_RSRC_ROOT"] = str(plugin_dir / "rsrc")
    env["WS_SKILLS_ROOT"] = str(plugin_dir / "skills")
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


def seed_package(launcher, plugin_dir: Path, built: Path) -> str:
    """Seed the built exe as plugin_dir's launcher runtime; return the report line."""
    os_name = launcher.host_os()
    if launcher.read_local_devenv_contract(plugin_dir, os_name) is None:
        return "not local-devenv"
    contract_path = plugin_dir / "runtime.json"
    contract = launcher.read_runtime_contract(contract_path)
    platform_name = f"{os_name}-{launcher.host_arch()}"
    runtime_dir = Path(os.environ.get("WS_MCP_RUNTIME_DIR", str(plugin_dir / ".runtime" / platform_name)))
    binary = runtime_dir / launcher.runtime_binary_name(contract, contract_path, os_name)
    identity = launcher.read_runtime_metadata(built)
    if identity is None:
        return "skipped (build metadata missing or hash mismatch)"
    if not launcher.runtime_capabilities_compatible(built, contract, allow_same_minor=True):
        return "skipped (build identity incompatible with package)"
    fingerprint = launcher.local_devenv_source_fingerprint(plugin_dir, os_name)
    runtime_dir.mkdir(parents=True, exist_ok=True)
    if not (binary.is_file() and launcher.sha256_file(binary) == launcher.sha256_file(built)):
        tmp = launcher.unique_runtime_temp_path(runtime_dir, f"{binary.name}.seed")
        try:
            shutil.copyfile(built, tmp)
            # A running launcher's exe is locked on Windows and the launcher has
            # no way around that either (it retries, then keeps the old one), so
            # a locked target is skipped rather than worked around.
            os.replace(tmp, binary)
        except OSError as exc:
            tmp.unlink(missing_ok=True)
            return f"skipped ({binary.name} not replaceable, likely in use: {exc})"
    launcher.write_runtime_metadata(binary, identity)
    launcher.write_compatibility_stamp(binary, contract, contract_path, runtime_dir, fingerprint)
    if not launcher.compatibility_stamp_current(binary, contract, contract_path, runtime_dir, fingerprint):
        return "skipped (compatibility stamp not written)"
    return f"seeded {binary}"


def seed_packages(launcher, built: Path) -> None:
    for name in SEED_PACKAGES:
        try:
            result = seed_package(launcher, REPO / name, built)
        except (OSError, RuntimeError, SystemExit) as exc:
            result = f"skipped ({exc})"
        print(f"seed {name}: {result}")


def rerun_daemon(local: bool, seed: bool = True) -> int:
    launcher = load_launcher()
    env = dict(os.environ)
    daemon_dir = launcher.proxy_daemon_dir(env)
    bin_dir = daemon_dir / launcher.DAEMON_BIN_DIR_NAME

    with tempfile.TemporaryDirectory(prefix="ws-mcp-dev-") as scratch:
        built = build(launcher, Path(scratch), local)
        check_built(launcher, built)
        if seed:
            seed_packages(launcher, built)
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
    daemon.add_argument("--no-seed", action="store_true", help="do not seed the build into the local-devenv plugin runtime dirs")
    args = parser.parse_args(argv)
    try:
        if args.command == "rerun" and args.target == "daemon":
            return rerun_daemon(args.local, not args.no_seed)
    except (RuntimeError, OSError, subprocess.SubprocessError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1
    return 2


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
