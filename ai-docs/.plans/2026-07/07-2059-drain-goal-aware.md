# Plan: 260707-feat-drain-goal-branch-staging — Phase 2: Make lead-drain-ready-queue goal-aware

## Relevant Ticket Contract

- Decision 1 (Trigger/bootstrap): activate goal-staging only when (a) an
  active Claude Code harness `/goal` Stop-hook reminder is present in the
  current conversation, and (b) the current branch is not already `goal/*`.
  When both hold, create and check out `goal/<slug>` (slug from the harness
  goal text) before dispatching the next ready ticket. Outside an active
  `/goal` context, preserve today's behavior exactly (no staging branch).
- Decision 2 (No new persisted state): the signal is git branch state only —
  "currently on `goal/<slug>`" is the entire state machine. No new
  session-state field. Optional: derive already-done tickets in the run from
  merge-commit `## Ticket Updates` on the goal branch (not required to
  implement, mentioned as a possibility only).
- Decision 3 (Per-ticket dispatch): with `goal/<slug>` checked out, each
  ticket's `lead-implement` run resolves its create-time merge target as the
  goal branch automatically — confirmed by
  `deriveImplementBranchPlan` (`agents-plugin-tool/internal/mcp/implement_resolver.go#L621-L634`):
  the `create` action path sets `plan.MergeTarget = obs.CurrentBranch`
  whenever current branch has neither `impl/` nor `implement/` prefix, with
  no consultation of any merge-target policy for that path. So checking out
  `goal/<slug>` before dispatch is sufficient — no resolver code change.
  Dispatch must additionally pass `policy.branch.merge_confirm: "skip"` as
  explicit caller policy so `lead-implement`'s existing Route step 3 ("Gather
  target, facts, and explicit caller policy for enter.implement") picks it
  up. Each ticket still gets its own `impl/<stem>` branch, auto-deleted once
  merged into the goal branch per
  `260707-feat-impl-branch-convention-autodelete` (already implemented on
  this branch, commit `24cb29b6`).
- Decision 4 (Completion/final merge): when the ready queue is empty AND
  current branch is `goal/<slug>`, `lead-drain-ready-queue` performs the
  single final merge `goal/<slug>` -> `main` directly in its own prose — it
  must NOT invoke `lead-implement`/`enter.implement` for this step (ticket-less,
  `enter.implement` requires `ticket_stem`/`scope_label`/`scope_slug`). This
  step must ask for explicit user approval before merging, worded
  equivalent in spirit to `lead-implement`'s Branch invariant
  ("Wait for user approval before merge...",
  `agents-plugin/rsrc/lead-implement/lead-implement.md#L15-L17`). This is the
  sole human confirmation point for the entire goal-driven run.
- Decision 5 (Remote/push untouched): this override never extends to
  push/remote actions, for both intermediate and final merges.
- Decision 6 (`policy.branch.merge_confirm` stays a plain policy fact):
  `lead-implement` remains ignorant of "goal" as a concept; it only consumes
  the fact its caller (drain, in this case) supplies. Do NOT add any
  goal-specific language to `agents-plugin/rsrc/lead-implement/lead-implement.md`
  in this phase — Phase 1 already updated its Branch invariant text
  goal-neutrally (already merged, commit `c8dc46a8`).
- Deferred-to-Implementation item: goal-detection, branch checkout/creation,
  and the final-merge step are visible only to the lead's own turn context,
  not to the delegated ticket-selection subagent. `lead-drain-ready-queue`'s
  existing norm of delegating ticket *selection* to a subagent
  (`lead-prefer-subagent`) is unchanged; the lead itself must own
  goal-detection, branch checkout/creation, and the final-merge step.
- Deferred-to-Implementation item: exact mechanics for detecting "an active
  harness `/goal` Stop-hook reminder is present" are Claude-Code-harness
  specific and left to skill prose judgment; host-neutral fallback (no
  equivalent signal) must default to today's non-staging behavior.
- Verification boundary (from Phase 2 text): exercise both branches of the
  mode switch end-to-end — goal-driven run creates/checks out `goal/<slug>`,
  dispatches at least two tickets with `merge_confirm: skip` merging into
  that branch with no ask, reaches the single final confirmed `main` merge
  only once the ready queue is empty; non-goal run reproduces today's exact
  per-ticket direct-to-target behavior with no staging branch and no
  `merge_confirm` override.

