"""Claude SessionStart(compact) hook: nudge a compacted lead to run lead-revive.

Claude Code adds this hook's stdout to the context right after a compaction.
A compaction summary that carries "invoke lead-revive first" is easy to skip;
a separate one-line plugin message is not. The hook keeps no workflow state,
so several Claude sessions sharing one MCP server cannot cross-talk through it.

Usage: compact-revive-nudge.py <package>. The package name is the plugin's
own, so the nudge names that plugin and its `<package>:lead-revive` skill;
this file is shared byte-identically by every package that ships it.

The nudge prints only in a workflow project: ai-docs/WORKFLOW.md at the
repository root of the payload's cwd (`git rev-parse --show-toplevel`), or,
outside a repository, at the cwd or its nearest ancestor holding one. That is
the same detection the Claude mod's workflow manual injection uses.
"""

import json
import os
import subprocess
import sys
from pathlib import Path
from typing import Optional

MARKER = Path("ai-docs") / "WORKFLOW.md"


def nudge_text(package: str) -> str:
    return (
        f"[message from {package} plugin] Context was just compacted. If you are a "
        f"top-level {package} lead session (not a spawned worker or subagent), invoke "
        f"{package}:lead-revive with your lead session key before doing anything else; "
        "otherwise ignore this line."
    )


def repo_root(cwd: Path) -> Optional[Path]:
    try:
        proc = subprocess.run(
            ["git", "rev-parse", "--show-toplevel"],
            cwd=cwd,
            capture_output=True,
            text=True,
            timeout=5,
            check=False,
        )
    except (OSError, subprocess.SubprocessError):
        return None
    top = proc.stdout.strip()
    return Path(top) if proc.returncode == 0 and top else None


def is_workflow_project(cwd: Path) -> bool:
    root = repo_root(cwd)
    if root is not None:
        return (root / MARKER).is_file()
    return any((d / MARKER).is_file() for d in (cwd, *cwd.parents))


def main(argv: list) -> int:
    package = argv[1].strip() if len(argv) > 1 else ""
    try:
        payload = json.loads(sys.stdin.read() or "{}")
    except ValueError:
        payload = {}
    if not isinstance(payload, dict):
        payload = {}
    # The hooks.json matcher already limits this to "compact"; the check keeps
    # a manual or misrouted run from nudging a fresh session. Without a
    # package name there is no skill to name.
    if not package or payload.get("source", "compact") != "compact":
        return 0
    cwd = payload.get("cwd")
    cwd = Path(cwd) if isinstance(cwd, str) and cwd else Path(os.getcwd())
    if cwd.is_dir() and is_workflow_project(cwd.resolve()):
        print(nudge_text(package))
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv))
