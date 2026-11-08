# Plan: tracker hierarchy display (context trees) + masonry card grid

Branch `delegate/tracker-hierarchy` (worktree
`/Users/kang-sw/.ws-worktrees/devenv/decree-thinness-strum`), based on
develop (the drawer UI is merged: `a6f0b0ab1`). Commit here; do not merge
or push. Do not run git stash. Read
`ai-docs/manuals/shipped-surface-boundary.md` before editing under
`agents-plugin-tool/`.

The renderer is `agents-plugin-tool/internal/wsdashboard/assets/tracker.js`
(+ `tracker.css`), shared by the dashboard drawer and the mockup
(`ai-docs/ref/tracker-mockup/index.html` loads it through
`mock-adapter.js`). Change the renderer once; both surfaces follow. The
view-state shape (`wstracker` Go / mockup `engine.js`) should not need to
change - parent/children links are already in it; if a derived field is
genuinely missing, add it in BOTH engine.js and Go `wstracker` with golden
parity (`internal/wstracker/testdata/gen-golden.js`) and say so.

## Settled (user, board 2026-11-08)

- **(467) a - context tree in every scope list.** Every scope's list (mine /
  received requests / assigned by me / team / all) renders as a context
  tree: for each matching item, its ancestor chain is shown above it as
  greyed context rows (not counted in the scope's count; click opens that
  item), unrelated siblings collapse into one `… N more` row (click
  expands), the matching item's own subtree is shown folded (`▸ N
  subtasks`). Each node folds/unfolds with ▸/▾. Fold state per node lives
  in browser storage per device (epic "Local UI state": display state is
  browser storage), keyed by store + item id. Received-request
  sub-sections (answered / sent, etc.) each render their own tree. Closed
  children keep the existing "closed N" fold.
- **(468) a - gantt tree rows + parent summary bar.** The timeline uses the
  same tree rows. A parent draws its own due/duration bar and a thin summary
  bar spanning its children's range; folded, only the summary bar shows. A
  parent without its own due is placed by its children's range.
- **(469) - kanban/calendar link highlight.** Cards stay independent tasks.
  Hover or keyboard focus on a card highlights its direct parent and direct
  children cards; when the parent is not on screen (other column/month),
  a tooltip shows the parent's title.
- **(470) - item window full subtree.** The item window's subtasks section
  shows the whole subtree with ▸/▾ folding, first level open, deeper levels
  folded (supersedes direct-children-only `subtasksHtml`).
- **Masonry card grid** (user 2026-11-07): the drawer's card grid should pack
  cards Pinterest-style with no awkward vertical gaps (a short card beside a
  tall one leaves empty space today). Pick the simplest robust technique
  (CSS columns if card order allows it, else a small JS row-span layout;
  `grid-template-rows: masonry` is not widely supported) and keep keyboard
  order sensible.

## Record

Add Settled rows to `ai-docs/ref/tracker-mockup/README.md` `## Settled`
table for 467, 468, 469, 470 and the masonry note (same style as existing
rows: board ids then one-sentence decision), and update any README text
that says the lists are flat.

## Out of scope

Context menus, inline create, status-radio, modal removal (471-474: the
next delegate, which builds on these tree rows - keep row markup
extensible: one row component with a stable item-id data attribute).
Assignment model (490-492).

## Verification

- `node --check` on every changed JS.
- `cd agents-plugin-tool && go test -count=1 ./internal/wsdashboard/ ./internal/wstracker/ ./internal/wswhiteboard/` (tracker strings are `js.tk.*` in `wsi18n/tables/en.json`; new strings go there - other languages fall back as the drawer did; keep them out of the whiteboard embed, already handled by the `js.tk.` prefix).
- Headless browser check (Playwright cached under `~/.npm/_npx`) on the
  mockup `index.html` (file://) with its fixtures: screenshots of a scope
  list context tree, the gantt with a folded and unfolded parent, kanban
  hover highlight, item window subtree, and the masonry grid, light theme
  at least, saved to `/tmp/tracker-hierarchy/`. Check there are no console
  errors.

## Return

Commit hashes, changed files, test output, screenshot paths, the masonry
technique chosen, anything left open.
