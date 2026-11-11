---
title: "Whiteboard item fields accept sanitized HTML through the reply sanitizer"
related:
  261110-bug-ws-whiteboard-query-output-noise: "landed first; its D7 prints item fields in whiteboard.query output as plain text through `Plain` (inline_tags.go), which knows only the inline allowlist, so this ticket extends it to the widened set"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: c1edf076db34c0ef
sage-review-completeness-reviewed: c1edf076db34c0ef
completed: 2026-11-11
---

# Whiteboard item fields accept sanitized HTML

## Background

Downstream leads often put images and other HTML into whiteboard item
options (policy, technical, announced) (user, 2026-11-10 whiteboard, topic
"선택지 렌더러 (별건)"). Today:

- Item block fields (option, `wb:why`, `wb:recommend`) render through
  `pageData.Block` (`agents-plugin-tool/internal/wswhiteboard/markdown.go:210`,
  templates `ctx`, `opts`, and the rec line in `assets/page.tmpl:38`, `:52-53`):
  block markdown with images and mermaid, no raw HTML. An open technical or
  announced item's row (`ack`) is the exception: its `wb:why` renders one
  line per reason through `pageData.Inline`, not `Block`
  (`assets/page.tmpl:55`, `{{range lines .Rationale}}...{{$.Page.Inline .}}`;
  `rationaleLines`, `render.go:651`). Only the allowlisted
  attribute-free inline tags of `inline_tags.go` (`<code>`, `<b>`, `<strong>`,
  `<i>`, `<em>`, `<kbd>`, `<br>`) render; every other tag shows as escaped
  text.
- Topic bodies render through the raw pipeline (`renderDoc`,
  `markdown.go:194`) with HTML written as is, and the dashboard shows them in a
  sandboxed iframe (`framedDoc`, `render.go:475`; commit a068fb9f1, a review
  finding that script in the daemon origin could post message or steer).
- Finished lead replies render through the raw pipeline and then
  `sanitizeReply` (`sanitize.go:143`, called from `RenderText`,
  `markdown_text.go:62`): the fragment is parsed and re-serialized, and only
  allow-listed elements and attributes survive (no script, style, iframe,
  form controls other than a disabled checkbox; href/src through the markdown
  target rule; inline style without fetches or positioning; prefixed SVG ids).
  The served pages send no CSP, so this file is the whole trust boundary.
- A link inside an option card does not pick the option (`assets/form.js:39`).

## Decisions

