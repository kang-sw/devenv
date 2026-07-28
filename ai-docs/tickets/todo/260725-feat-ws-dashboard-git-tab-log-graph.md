---
title: Files | Git tabbar + git log graph panel (UI 1)
sage-review-design: required
parent: 260725-epic-ws-dashboard-git-panel
related:
  260725-feat-ws-dashboard-git-diff-view: the log graph's labels/commits open this diff view; shares the daemon git endpoints defined there
  260725-feat-ws-dashboard-design-guide: new components built to this guide
related-mental-model:
  - ws-web-dashboard
---

# Files | Git tabbar + git log graph panel (UI 1)

## Background

The bottom-left file browser (`WorkRootFileExplorer`, `App.tsx:3126`, mounted in
the nav-stack at `App.tsx:2886`) is a plain vertical component with no tab
container. This ticket reframes it under a `Files | Git` tabbar and adds the Git
tab's content: a git log graph panel (UI 1). The narrow column drives compact
per-row rendering choices.

## Decisions

- **Dual tabbar over the file browser.** Introduce a `Files | Git` tabbar in the
  left nav-stack; Files = the existing explorer (extended, not rewritten), Git =
  the new log graph panel. No tabbar exists in the left nav today (greenfield);
  built to the design guide.
- **Log graph (UI 1):** render top-down from HEAD as a commit graph.
  - Each commit's **first line** carries a **timestamp**: **relative time within
    the last ~1 month, absolute date beyond that**. The subject line may be
    truncated in the narrow column.
  - At the **right end of each row**, a compact **change-file count** using
    `+ / - / *` = number of files **added / deleted / modified** in that commit
    (file counts, not line counts). Source: `git show --name-status` aggregation.
  - **`[ ] ALL` checkbox** at the top: unchecked = default (HEAD ancestry);
    checked = `git log --all` mode.
  - **Topmost row is always the current unstaged git status label** (working-tree
    status), distinct from commit rows.
- **Clicking the unstaged-status label opens the diff view (UI 2) in the right
  pane** (working-tree diff). Clicking a commit / selecting a range likewise
  drives UI 2. UI 2 itself is `260725-feat-ws-dashboard-git-diff-view`; this
  ticket wires the triggers.

## Constraints

- Compact rendering: narrow column, so time format and change counts must stay
  terse (relative/absolute switch, `+/-/*` counts right-aligned).
- The Files tab must preserve existing file-explorer behavior/commands.
- Depends on the daemon git-log endpoint (defined with the diff-view ticket's
  daemon plumbing) and opens panes via the diff-view ticket's right-pane host.

## Prior Art

- `WorkRootFileExplorer` (`App.tsx:3126`) / nav-stack mount (`App.tsx:2886`) — the
  container to extend.
- `git_toolbar.rs` `git_text`/`run_git` + `/git/*` routes — shell-out pattern to
  extend for `git log --graph`/`--all` and `--name-status` counts.
- Right-pane tab UI already exists in the dockview panes (not the left nav) —
  reference for tab chrome, but the left-nav tabbar is new.

## Phases

### Phase 1: Files | Git tabbar + log graph panel

Add the left-nav `Files | Git` tabbar (Files = existing explorer). Build the Git
tab: daemon endpoint for the log graph (top-down from HEAD, `--all` toggle) with
per-commit `+/-/*` file counts; render the graph with relative/absolute time,
truncated subject, right-aligned counts, the `[ ] ALL` toggle, and the topmost
unstaged-status label. Wire label/commit/range clicks to open the diff view.

Verification boundary: the Git tab shows a HEAD-down graph with correct time
formatting (relative <1mo, absolute beyond) and `+/-/*` file counts; ALL toggle
switches to `--all`; the unstaged-status label reflects working-tree status;
clicking it and clicking commits opens the diff view; the Files tab still works.

## Spec Impact

Target spec area: none in the workflow spec set — downstream ws-dashboard UI +
a new local daemon git-log endpoint, no workflow-system contract.

Contract-first spec: no.
