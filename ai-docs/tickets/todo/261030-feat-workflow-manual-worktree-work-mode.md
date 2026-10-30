---
title: Workflow manual worktree-based work mode with a parallel subsection
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

## Phases

### Phase 1: Worktree-based work mode in the workflow manual

Replace text exactly as below, per the decisions above. The prose was
approved verbatim by the user (2026-10-30); `{{.McpNamespace}}` and
`{{.SkillNamespace}}` are the playbook template variables.

**Workflow manual** (`agents-plugin/rsrc/lead-workflow-manual/lead-workflow-manual.md`):
insert directly after the `### Git` section, before `### API documentation`.

```markdown
### Worktree-based work mode

Only when the user asks (the `### Git` housekeeping worktree excepted), work
in worktrees instead of moving the current worktree's branch: per unit,
resolve its branch with `{{.McpNamespace}}/git.resolve_impl_branch` against
the exact base and pass it as `target_branch` to
`{{.McpNamespace}}/worktree.acquire`. Release that worktree before merging
its branch; `{{.McpNamespace}}/git.merge` cannot delete a branch a worktree
still has checked out.

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

Mirror the shared rsrc edits into `agents-plugin-wsflow` per
`wsflow-mirroring.md`, and update the `playbook_tools_test.go` pins to the new
sentences.

Verification: the plugin's playbook render and drift tests pass for both
packages, the Go tests covering the workflow manual and `lead-run` render
pass, and a fresh read of the changed manual and `lead-run` text finds each
moved rule exactly once.
