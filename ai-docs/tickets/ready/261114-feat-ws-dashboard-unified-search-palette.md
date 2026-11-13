---
title: "Dashboard unified search palette: board, file, content, and tracker slots, a jump strip, streamed async search, and absolute-path mode"
related:
  261112-feat-ws-dashboard-search-palette: "predecessor; this ticket reworks the palette it built and removes its .<id> jump"
  261110-feat-ws-dashboard-file-preview: "built the fs/find, fs/search, fs/list, fs/preview and fs/act routes and the change-root gate this ticket reuses unchanged"
  261113-feat-ws-whiteboard-board-frontend-performance: "concurrent edits to the board assets; board-slot rows come from the existing O.searchRows snapshot, not a new DOM scan; its lazy srcdoc (data-srcdoc until mounted) changes where board-slot body text is read"
  261113-bug-ws-release-gate-sweep-fixes: "Ctrl+K target guard in qShortcut (skips .xterm, textarea, contenteditable); landing-order sensitive, this ticket's key-map rewrite must keep it"
  261113-feat-ws-dashboard-iframe-shell: "the palette lives in the session frame; the shell document forwards Ctrl+K/Ctrl+Space// as a search message and owns the #rail session rows"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: b5535814cfc27023
sage-review-completeness-reviewed: b5535814cfc27023
---

# Dashboard unified search palette: board, file, content, and tracker slots, a jump strip, streamed async search, and absolute-path mode

## Background

Owner request relayed by MAIN (2026-11-13, whiteboard topic
`unified-search`). The owner wants these changes to the Ctrl+K palette:

- Split the results by source.
- Lay the palette out for narrow, medium, and wide widths.
- Search file names and file content asynchronously.
- Drop the `.123` syntax and make `(n)` the exact form.
- Cover the tracker and tracker content.
- Stream results, with cancellation.

The UX session wrote a spec and a mockup, and the owner approved both.

- Spec:
  `ai-docs/tickets/assets/261114-feat-ws-dashboard-unified-search-palette/unified-search-spec.md`
- Mockup:
  `ai-docs/tickets/assets/261114-feat-ws-dashboard-unified-search-palette/search-palette-mockup.html`

Where the two differ, the spec wins. The only difference is the scope chip:
the spec makes it a label, not a widen control. See Decision 4.

Owner answers on the MAIN board (2026-11-13):

- (543) ok: the scope facts restated in Decision 4.
- (547) ok: the mockup.
- (548)=a: today's session, store, and action rows move to the jump/command
  strip.
- (549)=b, with an exception for a typed absolute path. Verbatim:

  > 아, 다만, "절대경로를 직접 입력"한 경우에 한해, 해당 절대 경로 트리
  > depth 1만큼은 검색에 넣고, 절대경로를 직접 입력하는 경우 미리보기에 파일
  > 열기 까지는 두는 게 좋겠습니다. (윈도우 ctrl+r로 파일 열듯이)

  In English: only when the user types an absolute path directly, include
  depth 1 of that path's tree in the search, and let the preview go as far
  as opening the file, like opening a file with the Windows Run dialog.

No owner question is open. The spec was read against develop `f1e45292c`.

UX lead answers (2026-11-13, the spec owner): seven technical and design
questions from the ticket's design review, folded into the spec and into
Decisions 7, 8, and 10-13 below. These are the spec owner's technical calls, not
owner answers; they are cited as "UX spec, 2026-11-13". The assets copy of
the spec was refreshed with them on 2026-11-14.

### The palette today

- The palette lives in the tracker renderer,
  `agents-plugin-tool/internal/wsdashboard/assets/tracker.js`:
  - state `qs` (tracker.js:5686)
  - the fuzzy scorer `qScore` (5689)
  - `qRank` (5705), which holds the `.123` jump (`/^\.(\d+)$/`, 5706)
  - the shortcut test `qShortcut` (5721): Ctrl+K, Ctrl+Space, or `/`
  - `qResults` (5726), which appends stores, actions, and tracker items as
    group 5
  - `qGroup` (5743)
  - `qRender` (5750), which draws one flat grouped list
  - `qOpen` (5766), which clears the query on open
  - `qClose` (5773)
  - `qPick` (5779)
  - the `#tkqpop` dialog markup (`class: 'tk tk-palette'`, 5932)
  - the input key handlers (6262-6272); Esc closes at once
- Its CSS is the `.tk.tk-palette` rules in `assets/tracker.css` (81-90).
- The host feeds board rows and picks:
  - `paletteRows` (`assets/dashboard.js:4695`) and `palettePick` (4743).
  - The mount passes `searchRows: paletteRows, searchPick: palettePick`
    (4775-4776).
  - The `searchRows` row shape is documented at the top of tracker.js
    (149-152).
- Tests:
  - `search_palette_test.go`: `TestSearchPaletteAssets` (string hooks,
    line 9) and `TestSearchPaletteBehavior` (line 46).
  - The second runs the node harness
    `testdata/search-palette.test.cjs` when node is installed.

### Routes and client pieces this ticket reuses

- Name and content search (`wsdashboard/fs_search.go`):
  - `GET /s/<rid>/fs/find?q=[&root=]` searches names.
  - `GET /s/<rid>/fs/search?q=[&case=1][&regex=1][&root=]` searches
    content. It caps at 200 files, 2000 lines, and 10 s.
  - Both search the root's file list, which excludes git-ignored files.
- Listing and preview (`fs_routes.go`):
  - `GET fs/list?dir=[&root=]` lists one directory.
  - `GET fs/preview?path=[&root=]` returns a preview card.
  - All four GET routes resolve their root through `browseRoot`
    (fs_routes.go:181).
  - A `root` parameter is served only to `localViewer` for a request that is
    not `crossSiteFetch`, under `browseRootParam` (fs_routes.go:155).
  - Every refusal answers 404.
- `POST fs/act {action:"open", root}` (`fs_act_route.go`) is already a POST
  route. It is gated by `localViewer` and `sameOrigin` and checks `root`
  with `browseRootParam`. A bad root answers 400 (fs_act_route.go:204).
- `GET /tracker/stores/<id>/search?q=` (`tracker_routes.go:21-22`) answers
  `{"ids": [...]}` only: item IDs in tree order, with no excerpts. A
  malformed query answers 400. The client reaches it through the host
  adapter's `search(id, q, done)` (dashboard.js:4624).
