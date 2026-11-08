# Plan: claude-mod dashboard feed drops ghost subagents

Branch `delegate/claude-mod-ghost-agents` (worktree
`/Users/kang-sw/.ws-worktrees/devenv/declared-satiable-proximity`), based on
develop. Commit here; do not merge or push. Do not run `git stash`.

## Problem (user report, 2026-11-08)

The session dashboard's agent tree shows two nameless agents (`에이전트
ad1ac9`, `에이전트 a03496`) stuck at "thinking" for over an hour, each with
one failed tool call (Read, Bash). Lead diagnosis:

- Full ids `ad1ac92e68e2a7c23` and `a034961a2a0831107` appear in the daemon
  snapshot with no `parentId`, no `label`, `status: running`. No Claude
  transcript exists for either id (none under the session's `subagents/`).
- Each started within 0.4 s and 3 s after a main-thread turn ended (Stop hook
  at 07:49:58.09Z / 07:51:29.5Z; records start 07:49:58.37Z / 07:51:32.46Z).
  The likeliest source is a host-internal side agent run after a turn end:
  the host emits `tool.call` (denied or failed) with an `agentId`, but never
  `agent.spawn` and never `turn.complete` for it. This source is inferred,
  not confirmed.
- `agents-plugin/hooks/claude-mod/dashboard.ts`: `rec(id)` (~L668) creates a
  record for any agent id on any event (`toolBegan`, `turnStarted`, step
  events), and `pruneFinished` (~L836) only prunes finished records, so a
  never-announced, never-ended id stays "running" forever.
  `register.ts` ~L1278 (`tool.call`) and ~L1451 (`agent.spawn`) feed it.

## Fix

Make the feed show only subagents the host announced through
`agent.spawn`: events carrying an agent id the dashboard has not seen in
`agentSpawned` are ignored by the tree (lead id `undefined` unaffected).
Keep the change inside `dashboard.ts` (and `register.ts` only if needed).
Check the other per-agent consumers in that file (asks / permission
`toolChecked`, step streaming) so an ignored id leaves no partial state
either. Accepted trade-off: a subagent spawned before the mod (re)started is
not shown; note it in a comment at the site.

If reading the code shows the host always fires `agent.spawn` after the
first event for a real subagent (ordering), handle that ordering rather than
drop real agents: e.g. hold unannounced records out of the emitted tree
until announced. Pick the simplest correct form and say which in the commit.

## Tests

In `dashboard.test.ts` / `dashboard-engine.test.ts` (whichever covers the
tree): a tool call and turn start under an unannounced agent id produce no
agent event; an announced agent still appears with label, parent, and its
batches. Each new test fails before the fix.

## Verification

Run the claude-mod test suite the way the existing tests run (check
`agents-plugin/hooks/claude-mod/` and its package scripts; likely
`bun test` in that directory). Also run any wsflow mirror check the
`wsflow-mirroring.md` manual requires for `agents-plugin/hooks/` edits.
Read `ai-docs/manuals/shipped-surface-boundary.md` and
`ai-docs/manuals/wsflow-mirroring.md` before editing.

## Return

Commit hash, changed files, test output (which new tests failed before the
fix), and the ordering finding.
