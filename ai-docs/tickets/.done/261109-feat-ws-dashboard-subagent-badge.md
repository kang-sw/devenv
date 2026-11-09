---
title: "ws dashboard conversation view: a running-subagent badge N(+M), and the Agent tool for the support profile"
related:
  261107-epic-ws-dashboard-messenger-round2: the conversation view and composer this badge sits on
  261108-feat-ws-tracker-support-agent-chat: its Phase 2 chat card reuses the conversation view and this badge; this ticket also opens the support profile to the Agent tool
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 62ff8849f1c30565
sage-review-completeness-reviewed: 62ff8849f1c30565
completed: 2026-11-09
---

# ws dashboard conversation view: a running-subagent badge N(+M), and the Agent tool for the support profile

## Background

The user (2026-11-09) wants the conversation view to show, while the lead is
working, how many subagents it is running, attached at the right of the
interrupt badge's zone on the composer's top border, as
`N(+M) 서브에이전트` with a spinner: N is the direct children that are
running, M is every running descendant at level 2 and below. The same view is
to be reused by the tracker support-agent chat (Phase 2 of
`261108-feat-ws-tracker-support-agent-chat`), and the user wants the support
agent to be allowed to spawn subagents.

Today:
- The interrupt badge `#ibadge` is the first child of `#composer`
  (`assets/dashboard.tmpl`), absolutely positioned on the top border
  (`.ibadge` in `assets/dashboard.css`), shown by `syncBadge()` in
  `assets/dashboard.js` when `data-mode` is `live` and the lead is working
  (`#r-convlive .clive` `data-phase`).
- `agentTree` (`wsdashboard/pageview.go`) nests `wslive` `Snapshot.Agents` by
  `ParentID`; an agent whose parent was evicted hangs from the root.
  Retention (`wslive/state.go` `retain`) keeps every running agent up to
  `RunningAgentCap` (64) and the 5 most recently finished.
- A delegate agent that holds its own ws session key carries `ReplyID`;
  `lookup(ReplyID)` returns that session and its own snapshot (used for the
  agent row's `Linked` link and for peer rows' `RunningAgents` count, the
  closest existing pattern: `prun` in the `peerrow` template).
- The support profile (`wsagent/builtin.go` `SupportWSTools`,
  `BuiltinProfiles`) sets ws tools only and no base `Tools`, so the Claude
  launch passes an empty `--tools` and `dontAsk` denies everything outside
  `--allowedTools` (`wsagent/claude.go` `claudeArgs`,
  `claudeAllowedRules`); `Agent` is absent by omission.

## Decisions

- **Placement and visibility** (user; board (15) ok): the badge sits on the
  composer's top border at the right of the interrupt badge, on the same
  line, and shows only in live mode while one or more subagents are running,
  as a spinner plus `N(+M) 서브에이전트` (the `(+M)` part only when M > 0).
  Its label is a dashboard i18n string, so every i18n table holds it.
- **Same icon as other agent counts** (board (65), auto-confirmed from fact
  population): the badge reuses the dashboard's existing running-agent
  pattern (spinner and robot icon, as the peer row's `prun`; commits
  e6c3aed2 and faa7da76 made subagents look the same everywhere) followed
  by the `N(+M) 서브에이전트` label. This deliberately departs from
  e6c3aed2's bare-number form (count text only in the hover title): the user
  asked for the visible label on 2026-11-09 and the composer border has the
  room. Rejected: a text label without the robot icon (breaks the
  2026-11-08 one-look rule).
