# Plan: conversation tool summary v2 - per-request tokens and exact per-call data

Branch `delegate/tool-summary-v2` (worktree
`/Users/kang-sw/.ws-worktrees/devenv/declared-satiable-proximity`). Commit on
this branch; do not merge or push.

## Decisions (user, board, 2026-11-07)

- (164) ok: the tools summary row and its expanded list are left-aligned
  (today the row and list are centered in the conversation column), and each
  expanded line (one model request = one batch) shows its duration and failed
  state instead of only "1 call".
- (165) b: adapters (Claude mod and Pi) are changed, reversing (143)'s "no
  adapter change": send per model request its token usage (input, output,
  cache read, cache write) and per call its tool name, start/end time, and
  failed flag. No argument or result content (rejected (165) c: commands and
  paths would persist in the ring and show on LAN screens). Outcomes the user
  expects: the summary row head shows the gap's token totals; per-tool counts
  become exact; finished batches beyond the rolling window of 5 are no longer
  lost before the daemon counts them.

## Current state (from an evidence read; verify before relying)

- Claude adapter is a Claude Code mod, not shell hooks:
  `agents-plugin/hooks/claude-mod/dashboard.ts` (BatchView at ~216-226,
  `batchViews` ~353-390, TOOL_NAME_CAP / BATCH_CAP ~86-87, per-call
  startedAt/endedAt recorded ~207/325-336 but unsent, `stepEnded` ~293 ignores
  `usage`), wired in `register.ts` (~1249-1262 tool.call, ~1389-1393
  turn.step tap passes the full TurnStepResult incl. `usage`).
- Pi adapter: `agents-plugin-pi/src/agent-batches.ts` (BatchCall ~46-56 holds
  per-call argsPreview/resultPreview/startedAt/endedAt/isError; `batchView()`
  ~335 drops them; caps ~37-43), `agents-plugin-pi/src/dashboard-feed.ts`
  (message_end -> observeBatchEvent ignores `message.usage`; see
  agent-telemetry.ts:332 / agent-cost.ts:236 for how usage is read).
- Daemon: `agents-plugin-tool/internal/wslive/state.go` (`batchIn` ~716,
  `normalizeBatch` ~755-820, BatchCap=5, ToolNameCap=5),
  `wslive/toolsummary.go` (approximate attribution, 16 kinds / 40 batches),
  `wsdashboard/pageview.go`, `wsdashboard/assets/dashboard.tmpl|css`
  (`toolsline`, `.brow`).

## Outcome

1. Wire (additive, backward compatible both ways: an old daemon ignores new
   fields; a new daemon handles an old adapter by falling back to today's
   approximate path): each batch may carry `usage` {input, output,
   cacheRead, cacheWrite} and `calls` [{name, startedAt, endedAt?, failed}]
   bounded (e.g. 32 calls per batch; names length-capped, session keys
   scrubbed like toolNames). Design the batch-loss fix: e.g. a monotonic
   per-adapter batch seq the daemon tracks so it can count each finished
   batch once, and have adapters keep finished-but-unacknowledged batches
   or send a finished batch in an append-style event; pick the simplest
   design that makes a gap's counts exact under normal operation and say
   what remains lossy (e.g. ws-mcp restart).
2. Both adapters send the new fields (Claude: usage from turn.step result
   and per-call times it already records; Pi: per-call data already in
   BatchCall, usage from message_end). Keep existing fields unchanged.
3. Daemon: normalize/bound the new fields; toolsummary uses exact per-call
   names when present (approximate fallback otherwise), sums tokens per gap
   and per batch, records per-batch duration and failed; live row
   (Lead.ToolGap) carries the same.
4. Render: left-align the row and the list; the row head adds token totals
   (compact, e.g. `in 12.3k · out 1.1k · cache 210k`); each expanded line
   shows tool chips (with xN for repeats), duration, failed mark, and the
   request's tokens when known.
5. Tests: adapter unit tests (Claude mod tests and Pi tests, whichever
   exist for these files), wslive normalize/summary tests (exact path,
   fallback path, bounds, batch-loss fix), wsdashboard render test.

## Constraints

- Read before editing: `ai-docs/manuals/shipped-surface-boundary.md`
  (agents-plugin*/ and agents-plugin-tool/), `ai-docs/manuals/wsflow-mirroring.md`
  (agents-plugin/hooks/: check whether wsflow has a mirror of claude-mod that
  must change too), `ai-docs/manuals/ws-mcp.md` if internal/mcp changes.
- No argument or result content on the wire or in the ring.

## Verification

From `agents-plugin-tool/`: `go build ./...`, `go vet ./internal/... ./cmd/...`,
`go test -count=1 ./internal/wslive/ ./internal/wsdashboard/ ./internal/mcp/`.
Claude mod: its type check/tests as the package defines them. Pi: the package's
test/typecheck scripts (see its package.json; note the known source-test
issue in ticket 261107-bug-pi-source-tests-fetch-release-runtime if it bites,
and report rather than fix it). Headless Chromium fixture (cached Playwright
under ~/.npm/_npx/*/node_modules/playwright) of the row collapsed/expanded,
light and dark; screenshots to /tmp/tool-summary-v2-shots.

## Return

Changed files, the batch-loss design chosen and what stays lossy, wire field
names, verification output, what was not checked (real Claude/Pi sessions).
