---
title: Relax ready-promotion to dependency-closure + add bulk ready promotion
spec:
  - workflow-skills
related-mental-model:
  - workflow-skills
sage-review-design: completed
sage-review-completeness: completed
completed: 2026-08-20
---

# Relax ready-promotion to dependency-closure + add bulk ready promotion

## Background

`judge: initial-status` refused `ready/` whenever the earliest unfinished phase
block-depended on a ticket not yet **landed** (`.done/`), single-sourcing the
"immediately implementable" bar. That bar was filed under new-ticket
initial-status but reads as a general `ready/` invariant, so agents applied it
inconsistently at `todo/` -> `ready/` promotion — the intermittent "cannot
promote, dependency unlanded" refusals. Meanwhile the consumers already assume
dependents can coexist in `ready/`: `lead-drain-ready-queue` prefers a
`related:`/`parent:` prerequisite that is itself in `ready/`, and
`lead-goal-fan-out-step` orders dependent tickets serially. The producer gate
contradicted the consumers.

This relaxes the gate to a **closed-set** model and adds an explicit
bulk-promotion path so a dependency chain can be promoted together, in order.

## Decisions

- `ready/` means a **closed work front**: a ticket lands in `ready/` only when
  the tickets its **earliest unfinished phase** block-depends on are in
  `ready/`, `.done/`, or the same bulk-promotion action — not "every ready
  ticket is independently startable in isolation". The earliest-phase scoping
  from {#260729-write-ticket-unlanded-dependency-status} is preserved: a later
  phase's dependency does not block the landing.
- Closure is evaluated over `{existing ready/}` union `{this bulk action's set}`.
  Ordered promotion (prerequisites first) makes each dependent's gate pass
  against an already-promoted prerequisite, so no speculative "will-be-promoted"
  state is needed.
- Dependencies must be machine-readable `related: <stem>: prerequisite` /
  prerequisite `parent:` frontmatter edges — the only signal the drain selector
  reads for ordering; a dependency named only in phase prose does not satisfy
  the gate. An epic-hierarchy `parent:` is membership, not a blocking
  dependency: epics never land in `ready/`/`.done/`, so treating a bare
  hierarchy `parent:` as a closure edge would deadlock every child; only a
  prerequisite edge to a landable ticket counts.
- The gate is single-sourced as `## On: Dependency Closure Check`, run at every
  `ready/` landing (new creation, single promotion, bulk).
- Bulk promotion lives in its own `## On: Bulk Ready Promotion` handler rather
  than overloaded onto Cascade Edit (whose target selection is
  decision-propagation-scoped): validate closure, topologically order, run each
  ticket's Spec-address + Sage Review gates, commit the ordered set as one
  logical unit, and on a mid-run block commit the promoted prefix and report.
- Latent bug fixed in passing: Cascade Edit promoted targets to `ready/` running
  only Spec-address Check, skipping the Sage Review Gate; it now runs the Sage
  gate per ready-entering target.
- Rejected: open-set (allow a dependency in `todo/`/`idea/` when "order is
  stated") — a `ready/`-only drain cannot pull an unpromoted dependency, so the
  set would stall mid-drain; closure must bound to `ready/`/`.done/`/bulk-set.

## Phases

### Phase 1: Dependency-closure gate + bulk ready promotion

Skill prose — already drafted inline by the lead, uncommitted at authoring time:

- `agents-plugin/rsrc/lead-write-ticket/lead-write-ticket.md`: `judge:
  initial-status` closed-set form; new `## On: Dependency Closure Check`; new
  `## On: Bulk Ready Promotion`; new `judge: bulk-ready-promotion`; Cascade Edit
  sage-gap fix; Invariants / Route / Verify / Move cross-references.
- `agents-plugin/rsrc/lead-workflow-manual/lead-workflow-manual.md`: `ready/`
  status-directory definition redefined to closed-set semantics
  (earliest-phase scoped).

Remaining implementation:

- Update spec `workflow-skills`
  {#260729-write-ticket-unlanded-dependency-status} to the closed-set gate, the
  `related:`/`parent:` machine-readable requirement, the Dependency Closure
  Check single-source, the Bulk Ready Promotion handler, and the Cascade Edit
  sage-gate fix.
- Sync where the old rule is restated: `ticket-conventions` convention, the
  `workflow-skills`/`documentation-system` mental-model docs, and
  `agents-plugin/skills/lead-bootstrap/WORKFLOW.md` (and its wsflow twin) if it
  states the old ready/ meaning.
- Regenerate the wsflow rsrc byte-identical mirror and the rsrc manifest, and
  update any drift-guard test that pins the changed playbook text (candidate:
  `agents-plugin-tool/internal/mcp/playbook_tools_test.go`).

Verification:

- `WSRSRC_REGEN=1 go test ./internal/wsrsrc/... -count=1 -run
  TestGenerateRealManifest` then `WS_REGEN_WSFLOW_RSRC=1 go test
  ./internal/wsrsrc -count=1 -run TestRegenerateWsflowRsrcMirror`, then
  `go test ./internal/wsrsrc -count=1` shows mirror/manifest guards green.
- `python3 -m unittest discover agents-plugin-wsflow/tests` green (wsflow shim +
  runtime contract).
- The `feat` template already carries `related: <stem>: prerequisite` notation,
  so no template change is required.

### Result (a88b75fb) - 2026-08-20

Landed the closed-set ready gate across playbook, spec, and mental model.

- `lead-write-ticket`: `judge: initial-status` closed-set form; new single-sourced
  `## On: Dependency Closure Check`; new `## On: Bulk Ready Promotion` +
  `judge: bulk-ready-promotion`; Cascade Edit now runs the Sage Review Gate per
  `ready/`-entering target. `lead-workflow-manual`: `ready/` redefined as a closed
  work front draining in dependency order.
- Regenerated the wsflow byte-identical rsrc mirror and both rsrc manifests.
- Spec `{#260729-write-ticket-unlanded-dependency-status}` rewritten to the
  closed-set gate + bulk handler + Cascade sage-gate fix (anchor stem preserved,
  epic Planned-reference paragraph kept). Mental-model entry added for the
  closed-work-front invariant and the epic-hierarchy-`parent:` deadlock trap.

Deviations from the plan's sync list:

- Only the spec `{#260729}` actually stated the old unlanded bar. `ticket-conventions`,
  the `documentation-system` spec/mental-model, and `WORKFLOW.md` state "`ready/` =
  spec-addressed implementation target" without the unlanded-dependency rule, so
  they were left unchanged (surgical scope) rather than reworded.
- No drift-guard test pinned the changed playbook prose, so no test fixup was
  needed beyond the regenerated manifest/mirror.

Verification: `go test ./internal/wsrsrc -count=1`, `./internal/mcp`, `./internal/wsdoc`
all green; `go build ./...` clean; `python3 -m unittest discover agents-plugin-wsflow/tests`
green (10 tests).

## Spec Impact

- `workflow-skills` {#260729-write-ticket-unlanded-dependency-status}: rewrite
  the `judge: initial-status` unlanded-dependency rule to the closed-set
  Dependency Closure Check (`ready/`/`.done/`/bulk-set, earliest-phase scoped,
  `related:`/`parent:` machine-readable), and document the Bulk Ready Promotion
  handler and the Cascade Edit Sage-gate fix. No new ticket-system state field.
