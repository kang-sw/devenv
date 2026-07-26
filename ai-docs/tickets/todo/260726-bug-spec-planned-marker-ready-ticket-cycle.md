---
title: The 🚧 marker prerequisite is stated as a filesystem invariant that no contract-first path can satisfy
related:
  260726-bug-sage-ready-enforcement-single-chokepoint: same architectural move — mutation surfaces permissive, commit gate owns the invariant
  260723-epic-ticket-write-reshape: establishes the verify-as-mechanical-floor boundary this ticket's enforcement point belongs to
  260726-feat-verify-ticket-graph-advisories: cross-file reference resolution at verify is the natural host for a commit-state check
  260723-feat-ready-spec-address-hard-gate: adjacent ready spec-address hardening; that ticket is about the gate's strength, this one about the 🚧 prerequisite
sage-review-design: blocked
sage-review-completeness: blocked
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
  `ws/git.commit` already own mechanical guardrails.
- **Inline playbook invocation does not own its commit — the caller does.** This
  is the block-level finding and the ticket's real centre of gravity.
  `lead-write-spec` step 7 is an unconditional `ws/git.commit(paths: ["<file>"])`,
  and `lead-write-ticket`'s contract-first branch invokes `lead-write-spec`
  *inline*. So on the exact path this ticket exists to make executable, the `🚧`
  lands in a spec-only commit before the ticket ever moves — the same-commit
  invariant is violated by construction, not by accident. The fix is to make
  commit ownership explicit: a playbook invoked inline by another playbook
  returns its changed paths and does not commit; the outermost invocation
  commits. This is not a new rule so much as an unstated one — `lead-write-ticket`'s
  Commit step already says "separate follow-up invocations own their own commits
  and outputs", whose contrapositive is exactly this, and `lead-write-spec`
  simply never honoured it.
- **This is a new guardrail, not an extension of the graph-advisories ticket.**
  `260726-feat-verify-ticket-graph-advisories` (ready/, both stages completed)
  confines itself to frontmatter and status directories and puts body-reading
  checks in its Out of Scope, so a `🚧` check — which reads spec body prose —
  cannot ride it. Design it separately; borrow only its principle.
- **Severity: advisory, not a blocking finding.** Adopt that ticket's stated
  principle, "severity follows reversibility, not defect gravity" — a commit is
  reversible, so the check hands back the remedy rather than blocking the land.
  Do not re-derive this during implementation.

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
- Contract-first spec: no. The behavioral direction is stated here; the exact
  advisory wording settles during implementation. The FAIL/WARN question is
  already decided in `## Decisions` (advisory, per the reversibility principle)
  and is no longer deferred.

## Phases

### Phase 1: Restate the prerequisite and host the check at verify

- **Transfer commit ownership on inline invocation.** Make `lead-write-spec`
  step 7 conditional: when invoked inline by another playbook, return the changed
  spec path instead of committing, and let the caller stage both the spec and the
  ticket in one commit. State the rule where both playbooks can see it, not only
  in `lead-write-spec`. Without this the rest of the phase cannot hold.
- Rewrite the `spec-conventions` `🚧` prerequisite as a commit-state invariant,
  and state the resolution order explicitly in `lead-write-ticket`'s Spec-address
  Check: the ticket's `ready/` move and the spec `🚧` land in the same commit.
- Drop or rewrite `lead-write-spec`'s "Session reminder" so it stops restating a
  precondition that cannot hold at that point in the procedure.
- **Widen verify's subject set.** `TicketVerify(root, paths)` silently skips
  every non-ticket-shaped path, so a spec-only commit currently yields zero
  subjects and the violation is invisible. The check needs `ai-docs/spec/` paths
  admitted as subjects and spec bodies read for `🚧` markers. This is scope, not
  a detail — without it the guardrail cannot observe the case it exists for.
- Emit the check as a non-blocking advisory naming the offending spec stem and
  the ticket that should have moved.
- **Bump the plugin version.** `spec-conventions.md` and `ticket-conventions.md`
  live under `agents-plugin-tool/internal/wsdoc/conventions/` and are `go:embed`'d
  into `ws-mcp`; plugin-cache invalidation keys on the version string, so an
  unbumped build serves the old prerequisite text to installed plugins.

Rejected alternatives: leaving the ordering to per-execution discovery (the
current state; costs a human decision each time); allowing `🚧` without a backing
ready ticket (dissolves the marker's meaning); making `## Spec Impact` cover
contract-first cases (cannot resolve contradictions living in the spec corpus);
extending `260726-feat-verify-ticket-graph-advisories` instead of designing a new
guardrail (its settled scope excludes body-reading checks); restating the
invariant loosely enough for the current procedure to satisfy without touching
`lead-write-spec`'s commit (leaves the marker's guarantee unverifiable).

Verification boundary:

1. `lead-write-spec` invoked inline from the contract-first branch produces no
   commit of its own; the resulting single commit contains both the spec `🚧` and
   the ticket in `ready/`.
2. A commit adding an implementation `🚧` with its backing ticket left in `todo/`
   produces the advisory, naming the spec stem and the ticket.
3. A commit adding a `🚧` backed by an `epic`/`research`/`workset` ticket still
   produces the advisory — the narrowed evaluation window must not weaken which
   ticket categories may back an implementation marker.
4. `lead-write-spec` invoked directly (not inline) still commits as it does today.

## Blocked (2026-07-26)

### Design Reviewer — block

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | The same-commit invariant contradicts lead-write-spec's own commit step, which the ticket does not touch | critical | lead-write-spec step 7 is its own git.commit, and the contract-first branch invokes it inline, so the marker lands in a spec-only commit before the ticket moves. Phase 1 must either suppress/defer that commit when invoked inline, or restate the invariant as something the existing procedure can satisfy. |
| 2 | Verify cannot see the violation it is asked to report | important | TicketVerify silently skips non-ticket-shaped paths, so a spec-only commit yields zero subjects. Widening verify's subject set to ai-docs/spec/ paths and reading spec bodies for markers must be stated as Phase 1 scope, or the host must move. |
| 3 | The named host ticket's settled constraints rule out the proposed extension | important | 260726-feat-verify-ticket-graph-advisories restricts itself to frontmatter and status directories and puts body-reading checks in Out of Scope. Resolve the host choice in this ticket: it is a new, separately designed guardrail. |
| 4 | FAIL/WARN classification deferred though precedent already decided it | minor | Adopt the adjacent ticket's severity-follows-reversibility principle: a commit is reversible, so this is a non-blocking advisory, not a hard finding. |
| 5 | Convention text is go:embed'd, so the fix is not reachable downstream without a version bump | minor | Note the plugin version bump requirement in Phase 1 so the rewritten prerequisite reaches installed plugins. |

### Completeness Reviewer — concern

| # | Title | Severity |
|---|-------|----------|
| 1 | Phase 1 bullet 3 delegates the guardrail's shape to a coordination the phase cannot itself close | important |
| 2 | Reported by verify is never made concrete: no message shape, no severity, no statement of what verify reads | important |
| 3 | No acceptance check that the narrowed evaluation window preserves the marker's guarantee | minor |
