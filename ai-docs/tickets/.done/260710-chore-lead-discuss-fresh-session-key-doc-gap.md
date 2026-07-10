---
title: "lead-discuss/lead-sprint fresh-session playbook.print session_key gap"
completed: 2026-07-10
---

# lead-discuss/lead-sprint fresh-session playbook.print session_key gap

## Background

Dogfood surprise raised by the user: `lead-discuss` and `lead-sprint`
(both `agents-plugin/skills/` and their `agents-plugin-wsflow/skills/`
mirrors) instruct a parallel call pair:

- `ws/playbook.print(name: "lead-discuss", session_key: <your key>)`
- `ws/workflow_manual(session_key: <your key or "obsidian-latch" if fresh>, root: <absolute worktree path if fresh>)`

The `workflow_manual` line explicitly names the fresh-session fallback
(`"obsidian-latch"` + `root`), but the `playbook.print` line assumes the
caller already holds a real key. On a genuinely fresh session, no real key
exists yet — it is only minted by the parallel `workflow_manual` call, which
has not returned when `playbook.print` is dispatched. The instruction text
did not say what to pass in that case.

Verified against `agents-plugin-tool/internal/mcp/server.go`'s
`playbook.print` handler (`case "playbook.print"`): `session_key` is
optional there, and an unresolvable key is tolerated by
`buildOverrideLookup` (falls back to no overrides / seed defaults), unlike
`workflow_manual` which fails loud on an unresolvable key. So the gap was
not a functional break, only a documentation ambiguity.

## Phases

### Phase 1: Clarify the fresh-session fallback in skill text

Change the `playbook.print` line in all four affected files to
`session_key: <your key, omit if fresh>`, matching `playbook.print`'s
actual optional/graceful-fallback behavior.

### Result (pending-commit) - 2026-07-10

Applied directly as a hotfix (no separate implementation pass) to:
- `agents-plugin/skills/lead-discuss/SKILL.md`
- `agents-plugin/skills/lead-sprint/SKILL.md`
- `agents-plugin-wsflow/skills/lead-discuss/SKILL.md`
- `agents-plugin-wsflow/skills/lead-sprint/SKILL.md`

Each `ws/playbook.print(...)` / `wsflow/playbook.print(...)` line's
`session_key: <your key>` became `session_key: <your key, omit if fresh>`.
No other skill files use this parallel-call pattern with the same gap
(`lead-revive` only calls `workflow_manual`; all other `playbook.print`
callers omit `session_key` entirely).
