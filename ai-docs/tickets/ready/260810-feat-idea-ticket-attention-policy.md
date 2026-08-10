---
title: Idea ticket attention policy — topic-scope idea/ in worktrees, fold orphan idea/ in project_tree
sage-review-design: completed
related:
  260807-refactor-dissolve-project-index: related — both edit project_tree's renderTickets; this ticket's orphan-idea fold is orthogonal to that ticket's _index dissolution but shares the same surface
sage-review-completeness: completed
---

# Idea ticket attention policy — topic-scope idea/ in worktrees, fold orphan idea/ in project_tree

## Background

`ai-docs/tickets/idea/` is the mid-work capture surface: a dogfood surprise
becomes an `idea/` ticket on the spot, so idea/ must stay writable and
stageable at all times. But the backlog is large (56+ tickets), and idea/ is
attention noise in two distinct surfaces:

- A **topic-scoped work worktree** (`lead-scope-worktree`) force-includes *all*
  of idea/ via a carve-out, so every off-topic idea competes for attention even
  in a worktree deliberately narrowed to one work line.
- The **unscoped main worktree**'s `project_tree` dumps every idea ticket (with
  `parent:`/`related:` suffixes) into the session-bootstrap view.

The underlying tension is uniform: idea/ must be **capturable** (present on
disk, stageable) yet is **noise for focused attention**. Today each surface
conflates those two axes onto a single mechanism, which is why idea/ cannot be
quieted without breaking capture.

## Decisions

- **Two surfaces, two complementary mechanisms — not one lever.**
  - *Scoped work worktree* → resolve at the **sparse-checkout layer**: genuinely
    exclude idea/ from disk (strongest focus), and preserve capture with a forced
    tracked `.gitkeep` plus `--sparse` staging.
  - *Unscoped main worktree* → resolve at the **render layer**: `project_tree`
    folds orphan idea/ into a count line; sparse-checkout does not apply there.
- **`--sparse` staging over auto-widen.** A captured off-topic idea is staged
  with `git add --sparse`, so it commits but **self-hides** in this worktree
  (skip-worktree) while remaining fully present in the index and other
  worktrees — which is correct, since it is off-topic here. Rejected: auto-widen
  the sparse pattern on capture, which keeps the captured off-topic idea visible,
  grows clutter, and strengthens the disk=attention coupling this ticket unwinds.
- **Forced `.gitkeep` keeps the directory alive.** Verified (git 2.43): a `--no-cone`
  scope that excludes `idea/*` removes the directory from disk once its only
  tracked files are scoped out. A tracked, re-included `ai-docs/tickets/idea/.gitkeep`
  keeps idea/ materialized while existing idea tickets hide — necessary so writes,
  `tickets.create`, and the promote-into-hidden-dir remedy do not hit a vanished
  directory. Rejected: excluding idea/ without `.gitkeep`.
