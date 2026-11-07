---
title: "Dashboard: session status band (context, branch, sync, usage) around the composer"
parent: 261107-epic-ws-dashboard-messenger-round2
related:
  261107-feat-ws-dashboard-conversation-history: touches the same session page layout and assets; merge-time collision only
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: d11731ecb0142c7c
sage-review-completeness-reviewed: d11731ecb0142c7c
---

# Dashboard: session status band (context, branch, sync, usage) around the composer

## Background

In the terminal the user watches a status line (model, session id, folder,
branch, diff counts, context tokens and cache rate, usage limits, cost). The
session page shows none of it. The user asked for a band with the folder
name, branch state, push/pull state, usage or cost when available, and above
all the current context usage (2026-11-07: "폴더 이름, 브랜치 상태, 브랜치
변경내역(push/pull 상태) 그리고 가능하면 사용량이나 금액 정보, 제일 중요한 게
지금 컨텍스트 사용량").

A static mockup drew three placements:
`ai-docs/ref/whiteboard-mockup/r5-status-band/index.html` (4bd36d1ea,
usage chips 48e9dd44d).

## Decisions

- **Variant C** (user, 2026-11-07, "(58) c, note: 저는 이게 제 취향이긴
  해요."): the context bar sits directly above the composer; the rest (folder,
  branch, sync, usage, cost) is one line below the composer. Rejected: A, a
  one-line strip under the header; B, a 2-3 row terminal-style band.
- **Usage chips** (user, 2026-11-07: "이거 week말고 5h 제약이 같이 뜨면 어떻게
  되나요? 그리고 wk 표시는 필요 없긴 해요"): the 5-hour chip first
  (`42%/18:00 (+5%)`), then the weekly chip (`17%/Sun (-72%)`); no "wk"
  label, the reset clock time or weekday tells them apart; a chip appears only
  when its limit is reported; when both do not fit on a narrow phone band
  (below 360px band width, mockup 48e9dd44d), the higher-percent chip stays (a tie keeps the 5-hour chip), as the mockup
  draws.
- **States as drawn in the chosen mockup**: context and usage warn at 80%+ and
  turn critical at 95%+; sync shows ahead/behind arrows, synced, or no
  upstream; a host without usage-limit data (Pi) omits the limit chips. The
  cost chip follows **Pi cost estimate** below.
- **Claude data source** (user, 2026-11-07, board "(88) ok"): context tokens,
  window and percent, the cache figures, the 5-hour and weekly limits, and
  session cost come from the claude-mod `$.session.usage()` API and its
  `session.measure` hook, the figures the mod's own terminal band already
  computes (`agents-plugin/hooks/claude-mod/band.tsx` `usageFigures`,
  `register.ts` `session.measure`). Rejected: a ws status-line command (Orca
  already owns the single `statusLine` slot, so ws would have to wrap and
  tee it, which is fragile); parsing the transcript JSONL (no usage limits,
  new code). The limit chips appear only when `rateLimits` reports them
  (subscription accounts, after the first reading).
- **Pi data source** (user, "(89) ok"): context comes from Pi's
  `ctx.getContextUsage()`, as the ws Pi footer reads it
  (`agents-plugin-pi/src/agent-footer.ts`); `tokens` is null right after a
  compaction until the next response, and the bar shows `?` then. Pi exposes
  no usage-limit data, so Pi shows no limit chips.
- **Pi cost estimate** (user, "(90) a"): a Pi session shows the cost estimate
  the extension already computes (`agents-plugin-pi/src/agent-cost.ts`,
  known-price calls summed) as `~$1.23`, and `~$1.23+` when calls of unknown
  price contributed. Claude's cost is the exact `/cost` figure and shows
  without `~`. The Pi figure sums the lead's and its direct delegates' calls
  (`leadCost` + `directCost`), matching Claude's `/cost`, which includes
  in-process subagents; with no known-price call yet the chip is omitted. The
  chip has its own formatter (`formatCumulativeCost` prints `—` and ` + ?`,
  the footer's forms). Rejected: omitting the cost chip on Pi, which the
  earlier mockup state implied; the lead's calls alone (understates against
  Claude's figure).
- **Cache hit chip** (user, "(91) b"): a small `CH 92%` sits at the right end
  of the context bar: the last response's cache_read / (uncached input +
  cache_read + cache_creation), not a cumulative rate, computed the same way
  on both Claude and Pi (Pi's `latestCacheHitRate` in
  `agents-plugin-pi/src/agent-cost.ts` already uses it; Claude's band
  `usageFigures` leaves uncached input out, so the dashboard figure is
  computed separately). Rejected: leaving the cache rate out of the band;
  cache_read / (cache_read + cache_creation), which reads 100% on providers
  that report no cache writes.
- **Folder, branch, sync source** (user, "(92) ok"): the session's ws-mcp
  computes them at each 30-second snapshot heartbeat and writes them into the
  live snapshot, for both adapters: folder = last element of the session root
  path; branch and ahead/behind from cheap git calls (`rev-parse` and
  `rev-list --left-right --count` against the upstream), with no upstream
  told apart from in sync (the `internal/wsgit` `BranchStatus.Upstream` model).
  No automatic `git fetch`: behind reflects the last fetch, like a terminal
  status line. Rejected: adapter-side git (the Pi footer has it, but Claude
  would need its own); daemon-side git per render (cost grows with sessions
  served); `git status` (scans the working tree).
- **Snapshot and event contract** (user, "(93) ok"): the live snapshot gains
  new `omitempty` status fields and the intake gains a new event kind
  `status`. An adapter sends `status` only when its ws-mcp reports support for
  it, because today's reducer refuses unknown kinds (`internal/wslive/state.go`)
  and an older ws-mcp can still be running during an upgrade. Rejected:
  folding the figures into existing turn events (their timing differs from
  measure updates).
- **Support report**: the `dashboard.intake` text result gains one line
  naming the optional event kinds this ws-mcp accepts (for example
  `accepts: status`); an adapter starts sending `status` only after an intake
  result carries it, and stops when a later result lacks it. An older ws-mcp
  skips an unknown kind per event ("skipped event N", other events still
  applied, `internal/mcp/live_snapshot.go` `handleDashboardIntake`), so a
  status event sent across an upgrade is harmless noise, not a lost batch.
  Rejected: a new capability tool or runtime.json field (a second channel for
  one bit the intake call already round-trips).
- **Epic scope**: the epic's Non-Scope "usage tracking" line is narrowed to
  usage management; read-only usage display in this band is in scope (the
  user's request above).

## Constraints

- The data sources are the Decisions above (board items (88)-(93),
  2026-11-07). Both adapters change: the claude-mod hooks module and the Pi
  extension feed `status`; ws-mcp computes the git fields.
- Page preferences stay browser-local (epic cross-child decision).
- Manuals: `ai-docs/manuals/shipped-surface-boundary.md`; adapter changes also
  `ai-docs/manuals/wsflow-mirroring.md`; the `internal/mcp/` changes also
  `ai-docs/manuals/ws-mcp.md`, which documents the intake, together with the
  `dashboard.intake` tool description (`internal/mcp/server.go`), which lists
  the event kinds and the result shape: both gain the `status` kind and the
  `accepts:` line.
- The git read never runs per change snapshot: `liveSession.fire()`
  (`internal/mcp/live_snapshot.go`) writes the debounced change snapshots and
  the 30-second heartbeat alike, so the git figures are cached and refreshed
  at most once per heartbeat interval, outside `writeMu` and off the intake
  path. A session root that is not a repository shows the folder alone.
- The `status` payload does not reuse the `Event` struct's existing `status`
  JSON key (agent status); field names, shapes, and whether `reset` clears
  them are the implementer's, pinned by reducer tests.
- Missing or stale data: before the first `status` event, on an adapter or
  ws-mcp without support, or after an adapter stops reporting, the band omits
  the parts it has no figure for; the git parts still show, since ws-mcp
  computes them.
- Both adapter intake edges today return only a delivered flag (Claude
  `dashboard.ts`) or `isError` (Pi); they expose the result text to read the
  `accepts:` line. The "dashboard off" result carries no `accepts:` line, so
  the adapter stops sending `status` then; the adapter test covers it.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin/hooks/, agents-plugin-wsflow/)

## Prior Decisions

- 261028-feat-claude-mod-status-band (2026-10-28, ticket Decisions): "The band is a new feature module (`band.tsx`) joined to `register.ts`'s shared lifecycle fan-out; Claude Code accepts one hooks module per plugin and one unmatched hook per event." — bearing: constrains
- 261028-feat-claude-mod-status-band (2026-10-28, ticket Decisions): "Refresh is event-driven: usage on `session.measure`, model and effort on main-loop `turn.step`, git on `tool.call` (2 s throttle) and `turn.complete`" — bearing: supports
- 4bd36d1e (2026-11-07, commit): "Static mockup only (ai-docs/ref/whiteboard-mockup/r5-status-band/index.html), three variants A/B/C with context normal/warning/critical states and a Pi-style no-cost state; no product code." — bearing: supports
- 48e9dd44 (2026-11-07, commit): "two chips (5h first), 80%/95% warning/critical states, phone keeps the higher-percent chip only below 360px band width." — bearing: supports
- 3d92b646 (2026-11-07, commit): "Epic Non-Scope 'usage tracking' narrowed to usage management because the user asked for read-only usage/cost in the status band." — bearing: supports
- 261107-epic-ws-dashboard-messenger-round2 (2026-11-07, Cross-Child Decisions): "No host adapter change in this round, except steer promotion" — bearing: amended in the same commit to name the status band as the second exception
- 261106-feat-ws-dashboard-claude-adapter (2026-11-07, 001ba50c): "Engine tests for the board path live in dashboard-engine.test.ts, whose world already reads the dashboard knob, rather than engine.test.ts." — bearing: constrains
- 261106-feat-ws-dashboard-live-snapshot-pi (2026-11-06, ticket Result): "a status-less event for an agent that retention already dropped recreates it as running; tool names, epoch, agent ids, and mail titles are not scrubbed." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/hooks/claude-mod/band.tsx, agents-plugin/hooks/claude-mod/register.ts, agents-plugin/hooks/claude-mod/dashboard.ts, agents-plugin-pi/src/dashboard-feed.ts, agents-plugin-pi/src/agent-footer.ts, agents-plugin-tool/internal/wslive/state.go, agents-plugin-tool/internal/wslive/snapshot.go, agents-plugin-tool/internal/wsgit/git.go, agents-plugin-tool/internal/wsdashboard/assets, plus the wsflow hooks copy |
| scope.surface | cross-module | new intake event kind status in internal/wslive and new omitempty snapshot fields read by internal/wsdashboard, fed by two adapters |
| scope.new_public_symbol | yes | a KindStatus event kind constant in internal/wslive/state.go and new exported Snapshot fields |
| scope.new_type_contract | yes | the status event payload and the new snapshot status fields; no support-report mechanism exists yet (search for support or capability in dashboard.ts and dashboard-feed.ts found nothing) |
| scope.test_surface | existing | agents-plugin-tool/internal/wslive/state_test.go, agents-plugin-tool/internal/wsdashboard/pageview_test.go, agents-plugin-tool/internal/wsdashboard/pages_test.go, agents-plugin/hooks/claude-mod/dashboard-engine.test.ts, agents-plugin/hooks/claude-mod/band-engine.test.tsx, agents-plugin-pi/test/dashboard-feed.test.ts, agents-plugin-pi/test/agent-footer.test.ts, agents-plugin-tool/internal/wsgit/git_test.go |
| complexity.reuse_points | confirmed | usageFigures (agents-plugin/hooks/claude-mod/band.tsx#L629-L644), getContextUsage (agents-plugin-pi/src/agent-footer.ts#L177), CumulativeCost (agents-plugin-pi/src/agent-cost.ts), BranchStatus (agents-plugin-tool/internal/wsgit/git.go#L101-L107) all read |
| complexity.side_effect_risk | moderate | git calls added to every 30-second heartbeat per session, and a new event kind that older ws-mcp builds reject (state.go#L223-L225) |
| risk.correctness | moderate | context value must track the host line; the compaction null, cache rate definitions, and narrow-band chip rule are easy to get subtly wrong |
| risk.fit | moderate | the epic Cross-Child line now names this ticket as the second adapter-change exception (amended in the promotion commit); Pi's `latestCacheHitRate` already matches the decided cache formula, Claude's `usageFigures` does not and is not reused for it |
| risk.test | moderate | live host agreement and browser layout have no automated check; reducer, git, and render states are testable |
| risk.security_or_contract | moderate | new snapshot and intake wire contract with an old-ws-mcp compatibility requirement; git output reaches the page |

## Phases

### Phase 1: Status band in variant C

Implement the band on the live session page per the decisions above, fed by
the data sources settled before promotion. Suggested order: the `wslive`
contract and ws-mcp (status kind, snapshot fields, `accepts:` line, git
figures), then the page, then the two adapters and the wsflow hooks mirror.

Verification: page render tests for each state (normal, warning, critical,
no upstream versus synced, ahead/behind, Pi without limit chips, Pi
`~$X`/`~$X+` cost, the context `?` after compaction, the cache chip); the
narrow-phone chip rule; reducer tests for the `status` kind and an adapter
test that `status` is not sent to a ws-mcp that does not report support; a
test of the ws-mcp git fields on a repository with and without an upstream
and on a non-repository root; a test that the git read is not repeated per
change snapshot; the Pi cost sum of lead and direct calls and the omitted
chip with no known-price call.
Live: on Claude and Pi the context value tracks the host's own status line or
footer.
