---
title: "Whiteboard: topic-linked ticket badges, live-ticket glow, and a sticky head carrying the latest agent line and check pill"
blocked-by: 261113-feat-ws-whiteboard-topic-panels-and-log
related:
  261112-feat-ws-whiteboard-check-items: prerequisite
  261113-feat-ws-whiteboard-topic-panels-and-log: "prerequisite; latest-agent summary and authoritative topic folders"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: d765664857d80e60
sage-review-completeness-reviewed: d765664857d80e60
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

## Implementation Handoff (2026-11-13)

The panels/log prerequisite lands first. The sticky summary consumes
`TopicView.LatestAgent` (or `TopicDoc.LatestAgent()`), never the legacy recap
fallback in `Summary()` or the latest system event. Panel `recap` attributes remain panel metadata, not
this head's summary. Store topic links in authoritative topic `meta.json`
through the folder transaction; do not revive cache Whiteboard writes. Preserve
entry history and shared history-window/disclosure restoration. The old mockup
and Prior Decisions remain historical visual evidence only.

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
5. **Sticky unit**: a new `.tophead` wrapper holds the bar, the note
   (`.tnote`), the latest agent log line, and the check pill and sticks as one at the bar's
   current stick line; `rail.js` selectors on `.tbar` follow to
   `.tophead > .tbar` (full dependent list in Decision 13). (605.) Rejected:
   sticky siblings with computed offsets (needs a measured offset each).
6. **Check list in the head**: the summary pill is always in the head and
   the list stays in flow below it; clicking the pill while stuck opens the
   rows as a popover, and at rest it collapses or expands the list. (606.)
   Rejected: collapsing the list inside the head at the stick moment (the
   height change makes the body jump).
7. **Badge placement**: on a wide page the badges sit at the right of the
   bar, before the time and count chips; at 600px or narrower they take
   their own right-aligned, wrapping row under the bar. (607.) Rejected:
   beside the topic name (squeezes the name to three lines when narrow).
8. **Latest agent line in the head** is plain muted 12.5px text on the head's
   surface, no card and no accent bar, sharing a line with the check pill
   (the pill wraps below it under 600px); a hairline under the head separates
   it from the body. The line outside the head (a closed topic) keeps the
   summary card style. No agent line means no summary; a system event never
   substitutes for it. (r7b, 614.) Rejected: keeping a thin accent bar (reads as a second
   heading).
9. **Status resolution.** The server resolves each linked stem's status at
   render time from the project's ticket tree (which status directory holds
   the stem), through the injected resolver of Decision 15; an unknown stem
   shows a muted "unknown" badge, never an error. (620.) Rejected: the lead
   passing the status in the attribute (stale the moment a ticket moves).
10. **Editing.** `tickets` on `wb:topic` is a space-separated stem list that
    replaces the topic's list whole; `tickets=""` clears it. (621.)
    Rejected: add/remove commands (every other topic field is replace-whole;
    lists are short).
11. **Compatibility tier T0.** An optional field on the topic record and an
    optional attribute on an existing command (622;
    `ai-docs/manuals/release-compatibility.md`).
12. **Class name `.tophead`, not `.thead`.** `.thead` is the dashboard session
    header (dashboard.css 164-166/784/1744, dashboard.tmpl 19/138/186,
    dashboard.js 217/1030) and both sheets load on the dashboard page; the
    precedent dab009c4 renamed `.crow` to `.ckrow` for the same reason (the
    board must not depend on dashboard scoping). dashboard.js and
    dashboard.css are not touched. A test pins the board-owned name, like
    `TestCheckRowClassIsTheBoardsOwn`. Rejected: scoping every dashboard
    `.thead` rule.
13. **Every direct-child `.tbar` dependent follows the bar into `.tophead`**:
    rail.js:97/108, page.css 389-390 and 547-570, cards.js:284
    (`child(sec,"tbar")`), cards.js:682/875 (`b.parentNode.parentNode`), and
    the tests rail_offsets_test.go, topic_header_test.go, recap_test.go.
    `.tnote` moves inside `.tophead`, so there is one sticky element per
    topic and no second sticky offset to compute. Rejected: keeping `.tnote`
    a sibling sticky at a head-height offset.
14. **Grammar.** `wsxmlcmd.Group` (internal shared package) gains support for
    more than one extra attribute; `wb:topic` keeps `label` and adds optional
    `tickets`, a space-separated stem list. `TopicDoc`'s keep-invariant
    (`dropTopicDoc`: content, entries, or legacy recap) extends to a non-empty ticket list, so a
    topic with only `tickets=` survives. Rejected: encoding tickets in the
    label or in a separate command (a second way to address a topic).
