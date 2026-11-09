---
title: Whiteboard settled rows expand to show the original question
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 8f45bb45ad42631b
sage-review-completeness-reviewed: 8f45bb45ad42631b
completed: 2026-11-10
---

# Whiteboard settled rows expand to show the original question

## Background

A settled whiteboard item (answered, assumed, withdrawn, or superseded) renders
as one compact row: the ID, the title, the picked option's first line, the
lead's reading, and the user's quoted answer
(`agents-plugin-tool/internal/wswhiteboard/assets/page.tmpl`, template
`srow`). Once an item settles, its other options, the recommendation, and the
context (`wb:why`) are no longer visible anywhere on the page, although the
store keeps them. The user asked (board, 2026-11-09) for a way to unfold a
settled policy question and see what the original choices were; assumed items
are the same case.

## Decisions

1. **Unfolding shows the whole original item (board 143 ok).** An expanded
   settled row shows the fields an open item card shows, all already stored:
   every option, lettered a, b, c, with the picked one marked (none marked
   when the item has no pick, as a withdrawn or superseded item may not); the
   recommendation; and the context or reasoning (`wb:why`). Links stay where
   the compact row already prints them and are not repeated in the body. No
   new stored field and no store or command change.
2. **Server-rendered `<details>`, no JavaScript toggle (board 144 ok).** The
   settled row holds a `<details>` whose `<summary>` is today's compact row
   and whose body is the original item. The dashboard page, the file page, and
   the embed snapshot all come from the same Go renderer, so the row unfolds
   the same way on all three without script; the settled list is already a
   `<details class="settled">`. Rejected: a script toggle (needs the page
   scripts, which an embed snapshot may not run).
   Row shape (design review, auto-confirmed): the row element stays a
   `div.srow` wrapping the keyed `<details>`; as today, only an assumed row
   carries `data-id` (form.js's reply builder reads `.note` from every
   `.srow[data-id]`, and the dashboard's answer selectors match it, so a
   `data-id` on any other settled row breaks both). The assumed row's pen
   and `.note` sit after the `<details>` as its siblings, so they stay visible
   while the row is collapsed and are outside the toggle path. Rejected:
   `<details class="srow">` as the row itself (everything outside the summary
   hides while closed, so the pen and note would vanish from a collapsed
   assumed row, regressing 53d5934b's always-available pen) and the pen inside
   the `<summary>` (needs click and key suppression; a space typed in a
   textarea inside a summary toggles the row).
3. **Settled technical and announced items unfold too (board 145 a).** Their
   expanded body shows their reasoning or citation (`wb:why`), the same
   mechanism; links stay in the compact row (Decision 1). Rejected: policy items only (the reasoning behind a settled
   technical or announced item is just as lost once it settles).

4. **Settled options render static (design review, auto-confirmed).** The
   card's option and recommendation markup is factored into one shared
   sub-template with a static mode: no `data-opt`, no `aria-pressed`, not a
   `<button>` or `role="button"`, the pick shown by a class, no pointer
   cursor or hover state. Rejected: reusing the interactive card markup as
   is: dashboard.js matches `.opt[data-opt]` and
   `.opt[data-opt][aria-pressed="true"]` inside `.srow[data-id]` (readAnswers,
   writeAnswers, readItem, setItem, about `dashboard.js` L287-L314 and
   L551-L570), so a marked settled option would read as a pending draft
   answer. fb2ba4eb removed an earlier `.opt.static` variant; its styling
   (`.opt.static.picked`, e7fc38d5) is the precedent for the look.
5. **Open state keyed by item (design review, auto-confirmed).** The row's
   `<details>` carries `data-key="item:<ID>"`, which collides with neither the
   topic doc's `doc:<name>` keys nor the tree's path keys. The dashboard
   already restores any `details[data-key]`; `live.js`'s restore and toggle
   selector (`details.panel[id], details.tnode[data-key]`, about L161) widens
   to include it. Because the key is the item ID, a row the user opened stays
   open across an update even when that row itself changed.

## Constraints

- Read `ai-docs/manuals/shipped-surface-boundary.md` before editing (path row
  for `agents-plugin-tool/`).
- A row with nothing more to show (no options and no `wb:why`; links never
  count, since they stay in the compact row) stays a plain row, not an empty
  `<details>`.
- The assumed row's note pen and correction textarea (`form.js`
  `.note-toggle`, `.srow[data-id]`) keep working: clicking the pen or typing
  in the note must not toggle the row, and `form.js` still finds the note
  from the row (`closest(".card, .arow, .srow")`).
