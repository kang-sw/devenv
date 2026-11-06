---
title: Whiteboard items render markdown, images, and mermaid
related:
  261106-epic-ws-dashboard-serve-daemon: the daemon later serves the same shared mermaid asset and board-referenced images (its image route decision (238))
sage-review-design: completed
sage-review-completeness: completed
sage-review-completeness-reviewed: 7c09bf51a8123768
sage-review-design-reviewed: 7c09bf51a8123768
completed: 2026-11-06
---

# Whiteboard items render markdown, images, and mermaid

## Background

Every whiteboard item field is escaped plain text today; the only markup is
the `{{nn}}` item reference (`internal/wswhiteboard/render.go`, `Parts`). A
lead that needs a list, a code block, a table, a diagram, or a screenshot to
explain a question has to move it into the Now slot, away from the item it
explains. User discussion on 2026-11-06 (whiteboard items (223)-(227),
(237), (246), (270)-(275)) settled rich item text; the numbers are the
board's item IDs and each decision below states its content in full. A
local, uncommitted HTML mockup informed the card layout; the decisions
below are the whole layout contract, so a worker needs no mockup.

## Decisions

- **Markdown in item fields.** `title` renders inline markdown only (bold,
  italic, code, links). `options`, `rationale`, and `recommendation` render
  block markdown: lists, code blocks, tables, images, and mermaid blocks.
  (223) a, with the user's note extending block markdown to `options`.
  Rejected: block markdown in `rationale` only; no markdown, rich content
  kept in the Now slot.
- **Mermaid is embedded in the binary.** `mermaid.js` is embedded with
  `go:embed` and written once as a shared file into the board folder (and
  into `embed_dir` when set); pages load it by relative path. Its theme
  follows the `--wb-*` colour variables and the box/store/edge shapes the
  page CSS already defines (`page.css` `.box`, `.store`, `.edge`, `.sub`).
  The cost is roughly 3 MB of binary size; mermaid is MIT licensed. (224) a
  Rejected: inlining mermaid into every page (megabytes per update); no
  mermaid, hand-drawn inline SVG only; loading from a CDN (violates the
  no-external-`src` rule and offline use).
- **Images are path links, not embedded data.** A markdown image names a
  file path; the page links it. A file deleted later leaves a broken image,
  which is accepted. (225), the user's correction of a data-URI proposal.
  Rejected: reading the file at update time into a data URI.
- **Image link rule.** Image paths follow the panel ticket-row link rule
  (8a926fd83, `whiteboardFileLink` in `internal/mcp/whiteboard_panels.go`),
  which keys on where the page is written: a page inside the repository
  links the image by a path relative to the page; a page written to an
  `embed_dir` outside the repository links it by an absolute `file:` URL.
  (237)
- **Parser.** goldmark, a pure-Go CommonMark parser with no cgo. (226)
  Rejected: a hand-written partial parser (tables, code blocks, and link
  sanitizing are edge-case heavy).
- **Link and image rendering.** Custom goldmark link, autolink, and image
  node renderers apply the target allow-list below; `html.WithUnsafe` stays
  off. Autolinks (`<scheme:...>`, and Linkify output if that extension is
  on) are a separate node kind, so `<javascript:...>` would bypass a
  link-only override. The renderers live in the pure `wswhiteboard` package,
  so `RenderPage` takes the link resolution (session root and the
  `whiteboardFileLink` rule from `internal/mcp`) as an injected callback or
  inputs, the way panel rows receive a precomputed link.
  goldmark's default renderer blanks `file:` destinations as dangerous, and
  `WithUnsafe` would also turn raw HTML on. goldmark output reaches
  `html/template` as trusted HTML, so raw-HTML-off plus this allow-list is
  the whole escaping boundary on lead text; tests pin it. (design review)
- **Line breaks.** Block fields render with goldmark hard wraps, so a lead's
  one-reason-per-line `rationale` keeps its line breaks instead of joining
  into one paragraph; ack-row reasons (`.why`) render inline markdown only.
  (design review) Rejected: telling leads to write `- ` lists, which breaks
  every existing multi-line rationale habit.
