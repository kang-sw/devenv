---
title: "Anchor the design-review contradiction check on the ready/ inventory plus the parent epic, dropping the model-authored related: marker"
---

# Anchor the design-review contradiction check on the ready/ inventory plus the parent epic, dropping the model-authored related: marker

## Background

The sage design reviewer's only cross-ticket check is checklist item 5 in
`agents-plugin/rsrc/ticket-reviewer-design/ticket-reviewer-design.md` (and its
wsflow mirror): "Does the ticket's planned behavior contradict a cross-child
invariant its `parent:` epic states, or a constraint a ticket in `related:`
frontmatter states?" So today the contradiction anchor is the reviewed
ticket's `parent:` epic plus whatever tickets its own `related:` frontmatter
names.

The `related:` half is unreliable as a contradiction anchor because the marker
is authored by a model at ticket-write time: which tickets it lists — and
therefore what the contradiction check compares against — depends on model
behavior, and a stale, mis-scoped, or soft (`todo/`/`idea/`) related entry can
pull an inconsistent or unstable anchor into a `ready/`-landing review with
unpredictable side effects.

The invariant actually worth enforcing is status-based, not marker-based:
every ticket sitting in `ready/` must be mutually consistent, because `ready/`
is the implementation-ready queue a worker draws from one at a time. Design
review already runs precisely at the `ready/` landing (the only
`sage_gate(landing: "ready")` call site, `lead-ticket.md:89`), which is exactly
the moment to check the incoming ticket against the queue it is joining.

## Decisions

- **Contradiction anchor becomes `ready/` inventory + parent epic.** The
  reviewer checks the ticket under review for contradiction against (a) every
  other ticket currently in `ready/`, and (b) its `parent:` epic regardless of
  the epic's own status (`todo/`/`idea/`/`.done/`). The condition being
  enforced is "all `ready/` tickets are mutually consistent," plus the epic's
  cross-child invariants.
- **`related:` is dropped as an independent anchor.** A ticket named in
  `related:` is compared only if it is itself in `ready/`, in which case (a)
  already covers it. This removes the dependency on the model-authored marker
  without losing any real `ready/`-internal coverage.
- **Scope stays bounded to `ready/` + the named parent.** The reviewer does not
  scan `todo/`, `idea/`, or the whole tree — only the `ready/` inventory and
  the one named epic, to keep the read bounded.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for
  agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for
  agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/rsrc/,
  agents-plugin-wsflow/skills/, agents-plugin-tool/internal/wsdoc/conventions/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for
  agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/)

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/rsrc/ticket-reviewer-design/ticket-reviewer-design.md and its byte-identical mirror agents-plugin-wsflow/rsrc/ticket-reviewer-design/ticket-reviewer-design.md (diff empty); lead-ticket.md at both trees may also need edits depending on Phase 1's open implementation point |
| scope.surface | internal | reviewer prompt/checklist text (ticket-reviewer-design.md:55-57, 66); no exported code symbol touched |
| scope.new_public_symbol | no | none |
| scope.new_type_contract | no | reviewer Output contract (ticket-reviewer-design.md:74-96, verdict/issues shape) is unchanged by this ticket |
| scope.test_surface | none | no test in agents-plugin/tests or agents-plugin-wsflow/tests references ticket-reviewer-design.md or checklist item 5 (search returned nothing) |
| complexity.reuse_points | confirmed | tickets.query already supports a ready-scoped listing (statuses: ["ready"]); a sibling delegate playbook, ticket-fact-populator.md, already calls tickets.query, showing the generic-listing pattern this ticket's shipped-surface-boundary constraint requires is already in use |
| complexity.side_effect_risk | high | checklist item 5 and its critical severity row gate sage_gate(landing: "ready") at lead-ticket.md:89, confirmed the only such call site, so a wrong rewrite affects every future ready/ promotion |
| risk.correctness | moderate | Phase 1's own open implementation point (reviewer self-enumerates ready/ vs. lead hands a spawn-time digest) is unresolved, and either choice also requires rewriting the reader-scope Constraints line at ticket-reviewer-design.md:22-24, which Phase 1 does not name directly (it says only "Update the reviewer playbook's Inputs/Constraints accordingly") |
| risk.fit | low | change stays inside the one reviewer-playbook pair already covering this checklist item, following the existing shipped-surface-boundary/wsflow-mirroring conventions this ticket itself cites |
| risk.test | moderate | no automated test exercises this checklist item today (search returned nothing), so an incorrect rewrite has no CI backstop, only manual dogfood review |
| risk.security_or_contract | low | prompt-text-only change; the reviewer's read-only/no-mutation constraint (ticket-reviewer-design.md:21) is untouched by this ticket |

## Phases

### Phase 1: Re-anchor item 5 on the ready/ inventory

Rewrite checklist item 5 (and its `critical` severity-table row) in
`ticket-reviewer-design.md` so the contradiction check anchors on the current
`ready/` inventory plus the reviewed ticket's `parent:` epic, and no longer
reads `related:` as an independent anchor. Keep the single-ticket coherence
checks (duct-tape detection, right-problem framing) unchanged. Mirror the edit
to `agents-plugin-wsflow/rsrc/ticket-reviewer-design/`.

Open implementation point to resolve during execution: the reviewer runs as a
delegate; confirm whether the reviewer role may enumerate `ready/` itself (e.g.
`tickets.query` scoped to the ready status) or whether the lead must hand the
reviewer a `ready/` digest at spawn time. Prefer the reviewer reading the
inventory directly if the role allows it; fall back to a spawn-time digest only
if it does not. Update the reviewer playbook's Inputs/Constraints accordingly.
