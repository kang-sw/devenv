"""Claude SessionStart(compact) hook: nudge a compacted lead to run lead-revive.

Claude Code adds this hook's stdout to the context right after a compaction.
A compaction summary that carries "invoke lead-revive first" is easy to skip;
a separate one-line plugin message is not. The hook keeps no ws state, so
several Claude sessions sharing one ws-mcp cannot cross-talk through it.

Probe (temporary): each firing appends its stdin payload to
compact-hook-probe.jsonl under the ws cache home, to confirm whether a
subagent's own compaction ever fires this event. Remove the probe once that
is settled.
"""

import json
import os
import sys
import time
from pathlib import Path

NUDGE = (
    "[message from ws plugin] Context was just compacted. If you are a "
    "top-level ws lead session (not a spawned worker or subagent), invoke "
    "ws:lead-revive with your lead session key before doing anything else; "
    "otherwise ignore this line."
)


def probe(payload: dict) -> None:
    cache_home = os.environ.get("WS_CACHE_HOME") or os.path.join(
        os.environ.get("XDG_CACHE_HOME") or os.path.expanduser("~/.cache"), "ws"
    )
    record = {
        "ts": time.strftime("%Y-%m-%dT%H:%M:%S%z"),
        **{k: payload.get(k) for k in ("session_id", "source", "agent_type", "agent_id", "cwd")},
    }
    path = Path(cache_home) / "compact-hook-probe.jsonl"
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("a", encoding="utf-8") as fh:
        fh.write(json.dumps(record) + "\n")


def main() -> int:
    try:
        payload = json.loads(sys.stdin.read() or "{}")
    except ValueError:
        payload = {}
    if not isinstance(payload, dict):
        payload = {}
    try:
        probe(payload)
    except OSError:
        pass
    # The hooks.json matcher already limits this to "compact"; the check keeps
    # a manual or misrouted run from nudging a fresh session.
    if payload.get("source", "compact") == "compact":
        print(NUDGE)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