- Client pieces:
  - The page marks a local viewer with `data-sysopen` on the root element.
    `files.js` gets `local()` from it (dashboard.js:661), and so does
    preview.js:41.
  - `window.WsPreview.open(path, {root, line})` (preview.js:229) opens the
    preview card at an optional line.
  - For a local viewer, the card shows an "open in the default app" button
    (preview.js:94, 111). The button posts `fs/act` with the card's root
    (preview.js:83).
  - The Files pane already calls `fs/find` and `fs/search` with an abort
    signal (files.js:520-521).
- `TestFSChangeRootIsLoopbackOnly` (fs_routes_test.go:330) already pins the
  `fs/list` root gate. It checks that these get 404:
  - a LAN token holder
  - a cross-site request
  - a `//server/share` or `\\server\share` root (fs_routes_test.go:385)

## Decisions

1. **Five result slots** (owner request 2026-11-13; (547)). The slots are
   board (보드), files (파일), content (내용), tracker (트래커), and tracker
   content (트래커 내용).
   - Board: topics, bodies, questions/answers, and numbered items. It
     replaces today's topics, bodies, questions, and answers groups.
   - Files: name matches over the scope.
   - Content: literal content matches over the scope.
   - Tracker: item titles and aliases.
   - Tracker content: excerpts from item bodies and comments.

   The slot labels are UI strings: they go through i18n keys.
   - Supersedes: `261112-feat-ws-dashboard-search-palette`'s decision of
     2026-11-12, "Result groups in a fixed order: session labels, topic
     names, topic bodies, items, answers, tracker". The owner asked for this
     slot model on 2026-11-13 ((547)); Decision 2 moves the session rows out
     of the result list.
2. **Jump/command strip** ((548)=a).
   - Sessions, stores, and actions leave the result list. They become a
     "이동·명령" chip strip right under the input, at every width.
   - The strip filters at once as you type.
   - ↑ from the first row or Alt+0 enters it. ←/→ moves between chips, and
     ↓ returns to the results.
   - Rejected: keeping these rows as groups in the result list, as the
     palette does today.
3. **Item IDs** (owner request; (547)).
   - The `.123` syntax is removed.
   - A bare number fuzzy-matches board item IDs, prefix matches first.
   - `(531)` is the exact form: it yields one hit, preselected, so Enter
     opens it.
   - Rejected: keeping `.123` alongside `(n)`.
4. **Scope** ((543) facts; (549)=b).
   - Files and content search the viewed session's project folder, honouring
     `.gitignore`, through the existing Go routes.
   - The scope chip beside the input ("📁 devenv/ · .gitignore") is a label,
     not a control.
   - Rejected:
     - (549)=a, a widen-to-all-workspaces action. This drops the mockup's
       clickable chip (its recommendation a).
     - Shelling out to ripgrep.
5. **Absolute-path mode** ((549)=b, the owner's exception quoted in
   Background). A query that is an absolute path switches the files slot to
   path mode, like the Windows Run dialog.
   - Trigger:
     - The trimmed query passes the same test as `browseRootParam`: an
       absolute drive or root path. Network paths and device names are
       refused.
     - The client-side check mirrors that test. The server's refusal (404)
       is the authority.
     - Path mode is offered only to a local viewer (`data-sysopen`), the
       same gate the explorer's change-root uses. A LAN viewer gets an
       ordinary search over the literal text.
   - Depth 1 only:
     - A query that ends in a separator lists that directory's entries.
     - Any other query lists the parent directory's entries, filtered by the
       last segment (prefix matches first, then fuzzy).
     - Both use `GET fs/list?root=<dir>` with `dir` omitted (or `.`), and
       never recurse. `dir` equal to an absolute `root` answers 404
       (`containedDir` requires a non-empty relative path), so the listed
       directory is always the `root`; no server change is planned for the
       `dir == root` form.
   - While path mode is on:
     - The scope chip reads "📁 <dir> · 직접 입력".
     - The content slot shows an idle state ("경로 입력 중"), because content
       search does not reach outside the scope.
     - The board and tracker slots search as usual.
   - Keys in path mode:
     - Enter on a directory completes the query to that directory plus a
       separator, so the user keeps drilling down in place.
     - Enter on a file opens its preview card (`fs/preview` with
       `root=<parent dir>`). The card shows its existing "open in the
       default app" button, which runs `fs/act open` with that root. This is
       the owner's "미리보기에 파일 열기 까지".
     - Ctrl+Enter on a file runs `fs/act open` directly and keeps the
       palette open, like Run. This is a technical call in the spec, not an
       owner answer: it uses the same gate and the same action as the
       preview button, one step shorter.
   - Rejected:
     - Expanding `~/`. This is a technical call: the routes take absolute
       paths only, and expansion would need a new parameter.
     - Listing deeper than depth 1.
     - Running content search outside the project scope.
6. **Streaming** (owner request "stream with cancellation"; (547)).
   - Every keystroke aborts all in-flight queries (one AbortController per
     slot) and starts a new generation. A batch from an older generation is
     dropped.
   - Local sources (board, tracker titles, the strip) answer at once. Files,
     content, and tracker content wait for an 80 ms debounce.
   - The tracker title slot is local (the UX lead's tracker-slot answer, 2026-11-13): a fuzzy
     title and alias match over the loaded store views (`vss`), as today's
     `qResults` does, with no Items grammar and no error state. Rejected:
     routing it through the store search route like tracker content (it
     makes Phase 1 asynchronous; the grammar parser exists only in Go), and
     a hybrid of local rows replaced by the route answer.
   - Stale-while-revalidate: the previous results stay, dimmed, until the
     new generation's first batch replaces them, so nothing flickers.
   - The selection follows the same result while it remains, so a late
     batch never moves the cursor.
   - Each slot shows its state: 대기 / 검색 중(n) / 완료(n · time) / 한도
     ("200+") / 결과 없음. The cap state applies to the content slot only,
     from `fs/search`'s `more` flag (200-file cap, 2000-line cap, or 10 s
     timeout). `fs/find` answers at most 50 items with no truncation flag,
     so the files slot shows no cap note.
