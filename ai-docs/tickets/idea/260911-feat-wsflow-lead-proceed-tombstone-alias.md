---
title: "wsflow-only: re-add lead-proceed as a tombstone alias that routes to lead-run"
related:
  260909-refactor-lead-surface-collapse-worker-stop-protocol: the collapse that retired lead-proceed into lead-run and pinned the retired stems unresolvable; this re-adds lead-proceed in wsflow ONLY as a deprecation alias — a deliberate, documented carve-out of that pin, never touching the flagship ws surface
  260728-research-mcp-json-surface-removal-deprecation: sibling class — a surface removed with no deprecation path for existing callers; this is the skill-name-surface answer, scoped to the wsflow derivative
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

## Proposal (wsflow-only)

Re-add a `lead-proceed` skill shim in wsflow whose body is a pointer to the
`lead-run` playbook. wsflow skills are thin `playbook.read` shims, so the alias
resolves and runs the current `lead-run` procedure with no forked or copied
text — it always executes whatever `lead-run` currently is (zero procedure
drift).

The shim's *description* is the tombstone, doing double duty:
- a user who calls `lead-proceed` by name is gracefully routed onto the run
  path rather than blocked (no re-typing);
- an agent auto-selecting a skill is attention-steered toward `lead-run` by a
  description that marks `lead-proceed` retired and names `lead-run` canonical.

The flagship `ws` package is untouched — its retired stem stays unresolvable —
keeping ws at exactly five lead skills.

## What this actually touches

- **Deliberate ws↔wsflow divergence.** A wsflow skill with no ws counterpart.
  `ai-docs/manuals/wsflow-mirroring.md`, the skill-shim drift test, and the
  wsflow skill-inventory sweep (`agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py`,
  `agents-plugin/tests/test_skill_dispatch_contracts.py`) must be taught to
  allow this one carve-out; otherwise drift/inventory assertions fail. The
  surface-pinned invariant gains a documented wsflow exception, scoped to this
  alias.
- **rsrc-level invariant preserved.** No new `lead-proceed` *rsrc* body is
  created — the shim points at the `lead-run` rsrc — so
  `TestRetiredLeadPlaybooksNoLongerResolve` stays green. Only the wsflow *skill*
  inventory gains the alias.
- **Pointer, not copy.** The alias must resolve `lead-run`'s live body, never a
  snapshot, so the two never drift.

## Open points

- Description wording that both works-if-called and steers-to-`lead-run`,
  without reading as "you cannot call this" (which would confuse an agent that
  did land on it).
- Whether the alias stays wsflow-exclusive by policy, or is ever promoted to ws.
- Whether to generalize to a retired-stem -> successor alias table (reusable
  migration infrastructure if more collapses are expected) versus a one-off
  `lead-proceed` shim.
