# Dashboard file view renders HTML files in a sandbox

## Goal

A link on the board to an `.html` file inside the session's project root
opens the dashboard file view (`GET /s/<reply-id>/file?path=`), which today
serves every non-markdown text file as `text/plain` under the image route's
CSP, so an HTML document shows as raw source. User feedback 2026-11-12 with a
screenshot of `ai-docs/paged/261112-tracker-guide/document.html` shown as
source. Board item 368 a: render HTML files as pages, scripts allowed, inside
a CSP sandbox with an opaque origin, with the document's relative assets
(stylesheets, scripts, images, fonts next to it) loading.

## Decisions (settled; do not reopen)

- Render `.html` / `.htm` as a page under a CSP `sandbox` that allows scripts
  and does NOT allow same-origin, so the document runs in an opaque origin
  and cannot read the dashboard's storage, cookies, or responses.
  Rejected: rendering without scripts (b); keeping text plus an
  open-in-browser button (c).
- Relative asset URLs in the document resolve and load. The natural way is a
  path-style route whose URL directory mirrors the file's directory inside
  the root (for example `/s/<reply-id>/raw/<root-relative path>`), so the
  browser resolves `./style.css` itself; the file view link for an HTML file
  then points at (or redirects to) that route. The exact route shape is your
  call; a `<base>` injection or HTML rewriting is the less preferred
  alternative.
- Compatibility tier T0: an additive daemon route and dashboard behaviour.

## Security constraints (the core of this task)

- Every file the new route serves passes the same containment as the file
  view: `openContained` (lexical containment, symlink-resolved containment,
  the file itself not a symlink, Lstat/opened SameFile). Every refusal is a
  404. Path parsing from the URL path must not open a traversal (`..`,
  encoded slashes, backslashes on Windows, NUL); reuse `relativeInside`.
- The HTML response's CSP carries `sandbox allow-scripts` (plus only what a
  paged document truly needs, e.g. `allow-popups` is NOT needed; never
  `allow-same-origin`, `allow-top-navigation`, or `allow-forms`). Restrict
  fetches: `connect-src 'none'`, `form-action 'none'`, and resource loads
  limited to the same route (`'self'` or the route prefix) plus `data:` /
  `blob:` where harmless; no external network. Keep `frame-ancestors`
  consistent with how the dashboard opens the file view (check whether the
  board ever frames it; today the file view sets `frame-ancestors 'none'`).
- A sandboxed script can still send requests without reading responses
  (images, script tags, navigation of its own frame). Verify that the
  daemon's state-changing endpoints (message send, answers, tracker writes,
  anything POST/PUT/DELETE) are not reachable from an `Origin: null`
  document: check how they gate today (Origin/Sec-Fetch-Site/token checks)
  and record the finding in the commit's AI Context. If any is reachable,
  stop and report rather than widening scope.
- Sub-resources served by the route (css, js, images, fonts, other html)
  get correct content types, `X-Content-Type-Options: nosniff`, and the same
  sandbox CSP where it applies (an HTML subframe too). Images keep the image
  route's SVG rule (no script in an SVG served by the daemon).
- LAN access: check how a LAN token holder authenticates (cookie vs query
  token) and make sure the document's relative asset requests from an
  opaque origin still authenticate (a SameSite cookie is not sent from an
  opaque origin; a module script needs CORS). Loopback must work; if LAN
  cannot be made to work without weakening the sandbox, keep LAN behaviour
  safe (assets 404) and say so in the report.
- Size cap `fileViewMax` and the binary/NUL rule still apply to the HTML
  document; assets may be served by `http.ServeContent` with a sensible cap.

## Scope

- `agents-plugin-tool/internal/wsdashboard/file_route.go` (and the route
  registration where `handleFile` is wired), plus the header comment block
  describing what the file view shows.
- `FileActions` / `FileSrc` callers: an `.html` link opens the rendered page.
- Tests (Go, `file_route` test files): HTML served with the sandbox CSP and
  no `allow-same-origin`; a relative asset next to it loads with the right
  type; traversal and symlink attempts 404 on the new route; non-HTML text
  unchanged (still `text/plain`); markdown unchanged.
- Headless check (playwright-core under the scratchpad's `pw/node_modules`,
  chromium at `~/.cache/ms-playwright/chromium-1223/chrome-linux64/chrome`;
  scratchpad /tmp/claude-1000/-home-swkang-devenv/6900e174-9e2f-4bdd-b25f-ae5a39a1cd42/scratchpad):
  start a daemon binary built from this worktree on a free port (or an
  httptest harness) against a temp root holding an HTML file with a
  relative css, js, and image; confirm it renders styled, the script runs,
  `document.domain`/`origin` is opaque (`null`), and a `fetch` to the
  daemon is blocked. Also open the real
  `ai-docs/paged/261112-tracker-guide/document.html` from the main checkout
  (/home/swkang/devenv) if it is present there, and screenshot it. Shots into
  scratchpad/file-html/.

## Constraints

- Read `ai-docs/manuals/release-compatibility.md` and
  `ai-docs/manuals/shipped-surface-boundary.md` before editing (AGENTS.md
  Implementation Conventions rows for `agents-plugin-tool/`).
- Heavy Go test runs only through
  `~/.local/bin/ws-rgo -C agents-plugin-tool -m 10 -- test -count=1 ./internal/wsdashboard/...`;
  never run go build/test/vet locally (building one daemon binary for the
  headless check is also remote-only: if you need a local binary, ask the
  lead in your report instead, or use a node/httptest-free approach such as
  a Go test that writes the served responses to files the headless check
  then loads from a tiny static server reproducing the headers).
- Commit on branch `delegate/file-view-html-render` in this worktree; do not
  merge or push.