## Out of Scope

- Any change to `enter.implement`/`implement_resolver.go` (Phase 1, already
  done and merged — no resolver-side goal awareness needed per Decision 3's
  Background finding).
- Any change to `lead-implement`'s own Route/Policy-rules prose beyond what
  Phase 1 already did — `lead-implement` must stay goal-unaware (Decision 6).
- Any new persisted/tracked state for "is a goal run in progress" (Decision 2).
- Deriving already-done-tickets reporting from merge-commit `## Ticket
  Updates` — mentioned as a possibility in Decision 2 only if ever needed;
  not required for this phase's verification boundary.
- `260707-feat-impl-branch-convention-autodelete`'s own implementation — it
  is a prerequisite already landed on this branch (commit `24cb29b6` et al.,
  unmerged to `main`); this phase only relies on its auto-delete behavior,
  does not modify it.

## Codebase Findings

- `agents-plugin/skills/lead-drain-ready-queue/SKILL.md#L1-L23` — current
  full skill body: static prose, zero branch/merge logic, delegates ticket
  selection to a light-tier subagent, then hands off to `lead-proceed` with
  an explicit ticket path. This is the file to edit for this phase.
- `agents-plugin-wsflow/skills/lead-drain-ready-queue/SKILL.md` — confirmed
  byte-identical to the file above today. This skill is in the
  **substitution-mirrored** skill set (`agents-plugin-tool/internal/wsrsrc/skills_mirror_test.go#L15-L19`,
  list: `lead-drain-ready-queue`, `lead-prefer-subagent`,
  `lead-verify-discussion`), NOT a hand-curated shim and NOT listed under
  "Substitution-Mirrored Skill Generation" prose in
  `ai-docs/ref/wsflow-mirroring.md#L120-L161` (that doc's curated list text
  only names the other two — this is pre-existing doc drift, unrelated to
  this ticket; flagging only so the implementer does not skip the required
  regen step because the doc list looked exhaustive).
  - **Required after editing the full-ws source**: run
    `WS_REGEN_WSFLOW_SKILLS=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowSkillsMirror`
    (from `agents-plugin-tool/`) to regenerate
    `agents-plugin-wsflow/skills/lead-drain-ready-queue/SKILL.md`.
  - **Eligibility guard**: the generator fails loudly if the source body
    contains anything beyond namespace-only tokens (`ws:`/`ws/` ->
    `wsflow:`/`wsflow/`) — no "mercenary" word, no
    `<!-- ws:full-only:... -->` markers, no literal names of wsflow-excluded
    skills (`lead-write-code`, `lead-write-skeleton`, `lead-salvage`,
    `lead-skill-authoring`). New prose must stay within plain git-CLI
    commands and skill-name references only; do not introduce any
    `ws.mercenary.*` reference or full-only marker into this file.
- `agents-plugin/rsrc/lead-implement/lead-implement.md#L15-L17` — Branch
  invariant text already updated by Phase 1: "Wait for user approval before
  merge or another implementation slice, unless the resolved verdict's merge
  confirm is `skip`, in which case proceed with that merge without asking."
  This is the consumption side drain's new `merge_confirm: skip` policy fact
  relies on — no further edit needed here.
- `agents-plugin/rsrc/lead-implement/lead-implement.md#L38-L53` — Route step
  3-4 and "Policy rules" bullets. `policy.branch.merge_target` is set only
  when already on an implementation branch or user-named; `allow_rename`
  defaults `yes`. There is **no existing bullet for `merge_confirm`** in this
  policy-rules list, and per Decision 6 none should be added here — drain
  supplies `merge_confirm: skip` as "explicit caller policy" purely through
  in-conversation instruction when it hands off to `lead-proceed`, which
  Route step 3's existing generic language ("explicit caller policy") already
  covers without modification.
