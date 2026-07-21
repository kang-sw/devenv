---
title: "Work-root nav icon shape/color is inconsistent between compact and multi-root presentations"
---

# Work-root nav icon shape/color is inconsistent

## Background

Owner report (dogfooding, 2026-07-20, translated from Korean): "Work-roots
that have associated worktrees are shown with a gray icon, and work-roots
without worktrees are shown with a blue folder-shaped icon. The icon
rendering isn't consistent in color and shape between these — it's
confusing, needs cleanup ('traffic control')."

Investigation confirms this is real and traces it to a structural split in
how the left-nav renders a workspace depending on its work-root count, not a
one-off styling bug in a single branch.

## Current rendering — exact mapping (frontend, `ws-dashboard/frontend/src`)

A workspace with exactly one work root is "compact"
(`compactWorkspaceWorkRoot`, `resourceModel.ts:396-408`: returns the sole
root only when `workspace.workRoots.length === 1`). A workspace with more
than one work root (i.e. a git primary root plus one or more linked
worktrees living in the same workspace) renders as a parent "workspace" row
plus one child "workRoot" row per root. This root-count split is exactly
the "has associated worktrees" vs. "no worktrees" distinction the owner is
reacting to.

Three call sites build the row list, `App.tsx:9418-9497`:
- Compact case → `presentation="compactWorkRoot"` (`App.tsx:9421-9448`),
  `kind` passed at `App.tsx:9431`.
- Multi-root case → one `presentation="workspace"` parent row
  (`App.tsx:9455-9471`, no `kind` prop passed at all) followed by
  `presentation="workRoot"` child rows, one per root (`App.tsx:9474-9497`,
  `kind` passed at `App.tsx:9484`).

Each row can render up to two icons: a main glyph (`ResourceGlyph`,
`App.tsx:1542-1564`) and, only when a `kind` prop is supplied, a small
adjacent "kind badge" (`WorkRootKindIcon`, `App.tsx:1566-1574`, mounted at
`App.tsx:9587-9590`).

`ResourceGlyph` branches only on `presentation`, never on `kind`:
- `presentation === "compactWorkRoot"` → `FolderOpen`
  (`App.tsx:1553`), class `resource-row-icon resource-row-icon-compact`.
- `presentation === "workspace"` → `BriefcaseBusiness` (`App.tsx:1558`),
  class `resource-row-icon` only.
- `presentation === "workRoot"` (the `else` branch, same line) →
  `FolderGit2` **unconditionally**, class `resource-row-icon` only — this
  is used for every child row regardless of whether that root's `kind` is
  `plainDirectory`, `gitPrimaryRoot`, or `gitLinkedWorktree`.

`WorkRootKindIcon` (badge) branches on the actual `WorkRootView.kind`
(`resourceModel.ts:152`: `"plainDirectory" | "gitPrimaryRoot" |
"gitLinkedWorktree"`):
- `plainDirectory` → `Folder`
- `gitLinkedWorktree` → `GitBranch`
- else (`gitPrimaryRoot`) → `FolderGit2`
(`App.tsx:1566-1574`)

Colors (`styles.css`):
- `.resource-row-icon-compact` → `color: var(--ws-color-action)` = `#78a9ff`
  (blue) — `styles.css:2632-2634`, token at `styles.css:37`.
- `.resource-row-icon` (non-compact, i.e. the `workspace` and `workRoot`
  main glyphs) → `color: var(--ws-color-text-tertiary)` = `#a8a8a8` (light
  gray) — `styles.css:2623-2630`, token at `styles.css:35`.
- `.resource-kind-glyph` (the badge, both compact and non-compact) →
  `color: var(--ws-color-text-disabled)` = `#6f6f6f` (darker gray) —
  `styles.css:2636-2638`, token at `styles.css:36`.

Net effect per presentation:

| Presentation | When shown | Main glyph shape | Main glyph color | Kind badge shown | Kind badge shape (by `kind`) | Kind badge color |
|---|---|---|---|---|---|---|
| `compactWorkRoot` | workspace has exactly 1 root | `FolderOpen` | blue `#78a9ff` | yes | Folder / GitBranch / FolderGit2 | dark gray `#6f6f6f` |
| `workspace` (parent) | workspace has >1 root | `BriefcaseBusiness` | light gray `#a8a8a8` | no (`kind` never passed) | — | — |
| `workRoot` (child) | one per root, workspace has >1 root | `FolderGit2` (always, ignores `kind`) | light gray `#a8a8a8` | yes | Folder / GitBranch / FolderGit2 | dark gray `#6f6f6f` |

## What this means for the owner's report

- "work-roots with associated worktrees" = roots living in a multi-root
  workspace → rendered as `workRoot` child rows → light-gray `FolderGit2`
  main glyph.
