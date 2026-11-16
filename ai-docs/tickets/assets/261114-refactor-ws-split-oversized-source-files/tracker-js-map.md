# tracker.js mechanical split map

Status: **static preparation, not a Phase Result.** Source pin
`616271b51` (develop head `c36321736` differs only in a ticket doc; `tracker.js`
and `pages.go` are unchanged between them). No source file was edited. The
byte-identity, `node --check`, Go build/vet/compile and narrow-test commands
below were run once as a **baseline** against the unmodified tree and a
temp-dir split simulation; they are not results of an executed split.

Target: `agents-plugin-tool/internal/wsdashboard/assets/tracker.js`, 7217 lines,
521,616 bytes, LF only, final newline, SHA-256
`0bba48f6ac4d5324792cdaa05085aab22b93e3a86aab20115319077267f2aecf`.
Compatibility **T0**: the served bytes, `TrackerScript`, and asset identity stay
identical; no route, template, protocol, or on-disk shape changes.

## Assembly mechanism (reuse, do not invent)

Paths relative to `agents-plugin-tool/internal/wsdashboard/`.

- `pages.go:34` `//go:embed` lists `assets/tracker.js` explicitly (and globs
  `assets/dashboard-*.js`, `assets/dashboard-*.css`).
- `pages.go:37-58` `dashboardScriptFiles` / `dashboardStyleFiles`: ordered
  fragment lists; comment at `pages.go:37-39` states the no-separator, single-IIFE
  contract.
- `pages.go:71-90` `readDashboardSource(read, name)`: a `switch name` maps the
  logical names `dashboard.js`/`dashboard.css` to their lists and concatenates
  with no separator; every other name falls through to `read(name)`.
- `pages.go:256-265` `buildDashboardAssets.get` calls `readDashboardSource` for
  every asset, so the embedded path (`embeddedDashboardAsset`, `pages.go:230-232`)
  and dev-assets path (`loadDashboardAssets`, `pages.go:221-228`, reading
  `internal/wsdashboard/assets/<name>` from the dev root) both assemble.
- `pages.go:281` `TrackerScript: template.JS(get("tracker.js"))`. tracker.js is
  **only inlined**: `assets/dashboard.tmpl:37` (session page) and
  `assets/dashboard.tmpl:371` (second page kind), with `TrackerCSS` at `:8,:69,:363`.
  It is not served standalone by URL. Only the main dashboard script has a hashed
  URL (`ScriptFile`, `pages.go:315-317`; route check `pages.go:1299`).
- Asset identity: `assetID` (`pages.go:341-351`) hashes `json.Marshal(pageAssetsData)`
  (which contains the assembled `TrackerScript`), the template, guide id and i18n
  tables. Identical assembled bytes give an identical ID, so open pages do not
  see an asset change after deploy.
- No CSP hash covers inline page scripts (CSP headers exist only on
  file/image/fs routes: `file_route.go:321`, `fs_outside.go:535`,
  `fs_capability.go:428`, `attachment_route.go:61`). `assets/pwa/` holds only a
  manifest and icons; no precache list.

### Registration change (exact)

1. `pages.go`: add after `dashboardStyleFiles`:
   `var trackerScriptFiles = []string{"tracker.js", "tracker-panel.js", "tracker-wiki.js", "tracker-window.js", "tracker-forms.js", "tracker-menu.js", "tracker-writes.js", "tracker-render.js", "tracker-mount.js",}`
   (gofmt multi-line form, one entry per line, trailing commas, same as the
   existing lists; `testdata/asset-source.cjs` parses exactly that shape).
2. `pages.go` `readDashboardSource`: add `case "tracker.js": files = trackerScriptFiles`.
3. `pages.go:34`: append `assets/tracker-*.js` to the `//go:embed` line. The
   pattern must not be added before the first `tracker-*.js` file exists: an
   embed glob with no match is a compile error.
4. `testdata/asset-source.cjs` `declarations`: add
   `'wsdashboard/tracker.js': ['pages.go', 'trackerScriptFiles'],`.
