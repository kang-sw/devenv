---
title: "Dashboard agent host: a read-only file browser with image, video, and sandboxed HTML preview"
parent: 261110-epic-ws-dashboard-agent-host
related:
  261106-epic-ws-dashboard-serve-daemon: "its Non-Scope (no file explorer) is partly reopened here, for browsing and preview only"
  261110-feat-ws-dashboard-terminal: "shares the board card framework (Decision 10)"
sage-review-design: completed
sage-review-completeness: completed
sage-review-completeness-reviewed: 0c313a4b22992166
sage-review-design-reviewed: 0c313a4b22992166
completed: 2026-11-11
---

# Dashboard file preview

## Background

Track B of the epic: let the user see what an agent produced without leaving
the dashboard: browse a folder and preview HTML, images, and video
(2026-11-10 whiteboard, (01), (04)).

## Decisions

1. **Browse and preview only; editing is out of scope entirely (user,
   (04) a and note).** A file tree, and previews of HTML, images, and video,
   plus Markdown and diffs where useful, and an "open in external editor"
   action (the OS default app, Decision 7). Rejected: light single-file editing ((04) b); a full editor
   ((04) c). The 2026-07-30 drop for Orca already judged the editor side as
   territory others do better (`260730-research-ws-dashboard-drop-for-orca`).
2. **HTML and SVG render in isolation (technical, (07)).** HTML from a
   workspace renders on an origin separate from the dashboard, or in a
   sandboxed iframe without `allow-same-origin`, so it cannot reach the
   dashboard token or API; dashboard access already equals execution
   authority ((05)). SVG can carry script, so it is treated like HTML.
   Images and video are served statically from the dashboard origin.
3. **LAN token holders browse read-only inside the root (announced, (105)
   ok).** As the session file view already does (`file_route.go`); reads
   outside the root stay `localViewer` only. The epic's trusted-origin
   boundary covers approval and the terminal.
4. **HTML preview through a capability path (technical, (106) ok).** HTML
   and SVG load in an iframe with `sandbox="allow-scripts"` and no
   `allow-same-origin`, from a preview route whose path carries a
   short-lived, root-scoped read capability, so relative assets resolve
   without cookies; the route's CSP blocks requests to other origins.
   The daemon mints the capability when the page opens an HTML or SVG
   preview, only for the session root (Constraints), with a 15-minute
   lifetime; the card re-requests one when it reloads, and browser storage
   (Decision 12) keeps the file path, never the capability URL, so a
   restored card gets a fresh one. Because a sandboxed frame sends no
   cookie, the route is admitted by its capability rather than the LAN
   token, the same shape as the `publicAsset` exception in `access.go`,
   documented beside it. The CSP limits `default-src`, `connect-src`,
   `img-src`, and the other fetch directives to the capability's own path
   prefix, so the frame cannot request other dashboard routes either.
   Under a root picked with change root, where no capability is minted, an
   HTML or SVG file shows its source as plain text (Decision 6) with the
   "open in the default app" action (technical, design review).
   Rejected: `srcdoc`, which cannot load relative assets, while Chromium
   sends no SameSite=Lax cookie from a sandboxed frame (a068fb9f).
5. **Video streams with Range (technical, (107) ok).** mp4, webm, and ogg
   are served through `http.ServeContent`, as images are, with no size cap,
   and a test covers Range. Rejected: a size limit; the 2 MiB cap is for
   text rendering.
6. **Other text stays plain (technical, (108) ok).** No highlighting in
   Phase 1; `.diff` and `.patch` render through `gitdiff.go`. Rejected:
   highlighting other text in Phase 1. Fact population found chroma
   already a dependency (`gitdiff.go#L698-L732`), so highlighting needs no
   vendoring and is a cheap follow-up rather than the vendoring cost this
   line first cited.
7. **Open in the default app and reveal in the file manager (technical,
   (109) ok; technical, (134) ok).** One loopback-only route takes an
   in-root path and an action field, `open` (the OS default app) or
   `reveal` (the OS file manager with the entry selected), and checks the
   system-open gate (`localViewer` and `sameOrigin`) once for both. Reveal
   per OS: Windows `explorer /select,<path>`; macOS `open -R <path>`; Linux
   `xdg-open` on the parent folder; WSL converts with `wslpath -w` and runs
   `explorer.exe /select,<path>` (today's system-open path has no WSL
   handling). Rejected: widening the system-open route itself; one route per
   action, which would repeat the gate.
