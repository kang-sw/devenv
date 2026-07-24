---
title: "Agent chat bubble visual design is undifferentiated (all roles share the same box style)"
parent: 260622-epic-ws-dashboard-session-key-realignment
related:
  260711-feat-ws-dashboard-agent-activity-chat-ui: introduced the current bubble styling this idea questions
  260713-fix-ws-dashboard-agent-chat-ui-usability-polish: sibling usability-polish ticket; scoped to specific interaction bugs, not general visual design
---

# Agent chat bubble visual design is undifferentiated (all roles share the same box style)

## Background

User feedback while dogfooding the agent chat UI (no-auth local server,
screenshot of an OpenCode conversation tab): the chat surface looks
"terrible" — user message, system/harness banner, tool-output, thinking
block, and agent-turn reply all render as visually near-identical boxes.

Confirmed in code, not just visual impression:
`ws-dashboard/frontend/src/styles.css:3798-3906` (`.agent-chat-bubble` and
its role variants). Every bubble variant shares the same hairline
`border`, the same `--ws-color-panel-raised` background, and the same
padding, with no border-radius. Differentiation between roles is minimal:
`.agent-chat-bubble-user` only swaps the background token, and
`.agent-chat-bubble-tool` only switches to a dashed border. There is no
avatar/icon, no per-role color coding, and every button (`Copy`,
`Show thinking` toggle, tool-detail toggle, fork/resume) uses the same
bare-bordered button style as the content boxes themselves, so buttons
and content blocks are hard to visually distinguish from one another.

This was built during `260711-feat-ws-dashboard-agent-activity-chat-ui`
as functional-first scaffolding (its own code comments describe it as
"Phase 2 messenger bubble layout"); no dedicated visual-design pass has
happened since. This is not a defect in the sense of broken behavior —
the UI is fully functional — but it is a genuine, currently-open
usability/legibility gap.

## Discussion needed (TBD)

No implementation direction has been agreed yet. Before this becomes an
actionable ticket, decide:

- Target visual direction: keep the current flat/minimal aesthetic but
  strengthen role-based hierarchy (color/weight/spacing only), or move
  toward a more conventional messenger-app look (avatars, rounded
  bubbles, asymmetric alignment emphasis)?
- Which roles most need visual distinction: user vs. agent-turn vs.
  tool-use vs. thinking vs. system/harness banners — is a full redesign
  needed, or just added token-level differentiation (color/icon) reusing
  the existing box shape?
- Whether button affordances (Copy, toggles, fork/resume) should be
  visually demoted relative to message content, so they stop competing
  with the message boxes themselves.
- Whether this should be scoped as one ticket across the whole chat
  surface, or split per concern (bubble roles vs. buttons vs. popovers)
  given `260713-fix-ws-dashboard-agent-chat-ui-usability-polish` already
  covers one popover-styling phase adjacent to this.

Not actionable until the above is settled with the ticket's owner.

## Additional finding: a single turn visually splits into multiple bubbles when tool calls interleave (2026-07-14)

Dogfooding after `260713-bug-dashboard-agent-chat-transcript-role-turnid-echo`
landed (which added `role`/`turnId`): sending one "hello" to a fresh Codex
tab rendered as what looked like two separate replies (screenshot: a
standalone "Show thinking" box, then a text bubble, then a second
"Show thinking"-plus-"Command" box, then a second text bubble).

Confirmed via a headless-Playwright repro
(`ai-docs/ref/dashboard-headless-browser-verification.md` method) that this
is not a duplicate request: exactly one `POST .../prompt {"text":"hello"}`
fired, and the daemon's transcript shows all 4 blocks (thinking, agent text,
tool command, agent text) sharing the same `turnId`. The daemon/turn-id work
is correct; the split is purely a frontend grouping defect:

- `agentChatBubbles.tsx`'s `canMerge` (~line 121-126) requires
  `open.kind === kind` in addition to matching `turnId`. A turn shaped
  `agent text -> tool call -> agent text` (an extremely common Codex
  pattern) changes `kind` from `agentTurn` to `tool` and back, so the same
  `turnId` never merges across the tool call — the single turn renders as
  three-plus separate bubbles instead of one grouped turn.
- Separately, the first `thinking` block of a turn (`~line 101-115`) is
  pushed directly into `bubbles` as its own closed, textless bubble
  whenever no bubble is already `open` — it never becomes the `open` bubble
  itself, so it can never merge forward into the turn's own text/tool
  blocks that follow, even with a matching `turnId`.

This reads to a user as "the agent answered twice" rather than "one turn
with a tool call in the middle," which is exactly the messiness this
ticket's Background section is about — filing it here as a concrete,
evidence-based instance of the "needs role-based/turn-based hierarchy"
question above, rather than as its own bug ticket, since fixing it well is
a grouping/collapse UX design decision (e.g. should a mid-turn tool call
render nested inside one continuous turn bubble, or as a distinct
collapsible sub-item within it?) and not just a one-line logic patch.

## Suspended (2026-07-25)

Agent-GUI feature suspended per user directive (2026-07-25). The dashboard
agent-chat / Codex-tile UI is hidden and un-spawnable (spawn entry points
disabled behind `AGENT_GUI_SUSPENDED`); its acceptance steps are quarantined.
This ticket is excluded from drain selection until the feature is resumed.
Physical FE+BE module extraction is tracked separately in
`260725-refactor-dashboard-agent-gui-physical-module-isolation`.
