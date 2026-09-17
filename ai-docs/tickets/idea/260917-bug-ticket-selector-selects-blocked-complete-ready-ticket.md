---
title: ticket-selector selects a Blocked/all-phases-complete ready ticket, wasting drain cycles
related:
  260914-chore-ws-pi-root-manifest-runtime-deps: the owner-blocked, all-phases-complete ready ticket the selector mis-picked
  260914-chore-ws-pi-release-path-acceptance-and-docs: the second owner-blocked ready ticket in the same queue
---

# ticket-selector selects a Blocked/all-phases-complete ready ticket, wasting drain cycles

## Background

During a `ws:lead-run` goal-run drain (2026-09-17), the `ticket-selector`
subagent (small tier / haiku) selected
`260914-chore-ws-pi-root-manifest-runtime-deps` — a ticket that

1. carries an explicit `## Blocked (2026-09-16)` note stating "no agent (worker
   or lead) can perform it ... `ws:lead-run` cannot advance this ticket
   further" (owner-gated on a clean-machine `pi install` smoke), and
2. has **both** phases already closed with `### Result` sections (fully
   implemented and merged to `develop`), sitting in `ready/` only because the
   owner smoke has not been recorded.

The selector's own reasoning cited "both phases complete" as a *reason to
select* it. The lead only avoided dispatching a no-op worker by reading the
ticket body and finding the `## Blocked` note manually.

## Observations

Two distinct defects combine here.

### 1. The `## Blocked` skip is specified but was not honored

`agents-plugin/rsrc/ticket-selector/ticket-selector.md` step 3 already says:
"Skip candidates carrying a `## Blocked (...)` note". The rule exists; the
small-tier model did not apply it. Selection correctness depends on a haiku-tier
agent reliably parsing a prose note, which is fragile. A deterministic signal
(see directions) would remove the dependence on model judgment for a
safety-relevant skip.

### 2. No guard for an all-phases-`### Result` ticket stuck in `ready/`

The same step's "in-progress" preference is defined as "some phase has a
`### Result`, at least one does not". A ticket whose phases *all* have
`### Result` matches neither the in-progress rule nor any skip rule, so it falls
through to the "then the oldest" tiebreaker and remains selectable — and the
model here read completeness as a positive signal. A ticket that is fully
implemented but cannot leave `ready/` (owner-gated, awaiting an out-of-band
sign-off) has no agent-advanceable work and should be skipped or deprioritized,
independent of whether a `## Blocked` note happens to be present.

## Impact

- Wasted drain cycles: the lead must read the full body and re-select, or (worse)
  dispatch a worker that finds nothing to do.
- In a goal loop, a permanently owner-blocked ticket can be re-selected every
  cycle, so the loop never converges on the terminal condition without lead
  intervention. Both owner-blocked `260914` tickets in this queue exhibit the
  hazard.

## Possible directions (not decided)

- **Deterministic advanceability signal.** Have `ws/tickets.query` expose a
  machine-readable flag the selector can trust without prose parsing — e.g.
  `blocked_note` presence and an `all_phases_complete` / `advanceable` rollup
  (today the JSON exposes `phases[].result_present` and omits `unresolved_phases`
  when complete, but exposes no `## Blocked` note flag). Then move the skip from
  model judgment toward a hard filter (mirroring how `dispatch_blocked` is the
  hard gate for prerequisites).
- **Explicit all-complete skip rule** in the selector prompt: a candidate whose
  every phase has a `### Result` is not in-progress work; skip or rank it last,
  and never treat completeness as a selection reason.
- **Consider the terminal interaction:** a ready ticket that is fully complete
  but owner-gated arguably wants a distinct disposition (a nudge to close/move,
  or a `.done`-pending status) rather than lingering as a selectable `ready/`
  entry.

## Prior Art / surface

- `agents-plugin/rsrc/ticket-selector/ticket-selector.md` (step 3 selection
  ordering) is the primary surface; `ticket-batch-selector.md` shares the shape.
- Shipped rsrc with mirrors: `agents-plugin/`, `agents-plugin-wsflow/`,
  `agents-plugin-pi/`. Any fix reads `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/skill-authoring.md`, and `ai-docs/manuals/wsflow-mirroring.md`,
  and keeps the three copies in sync.
- A query-side signal would touch `agents-plugin-tool/internal/mcp/` (read
  `ai-docs/manuals/ws-mcp.md`).