- Expanded content uses the same block-markdown rendering, `{{nn}}` reference
  resolution, and sanitization as the open item card; reuse the card's
  option and recommendation rendering rather than a second copy, through the
  static mode of Decision 4.
- A changed settled row still keeps its change highlight and new-dot
  behavior; the live updates must not collapse a row the user has opened on
  every re-render if the row itself did not change. Neither updater morphs:
  the static page's `live.js` reloads the whole page on a change and restores
  open state only for `details.panel[id]` and `details.tnode[data-key]` from
  sessionStorage (`agents-plugin-tool/internal/wswhiteboard/assets/live.js#L5-L13`,
  `#L25-L28`), and the dashboard swaps the board region and restores open
  state only for a `details` carrying `data-key` or a panel id
  (`agents-plugin-tool/internal/wsdashboard/assets/dashboard.js#L72-L91`; the
  topic doc's `details.tdoc data-key="doc:..."` in `page.tmpl#L27` already
  rides that path).
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Never touch the user's dashboard daemons on ports 29417 / 45743; manual
  checks use a dev daemon with isolated `WS_CACHE_HOME` / `WS_CONFIG_HOME`.

## Prior Decisions

- fb2ba4eb (2026-11-09, commit): "Implemented by reusing form.js's existing note-toggle and reply builder (an assumed srow carries data-id, the pen, and a .note), not a new control" — bearing: constrains
- 53d5934b (2026-11-09, commit): "User request: assumed items count as settled and render as a settled row with a small marker ((92)-(94) ok), plus a note pen on assumed rows even in closed topics ((95))." — bearing: constrains
- 9ae1de87 (2026-11-09, commit): "C6 (pill.js/form.js tests) stays a follow-up per (148) a." — bearing: constrains
- e7fc38d5 (2026-11-08, commit): "Drop `.opt.static.picked` from the dimmed rule; only the settled-row `.pick` chip gets the 1px 40% ring and weight 500. `.opt.static.picked` returns to the original strong treatment." — bearing: constrains
- b0338cde (2026-11-08, commit): "root cause was the settled-row .pick inline-flex chip emitting text runs as separate flex items, not the .opt grid the plan suspected." — bearing: constrains
- 261106-feat-whiteboard-rich-items (2026-11-06, Decisions): "When any option of an item contains block markdown, all of that item's options render as stacked vertical cards; items with only short one-line options keep the current inline layout." — bearing: supports
- 6881738e (2026-11-06, commit): "A lone paragraph renders without <p> so short options and the recommendation stay inline beside their letter/star." — bearing: supports
- 261109-bug-ws-whiteboard-patch-cannot-change-item-class (2026-11-09, Decisions): "`wb:patch` accepts `class` only on an item that is open ... This follows the playbook rule \"Options change only on an open item\"" — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wswhiteboard/assets/page.tmpl, assets/page.css, assets/form.js, assets/live.js, render_test.go; possibly agents-plugin-tool/internal/wsdashboard/assets/dashboard.js for open-state keys |
| scope.surface | internal | template and asset markup only; itemData and pageData are unexported, no exported Go symbol changes |
| scope.new_public_symbol | no | none; any helper such as a has-more-to-show check lands on unexported itemData in render.go |
| scope.new_type_contract | no | none; no store, command, or View field change per Decision 1 |
| scope.test_surface | existing | agents-plugin-tool/internal/wswhiteboard/render_test.go TestRenderPageAssumedItemIsASettledRow L474, TestRenderPageQuietAndCollapsedSettled L529 |
| complexity.reuse_points | confirmed | page.tmpl policy template L42-L47 options and rec-line, ctx L38, links L40, details.tdoc data-key open-state path L27 |
| complexity.side_effect_risk | moderate | srow markup is shared by the static page, embed snapshot, and dashboard board region, and form.js reply builder reads .srow[data-id] .note |
| risk.correctness | moderate | pen button and note textarea must sit outside the summary toggle path, and the plain-row fallback needs a correct nothing-to-show test |
| risk.fit | low | follows the existing details.settled and details.tdoc patterns in page.tmpl |
| risk.test | moderate | toggle and open-state preservation are JS behavior with no JS test harness, pill.js/form.js tests deferred per 9ae1de87; exact-markup Go tests need updating |
| risk.security_or_contract | low | reuses the card's Page.Block sanitized rendering; no store, command, or protocol change |