8. **The tree lives in a Files tab of the conversation column (user,
   (121) b and note).** The column's header switches "Conversation |
   Files"; the Files tab shows the tree at full column height, and returning
   to the conversation keeps the tree's expanded folders, filter, and
   scroll and the composer draft. Opening a file shows its preview as a
   card in the board column; below the split width, where the tab bar shows
   one column at a time (`dashboard.tmpl`), opening a file switches to the
   board column. Rejected: a sheet over both columns
   ((121) a); the tree and preview together as one board card ((121) c).
9. **No top-bar Files button (announced, (122) ok).** The Files tab is the
   only entry, as a direct consequence of Decision 8.
10. **Preview cards are a kind on a shared board card framework
    (technical, (123) ok; user, (123) note).** The framework owns what every
    card kind shares: fold, base height, drag handle, the maximize sheet
    with Esc, pin, close, the persistence of that view state, and placement
    in the board flow (`261110-feat-ws-dashboard-terminal` Decision 4). A
    kind supplies its header content, its extra header actions, its body,
    and its lifecycle; terminal and preview are the first two kinds, and
    later kinds plug in the same way (technical, (129) ok). Each kind
    supplies its folded content and base height: a preview card folds to its
    header (kind icon and path), and its base height matches a 14-row
    terminal card. Card nodes are fixed anchors in the board morph
    (`morphBoard`, `dashboard.js#L452`) or live outside its swapped
    children, so a board update never removes, moves, or reloads a card (an
    iframe or video would reload). Pinning moves the card into the pinned
    group and may reload an HTML preview or restart a video; that reload is
    accepted. Cards keep the
    board column's standard gap between them, as topic cards do (user,
    2026-11-10 whiteboard note on the mockup). The preview kind adds an
    "open in the default app" action (Decision 7); wide HTML and video use
    the maximize sheet. Rejected: a separate preview layout (two card
    models); kind-owned placement, which a later right-edge dock would have
    to change in every kind. The framework persists each unpinned card's
    open time with its view state and rebuilds the card's place in the
    topic flow from it on reload, so a restored card does not jump to the
    head (technical, design review).
11. **One preview slot (user, (126) c; technical, (124) ok).** Opening a
    file replaces the content of a single unpinned preview card; replacing
    its content counts as an open, so the card moves to the head of the
    unpinned flow (Decision 21). Pinning it makes it a pinned card, and the
    next file opens in a new slot. Opening a file that already has a pinned
    card scrolls to that card and reloads it instead. Rejected: up to five
    unpinned cards with the oldest closing ((126) a); no cap ((126) b);
    duplicate cards for one file.
12. **Open previews persist in the browser (technical, (125) ok).** The
    slot and the pinned preview cards are kept per session in browser
    storage and restored on reload. Rejected: a server-side record (pure
    view state needs no new server contract).
13. **Conversation file links open the preview slot (user, (127) a).** A
    plain click opens the preview card; Ctrl/Cmd-click keeps today's file
    view page in a new tab. Today a plain click on a marked link opens the
    link menu (`dashboard.js#L1540-L1549`); for a file link the preview
    can show, that menu moves to right-click and to the keyboard
    activation, and it keeps its entries; a link the preview cannot show
    (outside the root, or not viewable) keeps the menu on plain click
    (technical, fact population). The page decides before the click from
    what it already knows: the in-root signal (`linkOpens`, `data-out`)
    and the file extension; a link that passes but the server then finds
    binary or oversize opens the card in its "not viewable" state with
    "open in the default app". Shift- and Alt-click keep passing through
    as today (`dashboard.js#L1546`) (technical, design review). Rejected: links unchanged, previews only
    from the Files tab ((127) b); retiring the menu for file links (its
    copy-path entries have no other home).
