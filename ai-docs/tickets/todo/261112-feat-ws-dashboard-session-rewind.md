---
title: "Dashboard session rewind: drop the conversation after a chosen user message, one direction only"
---

# Dashboard session rewind: drop the conversation after a chosen user message, one direction only

## Background

User request (2026-11-12): the dashboard should be able to rewind a session's
conversation. The user does not want Pi's full rewind model, where every
rewind is a fork and the user can move freely backward and forward through
the branches; a one-directional rewind is enough: the conversation after the
chosen point is discarded.

Each harness offers a different mechanism (Claude Code's interactive
`/rewind` checkpoint menu in the terminal, whatever the hosted Claude
adapter's SDK exposes, Pi's RPC session tree); which of them the daemon or
the claude-mod can drive is not yet verified and is settled at ready
promotion by fact population.

## Decisions

- **One direction only** (user, 2026-11-12): a rewind discards everything
  after the chosen point; there is no branch list, no redo, and no
  navigation back to the discarded part. Rejected: Pi-style fork-based
  rewind with free movement between branches (more than the user needs).
- **Not in 0.49** (user, 2026-11-12: ship 0.49 first): implemented after the
  0.49 release.
- **Every session kind** (whiteboard (482) a): terminal Claude, hosted
  Claude, and Pi sessions all offer rewind; the control is hidden on a
  session whose harness cannot rewind. If fact population shows a harness
  cannot be driven, only that harness is deferred. Rejected: hosted sessions
  first with terminal Claude later; hosted Claude only.
- **Conversation only** (whiteboard (483) a): a rewind removes the
  conversation after the chosen point; files changed since then stay as they
  are (the user recovers them with git if needed). Rejected: restoring files
  too, as Claude Code checkpoints can (harness support differs and
  shell-made changes are never captured); asking on each rewind.
- **Entry point** (whiteboard (484) ok): each user message in the
  conversation view has a "rewind to here" action; after a confirmation the
  conversation from that message on is removed and that message's text
  returns to the composer. Rejected: a separate rewind list dialog like
  Claude Code's `/rewind` menu (more steps, and the choice is made away from
  where the user reads the conversation).

## Constraints

- Manuals: `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/release-compatibility.md`; `ai-docs/manuals/ws-mcp.md`
  if a new inbox entry field, route, or live event kind is added (state its
  compatibility tier in `## Decisions`).

## Phases

### Phase 1: One-directional rewind from the conversation view

Implement per `## Decisions` once the queue settles and fact population has
confirmed each harness's mechanism.

Verification: the affected Go packages through
`~/.local/bin/ws-rgo -C agents-plugin-tool -m 8 -- test <packages> -count=1`
(never run heavy Go builds or tests locally), plus a live rewind on each
supported session kind.