## Phases

### Phase 1: Expandable settled rows

- `page.tmpl` `srow`: inside `div.srow` (`data-id` on assumed rows only), a `<details
  data-key="item:<ID>">` with the compact content as `<summary>` and an
  expanded body holding the options (static mode, picked one marked), the
  recommendation, and `wb:why`, for every settled class; the assumed row's
  pen and `.note` follow the `<details>` as siblings; keep a plain row when
  there is nothing to expand. Factor the card's option and recommendation
  markup into a shared sub-template with the static mode (Decision 4).
- `page.css`: summary marker and expanded body styling in the existing
  tokens, light and dark; the expanded option list visually matches the open
  card's options.
- `form.js` / `live.js` (and `dashboard.js` only if its `data-key` restore
  needs it): keep the assumed row's note pen working inside the new
  structure; widen `live.js`'s open-state selector to the `item:` keys so an
  opened row stays open across live re-renders (Decision 5).
- Tests in `render_test.go`: a settled policy item renders all its options
  with the picked one marked, the recommendation, and `wb:why` inside the
  row's `<details>` with no `data-opt` or `aria-pressed` on any option; a
  withdrawn policy item with no pick renders all options with none marked; a
  settled technical item renders its `wb:why`; a row with nothing extra stays
  a plain row; links appear once, in the compact row; the assumed row's pen
  and note sit outside the `<details>`; the `<details>` carries
  `data-key="item:<ID>"`; existing settled-row tests updated.
- Verify in a headless browser against a dev daemon: open a settled policy
  row and an assumed row, use the assumed row's pen on a collapsed and on an
  expanded row, and check that the dashboard chip for that item carries only
  the note, no option letter from the static options; with a row open, make an unrelated board change and check the row
  stays open on the dashboard page (region swap) and on the static file page
  (`live.js` reload); check the embed snapshot unfolds the same way.

### Result (bf509a9f8) - 2026-11-10

- `page.tmpl`: `srow` wraps `<details class="sx" data-key="item:<ID>">` when the
  item is `Expandable` (options or `wb:why`; links never count); the compact
  text moved into a `stext` sub-template used as the summary, and the body holds
  `ctx` then the shared `opts` sub-template. The assumed row's pen and `.note`
  follow the `<details>` as siblings; only an assumed row carries `data-id`.
  `opts` is the card's option and recommendation markup with a `Static` mode
  (`opt static`, `picked` class, no `data-opt`/`aria-pressed`/button/role).
- `render.go`: `itemData.Static`, `AsStatic`, `Expandable` (unexported).
- `page.css`: caret summary, indented body, `.opt.static` look (settled quiet
  ring on the pick, unpicked dimmed), light and dark via existing tokens;
  `.srow b` / `.srow.void b` narrowed to `.stext > b` so the void strike-through
  does not reach the expanded body.
- `live.js`: open-state restore selector widened to `details.sx[data-key]`.
  `dashboard.js` unchanged (it already restores any `details[data-key]`).
- Tests: new `TestRenderPageSettledRowExpands` (full and embed); updated
  `TestRenderPageAssumedItemIsASettledRow`, `TestMarkdownPickedChipShowsFirstLine`
  (first-line check scoped to the summary); embed size budget raised 35% -> 40%
  because settled rationales now ride in the snapshot (32.7% -> 35.2%).
- Verification: `go vet` and `go test -count=1 ./internal/wswhiteboard/
  ./internal/wsdashboard/ ./internal/wsi18n/` ok; `node --check live.js` ok.
  Headless Chromium on the static page and embed: rows open and close, the
  assumed pen works on collapsed and expanded rows without toggling (typed
  space included), the reply line carries only `(76) note: ...`, an opened row
  stays open across a `live.js` reload with an unrelated change, and the embed
  unfolds with static options (light and dark screenshots checked).
- Not run: the dashboard-daemon checks (chip carries only the note; open row
  survives a region swap). No fixture mode seeds a board without a live
  session; code reading supports both (dashboard selectors need `data-opt`,
  `restoreOpen` covers any `details[data-key]`).
- Lite review: no Critical or Important findings.
