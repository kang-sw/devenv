---
title: "Goal-aware branch staging for lead-drain-ready-queue: single deferred final merge instead of per-ticket merge confirmation"
related:
  260707-feat-impl-branch-convention-autodelete: prerequisite — per-ticket impl/<stem> branches merged into the goal-staging branch rely on that ticket's naming-gated auto-delete to avoid branch clutter
  260703-chore-implement-branch-rename-default-allow: precedent this ticket's new policy.branch.merge_confirm fact directly mirrors (a plain caller-suppliable enter.implement policy fact, not goal-aware logic inside lead-implement)
sage-review: completed
---

# Goal-aware branch staging for lead-drain-ready-queue: single deferred final merge instead of per-ticket merge confirmation

## Background

`lead-drain-ready-queue` dispatches ready tickets one at a time through
`lead-proceed`/`lead-implement`. Each ticket's `lead-implement` run ends at an
unconditional "wait for user approval before merge" invariant
(`agents-plugin/rsrc/lead-implement/lead-implement.md`), which becomes
repetitive friction when the user has already committed to driving many
tickets through to completion in one run (e.g. under an explicit goal, or
under the Claude Code harness's own `/goal` session-scoped Stop-hook, which
re-injects a repeated reminder into the conversation until its condition is
satisfied — confirmed present via direct observation in this session).

An earlier, simpler design was considered and dropped: have
`lead-drain-ready-queue` skip the merge-approval ask by simply leaving each
finished ticket's branch unmerged and moving straight to the next ready
ticket, with no per-run staging branch. That approach was rejected because it
leaves N independent unmerged branches with no single point to review or
merge the whole run's work, and it directly conflicts with this ticket's
staging-branch design over ownership of the same merge-gate/branch-target
decision — implementing it first would have been immediately superseded, so
it was dropped rather than built.

Source-verified prior art this design relies on
(`agents-plugin-tool/internal/mcp/implement_resolver.go`):
`deriveImplementBranchPlan`'s fresh-branch `create` path currently sets
`plan.MergeTarget = obs.CurrentBranch` unconditionally — it does not consult
any caller-supplied merge-target policy for that path (a caller-supplied
`policy.branch.merge_target` is only consulted, and only take effect, on the
already-on-`implement/*` rename path; on the create path it is explicitly
ignored with an emitted warning). This means checking out a staging branch
before invoking `lead-implement` for a ticket makes that staging branch
become the create-time merge target automatically, with no resolver code
change needed for that part.

`lead-drain-ready-queue` itself is confirmed (via source read of
`agents-plugin/skills/lead-drain-ready-queue/SKILL.md` and the ticket that
introduced it) to be a fully stateless, per-invocation shim today, with zero
branch/merge logic of its own — all branch/merge behavior currently lives
entirely in `lead-implement`.

## Decisions

1. **Trigger / bootstrap.** Goal-branch staging mode activates only when both
   hold: (a) the conversation currently has an active Claude Code harness
   `/goal` Stop-hook reminder present, and (b) the current branch is not
   already a `goal/*` branch. When both hold, `lead-drain-ready-queue` creates
   a new `goal/<slug>` branch (slug derived from the harness goal text) and
   checks it out before dispatching the next ready ticket. Outside an active
   `/goal` context, drain keeps today's behavior exactly as-is — dispatching
   straight to `lead-proceed`/`lead-implement` with no staging branch — for
   full backward compatibility with non-goal-driven usage.
2. **No new persisted state.** Whether a goal-driven drain run is in progress
   is derived entirely from git branch state, not from any new session-state
   field: while the current branch is `goal/<slug>`, that fact alone is the
   signal for every subsequent drain invocation in the same run. If ever
   needed (e.g. for reporting), which tickets are already done within the
   active goal run is derived by reading merge-commit messages on the goal
   branch — which already carry `## Ticket Updates` referencing ticket stems
   per this repository's commit conventions — rather than tracked state.
3. **Per-ticket dispatch.** With the goal branch checked out, each ticket's
   `lead-implement` run resolves its create-time merge target as the goal
   branch automatically (existing `obs.CurrentBranch`-derived behavior, no
   resolver change per the Background finding above). Dispatch additionally
   sets a new caller-suppliable policy fact, `policy.branch.merge_confirm:
   "skip"` (see Phase 1), so `lead-implement` does not stop to ask before
   this particular merge. Each ticket still gets its own ephemeral
   `impl/<stem>` branch (per `260707-feat-impl-branch-convention-autodelete`)
   that is auto-deleted once fully merged into the goal branch, per that
   ticket's naming-gated Branch Cleanup behavior.
