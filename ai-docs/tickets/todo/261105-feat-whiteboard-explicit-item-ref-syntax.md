---
title: Whiteboard explicit item-reference syntax - replace implicit (nn) detection with {{nn}}
related:
  261030-feat-session-whiteboard-mode: follow-up (the shipped whiteboard this changes)
---

# Whiteboard explicit item-reference syntax - replace implicit (nn) detection with {{nn}}

## Background

The session whiteboard (`261030-feat-session-whiteboard-mode`, merged in
394a2ad79) treats every `(nn)` inside a `links` string as an item reference:
`validateLinks` refuses an update whose link names an ID the store does not
hold, and `LinkParts` turns each matching `(nn)` into an in-page anchor
(`agents-plugin-tool/internal/wswhiteboard/store.go` `itemRefPattern` and
`validateLinks`; `render.go` `LinkParts`). Ordinary prose parentheses such as
`step (2)` therefore get refused or mislinked. The Phase 3 worker reported
this as minor; the user judged it important (2026-11-05), because a refused
batch refuses the whole update ((113)).

## Decisions

- **Item references use an explicit syntax, `{{nn}}` (user decision,
  2026-11-05).** Only an explicit reference is validated and linked; a plain
  `(nn)` anywhere is literal text and is never refused. The syntax applies in
  item body text (title, rationale, recommendation) as well as in `links`,
  so the lead can cite another item from any text.
  Rejected: keeping implicit `(nn)` detection (refuses or mislinks ordinary
  parentheses).

## Open Decision Queue

Last ID: (122)

(120) [open] [technical] A `{{nn}}` reference renders as the visible label `(nn)` wrapped in an in-page anchor when the item is on the page, else as plain `(nn)` - over rendering the raw braces - the board's ID display form stays `(nn)` everywhere.
(121) [open] [technical] A `{{nn}}` naming an ID the store never held is refused in the batch validation, and one naming an evicted item renders as plain `(nn)` - over accepting every reference silently - keeps the current typo guard, now scoped to deliberate references only.
(122) [open] [technical] The `whiteboard` playbook shows the syntax with a literal example, after verifying that the rsrc renderer leaves a `{{<digits>}}` token untouched (it substitutes `{{.Name}}` placeholders) - over a different token - the user chose `{{nn}}`; escape it in the playbook only if the renderer does not pass it through.

## Constraints

- Manuals: `ai-docs/manuals/ws-mcp.md`, `ai-docs/manuals/skill-authoring.md`,
  `ai-docs/manuals/wsflow-mirroring.md`,
  `ai-docs/manuals/shipped-surface-boundary.md` (the `whiteboard` playbook and
  its wsflow and Pi copies change).
- Out of scope: any other whiteboard minor from the 261030 Phase 3 Result.

## Phases

### Phase 1: Explicit reference syntax

Replace `itemRefPattern` matching of `(nn)` with `{{nn}}` in validation and
rendering, extend parsing to title, rationale, and recommendation, and
update the `whiteboard` playbook's Payload text with its mirrors and
manifest hashes.

Verification:

- a link or body text containing `step (2)` is accepted and renders
  literally;
- `{{nn}}` naming a stored item renders as an anchored `(nn)`; naming an
  unknown ID is refused;
- the rendered `whiteboard` playbook shows the `{{nn}}` example intact;
- wsflow mirror and Pi tree drift tests pass.