15. **Status resolver is injected.** The render takes a small resolver
    interface (stem -> status, ok) supplied by the host; both hosts (the MCP
    `whiteboard.update` file-page writer and the dashboard daemon) supply one
    that reads the session root's ticket tree through the existing tickets
    package. A stem the resolver does not find renders as a muted "unknown"
    badge, never an error. Rejected: wswhiteboard reading the ticket tree
    itself (couples the render package to the store layout).
16. **Sticky summary, the later addition.** Decision (299) b of
    `261111-feat-ws-whiteboard-topic-recap` rejected a sticky recap but
    allowed adding it later; the owner's request (board (29), a promotion of
    this ticket, which specifies the sticky head) is that later addition.
17. **The dashboard resolver registers what it reads.** The dashboard reuses
    its board render until the stamp over the render's watch list changes
    (`mcp/dashboard_page.go` `renderBoardNow` `out.watch`,
    `wsdashboard/stream_cache.go` `StampWatched`); today only the ticket panel
    feeds it, and only `ready/` and `todo/` (`mcp/whiteboard_panels.go`). The
    dashboard resolver passes every status directory it consults (`idea/`,
    `todo/`, `ready/`, `.done/`, `.dropped/`) to the same `watch` callback,
    so a linked ticket's move refreshes its badge and glow; a test pins that
    a move invalidates the memoized render. The static file page shows status
    as of the last `whiteboard.update` write, which is acceptable for a static
    artifact. Rejected: polling the tree on every render.
18. **Small semantics.** Setting or clearing `tickets` is topic metadata like
    `label`: it neither moves the topic's recency nor reopens a closed topic
    (the existing label and cleared-content behavior), and emits no system
    event; a test pins it.
    The glow category is the stem's second segment; only `feat`, `bug`,
    `refactor`, and `chore` can glow, and the glow test covers a non-listed
    category. Route Facts rows that still say `.thead` read as `.tophead`.
19. **Details the mockup settles.** The check pill reads done/total like
    `.chead` and is absent for a topic with no check items; at rest the check
    list starts expanded, as today. A closed topic renders no `.tophead`
    stickiness; its latest agent line keeps the summary card style (Decision 8).
    `whiteboard.query`
    prints a topic's linked stems on its topic line as `tickets: <stem> ...`.
    Parenthesised board numbers in these decisions are provenance only; each
    decision's prose stands alone.

## Constraints

- Lands after `261113-feat-ws-whiteboard-topic-panels-and-log` and
  `261112-feat-ws-whiteboard-check-items` (done); the check list
  comes from it, shipped as `.checks` with a `.chead` progress line
  (`agents-plugin-tool/internal/wswhiteboard/assets/page.tmpl#L64-L69`,
  `.../assets/page.css#L300-L302`). No summary pill exists yet: the check pill
  of Decisions 5, 6, 8 is new work in this ticket (search for a check pill in
  `page.tmpl` and `page.css` returned nothing).
- The dashboard embeds the wswhiteboard assets; both the file page and the
  dashboard page get the head. The wrapper class is `.tophead` (Decision 12);
  dashboard.js and dashboard.css are not touched.
- Moving `.tbar` under `.tophead` takes every direct-child dependent with it
  (Decision 13): rail.js, page.css, cards.js, and the rail_offsets,
  topic_header, and recap tests; `.tnote` moves inside the head.
- Both hosts inject the stem-to-status resolver (Decision 15); wswhiteboard
  does not read the ticket tree itself.
- Read `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/release-compatibility.md`, and
  `ai-docs/manuals/wsflow-mirroring.md` (the whiteboard playbook documents
  the new attribute) before editing.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin-tool/internal/wswhiteboard/assets/page.css, agents-plugin/rsrc/whiteboard/whiteboard.md)
- Convention: ai-docs/manuals/release-compatibility.md (declared for agents-plugin-tool/internal/wswhiteboard/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/whiteboard/whiteboard.md)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/whiteboard/whiteboard.md)

## Prior Decisions

