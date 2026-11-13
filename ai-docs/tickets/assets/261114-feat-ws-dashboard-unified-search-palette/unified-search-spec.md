# unified-search: one Ctrl+K palette over board, files, content and tracker (spec, UX session)

Source: owner request relayed by MAIN (2026-11-13; topic `unified-search`):
split the palette by source, lay it out for narrow/medium/wide widths, search
files and content asynchronously, drop the `.123` syntax with `(n)` as the
exact form, cover the tracker and tracker content, and stream with
cancellation. Mockup v2: `shared/docs/search-palette-mockup.html`. It
pre-applied (549)=a; this spec supersedes the mockup where they differ (only
the scope chip, see decision 4).

Owner answers: (543) ok (scope facts below); (547) ok (the mockup); (548)=a
(where today's rows go); (549)=b with an absolute-path exception (verbatim:
"549=b^아, 다만, "절대경로를 직접 입력"한 경우에 한해, 해당 절대 경로 트리
depth 1만큼은 검색에 넣고, 절대경로를 직접 입력하는 경우 미리보기에 파일 열기
까지는 두는 게 좋겠습니다. (윈도우 ctrl+r로 파일 열듯이)"). No owner question
remains open. Code read against develop f1e45292c.

Suggested stem: `261114-feat-ws-dashboard-unified-search-palette`.
Compatibility tier: T0. It adds frontend assets, i18n keys, and at most
optional query parameters on existing GET routes.

## Background

- Today's palette lives in `wsdashboard/assets/tracker.js` (about lines
  5689-5800): `qScore`/`qRank`/`qResults`/`qRender`, the `#tkqpop` dialog
  (`.tk.tk-palette`), opened by Ctrl+K, Ctrl+Space or `/` (`qShortcut`).
  - It renders one flat list in groups: sessions, topics, bodies, questions,
    answers, tracker (stores, actions, items), via `O.searchRows` plus
    `qResults`.
  - `.123` is a jump syntax in `qRank`.
- The daemon already has Go-native, `.gitignore`-honouring file search
  (`wsdashboard/fs_search.go`):
  - `GET /s/<rid>/fs/find?q=` searches names.
  - `GET /s/<rid>/fs/search?q=[&case][&regex]` searches content (literal by
    default; RE2; capped at 200 files, 2000 lines, 10 s).
  - Both cover the session's project root, or `root=<abs>` for a local
    viewer (`browseRootParam`).
- Other existing routes this spec reuses:
  - `GET fs/list?dir=&root=` lists one directory.
  - `GET fs/preview` is the preview card.
  - `POST fs/act {action:"open"}` opens a file in the host's default app.
    It is local-viewer and same-origin only.
  - `GET tracker/stores/<id>/search?q=` is tracker search in the Items
    search grammar (`tracker_routes.go`).

## Decisions

1. **Slots (sources).** There are five result slots: 보드, 파일, 내용,
   트래커, 트래커 내용.
   - 보드: topics, bodies, questions/answers and numbered items; it replaces
     today's topics/bodies/questions/answers groups.
   - 파일: name match over the scope.
   - 내용: literal content match over the scope.
   - 트래커: item titles and aliases.
   - 트래커 내용: body and comment excerpts.
2. **Jump/command strip ((548)=a).** Sessions, stores and actions leave the
   result list and become a "이동·명령" chip strip right under the input, at
   every width. It filters instantly as you type. You enter it with ↑ from
   the first row or with Alt+0; ←/→ moves between chips and ↓ returns.
3. **IDs.** The `.123` syntax is removed.
   - A bare number fuzzy-matches board item IDs, prefix first.
   - `(531)` is the exact form: one hit, preselected, so Enter opens it.
4. **Scope ((543) facts, (549)=b).**
   - Files and content search the viewed session's project folder,
     honouring `.gitignore`, through the existing Go routes (no ripgrep).
   - The scope chip beside the input ("📁 devenv/ · .gitignore") is a
     label, not a control: there is no widen-to-all-workspaces action. The
     mockup's clickable chip (rec a) is dropped.
5. **Absolute-path exception ((549)=b^).** A query that is an absolute path
   switches the 파일 slot to path mode, like a Run dialog.
   - Trigger: the trimmed query passes the same test as `browseRootParam`
     (an absolute drive or root path; network paths and device names are
     refused).
     - The client-side check mirrors that test; the server's refusal (404)
       is the authority.
     - `~/` is not expanded in this ticket. That is a technical call: the
       routes take absolute paths only, and expansion would need a new
       parameter.
     - Path mode is offered only to a local viewer (the page already knows
       this; it gates "change root" the same way). A LAN viewer gets
       ordinary search over the literal text.
   - Depth 1 only.
     - A query ending in a separator lists that directory's entries.
     - Otherwise the query lists the parent directory's entries, filtered
       by the last segment (prefix first, then fuzzy).
     - Both use `GET fs/list?dir=<dir>&root=<dir>` and never recurse.
     - Content search does not run outside the project scope.
   - The scope chip reads "📁 <dir> · 직접 입력" while path mode is on.
   - The 내용 slot shows an idle state ("경로 입력 중") in path mode, because
     content search does not reach outside the scope. Board and tracker
     slots search as usual.
   - Enter on a directory completes the query to that directory plus a
     separator, so you can keep drilling down in place.
   - Enter on a file opens its preview card (`fs/preview` with
     `root=<parent dir>`). The card shows the "open in the default app"
     button that preview cards already have (`fs/act open` with that root).
     This is the owner's "미리보기에 파일 열기 까지".
   - Ctrl+Enter on a file runs `fs/act open` directly and keeps the palette
     open, like Run. This is a technical call: it is the same gate and the
     same action as the preview button, one step shorter.
