# Plan: dashboard - held bubble header buttons overlap the clock

## Outcome

On the dashboard session page, a held (queued, not yet delivered) user
message bubble shows steer and cancel buttons in its header. Those buttons
overlap the bubble's timestamp (clock) text. This bug predates the image-marker
work and was found during its headless check on 2026-11-07. Fix the layout so that:

- the header's clock and the steer/cancel buttons never overlap, at any
  bubble width the conversation column allows, including the narrow tabs
  layout;
- the buttons stay visible and clickable;
- light and dark themes both hold;
- finished (non-held) user bubbles keep their current header look.

## Settled constraints

- Expected files: `agents-plugin-tool/internal/wsdashboard/assets/dashboard.css`.
  Change `dashboard.tmpl` or the held-bubble markup in Go only if CSS alone
  cannot fix it. Do not touch `dashboard.js` beyond a trivial class change.
  A parallel worker is editing `dashboard.js` and the wswhiteboard renderer.
- Use existing CSS variables. Make no behavior change to steer or cancel.
- Shipped-surface boundary applies (`ai-docs/manuals/shipped-surface-boundary.md`).

## Boundary

Commit on branch `delegate/held-bubble-header-overlap` only; do not merge or
push.

## Verification

In agents-plugin-tool:
- `go build ./... && go vet ./internal/... ./cmd/...`
- `go test -count=1 ./internal/wsdashboard/`; update pinned markup
  expectations if the markup changes.
- Headless Chromium using the cached Playwright under
  ~/.npm/_npx/*/node_modules/playwright. Render a session page that has a held
  bubble, at a wide and a narrow width, in light and dark. Confirm that the
  bounding boxes of the clock and the buttons do not intersect. Take a
  screenshot of each case.

Report what was not checked.
