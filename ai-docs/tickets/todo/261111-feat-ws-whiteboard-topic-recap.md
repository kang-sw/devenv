---
title: "Whiteboard topics carry an optional short recap with its own update time"
related:
  261110-feat-ws-whiteboard-item-field-html: "prerequisite: the recap renders by the item block-field rule that ticket introduces (sanitized HTML through sanitizeReply), (296)"
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
`agents-plugin-tool/internal/wswhiteboard/store.go`, `Topic.Body`), and the
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
   `topics` up. The embed snapshot always carries an open topic's recap.
5. **The playbook asks leads to keep a recap on in-flight topics ((300)
   b).** The whiteboard playbook says: give a topic whose state spans turns
   (in implementation, waiting on an answer, blocked) a recap, refresh it
   whenever that state changes, and keep it to one or two lines; a stale
   recap is worse than none. Rejected: only mentioning that the field exists
   ((300) a).

## Open Decision Queue
Last ID: (300)

(296) [open] [policy] The recap renders by the same rule as item option fields (block markdown plus HTML filtered by the reply sanitizer, `261110-feat-ws-whiteboard-item-field-html`), and the hard limit of two lines / about 240 characters is dropped in favour of the playbook's one-or-two-line guidance (Decision 5).
- Context: the user answered (296) "the same rule as the options (merging soon)" against a proposal of inline markdown only plus a refusal past two lines / 240 characters. The lead's reading, recorded as assumed on the board: the format follows the options, and since images and HTML then fit in a recap, a character or line refusal no longer measures what the reader sees.
- a: the reading above.
- b: the option rendering rule, plus keeping a hard source-length refusal.
> a: a source-length cap is a poor measure once markup and images are allowed; the playbook guidance carries the intent.

## Constraints

- The recap renders through the same pipeline and sanitizer as item block
  fields once `261110-feat-ws-whiteboard-item-field-html` lands, including
  its `contain: paint` clipping.
- The whiteboard playbook has three byte-identical copies
  (`agents-plugin/rsrc/whiteboard/whiteboard.md`,
  `agents-plugin-wsflow/rsrc/whiteboard/whiteboard.md`,
  `agents-plugin-pi/rsrc/whiteboard/whiteboard.md`), each hashed in its
  package's `rsrc/manifest.json`; the commands table, the topic-body
  section, and the query section gain the recap.
- Quiet dashboard tone: `--wb-*` tokens.
- Manuals: `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/skill-authoring.md`, `ai-docs/manuals/wsflow-mirroring.md`,
  `ai-docs/manuals/ws-mcp.md`.

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

- Store tests: set, replace, clear, an unchanged resend that keeps the
  update time, and a recap on a closed topic folding with the body.
- Render tests for the recap line and its time on the page and in the
  embed; a query test at depth `topics`.
- A browser check on the dashboard: a recap with inline markup and an image
  at the top of a topic, its relative time, and the closed-topic fold.
