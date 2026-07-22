---
title: ws dashboard WorkRoot polishing backlog
parent: 260710-epic-ws-dashboard-terminal-ux-polishing
spec:
  - 260524-ws-dashboard-git-worktree-creation
  - 260524-ws-dashboard-git-aware-workroot-toolbar
---

# ws dashboard WorkRoot polishing backlog

## Background

The WorkRoot management MVP is complete enough for the current milestone:
opened roots persist, linked Git worktrees are discovered and creatable,
workspace/root removal has an owner-facing UI, and selected WorkRoots expose a
Git-aware toolbar with branch/status/fetch/push/fast-forward pull behavior.

This ticket keeps remaining WorkRoot management polish out of the MVP critical
path. Future sessions should add concrete dogfood issues here or split them
when they become implementation-ready.

## Phases

### Phase 1: WorkRoot lifecycle polish

**Priority (2026-07-22):** worktree delete is the immediate priority item in
this ticket. The atomic `git worktree remove` operation (force-gated on
uncommitted/untracked changes, plus dashboard registry cleanup and active
terminal-session accounting) described below is scoped and ready; Phase 3
below layers the now-finalized removal/hide UX decisions ("Agenda B") on top
of it. Implement the daemon op and the Phase 3 UX together rather than
sequencing UX as a follow-up.

Improve owner operations for active, unavailable, linked, remembered, pinned,
recoverable, and prunable WorkRoots without turning the root picker into a
generic file manager.

Candidate areas include clearer unavailable/recovery states, remove/forget copy,
refresh timing, pinned directory behavior, workspace grouping clarity, and
worktree lifecycle edge cases.

- **Git worktree deletion is missing entirely** (found 2026-07-11 dogfood
  discussion): `crates/daemon/src/git_worktree.rs` only implements
  `git_worktree_add_options`/`_preview`/`_submit` (create-only); there is no
  `remove`/`prune`/`list` daemon operation, and the frontend has no
  delete/remove worktree command (only "Add worktree"). Deleting a worktree
  today requires bypassing the dashboard entirely and running
  `git worktree remove` manually in a terminal. Worse, `root_picker.rs`'s
  `remove_workspace` is misleadingly named — it only unregisters the
  workroot from the dashboard's own `OpenedWorkRoots` registry
  (`persistent_state.rs`) and never touches disk or runs
  `git worktree remove`. The two "delete" concepts are fully decoupled:
  removing via the dashboard leaves the real worktree directory and
  `.git/worktrees/` metadata behind (stale entries in `git worktree list`
  forever), while removing via terminal leaves a zombie entry in the
  dashboard registry. A real delete operation should atomically run
  `git worktree remove` (with an explicit force option gated on
  uncommitted/untracked changes) and clean up the dashboard registry entry
  together, and must account for active terminal sessions still using that
  worktree.
- **Long-term worktree placement direction** (owner, 2026-07-11): once this
  phase's real delete operation lands, move default worktree placement out
  of `<root>/.git/ws-worktree/<name>` and into `<root>/.ws-dashboard/worktrees/`
  (see `260711-idea-dashboard-workroot-scoped-artifact-consolidation`'s
  Decisions for the naming rationale). Owner judged migration risk low:
  worktree discovery is not path-bound today (worktrees are found via `git
  worktree list`, not a hardcoded expected path) and no placement under
  `.git/ws-worktree/` has ever shipped, so there is no compatibility clutter
  to carry forward. This should be sequenced as part of implementing this
  phase's delete/create path changes, not as a separate migration.

Verification should include resource model tests where possible plus browser
coverage for any visible navigation or lifecycle behavior changed.

### Phase 2: Git toolbar polish

Tune the selected WorkRoot Git surface as dogfood issues appear. Candidate areas
include branch dropdown ergonomics, new-branch flow feedback, status polling
load, push/pull/fetch progress affordances, long branch names, and stale status
recovery.

- **Status polling risks `.git/index.lock` contention** (found 2026-07-11
  dogfood, Windows): the 5s interval timer (`gitToolbar.ts:231-253`) plus
  focus/visibility-triggered refreshes have no in-flight/single-flight
  guard, client- or server-side, so overlapping `git status`/`git branch`
  invocations against the same repo can race on the index lock. See
  `260711-idea-dashboard-git-status-polling-index-lock-contention` for the
  full investigation and candidate fix directions (single-flight guard,
  `--no-optional-locks`, and/or a longer-term watch-based rearchitecture).

This phase should preserve the current safety decision that dashboard-triggered
pulls use `git pull --ff-only` and never attempt conflict resolution.

Verification should include daemon Git tests where practical and browser
coverage for any changed Git interaction.

### Phase 3: Worktree removal & hide UX (Agenda B, finalized 2026-07-22)

Owner finalized the "Agenda B — worktree UX" decisions below during dogfood
discussion on 2026-07-22. These are **final**; implement as specified rather
than re-litigating. They layer onto the atomic `git worktree remove` operation
already scoped in Phase 1 (force-gated on uncommitted/untracked changes, plus
registry cleanup and active terminal-session accounting) — that operation's
scope is unchanged, this phase adds the surrounding UX.

- **B-1 (removal confirmation, always shown):** worktree removal always shows
  a confirmation modal before running, regardless of clean/dirty state —
  worktree add/remove is a heavy operation, so confirmation is not
  conditional on there being something to lose. When the worktree has
  uncommitted or untracked changes, the modal must additionally surface a red
  data-loss warning calling that out explicitly (distinct from the baseline
  confirmation copy).
- **B-2 (optional branch deletion):** the same confirmation modal includes a
  "delete branch too" checkbox, **default OFF** (branch preserved unless the
  owner opts in). Before submitting, check whether the branch has unmerged /
  dangling commits — i.e. commits not reachable from another ref, the same
  condition under which plain `git branch -d` would refuse. If so, show a red
  parenthetical warning next to the checkbox (example UI string: "아직
  머지되지 않았습니다"). If the branch is safe to delete (no unique commits,
  or already merged), checking the box simply deletes it — never silently
  force-delete a branch with dangling commits.
- **B-3 (hide worktrees, pure UI):** add a hide action that is presentation-only
  — the worktree directory stays on disk and its branch is untouched; hiding
  only removes it from the dashboard's visible list. Restore path: the root
  workRoot's right-side "..." settings menu gains a "hidden worktrees"
  submenu; clicking a hidden entry there un-hides it. This supersedes the
  prior direction recorded in
  `260523-research-ws-dashboard-persistable-ui-state-map` (which rejected
  invisible worktrees in favor of a future explicit forget/remove action) —
  see that ticket's 2026-07-22 update for the reversal record.

Verification should include resource model tests for the branch
unmerged/dangling check and browser coverage for the confirmation modal
(both warning states) and the hide/unhide flow.
