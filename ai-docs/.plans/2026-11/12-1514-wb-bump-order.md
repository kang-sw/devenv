# Topics bumped by the same whiteboard update order by why they were bumped

## Goal

User request 2026-11-12, board item (423)=a: topics still order by latest
activity (52d28419d, `agents-plugin-tool/internal/wswhiteboard/view.go`
~478-489: `activity[topic]` = max of item `Changed` and topic
`Recency()`), but topics that rise in the same update (same activity mark)
should order by the reason they rose, instead of first-appearance order:

1. a new policy question (an added policy item, or a re-ask that leaves a
   policy item open: patch to title/class/options, or back to open)
2. a new acknowledgement item (technical / announced added or re-asked)
3. a topic body or recap change
4. anything else (answers, closes, withdrawals, edits that are not re-asks)

## Decisions

- Recency stays the primary key: a topic touched by a later update still
  sits above every topic of an earlier update (the 11-10 rule). The reason
  rank only breaks ties among topics whose latest activity is the same mark.
- The rank is the strongest reason the topic had at its latest mark (a
  topic that got both a new policy item and a body edit in that update
  ranks 1). Ties within the same rank keep the current stable
  first-appearance order.
- Derive the reason from what the store already records per item and per
  topic (item class/status/`Changed`, re-ask stamp, body/recap marks). If
  the store cannot tell a re-ask or "added in this mark" apart from other
  item changes, add the smallest optional field needed (for example the mark
  at which an item was last asked) - a new optional JSON field with
  omitempty is T0; never change an existing field's meaning.
- Closed-topic placement and ClosedCap are unchanged. The dashboard's
  morphBoard follows server order, so no asset change is expected.
- Compatibility tier T0.

## Scope and verification

- `internal/wswhiteboard` (`view.go`, `store.go` only if a field is needed).
- Tests next to the existing ordering tests (search
  `TestOpenPolicyItemDoesNotOutrankRecentTopic`): within one update, a
  policy-add topic outranks an ack-add topic, which outranks a body/recap
  topic, which outranks an answered-only topic; a re-ask counts as its
  class's add; a later update still outranks an earlier one regardless of
  reason; same-rank ties keep first-appearance order.

## Constraints

- Heavy Go test runs only through
  `~/.local/bin/ws-rgo -C agents-plugin-tool -m 8 -- test -count=1 ./internal/wswhiteboard/... ./internal/wsdashboard/...`;
  never run go build/test/vet locally.
- Commit on branch `delegate/wb-bump-order` in this worktree; do not merge
  or push.
