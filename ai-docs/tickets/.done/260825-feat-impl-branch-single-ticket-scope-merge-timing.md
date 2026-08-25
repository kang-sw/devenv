---
title: Single-ticket-scoped impl branch — relation-aware start gate and ticket-done merge deferral
related:
  260627-feat-enter-implement-deterministic-verdict-engine: substrate — owns deriveImplementBranchPlan / finishImplementBranchPlanTail and the deterministic implement verdict this ticket re-tunes
  260707-feat-drain-goal-branch-staging: interaction — introduced policy.branch.merge_confirm and the goal-staging branch model; this ticket's goal-branch seamless path and the merge-deferral default must stay consistent with it
  260523-bug-implement-merge-target-discovery: substrate — established the impl/<merge-root>/<stem> name-encoding the start gate parses
  260711-feat-current-branch-low-ceremony: precedent — the `current` no-branch/no-merge action shows the verdict engine already carries a low-ceremony branch action
  260824-epic-review-watermark-model: complementary (not parent) — coarser per-ticket merge cadence makes each ticket-done merge a single mainstream-integration moment the epic's marker/sweep keys on
  260824-feat-review-watermark-ledger: coordination — both hook tickets.close as a checkpoint; at that checkpoint the order is merge-review (this ticket) then marker/sweep recompute (that ticket), so the two close-hooks must coexist and be ordered
sage-review-design: completed
sage-review-completeness: completed
completed: 2026-08-25
---

# Single-ticket-scoped impl branch — relation-aware start gate and ticket-done merge deferral

## Background

Two related defects in the `enter.implement` branch/merge behavior surfaced during
a workflow-design discussion, both rooted in the same missing invariant: an
`impl/*` branch is not treated as single-ticket-scoped.

1. **The current cross-ticket default is a silent branch rename, not a block.**
   When `enter.implement` is entered while already on an `impl/<root>/<stem>`
   branch but the caller's scope differs from that branch, the resolver's
   tail logic (`finishImplementBranchPlanTail`,
   `agents-plugin-tool/internal/mcp/implement_resolver.go`) chooses `rename` —
   not `stop` — because `allow_rename` **defaults to `yes`**
   (`implement_resolver.go` normalizes `factOr(policy.Branch.AllowRename, "yes")`;
   a resolver test asserts "allow_rename absent should default to yes"). `rename`
   only steps aside for an explicit `allow_rename: no`, an already-existing target
   branch, or upstream/ahead/behind tracking state — none of which hold for the
   common local, unpushed impl branch. So a naive argument-less `lead-proceed`
   run that starts a *different* ticket while still sitting on a prior ticket's
   unmerged `impl/*` branch will **silently rename that branch and stack the new
   ticket's commits on top of the prior ticket's unmerged work**, mixing two
   tickets onto one branch and erasing the original branch name. This is the
   present default behavior, not a hypothetical.

2. **The merge gate fires per phase, keyed only on `merge_confirm`, with no
   ticket-completion awareness.** `lead-implement` is invoked once per phase
   (`lead-proceed` resolves exactly one phase per call). Each invocation ends at a
   final-action gate whose merge instruction branches solely on
   `policy.branch.merge_confirm` (default `ask`) — nothing in the resolver or the
   generated instruction inspects whether the run completed the ticket's last
   unfinished phase. The result is a merge prompt (or, under a goal drain's
   `merge_confirm: skip`, an unattended merge) at *every* phase boundary, splitting
   one ticket's work into per-phase merge commits with no ergonomic reason to do so.

The desired invariant: **an `impl/*` branch belongs to exactly one ticket; phases
within that ticket accumulate on the one branch and merge once at ticket
completion (unless a phase declares an explicit stop gate); starting a different
ticket while on an unmerged impl branch is blocked (non-goal) or seamlessly landed
(goal), never silently mixed.**

## Decisions