- "work-roots without worktrees" = the sole root of a single-root workspace
  → rendered `compactWorkRoot` → blue `FolderOpen` main glyph.

This confirms the owner's read: the same conceptual entity (a work root)
gets a different shape AND a different color for its primary glyph purely
based on how many roots its *workspace* happens to have — not based on the
root's own `kind`. A `plainDirectory` root and a `gitPrimaryRoot` root look
identical in the compact case (both blue `FolderOpen`) and near-identical in
the child case (both light-gray `FolderGit2` main glyph; they only differ in
the small, low-contrast badge next to the title).

Additional inconsistencies found during investigation, beyond the
color/shape swap the owner already named:

1. **Main glyph ignores `kind` entirely in the child-row case.** A plain,
   non-git directory nested under a multi-root workspace shows `FolderGit2`
   (a "git" glyph) as its primary icon, while its own kind-badge two icons
   over correctly shows a plain `Folder` — two icons on the same row
   visually disagree about whether this is a git root.
2. **The badge only exists for `compactWorkRoot`/`workRoot`, never for the
   `workspace` parent row.** So the same nav list mixes rows with one icon
   (parent `workspace`) and rows with two icons (compact/child roots), with
   no visual grouping cue tying badge to main glyph.
3. **Three independent color channels compete on the same row.** Separately
   from icon color, `resourceRowTone` (`App.tsx:9907-9923`) drives a
   left-border color for ready/muted/error row state
   (`styles.css:2598-2610`). A single row can therefore show a border tone
   color, a main-glyph color (blue-vs-gray by presentation), and a
   fixed-gray badge color simultaneously — three overlapping but
   independently-driven color signals with no shared legend.

## Severity

UX polish / visual-consistency issue, non-blocking. No functional behavior
is affected; work-root selection, opening, and worktree operations all work
correctly regardless of icon rendering.

## Non-Goals

This ticket originally captured and precisely documented the current
inconsistent state for a future design decision without prescribing a fix.
See `## Decided Direction (2026-07-21)` below: the user has since prescribed
a narrow fix for this round. The broader open design question (which of the
two icons should be dropped/merged/become the single source of truth for
git/plain/worktree status, shape unification, badge-on-`workspace`-row, the
three-competing-color-channels issue) remains out of scope here and still
likely wants input from whoever owns
`260524-research-ws-dashboard-visual-design-system-refresh` or general
dashboard visual-design direction.

## Decided Direction (2026-07-21)

Keep the current icon **set** unchanged - do not change shapes/glyphs. Only
change the icon **color** of a root work-root that has child worktrees
(the `presentation === "workspace"`/`workRoot` light-gray case,
`.resource-row-icon`, `color: var(--ws-color-text-tertiary)`,
`styles.css:2623-2630`) so it matches the color of a single work-root with no
worktrees (the `compactWorkRoot` case, `.resource-row-icon-compact`,
`color: var(--ws-color-action)` = `#78a9ff`, `styles.css:2632-2634`, token at
`styles.css:37`). Reuse the existing `--ws-color-action` token as-is - do not
invent a new blue. This is a narrow color-unification only; shape, badge
presence, and the border-tone channel are unchanged.

## Phases

### Phase 1: Unify with-child-worktrees root icon color to the single-root icon color

Change the main-glyph color for the `workspace` parent row and its `workRoot`
child rows (currently `.resource-row-icon` / `var(--ws-color-text-tertiary)`,
`styles.css:2623-2630`) to the same `var(--ws-color-action)` blue already used
by `.resource-row-icon-compact` (`styles.css:2632-2634`) for the single-root,
no-worktrees case. Do not change icon shape/glyph selection
(`ResourceGlyph`, `App.tsx:1542-1564`), the kind badge
(`WorkRootKindIcon`), or `resourceRowTone`'s border-color behavior.

Success: a root-with-worktrees (`workspace`/`workRoot` presentation) and a
single no-worktree root (`compactWorkRoot` presentation) render their main
glyph in the same color; no other icon/color behavior changed.

## Result (2026-07-21)

Implemented as a single CSS color-token edit: `.resource-row-icon,
.resource-kind-glyph` `color` changed `var(--ws-color-text-tertiary)` →
`var(--ws-color-action)` in `ws-dashboard/frontend/src/styles.css` (line
~2629), unifying the base-root-with-worktrees glyph color with the
single-root glyph.

No shape/glyph/badge/border logic changed; `App.tsx` untouched; the
now-redundant `.resource-row-icon-compact` rule left in place per the narrow
scope.

Verification: `npm run build` passed (tsc + vite); no color-assertion tests
exist. Single-scope review returned clean.

Implementation commit: 8ffb60b7 (plan 9181b7b1).
