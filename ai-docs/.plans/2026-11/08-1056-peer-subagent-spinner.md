# Plan: peer row shows running subagents with a spinner

Branch `delegate/peer-subagent-spinner` (worktree
`/Users/kang-sw/.ws-worktrees/devenv/foe-breath-retying`). Commit on this
branch; do not merge or push.

Read `ai-docs/manuals/shipped-surface-boundary.md` before editing (everything
here ships under `agents-plugin-tool/`). UI strings go through the i18n tables
(`internal/wsi18n/tables/*.json`; `internal/wsdashboard/i18n_test.go` checks
coverage).

## Request (user live feedback, 2026-11-08)

"피어 리드에 서브에이전트 떠 있으면 에이전트 패널에 N subagents라던지 그런
식으로 뭔가 스피너가 돌았으면 좋겠네요. 피어 리드가 작업중인지 보통
서브에이전트 여부로 판단해서요." The user judges whether a peer lead is busy
by whether it has subagents running.

## Current state (lead read)

- `internal/wsdashboard/pageview.go` (peer group build, around lines 415-435):
  a served peer with a live adapter gets `Lead`; only a non-compact peer (same
  worktree) gets `Agents` and `AgentCount = len(snap.Agents)`, which counts
  finished agents too. A `Compact` peer (live, another worktree, the common
  case in worktree mode) carries no agent information at all.
- `internal/wsdashboard/assets/dashboard.tmpl` `{{define "peerrow"}}`: the
  phase mark spins from the lead's phase only; the non-compact row shows
  `{{icon "agents"}}{{.AgentCount}}` with no running distinction.

## Decisions (lead, technical)

1. Add a running-subagent count to `PeerRow` (for example `RunningAgents`),
   computed from the peer's snapshot for every served peer with a live
   adapter, compact rows included. Use the same "running" notion the subagent
   tree already uses (`AgentNode.IsRunning` / agent status), not a new rule.
2. When that count is above zero, the peer row shows a spinner plus a short
   label "N subagents" (i18n, singular/plural handled the way the tables
   already handle counts, or a count-neutral form), on compact and
   non-compact rows alike, placed so a narrow row still fits (truncate the
   other text first). Reuse the existing `spin` element and styling from the
   agent tree. At zero nothing extra is shown.
3. Keep the existing total `AgentCount` display on non-compact rows as is (or
   fold it into the new label if that reads cleaner; your call, note it).
4. The live region swap (SSE / region re-render) must update the count and
   spinner without reload, as other peer-row state does today.

## Verification

- `cd agents-plugin-tool && go build ./... && go vet ./internal/wsdashboard/ && go test -count=1 ./internal/wsdashboard/ ./internal/wsi18n/...` (adjust the second path to what exists).
- Add tests in the existing `pageview_test.go` style: a compact peer and a
  non-compact peer with running and finished agents yield the expected running
  count; zero running yields no spinner markup in the rendered row.
- Optional but preferred: a headless Chromium screenshot (cached Playwright
  under `~/.npm/_npx/*/node_modules/playwright`) of a rendered page fixture
  with a peer running two subagents, to `/tmp/peer-subagent-spinner/`.

## Return

Commits, changed files, test output, screenshot path if taken, and anything
unsettled.