- **Separate SAFETY from IDENTITY.** The deterministic resolver owns only the
  *safety* question ("does this branch carry unmerged work that starting here would
  mix?") and must answer it with the dumbest reliable signal. The *identity*
  question ("which ticket owns this unmerged work?") is model judgment and is
  routed to the lead, never simulated inside the MCP tool. This follows the
  260605 pivot boundary (MCP tools do not own model-spawn orchestration).
- **Rejected: mechanical commit-content parsing inside the resolver.** A tempting
  design parses `mergeRoot..HEAD` commit messages, extracts edited ticket stems,
  and reads those tickets' frontmatter/content to auto-decide relation. Rejected:
  it takes a permanent dependency on commit hygiene (WIP/squashed commits are
  stem-silent), frontmatter-format stability, and cross-status stem→path
  resolution, purely to reduce false-positive blocks — a false positive is cheap
  to recover (the lead resolves it from its own just-closed-ticket context, or a
  one-shot explore), while a false negative is an expensive branch-mixing event.
  The safety gate must bias conservative and must not tie correctness to commit
  conventions.
- **Relation-detection layering (resolved to conservative).**
  - **L0 (exists):** branch-name stem vs caller scope → match ⇒ `continue`.
  - **L1 (new, mechanical, MCP git observation):** name mismatch **and** the
    current branch has unmerged commits ahead of its merge root ⇒ `stop` (hard
    safety block). Zero commit-content parsing; keyed on an ahead-of-merge-root
    count — a *new* git observation the MCP layer adds, distinct from the
    existing `Ahead`/`Behind` which are measured against upstream tracking, not
    the merge root.
  - **L2 (lead context):** the lead knows which ticket it just finished; when it
    does, it supplies the relation as an explicit fact/policy so the resolver
    reaches the right verdict without a stop.
  - **L3 (lead explore, fallback):** on an L1 stop the resolver emits guidance
    routing identity resolution to the lead — resolve from context, or dispatch an
    explore comparing the branch's commit history to the target ticket, then
    re-invoke. This commit-history inspection is **lead-side judgment, never
    resolver code**, so it does not contradict the resolver's
    zero-commit-content-parsing rule. The resolver's own stop-message hint is
    limited to the branch-name-encoded stem (`parseImplBranchRoot`) plus
    `target.ticket_stem` — zero commit parsing — and is best-effort, never
    load-bearing.
- **`enter.implement` already receives `target.ticket_stem`** (server.go input
  schema) but the branch planner does not consume it — so the mechanical safety
  addition needs **no schema change**, only new consumption of an existing input
  plus one new observed git fact (ahead-of-merge-root).
- **Goal-branch seamless path is preserved.** On a `goal/*` branch the existing
  `create` path plus the goal drain's `merge_confirm: skip` already lands each
  ticket into the goal staging branch; the start gate must not add friction there.
  Only the non-goal, unmerged, cross-ticket case blocks.
- **Merge deferral via a `tickets.close` review trigger (not a proceed→implement
  completion signal).** The per-phase final action defaults to continue-on-branch
  with no merge (the final-action gate's explicit merge option stays available for
  a caller who wants to merge early). The deferred merge is caught at
  `tickets.close`: closing a ticket while on an unmerged `impl/*` branch surfaces a
  merge-review nudge. Rationale: "this ticket is being closed" is a stronger, more
  mechanical "work complete, land it" signal than proceed inferring "this is the
  last unfinished phase," and `tickets.close` is an already-hit deterministic MCP
  checkpoint (the same one epic ③ uses for sweep recompute) — so no honor-system
  and no proceed→implement signal plumbing.
  - **Rejected: the proceed→implement completion signal** (earlier design of this
    ticket). It tied the trigger to proceed reasoning about phase topology; the
    close-time checkpoint is simpler and more robust (260630 diet — drop machinery
    that does not earn its cost).
  - **Rejected: keeping both as defense-in-depth.** Redundant given the close
    checkpoint is mechanically reliable; two triggers is exactly the machinery the
    diet direction removes.
- **`tickets.close` emits guidance, never performs the merge.** It is a thin mover;
  native merge stays lead-driven. The nudge reuses Phase 1's ahead-of-merge-root
  observation to detect the unmerged impl branch and returns a `next_instruction`;
  the lead sequences the merge (delegating exploration if needed). Ordering note:
  the close stages the `.done` move commit onto the impl branch and the nudge guides
  "after this close commit, merge `impl/<root>/<stem>` into `<root>`" — but that
  merge is a native lead action performed *after* `tickets.close` returns (the
  nudge fires precisely because the branch is still unmerged). So epic ③'s
  marker/sweep recompute, if it runs synchronously inside the same close call, does
  **not** see the just-landed range at that checkpoint; correctness does not depend
  on it doing so — ③ is advisory and its marker advances only on an actual stamping
  review, so the deferred range is captured at a later checkpoint (see ③'s
  skip-coverage invariant and its enter.*/session-start backstops).
- **Exception = ticket-declared user stop gate.** A phase may declare an explicit
  user direct-execution / verification gate (normally noted in the ticket). At such
  a phase the run stops for the user regardless of the merge-deferral default.
  Agents judge this normally when undeclared; the declaration is the override. This
  reuses existing lead judgment over free-form phase prose — **no new marker format
  or code path** is introduced by this ticket; the declaration is ordinary phase
  text the lead already reads, not a schema field.

## Constraints

- Preserve the existing `continue` / `create` / `current` verdict actions and the
  `impl/<merge-root>/<stem>` name-encoding; this ticket re-tunes the mismatch
  action and adds a `tickets.close` merge-review trigger, it does not rewrite the
  branch model.
- Do not introduce a numeric merge/phase count input; keep the resolver
  fact-and-observation driven (consistent with 260627).
- Host-neutral: the single-ticket-scope invariant and the goal/non-goal split must
  not hard-code devenv's `develop`/`main` topology.

## Phases

### Phase 1: Relation-aware start gate — replace the silent cross-ticket rename with a conservative safety stop

Goal: entering `enter.implement` on an `impl/*` branch whose scope differs from the
caller's target, while that branch has unmerged commits ahead of its merge root,
resolves to `stop` with lead-routing guidance instead of a silent `rename`. The
goal-branch seamless path and the same-scope `continue` path are unchanged.

Approach:
- In the branch-plan tail (`finishImplementBranchPlanTail` /
  `deriveImplementBranchPlan`, `implement_resolver.go`), gate the `rename` action
  on the current branch having **no** unmerged work ahead of its merge root; when
  unmerged work exists and the scope mismatches, resolve to `stop` regardless of
  `allow_rename` (the safety block is not overridable by the rename default). Add
  the ahead-of-merge-root observation to the branch-observation fact set the MCP
  layer builds (alongside the existing upstream/ahead/behind observation).
- Emit a stop `next_instruction` that routes identity resolution to the lead
  (L2 context / L3 explore) and names the suspected owning work by the
  branch-name-encoded stem only (no commit-content parsing) as a best-effort,
  non-load-bearing hint.
- Consume `target.ticket_stem` where it sharpens the guidance message; do not make
  the safety decision depend on stem parsing.
- Leave `allow_rename`'s meaning intact for the *no-unmerged-work* relabel case
  (renaming an empty/just-created branch stays a valid `rename`).

Verification: resolver unit tests over the branch-observation matrix — (a) on
`impl/<root>/<A>` with unmerged commits, target scope `<B>` ⇒ `stop` even with
`allow_rename: yes`; (b) same branch with no unmerged commits, target `<B>` ⇒
`rename` still allowed; (c) same-scope target ⇒ `continue`; (d) on a `goal/*`
branch ⇒ `create` seamless path unchanged; (e) the stop instruction carries the
lead-routing guidance. Update the affected spec prose (see Spec Impact).

### Result (3db94261) - 2026-08-25

`observeImplementBranch` now computes a new `AheadOfMergeRoot` count on
`implementBranchObservation` — merge root derived via the existing
`implementMergeRootFor` (branch-name encoding), `MergeBase` + `rev-list
--count mergeBase..HEAD`, fail-open to `0` on any git error, and skipped
(stays `0`) for rootless/legacy/non-impl branches. `finishImplementBranchPlanTail`
gains a `stop` branch inserted immediately after the same-target `continue`
check and **before** the `allow_rename` check, so when `AheadOfMergeRoot > 0`
and the target scope mismatches, the resolver stops unconditionally (not
overridable by `allow_rename`). `target.ticket_stem` is threaded into
`normalizedImplementFacts` (no input-schema change) for the stop reason;
`implementBranchPlan` gains an additive `SuspectedOwnerStem`
(`suspected_owner_stem,omitempty`) populated only from `parseImplBranchRoot`
(branch-name, zero commit-content parsing), which `implementNextInstruction`
keys on to emit the lead-routing stop guidance (resolve from session context or
dispatch an explore). `continue`/`create`/`current` actions, the
`impl/<merge-root>/<stem>` name-encoding, and the goal-branch seamless `create`
path are unchanged.

- Verification: `go test ./internal/mcp/...` + `go vet ./internal/mcp/...` pass.
  4 new resolver unit tests cover matrix (a)-(d) plus assertion (e) that neither
  the stop `Reason` nor `NextInstruction` claims commit-content parsing. Pinned
  tests (`TestResolveImplementBranchRenameDefaultsToAllowedWhenUnset`,
  `TestResolveImplementBranchStopOmitsPlannerInstructions`, the `merge target
  required` stop test) unchanged.
- Review: partitioned correctness + fit — both verified the code clean on every
  reviewed point (unconditional stop ordering, fail-open, SAFETY/IDENTITY
  separation, future-phase fit, no pinned test altered). Both raised the same
  Important — the Spec Impact prose was omitted from the code commit — resolved
  in the doc pre-pass (spec commit 119031b3). No unresolved findings.
- Commits: 350f77c9 (survey plan), 3db94261 (resolver + tests), 119031b3 (spec).

> Forward to Phase 2: `AheadOfMergeRoot` is computed inside
> `observeImplementBranch` from `obs.CurrentBranch` alone, independent of the
> `finishImplementBranchPlanTail` call site, so Phase 2's `tickets.close` hook can
> call `observeImplementBranch` and read the same field directly. `SuspectedOwnerStem`
> is keyed as a struct field (not a `Reason` substring), so a future stop-message
> variant will not break the hint.

### Phase 2: Default no-merge per phase + `tickets.close` merge-review trigger + ticket-declared stop-gate exception

Depends on Phase 1 (deferring merges makes it more likely a run sits on an
unmerged impl branch across tickets, so the Phase 1 safety block must be in place
first for the deferral to be safe).

Goal: within one ticket, phases accumulate on the single impl branch without a
per-phase merge; the deferred merge is reviewed at `tickets.close`; a
ticket-declared per-phase user stop gate overrides the no-merge default.

Approach:
- Change the per-phase final-action default (the merge instruction in
  `session_state.go`'s implement final action) so the default is
  continue-on-branch with **no** merge; the gate's explicit merge option stays
  available for a caller who wants to merge early, and `merge_confirm` semantics
  (ask vs skip) apply only when a merge is actually chosen. Do **not** add a
  proceed→implement completion signal.
- Add a merge-review trigger to `tickets.close`: when closing a ticket while the
  current branch is an `impl/*` branch with unmerged commits ahead of its merge
  root (Phase 1's observation), return a `next_instruction` nudging a merge review
  of `impl/<root>/<stem>` into `<root>`, sequenced after the close move commit. The
  tool performs no merge; the lead acts (delegating exploration if needed).
- Honor a ticket-declared per-phase user stop gate as an override that stops the
  run at that phase irrespective of the no-merge default.
- Verify the goal-drain interaction: under a goal drain the per-phase default is
  already no-merge, so the deferred merge lands at close/queue drain — confirm the
  goal staging model (260707) still yields its intended per-ticket merge and that
  the close nudge does not double-fire against an already-merged goal branch.
- Coordinate with epic ③ (`260824-feat-review-watermark-ledger`), which also hooks
  `tickets.close`. Both hooks run while the branch is still unmerged (the merge is a
  native lead action after close returns), so ③'s recompute at that call does not
  see the just-landed range; correctness rests on ③'s laziness (advisory recompute,
  marker advances only on a real stamping review, later-checkpoint backstops), not
  on same-call ordering. Settle the exact hook order/measurement with ③'s
  implementer so the two close-hooks compose without double-firing or false
  confidence.

Verification: session-state / implement-resolver tests that intermediate and final
phases both default to continue-without-merge (no auto-merge) and that the manual
merge option still resolves when explicitly chosen; `tickets.close` tests that an
unmerged `impl/*` current branch yields the merge-review nudge while a merged/clean
or non-impl branch yields none; a goal-drain path test that the staging merge still
occurs once and the nudge does not double-fire. Update the affected spec prose (see
Spec Impact).

### Result (819c5b56) - 2026-08-25

Landed via delegated-survey lead-implement (plan `ea839c22`, code+tests
`819c5b56`, spec doc-pre-pass `12ca2f4f`).

- Per-phase final-action default flipped to continue-on-branch **without**
  merging: `implementFinalActionInstruction`/`implementMergeInstruction`
  (`session_state.go`) rewritten to report the retained branch + commit range as
  the default outcome, with an explicit merge kept as a caller-chosen option
  whose approval is gated by `MergeConfirm` (`skip` drops the ask, else asks) —
  never a default merge trigger. No phase-index field and no proceed→implement
  completion signal were added; the flip is uniform because `implementTodoVerdict`
  carries no phase position, so the same instruction functions run for every
  phase.
- `tickets.close` merge-review nudge added: `implementCloseMergeReviewNudge`
  (`implement_resolver.go`) reuses Phase 1's `observeImplementBranch`/
  `AheadOfMergeRoot`/`parseImplBranchRoot` as-is (no new git-observation code),
  and `server.go`'s `tickets.close` case appends a second `next_instruction` line
  when the current branch is an unmerged `impl/<root>/<stem>`. It fails open to
  `""` on git error, non-impl branch, or clean/merged branch; `tickets.move` and
  `formatTicketMutate`'s signature are untouched. The tool still performs no
  merge and no commit; the nudge text tells the lead to review-and-merge after
  the close-move commit lands.
- Ticket-declared per-phase user stop gate: implemented as **no code** — ordinary
  lead judgment over free-form phase prose, no new marker/schema/code path (per
  the Decisions boundary).
- Epic ③ (`260824-feat-review-watermark-ledger`, unimplemented) coordination:
  the `tickets.close` hook is a standalone guidance-only `next_instruction` keyed
  on the impl-branch observation, so ③'s later hook composes without rework.
- Tests (update-in-place, not additive):
  `TestDeriveImplementTodoInstructionsMergeConfirmSkip` now covers the no-merge
  default across both `continue` and `create` `BranchPlan.Action` plus an
  explicit-merge choice honoring `MergeConfirm`; three new real-git
  `TestServeStdioTicketsCloseMergeReviewNudge*` integration tests (unmerged /
  clean / non-impl) give the first integration coverage of the real
  `aheadOfMergeRootCount` rev-list path. `go test ./internal/mcp/...` ok, `go vet`
  clean.
- Partitioned review (correctness=opus, fit=sonnet, test=sonnet): all clean. One
  accepted test-hygiene Minor (a redundant `"root-branch"` substring assertion
  subsumed by the full `impl/root-branch/...` check — no independent coverage,
  not a defect); left as-is.

> Forward: the goal-drain staging model was verified by read-check only
> (`lead-drain-ready-queue` / `lead-goal-fan-out-step` already gate merge as a
> single lead-approved once-per-unit action), not by a new goal-drain path test —
> the no-double-fire property rests on the nudge failing open on `goal/*` branches
> (`AheadOfMergeRoot` computes 0 for a non-`impl/*` branch), which the non-impl
> integration case exercises directly.

## Spec Impact

Target: `ai-docs/spec/mcp-tools.md` — (1) the `enter.implement` branch-plan and
merge-confirmation description (branch-action resolution and the "merge
confirmation defaults to asking unless the caller explicitly passes
`policy.branch.merge_confirm: skip`" sentence), and (2) the `tickets.close`
description, which gains the impl-branch merge-review trigger.

Expected caller-visible changes:
- Entering on an `impl/*` branch with unmerged work and a mismatched scope now
  resolves to `stop` (safety block) rather than a rename; `rename` remains only for
  the no-unmerged-work relabel case.
- The per-phase final action defaults to continue-on-branch with no merge; a merge
  happens only when explicitly chosen at the gate or reviewed at close.
  `merge_confirm` semantics are unchanged when a merge is chosen.
- `tickets.close` now returns a merge-review nudge when the current branch is an
  unmerged `impl/*` branch, sequenced after the close move commit; it performs no
  merge itself. No proceed→implement completion signal is introduced.