6. **Streaming.**
   - Every keystroke cancels all in-flight queries (AbortController per
     slot) and starts a new generation. A batch from an older generation is
     dropped.
   - Local sources (board, tracker titles, the strip) answer at once.
     Files, content and tracker content wait for an 80 ms debounce.
   - The tracker title slot ((570)=a) is local: an instant fuzzy match of
     titles and aliases over the loaded store state, like today's
     `qResults`, with no Items grammar and no error state. The grammar
     applies to the tracker content slot only.
   - Stale-while-revalidate: the previous results stay, dimmed, until the
     first batch of the new generation replaces them, so nothing flickers.
   - The selection follows the same result while it remains, so a late
     batch never moves the cursor.
   - Each slot shows its state: 대기 / 검색 중(n) / 완료(n · time) / 한도
     ("200+") / 결과 없음.
7. **Layouts.**
   - The palette width is min(viewport width − 24, 1240), the existing
     `min(680px, 100vw - 24px)` rule with a larger cap. The layout step is
     chosen from that resulting width.
   - Narrow (< 600 px): one list with label tabs (전체 + the five slots).
     전체 shows each slot's top 3.
   - Medium (600-1000 px): two columns: name results (보드, 파일, 트래커) |
     text results (내용, 트래커 내용). The focused section expands (12
     rows); the others show 3. An empty section collapses to its header.
   - Wide (≥ 1000 px): three columns and five panes, with 내용 taking the
     full height in the middle.
8. **Keys.**
   - Focus stays in the input.
   - ↑/↓ moves within a slot. Tab/Shift+Tab moves to the next or previous
     slot, skipping empty ones. Alt+1-5 jumps to a slot, and Alt+0 to the
     strip.
   - Enter opens and closes the palette. Ctrl+Enter opens the target under
     the palette and keeps it open, with focus still in the input:
     - Board item: scroll and flash behind the palette.
     - Tracker item, tracker content: the item window, under the palette.
     - File, content hit: the preview card, under the palette. In path
       mode, Ctrl+Enter on a file is `fs/act open` instead (decision 5).
     - Strip: a session chip opens in a new browser tab. Store and action
       chips behave as Enter.
   - Alt+digit matches on `e.code` (`Digit0`-`Digit5`), because macOS
     Option changes `e.key`.
   - Esc clears a non-empty query, then closes. Ctrl+K, Ctrl+Space and `/`
     reopen with the last query selected. The last query lives for the
     page's lifetime and is not stored.
9. **Opening, per source.**
   - 파일: the preview card.
   - 내용: the preview at that line, highlighted.
   - 보드: scroll to the item and flash it.
   - 트래커: the item window.
   - 트래커 내용: the item window at that comment.

## Constraints

- The palette dialog keeps the `.tk.tk-palette` class and the tracker's
  ownership of `.tk*`. The cross-stylesheet collision guard
  (`css_collision_test.go`) must stay green.
- At 420 px the palette is 386 px instead of 396 px, because the UA's
  `dialog:modal` max-width caps it. The fix branch
  `fix/ux-postswap-css-regressions` adds `max-width: none` (commit
  d2357c991); build on it once it is merged.
- Tracker content needs excerpts, and the store search route returns IDs
  only. It gains an optional, additive excerpt query parameter (T0) so
  that stores that are not loaded are covered too.
- The tracker content slot sends the raw query through the Items search
  grammar (the Go parser behind `tracker/stores/<id>/search`, fanned out to
  every registered store), so `status:` and the other keys work as in the
  Items box. A 400 puts that slot in an error state that shows the parser
  message; the other slots carry on. The title slot is local and has no
  grammar ((570)=a, decision 6).
- tracker.js stays host-agnostic: preview and open hooks arrive as mount
  options from dashboard.js (like `O.searchRows`/`O.searchPick`), never
  through `window.WsPreview`.
- No new non-GET route. Path mode reuses `fs/list`, `fs/preview` and
  `fs/act` under their existing local-viewer, same-origin and containment
  rules.
- A perf worker (`261113-feat-ws-whiteboard-board-frontend-performance`) is
  editing the board assets. Board-slot rows must come from the existing
  `O.searchRows` snapshot, not from a new DOM scan.

## Phases

### Phase 1: slot model, strip, IDs, local sources
Decisions 1-3, 6 (local half), 7, 8. Board and tracker title slots plus the
strip; `(n)` exact; `.123` removed.
- Verify with node tests for the ranking, the `(n)`/bare-number rules and
  the generation drop.
- Verify with Playwright at 420/820/1240: the layout steps, the key paths,
  and Esc twice.

### Phase 2: async sources and streaming
Decisions 4, 6 (async half), 9. Files (`fs/find`), content (`fs/search`) and
tracker content (store search) with per-slot abort, debounce, SWR dimming,
the cap state and the scope label.
- Verify with Playwright: typing fast leaves no older-generation rows; the
  selection holds across a late batch; "200+" appears at the cap.

### Phase 3: absolute-path mode
Decision 5.
- Verify with Go/HTTP tests that `fs/list` with an outside `root` is
  refused for a non-local viewer, and for a network path.
- Verify with Playwright as a local viewer: `/tmp/` lists depth 1;
  `/tmp/fo` filters; Enter on a folder completes the query; Enter on a file
  opens the preview with its open button. A LAN viewer sees no path mode.
