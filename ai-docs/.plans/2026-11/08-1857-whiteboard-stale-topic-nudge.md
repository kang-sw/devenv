# Plan: whiteboard stale-topic nudge

Branch `delegate/whiteboard-stale-topic-nudge` (worktree
`/Users/kang-sw/.ws-worktrees/devenv/declared-satiable-proximity`), based on
develop. Commit here; do not merge or push. Do not run `git stash`.

Source: user request on the session whiteboard (board (476), (477),
2026-11-08): leads rarely close finished topics, so the board fills with
quiet or stale-open topics. Add a light, rate-limited info nudge to the
`whiteboard.update` result.

Read first: `ai-docs/manuals/shipped-surface-boundary.md` (the nudge text
ships to downstream agents) and `ai-docs/manuals/ws-mcp.md` (the tool lives
under `internal/mcp/`). Code: `agents-plugin-tool/internal/wswhiteboard/`
(store, view; `state.topicClosed`, item `Changed`, topic `Changed`, statuses)
and the `whiteboard.update` handler under `internal/mcp/`.

## Behavior (settled by the user)

1. After a successful `whiteboard.update` batch, count the topics that are
   not closed. When that count is 5 or more, find the stale ones: a topic
   whose items and body have not changed for 60 minutes or more and that has
   no item waiting on the user (no `open` or `assumed` item). When at least
   one topic is stale, append one info line to the tool result, for example:
   `info: 3 of 7 open topics unchanged for 60+ min (A, B, C); if finished, close them with wb:close.`
   Keep the wording neutral and downstream-generic (no repo names, no board
   IDs from this project). Cap the listed names (e.g. first 5, then `, ...`).
2. Rate limit: at most once per hour per session board. Persist the time of
   the last nudge in the whiteboard store's state so the hour survives a
   daemon restart or a lead compaction. Store-format compatibility: an old
   store without the field reads as "never nudged".
3. The nudge appears only in the tool result the lead receives, never on the
   rendered page or embed.
4. Constants (5 topics, 60 minutes stale, 60 minutes between nudges) are
   package constants, not config knobs. Time comes from an injectable clock
   if the package already has one; otherwise add the smallest seam the tests
   need.
5. Update the `whiteboard` playbook only if it documents the result fields
   (`ids:`, `full:`, `embed:`): add one sentence about the optional `info:`
   line, and mirror per `ai-docs/manuals/wsflow-mirroring.md` if the playbook
   is mirrored. Regenerate any manifest the repo's tests require.

## Tests (each fails before the change)

- 5+ non-closed topics with stale ones -> one info line naming them; topics
  with an open or assumed item are excluded; closed topics are not counted.
- Fewer than 5 non-closed topics -> no line.
- A second update within the hour -> no line; after the hour -> line again.
- The last-nudge time survives a store reload.
- The page/embed render does not contain the line.

## Verification

`cd agents-plugin-tool && gofmt -l . && go vet ./internal/wswhiteboard/ ./internal/mcp/ && go test -count=1 ./internal/wswhiteboard/ && go test -count=1 -run 'Whiteboard' ./internal/mcp/`, plus `go test -count=1 ./internal/wsrsrc/` if a playbook changed.

## Return

Commit hash, changed files, test output (which new tests failed before),
the exact nudge text, and anything left open.
