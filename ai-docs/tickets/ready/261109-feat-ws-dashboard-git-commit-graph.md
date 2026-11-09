---
title: "ws dashboard git drawer: commit graph and commit diff viewer"
sage-review-design: completed
sage-review-completeness: completed
sage-review-completeness-reviewed: a0ec016492d9efd7
sage-review-design-reviewed: a0ec016492d9efd7
---

# ws dashboard git drawer: commit graph and commit diff viewer

## Background

The session page's git drawer landed in 134c15655 (plan
`ai-docs/.plans/2026-11/08-2149-git-drawer.md`, board items (508)-(512) of
the 2026-11-08 session). Its right panel lists changed files (staged /
unstaged / untracked with numstat, no diff, (512) a) and carries fetch,
pull (fast-forward only), and push. Its wide left area is a placeholder
(`.gleft` in the `gitbody` define of
`agents-plugin-tool/internal/wsdashboard/assets/dashboard.tmpl`, i18n key
`git.graph.later`, hidden under 760px). The plan reserved it for a commit
graph and said "No commit action anywhere."

The user asked to fill the placeholder (board 2026-11-09, topic
"대시보드: git drawer 커밋 그래프", items (97)-(110), all settled). For the
click behaviour the user described the target in their own words: "마우스를
올리면 호버툴팁, 클릭하면 플로팅 패널 ... 우측은 파일 diff 리스트, 좌측에는
gitlab/github 스타일의 단일 패널 멀티 diff 뷰어 ... on-demand loading".

## Decisions

- **Graph scope: every local branch and its upstream as lanes** (board (97)
  b), drawn like `git log --graph`. Branch labels reuse the drawer handle's
  impl (hammer) / goal (trophy) icons and stem abbreviation, and a branch
  checked out in a worktree carries a worktree marker. The main use is
  seeing how far each parallel impl branch is ahead of develop.
  Rejected: only the current branch and its upstream (hides the parallel
  impl branches); also remote-only branches (lanes explode on team
  repositories).
- **Read-only** (board (99)): the graph and the commit panel hold no write
  action (no commit, checkout, reset, merge, cherry-pick). Writes in the
  drawer stay fetch / pull / push, extending the git drawer plan's "No
  commit action anywhere".
- **Lane layout is computed by the daemon; the browser draws it as inline
  SVG, with no external library** (board (100)). The dashboard ships to
  downstream projects offline and loads no external script; a server-side
  layout is pinned by Go tests while the dashboard JS has no test harness.
- **Graph data: a new read-only `GET /s/<rid>/git/log`** (board (101)): no
  fetch, `--no-optional-locks`, the same access rules as `GET
  /s/<rid>/git/status` (a GET with no side effect, so it needs no Origin
  check bypass). First 200 commits, a "more" control at the bottom loads the
  next page.
