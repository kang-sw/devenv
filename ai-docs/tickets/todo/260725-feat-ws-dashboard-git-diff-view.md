---
title: Git diff view (UI 2) — GitHub/GitLab-style, single-commit and range
sage-review-design: required
parent: 260725-epic-ws-dashboard-git-panel
related:
  260725-feat-ws-dashboard-git-tab-log-graph: log graph triggers that open this view
  260725-feat-ws-dashboard-git-review-comments: comment gutter layered onto this view
  260725-feat-ws-dashboard-design-guide: built to this guide (interaction-heavy proving slice)
related-mental-model:
  - ws-web-dashboard
---

# Git diff view (UI 2) — GitHub/GitLab-style, single-commit and range

## Background

There is no diff-rendering UI in the dashboard today, but a `diff` SurfaceKind is
already declared-but-unwired (`surfaceRegistry.ts`) and the read-only right-pane
path is fully reusable. This ticket builds the diff view (UI 2): a
GitHub/GitLab-style inspection surface opened in the right pane, and the daemon
git diff/show/log-range plumbing behind it.

## Decisions

- **Host in the right pane, reusing the document path.** Open via the existing
  `openReadOnlyFile` → pane placement → `readOnlyWorkbenchPane` body flow,
  branching `readOnlyWorkbenchPane` on a diff pane type and wiring the reserved
  `diff` SurfaceKind (`surfaceRegistry.ts`). Default placement = right, same as
  documents.
- **Copy GitHub/GitLab diff UX, do not reinvent — build on `@codemirror/merge`.**
  CodeMirror 6 is already a dependency (powers the document viewer), so the diff
  renderer reuses it for consistent highlighting/theming instead of adding a
  second diff stack or hand-rolling hunks. Actively pull dependencies that raise
  UX quality. Layout mirrors GitHub/GitLab commit inspect:
  - **left** = changed-file list;
  - **right** = a **single scroll** listing each file's diff in order;
  - non-diff (unchanged) regions are collapsed behind a **"show more"** expander.
- **Modes:** single-commit diff AND **commit-range** diff. Range is driven from
  the log graph via a git-native "diff from here" affordance (mark a base commit,
  diff to HEAD or to another selected commit — `git diff <A>..<B>`). Working-tree
  diff (from the unstaged-status label) is the third entry point.
- **Design guide proving slice.** This interaction-heavy view is the primary
  vehicle for `260725-feat-ws-dashboard-design-guide`; its components define the
  first cut of the guide's primitives.

## Constraints

- Single-scroll, many-file diffs must stay performant (virtualize / lazy-render
  hunks as needed); "show more" keeps unchanged context collapsed by default.
- Must leave a clean seam for the comment gutter
  (`260725-feat-ws-dashboard-git-review-comments`) to attach per-line/per-range.
- Daemon diff/show endpoints extend the `git_toolbar.rs` shell-out pattern
  (`git_text`/`run_git`) and register alongside `/git/*`; no git library.

## Prior Art

- `readOnlyWorkbenchPane.tsx` / `readOnlyFilePlacement.ts` / `openReadOnlyFile`
  (`App.tsx:1101`) — the right-pane host to reuse; `surfaceRegistry.ts` reserved
  `diff` kind to wire.
- `@codemirror/merge` (CodeMirror 6, already in `package.json`) — diff renderer
  substrate.
- `git_toolbar.rs` `git_text`/`run_git` + `/git/*` routes — daemon plumbing to
  extend for `git show`/`git diff <range>`.

## Phases

### Phase 1: Diff view + daemon diff/show endpoints

Add daemon endpoints for single-commit (`git show`) and range (`git diff A..B`)
and working-tree diffs. Build the diff view on `@codemirror/merge`: left changed-
file list + right single-scroll diffs with "show more" collapse, hosted in the
right pane via the reserved `diff` SurfaceKind. Wire single-commit, range
("diff from here"), and working-tree entry points. Build to the design guide.

Verification boundary: selecting a commit shows its diff GitHub-style (file list +
single scroll, unchanged regions collapsed); "diff from here" produces a correct
range diff; the working-tree label shows the unstaged diff; the view opens in the
right pane like a document; large diffs stay responsive.

## Spec Impact

Target spec area: none in the workflow spec set — downstream ws-dashboard UI + new
local daemon git diff/show endpoints, no workflow-system contract.

Contract-first spec: no.
