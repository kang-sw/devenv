#!/usr/bin/env python3
"""One explicitly allocated cold-build attempt, with a shared 25-minute budget."""

import argparse
import json
import os
from pathlib import Path
import signal
import subprocess
import sys
import tempfile
import time


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--allocated", action="store_true", required=True,
                        help="Run only after the coordinating lead grants the actual slot")
    parser.parse_args()
    root = Path(__file__).resolve().parents[1]
    log_dir = Path(tempfile.mkdtemp(prefix="werkschaft-build-slot-"))
    started = time.monotonic()
    # 1485 seconds shared by test + app build; reserve 15 for owned-group cleanup.
    work_deadline = started + 1485
    final_deadline = started + 1500
    env = os.environ.copy()
    env.update({
        "PATH": str(Path.home() / ".cargo/bin") + ":" + env.get("PATH", ""),
        "RUSTUP_TOOLCHAIN": "stable-aarch64-apple-darwin",
        "RUSTUP_AUTO_INSTALL": "0",
        "CARGO_TARGET_DIR": str(root / "target"),
        "CARGO_BUILD_JOBS": "2",
        "CARGO_INCREMENTAL": "0",
        "CARGO_PROFILE_DEV_DEBUG": "1",
        "CARGO_PROFILE_TEST_DEBUG": "1",
        "CI": "true",
    })
    for key in ("RUSTFLAGS", "CARGO_ENCODED_RUSTFLAGS", "RUSTC_WRAPPER", "RUSTC_WORKSPACE_WRAPPER"):
        env.pop(key, None)
    steps = [
        ("unit-tests", [str(Path.home() / ".cargo/bin/cargo"), "test", "--frozen",
                        "--features", "custom-protocol", "-j", "2", "--", "--test-threads=1"]),
        ("debug-app", [str(root / "node_modules/.bin/tauri"), "build", "--debug",
                       "--bundles", "app", "--ci", "--no-sign", "--", "--frozen", "-j", "2"]),
    ]
    records = []
    groups = []
    processes = []
    result = 0
    print(f"logs: {log_dir}", flush=True)

    def interrupted(signum, _frame):
        raise InterruptedError(f"Build runner received signal {signum}")

    signal.signal(signal.SIGTERM, interrupted)
    signal.signal(signal.SIGINT, interrupted)
    try:
        for name, command in steps:
            remaining = work_deadline - time.monotonic()
            if remaining <= 0:
                result = 124
                break
            log_path = log_dir / (name + ".log")
            with log_path.open("w") as log:
                log.write(f"cwd: {root}\nargv: {json.dumps(command)}\n")
                log.flush()
                process = subprocess.Popen(command, cwd=root, env=env, stdout=log,
                                           stderr=subprocess.STDOUT, start_new_session=True)
                processes.append(process)
                groups.append(process.pid)
                (log_dir / "owned-process-groups.json").write_text(json.dumps(groups))
                try:
                    code = process.wait(timeout=remaining)
                except subprocess.TimeoutExpired:
                    code = 124
                records.append({"step": name, "exit": code, "log": str(log_path)})
                result = code
                if code:
                    break
    except Exception as error:
        records.append({"runner_error": str(error)})
        result = 1
    finally:
        # These process groups were created by this invocation, never discovered
        # by name or from the daemon endpoint. Terminate descendants as well.
        signal.signal(signal.SIGTERM, signal.SIG_IGN)
        signal.signal(signal.SIGINT, signal.SIG_IGN)
        cleanup_deadline = min(time.monotonic() + 15, final_deadline)

        def alive(group):
            for child in processes:
                child.poll()  # Reap our own exited parents before checking groups.
            try:
                os.killpg(group, 0)
                return True
            except ProcessLookupError:
                return False

        def send(group, sig):
            try:
                os.killpg(group, sig)
            except ProcessLookupError:
                pass

        for group in groups:
            send(group, signal.SIGTERM)
        term_deadline = min(time.monotonic() + 5, final_deadline)
        while time.monotonic() < term_deadline and any(alive(g) for g in groups):
            time.sleep(0.1)
        for group in groups:
            if alive(group):
                send(group, signal.SIGKILL)
        while time.monotonic() < cleanup_deadline and any(alive(g) for g in groups):
            time.sleep(0.1)
        lingering = [group for group in groups if alive(group)]
        if lingering:
            result = result or 125
        summary = {
            "cwd": str(root), "records": records, "owned_groups": groups,
            "lingering_groups": lingering, "exit": result,
            "elapsed_seconds": round(time.monotonic() - started, 2),
            "artifact": str(root / "target/debug/bundle/macos/Werkschaft.app"),
        }
        (log_dir / "summary.json").write_text(json.dumps(summary, indent=2) + "\n")
        print(json.dumps(summary, indent=2), flush=True)
    return result


if __name__ == "__main__":
    sys.exit(main())
