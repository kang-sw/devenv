# Plan: review-sweep repair - English pill label, sandboxed Now on served pages

## Outcome

Two findings of the range review sweep `9768d4607..develop` (full text under
"Slice 2" in
`/Users/kang-sw/.cache/ws@kang-sw-devenv/proj/17da6bdc/review-paths/57c9501f-01-findings.md`),
decided by the user on the board 2026-11-07:

- I5, board (98) b: the open-question pill in
  `agents-plugin-tool/internal/wswhiteboard/assets/pill.js` shows an English
  label like the rest of the page chrome: `↑ 3 open` / `↓ 3 open` (arrow,
  count, "open"). Supersedes the Korean wording of (75). Update pinned
  expectations in tests.
- m2f, board (99) c: on daemon-served pages (the session page's board column
  and the served board page), the Now slot renders inside a sandboxed iframe
  (`srcdoc`, `sandbox="allow-scripts"` and no `allow-same-origin`), so its
  inline scripts still run (canvas drawings documented in
  `rsrc/whiteboard/whiteboard.md` keep working) but run in an opaque origin
  that cannot reach the daemon's same-origin message/steer/open routes.
  - The iframe sizes to its content (e.g. a small inline script posting its
    height to the parent via postMessage; the parent accepts only messages
    whose `source` is that iframe's contentWindow) and follows the page theme
    (light/dark and the CSS variables the Now content uses today).
  - Links in Now behave as they do today as far as the sandbox allows (open in
    a new tab / the existing link actions); local images in Now still load.
    If a sandboxed opaque origin cannot load them through the daemon's file
    route (cookie/token), solve it without granting same-origin (e.g. inline
    data URIs at render, or a route that serves Now assets without the
    cookie) and report what you chose.
  - The disk board page (file://) keeps rendering Now inline as today; no
    change there.
  - The Now slot's open/collapsed/greyed behavior and live board swaps keep
    working (the iframe is re-created on swap; no flicker regression beyond
    that).

## Settled constraints

- Files: `agents-plugin-tool/internal/wswhiteboard/` (assets, render/regions
  code, tests) and `agents-plugin-tool/internal/wsdashboard/` (assets,
  page rendering, tests). Another delegate is editing these packages in
  parallel (frame-ancestors headers in wsdashboard `writeHTML`/pages,
  `wswhiteboard/store.go` fullDocumentPattern): keep your changes to the Now
  rendering path and pill.js, so the merge stays easy.
- No change to the whiteboard.update payload or the Now authoring rules.
  Update `rsrc/whiteboard/whiteboard.md` (all three package copies,
  byte-identical) only if its description of Now becomes wrong.
- Shipped-surface boundary applies (`ai-docs/manuals/shipped-surface-boundary.md`).

## Boundary

Commit on branch `delegate/review-fix-now-sandbox` only; do not merge or
push. Return to the lead if the sandbox approach cannot keep scripts or
images working without a product trade-off the user must decide.

## Verification

In agents-plugin-tool: `go build ./... && go vet ./internal/... ./cmd/...`;
`go test -count=1 ./internal/wswhiteboard/ ./internal/wsdashboard/ ./internal/mcp/`.
`node --check` on changed scripts. Headless Chromium (cached Playwright under
~/.npm/_npx/*/node_modules/playwright): a served session page whose Now holds
an inline-script canvas drawing, an SVG, a link and a local image -> drawing
renders, iframe height fits, a script inside the Now cannot fetch/POST to the
daemon origin (show the failure), theme light/dark, survives a board swap;
pill shows the English label. Report what was not checked.