14. **Icons from Lucide (technical, (130) ok).** Only the icons used are
    vendored from Lucide (ISC), with its license file, the way mermaid is
    vendored (`261110-feat-ws-dashboard-terminal` Decision 6); they serve
    the tree's file kinds and the card header buttons. Rejected: a
    per-extension colored icon theme (material-icon-theme and the like),
    loud against the dashboard's quiet tone; Codicons, which need an
    on-screen attribution.
15. **Name and content search (technical, (131) ok).** The tree's search
    box switches "Name | Content". Name search reuses the composer `@`
    mention fuzzy matcher over the same file list and shows flat results.
    Content search runs in Go on the server over that list: literal and
    case-insensitive by default, with toggles for case sensitivity and
    regular expressions; it skips binaries and files over 2 MiB and stops at
    200 files or 2000 matches with a "more" marker. Results group by file
    with line excerpts; clicking a match opens the preview slot scrolled to
    that line. The file list is the per-root git list `files_route.go`
    already caches, or its capped walk outside a repository. That list uses
    `--exclude-standard` (`files_route.go`), so name and content search do
    not find gitignored entries even while the View toggle shows them;
    accepted, since the toggle is for browsing. Rejected:
    shelling out to ripgrep, which a downstream machine or native Windows
    may lack.
16. **Ignored and dot entries (technical, (132) ok).** Gitignored entries
    are hidden by default and dot entries shown; a "View" popover toggles
    both, remembered per root in browser storage; shown ignored entries are
    dimmed. Rejected: showing everything, which buries the tree under
    pooled worktrees and build output.
17. **Git status marks (technical, (133) ok).** M, A, U, and D marks on files,
    and a mark on folders holding changes, come from
    `git status --porcelain=v1 -z --no-renames`, as `gitdrawer.go` already
    runs it, fetched with the listing (a rename shows as a deletion plus an
    addition), refreshed by the
    refresh button or by reopening the Files tab. Rejected: a live
    file-system watcher (per-OS mechanisms; the tree only needs to be right
    when looked at).
18. **Row menu (technical, (134) ok).** Right-click or the keyboard opens a
    row menu: preview, open in the default app, reveal in the file manager,
    copy relative path, copy absolute path. The two host actions use the
    Decision 7 route and show only to loopback viewers. Folders also offer
    "open terminal here", shown only once
    `261110-feat-ws-dashboard-terminal` has landed.
19. **Explorer header and keyboard (technical, (135) ok).** The header holds
    the root, change root (loopback only), collapse all, refresh, and the
    View popover. Keys: arrows move, Right/Left expand and collapse, Enter
    previews, `/` focuses search. Rejected: mouse only.
20. **Two phases (technical, (136) ok).** Phase 1: tree, previews, icons,
    name search, View toggles, row menu, keyboard. Phase 2: content search
    and git status marks, the parts that add new server search paths and
    verify on their own. Rejected: one phase.
21. **One placement rule for every card kind (policy, (137) a, assumed
    under the user's 2026-11-10 delegation of remaining details).** A card
    enters at the head of the unpinned flow, just below the pinned cards,
    when it is opened; replacing the preview slot's content counts as an
    open. After that its own activity never moves it up; it moves down as
    updated topics rise above it (`261110-feat-ws-dashboard-terminal`
    Decision 4). Pinned cards stay at the top. The framework owns this rule
    for every kind. Rejected: the slot standing under the pinned cards ((137)
    b), the fixed position terminal Decision 4 rejected ((37) b), and a
    per-kind rule ((129)).

## Prior Art

- The session file view, `GET /s/<reply-id>/file?path=`
  (`internal/wsdashboard/file_route.go` header): root containment with
  symlink checks; images served under the image route's CSP so an SVG runs
  no script; Markdown rendered through the board renderer in escaped mode;
  other UTF-8 text served as `text/plain` under that CSP, so an `.html` file
  never runs today; binaries get a "not viewable" page. It serves loopback
  viewers and LAN token holders alike, on the reasoning that the token
  already lets its holder message a lead that reads any file.
- The composer's `@` file mention list, `GET /s/<reply-id>/files`
  (`files_route.go`): git's file list or a capped walk, cached per root.
- The system-open route (`sysopen_route.go`) opens an http(s) URL or this
  session's file view in the host's default browser, or an image file (an
  absolute path with an image extension, nothing else) in the host's default
  app, loopback viewers only (`sysopen_route.go#L13-L41`).

## Constraints

- Independent of the session holder and of daemon lead sessions; it can
  land at any time.
- The default browsing root is the session's project root, the root the
  session file view, the file-link check, and the files list already use
  (`file_route.go`, `files_route.go`). "Change root" (loopback only,
  Decision 19) may offer another folder and, once
  `261110-feat-ws-dashboard-workspaces` exists, its workspace and worktree
  entries; capabilities for HTML preview are minted only for the session
  root (Decision 4).
- Reads stay inside the chosen root: no path traversal out of it.
- The board card framework is shared with terminal cards (Decision 10).
  Whichever of this ticket and `261110-feat-ws-dashboard-terminal` lands
  first builds it as one shared component; the other adds its kind to it.
- Out of scope: a wide-view dock at the right edge, where a card is dragged
  to and stays pinned, which the user is considering ((123) note). The
  framework keeps placement out of the card kinds (Decision 10) so such a
  dock needs no change to them.
- Manuals: `ai-docs/manuals/shipped-surface-boundary.md`.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)

