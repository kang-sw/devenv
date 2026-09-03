---
title: "MCP read-surface collapse — fold proven-redundant list/find/status triples into query"
parent: 260903-epic-mcp-tool-surface-affordance-reduction
related:
  260903-refactor-mcp-verb-vocabulary-unification: sibling layer ④ — this collapse targets the `query` survivor that layer names; for tickets/specs rename and collapse coincide
---

# MCP read-surface collapse — fold proven-redundant list/find/status triples into query

Layer ③ of `260903-epic-mcp-tool-surface-affordance-reduction`. See the epic for
the canonical verb table and cost model; this ticket carries ③'s specifics.

## Scope

Collapse read-surface triples to a single `query` tool **where redundancy is
proven a clean superset** — not merely "looks similar".

Proven (verified in `server.go` tickets.find/status handlers): `tickets.{list,
find,status}` and `specs.{list,find,status}` all return the same
`TicketInfo`/spec-metadata struct via the same formatter, and
`find(ticket_stem=X)` already equals `status(X)` (same `Resolve:true`, identical
output). So:

- `query()` = old `list` (enumerate, no query).
- `query(ticket_stem=X)` = old `status` (point resolve; fixed param remap).
- `query(query=…)` = old `find`.

Result: **6 → 2** for tickets + specs. No semantic reconciliation — the survivor
already is the superset. The survivor is the `query`-named tool from layer ④.

## Audit-gated extension

`mental_models` exposes the same `list`/`find`/`status` triple. It is a
searchable-corpus family, so it must not be left as a half-rename (only
`find→query`). Add it to this ticket's audit: verify the triple is a clean
superset as tickets/specs were, then collapse to `mental_models.query`; if the
audit finds it is NOT a clean superset, layer ④ still aligns the verb and the
residual `list`/`status` are a noted explicit exception. Any other family with a
`list`/`find`/`status` corpus triple is treated the same way.

## Ordering / dependencies

- Runs **after** ④ (acts on the canonical `query` name).
- Boundary: collapse only proven-superset triples; unproven similarity is left
  or routed to a fresh audit, never merged here. Novel merged signatures belong
  to layer ② (`260903-refactor-mcp-todo-signature-merge`), not here.

## Open questions

- Cost check: is `TicketsStatus` a cheaper targeted lookup than
  `TicketsFind(ticket_stem)`? If a hot playbook path relies on the cheap
  point-lookup, confirm find-exact short-circuits (does not full-scan) before
  removing `status`.
- Deprecation posture (hard cut vs alias) for the removed `list`/`status` names.
- mental_models audit outcome (clean superset or noted exception).

## Spec Impact

Not applicable at `idea/`. Adoption will touch the tickets/specs listing +
resolution behavior contract in the MCP tools spec; scope at promotion.