- `agents-plugin/rsrc/lead-implement/lead-implement.md#L87-L96` — Steps 7
  (Merge) and 8 (Branch Cleanup): existing report/cleanup shape to mirror in
  drain's own final-merge prose (per Decision 4, drain writes this itself,
  does not call into these steps). Branch Cleanup's ask-vs-auto-delete split
  is keyed on `impl/*` naming; `goal/<slug>` does not match that prefix, so
  if drain's prose chooses to address goal-branch cleanup after the final
  merge, the non-`impl/*` ask-first behavior is the applicable precedent
  (not a resolver change — this is optional local prose, ticket does not
  mandate goal-branch deletion).
- `agents-plugin-tool/internal/mcp/implement_resolver.go#L607-L634` —
  `implementTargetBranchName` (impl/<stem> truncation) and
  `deriveImplementBranchPlan` (create-path merge-target derivation). Read to
  confirm Decision 3's Background claim; no code change needed by this
  phase. `goal/<slug>` naming/truncation is a prose-level decision for drain
  itself (git CLI, not this Go helper) — ticket leaves exact slug mechanics
  (sanitization, max length, collision handling) to implementation.
- `ai-docs/mental-model/git-workflow-tools.md#L47-L53` — "Common Mistakes":
  the MCP git surface (`git.status`, `git.diff`, `git.log`,
  `git.merge_base`, `git.commit`) is a constrained wrapper with **no
  checkout, branch-create, or merge tool**. Branch creation, checkout, and
  merge must be done with raw `git` CLI commands in drain's own skill prose,
  exactly as `lead-implement`'s existing Branch Cleanup step already does
  (`git branch -d`, `git merge-base --is-ancestor`) — this is the established
  in-repo pattern for git mutation outside the MCP surface.
- `ai-docs/mental-model/workflow-skills.md#L67` — existing mental-model
  entry for `lead-drain-ready-queue`'s current (pre-Phase-2) behavior;
  expect this bullet's text (and its anchor `{#260703-drain-ready-queue-skill}`)
  to need an update or an added anchor describing goal-awareness, per
  `lead-implement`'s normal doc-closeout pass — routine, not a special step
  for this plan.
- `ai-docs/spec/workflow-skills.md#L413-L444` — existing spec section for
  `lead-drain-ready-queue`; likely needs an addition describing goal-aware
  staging behavior during the normal Documentation step. No contract-first
  spec entry required per ticket's own "Spec Impact: Contract-first spec:
  no" statement.