## Prior Decisions

- 261110-research-ws-dashboard-agent-host-foundations (2026-11-10, Confirmed Decisions): "File browsing is read-only with HTML, image, and video preview; editing is out of scope; HTML and SVG render isolated ((04) a, (07)); child `261110-feat-ws-dashboard-file-preview`." — bearing: supports
- bdd21be7 (2026-11-10, commit): "(90) a: terminal and file-preview stay todo until HTML mockups settle the terminal card (size, expansion, fit resize, open location) and the file browser placement; their technical decisions (101)-(109) are recorded now." — bearing: supports
- 261110-feat-ws-dashboard-terminal (2026-11-10, Decisions): "Card size and expansion (user, (118) a). A terminal card is folded (its header and the last output line), base (14 rows, the default), or a height the user drags with a handle under the card, remembered in the browser." — bearing: supports
- 30925622 (2026-11-07, commit, 261107-feat-ws-dashboard-link-actions): "Cmd/Ctrl+click is left native rather than window.open so the browser's own new-tab behavior (background tab, middle click) is kept." — bearing: constrains
- 2c1ae57e (2026-11-09, commit, 261109-feat-ws-dashboard-open-out-of-root-images): "Markup stays viewer-independent (entries render once through the shared entry cache): the server gates loopback on the route, the page JS offers the open items for a data-out link only when the page carries data-sysopen" — bearing: constrains
- 261109-feat-ws-dashboard-composer-file-mention (2026-11-09, Decisions): "Absolute browsing is loopback only: listing a directory outside the root is served only to a loopback viewer (`localViewer`, enforced on the server, not just hidden in the page); a LAN viewer gets root search only." — bearing: constrains
- 0336011f (2026-11-07, commit, 261107-feat-ws-dashboard-link-actions): "Non-local viewers get 404 from the open route (not 403) so the route looks absent to a LAN token holder and TestLANModeGuardsEveryRoute stays uniform; both new routes are added to that test." — bearing: constrains
- a068fb9f (2026-11-07, commit): "Images: a measured check showed Chromium sends no SameSite=Lax cookie from a sandboxed srcdoc frame, so in LAN mode the image route would refuse Now images." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsdashboard/file_route.go, files_route.go, sysopen_route.go, gitdiff.go, gitdrawer.go, pages.go route switch, access_test.go, assets/dashboard.tmpl, dashboard.css, dashboard.js, plus a vendored Lucide icon set |
| scope.surface | public-interface | new daemon HTTP routes under /s/<reply-id>/ served to loopback and LAN viewers, plus the conversation file-link click behavior |
| scope.new_public_symbol | yes | Phase 1 adds a directory listing route, a capability-path HTML and SVG preview route, a video preview path, and a loopback-only open/reveal route; Phase 2 adds a content-search route |
| scope.new_type_contract | yes | directory listing answer shape with ignored and git status marks, the open/reveal action field, the short-lived root-scoped capability path format, the content-search answer, and the browser-storage record of slot, pinned cards, and View toggles; files_route.go filesAnswer is the nearest existing contract |
| scope.test_surface | new-files | new routes need tests beside file_route_test.go, files_route_test.go, and sysopen_route_test.go; access_test.go TestLANModeGuardsEveryRoute must list every new route; headless Chromium checks for sandbox, video, tabs, slot, keyboard, and search |
| complexity.reuse_points | confirmed | openContained file_route.go#L125, http.ServeContent file_route.go#L201 and image_route.go#L454, fileViewMax file_route.go#L47, chroma highlighting in gitdiff.go#L698-L732, the cached git list or capped walk in files_route.go, the composer mention fuzzy search dashboard.js#L1065, git status porcelain -z parsing gitdrawer.go#L79-L95, sysopen_route.go localViewer and sameOrigin gate |
| complexity.side_effect_risk | moderate | plain click on a conversation file link stops opening the link menu dashboard.js#L1540-L1549 and opens the preview slot for every viewer; the conversation column gains a tab header; the open/reveal route runs host commands |
| risk.correctness | moderate | root containment, symlink and swap checks, capability expiry and root scoping, Range handling, per-OS reveal commands including WSL path conversion, and search caps must hold |
| risk.fit | high | the shared board card framework is built by whichever of this ticket and 261110-feat-ws-dashboard-terminal lands first, with the framework and kind split of Decision 10 |
| risk.test | high | sandbox isolation, video seek, tab state retention, slot pin and scroll, reload restore, and keyboard navigation need headless browser checks beyond Go handler tests |
| risk.security_or_contract | high | renders workspace-authored HTML and SVG next to an origin whose token equals execution authority, through a new capability path, and adds a host-side open/reveal route |

