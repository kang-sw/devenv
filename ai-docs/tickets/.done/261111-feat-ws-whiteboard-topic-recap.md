---
title: "Whiteboard topics carry an optional short recap with its own update time"
related:
  261110-feat-ws-whiteboard-item-field-html: "prerequisite: the recap renders by the item block-field rule that ticket introduces (sanitized HTML through sanitizeReply), (296)"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 0b644c193327ddc7
sage-review-completeness-reviewed: 0b644c193327ddc7
completed: 2026-11-11
---

# Whiteboard topic recap

## Background

User, 2026-11-11 whiteboard (topic `topic-recap`): a downstream lead
writes a one-line status at the top of a topic body by hand (for example
`READY · ticket X · reviewed, commit Y · in implementation: branch Z`), and
the user finds it very useful. They asked for an optional one- or two-line
recap field per topic, shown at the top of the topic, with its update time
shown separately.

Today a topic holds items and an optional body (`wb:body` / `wb:edit`,
`agents-plugin-tool/internal/wswhiteboard/store.go`, `TopicDoc.Body`, store.go#L198-L212), and the
topic header shows the topic's last change time. Closing a topic collapses
its settled items and body and shows the `result` sentence beside its name.

## Decisions

1. **A `wb:recap` command inside the `wb:topic` group sets the topic's
   recap ((295) ok).** Its body is the recap; an empty body clears it;
   sending the current text again is no change and does not move the
   recap's update time, so the shown time means the last real change.
   Rejected: a `wb:topic` attribute, since recap text easily holds quotes
   and code, and the body form matches `wb:body`.
2. **The recap's update time shows on the recap line ((297) ok).** It is a
   relative time ("12 min ago") on the right of the recap, the absolute time
   on hover, refreshed client-side while the page is open, in the same
   notation as the topic header's change time so the two compare at a
   glance.
3. **Placement: the top of the topic card, above the body, scrolling with
   the topic ((299) a).** Rejected: sticky under the topic header while a
   long topic scrolls ((299) b), which would stack on the dashboard's
   existing sticky layers; it can be added later.
4. **Closed topics, query, and embed ((298) ok).** Closing a topic folds the
   recap with its body, and the close `result` stands in its place.
   `whiteboard.query` prints the recap and its update time from depth
   `topics` up, as stored (like the topic body), and for a closed topic
   listed with `closed: true` as well, since the query reads state rather
   than the page fold. The embed snapshot always carries an open topic's
   recap and folds a closed topic's recap with its body.
5. **The playbook asks leads to keep a recap on in-flight topics ((300)
   b).** The whiteboard playbook says: give a topic whose state spans turns
   (in implementation, waiting on an answer, blocked) a recap, refresh it
   whenever that state changes, and keep it to one or two lines; a stale
   recap is worse than none. Rejected: only mentioning that the field exists
   ((300) a).
6. **The recap renders by the item option-field rule, with no hard length
   cap ((296) a; the user approved the board mockup).** The item block-field
   rule (`itemField`): block markdown plus
   HTML filtered by the reply sanitizer, as
   `261110-feat-ws-whiteboard-item-field-html` (merged 57dbcf247) gives
   option fields; the one-or-two-line size is the playbook's guidance
   (Decision 5), not a refusal. Rejected: inline markdown only with a refusal
   past two lines / about 240 characters, and the option rule plus a hard
   source-length refusal ((296) b), since a source-length cap does not
   measure what the reader sees once markup and images are allowed.

7. **A recap change is a topic change, like a body change.** A `wb:recap`
   that changes the stored recap bumps the topic's change time and recency
   (and so resets the stale-topic nudge), and on a closed topic it reopens
   the topic the way a body change does. An unchanged resend changes
   nothing (Decision 1). Rejected: a recap write that leaves recency alone or
   stays folded on a closed topic, since Decision 5 keeps recaps for
   in-flight topics, so a recap written to a closed topic means its work is
   in flight again, and one rule for body and recap is simpler to follow.
8. **Body and recap are independent parts of a topic's entry.** Clearing
   one keeps the other; the `TopicDoc` entry is dropped only when both are
   empty; a recap alone keeps a topic alive and can start one, as `wb:body`
   can. The store code that today treats an entry as the body follows:
   `clearTopicBody`, the same-batch revert in `setTopicBody` (it must not
   revert a `wb:recap` earlier in the batch), the `applyEdit` no-body
   refusal (decided by an empty body, not a missing entry), `topicKnown`,
   `pruneLabels`, the `applyClose` refusal text, query `BodyLen`, and the
   `Topics` doc comment. The recap's update time has its own change mark and
   time (for example `RecapChanged` and `RecapAt`), set only when the recap
   text really changes and stamped in the same post-Apply place as
   `ChangedAt`, so a later body-only change does not move it. Rejected: a
   separate recap map beside `Topics`, which would duplicate the topic
   lifecycle (close, prune, rename) for one field. (Design review, applied
   by the lead.)

## Constraints

- The recap renders through the same pipeline and sanitizer as item block
  fields from `261110-feat-ws-whiteboard-item-field-html` (merged), including
  its `contain: paint` clipping.
- The whiteboard playbook has three byte-identical copies
  (`agents-plugin/rsrc/whiteboard/whiteboard.md`,
  `agents-plugin-wsflow/rsrc/whiteboard/whiteboard.md`,
  `agents-plugin-pi/rsrc/whiteboard/whiteboard.md`), each hashed in its
  package's `rsrc/manifest.json`; the commands table, the topic-body
  section, and the query section gain the recap.
- Quiet dashboard tone: `--wb-*` tokens.
- The recap time may reuse `time.tago[data-at]` (ago.js refresh), but
  cards.js `topicAt` reads the first `time.tago[data-at]` in a topic section
  as the topic's last update: keep the recap's element after the header, or
  narrow that selector to `.tbar`.
- `wb:recap` joins the `wb:topic` group row of the playbook commands table
  and the group's accepted-command list in commands.go, besides its own row.
- `261111-bug-ws-whiteboard-title-patch-reask` (merged f13fd5ae2) also
  edited the three whiteboard.md copies: start from develop and regenerate
  the three manifest hashes from the final text.
- Manuals: `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/skill-authoring.md`, `ai-docs/manuals/wsflow-mirroring.md`,
  `ai-docs/manuals/ws-mcp.md`.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/skill-authoring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin-wsflow/rsrc/`, `agents-plugin-wsflow/skills/`, `agents-plugin-tool/internal/wsdoc/conventions/`)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin/hooks/`, `agents-plugin-wsflow/`)