- **No raw HTML in item markdown.** goldmark's raw HTML stays off; the Now
  slot remains the only HTML channel. (227) Rejected: allowing HTML in item
  fields (blurs the escaping boundary with verbatim `answer_quote` text and
  adds a second place a script can enter).
- **Option layout.** When any option of an item contains block markdown, all
  of that item's options render as stacked vertical cards; items with only
  short one-line options keep the current inline layout. (246) Rejected:
  always stacking (most items have one-line options and the board would
  grow long). "Block markdown" means the option's goldmark AST holds a
  block node other than a single paragraph (list, code block, table, image
  paragraph, mermaid block, or more than one paragraph); the worker pins
  the set in tests. A stacked option is a non-`button` element with
  `role="button"` and `aria-pressed`, keeping the `form.js` hooks, so links
  inside it work and its text is selectable; a click on a link inside it
  does not pick the option. It carries `tabindex="0"` and `form.js`
  activates it on Enter and Space, which a native button gave for free. A
  settled row's picked-option chip shows the
  option's first line as inline markdown. (design review)

- **Item references inside markdown.** `{{nn}}` references resolve through
  a goldmark inline extension, so a reference inside a code span or code
  block stays literal. (270) Rejected: substituting references before
  parsing (rewrites code and fights goldmark's escaping).
- **User words stay plain.** `answer_quote` stays escaped plain text, and the
  `answer` reading stays plain text with `{{nn}}` references; neither renders
  markdown. (271)
- **Link and image targets.** Markdown links accept relative, `file:`, and
  `http(s)` targets; images accept only relative and `file:` paths; any
  other scheme renders as text. (272) A relative image or link path
  resolves against the session root, the way `now_path` does, and a bare
  absolute path is a file path; both then go through the image link rule
  above. (281) A fragment-only target (`#item-12`) passes through unchanged
  as an in-page anchor. Rejected: remote images (the page would fetch from the
  network, against its no-external-`src` rule and offline use);
  page-relative paths, which break because the same item renders into the
  board folder and into an arbitrary `embed_dir`; the process working
  directory, which callers cannot see (281).
- **Mermaid asset lifecycle.** The shared mermaid file name carries the
  embedded asset's version hash and is written only when missing; a page
  loads it only when it contains a mermaid block. (273) Rejected: rewriting
  about 3 MB on every update; loading mermaid on every page.
- **Mermaid build and theme.** The vendored asset is the single-file IIFE
  `dist/mermaid.min.js` of a pinned mermaid 11.x release, kept under
  `internal/wswhiteboard/assets/` with its version and MIT license notice
  beside it. The split ESM build is rejected: browsers block module scripts
  over `file://`. The page resolves the `--wb-*` variables with
  `getComputedStyle` before `mermaid.initialize` (mermaid's colour math
  cannot read `var(...)` strings) and maps the box/store/edge look through
  `themeCSS`. A mermaid block renders as `<pre class="mermaid">` holding its
  source, so a host that cannot load the sibling script (a sandboxed or
  srcdoc embed) still shows readable source. (design review)
- **Playbook.** The whiteboard playbook's `## Payload` gains one short
  paragraph naming the inline and block markdown fields and the mermaid
  fenced block, mirrored to wsflow and Pi, and the existing "`options`:
  plain strings" line is reworded to match. (274) Refusals quote
  that section verbatim (f98d8e83), so the paragraph also reaches refusal
  text; keep it short.
- **One phase.** The playbook paragraph documents the landed rendering and
  verifies with it. (275) Rejected: a separate playbook phase.

## Constraints

- Shipped surface: the page template, CSS, scripts, the embedded mermaid
  asset, and the whiteboard playbook ship downstream. Read
  `ai-docs/manuals/shipped-surface-boundary.md`, `ai-docs/manuals/ws-mcp.md`,
  `ai-docs/manuals/skill-authoring.md`, and
  `ai-docs/manuals/wsflow-mirroring.md` before editing.
- Out of scope: the dashboard daemon and its image route (epic
  `261106-epic-ws-dashboard-serve-daemon`); the Now slot's rules; the store
  schema and tool argument shapes (item fields stay strings).
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/skill-authoring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin-wsflow/rsrc/`, `agents-plugin-wsflow/skills/`, `agents-plugin-tool/internal/wsdoc/conventions/`)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin/hooks/`, `agents-plugin-wsflow/`)