1. **Item block fields accept raw HTML filtered by the reply sanitizer (user,
   (43) b, (44) b).** Option, rationale, and recommendation text render through
   the raw HTML pipeline and `sanitizeReply`, so images, tables, `details`,
   simple SVG, and other kept markup show as static HTML, without an iframe.
   Rejected: guiding leads toward markdown images in the playbook only
   ((43) a); sharing the topic body's raw renderer ((43) c), which needs either
   one iframe per option (too heavy, and it breaks click-to-pick) or unsandboxed
   HTML (reopens the a068fb9f1 finding); allowing only `<img>` in item fields
   ((44) a).
   This reverses earlier decisions, by the user's (43) b and (44) b ((276)):
   `261106-feat-whiteboard-rich-items` ("no raw HTML in item markdown"),
   `261108-feat-ws-manual-shared-command-syntax` (its inline-tag mechanism
   rejected a raw-HTML parser plus sanitizer), and
   `261107-feat-ws-whiteboard-now-markdown` ("every other board field keeps
   raw HTML shown as text").
2. **Only the block fields change (announced, (47) ok).** `wb:title` keeps
   its one line of inline markdown with the allowlisted inline tags: the
   playbook defines the title as one line of inline markdown, and (43) and
   (44) are about option bodies.
3. **One sanitizer (technical, (48) ok).** Item fields reuse the existing raw
   pipeline and `sanitizeReply`, over its current allowlist. Rejected: a
   second, item-specific allowlist, which would drift from the boundary that
   already passed review.
4. **Item containers clip their paint (technical, (49) ok).** The option
   card, rationale, and recommendation containers get `contain: paint`, as
   the finished reply bubble does (`sanitize.go` header): the sanitizer keeps
   inline style minus positioning, so a field could otherwise paint outside
   its box. Rejected: leaving them unconstrained.
5. **The whiteboard playbook says item fields take sanitized HTML (announced,
   (50) ok).** The item-field markdown paragraph ("any other tag shows as
   text") is rewritten in both `agents-plugin/rsrc/whiteboard/whiteboard.md`
   and `agents-plugin-wsflow/rsrc/whiteboard/whiteboard.md`, which move
   together per `ai-docs/manuals/wsflow-mirroring.md`.
6. **The ack row keeps the inline allowlist (technical, (275)).** An open
   technical or announced item's `ack` row renders `wb:why` one line per
   reason through `pageData.Inline` inside a `<label>`
   (`assets/page.tmpl:55`, `rationaleLines`, `render.go:651`); it keeps the
   inline allowlist, and only the block containers (option card, `ctx`
   rationale, recommendation) widen. Rejected: sanitized HTML per ack line,
   since block HTML inside a label is invalid and breaks the
   one-line-per-reason layout.
7. **Layout and plain-text follow the raw parse (technical,
   review-raised).** Whether a field stacks as a card or renders in the
   inline option layout (`IsBlock`, `markdown.go`) is decided from the raw
   parse of the widened fields, so an option holding only `<img>` or
   `<table>` stacks as a card and never lands inside the native option
   `<button>`; the single-paragraph unwrap beside option letters survives
   the switch to the raw pipeline. `Plain` drops tags and keeps their text,
   except that the elements the sanitizer drops with their content
   (`replyDropped`, such as `<script>` and `<style>`) print nothing; an
   image prints its `alt` text, SVG prints its text content, and a table
   prints its cell text. Footnote and SVG id prefixes are salted per
   rendered field (item id, field kind, and option letter for an option),
   so identical texts on one page, in one item or across items, never
   share a prefix.
   Rejected: keeping the block-pipeline `IsBlock`, which reads raw HTML as
   one plain paragraph; a page-wide counter, which changes ids whenever
   another item is added above.

## Constraints

- Item references (`{{12}}`), local image paths, http(s) links, and mermaid
  fences in item fields keep working as today.
- Each rendered text gets its own footnote and SVG id prefixes: the footnote
  prefix is set per text by `convertWith` (`footnoteIDPrefix`,
  `agents-plugin-tool/internal/wswhiteboard/markdown.go#L252-L268`) and the
  SVG id prefix by `sanitizeReply` (`sanitize.go#L154-L156`), both a hash of
  the text, so identical texts share them; item
  fields rendered on one page must not collide with each other or with
  replies.
- `whiteboard.query` output stays plain text
  (`261110-bug-ws-whiteboard-query-output-noise` D7): item fields print
  through `Plain` (`agents-plugin-tool/internal/wswhiteboard/inline_tags.go`),
  which today rewrites only the inline allowlist. It must cover the
  widened set by D7's rule (tags dropped, their text kept); text matching
  still runs on the stored text, and the topic body and `answer_quote` still
  print verbatim.
- Manuals: `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/skill-authoring.md` (playbook text),
  `ai-docs/manuals/wsflow-mirroring.md`. Heavy Go builds and tests follow the
  machine's remote-build note when one applies.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/rsrc/, agents-plugin-wsflow/skills/, agents-plugin-tool/internal/wsdoc/conventions/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin/hooks/, agents-plugin-wsflow/)

## Prior Decisions

- 261110-bug-ws-whiteboard-query-output-noise (2026-11-10, Decisions D7): "Query output renders board inline markup as plain text in item fields ... The topic body and answer_quote print verbatim ... Text matching (D6) runs on the stored text, unchanged." — bearing: constrains
- 261109-feat-ws-whiteboard-settled-row-expand (2026-11-10, Result bf509a9f8): "srow wraps <details class=\"sx\" data-key=\"item:<ID>\"> when the item is Expandable ... the body holds ctx then the shared opts sub-template." — bearing: constrains
- 261107-feat-ws-whiteboard-now-markdown (2026-11-07, Result ed5d4779d): "The dashboard sets HTML only for a finished reply ... The finished bubble has contain: paint, and reply SVG is capped at max-width: 100%." — bearing: supports
- ed5d4779 (2026-11-07, commit): "Markdown-resolved links and images in a reply are recorded in a per-render table and marked with data-wbref=<index>; the sanitizer takes the URL from the table, never the markup." — bearing: constrains
- ed5d4779 (2026-11-07, commit): "SVG ids are prefixed (wbr-) instead of dropped so markers and gradients still work; non-SVG ids are dropped against DOM clobbering of page ids." — bearing: constrains
- 261107-feat-ws-whiteboard-now-markdown (2026-11-07, Decisions): "Inline HTML in Now only, on the board ... every other board field keeps raw HTML shown as text." — bearing: contradiction-candidate
- 261108-feat-ws-manual-shared-command-syntax (2026-11-08, commit c0f2e43b): "a dedicated goldmark inline parser ... no raw-HTML parser is registered and html.WithUnsafe stays off, so no typed HTML reaches the page. Rejected: raw-HTML parser plus sanitizer" — bearing: contradiction-candidate
- 261106-feat-whiteboard-rich-items (2026-11-06, Decisions): "No raw HTML in item markdown. goldmark's raw HTML stays off ... Rejected: allowing HTML in item fields (blurs the escaping boundary with verbatim answer_quote text and adds a second place a script can enter)." — bearing: contradiction-candidate

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wswhiteboard/markdown.go, sanitize.go, inline_tags.go, assets/page.tmpl, assets/page.css, agents-plugin/rsrc/whiteboard/whiteboard.md, agents-plugin-wsflow/rsrc/whiteboard/whiteboard.md |
| scope.surface | cross-module | board and dashboard rendering of item fields, exported Plain used by query.go, shipped playbook text in two packages |
| scope.new_public_symbol | no | none; reuses rawMarkdown, sanitizeReply, Plain |
| scope.new_type_contract | no | no type or signature change; the accepted markup of wb:option, wb:why, wb:recommend widens per the playbook |
| scope.test_surface | existing | agents-plugin-tool/internal/wswhiteboard/markdown_test.go, sanitize_test.go, render_test.go, query_test.go, plus the playbook mirror test |
| complexity.reuse_points | confirmed | sanitizeReply sanitize.go#L143, RenderText HTML path markdown_text.go#L52-L63, Plain inline_tags.go#L217 read |
| complexity.side_effect_risk | moderate | every item field on static boards, served pages, and settled rows changes renderer; footnote and SVG id prefixes now span item fields and replies on one page |
| risk.correctness | moderate | Block unwraps a single paragraph beside option letters and resolves item refs; the raw pipeline plus sanitizer must keep both, and ack rows render wb:why through Inline per line |
| risk.fit | moderate | reverses recorded no-raw-HTML-in-items decisions and touches mirrored playbooks |
| risk.test | moderate | needs allow and drop cases across three field kinds, stray closing tags, refs, mermaid, and Plain over the widened set |
| risk.security_or_contract | high | sanitizeReply becomes the only trust boundary for lead-authored item HTML in the daemon origin with no CSP |

## Phases

### Phase 1: sanitized HTML in item fields

Implement Decisions 1-7.

Verification:

- Go tests: an option, rationale, and recommendation holding `<img>`,
  `<table>`, `<details>`, and a small SVG render as kept HTML; `<script>`,
  `<style>`, `<iframe>`, event attributes, and `javascript:` URLs are dropped;
  a stray closing tag stays inside its field; item references, images, and
  mermaid fences still render; the title still escapes tags outside the
  inline allowlist; an ack row still escapes tags outside the inline
  allowlist; the option, rationale, and recommendation containers carry
  `contain: paint`.
- Go tests: an option holding only `<img>` or `<table>` stacks as a card,
  not inside the option button; a single-paragraph option still unwraps
  beside its letter; a settled row's expanded details render the widened
  fields; two identical option texts on one page, in the same item and in
  different items, get distinct footnote and SVG id prefixes.
- Go tests: `whiteboard.query` prints an item field holding the widened
  markup as plain text, with no tags left, no `<script>` or `<style>`
  content, and image alt, SVG, and table cell text kept.
- The playbook mirror test passes for the edited whiteboard playbook.

### Result (29feba493) - 2026-11-11

Landed in 29feba493 with review fixes in 0ba8b4fc2.

- `itemField` (markdown.go) renders option, rationale, and recommendation
  text through the raw pipeline and `sanitizeReply`. Its trusted-target
  table only lives as long as one field. The salt is `<id>/option/<letter>`,
  `<id>/why`, or `<id>/recommend`, and both `footnoteIDPrefix` and the SVG id
  prefix take it. An empty salt hashes as before, so reply and topic-body
  ids are unchanged. `Block` stays the escaping pipeline that `RenderText`
  uses without HTML.
- `IsBlock` reads the raw parse. A single paragraph stays inline only if
  every raw tag in it is phrasing content (`phrasingTags`). An image, table,
  SVG, div, or stray closing tag stacks the options. Titles and ack rows
  still go through `Inline` (D2, D6).
- `.opt, .ctx, .rec-line { contain: paint; }` (D4).
- `PlainHTML` (sanitize.go) prints options, recommendations, and rationale
  in `whiteboard.query`. It drops tags and keeps their text, prints an image
  as its alt text, and prints nothing for script or style. Code spans and
  code blocks stay literal (`protectCode`). `Plain` still handles titles,
  links, labels, and answers, which the page shows as text (D7).
- Playbook item-field and query paragraphs rewritten (D5). The rsrc
  manifest and the wsflow and Pi mirrors were synced.
- Verification: `ws-rgo go test ./...` (agents-plugin-tool) passed after
  29feba493. After 0ba8b4fc2, `wswhiteboard`, `wsdashboard`, `wsrsrc`
  (`-count=1`), and `mcp` passed. One `mcp` run hit an unrelated failure in
  `TestHostedReconnectAdoptsOnlyLiveSamePIDRecords`, which passed `-count=3`
  in isolation and in the following full `mcp` run.
  `python3 -m unittest discover agents-plugin-wsflow/tests` passed.
- Decisions beyond the ticket:
  - `PlainHTML` is a new function rather than a widened `Plain`, so that
    titles keep text like `Vec<T>`.
  - The settled row's picked chip renders `FirstLine(PlainHTML(option))`,
    so an image option shows its alt text, not escaped tags.
  - In form.js, a click on a `<summary>` inside an option no longer picks
    the option.
  - Lite review fix: item fields keep `class`, so form.js, live.js, and
    dashboard.js now select controls by data-* or aria-pressed attributes
    and by textarea, button, or label elements. A field that reuses
    `class="note"` used to break reply building.
  - Item field HTML is re-serialized by x/net/html (`<br/>`, `<img .../>`,
    `&#34;` in attributes), and test expectations follow that form.