5. `asset_source_test.go:20-22` `TestDashboardSourceAssembly`: add a
   `{"tracker.js", [...the list...]}` row.

`pages.go:281`, both template sites and `get("tracker.js")` stay unchanged. The
name `tracker.js` stays both the logical name and the opening fragment, as
`dashboard.js` does. `tracker-*.js` does not collide with `dashboard-tracker.js`
(that file is a `dashboard.js` fragment, the tracker adapter) or with the
`dashboard-*.js` glob.

## Destinations (contiguous, inclusive lines at 616271b51)

New line = original line - range start + 1. No bytes are inserted between
fragments. Every cut sits on a blank line followed by an existing
`  // ------ <section>` heading at IIFE top level, so each heading and its
section comment move with the code they introduce.

| # | Destination | Lines | Count | Bytes | Top-level IIFE contents |
|---|---|---|---:|---:|---|
| 1 | `tracker.js` (retained, opening) | 1-1150 | 1150 | 89,555 | Adapter-contract header comment (1-221); IIFE open `(function () {` 222 and `'use strict'` 223; shared helpers/state `alias`..`vals` incl. `AD`, `order`, `ui`, `SCOPES`, `SCOPE_MODES`, `esc`, `$` (228-370); small views `VS`..`evLine` (372-636); viewer/badge state `viewerOf`..`isNew` (637-658); item rows `stKey`..`sub` incl. row edit, permission predicates, move/multi-select (659-1150) |
| 2 | `tracker-panel.js` | 1151-1777 | 627 | 51,800 | Cards: notification feed `NK`..`attChips` (1151-1281); Items panel `panelState`..`scopeHasToday` incl. `SQ`/`searchFor`, closed list, tree, `TEAM_LOAD`/`teamDrillRecs`, `listAll`, `cardPanel` (1282-1777) |
| 3 | `tracker-wiki.js` | 1778-2444 | 667 | 52,215 | Wiki `WK_NAME`..`wkAct` (1778-2339); store cards `settingsHtml`, `cardTeam`, `kindFilter`..`cardActivity`, `lastWrite` (2340-2444) |
| 4 | `tracker-window.js` | 2445-3216 | 772 | 66,737 | View modes `modeBody`..`calendarBody` (kanban/activity/due/gantt/calendar); item reference card `crefHtml`; sync state `unpushedN`..`drawerHtml` incl. `TABS`, `layoutOf`; item window `commentHtml`..`refLinksHtml` incl. `visibleRecs`, `ccHtml`, `windowHtml` |
| 5 | `tracker-forms.js` | 3217-3952 | 736 | 47,054 | Markdown editor `editorHtml`..`savedAtts`; mention/reference popup `MP`..`mentionInsert`; dialog forms `optsHtml`..`closeDialog` incl. `ROLE_TYPES`, `DIALOGS`, store registry (3648-3795), `asciiId`; actions `toastTimer`, `toast`, `commit`, `wrote`, `submit`, `quick` |
| 6 | `tracker-menu.js` | 3953-4852 | 900 | 61,380 | Context menu `MN`..`menuKey` incl. `closeMenu`, assignment picker `asg*`, `menuAct`; date range picker `monAdd`..`menuFocusOut` incl. `range*`, `menuApplyClose`, `menuDiscard`, `menuOutside` |
| 7 | `tracker-writes.js` | 4853-5671 | 819 | 50,508 | Inline writes `nrowFill`..`dropOn` incl. new-row, insert markers `ins*`, drag-move `mv*`, sub rows, label step `label*`, item drafts, `saveEdit`/`saveCedit`; sync actions `syncNow`, `checkNow` |
| 8 | `tracker-render.js` | 5672-6616 | 945 | 57,295 | Render: drawer height/drag `DR_MIN`..`drDragEnd`, handles, `render`, open/collapse/route `openStore`..`leavePage`; palette `QSLOTS`..`qShortcut`; tooltip/link helpers `tipFor`..`STEPS`; display state `dsLoad`, `dsSave` |
| 9 | `tracker-mount.js` | 6617-7217 | 601 | 45,072 | Mount: `SPRITE`, `el`, `buildDom`, file helpers, `imgRetry`, `listen` (6737-7156), `mount` (7157-7211), `window.WsTracker` export (7213-7216), IIFE close `})();` 7217 |