- No repo precedent exists for detecting a Claude Code harness `/goal`
  Stop-hook reminder programmatically (`grep` across `ai-docs` found only
  this ticket's own text referencing it) — confirms the ticket's own
  Deferred-to-Implementation framing that this is a skill-prose-level,
  harness-observed judgment call for the lead itself, not a coded
  mechanism. No research escalation needed: the ticket already scopes this
  as intentionally left to implementation-time prose, with an explicit
  non-goal fallback default.

## Implementation Plan

1. Edit `agents-plugin/skills/lead-drain-ready-queue/SKILL.md`:
   - Add a bootstrap-detection step, evaluated by the lead itself (not the
     ticket-selection subagent): check whether an active `/goal` Stop-hook
     reminder is present in the current turn's context AND current branch is
     not already `goal/*`. If both hold, derive a slug from the harness goal
     text (sanitize to a branch-safe slug; keep it short — no strict
     numeric cap is prescribed by the ticket, follow the `impl/<stem>`
     class-of-detail spirit but do not invent a number the ticket did not
     give), then create and check out `goal/<slug>` via plain `git`
     commands (e.g. `git checkout -b goal/<slug>`) before dispatching the
     next ticket. If a `/goal` context is not detected, or current branch is
     already `goal/*`, skip branch creation (stay on the current branch,
     which is either `main`/other or already the active `goal/<slug>`).
   - Keep the existing ticket-selection delegation to the light-tier
     subagent unchanged (still returns exactly one ticket path or reports
     `ready/` empty).
   - When handing off to `lead-proceed` for a selected ticket while on a
     `goal/<slug>` branch, add explicit caller-policy instruction text
     alongside the handoff so the ensuing `lead-implement` Route step picks
     it up as "explicit caller policy": `policy.branch.merge_confirm:
     "skip"`. Do not set `merge_target` explicitly — Decision 3's Background
     finding confirms the create-path already derives it from the checked-
     out `goal/<slug>` branch automatically. When no `/goal` context is
     active, dispatch exactly as today: no merge_confirm override, no
     staging branch — preserving current behavior verbatim.
   - Add a completion step: when the ticket-selection subagent reports
     `ready/` is empty AND current branch is `goal/<slug>`, perform the
     final merge directly in this skill's own prose (not via
     `lead-proceed`/`lead-implement`): ask the user for explicit approval to
     merge `goal/<slug>` into `main` (wording equivalent in spirit to
     `lead-implement`'s Branch invariant ask), then on approval run the
     merge with plain `git` commands (e.g. `git checkout main && git merge
     --no-ff goal/<slug>` or equivalent, following repository commit-rule
     conventions for the merge commit). Explicitly state that this override
     never extends to push/remote actions (Decision 5) — no push step is
     added. When `ready/` is empty but current branch is NOT `goal/*`
     (non-goal run), keep today's plain stop-with-no-handoff behavior
     unchanged.
   - Preserve the existing `lead-prefer-subagent` delegation-posture
     reference and existing "Do not list `ready/` or read ticket files
     yourself" invariant verbatim; only add the goal-detection/staging/
     final-merge prose around the existing selection-and-handoff flow.
   - Keep the file free of `ws.mercenary.*` references, `<!--
     ws:full-only:... -->`/`<!-- ws:wsflow-only:... -->` markers, and literal
     names of wsflow-excluded skills, to satisfy the substitution-mirror
     generator's eligibility guard.
2. From `agents-plugin-tool/`, run
   `WS_REGEN_WSFLOW_SKILLS=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowSkillsMirror`
   to regenerate `agents-plugin-wsflow/skills/lead-drain-ready-queue/SKILL.md`
   from the edited source (`-count=1` is mandatory — the regen entrypoint is
   an env-gated test body with no changing input, and Go's test cache can
   return a stale green `ok` without running the write side effect).
3. Confirm the substitution-mirror drift guard is clean:
   `go test ./internal/wsrsrc -run TestWsflowSkillsMirrorUpToDate` (from
   `agents-plugin-tool/`).
4. Run the wsflow package tests to confirm the mirrored skill still passes
   the shipped-bundle checks:
   `python3 -m unittest discover agents-plugin-wsflow/tests` (from repo
   root).
5. Documentation pass (routine `lead-implement` doc-closeout, not a special
   step): update `ai-docs/mental-model/workflow-skills.md#L67`'s
   `lead-drain-ready-queue` bullet (add or extend an anchor describing
   goal-aware staging) and `ai-docs/spec/workflow-skills.md#L413-L444`'s
   `lead-drain-ready-queue` section to describe the new goal-branch-staging
   behavior, mode-switch condition, and the single-final-merge shape.
6. Record the Phase 2 `### Result` on
   `ai-docs/tickets/ready/260707-feat-drain-goal-branch-staging.md` per
   repository ticket conventions once implementation and review land.

## Verification Plan

- Primary verification is prose/manual per the ticket's own Phase 2
  Verification text — this is a skill-prose change with no unit-testable
  Go surface of its own (the underlying `enter.implement`
  `merge_confirm` behavior was already unit-tested in Phase 1). Exercise
  both branches of the mode switch end-to-end in a live drain run, per the
  ticket's stated boundary:
  - Goal-driven run: with an active `/goal` context, confirm
    `lead-drain-ready-queue` creates/checks out `goal/<slug>`, dispatches at
    least two ready tickets with `merge_confirm: skip` (each merges into
    `goal/<slug>` with no ask), and only reaches the single final confirmed
    `main` merge once `ready/` is empty.
  - Non-goal run: with no `/goal` context active, confirm drain reproduces
    today's exact per-ticket direct-to-target behavior — no staging branch,
    no `merge_confirm` override.
- Automated checks to run alongside the manual walkthrough:
  - `WS_REGEN_WSFLOW_SKILLS=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowSkillsMirror`
    then `go test ./internal/wsrsrc -run TestWsflowSkillsMirrorUpToDate`
    (from `agents-plugin-tool/`).
  - `python3 -m unittest discover agents-plugin-wsflow/tests` (from repo
    root).

## Escalations

- None.