4. **Completion / final merge.** When the ready queue is empty and the
   current branch is `goal/<slug>`, `lead-drain-ready-queue` performs the
   single final merge from `goal/<slug>` into `main` **directly, in its own
   skill prose** — it does not invoke `lead-implement`/`enter.implement` for
   this step. `enter.implement`'s machinery is ticket-scoped
   (`parseImplementTarget` requires `ticket_stem`/`scope_label`/`scope_slug`),
   and "merge `goal/<slug>` into `main`" has no associated ticket, so routing
   it through `lead-implement` would require inventing a ticket-less
   invocation mode there — which would push goal-staging-specific machinery
   into the supposedly goal-unaware utility, contradicting Decision 6. Drain's
   own final-merge prose must ask for explicit user approval before merging,
   using confirmation wording equivalent in spirit to `lead-implement`'s
   Branch invariant ("wait for user approval before merge"), so the behavior
   the user sees is the same even though the code path is separate. This is
   the sole human confirmation point for the entire goal-driven run.
5. **Remote/push stays untouched.** This override never extends to
   remote/push actions. Push remains an explicit, always-confirmed action
   regardless of goal-driven state, for both intermediate and final merges;
   merge itself has no remote implication under this design.
6. **`policy.branch.merge_confirm` is a plain policy fact, not goal-aware
   logic in `lead-implement`.** Shaped analogously to the existing
   `policy.branch.allow_rename` fact from
   `260703-chore-implement-branch-rename-default-allow`: default (absent/
   unset) preserves today's unconditional ask-before-merge behavior; only an
   explicit `skip` from the caller bypasses the ask for that one merge.
   `lead-implement` remains ignorant of "goal" as a concept and only consumes
   the fact its caller supplies — `lead-drain-ready-queue` is the sole
   utilizer that decides when to set it, keeping `lead-implement` a pure,
   goal-unaware utility.

## Deferred to Implementation

- Exact slug derivation from the harness goal text for `goal/<slug>` naming
  (sanitization, max length, collision handling) — same class of detail as
  the sibling ticket's `impl/<stem>` truncation mechanics.
- Exact code location for adding `policy.branch.merge_confirm` (resolver-side
  default vs. playbook-prose-only consumption) — follow whichever precedent
  `260703-chore-implement-branch-rename-default-allow`'s implementation
  actually used for `allow_rename`.
- Exact mechanics for `lead-drain-ready-queue` to detect "an active harness
  `/goal` Stop-hook reminder is present in context" at the skill-prose level.
  This is Claude-Code-harness-specific; host-neutral fallback behavior for
  other harnesses without an equivalent goal-hook feature is unspecified and
  should default to today's non-staging behavior when no equivalent signal
  exists.
- `/goal` detection and the goal-branch checkout/final-merge steps are only
  visible to the lead's own turn context, not to a delegated ticket-selection
  subagent — `lead-drain-ready-queue`'s current norm is to delegate ticket
  selection to a subagent (per `lead-prefer-subagent`). Implementation should
  make explicit that the lead itself owns goal-detection, branch
  checkout/creation, and the final-merge step, while ticket selection stays
  delegated as today; this is a division of responsibility, not a change to
  the delegation norm.

## Phases

### Phase 1: Add `policy.branch.merge_confirm` to enter.implement

- Add the caller-suppliable `policy.branch.merge_confirm` fact to
  `enter.implement` (`implement_resolver.go` and related input parsing),
  defaulting to today's unconditional ask-before-merge behavior when
  absent/unset, and skipping the ask only when the caller explicitly passes
  `skip`.
- Update `lead-implement`'s Branch invariant text ("Wait for user approval
  before merge or another implementation slice") to describe the new
  caller-opt-out condition without introducing any goal-specific language.
- Add or update resolver tests: default (absent) still stops for approval;
  explicit `skip` proceeds without asking; explicit non-`skip` value (or
  anything else) still asks.

### Phase 2: Make lead-drain-ready-queue goal-aware

- Depends on Phase 1 and on `260707-feat-impl-branch-convention-autodelete`.
- Update `lead-drain-ready-queue`'s skill prose to: detect an active `/goal`
  context per the Deferred-to-Implementation mechanics; if detected and not
  already on a `goal/*` branch, create and check out `goal/<slug>`; dispatch
  each ticket with the staging branch as the implicit merge target and
  `policy.branch.merge_confirm: "skip"`; detect completion (ready queue empty
  while on a `goal/*` branch) and perform the single final confirmed merge
  into `main` directly in drain's own prose (per Decision 4), with
  `merge_confirm` left unset for every dispatched ticket by that point.
- Preserve today's exact behavior when no `/goal` context is active (no
  staging branch, no `merge_confirm` override).
- Verification: exercise both branches of the mode switch end-to-end —
  a goal-driven run (active `/goal` context) correctly creates/checks out
  `goal/<slug>`, dispatches at least two tickets with `merge_confirm: skip`
  merging into that branch without an ask, and reaches the single final
  confirmed `main` merge only once the ready queue is empty; a non-goal run
  (no `/goal` context) reproduces today's exact per-ticket direct-to-target
  behavior with no staging branch and no `merge_confirm` override.

## Spec Impact

This touches documented `enter.implement` contract surface (a new policy
fact) and `lead-drain-ready-queue`/`lead-implement` skill-prose behavior;
ready-promotion will need spec addressing for both. Contract-first spec: no
— `policy.branch.merge_confirm` extends existing documented `enter.implement`
policy-fact behavior in the same shape as `allow_rename`, rather than
introducing a new kind of contract.