## Prior Decisions

- 261110-feat-ws-whiteboard-item-field-html (2026-11-11, Result): "`itemField` (markdown.go) renders option, rationale, and recommendation text through the raw pipeline and `sanitizeReply`... `.opt, .ctx, .rec-line { contain: paint; }` (D4)." — bearing: supports
- 29feba49 (2026-11-11, commit): "PlainHTML is a new function rather than widening Plain: titles, links, labels, and answers render tags as text on the page" — bearing: constrains
- 261107-feat-ws-whiteboard-xml-commands-topic-content (2026-11-07, Result): "A body change bumps topic recency and reopens a closed topic; the same body is no change; `wb:edit` on a topic without a body is refused" — bearing: constrains
- 1a7e75cf (2026-11-08, commit): "Added ChangedAt (RFC 3339, omitempty) to Item and TopicDoc, stamped in one place after Apply succeeds (every entry whose Changed == this batch's mark) instead of at each mutation site" — bearing: constrains
- 40e8f438 (2026-11-08, commit): "Plan: ai-docs/.plans/2026-11/08-1909-whiteboard-topic-header.md (user decisions 478-483)." — bearing: supports
- cca49a60 (2026-11-10, commit): "the topic body keeps its `topic body:` + <<< >>> markers after the topic line so wb:edit quoting is unchanged." — bearing: constrains
- 29feba49 (2026-11-11, commit): "The rsrc manifest hash and the wsflow and Pi rsrc mirrors were synced by copy, since remote builds do not return regen outputs." — bearing: supports
- 261111-bug-ws-whiteboard-title-patch-reask (2026-11-11, Decisions): "A patch that really changes an item's title is a re-ask, the same as a class or options change: it stamps `ReaskedAt`" — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | wswhiteboard/store.go, commands.go, query.go, render.go, view.go, assets/page.tmpl, page.css, wsdashboard/assets/dashboard.js, three rsrc/whiteboard/whiteboard.md copies and manifests |
| scope.surface | public-interface | new wb:recap command in whiteboard.update input and recap lines in whiteboard.query output |
| scope.new_public_symbol | yes | wb:recap command; recap field on TopicDoc |
| scope.new_type_contract | yes | TopicDoc gains recap text and recap update time persisted in whiteboard.json |
| scope.test_surface | existing | wswhiteboard/store_test.go, query_test.go, render_test.go, topic_header_test.go |
| complexity.reuse_points | confirmed | itemField and sanitizeReply in wswhiteboard/markdown.go and sanitize.go; ago.js relative time; applyBody pattern in store.go |
| complexity.side_effect_risk | moderate | persisted session state shape and dashboard board morph change |
| risk.correctness | moderate | unchanged-resend must not move the recap time while ChangedAt is stamped batch-wide after Apply |
| risk.fit | moderate | must align with topic recency, close fold, and reopen rules of the existing body |
| risk.test | moderate | needs a browser check on the dashboard beyond Go tests |
| risk.security_or_contract | moderate | user-visible HTML through the reply sanitizer and a new command in the shipped playbook |