## Phases

### Phase 1: tree and previews

Scope:

- Routes under the session: a directory listing for the browsing root
  (Constraints) with `openContained`'s containment and symlink checks;
  previews of images and video (`http.ServeContent`, Range, Decision 5),
  Markdown through the board renderer in escaped mode, `.diff` and `.patch`
  through `gitdiff.go`, and other text as plain (Decision 6); the HTML and
  SVG capability-path preview route with its CSP (Decision 4); the
  loopback-only open/reveal route with its action field (Decision 7). LAN
  access follows Decision 3.
- Frontend: the conversation column's "Conversation | Files" tabs and the
  tree (Decision 8); no top-bar button (Decision 9); preview cards as a kind on
  the shared board card framework (Decision 10); the single preview slot and pinned
  preview cards (Decision 11); browser-storage restore (Decision 12); file
  links in conversation messages open the slot, Ctrl/Cmd-click opens the
  file view page (Decision 13).
- Explorer: vendored Lucide icons (Decision 14); name search through the
  composer `@` matcher (Decision 15, name half only); the View popover with
  the ignored and dot toggles and dimmed ignored rows (Decision 16); the row
  menu (Decision 18); the header and keyboard navigation (Decision 19).

Verification:

- Go tests: traversal and symlink escapes are refused on the listing and
  every preview route; a capability serves only its own root and stops
  working once it expires; the HTML preview carries the CSP and the sandbox
  attributes; a video request with Range gets 206 with the right bytes; a
  LAN token holder browses inside the root and gets 404 outside it and on
  the open/reveal route; the open/reveal route refuses an unknown action
  and an out-of-root path, and builds the per-OS reveal command (WSL
  included) as a unit-tested pure function; the listing marks ignored
  entries; change root answers 404 to a LAN viewer; every new route except
  the capability preview route is in `TestLANModeGuardsEveryRoute`; the
  capability route refuses a missing, foreign-root, or expired capability
  and serves a LAN peer holding a valid one, without the LAN token.
