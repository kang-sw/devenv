# Plan: lead tool-usage summary rows in the dashboard conversation

Branch `delegate/conv-tool-summary` (worktree
`/Users/kang-sw/.ws-worktrees/devenv/declared-satiable-proximity`). Commit on
this branch; do not merge or push.

## Decision (user, board (143) b, 2026-11-07)

Between consecutive lead conversation entries (replies, user messages, mail,
compaction), the conversation shows one collapsed summary row of the lead's
finished tool calls in that gap: per-tool call counts (e.g.
`Bash 5 · Read 3 · whiteboard.update 2`), failed count, and elapsed time.
Expanding the row lists the batches in that gap, each with its tool names.
Progress accumulates visibly during a long turn, not only at turn end.

Rejected: (a) only enlarging the existing turn-end line; (c) per-call argument
previews, which need Claude and Pi adapter changes and widen what a LAN
viewer sees. No host adapter change: use only what the daemon already
receives (BatchView ToolNames, CallCount, Failed, StartedAt, EndedAt).

## Current state

- `agents-plugin-tool/internal/wslive/state.go`: batch lists arrive per event
  (`normalizeBatches`, `setBatches`, `closeBatches`); the ring receives
  `RingUser`, `RingReply`, `RingTurn` (state.go:223, with Done/Failed/
  DurationMs), `RingMail`, `RingCompact`. Finished batches are never recorded
  into the ring.
- `agents-plugin-tool/internal/wslive/ring.go`: `RingEntry` and ring kinds.
- `agents-plugin-tool/internal/wsdashboard/pageview.go` `conversation()`
  drops finished batches (`splitBatches`), keeping only running/generating.
- Templates/CSS/JS under `agents-plugin-tool/internal/wsdashboard/assets/`.

## Outcome

1. wslive: when a lead batch finishes (newly finished batches as seen by
   `setBatches`, and the ones `closeBatches` ends), accumulate them in
   State; flush one new ring entry kind (e.g. `RingTools = "tools"`) with
   per-tool counts, total calls, failed, duration (first start to last end),
   and a bounded per-batch tool-name list, immediately before the next
   conversation entry the state writes (reply, user, mail, compact, turn).
   No entry when the gap had no finished batch. Lead only, not subagent
   batches. Bound sizes (tool kinds, batches per entry, name length) so a
   pathological turn cannot bloat a ring file. Keep the existing RingTurn
   line unchanged.
   Decide yourself whether a still-open gap during a long turn also needs a
   live (non-ring) row from the current snapshot's finished batches so
   progress shows before the next reply; prefer it if it is a small,
   contained change in `conversation()`, since the user's complaint is the
   empty look during long turns.
2. wsdashboard: render the new kind as a collapsed `<details>` row styled
   like the existing turn line (muted, one line), expanding to the batch
   list. Works in the incremental conversation update and older-history
   paging paths that already handle the other kinds.
3. Tests: wslive state tests for the flush points, the no-batch gap, bounds;
   wsdashboard render test for the new row. Existing tests stay green.

## Verification

`go build ./...`, `go vet ./internal/... ./cmd/...`,
`go test -count=1 ./internal/wslive/ ./internal/wsdashboard/` from
`agents-plugin-tool/`; `node --check` on changed JS. If practical, a headless
Chromium check (cached Playwright under `~/.npm/_npx/*/node_modules/playwright`)
of a fixture page showing the row collapsed and expanded, light and dark.

## Return

Report changed files, decisions you took (live row yes/no, entry shape),
verification output, and what was not checked.
