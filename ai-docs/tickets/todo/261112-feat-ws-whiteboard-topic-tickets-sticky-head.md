---
title: "Whiteboard: topic-linked ticket badges, live-ticket glow, and a sticky topic head carrying recap and check pill"
related:
  261112-feat-ws-whiteboard-check-items: prerequisite
---

# Whiteboard topic tickets and sticky head

## Background

User request (2026-11-12, whiteboard topic `topic-tickets`): a topic should
show the tickets it is about, a topic with live work should stand out, and
while scrolling a long topic its recap, ticket list, and check list should
follow the sticky topic bar.

Mockup: `ai-docs/ref/whiteboard-mockup/r7-topic-tickets-sticky/`
(`index.html` plus rest / stuck / popover / narrow / glow PNGs in light and
dark), committed 6e0e69280 (r7) and 2cdf59ae9 (r7b). It is built on the
shipped `agents-plugin-tool/internal/wswhiteboard/assets/page.css` plus the
r6 check list from `261112-feat-ws-whiteboard-check-items`. Its commit
bodies record the measured head heights.

## Decisions

1. **Explicit links.** A lead links ticket stems to a topic through a
   `tickets` attribute on `wb:topic`; no auto-detection from text. (595 a.)
   Rejected: detecting stems mentioned in the body or items (noisy,
   surprising).
2. **Badge text** is the stem without its date prefix, cut at 17ch by CSS,
   with the full stem and status in a tooltip. (602.) Rejected:
   `#<date>-...` (the date alone says nothing about the ticket).
3. **Status is colour plus shape**: idea hollow circle, todo half-filled,
   ready filled, done check and grey, dropped cross and strikethrough, so it
   reads without colour. (603.)
4. **Glow** marks a topic with at least one live ticket (idea, todo, or
   ready) of an actionable category (feat, bug, refactor, chore); linked
   research and epic tickets show as badges but never light the glow
   (623); it uses the board primary (`--wb-glow`), a 1px ring plus an 18px
   soft halo. A topic whose tickets are all done or dropped, or that has
   none, has no glow. (604.) Rejected: a colour per status (adds hues; the
   glow means one thing, "live").
5. **Sticky unit**: a new `.thead` wrapper holds the bar, the recap, and the
   check pill and sticks as one at the bar's current stick line; `rail.js`
   selectors on `.tbar` follow to `.thead > .tbar`. (605.) Rejected: sticky
   siblings with computed offsets (needs a measured offset each).
6. **Check list in the head**: the summary pill is always in the head and
   the list stays in flow below it; clicking the pill while stuck opens the
   rows as a popover, and at rest it collapses or expands the list. (606.)
   Rejected: collapsing the list inside the head at the stick moment (the
   height change makes the body jump).
7. **Badge placement**: on a wide page the badges sit at the right of the
   bar, before the time and count chips; at 600px or narrower they take
   their own right-aligned, wrapping row under the bar. (607.) Rejected:
   beside the topic name (squeezes the name to three lines when narrow).
8. **Recap in the head** is plain muted 12.5px text on the head's surface,
   no card and no accent bar, sharing a line with the check pill (the pill
   wraps below it under 600px); a hairline under the head separates it from
   the body. A recap outside the head (a closed topic) keeps today's card
   style. (r7b, 614.) Rejected: keeping a thin accent bar (reads as a second
   heading).
9. **Status resolution.** The server resolves each linked stem's status at
   render time from the project's ticket tree (which status directory holds
   the stem); an unknown stem shows a grey "?" badge. (620.) Rejected: the
   lead passing the status in the attribute (stale the moment a ticket
   moves).
10. **Editing.** `tickets` on `wb:topic` is a space-separated stem list that
    replaces the topic's list whole; `tickets=""` clears it. (621.)
    Rejected: add/remove commands (every other topic field is replace-whole;
    lists are short).
11. **Compatibility tier T0.** An optional field on the topic record and an
    optional attribute on an existing command (622;
    `ai-docs/manuals/release-compatibility.md`).

## Constraints

- Lands after `261112-feat-ws-whiteboard-check-items`; the check pill and
  list come from it.
- The dashboard embeds the wswhiteboard assets; both the file page and the
  dashboard page get the head.
- Read `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/release-compatibility.md`, and
  `ai-docs/manuals/wsflow-mirroring.md` (the whiteboard playbook documents
  the new attribute) before editing.

## Phases

### Phase 1: topic tickets, glow, and sticky head

- Store and command: optional `tickets` on `wb:topic`; query output lists
  them; playbook text documents the attribute.
- Render: badges (Decisions 2, 3, 7), glow (Decision 4), `.thead` with recap
  and check pill (Decisions 5, 6, 8), light and dark.

Verification:
- Go tests in `internal/wswhiteboard`: the attribute round-trips through the
  store and query; badge status and glow follow the ticket states (live,
  all done, none).
- Headless Chromium screenshots matching the r7b mockup states (rest,
  stuck, stuck popover, narrow, glow) on the file page and the dashboard
  page; no page errors.