- dab009c4 (2026-11-13, commit): "renamed rather than scoping the composer rule (plan: the board must not depend on dashboard scoping)... TestCheckRowClassIsTheBoardsOwn pins that no board asset uses crow." — bearing: contradiction-candidate
- 261111-feat-ws-whiteboard-topic-recap (2026-11-11, Decisions): "Rejected: sticky under the topic header while a long topic scrolls ((299) b), which would stack on the dashboard's existing sticky layers; it can be added later." — bearing: constrains
- b4df8f6c (2026-11-11, commit): "cards.js topicAt now reads only `.tbar time.tago[data-at]`. Otherwise the recap's time could stand in for the topic's change time." — bearing: constrains
- 7f6a46ce (2026-11-10, commit): "Native position: sticky on the real .tbar (under a host class .wb-rail), not a cloned overlay: the stuck bar stays interactive (pen, chips)... .topic switches overflow hidden -> clip" — bearing: constrains
- 9034387e (2026-11-11, commit): "rail.js rails `section.topic[data-topic], [data-rail]`, and a marked node names its bar, capsule text, and capsule icon by data-rail-bar, data-rail-name, and data-rail-icon." — bearing: constrains
- d1361bb6 (2026-11-13, commit): "Embed size budget test still passes but is at 41.9% of its 42% cap (about 500 bytes of headroom); not raised here. A later embed asset needs to raise it or trim." — bearing: constrains
- 6e0e6928 (2026-11-12, commit): "Sticky unit: a `.thead` wrapper around bar + recap + check summary sticks at the bar's existing stick line. Rejected: separate sticky siblings with computed offsets (the `.tnote` pattern)." — bearing: supports
- 2cdf59ae (2026-11-12, commit): "Rejected: keeping a thin accent bar, since the head already has the dark bar as its anchor and the accent read as a second heading." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | wswhiteboard store.go, commands.go, query.go, render.go, assets/page.tmpl, page.css, rail.js; wsxmlcmd Group grammar; wsdashboard embed; rsrc whiteboard.md playbooks |
| scope.surface | cross-module | new wb:topic attribute (whiteboard.update), TopicDoc store field, wswhiteboard render plus wsdashboard page, shared rail.js and cards.js |
| scope.new_public_symbol | yes | the `tickets` attribute on wb:topic and a TopicDoc ticket-list field; exported names not yet fixed |
| scope.new_type_contract | yes | TopicDoc gains a stem-list field; a render-time stem-to-status resolver contract is needed and unspecified (wswhiteboard has no ticket-tree access today) |
| scope.test_surface | existing | agents-plugin-tool/internal/wswhiteboard/*_test.go (render_test.go, store_test.go, commands_test.go, query_test.go, rail_offsets_test.go, topic_header_test.go) |
| complexity.reuse_points | unconfirmed | rail.js stick-line and page.css sticky rules are reusable; no existing stem-to-status resolver found in wswhiteboard or wsdashboard |
| complexity.side_effect_risk | high | moving .tbar under .thead breaks direct-child tbar selectors in rail.js, page.css, cards.js; .thead collides with dashboard.css; embed budget guarded at 44 percent of 512 KiB |
| risk.correctness | moderate | sticky geometry, popover-on-stuck, and rail offsets are measured behavior pinned by rail_offsets_test.go |
| risk.fit | high | the planned .thead class already exists in dashboard.css and the board avoids dashboard-scoped classes (dab009c4) |
| risk.test | moderate | headless Chromium states on two pages plus the 44 percent embed budget test at render_test.go#L891-L899 |
| risk.security_or_contract | moderate | T0 optional attribute on a shared store shape and the Extra-only group grammar; stems render into tooltips so escaping must hold |

## Phases

### Phase 1: topic tickets, glow, and sticky head

- Store and command: optional `tickets` on `wb:topic` (Decision 14:
  `wsxmlcmd.Group` takes more than one extra attribute; `dropTopicDoc` keeps
  a topic with a non-empty ticket list); query output lists them; playbook
  text documents the attribute.
- Render: badges (Decisions 2, 3, 7), glow (Decision 4), status through the
  injected resolver supplied by both hosts (Decisions 9, 15), `.tophead`
  with note, latest agent line, and check pill (Decisions 5, 6, 8), and every
  direct-child `.tbar` dependent moved with the bar (Decision 13), light
  and dark.

Verification:
- Go tests in `internal/wswhiteboard`: the attribute round-trips through the
  store and query; badge status and glow follow the ticket states (live,
  all done, none); an unknown stem renders the muted "unknown" badge; a
  topic with only `tickets=` survives `dropTopicDoc`; a test pins that the
  board uses `.tophead` and no board asset uses `.thead` (like
  `TestCheckRowClassIsTheBoardsOwn`).
- Keep the embed size test meaningful at its current 44% of 512 KiB cap;
  trim new CSS/JS to fit, or state the reason for a changed cap in the commit.
- Headless Chromium screenshots matching the r7b mockup states (rest,
  stuck, stuck popover, narrow, glow) on the file page and the dashboard
  page; no page errors.