- **Hover a commit: tooltip; click: floating commit panel** (board (98),
  user's variant of option b). Tooltip (board (102)): subject line, author,
  relative time, short hash, ref names on that commit. Panel header: full
  message, author / committer and times, a copy-hash button (same shape as
  the tracker's ID copy button), parent hashes.
  Rejected: commit details inside the drawer's right panel without diff
  (the (98) recommendation the user replaced); hover only.
- **Floating panel layout: right side the commit's changed-file list, left
  side one scrolling multi-file diff viewer** (GitHub / GitLab "Files
  changed" style, board (98)). The panel is a centred window over the page
  like the tracker's item window: close button, Esc, click outside (board
  (105)). Under 760px the graph is hidden, so the panel has no entry point
  there.
- **On-demand loading** (board (103)): `GET /s/<rid>/git/commit/<oid>`
  returns the commit metadata and the file list (status, rename source,
  numstat) without patches; `GET /s/<rid>/git/commit/<oid>/diff?path=<p>`
  returns one file's diff. The viewer reserves a section per file and
  fetches its diff when the section nears the viewport
  (IntersectionObserver). A file with more than 400 changed lines (one
  named constant), a binary file, or a deleted file shows a "load diff"
  button instead of loading by itself. Clicking a file in the right list
  scrolls to its section and loads it.
  Rejected: one request returning the whole commit patch (large commits,
  e.g. regenerated manifests or all 11 i18n tables, would stall the first
  paint).
- **Merge commits diff against their first parent; a root commit diffs
  against the empty tree** (board (104)). Rejected: git's combined diff
  (does not fit a GitHub-style viewer; first-parent is "what this merge
  brought into develop").
- **Route guards** (board (106)): every new route is a read-only GET under
  the same Origin / LAN rules as `git/status` and is registered in
  `TestLANModeGuardsEveryRoute`. `path` must be one of the paths in that
  commit's file list (any other path is refused). One file's diff is cut at
  1 MiB with a visible "truncated" notice.
- **Unified diff only** (board (107) a). Rejected for now: a split
  (side-by-side) toggle; the left area of the panel is narrower than the
  window because the file list sits beside it, and split view needs its own
  line-pairing logic that can be added later.
- **The working-tree file list opens the same viewer** (board (108) a,
  replacing (512) a for this list): clicking a file in the drawer's right
  panel (staged / unstaged / untracked) opens the floating panel for the
  working tree, with the same file list / multi-diff layout. Staged shows
  index vs HEAD, unstaged shows working tree vs index, untracked shows the
  whole file as added. The file list comes from the existing `git/status`
  read; per-file diffs come from a read-only GET of the same shape and
  guards as the commit diff route, carrying the side, with `path` limited
  to the paths `git/status` reports.
- **Syntax highlighting with `github.com/alecthomas/chroma/v2`, run in the
  daemon** (board (109) b, (110)). The user asked for a package covering
  wide language sets (C++, Rust, ...); chroma is MIT and covers 200+
  languages. The daemon highlights the whole old and new file contents and
  maps the highlighted lines onto the hunk lines, so multi-line comments
  and strings that cross a hunk boundary stay correct. A file above a size
  cap (one named constant) is not highlighted, and highlighted results are
  cached by (blob oid, path). Rejected: highlight.js or Prism bundled into
  the assets (they colour hunk fragments in isolation and add 100 KB+ to
  the offline asset set).
- **Design-review refinements** (2026-11-09, accepted by the lead from the
  design and completeness reviews):
  - Working-tree content is read under the existing file route's rule: no
    symlink is followed (Lstat, then confirm the opened file is the one
    Lstat saw, as in `file_route.go`), and the path stays inside the session
    root. A symlink entry (untracked, or a tracked file whose type changed)
    shows its link target as text, the way git diffs a symlink. An untracked
    directory entry (`dir/` from `--untracked-files=normal`) and a gitlink
    (mode 160000) show a notice instead of a diff. Phase 2's path-guard tests
    include a symlink pointing outside the root.
  - The working-tree diff route is `GET /s/<rid>/git/worktree/diff?side=<staged|unstaged|untracked>&path=<p>`,
    registered in `TestLANModeGuardsEveryRoute`.
  - `<oid>` is accepted only as a 4-64 character hex object id; anything
    else is refused before git runs, so no ref expression or leading `-`
    reaches git argv.
  - Log order is `--topo-order`. Lanes stay continuous across "more" pages:
    the daemon recomputes the layout from the top for the requested range
    and returns the slice (or an equivalent cursor carrying open-lane
    state); the paging test asserts lanes continue across a page boundary.
    The JSON shape (per-commit column, edges, refs, worktree markers, the
    "more" cursor) is settled in Phase 1 before the SVG drawing is written.
  - chroma emits class-based tokens (no inline styles), styled through
    `dashboard.css` theme tokens for both light and dark themes.
  - Intra-line change marks are computed by the daemon, word-level, for a
    removed run and the added run that directly follows it, paired line by
    line only when the two runs have the same length; otherwise no
    intra-line marks. A Go test covers one paired and one unpaired case.
  - The highlight cache is an LRU with a named entry bound. Working-tree
    content (no blob oid) is not cached.
  - chroma is pinned to a version that builds under go.mod's current `go`
    directive; if the directive must rise, the commit says so next to the
    binary-size report.

## Constraints

- The dashboard and its strings ship downstream:
  `ai-docs/manuals/shipped-surface-boundary.md` applies to every file under
  `agents-plugin-tool/`.
- i18n: `TestOtherTablesHoldEveryEnglishKey` requires every en key in all
  11 tables under `agents-plugin-tool/internal/wsi18n/tables/`; every key
  the code no longer uses is removed (the keys test fails on unused keys).
  `git.graph.later` goes away once the graph replaces the placeholder.
- Git commands run with `--no-optional-locks` and never fetch; the graph
  reads local refs only.
- The dashboard JS has no executing test harness: pin server-side behaviour
  (lane layout, routes, guards, diff mapping, highlighting) with Go tests
  and check the browser side with a headless browser against a dev daemon
  (isolated `WS_CACHE_HOME` / `WS_CONFIG_HOME`, spare port; never the
  user's daemon on port 29417).
- Report the ws-mcp binary size before and after adding chroma.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)

## Prior Decisions

- 134c15655 (2026-11-08, commit): "No commit action anywhere (plan). A non-repository root renders no handle rather than a dimmed one: there is nothing to open." — bearing: supports
- 134c15655 (2026-11-08, commit): "Data: GET /s/<rid>/git/status (new, read-only, no optional locks, no fetch) answers the file list ... Making the GET side-effecting was rejected: a read route that refreshes the shared watcher would bypass its Origin check." — bearing: constrains
- 134c15655 (2026-11-08, commit): "The git drawer reuses the panels drawer's frame with a mode switch (drMode \"panels\" | \"git\") instead of a second drawer: one height, drag, clip, and outside-click machinery" — bearing: constrains
- 134c15655 (2026-11-08, commit): "i18n added (en, ko only): git.busy, git.detached, ... git.graph.later ..." (since present in all 11 tables) — bearing: supports
- 284b6e50 (2026-11-08, commit): "The drawer-body scroll comment in dashboard.js now notes drbody is hidden while the frame shows the git sheet (drift from the git drawer commit)." — bearing: constrains
- 261109-feat-ws-dashboard-composer-file-mention (2026-11-09, Decisions): "Absolute browsing is loopback only ... enforced on the server, not just hidden in the page; a LAN viewer gets root search only." — bearing: constrains
- 261109-feat-ws-dashboard-open-out-of-root-images (2026-11-09, commit 1b06ffd4): "Go tests and headless Chromium loopback/LAN checks passed; review Critical (\??\UNC namespace bypass) fixed in 8295d5c83." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsdashboard/assets/dashboard.tmpl, dashboard.js, dashboard.css, internal/wsdashboard/gitdrawer.go and pages.go route dispatch, internal/wsi18n/tables/*.json, go.mod for chroma |
| scope.surface | public-interface | new daemon HTTP routes GET /s/<rid>/git/log, /git/commit/<oid>, /git/commit/<oid>/diff, and a working-tree diff route |
| scope.new_public_symbol | yes | the four new GET routes and their handlers |
| scope.new_type_contract | yes | JSON shapes for log page with lane layout, commit metadata plus file list, per-file highlighted diff |
| scope.test_surface | existing | internal/wsdashboard/gitdrawer_test.go, internal/wsdashboard/access_test.go TestLANModeGuardsEveryRoute, internal/wsi18n/keys_test.go |
| complexity.reuse_points | confirmed | handleGitStatus and run.RunGit with --no-optional-locks in internal/wsdashboard/gitdrawer.go#L80-L88; idcopy button and copyText in assets/tracker.js#L1718-L1724; pages.go#L764 git/status dispatch |
| complexity.side_effect_risk | moderate | git reads are read-only but chroma adds a new module dependency and binary growth; working-tree diff reads files under the session root |
| risk.correctness | high | daemon lane layout across many branches and merges, highlight-to-hunk line mapping, first-parent and empty-tree bases, rename and binary handling |
| risk.fit | moderate | new third-party dependency in a shipped offline binary and reuse of the shared drawer frame and tracker window patterns |
| risk.test | moderate | dashboard JS has no executing harness; browser side relies on headless screenshots against a dev daemon |
| risk.security_or_contract | moderate | new routes expose repository contents over LAN mode; path parameter must be confined to the commit or status file list |

## Phases

### Phase 1: commit graph, tooltip, and commit panel without diffs

Replace the `.gleft` placeholder with the graph:

- `GET /s/<rid>/git/log` with paging (200 per page, "more"), returning
  commits with parents, refs, and the daemon-computed lane layout for local
  branches and their upstreams; worktree checkout markers from `git
  worktree list --porcelain`.
- Inline SVG graph in `dashboard.js` / `dashboard.css`, branch labels with
  the handle's icons and abbreviation, the hover tooltip, and the floating
  commit panel: header (full message, author / committer, times, copy-hash
  button, parents) and the right-side file list from `GET
  /s/<rid>/git/commit/<oid>` (status, rename source, numstat). The left
  diff area shows a placeholder until Phase 2.
- Route guards and tests as in Decisions.

#### Verification

From `agents-plugin-tool/`: `gofmt -l ./internal ./cmd`, `go vet
./internal/...`, `go test ./internal/wsdashboard/ ./internal/wsi18n/
./cmd/ws-mcp/ -count=1`, `node --check` on changed JS. Go tests cover:
lane layout on a temp repo with several branches, a merge, an upstream
ahead and behind, and a detached worktree; paging; the commit metadata and
file-list route (rename, binary, merge first-parent, root commit); the
route guards (`TestLANModeGuardsEveryRoute`, GET only, unknown oid
refused). A headless browser screenshot of the drawer with the graph, a
tooltip, and the open commit panel.

### Phase 2: multi-file diff viewer, highlighting, and working-tree diffs

Fill the panel's left side:

- `GET /s/<rid>/git/commit/<oid>/diff?path=<p>` per file (first-parent /
  empty-tree base, path limited to the commit's file list, 1 MiB cut with a
  notice), unified rendering with added / removed line colours and
  intra-line change marks.
- On-demand loading as in Decisions (IntersectionObserver, 400-line /
  binary / deleted manual load, list click scrolls and loads).
- chroma highlighting in the daemon (whole old / new contents, mapped onto
  hunk lines, size cap, (oid, path) cache).
- The working-tree file list opens the same panel for staged / unstaged /
  untracked diffs.

#### Verification

Same commands as Phase 1. Go tests cover: per-file diff for modify, add,
delete, rename, binary, the 1 MiB cut, a path outside the file list
refused; highlight mapping where a multi-line comment starts before a hunk;
the size cap skipping highlight; working-tree staged / unstaged / untracked
diffs and their path guard. A headless browser screenshot of the viewer
with several files, one waiting behind "load diff", and a highlighted
hunk. Report the binary size delta.
