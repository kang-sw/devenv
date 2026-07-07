---
title: "Adopt impl/<stem> branch naming (max 15 chars) and auto-delete fully-merged impl/* branches without asking"
related:
  260707-feat-drain-goal-branch-staging: this ticket's impl/ naming + auto-delete is a prerequisite building block for that ticket's per-ticket ephemeral branch churn into a goal-staging branch
sage-review: completed
---

# Adopt impl/<stem> branch naming (max 15 chars) and auto-delete fully-merged impl/* branches without asking

## Background

`lead-implement` currently creates implementation branches named
`implement/<scope-slug>` and its Branch Cleanup step (step 8) always asks for
explicit user approval before `git branch -d`, even when its own structural
guardrails (strict-ancestor check via `git merge-base --is-ancestor`, not
checked out, not linked to an active worktree, merge target not ambiguous, no
commits unreachable from the merge target) already pass cleanly. This
confirm-before-delete step is routine, low-risk friction once those guardrails
hold.

Raised alongside `260707-feat-drain-goal-branch-staging`, which depends on
this ticket's naming convention as the trust signal for automatically
cleaning up the short-lived per-ticket branches it creates while merging work
into a goal-staging branch.

## Decisions

- Rename the branch-creation convention from `implement/<scope-slug>` to
  `impl/<stem>`, with `<stem>` capped at a maximum of 15 characters.
- Branch Cleanup gains a naming-based precondition: if the branch name
  matches the `impl/*` convention **and** all of the step's existing
  structural guardrails already pass, delete without asking. If the branch
  does not match `impl/*` (including the legacy `implement/*` convention, or
  any other name), keep today's ask-first behavior unchanged.
- The naming convention itself is the trust boundary: only branches this
  tooling creates and fully owns the semantics of get the streamlined
  auto-delete path; anything else stays conservative.
- Out of scope: no change to the guardrail logic itself (ancestor check,
  checked-out check, worktree-link check, ambiguous-target check, unreachable-
  commits check all stay exactly as-is); no change to the fresh-branch
  creation logic beyond the naming convention.
- The `implement/` prefix string is load-bearing for a second purpose beyond
  naming: `deriveImplementBranchPlan` (`implement_resolver.go`) also uses it
  to detect whether the *current* branch already is an implementation branch
  (driving the create/continue/rename/stop branch-plan decision, and gating
  the `policy.branch.merge_target` warning). A naive rename would make the
  resolver stop recognizing an in-progress legacy `implement/*` branch as an
  implementation branch, mis-deriving `Action: create` (spawning a sibling
  `impl/*` branch next to unfinished work) instead of `continue`/`rename` for
  any session already on an old-style branch when this ships. Phase 1 must
  update this detection check to accept **either** prefix (`implement/` or
  `impl/`) when testing "already on an implementation branch," while only
  ever constructing new branch names under the `impl/` convention going
  forward.

## Deferred to Implementation

- Exact truncation/collision mechanics when the natural scope slug exceeds 15
  characters (truncate, abbreviate, hash-suffix, or otherwise) — not yet
  specified by the user; pick a reasonable pragmatic scheme during
  implementation.
- Whether the renamed convention applies retroactively to any in-flight
  `implement/*` branches, or only to newly created branches going forward
  (default assumption: new branches only — no migration of existing branch
  names).

## Phases

### Phase 1: Rename branch convention and add naming-gated auto-delete

- Update the branch-name construction in `implement_resolver.go`'s fresh-
  branch `create` path (and the `lead-implement` playbook prose describing
  it) from `implement/<scope-slug>` to `impl/<stem>`, applying the 15-
  character cap per the deferred truncation scheme chosen during
  implementation.
- Update the "already on an implementation branch" detection check
  (currently `strings.HasPrefix(obs.CurrentBranch, "implement/")`) to accept
  either the legacy `implement/` prefix or the new `impl/` prefix, so
  in-progress legacy-named branches are still correctly recognized as
  continue/rename candidates rather than misidentified as fresh-start state.
- Update `lead-implement`'s Branch Cleanup step (step 8) so that when the
  branch name matches `impl/*` and every existing skip condition is clear,
  deletion proceeds without asking; branches that don't match `impl/*` keep
  the current ask-first flow verbatim.
- Add or update resolver/playbook tests covering: `impl/*` branch auto-
  deletes when guardrails pass; `impl/*` branch still skips deletion when any
  guardrail fails (checked out, worktree-linked, ambiguous target,
  unreachable commits); non-`impl/*` branch (e.g. legacy `implement/*`) still
  asks before deleting even when guardrails pass.

## Spec Impact

`lead-implement`'s branch-naming and Branch Cleanup behavior is documented
caller-visible workflow-skill contract surface; addressing this at
ready-promotion time will need a spec update describing the new naming
convention and the naming-gated auto-delete precondition, including a
one-line note that the `impl/*` trust boundary is name-based (a hand-created
`impl/foo` branch not produced by `enter_implement` would also qualify once
its structural guardrails pass — a narrow, low-blast-radius edge case since
the guardrails themselves are unchanged). Contract-first spec: no — this
refines existing documented behavior rather than introducing a new planned
contract.