7. **Layouts** (owner request "narrow/medium/wide"; (547); width rule from
   the UX spec, 2026-11-13).
   - Palette width: min(viewport width − 24, 1240). This is the existing
     `min(680px, 100vw - 24px)` rule on `.tk.tk-palette` with the cap raised
     to 1240 px, and it keeps the merged `max-width: none` (see
     Constraints).
   - The layout step is chosen from that resulting palette width, not from
     the viewport: < 600 px narrow, 600-999 px medium, ≥ 1000 px wide.
   - 420, 820, and 1240 px are the headless check viewports, not caps. At a
     420 px viewport the palette is 396 px, matching the merged fix.
   - Rejected:
     - min(dashboard width − 48, a per-step maximum), as this decision
       first read. It changes the existing margin rule for no gain.
     - A separate 820 px cap for the medium step. One cap keeps the width
       continuous as the window resizes.
   - Narrow (< 600 px): one list with label tabs (전체 plus the five slots).
     전체 shows each slot's top 3.
   - Medium (600-999 px): two columns.
     - The left column holds name results: board, files, tracker. The right
       column holds text results: content, tracker content.
     - The focused section expands to 12 rows; the others show 3.
     - An empty section collapses to its header.
   - Wide (≥ 1000 px): three columns and five panes. Content takes the full
     height of the middle column. Left column: board above files. Right
     column: tracker above tracker content (the mockup's grid areas
     `"board content tracker" "files content tnote"`).
8. **Keys** ((547); Ctrl+Enter targets, Alt+digit matching, and reopen
   from the UX spec, 2026-11-13).
   - Focus stays in the input.
   - ↑/↓ moves within a slot.
   - Tab/Shift+Tab moves to the next or previous slot, skipping empty ones.
   - Alt+1-5 jumps to a slot, and Alt+0 to the strip.
     - Matched on `e.code` (`Digit0`-`Digit5`), because macOS Option
       changes `e.key`.
     - If Linux Chrome reserves Alt+digit, that is accepted: Tab/Shift+Tab
       reach every slot.
     - Rejected: a fallback key binding. Tab already covers the loss, and a
       second binding adds a key to learn and to guard against IME.
   - Enter opens the selection and closes the palette.
   - Ctrl+Enter opens the target under the palette and keeps the palette
     open, with focus still in the input:
     - Board item: scrolls to the item and flashes it behind the palette.
     - Tracker item, tracker content hit: opens the item window.
     - File, content hit: opens the preview card.
     - In path mode, Ctrl+Enter on a file stays `fs/act open` (Decision 5).
     - Strip: a session chip opens in a new browser tab. Store and action
       chips behave as Enter.
     - Rejected: one generic "open in the background" with no per-source
       target, as this decision first read. It left the board, preview, and
       strip cases undefined.
   - Esc clears a non-empty query, then closes the palette.
   - Ctrl+K, Ctrl+Space, and `/` all reopen the palette with the last query
     selected.
     - The last query lives for the page's lifetime only and is not stored.
     - `/` keeps today's guard: it does not open the palette from an
       editable field.
     - Rejected: persisting the last query (localStorage or the server).
       A stale query across reloads helps no one, and a stored query is new
       state to manage.
   - The reopen keys keep the release-gate target guard
     (`261113-bug-ws-release-gate-sweep-fixes`): Ctrl+K and Ctrl+Space do
     not open the palette when the event target is an `.xterm`, a
     textarea (the composer), or a contenteditable (see Constraints).
   - Reopen-with-last-query also holds when the iframe shell's `search`
     message opens the palette (Constraints, iframe shell).
9. **What a pick opens, per source** ((547); host hooks per Decision 13):
   - Files: the preview card.
   - Content: the preview at that line, highlighted.
   - Board: scroll to the item and flash it.
   - Tracker: the item window.
   - Tracker content: the item window at that comment.
   - The tracker content slot queries every registered store, not only
     loaded or viewed ones: one request per store, each with its own abort.
     The tracker title slot covers the loaded store views only (Decision 6). A hit
     from a store with no loaded view opens the item window through the
     same open path the board's item links use (the item window for that
     store id).
   - Excerpt answer: the parameter name, the excerpt answer fields, and the
     `TrackerHost.Search` signature are the implementer's choice (Phase 2
     Result records them; the Go test pins them). The answer must say which
     field matched (body or comment) and, for a comment match, carry the
     comment id, so "the item window at that comment" can open it. Excerpts
     stay T0 additive (Decision 10).
10. **Compatibility tier: T0.**
    - Verified against `agents-plugin-tool/internal/wsdashboard` at
      `f1e45292c`:
      - Path mode and the async slots use only existing routes: `fs/find`,
        `fs/search`, `fs/list`, and `fs/preview` (GET, dispatched in
        `handleFSRoutes`, fs_routes.go:421-432), the tracker store search
        (GET), and the existing POST `fs/act` (pages.go:1026).
      - All of them already accept `root` under the local-viewer gate
        described in Background.
    - What changes is frontend assets and i18n keys, plus one optional,
      additive query parameter on the existing tracker store search GET
      route (Decision 11). Omitting it gives today's `{"ids": [...]}`
      answer unchanged, so an older client is unaffected: T0.
    - No new route, and no new or changed non-GET route.
11. **Tracker content source** (UX spec, 2026-11-13).
    - Tracker content comes from the existing store search route,
      `GET /tracker/stores/<id>/search?q=`, which gains an optional,
      additive excerpt query parameter (T0, Decision 10).
    - With the parameter, the answer adds body and comment excerpts per
      matched item beside `ids`. Without it, the answer is unchanged.
    - Rejected: matching tracker content client-side over the loaded store
      views (`vss`) only. It misses every store that is not loaded.
12. **Query text per slot** (UX spec, 2026-11-13).
    - The tracker content slot sends the raw query text through the Items
      search grammar, so `status:` and the other keys work as in the Items
      box. The tracker title slot is a local fuzzy match without the grammar
      (the tracker-slot answer, Decision 6), so a filter query is served by the content
      slot.
    - A 400 from the store search route puts only the tracker content slot
      in an error state that shows the parser message. The other slots
      carry on.
    - The files and content slots use the literal query text.
    - Rejected:
      - Stripping or escaping grammar keys before the tracker search. It
        makes the palette disagree with the Items box.
      - Failing the whole palette on a parse error. One slot's grammar must
        not blank the other four.
