---
title: "MCP read-surface collapse — fold proven-redundant list/find/status triples into query"
parent: 260903-epic-mcp-tool-surface-affordance-reduction
related:
  260903-refactor-mcp-verb-vocabulary-unification: sibling layer ④ — this collapse targets the `query` survivor that layer names; for tickets/specs rename and collapse coincide
sage-review-design: completed
sage-review-design-reviewed: e6c549134d110f98
sage-review-completeness: completed
sage-review-completeness-reviewed: e6c549134d110f98
---

# MCP read-surface collapse — fold proven-redundant list/find/status triples into query

## Background

Layer ③ of `260903-epic-mcp-tool-surface-affordance-reduction`. Collapse
read-surface triples to a single `query` tool **where redundancy is proven a
clean superset** — not merely "looks similar". See the epic for the canonical
verb table and cost model; this ticket carries ③'s audited specifics.

## Decisions — audit outcomes

### tickets + specs: collapse (proven clean superset)

Verified in `agents-plugin-tool/internal/mcp/server.go` +
`internal/wsdoc/tickets.go`: `tickets.{list,find,status}` and
`specs.{list,find,status}` all return the same `TicketInfo`/spec-metadata struct
via the same formatter, and `find(ticket_stem=X)` already equals `status(X)`
(same `Resolve:true`, identical output). So the `query`-named survivor (from ④)
absorbs all three call shapes:

- `query()` = old `list` (enumerate, no query).
- `query(ticket_stem=X)` = old `status` (point resolve; fixed param remap).
- `query(query=…)` = old `find`.

Result: **6 → 2** for tickets + specs. No semantic reconciliation — the survivor
already is the superset.

**Cost check resolved — removing `status` is cost-neutral.** `tickets.status` is
NOT a cheaper targeted lookup: `TicketsStatus`→`scanTickets` and
`TicketsFind`→`scanTicketsWithBodies` both walk the full board and `readTicket`
every `.md` file; the exact stem is never used to skip the walk. No hot
point-lookup path is regressed by removing `status`. (Incidental finding, not
required by this ticket: `find(ticket_stem=X)` currently reads on-disk bodies
twice — the stem-filter `continue` sits after the body read; moving it ahead is a
trivial optional cleanup the collapse can fold in.)

### mental_models: NOT a clean superset → verb-align only (noted exception)

Audit result: `mental_models.{list,find,status}` do **not** collapse cleanly,
for two independent reasons:

- `mental_models.list` is a divergent legacy implementation — its own private
  struct and pre-formatted output via `MentalModelsList` (own `WalkDir`, header +
  `sources:` layout, **no JSON path**), not `scanMentalModels`/
  `formatMentalModels`. So `find()` is a data superset but is **not
  output-identical** to today's `list`.
- `mental_models.find` has no `path` argument, while `status` resolves one doc by
  `path`; they overlap only on `domain`. `status(path=X)` has no `find`
  equivalent.

So ③ does **not** collapse mental_models. Per the epic's corpus-triple rule, ④
still aligns the verb (`find→query`) and `mental_models.list`/`status` are left as
an explicit, noted exception. A genuine `mental_models.query` merge would require
re-pointing `list` at `scanMentalModels`/`formatMentalModels` (an output-format +
JSON-support change) and adding the `path` key to the merged tool — that is
semantic reconciliation (novel signature), out of ③'s clean-collapse scope;
captured as a separate follow-up rather than forced here.

## Ordering / dependencies

- Runs **after** ④ (acts on the canonical `query` name).
- Boundary: collapse only proven-superset triples; unproven similarity is left or
  routed to a fresh audit, never merged here. Novel merged signatures belong to
  layer ② (`260903-refactor-mcp-todo-signature-merge`) or a dedicated follow-up,
  not here.
- Deprecation posture: one-shot hard cut inherited from the epic — no alias for
  the removed `list`/`status` names; the rename/collapse removes them in-package.

## Phases

### Phase 1: Collapse tickets and specs read triples into query

