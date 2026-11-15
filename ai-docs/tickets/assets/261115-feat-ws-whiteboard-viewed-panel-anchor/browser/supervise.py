"""Bounded, offline fixture runner; owns only its spawned process tree."""
import json
import os
from pathlib import Path
import signal
import subprocess
import sys
import time

out = Path(__file__).resolve().parent
root = next(p for p in out.parents if (p / "agents-plugin-tool").is_dir())
env = os.environ.copy()
env.update(GOTOOLCHAIN="local", GOPROXY="off", GOSUMDB="off",
           WS_PLAYWRIGHT_MODULE="/Users/kang-sw/.npm/_npx/705bc6b22212b352/node_modules/playwright",
           WS_VIEWED_CAPTURE_DIR=str(out))
cmd = ["go", "test", "./internal/wsdashboard", "-count=1", "-timeout=150s",
       "-run", "^TestBoardViewedBrowser$", "-v"]
run = sys.argv[1]

def ps():
    rows = []
    text = subprocess.check_output(["ps", "-axo", "pid=,ppid=,stat=,command="], text=True)
    for line in text.splitlines():
        a = line.strip().split(None, 3)
        if len(a) == 4:
            rows.append(dict(pid=int(a[0]), ppid=int(a[1]), stat=a[2], command=a[3]))
    return rows

tracked = {}
start = time.monotonic()
timed = False
with (out / ("run-" + run + ".log")).open("w") as log:
    p = subprocess.Popen(cmd, cwd=root / "agents-plugin-tool", env=env,
                         stdout=log, stderr=subprocess.STDOUT, start_new_session=True)
    while p.poll() is None:
        rows = ps()
        ids = {p.pid, *tracked}
        for _ in range(12):
            for row in rows:
                if row["ppid"] in ids:
                    ids.add(row["pid"])
        for row in rows:
            if row["pid"] in ids:
                tracked[row["pid"]] = row
        if time.monotonic() - start > 180:
            timed = True
            os.killpg(p.pid, signal.SIGTERM)
            for sig, pause in [(signal.SIGTERM, 2), (signal.SIGKILL, 0)]:
                for row in ps():
                    if row["pid"] in tracked and row["command"] == tracked[row["pid"]]["command"]:
                        try:
                            os.kill(row["pid"], sig)
                        except ProcessLookupError:
                            pass
                time.sleep(pause)
            break
        time.sleep(.15)
    p.wait(timeout=10)
time.sleep(.5)
report = dict(command=cmd, cwd=str(root / "agents-plugin-tool"),
              environment={k: env[k] for k in ["GOTOOLCHAIN", "GOPROXY", "GOSUMDB", "WS_PLAYWRIGHT_MODULE", "WS_VIEWED_CAPTURE_DIR"]},
              outer_cap_seconds=180, elapsed_seconds=round(time.monotonic() - start, 3),
              returncode=p.returncode, timed_out=timed, tracked=list(tracked.values()),
              finalPS=[row for row in ps() if row["pid"] in tracked])
(out / ("supervisor-" + run + ".json")).write_text(json.dumps(report, indent=2) + "\n")
print((out / ("run-" + run + ".log")).read_text())
print(json.dumps({k: v for k, v in report.items() if k != "tracked"}, indent=2))
print("tracked owned PIDs:", len(tracked))