- **Position without the interrupt badge** (design review, auto-confirmed):
  the badge also shows while the lead is idle and background subagents run;
  when `#ibadge` is hidden the badge keeps the slot it has beside it (to the
  right of the border's center), so it never jumps sideways when the lead
  starts or stops working. Rejected: centering it when alone (moves under
  the user's eye on every turn boundary).
- **Counted on the server** (board (16) ok): N and M are computed from the
  server's agent tree (N = running direct children of the lead; M = running
  agents at level 2 and below), not by JS walking the DOM. The count rides
  the region that already re-renders on every live tick (`r-convlive`, for
  example a data attribute next to `data-phase`), and `dashboard.js` reads
  it the way `syncBadge()` reads `data-phase`. Rejected: counting DOM rows in
  JS (it would count only what the agents panel happens to render).
- **Known miscount accepted** (board (16) ok): when a finished parent is
  evicted by retention, its running children count toward N instead of M.
  Running agents are all kept (up to 64), so N + M stays right.
- **Linked delegate sessions count into M** (board (16) ok): a running agent
  with a `ReplyID` whose session `lookup` resolves contributes that
  session's own running agents (recursively, guarded against cycles) to M,
  since a ws-keyed delegate's subagents live in its own snapshot.
- **Main view first, support chat reuses it** (board (16) ok): the badge is
  built into the main conversation view as a template piece the support
  chat card (support-agent-chat Phase 2) can reuse; that card's embedding is
  not part of this ticket.
- **Support profile may spawn subagents** (user; board (17) ok): the builtin
  support profiles allow Claude's `Agent` tool. Subagents must stay inside
  the same tool limit (the tracker tools and `git.search`); the Pi profile
  is unaffected because Pi has no subagent tool.
- **Stop if the limit leaks** (board (38) ok): if the live check shows a
  Claude subagent escaping the support tool limit, the phase stops and
  reports without shipping `Agent`. Rejected: adding a `--disallowedTools`
  style workaround now (inheritance is unproven, and a workaround on an
  unknown mechanism could still leak).
- **Live verification** (board (39) ok): verification includes one live
  support-agent conversation in which a subagent is refused a tool outside
  the profile, as support-agent-chat Phase 1 did (board (444)). The Result
  records which subagent types that conversation can spawn (the launch
  passes `--setting-sources ""` and the profile's plugin dirs, so few are
  reachable) and tests the default one.
- **A leak stops only the tool, not the badge** (completeness review,
  auto-confirmed): if (38) stops `Agent`, the badge part may still merge at
  the lead's call, since it does not depend on the support profile.
- **Standalone ticket** (board (37) ok): this ticket and the composer
  file-mention ticket are standalone with `related:` links. Rejected:
  children of `261107-epic-ws-dashboard-messenger-round2` (that epic awaits
  closure after the user's live check; new children would hold it open).

## Constraints

- Read `ai-docs/manuals/shipped-surface-boundary.md` before editing under
  `agents-plugin-tool/`.
- Adding `Agent` must not widen what the support agent can reach: no other
  base tool (Bash, Read, Write, ...) becomes available to it or to its
  subagents. Whether a Claude subagent inherits the parent's `--tools` /
  `--allowedTools` / `dontAsk` limit is not established in this repository;
  the phase proves it (see Verification) and, if it does not hold, stops and
  reports rather than shipping the tool.
- The support-agent chat card itself (support-agent-chat Phase 2) is out of
  scope.

## Prior Decisions

- 261108-feat-ws-tracker-support-agent-chat (2026-11-08, Decisions): "Runner-only tool listing (board (443) ok): the git search tool and `tracker.show` are ws-mcp tools listed only in a runner conversation whose profile allows them, never in an ordinary lead session's tool list" — bearing: constrains
- 85b9da5f (2026-11-08, commit): "Claude emits system/init only per user turn, so the init check runs at the first turn's init and kills the child on mismatch; dontAsk with --allowedTools still denies unlisted calls in that window." — bearing: constrains
- 5651e314 (2026-11-08, commit): "Running uses AgentNode.IsRunning's rule (status running) over the flat snapshot agent list, so nested running agents count and finished/failed ones do not." — bearing: supports
- e6c3aed2 (2026-11-08, commit): "User direction (2026-11-08): \"subagents are shown with a robot icon; the spaces are mostly narrow\". The running indicator must be compact, and subagents should be drawn the same way everywhere." — bearing: contradiction-candidate
- faa7da76 (2026-11-08, commit): "Running count is computed for every live served peer with an adapter, compact (other-worktree) rows included; finished agents are not counted; zero draws nothing." — bearing: supports
- 53dc37e8 (2026-11-07, commit): "Only finished evictions are remembered; a running agent dropped by RunningAgentCap is still running, so recreating it on its next event is right." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsdashboard/pageview.go, assets/dashboard.tmpl, assets/dashboard.css, assets/dashboard.js, internal/wsi18n/tables/*.json, internal/wsagent/builtin.go, plus tests |
| scope.surface | cross-module | wsdashboard view and template-JS data attribute, wsi18n tables, wsagent builtin support profile Tools reaching claudeArgs --tools and --allowedTools |
| scope.new_public_symbol | yes | a running-count field on the exported conversation view read by the r-convlive template, and a new dashboard i18n key; exact names not fixed by the ticket |
| scope.new_type_contract | no | the counting function is a pure internal helper; the new data attribute on r-convlive is a template-to-JS contract, not a Go type |
| scope.test_surface | existing | wsdashboard/pageview_test.go, wsdashboard/i18n_test.go, wsagent/builtin_test.go, wsagent/claude_test.go |
| complexity.reuse_points | confirmed | agentTree and its lookup in pageview.go#L345-L380, AgentNode.IsRunning pageview.go#L128-L129, peerGroup RunningAgents loop pageview.go#L440-L466, syncBadge in dashboard.js |
| complexity.side_effect_risk | moderate | adding Agent to the builtin Claude support profile widens what a support conversation can launch |
| risk.correctness | moderate | recursive count over linked delegate snapshots needs a cycle guard and the evicted-parent N versus M split |
| risk.fit | moderate | the new badge must sit beside the absolutely centered .ibadge on the composer top border without overlap and be reusable by the support chat card |
| risk.test | moderate | subagent tool-limit inheritance is proven only by one live Claude conversation, and the badge layout only by a hand browser check |
| risk.security_or_contract | high | whether a Claude subagent inherits the parent --tools, --allowedTools, and dontAsk limit is unproven in this repository and a leak would expose base tools to the support agent |

## Phases

### Phase 1: badge and support-profile Agent tool

- Server: compute N and M for the session view from the agent tree plus
  linked delegate sessions (the `lookup` already passed to `agentTree`);
  expose them on the live conversation region; keep the computation a pure
  function with table tests (direct children, grandchildren, an evicted
  parent, a linked delegate with its own running agents, a lookup cycle,
  finished agents ignored).
- Page: the badge markup inside `#composer` next to `#ibadge`, CSS on the top
  border to the right of the interrupt badge (both visible at once must not
  overlap; the same slot when `#ibadge` is hidden; reduced-motion respected
  like `.ibadge`), and `dashboard.js` showing or hiding it on every region
  swap: spinner, robot icon, then `N(+M) 서브에이전트`, with `(+M)` only when
  M > 0 and the label from the dashboard i18n tables.
- Support profile: add `Agent` to the builtin support Claude profile only
  (`BuiltinProfiles` builds the Claude and Pi profiles in one loop, so the Pi
  profile must not gain it) so the launch lists it in `--tools` and
  `--allowedTools`; keep the claude args tests in step.
- Verification: `go test` for `internal/wsdashboard/` and
  `internal/wsagent/`; `node --check` on edited JS; i18n table test green.
  One live support-agent conversation over the daemon (as support-agent-chat
  Phase 1 did) in which the agent spawns a subagent, and that subagent tries
  a tool outside the profile (for example Bash or Read) and is refused; record
  the transcript evidence in the Result. A hand check of the badge in a
  browser is listed as unverified if not done.

### Result (4b0f344ee) - 2026-11-09

Landed in 087228bc2 (support profile) and 4b0f344ee (badge).

- **Counts.** `subagentCounts` (`wsdashboard/pageview.go`) is a pure
  function that fills `ConversationView.Subagents{Direct, Deep}`:
  - Direct is the running agents whose parent is not in the list.
  - Deep is the running agents whose parent is in the list.
  - Deep also gets the running agents of each running delegate whose
    `ReplyID` session has a live adapter. This recurses through that
    session's own delegates. A visited set, seeded with the viewed
    session, counts each session once.
  - The convlive region carries the counts as `data-sub` and
    `data-subdeep`, next to `data-phase`.
- **Badge.**
  - The template piece `subbadge` holds a spinner, the robot icon, and
    `.slabel`. It sits with `#ibadge` in a new `.cbadges` grid row on the
    composer's top border (columns `1fr auto 1fr`). The interrupt badge
    stays centered, and the subagent badge starts the right column.
  - A hidden `#ibadge` now uses `visibility: hidden`, so the subagent badge
    keeps its slot.
  - `dashboard.js` `subBadge()` runs on every convlive swap and mode
    change. It shows the badge in live mode while N+M > 0, as
    `wsTn("js.subagents", N+M, "N(+M)")`; `(+M)` appears only when M > 0.
  - All 11 i18n tables have `js.subagents.one`, `js.subagents.other`, and
    `js.subagents.title` (ko: `{1} 서브에이전트`).
  - Reduced motion turns off the rise animation of both badges. The
    spinner already stopped under the global `.spin` rule.
- **Support profile.**
  - `SupportClaudeTools = ["Agent"]` is set on `tracker-support-claude`
    only, so the launch passes `--tools Agent` and puts `Agent` first in
    `--allowedTools`. The Pi profile still has no built-in tool.
  - Claude 2.1.319 reports the tool as `Task` in system/init. The init
    check therefore accepts `Task` when a profile allows `Agent`.
    Otherwise every launch would have been refused.
  - The ws-mcp runbook records both.

Verification:

- `go test ./internal/wsdashboard/ ./internal/wsi18n/ ./internal/wsagent/
  -count=1` passed. New tests:
  - `TestSubagentCounts`: direct children, grandchildren, a running agent
    under a finished parent, an evicted parent, finished agents ignored, a
    linked delegate (nested agents plus its own delegate), a finished
    delegate, the same delegate twice, a lookup cycle back to the viewed
    session, stale and unserved delegates, a self-parent.
  - `TestConvLiveCarriesSubagentCounts`.
  - The page hook pins for `.cbadges`, `#sbadge`, and the data attributes.
  - `TestClaudeArgsBuiltinSupportProfile`.
  - The `Task`/`Agent` init cases.
  - The builtin profile tool lists.
- `go vet ./internal/...` is clean, and `node --check
  assets/dashboard.js` passed.
- **Live check.** A dev daemon ran on port 39437 with isolated
  `WS_CACHE_HOME` and `WS_CONFIG_HOME`. One `tracker-support-claude`
  conversation was opened through `POST /agents` on a scratch root holding
  `probe.txt`.
  - The launch passed the init check.
  - Subagent types offered: `claude`, `Explore`, `general-purpose`,
    `Plan`.
  - The default-type subagent (no `subagent_type`) reported its only
    tools as `Agent` and the five profile ws tools. It had no Bash, Read,
    or Write, and `leak.txt` was never created.
  - An `Explore` subagent had only the ws tools.
  - `statusline-setup` was refused by the Agent tool: "Agent type
    'statusline-setup' is unavailable because every tool it may use is
    denied by the current permission settings."
  - A direct claude CLI probe with the same flags matched.
  - No leak, so `Agent` ships.
- **Browser check.** Headless chrome-headless-shell screenshots against
  the dev daemon used a fake agent session:
  - lead working at 2(+3): both badges on one line, no overlap;
  - lead idle at 2: same slot, interrupt badge hidden;
  - nothing running: badge hidden;
  - 400px wide: the label is ellipsized.
  - A hand check in the user's own browser was not done.
- **Review.** Lite, one pass: clean, no findings.

Decisions taken:

- A linked delegate counts only when its snapshot has a live adapter, the
  same rule as the peer rows' count. A dead delegate's stale snapshot never
  holds M up.
- Agents are classified as Direct or Deep by whether their parent is in
  the list, not by `agentTree` depth. The two are the same for real
  snapshots. A malformed parent cycle counts as Deep.
- The init check accepts `Task` for `Agent`, rather than the profile
  naming `Task`, so that a future Claude that reports `Agent` in init is
  still accepted.
- The label pluralizes on N+M, with the `N(+M)` text as `{1}`.

#### Forward

- The support chat card (support-agent-chat Phase 2) reuses `subbadge`.
  Its markup hardcodes `id="sbadge"`, so a second composer must give it its
  own binding: call `subBadge(badgeEl, cliveEl, on)` with that card's
  elements.