Total 7217 lines. Largest new file 945 lines. Retained `tracker.js` is 1150:
below the 2000 hard advisory; the soft (>1000) check fires only on a crossing
from <=1000, and `tracker.js` goes from 7217 down, so no advisory; every new
file stays <=1000, so no new-file soft advisory either
(`agents-plugin-tool/internal/mcp/git_merge_size.go:222-232`). No
`.gitattributes` `ws-size-guard` entry exists or is needed for these paths.
If the lead wants every file <=1000, use a 10-file variant: retained
`tracker.js` 1-658 (header, shared state, small views, viewer state),
`tracker-rows.js` 659-1281 (item rows, cards), `tracker-panel.js` 1282-1777,
and the remaining rows unchanged.

### Commit sequence (one move-only commit per destination)

- **Commit 0, scaffolding, no move (lead decision, see below):** add
  `trackerScriptFiles = []string{"tracker.js"}`, the `case`, the cjs declaration
  and the assembly-test row, and switch every raw test reader (Hazard 2) to the
  logical reader. No embed change. Byte-identical by construction.
- **Commits 1-8, peel from the tail:** `tracker-mount.js`, `tracker-render.js`,
  `tracker-writes.js`, `tracker-menu.js`, `tracker-forms.js`, `tracker-window.js`,
  `tracker-wiki.js`, `tracker-panel.js`. Each commit truncates `tracker.js` to
  lines 1..(start-1), creates the destination from the range above, inserts its
  name **directly after `"tracker.js"`** in `trackerScriptFiles`, and updates the
  assembly-test row. Commit 1 also appends `assets/tracker-*.js` to the embed
  line. Peeling from the tail keeps `tracker.js` a prefix, so the remaining
  ranges keep their 616271b51 coordinates throughout.
- Extraction per commit (bash, repo root):
  `A=6617 B=7217 N=tracker-mount.js; P=agents-plugin-tool/internal/wsdashboard/assets; git show 616271b51:$P/tracker.js | sed -n "${A},${B}p" > $P/$N; git show 616271b51:$P/tracker.js | sed -n "1,$((A-1))p" > $P/tracker.js`.
  Do not hand-edit any moved line.

## JS hazards

1. **Scope/strictness/hoisting: moot, confirmed.** The single IIFE opens at
   `tracker.js:222`, `'use strict'` is its first statement (`:223`), and it closes
   at `:7217`. Concatenation recreates the identical function body, so strict
   mode, function-declaration hoisting, and `var` hoisting are unchanged. The
   file uses no top-level `let`/`const` (`grep -n '^  \(let\|const\) '` is
   empty), so there is no TDZ exposure in any case. Only the opening (unclosed
   IIFE) and closing (`})();`) fragments fail `node --check` alone; the seven
   middle fragments parse standalone, which shows no string, comment, template or
   brace crosses an internal cut. No fragment is a runnable standalone script.