Remove `tickets.list`, `tickets.status`, `specs.list`, `specs.status`; the
④-renamed `tickets.query`/`specs.query` survivor serves all three prior call
shapes (enumerate / point-resolve by stem / search). Apply the fixed param remap
for old `status` call sites (`tickets.status(X)` → `tickets.query(ticket_stem=X)`;
`specs.status(X)` → `specs.query(spec_stem=X)`), and update Go registration +
dispatch, `runtime.json`, the MCP/workflow specs, playbook token call sites (and
the wsflow mirror — regenerated via the mirror generator in
`agents-plugin-tool/internal/wsrsrc/`, not hand-edited), and tests. `mental_models`
is **not** collapsed in this phase (audit: not a clean superset — verb-aligned by
④ only); this phase's spec pass records that noted exception in the mental-model
discovery anchor (below).

Acceptance: the two survivor tools return byte-identical output to the removed
tools for every prior call shape; the removed names are gone in-package (a
`references.trace` / grep sweep is clean); the full Go test suite is green
(`go test ./...` in `agents-plugin-tool/`); and playbook paths that called
`status` now call `query(ticket_stem)`/`query(spec_stem)` with unchanged results.

### Result

Collapsed tickets + specs read triples into the `query` survivor in commit
`bd335c7f` (fix `d3b2248b`): removed `tickets.list`/`tickets.status`/`specs.list`/
`specs.status` from Go registration + dispatch, both `runtime.json` `tools`
sections, `mcp-tools.md`/`workflow-skills.md`/`documentation-system.md`, nine
`agents-plugin/rsrc/**` playbook token remaps + regenerated wsflow mirror, and the
two test files. `mental_models` left un-collapsed with the noted-exception spec
text pointing at `260904-research-mental-models-query-reconciliation`. CLI
subcommand surface, `runtime.json` `"commands"`, and the shared `wsdoc`
functions (still used by `references.trace` + the legacy-marker resolver) left
intact.

**Design correction (recorded):** the ticket's Decision claim that
`query(ticket_stem=X)` was *already* a byte-identical superset of `status(X)` did
NOT hold as layer ④ left it — the ④-renamed `query` point-resolve shape produced
find-style output (array / empty-on-missing), not status-style (single object /
error-on-missing). The collapse therefore routes the point-resolve shape to reuse
the existing `TicketsStatus`/`SpecsStatus` + their formatters, giving genuine
byte-identical output (object JSON + exact `ticket not found: %s` /
`spec anchor not found: %s` errors). No new tool signature; the only in-package
callers of `query(ticket_stem/spec_stem)` were tests, so no live caller relied on
the old find-semantics. Two pinning tests were added to lock the byte-identical
shape.

Verification: `go test ./... -count=1` in `agents-plugin-tool/` green across 13
packages (cache-disabled — an earlier cached run masked a Critical); wsflow
Python suite 10/10; grep sweep clean. Review: partitioned correctness + fit —
fit clean; correctness caught one **Critical** (stale
`agents-plugin/skills/manifest.json` after the in-diff `AGENTS.template.md` edit,
failing `TestSkillsManifestDriftIsVisible`) which was fixed by the
`WSRSRC_REGEN_SKILLS` regen (`d3b2248b`) and re-review confirmed **[resolved]**;
one Minor (optional text-mode assertion for the tickets pinning test) recorded,
not fixed.

## Spec Impact

Edits to existing anchors only — no new spec stem, no heading `{#slug}` change:

- `mcp-tools.md` `{#260505-ticket-discovery-tools}` and
  `{#260505-spec-discovery-tools}` — remove the `list`/`status` tool entries and
  fold their documented behavior into the `query` survivor's contract
  (enumerate / point-resolve / search call shapes).
- `mcp-tools.md` `{#260505-mental-model-discovery-tools}` — record the noted
  exception: `mental_models.query` (verb-aligned) coexists with the un-collapsed
  `list`/`status`, with a pointer to the reconciliation follow-up.