## Prior Decisions

- f98d8e83 (2026-11-04, commit): "The inner payload schema has one source: refusals quote the rendered whiteboard playbook's ## Payload section instead of a Go copy, so schema text cannot drift from the playbook." — bearing: constrains
- f98d8e83 (2026-11-04, commit): "Store lives in its own pure package internal/wswhiteboard (Apply/Query/BuildView/RenderPage with an injected Env) so batch semantics are unit-tested without the MCP server; the MCP layer only does persistence, file writes" — bearing: constrains
- 261105-feat-whiteboard-explicit-item-ref-syntax (2026-11-05, commit 12e7cc4e): "answer_quote is never resolved (user's verbatim words); topic names are not in the ticket's field list and stay literal." — bearing: supports
- 261106-feat-whiteboard-dashboard-panels (2026-11-06, Result 8a926fd83): "`PanelRow` gains `Link`. In `wswhiteboard`, `rowHref` marks only links with no scheme or a `file:` scheme as `template.URL`; a `javascript:` link still becomes `#ZgotmplZ`." — bearing: supports
- 261106-feat-whiteboard-dashboard-panels (2026-11-06, Decisions): "On the full page a ticket link is a path relative to `ai-docs/whiteboards/`. In an embed written to an `embed_dir` outside the repository, it is an absolute `file:` URL" — bearing: supports
- 261106-feat-whiteboard-dashboard-panels (2026-11-06, Decisions): "No truncation. The embed has no truncation logic. The fixed inline cost of an embed is about 25KB (CSS, `form.js`, sprite), far from the limit" — bearing: constrains
- 261106-feat-whiteboard-board-text-language (2026-11-06, Resolution): "Resolved by 8f2d513d1: one line in the whiteboard playbook's Payload section (mirrored to wsflow and Pi) says board text follows the conversation language." — bearing: supports
- 260525-feat-ws-dashboard-markdown-renderer-polish (2026-05-25, commit 3eb29bcb): "Raw HTML remains ignored by the Markdown renderer; tests assert inert HTML is absent from rendered markup." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wswhiteboard/render.go, assets/page.tmpl, assets/page.css, render_test.go, go.mod for goldmark, page writer internal/mcp/whiteboard_tools.go#L404-L411, agents-plugin/rsrc/whiteboard/whiteboard.md plus wsflow and Pi mirrors and manifests |
| scope.surface | cross-module | wswhiteboard renderer, internal/mcp page writer for the shared mermaid file, and the shipped whiteboard playbook in three packages |
| scope.new_public_symbol | yes | an exported mermaid asset name or bytes accessor in wswhiteboard is needed if internal/mcp writes the shared file, since pages are written in internal/mcp/whiteboard_tools.go |
| scope.new_type_contract | no | item fields stay strings and tool argument shapes are out of scope per Constraints |
| scope.test_surface | existing | agents-plugin-tool/internal/wswhiteboard/render_test.go incl TestRenderPageEmbedSizeBudget, internal/wsrsrc/wsflow_mirror_test.go, internal/wsrsrc/pi_mirror_test.go |
| complexity.reuse_points | confirmed | pageData.Parts and itemRefPattern render.go#L41 L219-L249, rowHref render.go, whiteboardFileLink whiteboard_panels.go#L99-L115 |
| complexity.side_effect_risk | moderate | writes a new shared file into the board folder and embed_dir and adds roughly 3 MB to the binary |
| risk.correctness | moderate | a goldmark inline extension must keep {{nn}} literal in code spans and blocks and the option-stacking rule depends on block detection |
| risk.fit | moderate | first goldmark dependency, absent from agents-plugin-tool/go.mod, and the Payload section is quoted verbatim in refusals per f98d8e83 |
| risk.test | moderate | mermaid rendering and stacked cards are visual and the prior panels Result notes no headless browser was available |
| risk.security_or_contract | high | goldmark output must be trusted as template.HTML, so raw-HTML-off and link and image scheme filtering become the only escaping boundary on lead text |

