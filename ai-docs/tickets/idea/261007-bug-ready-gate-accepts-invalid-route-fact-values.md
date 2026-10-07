---
title: "Ready promotion accepts Route Facts values the implement resolver rejects"
related:
  261007-bug-pi-child-start-fails-after-version-bump: dogfood discovery
---

# Ready promotion accepts Route Facts values the implement resolver rejects

## Background

Dogfood, 2026-10-07. The lead hand-edited `## Route Facts` in
`261007-bug-pi-child-start-fails-after-version-bump` while folding reviewer
findings, writing `scope.new_public_symbol: internal`,
`scope.new_type_contract: internal`, and `complexity.reuse_points: existing`.
`tickets.sage_gate` (landing `ready`), `tickets.sage_stamp`, `tickets.move`
to `ready/`, and `tickets.query` all accepted and projected those values. The
first consumer to validate them was the worker's `route.resolve_implement`,
which rejected the table as unreadable (`implement_resolver.go`
`parseEnumFact`: `yes|no|unknown`, `confirmed|unconfirmed|not-applicable|unknown`)
and stopped the run before any edit. It also reports only the first bad
value, so the other two surfaced only from reading the source.

A ticket can therefore reach `ready/` with Route Facts that guarantee a worker
stop (c), costing one worker spawn per occurrence.

## Phases

### Phase 1: Validate Route Facts enums before a ticket lands in ready/

Candidate direction, not settled: share the resolver's enum tables with the
ready-landing gate (`sage_gate` or `tickets.move`) so an invalid value is
refused with every offending fact listed, and have `route.resolve_implement`
report all invalid facts at once.
