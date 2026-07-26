---
title: The 🚧 marker prerequisite is stated as a filesystem invariant that no contract-first path can satisfy
related:
  260726-bug-sage-ready-enforcement-single-chokepoint: same architectural move — mutation surfaces permissive, commit gate owns the invariant
  260723-epic-ticket-write-reshape: establishes the verify-as-mechanical-floor boundary this ticket's enforcement point belongs to
  260726-feat-verify-ticket-graph-advisories: cross-file reference resolution at verify is the natural host for a commit-state check
  260723-feat-ready-spec-address-hard-gate: adjacent ready spec-address hardening; that ticket is about the gate's strength, this one about the 🚧 prerequisite
sage-review-design: required
---

# The 🚧 marker prerequisite is stated as a filesystem invariant that no contract-first path can satisfy

## Background

Reported downstream (wsflow 0.36.1) and re-verified against source. Three rules
form a cycle with no documented resolution order:

- `spec-conventions`: "`🚧` entries for implementation behavior require a
  non-`epic`, non-`research`, non-`workset` `ready/` ticket."
- `ticket-conventions`: a non-`epic`/`research`/`workset` ticket entering
  `ready/` requires spec addressing.
- `lead-write-ticket` → **On: Spec-address Check** step 3: when `judge:
  contract-first-spec` is yes, execute `lead-write-spec` and list the resulting
  stem in `spec:` — i.e. produce a `🚧` entry.

The cycle is closed by `lead-write-ticket` → **On: Move** step 4: "Deferred
`todo/` → `ready/` promotion: move only after **Spec-address Check** passes."
So at the moment the `🚧` is written, the ticket is provably still in `todo/`
and the prerequisite provably does not hold.

`lead-write-spec` makes this visible without resolving it: `judge:
contract-first-spec` emits "Session reminder: implementation `🚧` entries require
a non-`epic`, non-`research`, non-`workset` `ready/` ticket" immediately after
writing the marker — restating an unsatisfiable precondition.

The escape hatch (`## Spec Impact` as the default ready-addressing path) does not
cover the contract-first case. Downstream, the existing spec text actively
contradicted the ticket's central decision (a schema spec said "writers must
preserve the meaning of existing indices" while the ticket repurposed an index
under owner authorization). A ticket-local `## Spec Impact` cannot resolve a
contradiction that lives in the spec corpus — that is precisely what
contract-first exists for.

Each execution currently rediscovers an ad-hoc ordering; downstream needed a
human decision to break the loop.

## Decisions

- **Restate the prerequisite as a commit-state invariant, not a filesystem-state
  invariant.** The requirement becomes: the commit that introduces an
  implementation `🚧` entry must also land its backing ticket in `ready/`.
  Intermediate working-tree states are not violations.
- **Same principle as `260726-bug-sage-ready-enforcement-single-chokepoint`.**
  Mutation-time surfaces stay permissive; the commit boundary owns the invariant.
  Write the principle once in the concept/convention layer and have both tickets
  reference it rather than restating it twice.
- **Enforcement belongs at verify, not in prose.** A prose-only ordering rule is
  the failure mode `260723` was created to stop. `tickets.verify` /
  `ws/git.commit` already own mechanical guardrails, and
  `260726-feat-verify-ticket-graph-advisories` is extending exactly that surface
  to cross-file reference resolution.

## Constraints

- Do not weaken what `🚧` means. The marker exists so planned behavior is visible
  and stable before implementation; this ticket changes *when the prerequisite is
  evaluated*, not whether it holds.
- The `## Spec Impact` default path stays the default. This ticket only makes the
  contract-first branch executable.

## Spec Impact

- Target spec area: the bundled `spec-conventions` and `ticket-conventions`
  documents, plus whichever verify guardrail hosts the check.
- Expected caller-visible change: authors following the contract-first branch get
  a stated ordering instead of an unsatisfiable precondition; a commit that adds
  an implementation `🚧` without landing its ticket in `ready/` is reported by
  verify.
- Contract-first spec: no. The exact guardrail wording and its FAIL/WARN
  classification should be settled during implementation against the existing
  verify finding set.

## Phases

### Phase 1: Restate the prerequisite and host the check at verify

- Rewrite the `spec-conventions` `🚧` prerequisite as a commit-state invariant,
  and state the resolution order explicitly in `lead-write-ticket`'s Spec-address
  Check: the ticket's `ready/` move and the spec `🚧` land in the same logical
  commit.
- Drop or rewrite `lead-write-spec`'s "Session reminder" so it stops restating a
  precondition that cannot hold at that point in the procedure.
- Add the corresponding check to the verify guardrail set, coordinating with
  `260726-feat-verify-ticket-graph-advisories` on whether it is a new guardrail
  or an extension of that ticket's cross-file reference resolution.

Rejected alternatives: leaving the ordering to per-execution discovery (the
current state; costs a human decision each time); allowing `🚧` without a backing
ready ticket (dissolves the marker's meaning); making `## Spec Impact` cover
contract-first cases (cannot resolve contradictions living in the spec corpus).

Verification boundary: a contract-first ticket promoted to `ready/` with a new
`🚧` entry completes through the rendered playbook without a hand-authored
ordering decision, and a commit that adds an implementation `🚧` with its ticket
left in `todo/` is reported by verify.
