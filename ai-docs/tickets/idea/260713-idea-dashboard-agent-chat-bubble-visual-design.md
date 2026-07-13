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
