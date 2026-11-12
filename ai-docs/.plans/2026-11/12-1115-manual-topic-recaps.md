# Plan: workflow_manual session state lists open whiteboard topic recaps

## Goal

User decision (board 363 a, 2026-11-12): the `## Session State` section that
`workflow_manual` (and `workflow_state`) returns to a lead after compaction
also lists the recap of each open whiteboard topic, so a revived lead sees
"where each topic stands" without a separate `whiteboard.query`. Today the
section holds only agenda blobs and todos; recaps are visible only through
`whiteboard.query` and the embed snapshot.

Compatibility tier: T0 (additive lines in a tool's text output).

## Scope

- `agents-plugin-tool/internal/mcp/workflow_manual.go`:
  - `renderSessionState(rec sessionRecord)` gains a block after the agenda
    blobs and before the todo summary, rendered only when at least one open
    topic has a non-empty recap:

    ```text
    ### Whiteboard recaps
    - <stem or name>: <recap>
    ```

    One line per open topic with a recap, in the board's topic order (the
    order `wswhiteboard.Query` prints). Closed topics and topics without a
    recap are skipped. A multi-line recap keeps its continuation lines
    indented (as `wswhiteboard.FormatQuery` does) so it never reads as a new
    heading or list item. Keep the handler-owned strings minimal (heading +
    lines); no manual prose.
  - Both callers read the whiteboard part: add `partWhiteboard` to the
    `readState` part mask in `handleWorkflowManual`'s CONTINUE path and in
    `handleWorkflowState` (currently `partAgenda|partTodos`), so the
    byte-identity invariant between the two outputs (comment near
    workflow_manual.go:261) still holds.
  - Prefer reusing `wswhiteboard` (for example `Query` with a topics-depth,
    open-status filter, or a small exported helper there that returns
    open topics' name/stem and recap) over re-walking the store shape in the
    mcp package.
- Tests (in `internal/mcp`, next to the existing workflow_manual /
  workflow_state tests): a session with two open topics (one with a recap, one
  without) and one closed topic with a recap renders exactly one recap line;
  no recap at all renders no heading; workflow_state stays byte-identical to
  the manual's Session State suffix.
- Out of scope: playbook/manual text changes, the dashboard, the embed.

## Verification

- `~/.local/bin/ws-rgo -C agents-plugin-tool -m 8 -- test -count=1 ./internal/mcp/... ./internal/wswhiteboard/...`
  (never run Go build/test/vet locally).

## Branch

Commit on `delegate/manual-topic-recaps` in this worktree. Do not merge or
push. Stop and report if the session record's whiteboard part cannot be read
on the workflow_manual path without a store layout change.
