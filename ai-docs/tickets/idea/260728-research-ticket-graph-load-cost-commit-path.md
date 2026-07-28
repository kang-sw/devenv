---
title: ticket-graph load now runs a full-archive scan on every ticket-touching commit
related:
  260726-feat-verify-ticket-graph-advisories: introduced loadTicketGraph and the whole-board scan this ticket questions the cost of
---

# ticket-graph load now runs a full-archive scan on every ticket-touching commit

## Background

`agents-plugin-tool/internal/wsdoc/tickets_graph.go:79-96` and
`tickets_verify.go:90-96`: every `TicketVerify` call — which means every
`ws/git.commit` that touches any ticket path — now calls `loadTicketGraph`,
which runs `scanTickets(IncludeDone+IncludeDropped)` plus `scanSpecs`. That is a
full read of every ticket file in the repository, not just the paths being
verified.

On `main` this cost was zero: `TicketVerify` only read the caller-supplied
paths. The graph load is new surface from
`260726-feat-verify-ticket-graph-advisories`, which added cross-file
`parent:`/`related:` advisories and needed the whole board to resolve them
against.

## Measured 2026-07-28

On the live tree:

- 462 ticket files total (332 in `.done/`, 41 in `.dropped/`).
- Cold single-path `tickets.verify`: 103ms.
- 50 repeated single-path `tickets.verify` calls: 853ms total, ~17ms/call once
  warm.

Not slow in absolute terms, and not a correctness bug at current scale — the
archive is small enough that nobody has noticed. The shape of the cost is what
is worth flagging: it is O(size of the entire archive), on a path that fires on
every commit, and the archive only grows. `.done/` and `.dropped/` are
append-only by construction (tickets move in, nothing prunes them), so this
call's cost trends upward monotonically with the project's age, not with the
size of the change being committed.

## Why this is worth deciding rather than just fixing

A hotfix would need to pick a specific mitigation — cache the graph across
calls within a session, scope the scan to open tickets plus only the referenced
closed ones, index the archive once and incrementally update it — and each of
those trades away something `260726-feat-verify-ticket-graph-advisories`
relied on (freshness, simplicity, or the "closed tickets can still be
`related:` targets" case the advisories check). Picking one without checking
what that ticket's design actually needs the whole board for risks solving the
wrong problem. The right first step is deciding whether an O(archive) scan on
the commit hot path is acceptable at all, and at what archive size it stops
being acceptable, before choosing a specific fix.

## A related gap: graph-load failure is silent

Separately, worth carrying into whatever ticket acts on this: `TicketVerify`
swallows a `loadTicketGraph` error and proceeds as if the graph were absent
(`err != nil` is not surfaced). That is the right fail-safe *direction* — a
broken graph should not block an otherwise-valid commit — but it also means a
degraded or partially-loaded graph produces no signal to the caller at all. If
a mitigation changes how the graph loads (caching, incremental indexing), it
should not make this silent-failure mode worse, and ideally should give it a
visible advisory.

## Non-Scope

- Does not propose a specific caching or scoping strategy. That is an
  implementation decision downstream of settling the cost-acceptability
  question above.
