---
title: "ws dashboard conversation view: a running-subagent badge N(+M), and the Agent tool for the support profile"
related:
  261107-epic-ws-dashboard-messenger-round2: the conversation view and composer this badge sits on
  261108-feat-ws-tracker-support-agent-chat: its Phase 2 chat card reuses the conversation view and this badge; this ticket also opens the support profile to the Agent tool
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
  the profile, as support-agent-chat Phase 1 did (board (444)).
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
  overlap; reduced-motion respected like `.ibadge`), and `dashboard.js`
  showing or hiding it on every region swap with the label `N(+M)`.
- Support profile: add `Agent` to the builtin support Claude profile so the
  launch lists it in `--tools` and `--allowedTools`; keep the claude args
  tests in step.
- Verification: `go test` for `internal/wsdashboard/`, `internal/wslive/`,
  `internal/wsagent/`; `node --check` on edited JS; i18n table test green.
  One live support-agent conversation over the daemon (as support-agent-chat
  Phase 1 did) in which the agent spawns a subagent, and that subagent tries
  a tool outside the profile (for example Bash or Read) and is refused; record
  the transcript evidence in the Result. A hand check of the badge in a
  browser is listed as unverified if not done.