- **`.gitkeep` is necessary but not sufficient.** Verified: a new out-of-scope idea
  file is refused by plain `git add` ("outside of your sparse-checkout definition,
  so will not be updated"); only `git add --sparse` stages it. The ws mutation guard
  `scopeBlockedMoveError` does **not** run on creation — it guards
  `tickets_mutate.go`'s move/close of existing tickets. `tickets.create_empty`
  writes the file directly and does not stage, so nothing ws-side refuses a fresh
  non-colliding idea create today; the refusal surfaces only when the commit path
  tries to stage the file. So the commit/staging path must opt into `--sparse`;
  `.gitkeep` alone (directory present) still leaves the capture uncommittable.
- **Staging locus: `ws/git.commit`, not `TicketCreate`.** The `--sparse` staging
  lands in `ws/git.commit`'s explicit-path staging — git.commit already stages only
  the caller-named `paths` (not `-A`), so treating a named path as intentional and
  staging it with `--sparse` is the thinnest committable-capture fix, keeps
  `tickets.create_empty` stage-free (its documented contract), and matches the
  principle that staging rides the normal commit flow. Guardrail: never stage a
  deletion of an absent skip-worktree path (present-on-disk additions/updates only).
  Rejected: an idea-targeted staging step (special-cases idea in the commit layer,
  and git.commit cannot tell an idea capture from any other path); threading a
  `GitRunner` into `TicketCreate` (breaks its no-git signature and the
  server.go call site, and its "does not stage or commit" contract).
- **Parented-idea exception (project_tree).** An idea that is an epic child is
  planned decomposition work, not speculative backlog. Render keeps parented idea
  (and all ready/todo) visible and folds only **orphan** idea, preserving epic
  decomposition legibility in the tree. Rejected: folding parented idea too.
- **Count line over full omission (project_tree).** The fold emits a hidden-count
  line, preserving the "N ideas exist" signal at zero recovery cost (full bodies
  reachable via `tickets.list(status:"idea")`). This is the cheap-miss side of the
  cost-of-miss doctrine — the agent self-selects on demand. Rejected: silent full
  omission, which loses the existence signal.
- **Active-glob display deferred.** Showing the worktree's active sparse
  re-include globs in the workflow_manual banner easily becomes noise; instead the
  banner carries a one-line pointer that `git sparse-checkout list` reveals the
  active topic. The globs themselves are out of scope.

## Phases

### Phase 1: Bring idea/ into worktree topic-scope with capture preserved

The scoped-worktree half. `lead-scope-worktree`, the scope-aware mutation guard,
and the workflow_manual scope banner change so idea/ participates in topic
scoping like ready/todo, while mid-work capture still works.

- **`lead-scope-worktree` playbook** (and its wsflow mirror per
  `ai-docs/ref/wsflow-mirroring.md`): remove the "idea/ always stays visible"
  Invariant/carve-out. The derived pattern additionally excludes
  `/ai-docs/tickets/idea/*` and re-includes a tracked
  `/ai-docs/tickets/idea/.gitkeep`. The mandatory verify-by-listing step extends
  to idea/. The widen-then-move remedy for promoting into a hidden status
  directory (documented in `ai-docs/ref/worktree-ticket-scope.md` and the
  `lead-scope-worktree` playbook) now also governs idea→todo triage under an
  active scope.
- **Forced `.gitkeep`.** The scope skill ensures a tracked
  `ai-docs/tickets/idea/.gitkeep` exists (create + commit if absent) before
  applying a pattern that scopes idea/ out, so the directory never vanishes.
- **Capture staging under scope.** Current behavior (verified): `tickets.create_empty`
  does **not** stage or commit — `TicketCreate` (`agents-plugin-tool/internal/wsdoc/ticket_create.go`)
  writes the new file with a plain `os.WriteFile` and never invokes git. Its only
  scope-aware refusal is a duplicate-stem-in-index collision (a hand-written error),
  **not** `scopeBlockedMoveError`, which belongs to
  `agents-plugin-tool/internal/wsdoc/tickets_mutate.go`'s move/close of an
  *existing* ticket. So a genuinely new, non-colliding idea stem is written to disk
  regardless of scope — as long as the directory exists (hence forced `.gitkeep`).
  The break is at **commit** time: `ws/git.commit`'s staging runs a plain `git add`,
  which refuses the out-of-scope new idea path. The fix (locus decision, see
  Decisions): **`ws/git.commit` stages its explicit `paths` with `git add --sparse`**
  under an active scope, so a caller-named out-of-scope path (notably an off-topic
  idea capture) is committable and then self-hides in this worktree. `TicketCreate`
  and `tickets.create_empty` are untouched (they keep their "does not stage or
  commit" contract). Guardrail: the `--sparse` staging must never stage a *deletion*
  of an absent skip-worktree path — only additions/updates of paths present on disk.
  This interacts with the existing `#260513-git-commit-result-edition-detection`
  deletion-staging path (git.commit stages concrete removed paths git status
  reports): the implementer must distinguish an absent-because-sparse-hidden path
  (skip) from an absent-because-genuinely-deleted path (stage the deletion).
  This does not touch the existing cross-scope refusals in `tickets_mutate.go`
  (ready/todo moves, idea→todo promotion into a hidden dir keep widen-then-retry).
- **`agents-plugin-tool/internal/mcp/scope_announcement.go`**: add `idea` to the
  counted status list (currently `["ready", "todo"]`) so hidden idea tickets are
  counted; update the banner text ("hidden in ready/ and todo/") to include idea/;
  append a one-line pointer that `git sparse-checkout list` reveals this worktree's
  active topic. This also removes an existing asymmetry — `project_tree`'s
  `ticketScopeAnnotation` (`agents-plugin-tool/internal/mcp/server.go`) already
  passes `["ready", "todo", "idea"]`, only `scopeAnnouncement` is stuck on
  ready/todo.

Constraints: the `--sparse` relaxation must **not** widen the sparse pattern (no
auto-widen) — the captured file stays outside scope, staged but hidden. Do not
relax the guard for non-idea statuses.

Verification: under an active scope excluding `idea/*` —
1. `ls ai-docs/tickets/idea/` shows only `.gitkeep`; existing idea tickets absent.
2. `tickets.create_empty(initial_state:"idea")` writes the new file under idea/
   (directory present via `.gitkeep`), and a subsequent `ws/git.commit` on that path
   stages and commits it via `git add --sparse` with no manual widen.
3. the committed idea file is absent from `ls` after a fresh sparse re-apply
   (self-hidden) yet present in the index and in a full/other-worktree checkout.
4. a fresh `workflow_manual` call reports the hidden idea count and the
   `git sparse-checkout list` pointer.
5. an attempted cross-scope ready/todo move still refuses with the widen tip.

### Result (585af568) - 2026-08-10

Landed on `impl/idea-attention-scope-worktree`, range `b34ff4b3..585af568`
(6 commits), merged into goal branch `goal/main/copper-lantern-marsh`.

- **`lead-scope-worktree` playbook + shims.** Canonical
  `agents-plugin/rsrc/lead-scope-worktree/lead-scope-worktree.md` dropped the
  "idea/ always stays visible" carve-out; the derived `--no-cone` pattern now
  excludes `/ai-docs/tickets/idea/*` and re-includes tracked
  `/ai-docs/tickets/idea/.gitkeep`, with the forced-`.gitkeep`, verify-by-listing,
  and widen-then-move-for-idea→todo steps added. The wsflow rsrc mirror + both
  `manifest.json` were regenerated (env-gated, not hand-edited); both SKILL.md
  `description:` shims (ws + wsflow trees) hand-edited to drop the carve-out phrase.
- **`git.commit` `--sparse` staging.** New `wsgit.CommitOptions.SparseScopeActive`
  bool threads into `stagingCommandsForCommit`, adding `--sparse` to the `add`
  branch only — never `rm --cached`. Deletion-safety holds because a tracked,
  skip-worktree, on-disk-absent, unmodified path never appears in pre-staging
  `git status --porcelain=v2`, so the existing `#260513`
  `deletedPathsUnderCommitRoot` routing can never misclassify a sparse-hidden path
  as a deletion (correctness partition confirmed this; a synthetic regression test
  pins the "present in paths, absent from status snapshot → add, not rm" branch).
  The flag is computed by `internal/mcp` via new `wsdoc.SparseCheckoutActive(root)`
  and passed across the `{#260720-wsdoc-commit-boundary}` import ban, mirroring the
  existing Verifier/verifyAdapter bridge. `TicketCreate`/`tickets.create_empty`
  untouched (stage-free contract preserved).
- **`scope_announcement.go`.** Counted-status list gained `idea`; banner names
  idea/ and carries a `git sparse-checkout list` topic-recovery pointer, closing
  the prior asymmetry with `project_tree`'s already-idea-aware annotation.
- **Specs.** `spec/mcp-tools.md` gained `{#260810-scope-announcement-idea-inclusion}`
  and `{#260810-git-commit-sparse-staging}`; `spec/workflow-skills.md`
  `lead-scope-worktree` roster trimmed to the uniform-scope fact plus a terse
  cross-ref (mechanism sole-sourced in mcp-tools.md). Mental-model updater found
  no gap — the modification reasoning is sole-sourced in the spec anchor + code
  comments, so no model edit.

Deviation: `spec/workflow-skills.md` had no literal "idea/ always stays visible"
clause at the plan's cited location; the removal became a positive uniform-scope
statement, later trimmed to a terse pointer on fit review. Verification: full Go
build/vet, `wsgit`/`mcp`/`wsdoc` test packages, both env-gated regen tests
(zero residual diff), and the python wsflow bundle (10/10) all green; partitioned
review clean (correctness clean first pass; fit + test clean on re-review).

Phase 2 (project_tree orphan-idea fold) remains — ticket stays in `ready/`.

### Phase 2: Fold orphan idea/ in project_tree, keep parented idea and ready/todo

The unscoped-worktree half, independent of Phase 1 (sparse-checkout does not help
in the full checkout).

- **`agents-plugin-tool/internal/wsdoc/project_tree.go` `renderTickets`**: keep
  rendering ready/ and todo/ in full.
  For idea/, render in full only tickets carrying a `parent:` (epic children —
  planned decomposition), and fold the remaining **orphan** idea tickets into a
  single hidden-count line (e.g. `idea: N orphan hidden — tickets.list status=idea
  to view`). ready/todo output is unchanged.
- The count line preserves the existence signal at zero recovery cost; full idea
  bodies stay reachable via `tickets.list(status:"idea")` / `tickets.find`.

Verification: `project_tree` renders every parented idea (with its
`parent:`/`related:` suffixes) and all ready/todo exactly as today; orphan idea
tickets collapse to one count line whose N equals the orphan idea count; the
folded stems are absent from the tree body but returned by
`tickets.list(status:"idea")`.

## Spec Impact

Multi-anchor amendment across two spec files; no single new stem covers this
policy, so the changes are recorded here per section.

- **`spec/mcp-tools.md`** — workflow_manual sparse-checkout scope announcement (the
  block rendered on the fresh-with-root and continue branches): the counted/hidden
  statuses extend to include `ai-docs/tickets/idea/`, and the block gains a
  one-line pointer to `git sparse-checkout list` for recovering the worktree's
  active topic. Caller-visible change: the banner counts hidden idea tickets and
  names the topic-recovery command.
- **`spec/mcp-tools.md`** — `git.commit` staging (the section stating it stages only
  the requested paths, `#260513-git-commit-result-edition-detection` area): under an
  active worktree sparse-checkout, explicit-path staging uses `git add --sparse` so a
  caller-named out-of-scope path (notably an off-topic idea capture) is committable,
  guarded so an absent skip-worktree path is never staged as a deletion. This is a
  *new* staging behavior, not a relaxation of an existing refusal —
  `tickets.create_empty` has no creation-time scope refusal today beyond the
  duplicate-stem-in-index case, which is unaffected, and the `tickets_mutate.go`
  cross-scope refusals are unchanged. Caller-visible change: capturing an off-topic
  idea under an active scope no longer strands the file uncommittable.
- **`spec/mcp-tools.md`** — `project_tree` ticket inventory: the inventory renders
  parented idea plus all ready/todo in full and folds orphan idea tickets into a
  hidden-count line; full idea bodies remain reachable via the discovery tools.
  Caller-visible change: project_tree no longer dumps every idea ticket.
- **`spec/workflow-skills.md`** — `lead-scope-worktree`: the worktree topic scope now
  covers `ai-docs/tickets/idea/` (the always-visible carve-out is removed), with a
  forced tracked `.gitkeep` keeping the directory materialized and `--sparse`
  capture keeping mid-work idea creation working. Caller-visible change: a scoped
  worktree hides off-topic idea tickets.
