---
title: Whiteboard explicit item-reference syntax - replace implicit (nn) detection with {{nn}}
related:
  261030-feat-session-whiteboard-mode: follow-up (the shipped whiteboard this changes)
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 6c67f1f12fd44f30
sage-review-completeness-reviewed: 6c67f1f12fd44f30
---

# Whiteboard explicit item-reference syntax - replace implicit (nn) detection with {{nn}}

## Background

The session whiteboard (`261030-feat-session-whiteboard-mode`, merged in
394a2ad79) treats every `(nn)` inside a `links` string as an item reference:
`validateLinks` refuses an update whose link names an ID the store does not
hold, and `LinkParts` turns each matching `(nn)` into an in-page anchor
(`agents-plugin-tool/internal/wswhiteboard/store.go` `itemRefPattern` and
`validateLinks`; `render.go` `LinkParts`, which matches with its own
`linkItemPattern` at `render.go#L37-L38` over the `LinkView.Refs` that
`view.go` builds with a third copy, `refPattern`, at
`agents-plugin-tool/internal/wswhiteboard/view.go#L141` and `#L287-L295`).
Ordinary prose parentheses such as
`step (2)` therefore get refused or mislinked. The Phase 3 worker reported
this as minor; the user judged it important (2026-11-05), because a refused
batch refuses the whole update ((113)).

## Decisions

- **Item references use an explicit syntax, `{{nn}}` (user decision,
  2026-11-05).** A plain `(nn)` anywhere is literal text. The syntax applies
  in every lead-authored text the page renders - title, rationale,
  recommendation, option text, the lead's answer reading, and `links` - so
  the lead can cite another item from any text ((124)). The user's
  `answer_quote` stays literal, since it is their verbatim words.
  Rejected: keeping implicit `(nn)` detection (refuses or mislinks ordinary
  parentheses).
- **Only the strict token `{{[0-9]+}}` is a reference ((120), user
  refinement 2026-11-05).** Any other `{{...}}` text, such as `{{my-tag}}`,
  renders verbatim, so a document quoted verbatim keeps its braces.
- **References are cosmetic and never refused ((121) rejected, 2026-11-05).**
  `validateLinks` stops checking item IDs, so no update is refused over a
  reference. Rejected: refusing a `{{nn}}` that names an ID the store never
  held (strictness buys nothing for a display feature).
- **A `{{nn}}` resolves three ways (technical decision (123)).** An item on
  the page renders as the label `(nn)` in an in-page anchor; an item the
  store holds but the page omits (an archived settled item) renders as plain
  `(nn)`; an ID the store does not hold renders verbatim as `{{nn}}`.
  Rejected: rendering the raw braces for a resolved reference (the board's
  ID display form stays `(nn)` everywhere), and a two-way rule (on page or
  verbatim), because an archived item is a real, queryable reference. The
  store cannot tell an ID evicted past `ArchiveCap` from a typo, so both
  stay verbatim.
- **The `whiteboard` playbook shows the syntax with a literal example
  (technical decision (122)).** The worker first verifies that the rsrc
  renderer leaves a `{{<digits>}}` token untouched (it substitutes
  `{{.Name}}` placeholders) and escapes the example in the playbook only if
  it does not. Rejected: a different token (the user chose `{{nn}}`).

## Constraints

- Manuals: `ai-docs/manuals/ws-mcp.md`, `ai-docs/manuals/skill-authoring.md`,
  `ai-docs/manuals/wsflow-mirroring.md`,
  `ai-docs/manuals/shipped-surface-boundary.md` (the `whiteboard` playbook and
  its wsflow and Pi copies change).
- Out of scope: any other whiteboard minor from the 261030 Phase 3 Result.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/rsrc/, agents-plugin-wsflow/skills/, agents-plugin-tool/internal/wsdoc/conventions/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin/hooks/, agents-plugin-wsflow/)

## Prior Decisions

