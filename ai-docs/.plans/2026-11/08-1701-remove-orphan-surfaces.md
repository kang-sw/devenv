# Plan: remove orphaned delegates and tools (board (459) a)

Branch `delegate/remove-orphan-surfaces` (worktree
`/Users/kang-sw/.ws-worktrees/devenv/renewed-paving-daffodil`), based on
develop. Commit here; do not merge or push.

The user approved this deletion on board (459) a (2026-11-08; an AGENTS.md
"Always ask" item, so the approval covers exactly the list below). Read
before editing: `ai-docs/manuals/shipped-surface-boundary.md`,
`ai-docs/manuals/skill-authoring.md`, `ai-docs/manuals/wsflow-mirroring.md`,
`ai-docs/manuals/ws-mcp.md`. Source tickets:
`ai-docs/tickets/idea/260831-chore-remove-orphaned-review-adjudicator-delegate.md`,
`260730-refactor-retire-goal-fan-out-step-and-session-note.md`,
`260726-feat-proceed-sage-posture-consumption-guard.md`. Triage evidence:
`/tmp/idea-triage/2605-2608.md` (rows 260831, 260730, 260727) and
`/tmp/idea-triage/2609.md` (Gaps: `route.resolve_proceed`).

## Remove

1. The delegate playbooks `review-adjudicator`, `implementer-relay` and
   `implementer-elevated` (under `agents-plugin/rsrc/` and any wsflow or Pi
   mirror), with their manifest entries and the render tests that only keep
   them alive.
2. The `session.note` MCP tool: handler, registration, schema, every
   `runtime.json` (ws, wsflow, Pi) and generated manifest entry, and tests that
   exercise it. Keep tests that assert a surface does not mention it only if
   they still make sense; otherwise remove them.
3. The `route.resolve_proceed` MCP tool and its resolver, the same way.

## Stop conditions

Before deleting each item, search every package (`agents-plugin*/`,
including skills, playbooks, hooks, Pi `src/`, and Go callers other than
tests) for a live caller. A caller other than its own tests, manifests,
inventories, or a retired alias that already routes elsewhere means stop for
that item: leave it in place and report the caller with file:line. Continue
with the rest. Do not change any other tool's behavior or any skill's flow.

## Tickets

When done, close `260831-chore-remove-orphaned-review-adjudicator-delegate`
and `260730-refactor-retire-goal-fan-out-step-and-session-note` as done, and
`260726-feat-proceed-sage-posture-consumption-guard` as dropped (the tool it
guards is removed), each with a one-line reason. Use `ws/tickets.close` with
session key `manila-hurray-backpack` (this worktree's worker key) and confirm
with `git status` that it acted in this worktree; otherwise fall back to
`git mv` there after appending a `## Closed (2026-11-08)` line. Skip a ticket
whose item hit a stop condition. `260727-refactor-implementer-delegate-shared-base`
is closed by a parallel delegate; do not touch it.

## Verification

From `agents-plugin-tool`: `gofmt -l . && go vet ./... && go test -count=1 ./...`.
Run the package tests for anything touched under `agents-plugin/tests` and
`agents-plugin-wsflow/` (pytest), and in `agents-plugin-pi` the tests that
cover `runtime.json` / tool inventories. Regenerate any manifest the
manuals say must be regenerated after a shipped edit. Report pre-existing
failures separately from ones you caused.

## Return

Commit hashes, removed files and entries per item, any item stopped with its
caller, tickets closed, and test output.
