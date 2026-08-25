---
title: Single-ticket-scoped impl branch — relation-aware start gate and ticket-done merge deferral
related:
  260627-feat-enter-implement-deterministic-verdict-engine: substrate — owns deriveImplementBranchPlan / finishImplementBranchPlanTail and the deterministic implement verdict this ticket re-tunes
  260707-feat-drain-goal-branch-staging: interaction — introduced policy.branch.merge_confirm and the goal-staging branch model; this ticket's goal-branch seamless path and the merge-deferral default must stay consistent with it
  260523-bug-implement-merge-target-discovery: substrate — established the impl/<merge-root>/<stem> name-encoding the start gate parses
  260711-feat-current-branch-low-ceremony: precedent — the `current` no-branch/no-merge action shows the verdict engine already carries a low-ceremony branch action
  260824-epic-review-watermark-model: complementary (not parent) — coarser per-ticket merge cadence makes each ticket-done merge a single mainstream-integration moment the epic's marker/sweep keys on
sage-review-design: completed
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
    re-invoke. Any commit-stem hint in the stop message is best-effort and never
    load-bearing.
- **`enter.implement` already receives `target.ticket_stem`** (server.go input
  schema) but the branch planner does not consume it — so the mechanical safety
  addition needs **no schema change**, only new consumption of an existing input
  plus one new observed git fact (ahead-of-merge-root).
- **Goal-branch seamless path is preserved.** On a `goal/*` branch the existing
  `create` path plus the goal drain's `merge_confirm: skip` already lands each
  ticket into the goal staging branch; the start gate must not add friction there.
  Only the non-goal, unmerged, cross-ticket case blocks.
- **Merge deferral to ticket completion.** `lead-proceed` computes the target
  phase (first phase with no `### Result`); it will additionally pass a
  "this run completes the ticket's last unfinished phase" signal to
  `enter.implement`. The merge gate fires only when that signal is set; otherwise
  the per-phase final action defaults to continue-on-branch without a merge.
- **Exception = ticket-declared user stop gate.** A phase may declare an explicit
  user direct-execution / verification gate (normally noted in the ticket). At such
  a phase the run stops for the user regardless of the merge-deferral default.
  Agents judge this normally when undeclared; the declaration is the override.

## Constraints

- Preserve the existing `continue` / `create` / `current` verdict actions and the
  `impl/<merge-root>/<stem>` name-encoding; this ticket re-tunes the mismatch
  action and adds a completion-aware merge trigger, it does not rewrite the branch
  model.
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
  (L2 context / L3 explore) and names the suspected owning work as a best-effort,
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

### Phase 2: Ticket-done merge deferral and ticket-declared stop-gate exception

Depends on Phase 1 (deferring merges makes it more likely a run sits on an
unmerged impl branch across tickets, so the Phase 1 safety block must be in place
first for the deferral to be safe).

Goal: within one ticket, phases accumulate on the single impl branch without a
per-phase merge; the merge gate fires only when the run completes the ticket's last
unfinished phase, or when the phase declares an explicit user stop gate.

Approach:
- `lead-proceed` (proceed resolver / skill) computes and passes a
  ticket-completion signal ("this run's target is the last unfinished phase") to
  `enter.implement`.
- `enter.implement`'s final-action / merge instruction fires the merge gate only
  when the completion signal is set; otherwise the default per-phase final action
  is continue-on-branch with no merge. Existing `merge_confirm` semantics (ask vs
  skip) still apply *when* the gate fires.
- Honor a ticket-declared per-phase user stop gate as an override that stops the
  run at that phase irrespective of the deferral default.
- Verify the goal-drain interaction: under a goal drain, deferral yields one merge
  into the goal staging branch at ticket completion (closer to 260707's intended
  "one confirmed merge at queue-empty") rather than a per-phase staging merge —
  confirm no regression against the staging model.

Verification: proceed-resolver tests that the completion signal is set only on the
last unfinished phase; implement-resolver / session-state tests that the merge
instruction is emitted only under the completion signal (and under a declared stop
gate), and that intermediate phases resolve to continue-without-merge; a
goal-drain path test that the per-ticket staging merge still occurs once at
completion. Update the affected spec prose (see Spec Impact).

## Spec Impact

Target: `ai-docs/spec/mcp-tools.md`, the `enter.implement` branch-plan and
merge-confirmation description (currently around the branch-action resolution and
the "merge confirmation defaults to asking unless the caller explicitly passes
`policy.branch.merge_confirm: skip`" sentence), plus any `lead-proceed` phase-scope
description that must now mention the ticket-completion signal.

Expected caller-visible changes:
- Entering on an `impl/*` branch with unmerged work and a mismatched scope now
  resolves to `stop` (safety block) rather than a rename; `rename` remains only for
  the no-unmerged-work relabel case.
- The merge gate is emitted only at ticket completion (last unfinished phase) or a
  declared per-phase stop gate; intermediate phases continue on the branch without
  a merge. `merge_confirm` semantics are unchanged when the gate fires.
- A new ticket-completion signal flows from `lead-proceed` to `enter.implement`;
  document its derivation (first phase with no `### Result` == last unfinished).