13. **Host hooks for file logic** (UX spec, 2026-11-13).
    - tracker.js stays host-agnostic. File search, preview, and open arrive
      as new mount options passed from dashboard.js, like
      `O.searchRows`/`O.searchPick`.
    - tracker.js never reaches `window.WsPreview` or the `fs/*` routes
      directly; dashboard.js implements the hooks over them.
    - Rejected: calling `window.WsPreview` and `fs/*` from tracker.js. It
      ties the tracker renderer to one host page.
    - Hook names and signatures (file search, content search, preview, open,
      list, and the board keep-open pick for Ctrl+Enter, whether a keep-open
      flag on `O.searchPick` or a new hook) are the implementer's choice and
      are recorded in the phase Result. The host-agnostic rule above holds
      whatever the names.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for
  `agents-plugin-tool/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for
  `agents-plugin-tool/internal/`)
- The dialog keeps the `.tk.tk-palette` class, and the tracker keeps its
  ownership of `.tk*`. The cross-stylesheet collision guard
  (`css_collision_test.go`) stays green.
- The UA `dialog:modal` max-width fix (`max-width: none` on
  `.tk.tk-palette`) is merged on develop: `d2357c991`, via merge
  `65a917d1b`. At a 420 px viewport the palette is now 396 px. Keep the
  rule when raising the width cap (Decision 7); do not re-apply it.
- Browser verification uses a repo-local headless harness, following the
  convention in `261113-feat-ws-dashboard-board-reorder-hold` (its
  Phase 1 harness item, `scripts/board-headless.mjs`):
  - A committed `scripts/<name>-headless.mjs` (here
    `scripts/palette-headless.mjs`) beside `scripts/dev.py`, driving a dev
    daemon started by `scripts/dev.py rerun daemon`.
  - It resolves Playwright from the npx cache, with no `package.json` and
    no Playwright dependency. It is a developer command and is not shipped.
  - Each scenario asserts and exits non-zero on failure; LOOK captures are
    taken from the same script.
- No new non-GET route. Path mode reuses `fs/list`, `fs/preview`, and
  `fs/act` under their existing local-viewer, same-origin, and containment
  rules.
- A perf worker (`261113-feat-ws-whiteboard-board-frontend-performance`) is
  editing the board assets. Board-slot rows come from the existing
  `O.searchRows` snapshot, not from a new DOM scan.
- Keep what the palette already guarantees (`261112-feat-ws-dashboard-search-palette`):
  - IME composition (`isComposing` / keyCode 229) never triggers a shortcut
    or a selection.
  - The `/` focus guard stays.
  - Closing returns focus to the opener.
  - `prefers-reduced-motion` is honoured.
- Keep the release-gate Ctrl+K guard
  (`261113-bug-ws-release-gate-sweep-fixes`, ready; implemented as
  `701398372` on `impl/develop/bribe-amiss-icky`, not yet on develop):
  `qShortcut` skips Ctrl+K and Ctrl+Space when the event target is an
  `.xterm`, a textarea (the composer), or a contenteditable, pinned in
  `testdata/search-palette.test.cjs`. Landing order matters: whichever of
  the two tickets lands second rebases onto the other and keeps the guard
  and its test.
- Iframe shell (`261113-feat-ws-dashboard-iframe-shell`, completed on
  `impl/develop/perky-bring-tweet`, about to merge to develop):
  - The palette stays in the session frame. The shell document's keydown
    sends a `search` message to the shown frame for Ctrl+K, Ctrl+Space, and
    `/`; frames send `key {esc}` to the shell.
  - `paletteRows` reads the session rows `#rail .rs[data-rid]` from the
    parent document (`shParentDoc`), and session picks go through `go()`.
    The strip's session chips keep reading rail rows from the shell
    document.
  - Reopen-with-last-query also holds when the shell's `search` message
    opens the palette.
  - Esc-clears-then-closes is handled in the palette before any frame Esc
    forwarding to the shell.
- Board-slot body text reads `srcdoc || data-srcdoc`:
  `261113-feat-ws-whiteboard-board-frontend-performance` Decision 2 has the
  server emit `data-srcdoc` and set `srcdoc` only when a board is mounted.
- Edge states:
  - Path mode shows an inline error row in the files slot when `fs/list`
    fails (non-existent directory, 404, 400).
  - The strip with an empty query shows all chips in their default order.
- Visual reference: the approved mockup
  `ai-docs/tickets/assets/261114-feat-ws-dashboard-unified-search-palette/search-palette-mockup.html`.
  Where it differs from these Decisions (the scope chip), the Decisions win.

## Prior Decisions

