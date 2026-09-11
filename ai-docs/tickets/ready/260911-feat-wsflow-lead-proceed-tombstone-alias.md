---
title: "wsflow-only: re-add lead-proceed as a tombstone alias that routes to lead-run"
related:
  260909-refactor-lead-surface-collapse-worker-stop-protocol: the collapse that retired lead-proceed into lead-run and pinned the retired stems unresolvable; this re-adds lead-proceed in wsflow ONLY as a deprecation alias — a deliberate, documented carve-out of that pin, never touching the flagship ws surface
  260728-research-mcp-json-surface-removal-deprecation: sibling class — a surface removed with no deprecation path for existing callers; this is the skill-name-surface answer, scoped to the wsflow derivative
sage-review-completeness: completed
sage-review-design: completed
sage-review-design-reviewed: 07231d4e71fe7e35
sage-review-completeness-reviewed: 07231d4e71fe7e35
---

# wsflow-only: lead-proceed as a tombstone alias routing to lead-run

## Background

The lead-surface collapse (`260909-refactor-lead-surface-collapse-worker-stop-protocol`,
commit `ce5f30ef`) retired `lead-proceed` into `lead-run` and pinned the retired
stems unresolvable (`TestRetiredLeadPlaybooksNoLongerResolve`) so the lead
surface cannot silently re-expand. The retirement is documented in CHANGELOG,
but that notice reaches changelog readers, not a user who — carrying muscle
memory from a previously-shipped binary — invokes the old `lead-proceed` name
in-session and hits a bare "no longer resolves" failure with no pointer to the
successor.

`ws` is the flagship agent-first package whose whole point is the collapsed
five-skill surface; `agents-plugin-wsflow` is the conservative agentless
derivative. A backward-compat alias fits the derivative's role without muddying
the flagship surface, so this affordance is deliberately wsflow-exclusive.

## Decisions

- **D1 — wsflow-exclusive.** The alias lives only in `agents-plugin-wsflow`; the
  flagship `ws` package keeps its retired stem unresolvable and stays at exactly
  five lead skills. Rejected: adding the alias to `ws` as well — it would muddy
  the agent-first surface the collapse exists to keep clean; compat belongs in
  the conservative derivative.
- **D2 — Pointer, not copy.** The wsflow `lead-proceed` shim points at the
  `lead-run` playbook (wsflow skills are thin `playbook.read` shims), so calling
  it runs the current `lead-run` procedure with no forked text. Rejected: copying
  `lead-run`'s body under the `lead-proceed` name — it would drift from `lead-run`
  on every future edit.
- **D3 — The description is the tombstone, doing double duty.** The shim's
  description marks `lead-proceed` retired and names `lead-run` canonical, so a
  by-name caller is gracefully routed onto the run path (not blocked) while an
  auto-selecting agent is steered toward `lead-run`. Rejected: a bare
  redirect-notice that refuses to run — it forces the name-caller to re-type and
  loses the graceful degrade that is the point.
- **D4 — One-off shim, not a generalized alias table.** Scope is the single
  `lead-proceed` alias. Deferred: a generic retired-stem -> successor redirect
  table (reusable migration infrastructure) — unrequested scope; revisit only if
  more collapses need it.
- **D5 — The rsrc-level unresolvable pin is preserved.** No new `lead-proceed`
  rsrc body is created, so `TestRetiredLeadPlaybooksNoLongerResolve` stays green.
  Only the wsflow *skill* inventory gains the alias, and only the wsflow-side
  drift and inventory guards are taught the carve-out.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (the alias ships in the
  wsflow package surface).
- Convention: ai-docs/manuals/skill-authoring.md (a new skill shim plus its
  description).
- Convention: ai-docs/manuals/wsflow-mirroring.md (this is a deliberate
  ws<->wsflow divergence; the mirroring rules and drift guard must record and
  assert the carve-out rather than flag it).

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-wsflow/skills/lead-proceed (new), ai-docs/manuals/wsflow-mirroring.md, agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py, agents-plugin/tests/test_skill_dispatch_contracts.py |
| scope.surface | public-interface | a new caller-invocable skill name, lead-proceed, added to the wsflow skill inventory |
| scope.new_public_symbol | yes | the skill name lead-proceed, newly resolvable in agents-plugin-wsflow/skills/ |
| scope.new_type_contract | no | none; a SKILL.md shim with no new function, type, or tool signature |
| scope.test_surface | existing | agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py, agents-plugin/tests/test_skill_dispatch_contracts.py |
| complexity.reuse_points | confirmed | the playbook.read shim pattern already used by every wsflow skill, e.g. agents-plugin-wsflow/skills/lead-run/SKILL.md#L1-L14 |
| complexity.side_effect_risk | moderate | edits two existing guard/inventory tests' allow-lists to admit one wsflow-only skill with no ws counterpart, which is exactly the symmetry those guards otherwise enforce |
| risk.correctness | low | verify step is concrete: named tests pass, lead-proceed resolves in wsflow and runs lead-run, ws still refuses lead-proceed (pinned by TestRetiredLeadPlaybooksNoLongerResolve, agents-plugin-tool/internal/mcp/playbook_tools_test.go#L2248-L2262, which checks only agents-plugin/rsrc and is untouched) |
| risk.fit | low | follows the established thin-shim pattern (D2) and the collapse's own already-completed, done predecessor ticket |
| risk.test | low | the three verify assertions are named and existing tests already exercise the inventory sets this change extends |
| risk.security_or_contract | low | no security surface; the retired-stem-unresolvable contract is explicitly preserved for ws (D5) and only wsflow gains additive behavior |

## Phases

### Phase 1: wsflow lead-proceed tombstone alias

Add a `lead-proceed` skill shim to `agents-plugin-wsflow` (SKILL.md plus
whatever the package's shim layout requires) whose body invokes the `lead-run`
playbook via `playbook.read`, and whose description marks it a retired alias of
`lead-run` — steering auto-selectors to `lead-run` while still executing when
called by name. Teach `ai-docs/manuals/wsflow-mirroring.md` and the wsflow
drift / skill-inventory guards
(`agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py`,
`agents-plugin/tests/test_skill_dispatch_contracts.py`) to allow this one
wsflow-only skill with no `ws` counterpart, so the divergence is recorded and
asserted rather than tripping a drift failure. Leave the flagship `ws` package
and the rsrc-level retirement pin untouched.

Verify: the wsflow bundle tests and the dispatch-contract name sweep pass;
`lead-proceed` resolves in wsflow and runs the `lead-run` procedure; `ws` still
refuses `lead-proceed`.

Open point for execution: the exact description wording that both works-if-called
and steers-to-`lead-run` without reading as "you cannot call this" (which would
confuse an agent that did land on it).
