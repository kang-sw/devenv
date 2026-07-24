---
title: ws dashboard WorkRoot polishing backlog
parent: 260710-epic-ws-dashboard-terminal-ux-polishing
spec:
  - 260524-ws-dashboard-git-worktree-creation
  - 260524-ws-dashboard-git-aware-workroot-toolbar
  - 260722-ws-dashboard-git-worktree-removal
  - 260722-ws-dashboard-worktree-removal-hide-ux
sage-review-design: required
sage-review-completeness: required
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

**Priority (2026-07-22)** *(fulfilled — the daemon remove op + Phase 3 UX
shipped, see Result (bf7deab0); retained for historical context)***:** worktree
delete is the immediate priority item in
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
  invocations against the same repo can race on the index lock. **Update
  2026-07-24:** the near-term `--no-optional-locks` fix already SHIPPED via
  `260714-bug-git-status-poll-index-lock-staleness` (`.done/`; the toolbar
  poll now passes `--no-optional-locks`, documented in the Git-Aware WorkRoot
  Toolbar spec). Remaining open direction narrowed to the long-term
  watch-based rearchitecture (single-flight guard deprioritized) — see
  `260711-idea-dashboard-git-status-polling-index-lock-contention` (`todo/`).

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

### Result (bf7deab0) - 2026-07-22

Shipped the worktree-removal slice: Phase 3 UX (B-1/B-2/B-3) plus the Phase 1
daemon `git worktree remove` op it depends on. Commits: `f0843859` (daemon
remove op), `2e373a9c` (frontend removal/hide UX + `g w h` hotkey), `bf7deab0`
(review fixes).

Daemon op: preview/submit two-step, 409 force-gate (explicit force only),
runs `git worktree remove`/branch-delete from the workspace PRIMARY git root,
non-destructive branch delete (`git merge-base --is-ancestor` detection,
`git branch -d` only, unmerged branch kept with reported skip, checkbox
default OFF), atomic registry unregister+persist with DISK-as-source-of-truth
on persist failure (no re-register; terminal/codex/claude session cleanup
still runs).

Frontend: B-1 always-real confirm modal + conditional data-loss notice + 409
in-place preview refresh; B-2 default-OFF delete-branch checkbox +
unmerged keep-warning; B-3 pure browser-local hide/unhide (`workNavOrder`)
fully decoupled from the daemon registry (not gated on removal
availability); `g w h` hotkey reveals the hidden-worktrees section.

Specs: documented under new anchors
`260722-ws-dashboard-git-worktree-removal` and
`260722-ws-dashboard-worktree-removal-hide-ux` in
`ai-docs/spec/ws-web-dashboard/index.md`.

Review: 3-partition review (correctness/fit/test) found 1+3+4 Important
issues, all resolved in `bf7deab0`; a focused opus re-review returned clean.
Safety invariants verified load-bearing by daemon tests
(rollback-persist-failure, codex/claude session cleanup, primary-root argv
builder) plus frontend unit tests.

KNOWN LIMITATION: ticket-mandated e2e browser coverage (confirm-modal
warning states + hide/unhide flow) was AUTHORED in
`dashboard-acceptance.spec.ts` and type-checks, but could not be run to
green due to a PRE-EXISTING acceptance-harness locator ambiguity
(`openWorkRootInBrowser` / `[data-command-id="rootPicker.open"]` also
matching the `.open-work-root-empty-cta` empty-state CTA from commit
`21116b54`), which red-lines the whole acceptance suite at step 1. Tracked
in idea ticket `260722-bug-e2e-open-work-root-locator-ambiguity`.

SCOPE REMAINING (ticket stays in `ready/`, do NOT move it to `.done/`):
Phase 1 full WorkRoot lifecycle polish + placement migration (only the
daemon remove-op was borrowed for this slice), and Phase 2 git toolbar
polish / index.lock handling.

### Result (1bb24f25) - 2026-07-24