2. **Raw test readers break or pass vacuously after the split.** 18
   `dashboardFS.ReadFile("assets/tracker.js")` sites in 12 files would read only
   the opening fragment: `search_palette_test.go:17,103`, `support_ui_test.go:52`,
   `tracker_assignees_test.go:19`, `tracker_busy_retry_test.go:31`
   (`trackerJSBlock`, also used by `drawer_height_test.go:49` and
   `tracker_store_registry_test.go:37`), `tracker_cc_test.go:14` (`trackerBlock`,
   also `trackerFn`), `:53`, `:165`, `tracker_item_route_test.go:18`,
   `tracker_quickadd_test.go:387`, `tracker_search_test.go:17`,
   `tracker_settings_test.go:15,70`, `tracker_store_label_test.go:16`,
   `tracker_team_drill_test.go:22`, `tracker_wiki_test.go:18,137,490`. Replace each
   with `dashboardTestAsset("assets/tracker.js")` (`asset_source_test.go:11-13`),
   a reader substitution only; assertions unchanged. Negative assertions would
   otherwise pass vacuously: `tracker_wiki_test.go:490-500` (no `message`
   listener or sandbox markup), `search_palette_test.go:51,62`. The two
   `mustAsset(t, "tracker.js")` sites (`pincol_width_test.go:181`,
   `shell_search_test.go:36`) already go through `readDashboardSource`
   (`pageview_band_test.go:150-157`) and need no edit. Node readers:
   `testdata/search-palette.test.cjs:7` (blocks at 6110-6546 and 7035;
   negative check at `:403`) and `tracker_draft.test.js:7` read the file by path.
   Switch both to `readAssetSource('wsdashboard', 'tracker.js')` from
   `testdata/asset-source.cjs`.
3. **Mockup loads tracker.js by `<script src>` (needs a decision).**
   `ai-docs/ref/tracker-mockup/index.html:84` loads
   `../../../agents-plugin-tool/internal/wsdashboard/assets/tracker.js` from
   `file://` (README `:17-40`). After the split it would load an unclosed IIFE
   (SyntaxError). Separate `<script>` tags per fragment cannot work, because the
   fragments share one closure. Options: (a) a generated assembled copy beside
   the mockup with a regen switch and staleness test, matching the existing
   `i18n-en.js` pattern (`tracker_mockup_i18n_test.go:11-44`). It would be about
   7217 lines of `.js` and need a `linguist-generated` attribute to stay out of
   the size guard. (b) accept or document the mockup breakage. (c) retire the
   mockup, which falls under "Always ask". Nothing else in `scripts/` (including
   `scripts/board-headless.mjs`) or other packages reads tracker.js by path;
   `scripts/palette-headless.mjs` drives a live daemon and gets the assembled page.
4. **Asset-identity tests omit tracker.js today.** `assets_id_test.go:65-67`
   lists `dashboardScriptFiles`, `dashboardStyleFiles` and `tracker.css` but not
   `tracker.js`; `:81-86` iterates only the dashboard lists. Adding
   `trackerScriptFiles` to both is optional extra coverage. The second test edits
   and restores working-tree source files.
5. **Notes only, no edit:** comments naming `tracker.js <fn>` (e.g.
   `wslive/ui.go:32-33`, `wswhiteboard/render.go:157`, `dashboard-tracker.js:2-5`,
   `tracker.css:2`, many `tracker_*_test.go` doc comments) stay accurate as the
   logical name. `wslive/testdata/historical-summary/wsagent.json:137` is a
   frozen historical hash fixture. `tracker.js:<line>` citations exist only in
   `ai-docs/tickets/.done/` and `ai-docs/.plans/` (historical; do not rewrite).
   No live doc or manual cites tracker.js line numbers.
6. **Quiet point.** At this snapshot no local branch carries unique edits to
   `tracker.js` or `pages.go` beyond develop (the two refs that appear contain
   only develop merges). Re-check before starting; any parallel tracker.js edit
   conflicts with every move commit.

## Verification gates (macOS)

Run after commit 0 and after every move commit. Baseline status at the
unmodified tree: all listed commands below passed (narrow Go sample `ok` in
3.45 s; search-palette `exit 0`; tracker_draft 7/7 pass).

Byte identity through the registration list (repo root, bash):