- 261112-feat-ws-dashboard-search-palette (2026-11-12, Decisions): "Result groups in a fixed order (user, 2026-11-12): session labels, topic names, topic bodies, items (question titles), answers, tracker." — bearing: contradiction-candidate
- 261109-feat-ws-dashboard-composer-file-mention (2026-11-09, Decisions): "Absolute mode completes one directory at a time: fuzzy matching runs only inside the directory being typed; no recursive search under an absolute path (unbounded cost on something like D:/)." — bearing: supports
- 7f890500 (2026-11-10, commit): "Change root is a `root` query parameter gated by localViewer and crossSiteFetch under the files route's abs-mode path rules; no capability is minted there, so HTML and SVG answer as source text." — bearing: constrains
- 91bf22ca (2026-11-10, commit): "A query with no status token keeps hidden items ... The search route returns hidden matches; the panel applies its own hidden" filtering; the parser stays in wstracker behind TrackerHost.Search. — bearing: constrains
- 057a2ab0 (2026-11-08, commit): "The dashboard stays free of wstrackerhost/wstracker/wsledger: the interface speaks JSON-level types and the cmd/ws-mcp adapter converts, so the page package never grows a dependency on the ledger stack." — bearing: constrains
- d2357c99 (2026-11-14, commit): "`.tk.tk-palette`: added `max-width: none`; the UA `dialog:modal` max-width capped it, so at 420px it measured 386px against the intended min(680px, 100vw - 24px) = 396px." — bearing: supports
- 1ed33f98 (2026-11-14, commit): "worktree-delete (f1e45292c) added .st in dashboard.css, colliding with whiteboard .wbtk .st under the cross-stylesheet collision guard; renamed the newer class per the guard's prefix rule." — bearing: constrains
- 65a917d1 (2026-11-14, commit): "palette max-width:none and alias label nowrap" — UX ticketless fix merged on develop; the width rule Decision 7 keeps. — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | assets/tracker.js, assets/tracker.css, assets/dashboard.js, internal/wsdashboard/tracker_routes.go, cmd/ws-mcp/tracker_host.go, search_palette_test.go, testdata/search-palette.test.cjs, scripts/palette-headless.mjs, i18n tables |
| scope.surface | cross-module | the store search route's optional excerpt parameter, the wsdashboard TrackerHost interface and its cmd/ws-mcp implementation, and new tracker mount options (tracker_routes.go#L180-L184, cmd/ws-mcp/tracker_host.go#L206) |
| scope.new_public_symbol | yes | optional excerpt query parameter on GET /tracker/stores/<id>/search, new tracker mount-option hooks beside searchRows and searchPick; names unsettled |
| scope.new_type_contract | yes | excerpt-bearing store search answer beside ids, an excerpt-capable TrackerHost search signature, the file search and preview hook shapes; none specified in the ticket |
| scope.test_surface | new-files | existing: search_palette_test.go, testdata/search-palette.test.cjs, tracker_search_test.go, fs_routes_test.go, css_collision_test.go; new: scripts/palette-headless.mjs (not present yet) |
| complexity.reuse_points | confirmed | fs/find, fs/search, fs/list, fs/preview, fs/act, browseRootParam (fs_routes.go#L155), O.searchRows, WsPreview.open (preview.js#L229), the Items search grammar; all read |
| complexity.side_effect_risk | moderate | rewrites the shared palette block in tracker.js and the hooks in dashboard.js, both edited by in-flight ready tickets (261113-bug-ws-release-gate-sweep-fixes cites tracker.js qShortcut lines) |
| risk.correctness | high | streamed per-slot generations with abort, stale-while-revalidate, three layout steps, Alt+digit on e.code, IME guards and many key paths |
| risk.fit | moderate | tracker.js must stay host-agnostic (Decision 13) and keep css collision guard prefixes while sharing files with parallel dashboard tickets |
| risk.test | high | node harness runs only when node is installed (search_palette_test.go), the headless harness is new and pending 261113-feat-ws-dashboard-board-reorder-hold, layout and streaming are visual and timing driven |
| risk.security_or_contract | moderate | absolute-path mode lists directories and opens files on the host but reuses the existing localViewer, same-origin and root gates; the search route change is additive T0 |

## Phases

### Phase 1: Slot model, jump strip, IDs, and local sources

Implements Decisions 1-3, the local half of 6 (including the local tracker
title slot of the tracker-slot answer), 7, and 8.

- Replace the flat grouped list (`qResults`/`qRank`/`qGroup`/`qRender` in
  tracker.js:5705-5765) with the five-slot model.
  - Fill the board slot from `O.searchRows`.
  - Fill the tracker title slot from the loaded store views (`vss`), as
    `qResults` does today.
  - Leave the files, content, and tracker content slots in their idle state
    for now.
- Move sessions, stores, and actions into the strip.
  - Their pick behaviour stays as in today's `qPick` (tracker.js:5779) and
    `palettePick`.
- Remove the `.123` jump in `qRank` (tracker.js:5706). Add the bare-number
  rule and the exact `(n)` rule.
- Add the three layout steps (Decision 7: width min(viewport − 24, 1240),
  step chosen from the palette width), the key map (Decision 8, including
  Ctrl+Enter for the board slot, the tracker title slot, and the strip, and
  Alt+digit on `e.code`), the generation counter for local sources,
  Esc-clears-then-closes, and reopen-with-last-query from Ctrl+K,
  Ctrl+Space, and `/`. Today `qOpen` clears the query (tracker.js:5771),
  and Esc closes at once (tracker.js:6272).
- The tracker title slot is a local fuzzy title/alias match with no
  grammar and no error state (the tracker-slot answer, Decision 6).
- Board-slot body text reads `srcdoc || data-srcdoc` (Constraints).
- i18n: keys live in `internal/wsi18n/tables/*.json` (11 tables: en, ko,
  de, es, fr, it, ja, pt-BR, ru, zh-Hans, zh-Hant), and `keys_test.go`
  requires every English key to be used and present in every table.
  - Add keys for the five slot labels and the strip label to all 11
    tables.
  - Remove retired keys (for example `js.tk.qs.sessions`, if the session
    group label is retired) from all 11 tables.
  - Change `js.tk.qs.placeholder` ("Search, or .153 to jump to an item…",
    en.json:950) with the `.123` removal.
- Add `scripts/palette-headless.mjs` (Constraints: headless harness).

Verification:

- Node harness: extend `testdata/search-palette.test.cjs` (run by
  `TestSearchPaletteBehavior`). Cover:
  - slot assignment and ranking
  - the bare-number prefix-first rule
  - `(n)` as one preselected hit
  - `.123` no longer jumping
  - strip filtering
  - a stale generation's batch being dropped
  - Esc twice
  - IME composition
  - the layout step chosen from the palette width: 599 -> narrow, 600 and
    999 -> medium, 1000 -> wide
  - Alt+digit matched on `e.code` (`Digit0`-`Digit5`) when `e.key` is a
    macOS Option glyph
  - a query with a grammar key (`status:doing`) fills the tracker title
    slot by fuzzy title match only, with no error state
  - Ctrl+Enter: a board item calls the scroll-and-flash pick and the
    palette stays open with focus in the input; a tracker title row opens
    the item window and the palette stays open; a session chip opens a new
    tab; a store or action chip behaves as Enter
  - reopen through Ctrl+K, Ctrl+Space, and `/` restoring the last query
    selected; `/` from an editable field does not open the palette
  - the release-gate guard: the existing Ctrl+K/Ctrl+Space target-guard
    test (`.xterm`, textarea, contenteditable) stays green (if
    `261113-bug-ws-release-gate-sweep-fixes` has not landed yet, add the
    equivalent case)
  - the iframe shell: the shell's `search` message opens the palette with
    the last query selected; Esc clears a non-empty query before any frame
    Esc forwarding to the shell, and closes on the second Esc; strip
    session chips read `#rail .rs[data-rid]` rows from the shell document
    and a session pick goes through `go()`
  - board-slot body text read from `data-srcdoc` when `srcdoc` is empty,
    and from `srcdoc` when it is set
  - the strip with an empty query listing all chips in their default order
- i18n: `keys_test.go` passes with the new keys in all 11 tables and the
  retired keys removed.
- Update `TestSearchPaletteAssets` for the changed hooks.
- `go test ./internal/wsdashboard/` passes, including
  `css_collision_test.go`.
- `scripts/palette-headless.mjs` at 420, 820, and 1240 px viewports:
  - Assert the palette width is 396, 796, and 1240 px and the layout step
    is narrow, medium, and wide.
  - Key paths: Tab/Shift+Tab across slots, Alt+0-5, Esc twice, reopen with
    the last query, Ctrl+Enter on a board item flashing it behind the open
    palette.
  - In the framed page (iframe shell), Ctrl+K from the shell document
    opens the palette in the session frame, and Esc behaves as above.
  - `/` with the Files pane shown: `files.js` also binds `/` on `document`
    (bubble phase, files.js:798-804) for the Files pane search, and the
    palette's capture handler (tracker.js:6262-6264) wins through
    `stopImmediatePropagation`. Keep that: `/` opens the palette and the
    Files pane search does not take focus; assert it.
  - LOOK captures, light and dark. Show the strip, a filled board slot, a
    filled tracker slot, and the idle async slots.
  - Compare each capture side by side with the mockup's matching width
    state.
  - Record any deviation, and say whether a Decision justifies it.

### Result (ee2d95201) - 2026-11-14

Implemented in `ee2d95201`. The headless harness and its two fixes are in
`63c1d70fd`; the review and lead-note fixes are in `a593864d6`.

- Slot model: `QSLOTS` = board, files, content, tracker, tnote. `qBoard`,
  `qTracker`, and `qStrip` fill the local slots and the strip. `qView`,
  `qIdx`, and `qs.sel` keep the selection on a row's key across re-ranks
  and late batches. `qRank` and the grouped list are removed.
- IDs: `(n)` returns exactly item n, one hit, preselected (the board slot,
  or "all" at narrow). A bare number matches board item ids only, prefix
  first, with no text match. `.123` is gone.
- Ordering: the board slot ranks by score, then group. An empty query sorts
  topics, then items, newest first. With an empty query the tracker slot
  shows items in progress (`doing`). The strip lists sessions, stores, and
  actions in that order and is filtered by the query. It is hidden when
  nothing matches.
- Steps: `qStepOf` gives narrow <600, medium <1000, wide otherwise, from the
  palette's own width, and recomputes on resize. Keys: Tab and Shift+Tab
  skip empty slots. Alt+0-5 match on `e.code` and are ignored with Ctrl
  held (AltGr). Up from the first row or Alt+0 enters the strip, Left/Right
  move along it, Down leaves it. Enter opens and closes. Ctrl/Cmd+Enter
  opens behind the open palette (board: `searchPick(row, {keep: true})`,
  which skips the focus move; a session chip opens a new tab). Esc clears
  the query, then closes.
- Reopen restores the last query, selected. Each opening starts on the
  step's first view.
- The board-slot body text reads a frame as either `iframe.doc-frame` or
  the unmounted `div.doc-frame` placeholder (`data-srcdoc`, then `srcdoc`).
  Lead call; see Deviations.
- i18n: added `js.tk.qs.all`, `slot.board|files|content|tnote`, `strip`,
  `st.idle|run|done|cap|none|error|path`, `scope`, `scopepath`, `more`,
  `listfail`, `exact`, `comment`, and `body` to all 11 tables. Removed
  `topics`, `bodies`, `questions`, `answers`, and `openpage`. Rewrote
  `placeholder` (no `.153`) and `hint`. Regenerated the tracker mockup's
  `i18n-en.js`.
- Verification:
  - `go test ./internal/wsdashboard/ ./internal/wsi18n/ ./internal/wstracker/`
    passes, including `css_collision_test.go`, `keys_test.go`, and
    `TestSearchPaletteAssets`.
  - The node harness `testdata/search-palette.test.cjs` has been rewritten
    for the slot model.
  - `scripts/palette-headless.mjs` layout and keys pass: 396, 796, 1216,
    and 1240 px at 420, 820, 1240, and 1400 px viewports; Tab, Alt, Esc
    twice, reopen, the Ctrl+Enter flash, Ctrl+K from the shell, and `/`
    with the Files pane shown.
- Deviations:
  - The width is 1216 px, not 1240 px, at a 1240 px viewport. Decision 7's
    formula gives 1216 px there. The 1240 px cap is checked at 1400 px.
  - The mockup's fuzzy/literal mode chip is not built. Decision 12 makes
    every source take the literal text, so the chip would switch nothing.
  - The scope chip is prefixed with a folder glyph, as in the mockup.
  - Placeholder `div.doc-frame` tolerance: a lead note asked for it, because
    the frontend-performance ticket's revised Decision 2 makes board frames
    inert placeholders. Both DOM states have node fixtures. The perf
    worker's own change to this read is reconciled by whichever branch
    lands second.
- LOOK: captures at 420/820/1240, light and dark, compared with the mockup
  at narrow, medium, and wide. Layout, panes, the header state line, and
  highlights match. At narrow, the English tab labels overflow the tab row,
  which scrolls ("Tracker content" is cut off).

### Phase 2: Async sources and streaming

Implements Decision 4, the async half of 6, 9, and 11-13. Depends on
Phase 1.

- Files come from `fs/find` and content from `fs/search`, both with the
  literal query text (Decision 12), reached through new mount-option hooks
  that dashboard.js passes to the tracker mount beside
  `searchRows`/`searchPick` (Decision 13).
- Tracker content comes from the store search route with the new optional
  excerpt parameter (Decision 11), through the adapter's `search`, with the
  raw query text through the Items search grammar. A 400 puts only that
  slot in its error state with the parser message (Decision 12).
  - Go side: add the optional excerpt parameter to the store search handler
    (`tracker_routes.go`). Without it, the answer stays `{"ids": [...]}`
    byte-for-byte. The handler cannot produce excerpts alone: it reads
    through `TrackerHost.Search(ctx, id, query) ([]string, error)`, which
    returns ids only (`agents-plugin-tool/internal/wsdashboard/tracker_routes.go#L180-L184`),
    implemented by `dashboardTracker.Search` in
    `agents-plugin-tool/cmd/ws-mcp/tracker_host.go#L206-L220`, and
    wsdashboard knows the tracker only through JSON-level types. The
    excerpt path therefore also changes that interface and its adapter.
    The client adapter's `search(id, q, done)` likewise passes on `ids`
    only (`agents-plugin-tool/internal/wsdashboard/assets/dashboard.js#L4624-L4630`).
  - The excerpt parameter name, the excerpt answer fields, and the
    `TrackerHost.Search` signature are chosen by the implementer, recorded
    in this phase's Result, and pinned in the Go test. The answer carries
    which field matched (body or comment) and, for a comment match, the
    comment id (Decision 9).
- The tracker content slot queries every registered store, one request per
  store, each abortable (Decision 9).
  - `tkFetch` (dashboard.js:4449) and the adapter `search(id, q, done)`
    (dashboard.js:4624-4629) take no AbortSignal today. The adapter gains an
    optional `signal` argument that it passes to `fetch`: additive,
    client-only, T0; old callers are unaffected.
  - The store search route returns hidden items (query.go:245, 268-273).
    The tracker content slot drops hidden ids on the client, as `itemFilter` does
    (tracker.js:2285-2296), since the palette skips hidden items today
    (tracker.js:5736).
  - A hit from a store with no loaded view opens the item window through
    the same open path the board's item links use.
- Each slot gets its own AbortController, the 80 ms debounce, and dimming
  while stale (stale-while-revalidate).
- Add the per-slot state line, including the 한도 ("200+") cap state (the
  content slot only, from `fs/search`'s `more`; the files slot shows no
  cap note, since `fs/find` has a fixed 50-item cap and no flag), the
  error state, and the scope label chip.
- Picks, through the Decision 13 hooks (dashboard.js implements them over
  `window.WsPreview`; tracker.js does not reach it). Hook names and
  signatures are the implementer's choice, including the board keep-open
  pick for Ctrl+Enter (a keep-open flag on `O.searchPick` or a new hook);
  record them in the Result:
  - Files open the preview card.
  - Content opens the preview at that line, highlighted.
  - Tracker content opens the item window at the comment.
  - Ctrl+Enter opens the same target under the palette and keeps it open
    with focus in the input (Decision 8).

- New strings of this phase (per-slot state lines, the tracker content
  error state, the cap note) go through i18n keys in all 11 wsi18n tables,
  as Phase 1 does.
- Tracker content rows: the implementer picks a row cap (for example a
  fixed number of excerpts per store, merged in store order) and records it
  in the Result; the slot shows the cap the same way the content slot shows
  `more`.

Verification:

- Go/HTTP: a store search test with the excerpt parameter returns excerpts
  for body and comment matches; without it the answer is unchanged
  (`{"ids": [...]}` only); a malformed query still answers 400 with the
  parser message. The test pins the chosen parameter name, answer fields
  (matched field, and the comment id for a comment match), and the
  `TrackerHost.Search` signature.
- Node harness cases with stubbed fetches:
  - Typing fast leaves no older-generation rows.
  - Every keystroke aborts each slot's in-flight request, including one
    request per registered store for the tracker content slot (the adapter's
    `signal` reaches `fetch`).
  - The selection holds when a late batch arrives.
  - "200+" shows on the content slot when `fs/search` answers `more`; the
    files slot shows no cap note.
  - Hidden item ids from the store search are dropped by the tracker
    content slot.
  - A hit from a store with no loaded view opens the item window for that
    store id through the board's item-link open path.
  - Each slot state appears in order.
  - A 400 from the tracker content request leaves that slot in its error
    state with the parser message; files and content still complete.
  - Files and content requests carry the literal query text; the tracker
    content request carries the raw text and the excerpt parameter.
  - File and content picks, and their Ctrl+Enter, call the mount-option
    hooks; tracker.js references no `WsPreview` (also pinned as a string
    check in `TestSearchPaletteAssets`).
- `scripts/palette-headless.mjs` against a live session page at 420, 820,
  and 1240 px:
  - Ctrl+Enter on a file or content hit opens the preview card under the
    palette, and the input keeps focus.
  - A tracker content hit from a store that is not loaded appears.
  - LOOK captures showing streaming in progress (dimmed), a completed
    state, the cap state, and a tracker content slot error state.
  - Compare each capture side by side with the mockup.

### Result (ee2d95201) - 2026-11-14

The Go leaf is `32e2ac2cb`. The client is in `ee2d95201`.

- Excerpt parameter: `excerpts=1` on `GET /tracker/stores/<id>/search`
  answers `{"hits": [...], "ids": [...]}`. Without the parameter the answer
  stays `{"ids": [...]}` byte for byte. A grammar error is a 400 carrying
  the parser message. Compatibility tier: T0, additive.
- Hit fields: `TrackerSearchHit{id, title, alias, kind, status,
  hidden(omitempty), field ("body"|"comment"|"item"), comment(omitempty),
  text(omitempty)}`.
- The search signature `TrackerHost.Search` is unchanged. A new method,
  `TrackerHost.SearchHits(ctx, id, query string) ([]TrackerSearchHit,
  error)`, is implemented by `dashboardTracker.SearchHits` over
  `wstracker.(*View).SearchHits(q Query) []SearchHit` (`query_hits.go`).
  It returns at most 3 comment hits per item. Excerpts are about 160 runes,
  starting about 40 runes before the match.
- Client: `tkFetch(path, body, done, keepalive, signal)`. The adapter
  `search(id, q, done, o)` takes `o.excerpts` and `o.signal` and answers
  `{ok, ids, hits}` or `{ok: false, error, status}`. The Items panel call is
  unchanged.
- Mount-option hooks (Decision 13), from dashboard.js `paletteFiles`, absent
  on a page without a session:
  - `fileFind(q, signal)`, `fileSearch(q, signal)`, and
    `fileList(dir, signal)` return promises. A refusal rejects with
    `{status, text}`.
  - `filePreview(path, opt)` calls `WsPreview.open`.
  - `fileOpen(name, dir)` posts `fs/act open`.
  - `fileLocal()` returns `data-sysopen`.
  - `fileScope()` returns the head's `.meta` text.
  - tracker.js references no `WsPreview` and no `/fs/` route. Both are
    pinned in `TestSearchPaletteAssets`.
- Streaming:
  - A query starts a generation.
  - Each keystroke aborts every request in flight: one AbortController per
    file request and per store.
  - The async slots wait 80 ms.
  - Previous rows stay dimmed (`tk-qstale`) until the generation's first
    batch.
  - A batch from an older generation is dropped.
  - The slot state runs idle, searching (n), then done (n · s), limit (n+),
    no results, or error.
- Tracker content:
  - Every registered store gets one request, loaded or not.
  - At most 8 hits per store are shown, with "N+" when capped.
  - Hidden hits, by the hit's flag or the loaded view, are dropped unless
    the query contains `status:hidden`.
  - A 400 from any store puts only this slot in its error state with the
    parser message.
  - A hit in a loaded store opens the item window at the comment. A hit in
    a store with no loaded view takes the `#/<store>/<item>` route.
- Files: directory items from `fs/find` are dropped. Ctrl+Enter on a file or
  content hit previews it under the open palette, and the input keeps focus.
- Verification:
  - The node harness covers abort per keystroke and per store, the stale
    drop, selection held across a late batch, the 210-hit cap, hidden hits,
    the 400 error, the unloaded-store route, literal-query hook calls, and
    the adapter's `excerpts=1`, signal, and status.
  - `palette-headless` async passes: file and content previews with the
    palette open, a hit from a store whose view did not load (simulated by
    aborting its view route), and the error state.
- LOOK: streaming (dimmed), done, cap, and error at 820 and 1240, compared
  with the mockup. They match apart from the mockup's per-row "+N more" tab
  hint wording.

### Phase 3: Absolute-path mode

Implements Decision 5. Depends on Phase 2.

- Add the client-side absolute-path test (mirroring `browseRootParam`),
  gated on `data-sysopen`.
- Add depth-1 listing through `fs/list?root=<dir>` with `dir` omitted
  (`dir` equal to an absolute `root` answers 404; Decision 5), with
  last-segment filtering. A query that does not end in a separator lists
  its parent directory the same way, `root=<parent>`.
- Add the path-mode scope label and the idle content slot.
- A failed `fs/list` (non-existent directory, 404, 400) shows an inline
  error row in the files slot.
- The listing, the preview, and the open action go through the Decision 13
  mount-option hooks; dashboard.js implements them over `fs/list`,
  `WsPreview.open`, and `fs/act`.
- Enter on a directory completes the query. Enter on a file opens the
  preview through the hook (`WsPreview.open(name, {root: parent})` on the
  host side), whose card already carries the open button. Ctrl+Enter on a
  file posts `fs/act open` with that root and keeps the palette open
  (Decisions 5 and 8).

- New strings of this phase (the path-mode scope label, "경로 입력 중", the
  `fs/list` error row) go through i18n keys in all 11 wsi18n tables.
- The path-mode `fs/list` call uses the same 80 ms debounce as the other
  async slots, and is aborted by the next keystroke like them.

Verification:

- Go/HTTP: keep `TestFSChangeRootIsLoopbackOnly` green. It already refuses
  an outside `root` for a LAN viewer, for a cross-site fetch, and for a
  network path. Add a case pinning the exact request shape path mode
  sends: `root=<abs dir>` with `dir` omitted answers 200 with that
  directory's entries (and `dir` equal to an absolute `root` is not the
  shape used; no server change is planned for it).
