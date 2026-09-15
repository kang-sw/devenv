---
title: "Ticket fact population emits out-of-enum scope.new_public_symbol value (likely)"
---

# Ticket fact population emits out-of-enum `scope.new_public_symbol` value

## Observed

During a parallel `ws:lead-run` batch (2026-09-15), two independently
authored `ready/` tickets both carried a `## Route Facts` row
`scope.new_public_symbol | likely`. The route parser accepts only
`yes` / `no` / `unknown` for this fact, so `route.resolve_implement`
rejected the entire facts table and every fact fell back to `unknown`
("route facts missing (unreadable)"). Both workers correctly stopped
`stop: c` at Execute step 1 rather than re-deriving facts.

Affected tickets (fixed inline on their impl branches, `likely` -> `yes`):
- `260914-feat-ws-pi-agent-widget-recursive-gutter-and-state-bullets`
- `260914-feat-ws-pi-mailbox-native-steer-push`

## Why it matters

`likely` is a plausible natural-language hedge a fact-populating author
(human or `ticket-fact-populator`) reaches for when a new symbol is
intended but its name is unfixed. Because it silently invalidates the
whole table (not just one cell), it turns a ready ticket into a hard
worker stop, wasting a dispatch — and in a parallel batch it wasted two.

## Candidate follow-ups (pick during triage)

- Make `ticket-fact-populator` / the ticket-authoring path constrain
  `scope.new_public_symbol` to the `yes/no/unknown` enum at authoring
  time (reject/normalize `likely` and similar hedges).
- Consider whether the route parser should fail *per-cell* (flag the one
  bad cell) instead of collapsing the entire table to `unknown`, so a
  single typo does not erase valid facts.
- Consider a ticket-authoring lint / `tickets.verify` check for
  out-of-enum Route Facts values before a ticket reaches `ready/`.

## Notes

Captured under the "Dogfood surprises get captured" discipline; no
design commitment implied. Route-parser vs. authoring-side enforcement
is the open triage question.
