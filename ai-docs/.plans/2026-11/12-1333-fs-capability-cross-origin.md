# File view and preview capabilities cannot be used from another origin

## Goal

Review sweep finding I1 (findings: /home/swkang/.cache/ws@kang-sw-devenv/proj/dac18b1d/review-paths/769fe701-01-findings.md; partition
report with the trace: /home/swkang/.cache/ws@kang-sw-devenv/proj/dac18b1d/review-paths/d169a2be-01-sweep-dash-go.md). A document served
by the capability preview route (7f890500d) or the file view route
(d8ac1bd6f) can read its own capability URL from `location.href` and
navigate itself to another origin (CSP sandbox allows self-navigation; the
session page has no `frame-src`; the preview iframe is
`sandbox="allow-scripts"`). Because both routes answer with
`Access-Control-Allow-Origin: *` and check neither Origin nor
Sec-Fetch-Site, that origin can then `fetch` workspace files through the
capability for its 15-minute life (the view route also serves gitignored
non-dot files). User-approved direction (board 380, 2026-11-12).

## Decisions

- Make the capability unusable from any origin other than the dashboard's
  own pages and the opaque-origin documents the routes themselves serve:
  - Drop or narrow `Access-Control-Allow-Origin: *` on the view route; on the
    cap route keep CORS only as far as a sandboxed (Origin `null`)
    document's own module scripts need it, if at all (check what the preview
    really needs; prefer no ACAO).
  - Refuse cap/view requests whose `Sec-Fetch-Site` is `cross-site` (and,
    where present, an `Origin` that is neither the daemon's own origin nor
    `null`), so an attacker page cannot read through a leaked capability.
    Mind how a sandboxed opaque-origin document's own sub-resource requests
    look (Sec-Fetch-Site for an opaque initiator); the HTML demo's relative
    CSS/JS/SVG must keep loading.
  - Add `frame-src` (and/or `navigate-to`-equivalent hardening that
    browsers actually support) to the session page so a framed preview
    cannot be navigated to another origin inside the dashboard.
- Rejected: shortening the TTL only (narrows the window, does not close it).
- Fix the security comment at fs_capability.go:71-83 to state the real model.
- LAN mode: the lanGuard pass-through by URL shape (previewCapability) must
  not become a bypass of the new checks.
- Compatibility tier T0 (daemon route headers and checks).

## Verification

- Go tests in the file_route / fs_capability test files: a cross-site
  request (Sec-Fetch-Site cross-site, foreign Origin) to the view and cap
  routes is refused; a same-origin / opaque-origin sub-resource request still
  succeeds; no `Access-Control-Allow-Origin: *` on the view route; the session
  page carries the frame-src directive.
- Run only through
  `~/.local/bin/ws-rgo -C agents-plugin-tool -m 10 -- test -count=1 ./internal/wsdashboard/...`;
  never go build/test/vet locally.
- Headless (playwright-core under the scratchpad's pw/node_modules; chromium
  ~/.cache/ms-playwright/chromium-1223/chrome-linux64/chrome; scratchpad
  /tmp/claude-1000/-home-swkang-devenv/6900e174-9e2f-4bdd-b25f-ae5a39a1cd42/scratchpad,
  existing script scratchpad/pw/demo.js): the HTML demo
  /home/swkang/devenv/ai-docs/paged/file-view-demo/index.html still shows all
  checks ok through the file view, and a page on another origin that fetches
  a captured capability URL is refused. You may build nothing locally; use
  an httptest harness or a Go test that writes responses for a tiny static
  server, as the previous file-view delegate did, or report if you need the
  lead to replace the daemon.

## Constraints

- Read ai-docs/manuals/release-compatibility.md and
  ai-docs/manuals/shipped-surface-boundary.md before editing.
- Commit on branch delegate/fs-capability-cross-origin in this worktree; do
  not merge or push.