- Headless Chromium: sandboxed HTML loads its relative assets but cannot
  read the parent document, the dashboard cookie, or the dashboard API;
  video plays and seeks; switching tabs keeps the composer draft and the
  tree state; the slot replaces, pins, and scrolls to a pinned card as in
  Decision 11; a reload restores the slot and pinned cards; a link click
  opens the slot and a Ctrl/Cmd-click opens the file view page; the View
  toggles hide and show ignored and dot entries and survive a reload; the
  row menu shows host actions to a loopback viewer only; keyboard
  navigation moves, expands, collapses, and previews; a POST from the
  sandboxed frame to a dashboard route is refused and a request outside the
  capability prefix is blocked; name search finds a file; Markdown and a
  `.diff` render in the card; the file-link menu opens on right-click and
  the keyboard for a previewable link and on plain click for a link the
  preview cannot show; on a narrow layout, opening a file switches to the
  board column.
- Board card framework, when this ticket builds it (Constraints): fold,
  base height, a dragged height remembered across a reload, maximize and
  Esc, pin, close, and the Decision 21 placement (a card enters at the head
  of the unpinned flow and moves down as a topic updates); a playing video
  and a loaded HTML preview keep playing and stay loaded through a board
  update; a reload restores an unpinned card at its earlier place in the
  topic flow.
- When the framework already exists (built by
  `261110-feat-ws-dashboard-terminal`): a preview card folds to its header
  and opens at the 14-row-terminal base height, and enters by the
  Decision 21 placement.
- The vendored Lucide icons ship with Lucide's ISC license file
  (Decision 14).

### Result (d9de23c54) - 2026-11-11

Landed in `7f890500d`, `13d6c3c7d`, `434bfac25`, `e1f3e1519`, and the review
fixes in `d9de23c54`:

- Routes under `/s/<reply-id>/fs/`: `list`, `preview`, `raw`, `cap/<capability>/<path>`,
  and `act` (`fs_routes.go`, `fs_capability.go`, `fs_act_route.go`). The
  capability is a stateless HMAC over the reply-id, the session root, and the
  expiry, under a key the daemon draws at start; the capability response
  sandboxes itself through its own CSP, so the file opened in a tab of its own
  is isolated too.
- The shared board card framework (`assets/cards.js`, `window.WsCards`): this
  ticket built it. It owns the header buttons, base height (260 px for every
  kind unless a kind overrides it), the drag handle, the maximize sheet,
  placement, and the browser-storage record. The board morph weaves the cards
  into the topics and settles around them without moving a card.
- The preview kind (`assets/preview.js`, `window.WsPreview.open(path, {root, line})`),
  the conversation link behavior, the "Conversation | Files" switch, and the
  explorer (`assets/files.js`): lazy tree, View popover, change root, collapse
  all, refresh, row menu, keyboard, and name search.
- Lucide icons vendored as unchanged `lucide-static` 1.71.0 files with the
  package's license under `assets/lucide/`, served as one inline sprite.

Verification:

- `go test ./internal/wsdashboard/ ./internal/wsi18n/ ./internal/mcp/ -count=1`
  on the remote builder: `ok` for all three; `go build ./...` and `go vet` of
  the two changed packages clean. The Go tests cover every item of this
  phase's Go list (containment and symlink escapes on every read route, the
  capability's root, expiry, CSP, and LAN admission, Range, the per-OS and WSL
  host commands, ignore marks, change root for a LAN viewer, and
  `TestLANModeGuardsEveryRoute`).
- Headless Chromium against a fixture daemon, all passing: 42 checks of the
  cards and previews (sandbox isolation: parent, cookie, storage, a dashboard
  route, the index, another origin, and a POST are all blocked while relative
  assets load; video plays and seeks; slot replace, pin, and pinned-file
  reopen; a board update lets a newer topic rise above an unpinned card while a
  running frame and a playing video keep running; fold, base height, dragged
  height, maximize and Escape; a reload restores cards, places, height, and
  fold), 19 checks of link behavior, host actions, and the narrow layout, and
  102 checks of the Files tab (state kept across the switch, View toggles
  across a reload, row menu and its loopback-only host actions, keyboard,
  name search, change root, split and tabs layouts, dark theme).

Decisions taken under the delegation of small details:

- `.git` is always marked ignored; ignore and status marks are asked from the
  listed directory, so a nested repository answers by its own rules.
- A keyboard activation of a previewable link opens the link menu (with a new
  first entry, Preview) instead of the slot, so every link action stays
  reachable by keyboard.
- Unpinning a preview card makes it the slot and closes the other unpinned
  preview card.
