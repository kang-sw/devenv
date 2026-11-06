---
title: Whiteboard items render markdown, images, and mermaid
related:
  261106-epic-ws-dashboard-serve-daemon: the daemon later serves the same shared mermaid asset and board-referenced images (its image route decision (238))
---

# Whiteboard items render markdown, images, and mermaid

## Background

Every whiteboard item field is escaped plain text today; the only markup is
the `{{nn}}` item reference (`internal/wswhiteboard/render.go`, `Parts`). A
lead that needs a list, a code block, a table, a diagram, or a screenshot to
explain a question has to move it into the Now slot, away from the item it
explains. User discussion on 2026-11-06 (whiteboard items (223)-(227),
(237), (246)) settled rich item text, with the HTML dashboard mockup
(`ai-docs/ref/whiteboard-mockup/r4-dashboard/`, uncommitted) showing the
card layout.

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
  (8a926fd83): a path inside the repository renders relative to the page, a
  path outside it (an `embed_dir` outside the repository) renders as a
  `file:` URL. (237)
- **Parser.** goldmark, a pure-Go CommonMark parser with no cgo. (226)
  Rejected: a hand-written partial parser (tables, code blocks, and link
  sanitizing are edge-case heavy).
- **No raw HTML in item markdown.** goldmark's raw HTML stays off; the Now
  slot remains the only HTML channel. (227) Rejected: allowing HTML in item
  fields (blurs the escaping boundary with verbatim `answer_quote` text and
  adds a second place a script can enter).
- **Option layout.** When any option of an item contains block markdown, all
  of that item's options render as stacked vertical cards; items with only
  short one-line options keep the current inline layout. (246) Rejected:
  always stacking (most items have one-line options and the board would
  grow long).

- **Item references inside markdown.** `{{nn}}` references resolve through
  a goldmark inline extension, so a reference inside a code span or code
  block stays literal. (270) Rejected: substituting references before
  parsing (rewrites code and fights goldmark's escaping).
- **User words stay plain.** `answer_quote` stays escaped plain text, and the
  `answer` reading stays plain text with `{{nn}}` references; neither renders
  markdown. (271)
- **Link and image targets.** Markdown links accept relative, `file:`, and
  `http(s)` targets; images accept only relative and `file:` paths; any
  other scheme renders as text. (272) Rejected: remote images (the page
  would fetch from the network, against its no-external-`src` rule and
  offline use).
- **Mermaid asset lifecycle.** The shared mermaid file name carries the
  embedded asset's version hash and is written only when missing; a page
  loads it only when it contains a mermaid block. (273) Rejected: rewriting
  about 3 MB on every update; loading mermaid on every page.
- **Playbook.** The whiteboard playbook's `#### Payload` gains one short
  paragraph naming the inline and block markdown fields and the mermaid
  fenced block, mirrored to wsflow and Pi. (274)
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
spans, image link rule inside and outside the repository, stacked options
only when an option has block markdown, mermaid asset written once and
referenced relatively), the existing embed size budget test, the wsflow and
Pi mirror tests, then `go test ./...` in `agents-plugin-tool`. Render one
full page and one embed with markdown, an image, and a mermaid block to a
scratch path for the lead to look at.
