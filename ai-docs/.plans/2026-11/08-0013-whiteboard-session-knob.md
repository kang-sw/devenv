# Plan: first whiteboard use turns the session's whiteboard knob on

Branch `delegate/whiteboard-session-knob` (worktree
`/Users/kang-sw/.ws-worktrees/devenv/foe-breath-retying`). Commit on this
branch; do not merge or push.

## Problem

A lead that starts using the whiteboard because the user asked, while
`workflow.whiteboard_enabled` resolves `off`, loses the whiteboard mandate
after compaction: `ws/workflow_manual` appends the whiteboard section only
when the knob is on (`agents-plugin-tool/internal/mcp/workflow_manual.go`
around `whiteboardEnabled`, `internal/mcp/whiteboard_text.go`
`applyWhiteboardVariants`), so the revived lead drifts back to chat-only
questions. Observed in this repository's own session 2026-11-08 (knob was
builtin `off`; the user had to repeat the instruction).

## Decision (user, 2026-11-08, board (270) a)

"The first `whiteboard.update` turns that session's
`workflow.whiteboard_enabled` knob on at session scope", so the manual (and
the lead-discuss / lead-ticket variants) carry the mandate for the rest of
the session, revive included. Rejected: a playbook sentence telling the lead
to tune the knob (relies on model compliance); an auto-registered agenda blob
(duplicates state, agenda is not the mandate text).

Settled details (lead):
- Trigger: a successful `whiteboard.update` by a top-level lead whose
  resolved knob (for the current harness bucket, the same resolution
  `whiteboardEnabled` uses) is `off`. Write `on` at session scope for the
  current harness, exactly what `config.tune(key:
  workflow.whiteboard_enabled, scope: session, value: on)` writes.
- Do not override an explicit session-scope `off`: when the session scope
  already holds a value for that harness, leave it. Project, repo, or global
  `off` is overridden only at session scope (the session opted in by using
  the board).
- A refused or invalid batch does not flip the knob. `whiteboard.query`
  never flips it.
- Child sessions (the knob already gates top-level leads only) are
  unaffected.
- If the whiteboard playbook or a config manifest description states that
  the board needs the knob on beforehand, update that text to match; read
  `ai-docs/manuals/skill-authoring.md`, `shipped-surface-boundary.md`, and
  `wsflow-mirroring.md` before editing anything under `agents-plugin*/`
  (mirror to wsflow where the manual requires). Read
  `ai-docs/manuals/ws-mcp.md` before editing `internal/mcp/`.

## Verification

From `agents-plugin-tool/`: `go build ./...`, `go vet ./internal/...`,
`go test -count=1 ./internal/mcp/` (and any package you touch). New tests:
first update with the knob off flips it to session `on` and the next
`workflow_manual` render includes the whiteboard section; an explicit
session `off` is left alone; a refused batch does not flip it.

## Return

Changed files, test output, and any shipped text changed.