- 82b10a28 (2026-11-05, commit): "User judged the whiteboard's implicit (nn) detection in links (refuses or mislinks prose like 'step (2)', and a refusal fails the whole batch) important, and chose an explicit {{nn}} syntax covering body text too" — bearing: supports
- 3e99f523 (2026-11-04, commit): "Minors left, reported as unresolved: ... (nn) link check catching innocuous parentheses, ..." — bearing: supports
- f98d8e83 (2026-11-04, commit): "links are validated only when supplied in the batch (validating all items would refuse every update after archive eviction)" — bearing: constrains
- f98d8e83 (2026-11-04, commit): "The inner payload schema has one source: refusals quote the rendered whiteboard playbook's ## Payload section instead of a Go copy, so schema text cannot drift from the playbook." — bearing: constrains
- f98d8e83 (2026-11-04, commit): "Knob-on text is applied code-side in printPlaybook (the prefer_subagent precedent) with replace-or-append fallback, so rsrc mirrors stay byte-identical" — bearing: constrains
- dbb9a4cd (2026-11-04, commit): "A noted acknowledgement emits '(nn) ok, note: <text>' when checked and '(nn) note: <text>' when not, reusing the existing segment grammar rather than adding a new verb." — bearing: constrains
- 261030-feat-session-whiteboard-mode (2026-10-30, Decisions): "Persisted item store with rotation (user decision (48a)). Discussion items and other asked items persist in a server-side store with server-minted IDs, statuses, and the user's verbatim answer quote." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wswhiteboard/store.go, render.go, view.go, assets/page.tmpl; agents-plugin/rsrc/whiteboard/whiteboard.md with agents-plugin-wsflow and agents-plugin-pi copies and three rsrc/manifest.json files |
| scope.surface | public-interface | whiteboard.update refusal behavior and the shipped whiteboard playbook Payload text change |
| scope.new_public_symbol | no | none; the {{nn}} token is text syntax, not a symbol |
| scope.new_type_contract | no | payload schema fields unchanged; only link and body text interpretation changes |
| scope.test_surface | existing | agents-plugin-tool/internal/wswhiteboard/store_test.go#L440 asserts the refusal that must go; render_test.go TestRenderPageLinkAnchors; wsrsrc/wsflow_mirror_test.go and pi_mirror_test.go |
| complexity.reuse_points | confirmed | render.go LinkParts and linkPart with the page.tmpl links template; title, rationale, and recommendation have no part-splitting path yet |
| complexity.side_effect_risk | moderate | three duplicated (nn) patterns in store.go, render.go, view.go must all change, and body text rendering moves from plain escaped text to split parts |
| risk.correctness | moderate | three-way resolution needs store membership as well as on-page state, while LinkView.Refs today carries only parsed IDs |
| risk.fit | low | decisions are settled and the change stays inside the existing wswhiteboard package and playbook mirrors |
| risk.test | moderate | an existing refusal test flips, new cases are needed for body fields and archived IDs, and the rendered playbook example must be checked against wsrsrc substitution |
| risk.security_or_contract | moderate | anchors injected into user-supplied body text must keep html/template escaping, and whiteboard.update stops refusing a class of input |

## Phases

### Phase 1: Explicit reference syntax

Replace the `(nn)` matching (`store.go` `itemRefPattern`, `render.go`
`linkItemPattern`, `view.go` `refPattern`) with the strict `{{[0-9]+}}`
token in rendering, drop the item-ID check from `validateLinks`, extend
parsing to title, rationale, recommendation, option text, and the answer
reading, and update the `whiteboard` playbook's Payload text (`agents-plugin/rsrc/whiteboard/whiteboard.md#L62-L63`, which today says a `(nn)` naming no stored item is refused) with its mirrors and
manifest hashes.

Verification:

- a link or body text containing `step (2)` is accepted and renders
  literally;
- `{{nn}}` naming an item on the page renders as an anchored `(nn)`, one
  naming an archived item as plain `(nn)`, and one naming an ID the store
  does not hold as verbatim `{{nn}}`; no such update is refused;
- `{{my-tag}}` and other non-digit brace tokens render verbatim;
- the rendered `whiteboard` playbook shows the `{{nn}}` example intact;
- each parsed field resolves references the same way, not only `links`;
- body text carrying HTML next to a `{{nn}}` still renders escaped;
- `TestApplyLinksReferToExistingIDs` (`store_test.go`) is inverted to assert
  acceptance and moved to `{{nn}}`; the `validateLinks` ID check and its
  call site are removed, not left as no-ops;
- wsflow mirror and Pi tree drift tests pass.

The Result notes that links already stored as `(nn)` in a live session lose
their anchors after the upgrade (cosmetic; the store is session-scoped).
