# Plan: claude-mod feed resolves unannounced agents through $.agent.list()

Branch `delegate/claude-mod-agent-list-recovery` (worktree
`/Users/kang-sw/.ws-worktrees/devenv/declared-satiable-proximity`), based on
develop (which has 0aaf8bdcc). Commit here; do not merge or push. Do not run
`git stash`.

## Background

0aaf8bdcc (`ai-docs/.plans/2026-11/08-1812-claude-mod-ghost-agents.md`) made
`agents-plugin/hooks/claude-mod/dashboard.ts` hold a subagent out of the
dashboard tree until `agent.spawn` announces it, to hide nameless "ghost"
agents. Its cost: after the mod restarts (`/reload-plugins`, session
resume), a subagent spawned earlier is never announced and never shown. The
user finds that inconvenient during development and asked for a cleaner
solution (board (465)).

Engine types (`AgentLoop.agentId` in the claude-code engine `index.d.ts`;
a copy is at `/private/tmp/tc/types/claude-code/index.d.ts`) say: "A
workflow's agents and the engine's own forks (compaction, memory) carry ids
no list names." `$.agent.list()` returns `AgentInfo` records (`id`,
`description`, `type`, `status`, `parentId?`, `name?`, `teammateId?`) for
the session's real subagents and teammates.

## Change

1. When an event arrives for an agent id the feed has not announced, resolve
   it through `$.agent.list()`: listed -> announce it with `description` as
   label, `type` as caption, `parentId` as parent (same shape as
   `agentSpawned`); not listed -> keep it held (an engine fork or workflow
   agent). One lookup per unknown id, not per event; do not block or delay
   the event path on it (the record is held until the lookup answers, as
   now). A lookup that throws leaves the record held, so the existing
   spawn-announce path still applies.
2. At feed start (mod start or reload), seed from `$.agent.list()`: every
   listed agent whose status is running (or idle teammate) is announced, so
   activity after a reload shows under its label and parent.
3. Keep `agentSpawned` as is; a later spawn for a listed id just refreshes
   label/parent.
4. The `dashboard.ts` core takes this through its injected edges (follow the
   file's existing `edges` / dependency style so tests can fake it), and
   `register.ts` wires `$.agent.list()`.
5. Update the 0aaf8bdcc trade-off comment at `rec()` and the file header:
   the remaining gap is only when `$.agent.list()` is unavailable or fails.

## Tests

Unit tests with a faked list: an unannounced id that the list names is shown
with label/parent/caption and its earlier batches; one the list does not name
stays hidden; a list lookup that rejects keeps it held and a later spawn
still shows it; seeding at start shows a running listed agent's next turn
without any spawn; one lookup per unknown id. Each fails before the change.

## Verification

`claude plugin test agents-plugin` and `claude plugin test
agents-plugin-wsflow`, `claude plugin validate` on both, and
`go test ./internal/wsrsrc -run 'TestWsflowHooksMirrorUpToDate'` from
`agents-plugin-tool`. Resync the wsflow copies byte-identical. Read
`ai-docs/manuals/shipped-surface-boundary.md` and
`ai-docs/manuals/wsflow-mirroring.md` before editing.

## Return

Commit hash, changed files, test output (which new tests failed before),
and whether `$.agent.list()` is available at feed start (e.g. needs an
await after session start).
