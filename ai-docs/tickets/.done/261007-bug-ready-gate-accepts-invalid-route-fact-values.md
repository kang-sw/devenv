---
title: "Ready promotion accepts Route Facts values the implement resolver rejects"
completed: 2026-11-13
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

Further dogfood on 2026-11-11 found `scope.test_surface | new` (allowed:
`none`, `existing`, `new-files`, `unknown`) and
`scope.new_public_symbol | likely` (allowed: `yes`, `no`, `unknown`) also
passed both promotion gates. The resolver treated all facts as unknown after
failure, risking a lite review for high-risk work. A ready ticket can thus
guarantee a worker stop (c), costing one worker spawn per occurrence.

## Decisions

The owner approved merging the duplicate reports and validating with the
implement resolver's existing parser at ready promotion. Return the offending
key and allowed values. Changing resolver aggregation or the fact-populator
playbook is outside this fix. This changes validation behavior only, not the
wire or store schema.

## Phases

### Phase 1: Validate Route Facts enums before a ticket lands in ready/

Validate the Route Facts table at the ready gate using the implement resolver's
parser, for both sage readiness and moves to ready. Refuse invalid facts with
the parser's key and allowed values, and verify valid values and existing
gate behavior remain supported.

### Result (2db9f416e) - 2026-11-13

Both gates now reject invalid values before writes using the shared implement
parser, identifying the key and allowed values. Existing missing-section and
non-implementation-category behavior is unchanged. Remote w5090 targeted
`wsdoc` and `mcp` tests passed; `go test ./...` passed except the known
`cmd/ws-mcp` LAN-change daemon replacement timeout.

Forward: Gate key formatting relies on the resolver error text; consider
structured validation errors if that format changes.