```sh
cd agents-plugin-tool/internal/wsdashboard
node -e "process.stdout.write(require('./testdata/asset-source.cjs').readAssetSource('wsdashboard','tracker.js'))" > "$TMPDIR/tracker-assembled.js"
git show 616271b51:agents-plugin-tool/internal/wsdashboard/assets/tracker.js | cmp - "$TMPDIR/tracker-assembled.js" && echo BYTE-IDENTICAL
shasum -a 256 "$TMPDIR/tracker-assembled.js"   # expect 0bba48f6ac4d5324792cdaa05085aab22b93e3a86aab20115319077267f2aecf
node --check "$TMPDIR/tracker-assembled.js"
```

Independent of the cjs parser (final state; drop missing names mid-series):

```sh
cd agents-plugin-tool/internal/wsdashboard/assets
cat tracker.js tracker-panel.js tracker-wiki.js tracker-window.js tracker-forms.js tracker-menu.js tracker-writes.js tracker-render.js tracker-mount.js \
  | cmp - <(git show 616271b51:agents-plugin-tool/internal/wsdashboard/assets/tracker.js) && echo BYTE-IDENTICAL
wc -l tracker*.js   # every file < 2000; new files <= 1000
```

Go, from `agents-plugin-tool/` with `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`:

```sh
gofmt -l internal/wsdashboard            # expect empty
go build ./...
go vet ./internal/wsdashboard
go test -run '^$' ./internal/wsdashboard
go test -count=1 -timeout=5m -run '^(TestDashboardSourceAssembly|TestAssetIDFollowsAssetContent|TestSearchPalette.*|TestSearchDrawerSplitOwnership|TestUIScopesMatchTheTrackerRenderer|TestTrackerStopsAtItsWidestLayout|TestShellSearchButtonClick|TestDrawersShareOneHeight|TestTracker(Assignee|Wiki|Related|CC|ItemWindowCC|ItemLink|Selection|QuickAdd|Range|Search|Store|Team|Take|Move|NoFrame|Labels|Priority|Calendar|Gantt).*)$' ./internal/wsdashboard
```

Optional only if Hazard 4 coverage is added: `-run '^TestDevAssetsIDFollowsEverySourceFragment$'`
(writes and restores source files). Never run the whole `./internal/wsdashboard`
package without `-run` under this allocation.

Node (repo root):

```sh
node agents-plugin-tool/internal/wsdashboard/testdata/search-palette.test.cjs
node --test agents-plugin-tool/internal/wsdashboard/tracker_draft.test.js
```

Review: moved-line-aware diff (`git diff --color-moved=dimmed-zebra -M`) shows
only moved blocks plus the registration lines; the byte-identity `cmp` is the
acceptance criterion.

## Interaction with 261116-feat-ws-dashboard-session-page-payload-and-render-cost

- Its Phase 1 externalizes every remaining inline asset, including
  `TrackerScript` (`dashboard.tmpl:37,371`; `pages.go:281`), to content-hashed
  immutable URLs. The split changes no bytes, so its hash inputs and payload
  measurements are unaffected in either order.
- Shared surfaces: `pages.go` (embed line 34, `buildDashboardAssets`,
  `readDashboardSource`), `assets_id_test.go`, `asset_source_test.go`, and
  possibly the `pages_test.go`/`toast_test.go` placement assertions. These are
  small textual conflicts. Neither ticket edits `tracker.js` content.
- Semantic requirement either way: 261116 must hash and serve the **assembled**
  source via `get("tracker.js")`/`readDashboardSource`, never
  `dashboardFS.ReadFile("assets/tracker.js")`, and its new tests must read
  through `mustAsset`/`dashboardTestAsset`. If 261116 lands first, re-grep its
  diff for raw tracker.js reads before the split.
- Naming: a future hashed route such as `tracker-<hash>.js` would not collide
  with fragment files (the existing route matches an exact `ScriptFile` name,
  `pages.go:1299`), but the lead may prefer a distinct prefix for readability.
- **Recommended order: split first.** It is a short, quiet-point, byte-identical
  series that rebases cheaply now, while 261116 is a high-risk, multi-file
  branch that will be long-lived. Landing the split first also makes the
  assembled-reader contract exist before 261116 writes new tracker readers.
  Do not run both concurrently on `pages.go`.