## Phases

### Phase 1: topic recap

- Store: a recap and its update time per topic, set and cleared by
  `wb:recap`; refusal and validation text names the command.
- Rendering: the recap line on the file page, the dashboard board, and the
  embed snapshot (Decisions 2-4); the dashboard's board morph handles a
  recap change like a body change.
- `whiteboard.query`: the recap and its time at depth `topics` and above.
- Playbook: the command row and the recap guidance (Decision 5) in all
  three copies.

Verification:

- Store tests: set, replace, clear, an unchanged resend that keeps both the
  recap time and the topic's `ChangedAt`; a recap change that bumps the
  topic's change time and recency; a recap written to a closed topic that
  reopens it (Decision 7); a topic that already has a recap and is then
  closed folds the recap with its body (Decision 4); clearing the body with
  a recap present keeps the recap and the topic (and the reverse); a
  `wb:recap` in the same batch as a body revert survives; a later body-only
  change leaves the recap time alone (Decision 8).
- Render tests for the recap line and its time on the page and in the
  embed; query tests at depth `topics`, including a closed topic with
  `closed: true`.
- A browser check on the dashboard: a recap with inline markup and an image
  at the top of a topic, its relative time, and the closed-topic fold.

### Result (b4df8f6cc) - 2026-11-11

Landed `wb:recap`, a Leaf command in the `wb:topic` group. It is stored raw
and trimmed. An empty or blank recap clears it, and resending the same recap
is no change.

- **Store and timing.** The body and the recap are independent parts of the
  topic entry, each with its own change mark and time
  (`Changed`/`ChangedAt` for the body, `RecapChanged`/`RecapAt` for the
  recap). `TopicDoc.Recency()` takes the larger of the two. Page order, the
  topic's update time, query `last change`, and the stale-topic nudge all use
  it.
- **Changes versus clears.** Changing a recap reopens a closed topic.
  Clearing a recap or a body neither reopens the topic nor bumps its recency.
  That mirrors the existing body clear, so Decision 7 applies to set and
  replace only.
- **Batch revert.** A body set back to its starting text within one batch
  restores only the body's fields, so a recap earlier in that batch still
  stands.
- **Page.** The recap line sits at the top of the card, rendered through
  itemField (sanitized block markdown) with `contain: paint`. Its time is on
  the right in the topic-bar notation: relative on the full page, local HH:MM
  on the embed, with the absolute local time as the hover title. A closed
  topic folds the recap into the `tdoc` fold above the body, and a closed
  topic that has only a recap gets a fold of its own. `cards.js` `topicAt`
  now reads `.tbar time.tago[data-at]` only.
- **Query.** It prints `recap (changed: <time>): ...` after the topic line
  at every depth, including for closed topics listed with `closed: true`.
- **Playbook.** All three `whiteboard.md` copies gain the command row, the
  group lists, a recap bullet with the keep-it-current guidance, and the
  close/query wording. The copies are byte-identical, and each manifest
  carries the sha256.

Verification:
- New `recap_test.go` tests cover the store, render, regions, and query
  behaviour, and the existing tests were updated for the new close-refusal
  text and command list.
- Run remotely through ws-rgo, all ok: `go test` for wswhiteboard,
  wsdashboard, wsrsrc, mcp, and wsi18n, and `go vet` for wswhiteboard. The
  wsflow package tests (20) are ok, and `node --check cards.js` passes.
- Headless chrome-headless-shell renders checked the whiteboard page itself:
  full layout, embed, dark mode, and the closed fold opened. They show a
  recap with bold, code, an image, a second line, and relative times.
- Not done: a check on the live dashboard. A local Go build is not allowed,
  and the installed binary predates this change. The dashboard board region
  uses the same templates and CSS (`RenderRegions`), and the regions test
  pins the recap's position outside the body frame.
- Lite review: no Critical or Important findings. Minor findings left
  unfixed: a same-batch recap round-trip on a closed topic still reopens it
  (the body already behaves the same way), the `topicAt` selector change has
  no JS test, and there was no dashboard-specific browser render.
