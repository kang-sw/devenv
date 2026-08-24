---
title: Review policy config + release gate — AGENTS.md review-track, host-neutral gating, devenv ship gate
parent: 260824-epic-review-watermark-model
related:
  260824-feat-lead-review-range-scenario: prerequisite — the gate reviews a range through this scenario
  260824-feat-review-watermark-ledger: prerequisite — the gate reviews the unreviewed range up to the marker
sage-review-design: completed
sage-review-completeness: completed
---

# Review policy config + release gate — AGENTS.md review-track, host-neutral gating, devenv ship gate

## Background

The mechanism (range review + marker + sweep) is host-neutral; whether and where
review *blocks* is per-project policy. This ticket adds the policy surface and
the one mandatory gate — the release boundary — for projects that declare one.
devenv is such a project (`develop`→`main` ship); a messy downstream with no
release boundary declares none and gets advisory-only review. Depends on the
range scenario (②) and the marker/ledger (③).

Circled numbers denote the epic's sibling children: ② =
`260824-feat-lead-review-range-scenario`, ③ =
`260824-feat-review-watermark-ledger`, ④ = this ticket
(`260824-feat-review-release-gate-policy`) (see `related:`).

## Decisions

Settled at the epic level; restated as constraints:

- **config split (three homes):** `AGENTS.md` (tracked, per-track) declares the
  review-track branch and whether a release boundary exists; the `ai-docs/`
  ledger holds marker+verdict state; `_review.local.md` holds machine-local
  review mechanics. Do not put the review-track branch in `_review.local.md`
  (it is a shared structural fact, and local config is gitignored) or the marker
  in `AGENTS.md` (churn).
- **Host-neutral first:** never encode devenv's `develop`/`main`/ship shape as
  the mechanism. Gating is opt-in; absence of a declared boundary means
  advisory-only, not "no review."
- **workflow_manual discovery:** when the review-track branch is unset,
  `workflow_manual` surfaces a **non-blocking** "configure this first" nudge at
  most **once per session** (session-scoped, not per-checkpoint), not a hard
  block.
- **Fallback:** a boundary project with no marker yet can review `main..develop`
  directly (the release branch is itself a natural "reviewed-up-to" proxy) until
  the marker exists.
- **Finding resolution (epic Cross-Child):** the gate resolves a blocking finding
  by *routed-ticket status*, not an inline fix or a ledger flag. It blocks while a
  recorded blocking finding's routed ticket is open, and clears on that ticket's
  `.done` (or a recorded lead waiver). See Phase 2 gate verdict.

## Phases

### Phase 1: Policy config surface (AGENTS.md + _review.local.md) + workflow_manual nudge

- Define the `AGENTS.md` fields: review-track branch, release-boundary
  declaration (present/absent). Define the `_review.local.md` review-mechanics
  home (already exists; note the split so nothing double-owns).
- `workflow_manual` scans for the review-track config and emits a non-blocking,
  scoped nudge when unset.

Verification: a repo with the field set exposes the review-track to the sweep;
an unset repo gets exactly one scoped nudge, never a block; the three config
homes have no overlapping ownership.

### Phase 2: Mandatory release gate (devenv ship)

- For a project that declares a release boundary, insert a **mandatory** range
  review into the promotion path. For devenv: into `lead-ship` pre-flight
  (`ai-docs/ship/ws.md` / `lead-ship` playbook), which today has no review step
  — review the unreviewed range (marker..HEAD, or `main..develop` fallback)
  before `develop`→`main`.
- **Gate verdict (concrete, per the epic's finding-resolution decision):**
  promotion is blocked when **either** (a) the just-run range review raises a
  blocking finding, **or** (b) a blocking finding recorded in the ledger since the
  last release references a routed ticket that is still open (not `.done`) — in
  both cases absent an explicit lead waiver. The gate does not read a ledger
  "resolved" flag (there is none); it cross-references recorded blocking entries
  against their routed tickets' status. A blocking finding is thus cleared by
  landing its fix (the routed ticket closes) or by a recorded lead waiver, never
  by editing the ledger.
- Downstream without a declared boundary: no gate inserted; advisory-only.
- Depends on Phase 1, ②, and ③.

Verification: on devenv, ship pre-flight refuses to promote when the range
review raises a blocking finding, and also when a ledger blocking entry since the
last release points at a still-open routed ticket; it proceeds when the range is
clean, when every such routed ticket is `.done`, or on a recorded lead waiver; a
no-boundary project's ship/promotion path is unchanged.

## Spec Impact

Target: `ai-docs/spec/workflow-skills.md` (lead-ship gains a pre-flight range
review for boundary projects; the review-track/boundary config contract) and the
`ai-docs/ship/ws.md` config. New caller-visible behavior: a mandatory review
gate at the declared release boundary (devenv ship), host-neutral advisory-only
elsewhere; `AGENTS.md` review-track/boundary fields and the `workflow_manual`
nudge.
