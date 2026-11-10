---
title: "Whiteboard item fields accept sanitized HTML through the reply sanitizer"
related:
  261110-bug-ws-whiteboard-query-output-noise: "landed first; its D7 prints item fields in whiteboard.query output as plain text through `Plain` (inline_tags.go), which knows only the inline allowlist, so this ticket extends it to the widened set"
---

# Whiteboard item fields accept sanitized HTML

## Background

Downstream leads often put images and other HTML into whiteboard item
options (policy, technical, announced) (user, 2026-11-10 whiteboard, topic
"선택지 렌더러 (별건)"). Today:

- Item block fields (option, `wb:why`, `wb:recommend`) render through
  `pageData.Block` (`agents-plugin-tool/internal/wswhiteboard/markdown.go:210`,
  templates `ctx`, `opts`, and the rec line in `assets/page.tmpl:38`, `:52-53`):
  block markdown with images and mermaid, no raw HTML. Only the allowlisted
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

## Constraints

- Item references (`{{12}}`), local image paths, http(s) links, and mermaid
  fences in item fields keep working as today.
- `sanitizeReply` gives each reply its own footnote and SVG id prefixes; item
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

## Phases

### Phase 1: sanitized HTML in item fields

Implement Decisions 1-5.

Verification:

- Go tests: an option, rationale, and recommendation holding `<img>`,
  `<table>`, `<details>`, and a small SVG render as kept HTML; `<script>`,
  `<style>`, `<iframe>`, event attributes, and `javascript:` URLs are dropped;
  a stray closing tag stays inside its field; item references, images, and
  mermaid fences still render; the title still escapes tags outside the
  inline allowlist.
- Go tests: `whiteboard.query` prints an item field holding the widened
  markup as plain text, with no tags left.
- The playbook mirror test passes for the edited whiteboard playbook.
