# wb-topic-recency-order

## Intent

User directive (2026-11-10 whiteboard): the whiteboard stops floating topics
with open questions to the top. Topics order simply by latest update, newest
first: the most recently updated topic rises to the top of the topic list,
below whatever the page keeps fixed above the topics.

## Scope

- `agents-plugin-tool/internal/wswhiteboard/view.go`: the topic sort
  (`sort.SliceStable(order, ...)` using `rank` then `activity`). Order open
  topics by `activity` alone (latest item or body change, newest first;
  equal activity keeps first-appearance order). Remove `rank` from ordering;
  keep `visible`/`Quiet`, `BulkAck`, `Answerable`, and the unread-body
  logic only where something other than ordering still reads it (delete
  what becomes dead).
- Unchanged: closed topics with nothing waiting still go last, most
  recently closed first, under the `ClosedCap` window; a closed topic that
  still has something waiting stays among the shown topics (now placed by
  activity).
- Update comments and tests (`view_test.go` or wherever ordering is
  asserted) to the new rule; add a test that a topic with an open policy
  item does not outrank a more recently changed topic with none.
- Check whether any shipped text (whiteboard playbook under
  `agents-plugin*/rsrc/`, `wsdoc` conventions, tool descriptions) states the
  old "waiting topics first" ordering; if so, update it and mirror per
  `ai-docs/manuals/wsflow-mirroring.md`. The manual's whiteboard section
  says closed topics with nothing waiting go last; that stays true.
- Client: confirm `dashboard.js` morphBoard just follows server order and
  needs no change; do not touch topic-note code (~L770-810, topicNotes).

## Verification

Remote only: `~/.local/bin/ws-rgo -C agents-plugin-tool -m 15 -- test ./internal/wswhiteboard/ ./internal/wsdashboard/`
(never run heavy go build/test locally).

## Boundary

Commit on branch `feat/wb-topic-recency-order` in this worktree. Return to
the lead if the change needs anything beyond ordering (for example a new
store field).
