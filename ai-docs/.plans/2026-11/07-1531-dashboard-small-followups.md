# Plan: dashboard round-2 small follow-ups

## Outcome

Four small leftovers from the dashboard round-2 landings, each a separate
commit:

1. **Steer route guard coverage.** `agents-plugin-tool/internal/wsdashboard/access_test.go`
   `guardedRoutes` lacks the `/steer` route (added by steer promotion,
   6e64e2bae). Add it so the Origin/LAN/token guard tests cover it like the
   other mutating routes. Test-only; global guards already apply.
2. **Panel ticket-row paths as links.** On the whiteboard Tickets panel
   (`internal/mcp/whiteboard_panels.go` and its template in
   `internal/wswhiteboard/assets/page.tmpl`), a ticket row's path is still
   plain text. On served pages make it the same file-action anchor that
   served-page file links became in link-actions (073fd3873: anchor around
   the label, path as `title`, the shared link markers/attributes that
   `dashboard.js` link menu reads). On the disk board page keep today's
   behavior unless the existing file-link writer already handles the disk
   case. Reuse the existing link writer; do not invent a second one.
3. **Held bubble header overlap.** On the session page, the held (queued)
   message bubble's header puts the steer and cancel buttons over the clock
   time (pre-existing, seen in the image-markers check). Lay out the header so
   the clock and the buttons never overlap at any bubble width, light and
   dark. `internal/wsdashboard/assets/dashboard.css` (and the template only if
   needed).
4. **gofmt drift.** `gofmt -l` reports
   `internal/mcp/implement_resolver_test.go`, `internal/wsdoc/ticket_create.go`,
   `internal/wsdoc/tickets_mutate.go`. Run `gofmt -w` on exactly those files
   (and any other `gofmt -l` hit, listed in the commit).

## Settled constraints

- No protocol, route, or behavior change beyond the four items.
- Another worker (status band) is editing `internal/wslive`, `internal/mcp/live_snapshot.go`,
  the session page band, and the adapters on a parallel branch: keep the CSS
  change confined to the held bubble header rules.
- Shipped-surface boundary applies (`ai-docs/manuals/shipped-surface-boundary.md`);
  `internal/mcp/` edits read `ai-docs/manuals/ws-mcp.md`.

## Boundary

Commit on branch `delegate/dashboard-small-followups` only; do not merge or
push. If item 2 needs a new link kind or a server-side route change, skip it
and report back.

## Verification

In agents-plugin-tool: `gofmt -l ./internal ./cmd` empty;
`go build ./... && go vet ./internal/... ./cmd/...`;
`go test -count=1 ./internal/wsdashboard/ ./internal/wswhiteboard/ ./internal/mcp/ ./internal/wsdoc/`.
Headless Chromium (cached Playwright under ~/.npm/_npx/*/node_modules/playwright):
held bubble with steer/cancel at narrow and wide widths, no overlap, light and
dark; a served board's ticket row path is an anchor the link menu opens.
Report what was not checked.
