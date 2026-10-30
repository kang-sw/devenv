---
title: Workflow manual worktree-based work mode with a parallel subsection
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 055a1cca2920b146
sage-review-completeness-reviewed: 055a1cca2920b146
---

# Workflow manual worktree-based work mode with a parallel subsection

## Background

When the user puts a lead into "parallel worktree mode", leads often defer a
ticket because it touches the same files as another one. File overlap is the
normal case and Git merges most of it; what actually hurts is a semantic
conflict between branches, which file overlap neither predicts nor rules out.
The user's stance: unless one ticket strictly depends on another, run them in
parallel and merge afterwards.

The only text that already says this is step 3 of the
`ticket-batch-selector` delegate prompt ("overlapping files ... are
parallel-safe"), which the lead never reads. The lead's own guidance on
worktrees is split between the workflow manual's `### Git` section (the
housekeeping worktree, worker profiles, nudges) and `lead-run`'s
`## Parallel route (opt-in)` section (provisioning, release before merge,
serial merges). A lead told to work in worktrees outside a `lead-run` batch
has no guidance at all.

The user asked (2026-10-30) to consolidate this into the workflow manual as a
"worktree-based work mode" section (work is allocated into
`worktree.acquire` worktrees without moving the current worktree's branch)
with a parallel-mode subsection beneath it, and to state that the lead never
starts worktree-based work before the user asks for it.

## Decisions

1. **Placement.** Add `### Worktree-based work mode` to
   `agents-plugin/rsrc/lead-workflow-manual/lead-workflow-manual.md`,
   directly after `### Git`: the current worktree's branch stays put and each
   unit of work goes into a `worktree.acquire`-allocated worktree.
   `#### Parallel mode` sits beneath it. (User's structure request.) The
   resolved `#### Worktree profiles` list stays at the end of `### Git`: the
   tool injects it before the next `###` heading.
2. **Never start unasked.** The lead never starts worktree-based work before
   the user asks for it. The `### Git` housekeeping worktree the lead acquires
   while a worker occupies the root is exempt and stays lead-initiated.
   (User directive; exemption confirmed.)
3. **Parallel safety.** Only a strict dependency orders work: a `blocked-by:`
   edge, a prerequisite `related:`/`parent:` line, or a body that names the
   other unit as something it needs first. File overlap is never a reason to
   defer or serialize. (User directive; matches `ticket-batch-selector`
   step 3.)
4. **Selector keeps its copy.** The criterion also stays in
   `ticket-batch-selector` step 3, worded consistently with the manual.
   Rejected: moving it only to the manual - the rendered selector delegate
   does not load the lead's workflow manual.
5. **Split with `lead-run`.** `lead-run`'s `## Parallel route (opt-in)` keeps
   the per-run opt-in, the `ticket-batch-selector` dispatch, the Spawn-step
   differences (including the worker's `root_override` and Branch line), and
   report handling. Generic provisioning (`git.resolve_impl_branch` +
   `worktree.acquire`), release before merge, serial merges, and the merge
   rules below move to the manual, and `lead-run` points at them. The profile
   choice stays in `### Git`; `worker_key` ownership is left to the
   `worktree.acquire` description. (User approval.)
6. **Instruction is approval.** A user instruction to work in worktree or
   parallel mode counts as `lead-run`'s per-run approval. Rejected: a second
   ask - asking again for what the user just requested adds a round trip with
   no new information.
7. **Semantic check at merge.** Before merging a branch onto a target that
   earlier merges moved, the lead reads what the branch adds on top of them,
   names any interaction, and delegates its resolution; verification reruns
   only when an interaction is found. (User answer.) Rejected: rerunning
   verification after every merge; checking Git conflicts only.
8. **Preview with `git merge-tree`.** The step 7 input is the diff from the
   target to the tree `git merge-tree --write-tree <target> <branch>` prints.
   Rejected: a trial `git merge --no-commit` or a new `git.merge` dry-run
   parameter - merge-tree touches no worktree, index, or ref, so it works
   while the target is checked out elsewhere, and as a read-only diagnosis it
   already belongs to native Git under the manual's `### Git` rule. No
   fallback for Git older than 2.38 is written: a missing subcommand fails
   loudly, and the manual is injected into every session, so its text is
   budgeted.
9. **Conflicts go to a delegate.** A textual conflict `git.merge` leaves on
   the target, like a step 7 interaction, goes to `lead-delegate` as a
   bounded resolution task that reruns the affected verification, and reaches
   the user only when the delegate cannot settle it. (User answer.)
   Rejected: the current parallel-route merge stop that surfaces every such
   conflict to the user.

10. **Pi mirror too.** Both edited files are copied byte-identical into
    `agents-plugin-pi/rsrc/` in the same change as the wsflow mirror, with
    the mirrored `manifest.json`. `TestPiMirrorUpToDate` requires it, and the
    wsflow regen writes only the wsflow tree; the Pi tree is hand-synced
    outside version bumps. (Acknowledged by the user.)

11. **Release on an unmerged end.** A unit's worktree is also released when
    the unit ends without merging (its ticket left blocked or abandoned); the
    branch stays retained. A stop that resumes in the same worktree has not
    ended and keeps its lease. The rule sits in the manual's release sentence,
    since the failure (a pooled worktree holding the branch so no other
    checkout or merge can take it) applies to any unit. (User answer; restores
    the current `lead-run` stopped-worker rule the first compressed prose
    dropped.) Rejected: a `lead-run`-only clause (non-ticket units lose it);
    dropping the rule.
12. **Provisioning for any unit.** The manual passes a new branch to
    `worktree.acquire` per unit and names `git.resolve_impl_branch` only for a
    ticket. Rejected: resolving every unit's branch through
    `git.resolve_impl_branch` - it requires a ticket stem, so a non-ticket unit
    such as a delegate task has none.

## Constraints

- Shipped surface: the manual and playbooks ship downstream, so the new text
  names no repository-specific branch, path, or cadence
  (`shipped-surface-boundary.md`).
- Mirror: any `agents-plugin/rsrc/` edit follows `wsflow-mirroring.md`.
- Authoring: apply `skill-authoring.md`'s invariant checklist to every changed
  rule line.
- Budget: the workflow manual is injected into every session, so its new text
  stays at the approved prose below; anything a tool description or another
  manual section already says is not restated (user, 2026-10-30).
- Pinned text: `agents-plugin-tool/internal/mcp/playbook_tools_test.go` pins
  the current `## Parallel route (opt-in)` sentences; update those pins with
  the move.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/rsrc/, agents-plugin-wsflow/skills/, agents-plugin-tool/internal/wsdoc/conventions/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin/hooks/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)

## Prior Decisions

- 74073894 (2026-10-07, commit): "Release-before-merge fixes git.merge's impl-branch delete failing while the pooled worktree still has that branch checked out, observed in this session's 261007 parallel run." — bearing: supports
- 74073894 (2026-10-07, commit): "the approved batch is dependency-disjoint and merges stay serial with per-report approval, so holding merges until every report arrives buys nothing." — bearing: constrains
- 7d63642f (2026-09-21, commit): "Keep worktree.acquire generic: lead-run resolves the canonical ticket branch first instead of reconstructing a hash-derived name." — bearing: supports
- 260915-refactor-lead-run-playbook-diet-drop-assignment-note (2026-09-15, commit 0a1d5b5b): "Pins were kept only for structural fragments (section names, terminal lines, ... worktree lifecycle, ticket-batch-selector reference) and all use whitespace-normalized comparison" — bearing: constrains
- 261020-feat-worktree-profiles (2026-10-20, commit 5a7122c0): "workflow_manual fails on a bad committed profile, naming file and key, like it already does for a malformed committed config file" — bearing: constrains
- 261026-feat-delegate-default-with-plan-brief-and-post-hoc-review (2026-10-26, Decisions): "Default to delegate when the outcome is settled and you can name the files before starting." — bearing: supports
- 1d62d148 (2026-10-23, commit): "Run in parallel worktree mode next, at the user's direction." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/rsrc/lead-workflow-manual/lead-workflow-manual.md, agents-plugin/rsrc/lead-run/lead-run.md, agents-plugin-wsflow rsrc mirrors, agents-plugin-pi/rsrc mirrors guarded by agents-plugin-tool/internal/wsrsrc/pi_mirror_test.go, agents-plugin-tool/internal/mcp/playbook_tools_test.go |
| scope.surface | public-interface | shipped lead workflow manual injected into every session and the shipped lead-run playbook; no exported Go symbol |
| scope.new_public_symbol | no | none |
| scope.new_type_contract | no | none, prose only |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/playbook_tools_test.go pins near L3149-L3166; wsflow and pi mirror drift tests |
| complexity.reuse_points | confirmed | git.resolve_impl_branch, worktree.acquire, worktree.release, git.merge, ticket-batch-selector step 3 at agents-plugin/rsrc/ticket-batch-selector/ticket-batch-selector.md#L27-L35, injectWorktreeProfiles at agents-plugin-tool/internal/mcp/workflow_manual.go#L174-L189 |
| complexity.side_effect_risk | moderate | the manual is injected into every lead session in every downstream project, and three byte-identical copies must move together |
| risk.correctness | moderate | rules move between two surfaces and some current text is dropped, so a moved rule can be lost or duplicated |
| risk.fit | moderate | cross-surface split between the manual and lead-run under the shipped-surface and budget constraints |
| risk.test | moderate | existing pins must be rewritten to the new sentences and the pi mirror copy is not named in the phase |
| risk.security_or_contract | moderate | observable workflow behavior changes: merge conflicts no longer stop to the user and an instruction counts as per-run approval |

## Phases

### Phase 1: Worktree-based work mode in the workflow manual

Replace text exactly as below, per the decisions above. The prose was
approved verbatim by the user (2026-10-30), with Decisions 11 and 12 applied
to its first paragraph; `{{.McpNamespace}}` and
`{{.SkillNamespace}}` are the playbook template variables.

**Workflow manual** (`agents-plugin/rsrc/lead-workflow-manual/lead-workflow-manual.md`):
insert directly after the `### Git` section, before `### API documentation`.

```markdown
### Worktree-based work mode

Only when the user asks (the `### Git` housekeeping worktree excepted), work
in worktrees instead of moving the current worktree's branch: per unit, pass
a new branch as `target_branch` to `{{.McpNamespace}}/worktree.acquire` (for
a ticket, the one `{{.McpNamespace}}/git.resolve_impl_branch` returns).
Release that worktree before merging its branch or when the unit ends
unmerged; a worktree that keeps a branch checked out blocks
`{{.McpNamespace}}/git.merge` and every other checkout of it.

#### Parallel mode

Only a strict dependency (`blocked-by:`, a prerequisite `related:` or
`parent:`, or a body-stated need-first) orders units; overlapping files never
defer one, because the conflicts that matter are semantic and are checked at
merge. Merge units one at a time as they finish. When earlier merges moved the
target, first diff the target against the tree
`git merge-tree --write-tree <target> <branch>` prints, looking for
interactions with those merges. An interaction or a merge conflict goes to
`{{.SkillNamespace}}:lead-delegate`, which reruns the affected verification;
the user sees it only when the delegate cannot settle it.
```

**`lead-run`** (`agents-plugin/rsrc/lead-run/lead-run.md`): replace the whole
`## Parallel route (opt-in)` section with:

```markdown
## Parallel route (opt-in)

Serial is the default. This route runs the workflow manual's
`#### Parallel mode` over ready tickets and opens on one per-run user
approval, because provisioning is expensive in large or submodule-heavy
repositories; a user instruction to work in worktree or parallel mode is that
approval. The approved batch is the concurrency cap.

- Render `ticket-batch-selector` at its recommended tier; it owns the
  parallel-safety read. Present its `batch:` with the approval request, or
  report it and proceed when the instruction already approved.
- Provision each ticket by the manual's `### Worktree-based work mode`, skip
  Spawn step 3, and render its worker with `root_override: <worktree path>`
  and the acquired branch on the Branch line. A route verdict other than
  `continue` is followed as reported, not re-provisioned.
- Handle each report by **Handle the report** as it arrives.
```

**`ticket-batch-selector`** step 3: unchanged; its "overlapping files, whose
conflicts are resolved downstream" already states the criterion (Decision 4).

Text this replacement drops from the current `lead-run` section, on purpose:
the quoted per-run approval question (Decision 6), the "merge stop: surface
it" conflict rule (Decision 9), and the `worker_key` sole-owner and
`root_override`-in-manual wording (Decision 5). Every other current rule
lands in the manual or the new `lead-run` text.

Lease on a phase handoff: a `completion: phase` report ends the turn without
the unit continuing in place, so it counts as ending unmerged under
Decision 11; release the worktree, and the next cycle re-acquires the
retained branch. No prose change is needed for this reading.

Pins: keep the 260915 structural-fragment policy. Replace the parallel-route
pins with the section heading, the sentence naming the manual's
`#### Parallel mode`, the instruction-is-approval clause, and the
`root_override` clause; pin the manual's release-before-merge sentence and
the strict-dependency sentence in the workflow-manual render test.

Mirror the shared rsrc edits into `agents-plugin-wsflow` per
`wsflow-mirroring.md` and into `agents-plugin-pi/rsrc/` byte-identically
(Decision 10), and update the `playbook_tools_test.go` pins to the new
sentences.

Verification: the plugin's playbook render and drift tests pass for both
packages, `TestPiMirrorUpToDate` passes, the
worktree-profile render test is re-anchored so the `#### Worktree profiles`
list must precede `### Worktree-based work mode` (it currently anchors only on
`### API documentation`), the Go tests covering the workflow manual and `lead-run` render
pass, and a fresh read of the changed manual and `lead-run` text finds each
moved rule exactly once.

## Sage Review Round 1 (2026-10-30)

### Design Reviewer — block

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | Stopped-worker worktree release is dropped without a decision | important | missing |
| 2 | Profile-placement test loses its end-of-Git-section guarantee | minor | autonomous |
| 3 | Generic worktree mode provisions through a tool that requires a ticket | minor | autonomous |

### Completeness Reviewer — pass

| # | Title | Severity |
|---|-------|----------|
| 1 | Dropped lead-run text is not enumerated | minor |
| 2 | Verification does not name the worktree-profiles injection position | minor |
| 3 | Pin replacement targets left to the implementer | minor |
