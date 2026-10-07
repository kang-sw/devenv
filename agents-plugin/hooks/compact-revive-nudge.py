"""Claude SessionStart(compact) hook: nudge a compacted lead to run lead-revive.

Claude Code adds this hook's stdout to the context right after a compaction.
A compaction summary that carries "invoke lead-revive first" is easy to skip;
a separate one-line plugin message is not. The hook keeps no ws state, so
several Claude sessions sharing one ws-mcp cannot cross-talk through it.
"""

import json
import sys

NUDGE = (
    "[message from ws plugin] Context was just compacted. If you are a "
    "top-level ws lead session (not a spawned worker or subagent), invoke "
    "ws:lead-revive with your lead session key before doing anything else; "
    "otherwise ignore this line."
)


def main() -> int:
    try:
        payload = json.loads(sys.stdin.read() or "{}")
    except ValueError:
        payload = {}
    if not isinstance(payload, dict):
        payload = {}
    # The hooks.json matcher already limits this to "compact"; the check keeps
    # a manual or misrouted run from nudging a fresh session.
    if payload.get("source", "compact") == "compact":
        print(NUDGE)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