## Phases

### Phase 1: Markdown item rendering with mermaid and images

Render item `title` (inline) and `options`, `rationale`, `recommendation`
(block) through goldmark with raw HTML off, keeping `{{nn}}` references
working inside markdown, on both the full page and the embed. Embed the
mermaid asset and write it as a shared file beside the pages that need it;
stack option cards per the option layout decision; document the markdown
fields in the whiteboard playbook and mirror it.

Verification: render tests in `internal/wswhiteboard` (inline-only title,
block fields, raw HTML escaped, `{{nn}}` inside markdown and inside code
spans, relative and absolute image and link paths with a page inside the
repository and an `embed_dir` outside it, the link and image target
allow-list including `javascript:` as a link and as an autolink, remote
images, and a fragment-only link, hard-wrapped
rationale lines, ack-row reasons (`.why`) rendering inline markdown only,
stacked options only when an option has block markdown,
the mermaid `<pre>` source fallback and its theme initialization reading
resolved `--wb-*` values), a test where the shared mermaid file is written
(the page writer in `internal/mcp`) that it is written once, named by its
version hash, and referenced relatively, the existing embed size budget
test, the wsflow and
Pi mirror tests, then `go test ./...` in `agents-plugin-tool`. Render one
full page and one embed with markdown, an image, and a mermaid block to a
scratch path for the lead to look at, including the mermaid theme in light
and dark. No inline-HTML host is available here, so the embed's behaviour
inside a real host is checked by the source fallback test only.

### Result (6881738e2, 10da59e05) - 2026-11-06

Landed:

- Renderer: `internal/wswhiteboard/markdown.go` runs two goldmark (v1.8.7)
  pipelines per page. The block pipeline has hard wraps, GFM tables, and
  strikethrough. The inline pipeline's only block parser is the paragraph,
  with no paragraph transformers, so `- `, `# `, fences, and `[x]: y` stay
  literal in titles. Neither pipeline registers goldmark's HTML block or raw
  HTML parser, so a typed tag is escaped text, and a line that starts with
  a tag no longer swallows the markdown after it. `html.WithUnsafe` stays
  off as a backstop. Custom renderers cover links, autolinks, images, fenced
  code (mermaid becomes `<pre class="mermaid">`), paragraphs (a lone
  paragraph renders without `<p>`), and the `{{nn}}` inline node. A
  reference inside link text renders as plain `(nn)`.
- Target allow-list (`resolveTarget`): entity references and backslash
  escapes in a destination are resolved before its scheme is classified,
  because goldmark leaves `Destination` raw. `fileHref` then checks the href
  that `FileLink` returns: a relative href whose first segment holds a colon
  gets a `./` prefix, and a decoded `#` or `?` is re-escaped. External
  http(s) links open in a new tab.
- Link rule injection: `View`/`ViewInput.FileLink`. `internal/mcp` passes
  `whiteboardItemFileLink(root, whiteboardPageDir(in))`, which resolves the
  path against the session root and then applies `whiteboardFileLink`.
- Template, CSS, and JS: titles use `.Page.Inline`; options, rationale, and
  the recommendation use `.Page.Block`; ack `.why` lines are inline; the
  picked chip uses `.Page.FirstLine`. `itemData.Stacked` (`IsBlock`:
  anything but one paragraph, or a paragraph with an image) switches the
  options to `.opts.stack` cards (`div[role=button][tabindex=0]`).
  `form.js` ignores clicks on links inside an option and handles Enter and
  Space on the cards. `.md` styles were added.
