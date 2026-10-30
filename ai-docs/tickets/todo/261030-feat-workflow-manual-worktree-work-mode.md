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
   differences, and report handling. Generic provisioning
   (`git.resolve_impl_branch` + `worktree.acquire`, profile, `root_override`,
   `worker_key` ownership), release before merge, serial merges, and the merge
   rules below move to the manual, and `lead-run` points at them. (User
   approval.)
6. **Instruction is approval.** A user instruction to work in worktree or
   parallel mode counts as `lead-run`'s per-run approval. Rejected: a second
   ask - asking again for what the user just requested adds a round trip with
   no new information.
7. **Semantic check at merge.** Before merging a branch onto a target that
   earlier merges moved, the lead reads what the branch adds on top of them,
   names any interaction, and delegates its resolution; verification reruns
   only when an interaction is found. (User answer.) Rejected: rerunning
   verification after every merge; checking Git conflicts only.
8. **Preview with `git merge-tree`.** The step 7 input is
   `git merge-tree --write-tree <target> <branch>` followed by
   `git diff <target> <result tree>`; on Git older than 2.38, read
   `git diff <target>...<branch>`. Rejected: a trial `git merge --no-commit`
   or a new `git.merge` dry-run parameter - merge-tree touches no worktree,
   index, or ref, so it works while the target is checked out elsewhere, and
   as a read-only diagnosis it already belongs to native Git under the
   manual's `### Git` rule.
9. **Conflicts go to a delegate.** A textual conflict `git.merge` leaves on
   the target, like a step 7 interaction, goes to `lead-delegate` as a
   bounded resolution task and reaches the user only when the delegate cannot
   resolve it or the resolution needs a product decision. (User answer.)
   Rejected: the current parallel-route merge stop that surfaces every such
   conflict to the user.

## Constraints

- Shipped surface: the manual and playbooks ship downstream, so the new text
  names no repository-specific branch, path, or cadence
  (`shipped-surface-boundary.md`).
- Mirror: any `agents-plugin/rsrc/` edit follows `wsflow-mirroring.md`.
- Authoring: apply `skill-authoring.md`'s invariant checklist to every changed
  rule line.
- Pinned text: `agents-plugin-tool/internal/mcp/playbook_tools_test.go` pins
  the current `## Parallel route (opt-in)` sentences; update those pins with
  the move.

## Phases

### Phase 1: Worktree-based work mode in the workflow manual

Add the section and subsection to the workflow manual, move the generic
parallel rules out of `lead-run` into it, keep `lead-run`'s entry-point parts
with a pointer, and keep `ticket-batch-selector` step 3 consistent with the
manual's criterion, per the decisions above. The verbatim prose is settled with the user before
the ticket is promoted and is recorded here.

Verification: the plugin's playbook render and drift tests pass for both
packages, and a fresh read of the changed manual and `lead-run` text finds
each moved rule exactly once.