Shipped the Phase 1 worktree default-placement migration slice (the
"Long-term worktree placement direction" item): new-worktree Auto placement
moved from `<root>/.git/ws-worktree/<name>` to
`<root>/.ws-dashboard/worktrees/<name>`. Commits: `ea64fded` (survey plan),
`1bb24f25` (implementation).

Daemon (`crates/daemon/src/git_worktree.rs`): `GitWorkspaceContext` gains a
`worktree_base_dir` computed once in `resolve_workspace_git` from
`common_dir.parent()/.ws-dashboard/worktrees` (falls back to the old
`common_dir/ws-worktree` shape only if `common_dir` unexpectedly has no
parent); `resolve_preview_with_context`'s `Auto` arm now joins that field
instead of recomputing `common_dir.join("ws-worktree")`. New best-effort
`ensure_worktree_base_dir_excluded` helper appends `/.ws-dashboard/worktrees/`
to `<root>/.git/info/exclude` (idempotent, silent on I/O failure) so the new
in-tree placement stays invisible to the root repo's `git status` — preserving
the free invisibility the old `.git/`-internal placement had. The now-dead
`common_dir` struct field was removed. UI base-dir labels and the spec's
"Automatic path naming" prose (`260524-ws-dashboard-git-worktree-creation`)
updated to `.ws-dashboard/worktrees`.

Migration risk confirmed low: `discovery.rs` finds worktrees via
`git worktree list --porcelain`, not a path pattern, so pre-existing
`.git/ws-worktree/*` worktrees keep working with no backfill.

Verification: daemon `git_worktree` suite (14/14 pass) including a NEW
`git status --porcelain` cleanliness assertion in `routes.rs`, independently
confirmed load-bearing (fails without the `.git/info/exclude` step); frontend
`test:git` green.

e2e status correction: an earlier assumption (carried into the survey plan
from the prior Phase 3 Result) that `dashboard-acceptance.spec.ts` is
type-check-only because the `openWorkRoot` locator ambiguity blocks it is
STALE — `260722-bug-e2e-open-work-root-locator-ambiguity` was already fixed
and closed (`e2290cdd`) before this slice. The test partition re-ran Playwright
and confirmed the suite runs past this diff's touched worktree steps
(B-1 data-loss scratch path / hide-unhide) successfully; the suite fails only
later at an unrelated agent-chat assertion, out of scope for this slice.

Review: 3-partition (correctness clean, fit clean, test 1 Important + 1 Minor).
Important = the stale e2e "type-check-only" characterization above, adopted by
recording the accurate status here (no code change). Minor = no test for the
`common_dir.parent()`-is-`None` fallback branch, plan-sanctioned out of scope.

SCOPE REMAINING after this slice: broad Phase 1 WorkRoot lifecycle polish
(unavailable/recovery states, remove/forget copy, refresh timing,
pinned-directory behavior, workspace grouping) and Phase 2 git toolbar polish
(remaining index.lock watch-based rearchitecture). The placement migration is
now DONE.

### Status demoted ready -> todo (2026-07-24)

Both concrete implementation-ready slices this ticket carried have now shipped
(worktree remove op + Phase 3 removal/hide UX in Result bf7deab0; default
placement migration in Result 1bb24f25). A combined sage review on 2026-07-24
(design + completeness) returned `concern`/`concern`: the ticket is coherent as
a deliberate living backlog, but its remaining Phase 1/Phase 2 scope is
"candidate areas" with no concrete chosen behavior, so a goal-loop step that
picks it as a `ready/` implementation target would stall at proceed with nothing
buildable. Per that finding this ticket is demoted to `todo/` (accepted backlog);
re-promote a concrete slice to `ready/` — or split one out — once an owner
concretizes which polish item matters and its target behavior. Sage-review
frontmatter is left `required` so that future ready re-promotion re-runs the
gate against the concretized scope.