- Mermaid: the 11.17.2 `dist/mermaid.min.js` IIFE is vendored in
  `assets/mermaid/` with its LICENSE and a README. `MermaidAsset()` names
  it `wb-mermaid-<sha12>.js`, which matches no whiteboard GC pattern.
  `assets/mermaid-init.js` resolves the `--wb-*` colours through a probe
  element and `getComputedStyle`, then calls `mermaid.initialize` with
  theme `base`, `securityLevel: "strict"`, and a `themeCSS` that maps the
  box, store (`A:::store`), edge, and sub looks. A page loads the asset
  only when it rendered a mermaid block (`pageData.Mermaid`, read by the
  closing tags). `ensureWhiteboardMermaid` writes the file only when it is
  missing, before the page that uses it, both into the board folder and
  into `embed_dir`.
- Playbook: `## Payload` gains one Markdown bullet, and the `options` line
  no longer says "plain strings". The change is mirrored to wsflow (regen)
  and Pi (copy), with manifests regenerated.

Verification:

- `go test ./internal/wswhiteboard`: ok. The new `markdown_test.go` covers
  the inline-only title, block fields, raw HTML escaping, references in
  markdown and code, the link and image allow-list (javascript:, autolink,
  entity, data, remote, fragment, `//host`, `file://host`), that a file
  href cannot become a scheme, hard wraps, inline ack reasons, the
  stacking set, the picked chip's first line, and the mermaid fallback and
  theme init. The existing embed size budget test also passes.
- `go test ./internal/mcp`: ok. The new `whiteboard_markdown_test.go`
  covers repository-relative links and `file:` links for an embed_dir
  outside the repository, that the real rule never yields a scheme, and
  that the mermaid file is written once under its hash name, referenced
  relatively, and also written into embed_dir.
- `go test ./...` in `agents-plugin-tool`: ok.
  `python3 -m unittest discover agents-plugin-wsflow/tests`: ok.
- Scratch renders, checked with headless Chromium in light and dark (the
  dark render forces the dark media block; the headless shell ignores
  `--force-dark-mode`), are in
  `/tmp/claude-1000/-home-swkang-devenv/a921ec72-4ebb-4fe9-99fe-e15094718125/scratchpad/wb/`:
  `full.html`, `embed/embed.html`, `full-light.png`, `full-dark.png`,
  `embed-light.png`.

Decisions:

- Raw HTML is removed at the parser rather than escaped in a renderer. This
  makes typed tags text and stops a leading tag from swallowing the
  markdown after it.
- Per the ack-row decision, an open technical or announced item's rationale
  shows inline only in the ack row, so a diagram there shows as text. The
  playbook bullet says so.
- `UsesMermaid(html)` decides the asset write, so `RenderPage` keeps its
  signature.
- Lite review: one Critical, fixed in 10da59e05: a file named
  `javascript:...` beside the page produced a `javascript:` href. One Minor
  fixed: `%23` and `%3F` in file names. One Minor left open: in the inline,
  non-stacked option layout, a link inside the native `<button>` may not be
  clickable in some browsers.

## Sage Review Round 1 (2026-11-06)

### Design Reviewer — concern

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | Base path for relative image and link targets is unstated | important | autonomous |
| 2 | Block-markdown rationale silently drops today's one-bullet-per-line rendering | important | autonomous |
| 3 | goldmark's default renderer blanks file: URLs unless the link and image renderers are overridden | minor | autonomous |
| 4 | Option markup is a button, which cannot hold block markdown or working links | minor | autonomous |
| 5 | Mermaid build choice matters for file:// loading | minor | autonomous |
| 6 | Playbook still says options are plain strings | minor | autonomous |
| 7 | Embed-side relative loads depend on the host | minor | autonomous |

### Completeness Reviewer — block

| # | Title | Severity |
|---|-------|----------|
| 1 | Base of relative image and link paths is undefined | important |
| 2 | Mockup reference is uncommitted | minor |
| 3 | Whiteboard item numbers are opaque to a fresh reader | minor |
| 4 | Block markdown trigger for stacked option cards is not defined | minor |
| 5 | Mermaid theming has no acceptance check | minor |