- Node harness: cases for the absolute-path test.
  - Accepts: POSIX roots and Windows drive paths.
  - Refuses: `//server/share`, `\\server\share`, device names, relative
    paths, and `~/`.
  - Covers: the separator vs last-segment split, and the local-viewer gate.
  - A failed `fs/list` stub (404 and 400) shows the inline error row in
    the files slot.
  - Ctrl+Enter on a file in path mode calls the open-action hook, not the
    preview hook, and the palette stays open.
- `scripts/palette-headless.mjs` as a local viewer:
  - `/tmp/` lists depth 1.
  - `/tmp/fo` filters.
  - Enter on a folder completes the query.
  - Enter on a file opens the preview with its open button.
- `scripts/palette-headless.mjs` LOOK captures of path mode at 820 and
  1240 px. Compare each capture side by side with the mockup.
- LAN viewer: a node-harness case with `data-sysopen` absent. An absolute
  path query gets the ordinary literal-text search and no path mode
  appears.

### Result (ee2d95201) - 2026-11-14

- `qAbsPath` mirrors `browseRootParam`.
  - Accepted: an absolute POSIX path or a drive path.
  - Refused: a network path (`//host`, `\\host`), a device namespace
    (`\\?\`, `\\.\`), a Windows device name segment, `~/`, a relative
    path, and NUL.
- `qPathOf` gates on `fileLocal()`. The query splits at its last separator:
  a trailing separator lists that directory, otherwise the parent is listed
  and filtered by the last segment, prefix first, then fuzzy.
- Listing goes through `fs/list?root=<dir>` with `dir` omitted.
  - Entries marked `gone` or `link` are skipped.
  - A refusal shows an inline "Cannot list this folder" error row.
  - Content shows "Typing a path", and the scope chip names the typed
    directory.
- Enter on a folder completes the query. Enter on a file closes the palette
  and calls `filePreview(name, {root: dir})`. Ctrl+Enter on a file calls
  `fileOpen(name, dir)` (`fs/act open`) and keeps the palette open.
- Switching the query into or out of path mode drops the files slot's rows,
  so neither row shape is drawn as the other. This was the review's
  Critical finding, fixed in `a593864d6`.
- Verification:
  - The node harness covers the accept and refuse cases, the separator
    split, the local gate, a LAN viewer (ordinary literal search), the 404
    error row, Ctrl+Enter open, and the path/ordinary switch.
  - `palette-headless` path passes on a temp listing directory standing in
    for `/tmp/`: `<dir>/` lists depth 1, `<dir>/fo` filters, Enter on a
    folder completes, and Enter on a file previews it with the open button.
- LOOK: path mode at 820 and 1240, compared with the mockup. They match.
- Open (minor, not fixed):
  - The `fs/act open` result toast may sit behind the modal palette.
  - A truncated `fs/list` (`more`) is not marked.

## Sage Review Round 1 (2026-11-14)

### Design Reviewer — block

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | Tracker title slot is both local instant and server grammar with parser error | important | missing |
| 2 | Key-map rewrite does not preserve the release-gate Ctrl+K target guard | important | autonomous |
| 3 | Iframe-shell frame context not accounted for | important | autonomous |
| 4 | Board-slot body text vs lazy srcdoc in the performance ticket | minor | autonomous |
| 5 | Excerpt parameter and TrackerHost signature unnamed | minor | autonomous |

### Completeness Reviewer — block

| # | Title | Severity |
|---|-------|----------|
| 1 | Tracker title slot source is contradictory (local instant vs grammar route) | important |
| 2 | Which stores the tracker slots query, and how a hit from an unloaded store opens, are unstated | minor |
| 3 | Excerpt parameter name and answer shape are left open | minor |
| 4 | Host hook shapes and the board Ctrl+Enter hook are not named | minor |
| 5 | Unspecified edge states and the LAN-viewer check method | minor |
