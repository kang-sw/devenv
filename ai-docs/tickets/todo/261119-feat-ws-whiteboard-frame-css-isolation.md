---
title: 화이트보드 doc-frame 목업 CSS 네임스페이스 충돌 방지 및 상위 스타일 오버라이드 허용
---

# Whiteboard doc-frame mockup CSS namespace isolation

## Background

Dashboard panel bodies already render inside an opaque sandboxed iframe
(`agents-plugin-tool/internal/wswhiteboard/render.go:742-782`:
`sandbox="allow-scripts allow-popups allow-popups-to-escape-sandbox"`, `srcdoc=...`).
The frame document links one stylesheet (`render.go:749`, `frameCSSFile`).
`frameCSSFile` is built at `render.go:215` as `readPageSource("page.css")` plus
the small `frameCSS` constant (`render.go:661-664`, currently only
`html, body { background: transparent; overflow: hidden; }`). `readPageSource`
(`render.go:48-60`) concatenates all six whiteboard stylesheets: `page.css`,
`page-items.css`, `page-stream.css`, `page-panel-diff.css`, `page-panels.css`,
`page-sticky.css`.

That stylesheet carries unscoped internal board chrome class names (`.card`,
`.btn`, `.head`, `.chip`, `.panel`, `.tbar`, and similar). Author `<wb:raw>`
mockup HTML placed in a panel body therefore matches board rules even though it
is inside the iframe. Example: `page-panel-diff.css:29` defines
`.pdg-bar { position: absolute; left: 5.5px; top: 1px; bottom: 1px; width: 3px; pointer-events: none; }`
for the diff gutter, and `panel-diff.js:189` appends `span.pdg-bar` into every
`.pdg-hit`. In a Paged Docking Gallery mockup inside `doc-frame`, a toolbar
that reused `.pdg-bar` was crushed into a 3px vertical gutter bar. This was
found in a live dogfood session.

The same collision probably applies to inline bodies on the standalone
`index.html` path (`RenderPage` with `framed=false`). This path was reported by
the owner and is not yet verified in code.

## Owner goal and design constraint

Do not strip all inherited styling blindly. Inheriting the dashboard's `--wb-*`
theme tokens and base typography is wanted, so mockups blend with the dashboard.
The chosen approach must:

- (a) remove internal-chrome namespace collisions between board classes and
  mockup markup, in both framed and inline paths;
- (b) let author or mockup `<style>` overrides win cleanly without specificity
  fights.

Note: the frame is sandboxed without `allow-same-origin`, so it has an opaque
origin and does not inherit parent page CSS or custom properties through the
DOM. The only channel for `--wb-*` tokens into a frame is the injected
`frameCSSFile`. Any slimming must keep the tokens and base rules in that file.

## Candidate directions (open for design)

1. Slim the frame stylesheet to theme tokens (`:root` and `[data-theme]`
   `--wb-*`) plus `.doc-body.md` base typography, wrapped in
   `@layer wb-frame`, so unlayered mockup styles beat base rules and board chrome
   does not leak into frames.
2. Scope panel-diff gutter selectors in `page-panel-diff.css` to
   `.pdg-layer > .pdg-hit > .pdg-bar` (or `data-ws-pdg`), so bare `.pdg-*`
   classes never affect authored DOM in framed or inline output.
3. Evaluate whether Shadow DOM or other scoping is ever needed for `<wb:raw>`,
   without breaking the `doc-frame.js` selection mention behavior or the
   `panel-diff.js` DOM walks.

These are not decided. The trade-offs between layers, scoping, and Shadow DOM
are the main open question.

## Constraints

- `page.css` is assembled byte-for-byte from the six files, and the comment at
  `render.go:36-39` says cascade and frame hashes are preserved. Any change to
  the frame stylesheet changes frame hashes. Check whether that is acceptable
  and whether any test pins them.
- `agents-plugin-tool/internal/wsdashboard/css_collision_test.go` enforces that
  a class defined in two dashboard module stylesheets is an intentional overlap.
  It registers the whiteboard `page.css` with prefix `wb` and root `dw`. New
  classes must take the `wb` prefix, and the change must keep that test green.
- Shipped surface and compatibility are in
  `ai-docs/manuals/shipped-surface-boundary.md` and
  `ai-docs/manuals/release-compatibility.md`. A CSS and asset-only change is
  expected to be T0 there. The frame CSS is generated at runtime and is not a
  persisted shape.
- Per-frame payload size and rendering cost are governed by
  `ai-docs/manuals/performance.md`. Check the slimmed stylesheet against it,
  since the same stylesheet is repeated in every panel's `srcdoc`.
- `doc-frame.js` selection mention behavior and `panel-diff.js` DOM walks must
  keep working. Check whether existing `wswhiteboard` tests (for example
  `acceptance_test.go` and `asset_source_test.go`) cover them; coverage is not
  verified yet.

## Decisions

Compatibility tier: **expected T0** (CSS and asset-only; no stored shape, MCP
argument, or event changes). To be confirmed when the design settles.

No design direction is selected. The three candidates above remain open.

## Open questions

- Is `@layer` alone enough to let mockup `<style>` win, given that unlayered
  author rules already beat layered rules? Or does collision avoidance need
  selector-level scoping (candidate 2) as well?
- Which subset of `page.css` counts as "theme tokens and base typography", and
  how is that boundary kept from drifting as board classes are added?
- Does the standalone `index.html` inline path need the same treatment, or can it
  keep the full stylesheet?
- How do frame hashes and any cache keys change, and is that change acceptable?

## Phases

### Phase 1: Settle the frame CSS isolation approach

Confirm the collision inventory: which bare board classes in the six files can
match author mockup markup, in both the framed and inline paths. Choose among the
candidate directions with the owner, then state the compatibility tier and the
frame-hash impact in `## Decisions`. Define the verification: a mockup containing
`.pdg-bar`, `.card`, `.btn`, and `.head` must keep its own styling, and inherited
`--wb-*` tokens must still apply. The verification is a side-by-side capture of
the result against the mockup, plus the `wswhiteboard` and `wsdashboard` test
suites run by the implementer. No local `go test` was run while authoring this
ticket.
