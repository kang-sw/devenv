# whiteboard: todo panel rendering polish

User decision (2026-11-06, whiteboard items (221) b, (222)): polish how the
whiteboard's todo panel renders. Rendering only; no change to the todo store,
tool schemas, or playbook text.

## Current state

- `agents-plugin-tool/internal/mcp/whiteboard_tools.go` `todoPanel`: one
  `PanelRow{State, Label: key, Text: title}` per todo in store order. The
  embed layout already drops done rows (merged in 32237da04); the full page
  keeps them interleaved.
- `internal/wswhiteboard/render.go` `rowMarker`: the row's box is the text
  marker `[x]` `[~]` `[>]` `[ ]` in monospace.
- `internal/wswhiteboard/assets/page.tmpl` `define "panel"`, `page.css`
  `.rows` (done = strike-through, wip = bold, defer = muted),
  `assets/sprite.svg` holds the icon symbols (`i-check`, `i-todo`, ...).
- Other panels (agenda, tickets, mailbox) share the `panel` template and
  `.rows` styles; keep them unchanged.

## Assignment

1. Replace the text marker with icon checkboxes from the sprite: pending =
   empty box, wip = box with an accent in-progress mark, done = checked box,
   defer = box with a defer mark (add symbols to `sprite.svg` as needed,
   in its existing stroke style).
2. A wip row gets an accent background tint using the existing `--wb-*`
   theme variables (light and dark).
3. Full page: unfinished rows first in store order; done rows move below
   them, muted, folded behind a `done N` chip (a `<details>` the way the
   board folds settled items; closed by default). The panel's `done/total`
   count chip stays. Embed behaviour stays as it is (done rows omitted).
4. The todo key stays as the row's tooltip.
5. Keep the change inside the todo panel's rendering path; if the shared
   `panel` template needs a branch, scope it to rows that carry a `State`.

Read before editing: `ai-docs/manuals/shipped-surface-boundary.md` and
`ai-docs/manuals/ws-mcp.md` (the page and template ship downstream).

## Verification

- Update or add tests in `internal/mcp/whiteboard_panels_test.go` (row order,
  done folding on the full page, embed unchanged) and
  `internal/wswhiteboard` render tests (marker rendering, no `[x]` text).
- `go test ./internal/wswhiteboard/... ./internal/mcp -run 'Whiteboard|Panel' -count=1`,
  then `go test ./...` in `agents-plugin-tool` run in the background and its
  output read before reporting.
- Render one full page and one embed to a scratch path and report their
  paths, so the lead can look at them; no headless browser is installed.

## Commit

One commit on develop through `ws/git.commit` with `## AI Context` and the
Co-Authored-By trailer. Do not push. Keep `ai-docs/ref/whiteboard-mockup/`
out of the commit.

## Return boundary

Stop and report before changing the todo store, tool schemas, the other
panels' rendering, or any playbook text.
