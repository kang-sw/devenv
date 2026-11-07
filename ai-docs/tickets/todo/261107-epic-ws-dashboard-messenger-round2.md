---
title: "ws dashboard round 2: the session page as a messenger-style substitute for the terminal"
related:
  261106-epic-ws-dashboard-serve-daemon: predecessor epic; its hand checks may still be open
sage-review-design: required
---

# ws dashboard round 2: the session page as a messenger-style substitute for the terminal

## Scope

Dogfooding the round-1 dashboard (2026-11-07) showed the user returning to the
terminal for things the session page should cover. This round makes the live
session page usable as the primary conversation surface for a lead session:

- composer: no copy or send buttons; an input-mode gear option (Enter sends by
  default, Ctrl/Cmd+Enter sends as the alternative), IME-safe;
- layout: agents and conversation share a fixed, draggable 1:3 split, each
  scrolling, so content changes never move the other panel;
- lead status shown once: no top lead line; a compact one-line lead row in the
  agents tree;
- lead replies rendered as markdown in the conversation;
- image attachment from the composer (child ticket);
- conversation performance for very long sessions and on-demand loading of
  older history (user, 2026-11-07: the user runs very long sessions; child
  ticket);
- promoting a held message to steering (user, 2026-11-07; child ticket;
- link click actions and mermaid in the conversation (user, 2026-11-07; child
  ticket);
- a session status band with context usage first (user, 2026-11-07; child
  ticket).

## Non-Scope

- Approving or denying permission prompts from the page: the page shows that
  an approval is waiting; the user handles it in the terminal (user decision,
  2026-11-07: auto mode is the default posture, so manual approvals are rare).
- Slash commands, skill search, login, and usage management: the user drives
  the lead in natural language and these are not the dashboard's role.
  Read-only usage display in the status band is in scope (user, 2026-11-07,
  `261107-feat-ws-dashboard-status-band`).

## Child Tickets

- `261107-feat-ws-dashboard-composer-image-attach` - paste/drop images into
  the composer; delivered as file paths.
- `261107-feat-ws-dashboard-conversation-history` - incremental conversation
  updates instead of whole-region swaps, and older history loaded on
  scroll-up beyond the 50 drawn messages.
- `261107-feat-ws-dashboard-steer-promotion` - a held message promoted to
  steering, injected at the next main-thread tool boundary.
- `261107-feat-ws-dashboard-link-actions` - Cmd/Ctrl+click and a click menu
  for links, a file view route, system-browser open, and mermaid in finished
  conversation messages.
- `261107-feat-ws-dashboard-status-band` - context, branch, sync, and usage
  around the composer (mockup variant C).
- `261107-feat-ws-dashboard-pull-down-drawer` - the side panels (Todo,
  Agenda, Tickets, Mailbox) move into a drawer pulled down from the top, a
  frame later tabs register into (user, 2026-11-07).
- Related, not a child: `261107-feat-ws-whiteboard-now-markdown` - the Now
  slot takes markdown with inline HTML (affects disk boards too).
- Done as bounded delegates, no ticket (2026-11-07): the composer buttons and
  input mode, the fixed split, the lead status de-duplication (one delegate),
  and lead-reply markdown rendering (one delegate).

## Cross-Child Decisions

- Page preferences (layout, input mode, split ratio) are browser-local
  (localStorage under `ws.dashboard.`); no server-side preference state.
- No host adapter change in this round, except steer promotion: everything
  reaches the session through the existing inbox drain as text. Steer
  promotion changes both adapters (user, 2026-11-07, option (a) of (29)).
  The status band is the second exception: both adapters feed a new `status`
  live event (user, 2026-11-07, board (88), (89), (93)).
- The page keeps showing a waiting approval (conversation and agents tree)
  after any layout change.

## Completion Criteria

- Done: every scope item has landed on `develop` and the user has used the
  page for a working session on Claude and Pi without returning to the
  terminal except for an approval.
- Dropped: the user abandons the page as a conversation surface.
- Deferred: native image blocks in the injected prompt (instead of file
  paths); page-side permission handling.