- A card's open time is the later of the browser's clock and the newest topic
  or card plus one, so a LAN browser whose clock runs behind still opens at the
  head.
- The Files tab choice is not kept across a reload; the View toggles (per
  root, 24 roots at most) and a changed root (per session) are.
- The changed root is typed as an absolute path; a browser has no folder
  dialog that yields one.
- Under a changed root Markdown renders without file links, and HTML or SVG
  shows as source.
- On WSL, `open` runs `explorer.exe` on the converted path, as reveal does.
- "Open terminal here" is left out of the row menu until the terminal ticket
  lands.

Known limits:

- A previewed HTML file can read any file under the session root through its
  capability, and CSP does not govern a frame navigating itself, so such a
  page could carry what it read to another origin. The dashboard token, cookie,
  and routes stay out of its reach. Whether to narrow the capability (for
  example to the file's own folder) is open with the user.
- A capability lives 15 minutes and is renewed only when a card loads; a
  frame left open longer can no longer fetch further assets until the card is
  reopened.
- Escape does not leave maximize while focus is inside a sandboxed frame; the
  minimize button and the scrim do.
- A symlink row and the "more entries" row are covered by the unit test only,
  not in the browser.

### Phase 2: content search and git status marks

Scope:

- A content-search route under the session over the Decision 15 file list,
  with the literal, case, and regex modes, the binary and 2 MiB skips, and
  the 200-file and 2000-match caps; the "Name | Content" switch and grouped
  results, a match opening the slot at its line (Decision 15).
- Git status marks from `git status --porcelain=v1 -z --no-renames` delivered with the
  listing, on files and on folders holding changes, refreshed by the refresh
  button or a Files tab reopen (Decision 17).

Verification:

- Go tests: content search finds literal, case-sensitive, and regex
  matches, skips binaries and oversize files, stops at both caps with the
  "more" marker, stays inside the root, and is in
  `TestLANModeGuardsEveryRoute`; status parsing covers modified, added,
  untracked, deleted, and folder roll-up.
- Headless Chromium: a content match opens the slot scrolled to its line;
  marks show and refresh after a file changes and the refresh button runs.

### Result (d9de23c54) - 2026-11-11

Landed in `adac6e981` (routes), `e1f3e1519` (explorer UI), and `d9de23c54`
(review fixes):

- `GET /s/<reply-id>/fs/search` runs in-process over the cached file list
  with RE2, reading each file through `openContained`; `fs/find` serves the
  name search with the composer's ranking, also under a changed root;
  `fs/preview?text=1` answers any text file as plain text so a match in a
  Markdown or HTML file opens at its line.
- `fs/list` entries carry `st` (`M`, `A`, `U`, `D`, and `*` on a folder that
  holds changes) and `gone` for a deleted entry, read per listed directory.
- The explorer's "Name | Content" switch with case and regex toggles, grouped
  results with a "more" marker, and marks refreshed by the refresh button and
  by reopening the Files tab.

Verification:

- Go tests (same run as Phase 1, `ok`): literal, case-sensitive, and regex
  search, the binary and oversize skips, both caps and the deadline with
  `more`, containment, a LAN viewer under another root, `TestStatusMarks`
  (modified, added, intent-to-add, untracked, deleted, folder roll-up, an
  untracked parent), `TestFSListCarriesGitMarks` on a real repository
  (including a nested repository), and both routes in
  `TestLANModeGuardsEveryRoute`.
- Headless Chromium, within the 102 Files tab checks: a content match opens
  the slot with the marked line in view; the case and regex toggles and a bad
  expression's reason; marks show and change after a file is modified and
  refresh runs, and after the tab is reopened.

Decisions:

- The search stops when its request ends, so a search-as-you-type client's
  abort frees the daemon; a 10-second deadline also ends it with `more`.
- A folder holding changes shows a dot, not a letter, since it may hold any
  mix of changes; the mockup drew `M` there.
- The case and regex toggles are icon buttons with translated titles.

Review: one independent lite pass found no Critical or Important issue and
four Minor ones; three are fixed in `d9de23c54`, and the fourth is the
capability-scope limit recorded under Phase 1.
