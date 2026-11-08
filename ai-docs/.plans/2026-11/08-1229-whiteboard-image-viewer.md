# Plan: in-page image and diagram viewer (lightbox with zoom and pan)

Branch `delegate/whiteboard-image-viewer` (worktree
`/Users/kang-sw/.ws-worktrees/devenv/declared-satiable-proximity`). Commit on
this branch; do not merge or push.

Read `ai-docs/manuals/shipped-surface-boundary.md` before editing (everything
here ships under `agents-plugin-tool/`). UI strings go through the i18n tables
(`internal/wsi18n/tables/*.json`, coverage checked by
`internal/wsdashboard/i18n_test.go`); the standalone whiteboard file follows
whatever string mechanism its page already uses.

## Request (user, 2026-11-08)

"머메이드에서도 느낀 건데 뭔가 확대하고 축소하고 패닝하고 이런 게 있던지,
아니면 사진을 클릭했을 때 새 창이 아니라 뭔가 모달 오버레이처럼 뜨는 그런
뷰어 위젯이 하나 뜨면 참 편할 거 같아요." Wireframes and screenshots on the
board are too small to read inline, and clicking opens a new tab.

## Decisions (lead, technical)

1. One small viewer module, no external library (the pages work offline and
   the standalone whiteboard file embeds its assets). Opening: a click on an
   image (`<img>`) or a rendered mermaid diagram in whiteboard item fields and
   topic bodies, and on conversation images in the dashboard page. It opens a
   modal overlay over the whole page (dimmed backdrop), not a new tab.
2. Inside the overlay: wheel / trackpad pinch zoom centred on the pointer,
   drag to pan, double-click toggles fit / 100% (or 2x for SVG), `+` / `-` /
   `0` keys and small on-screen buttons (zoom in, zoom out, fit, open
   original in a new tab, close), Esc or a backdrop click closes. Touch:
   pinch zoom and one-finger pan. Mermaid and inline SVG zoom as vectors
   (clone the SVG into the overlay rather than rasterising), so text stays
   sharp.
3. Topic bodies render inside an iframe (`doc-frame.js`, `<base
   target="_blank">` in `wswhiteboard/render.go`). The overlay must cover the
   whole page, not just the iframe: have the frame post the clicked image
   source or the serialised SVG to the parent page, which opens the viewer;
   keep the existing link behaviour for real links. An image wrapped in a
   link opens the viewer (the viewer's "open original" button still reaches
   the link target).
4. Works on the dashboard session page (light and dark via the existing
   `data-theme` variables), the dashboard board-first page if it shows the
   same content, and the standalone whiteboard HTML file. Region swaps (SSE
   re-render) must not break an open viewer or leave duplicate listeners: use
   delegated event listeners.
5. Keyboard and screen-reader basics: focus moves into the overlay on open and
   returns to the clicked element on close; the overlay has `role="dialog"`
   and `aria-modal`; images get `cursor: zoom-in`.

## Verification

- `cd agents-plugin-tool && go build ./... && go vet ./internal/wswhiteboard/ ./internal/wsdashboard/ && go test -count=1 ./internal/wswhiteboard/ ./internal/wsdashboard/ ./internal/wsi18n/...`.
- Add render tests in the existing style for whatever markup or asset hooks
  you add (for example the viewer script being included on each page kind).
- Headless Chromium (cached Playwright under
  `~/.npm/_npx/*/node_modules/playwright`): on a rendered board page with an
  item image, a topic-body image, and a mermaid diagram, open each, zoom with
  the wheel, pan by drag, close with Esc; check the overlay covers the full
  page when opened from the topic-body iframe; no console errors; light and
  dark. Screenshots to `/tmp/whiteboard-image-viewer/`.

## Return

Commits, changed files, test output, screenshot paths, and anything left open.
