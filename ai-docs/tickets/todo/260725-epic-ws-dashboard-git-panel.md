---
title: Dashboard git panel — log graph, diff view, and local review comments
sage-review-design: required
related:
  260710-epic-ws-dashboard-terminal-ux-polishing: sibling board; owns UX/visual polish of existing surfaces, explicitly excludes new feature surfaces like this one
  260524-research-ws-dashboard-visual-design-system-refresh: design-language research this epic actions via the git widget as the first vertical slice
related-mental-model:
  - ws-web-dashboard
---

# Dashboard git panel — log graph, diff view, and local review comments

## Scope

A new git surface in the dashboard, built as a `Files | Git` tabbar over the
bottom-left file browser. The Git tab shows a git log graph; commits/labels open
a GitHub/GitLab-style diff view in the right pane; the diff view supports single
commit and commit-range diffs and a local code-review comments layer whose
payload can be copied to the clipboard for an agent to act on.

This is greenfield feature work (new surfaces), so it lives on its own board
rather than under `260710` (whose Non-Scope excludes new feature surfaces). It
also carries the epic-owned **dashboard-local design guide**: the git widget is
the interaction-heavy vertical slice used to establish a coherent visual/design
language and retire the current `ai-docs/ref/design.md` (an IBM Carbon marketing
extraction, wrong for a dark operational dashboard). This actions the hypothesis
already recorded in `260524-research-ws-dashboard-visual-design-system-refresh`.

## Non-Scope

- Team-shared / server-side review comments (GitHub-PR-style shared threads with
  auth). Comments here are local-only. A shared-review backend is a separate,
  larger effort and is intentionally out of scope.
- Git write operations beyond what already exists in the git toolbar
  (branch switch/create, fetch/push/pull). This epic is read/inspection +
  local comments; it does not add commit/stage/rebase UI.
- Full dashboard-wide visual rewrite. The design guide is seeded by this slice
  and grows incrementally; retiring `ref/design.md` does not mean restyling every
  existing surface in this epic.

## Child Tickets

- `260725-feat-ws-dashboard-design-guide` - dashboard-local design guide
  (tokens + primitives), seeded by the git widget; replaces `ref/design.md`.
  Cross-cutting; the other children build to it.
- `260725-feat-ws-dashboard-git-tab-log-graph` - `Files | Git` tabbar over the
  bottom-left file browser + the git log graph panel (UI 1): graph render,
  relative/absolute commit time, per-commit change-file counts, `[ ] ALL`
  toggle, topmost unstaged-status label.
- `260725-feat-ws-dashboard-git-diff-view` - the diff view (UI 2) in the right
  pane: GitHub/GitLab-style file-list + single-scroll diffs with "show more"
  collapse, single-commit and commit-range ("diff from here") modes; plus the
  daemon git log/diff/show endpoints the panel and view need.
- `260725-feat-ws-dashboard-git-review-comments` - local code-review comments on
  commit lines/ranges, stored in a gitignored `.ws-dashboard/reviews/` store via
  new daemon endpoints, with a "copy comments to clipboard" action emitting
  `<code-comments>…</code-comments>` payloads for an agent.

## Cross-Child Decisions

- **Design guide is authoritative for all new components.** Every new component
  in this epic (tabbar, log graph, diff view, comment gutter) is built to the
  `260725-feat-ws-dashboard-design-guide` tokens/primitives, not ad-hoc CSS. The
  diff view is the interaction-heavy proving slice.
- **Reuse dependencies over reinvention.** The diff view is built on
  `@codemirror/merge` (CodeMirror 6 is already a dependency and powers the
  document viewer) rather than a hand-rolled hunk renderer or a second diff
  stack. Pull in libraries actively where they raise UX quality.
- **Daemon git access extends the existing shell-out pattern.** New log/diff/show
  endpoints use `git_toolbar.rs`'s `git_text`/`run_git` shell-out and register
  alongside the existing `/git/*` routes. No git library is introduced.
- **Right pane reuse.** The diff view is hosted via the existing read-only pane
  path (`openReadOnlyFile` → pane placement → `readOnlyWorkbenchPane` body) and
  the already-declared-but-unwired `diff` SurfaceKind
  (`surfaceRegistry.ts`). It does not invent a new placement system.
- **Comments are local, file-backed, not git-tracked.** Stored under a gitignored
  `.ws-dashboard/reviews/` store (a new per-repo local-data concept — only
  `.ws-dashboard/worktrees` exists today), keyed by commit hash → file →
  line-range, with a separate working-tree bucket. Rationale: GitHub PR comments
  are permanent in GitHub's DB, not in git; tracking review notes in the repo
  pollutes history. File-backed (not browser localStorage) so an agent/other
  tools can read them.
- **Comment anchoring is best-effort for working-tree targets.** Commit-hash
  targets are stable. Working-tree comments may go stale on edit; instead of
  precise re-anchoring, the clipboard copy specializes for working-tree targets —
  it embeds the original code segment plus a "position may differ" note, so the
  payload is self-correcting regardless of line drift.

## Completion Criteria

- Done: the Git tab (log graph), the diff view (single + range), and local review
  comments with clipboard export are usable for real code review in the dogfood
  dashboard, and the new components are built to a landed dashboard-local design
  guide that has replaced `ref/design.md`.
- Dropped: the dashboard adopts an external git-UI surface, or the git-inspection
  direction is abandoned.
- Deferred: shared/server-side review comments; git write/mutation UI; broad
  restyle of pre-existing surfaces to the new design guide.
