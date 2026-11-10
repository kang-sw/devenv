---
related:
  261108-feat-ws-tracker-wiki-git-branch: "introduced the wiki editor and wiki: links this guard protects"
title: "Dashboard: wiki navigation drops an unsaved wiki edit without asking"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 92ea43f3cd795b09
sage-review-completeness-reviewed: 92ea43f3cd795b09
completed: 2026-11-10
---

# Dashboard: wiki navigation drops an unsaved wiki edit without asking

## Background

Reported as a minor by the tracker wiki Phase 2 worker (2026-11-09) and kept
out of the 0.48.13 gate fixes. In the tracker page, a click on a rendered
`wiki:<name>/<path>` link (`a[data-wiki][data-page]`) goes to `wkOpenLink`
(`agents-plugin-tool/internal/wsdashboard/assets/tracker.js`, document click
handler near L4105), which calls `wkOpenPage` (near L1442); `wkOpenPage`
sets `st.edit = null` unconditionally. When the wiki panel holds an unsaved
edit (a link followed from the editor's preview, from an item window, or from
a comment while a page is being edited), the draft is replaced by the target
page with no prompt.

The panel has no leave-with-unsaved-edit guard anywhere: switching wiki
(`wk-pick`, near L1513) and starting a new wiki (`wk-newwiki`, near L1514)
also null `st.edit` silently. The dashboard assets use no `confirm()` today.

## Decisions

1. **One guard runs before every navigation that would drop a dirty wiki
   edit: `wkOpenPage` (which covers `wkOpenLink` and page-tree opens),
   `wk-pick`, and `wk-newwiki`.** An edit is dirty when its text differs from
   what it started from (`edit.text !== edit.orig`; a new page is dirty once
   its text differs from the seeded heading, so the edit records that seed
   as its baseline). Declining keeps the editor, the draft, the current
   page, and the item window untouched, so in `wkOpenLink` the check runs
   before `closeItemWindow`; accepting proceeds as today. A navigation that
   drops nothing (for example `wk-pick` of the current wiki) and the
   reopen after an intentional save or create do not prompt.
2. **The guard asks with the browser's `window.confirm` and a new i18n
   string,** over an in-page modal - no modal confirm exists in the
   dashboard to reuse, and a blocking native confirm is the smallest correct
   fix. Rejected: silently keeping the draft in a side store (new state the
   user cannot see).
3. **Explicit discards stay prompt-free:** the editor's cancel button, Esc
   in the editor, and taking "theirs" in a conflict already express the
   intent to drop the draft.

## Constraints

- New i18n keys go into every table under
  `agents-plugin-tool/internal/wsi18n/tables/` (the package tests require
  key parity).
- Browser checks use an isolated dev daemon (`WS_CACHE_HOME` /
  `WS_CONFIG_HOME`); never the user's daemons on ports 29417 or 45743.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)

## Prior Decisions

- 261108-feat-ws-tracker-wiki-git-branch (2026-11-09, commit 1a62e9e3): "the one open Minor (a wiki: link from the editor preview or the item window drops an unsaved edit without asking) is recorded in the Result" — bearing: supports
- b40bdc1c (2026-11-10, commit): "A minor from the tracker wiki worker's report, deferred during the 0.48.13 gate; captured so it is not lost with the session agenda." — bearing: supports
- 8f0adc0f (2026-11-09, commit): "fixed although minor, because Esc on the toggle closed the whole item window and dropped an unsaved edit, against the ticket's 'keeping each place's save and cancel behavior and keys'" — bearing: constrains
- 9abd5fd9 (2026-11-09, commit): "A wiki: link to a page that is not in the wiki opens the new-page editor at that path (the page read's 404 does the same), as the mockup's dashed 'missing' link promised." — bearing: constrains
- 389b387f (2026-11-09, commit): "stale responses for the live state re-pull so a quick page switch during a slow read still lands on the right page." — bearing: constrains
- 261109-feat-ws-tracker-shared-markdown-editor (2026-11-09, Decisions): "a save naming an oid whose staged copy expired ... is refused by the ledger; the editor shows that refusal as a save error and keeps the draft" — bearing: constrains
- c1ade9b1 (2026-11-10, commit): "js.tk.ev.assign.topic was added (not in the brief) ...; its translations are the existing js.tk.ev.assign text minus the completion slot." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsdashboard/assets/tracker.js, agents-plugin-tool/internal/wsi18n/tables/*.json (11 tables) |
| scope.surface | internal | browser-side handlers wkOpenPage, wk-pick, wk-newwiki in tracker.js; no exported Go symbol changes |
| scope.new_public_symbol | no | none; one new js.tk.wk.* i18n key, not an exported symbol |
| scope.new_type_contract | no | none; reuses the existing st.edit shape { text, orig, isNew } at tracker.js#L1125 |
| scope.test_surface | existing | agents-plugin-tool/internal/wsdashboard/tracker_wiki_test.go, agents-plugin-tool/internal/wsi18n/keys_test.go key sweep |
| complexity.reuse_points | confirmed | edit.orig set in wkStartEdit tracker.js#L1461-L1462 and conflict re-edit tracker.js#L1542 |
| complexity.side_effect_risk | low | guard is confined to three navigation entry points; wkOpenPage is also called by wkCreate tracker.js#L1481 and wk-open tracker.js#L1517 |
| risk.correctness | moderate | new pages start with orig '' and a seeded heading tracker.js#L1461, so a plain text !== orig check would mark every new page dirty; conflict re-edit sets orig to theirsText |
| risk.fit | low | follows existing handler style; window.confirm is new to the assets but chosen in Decision 2 |
| risk.test | moderate | the prompt behavior is only observable in a headless-browser walk; Go tests cover key parity, not the guard |
| risk.security_or_contract | low | no API, route, or storage contract change |

## Phases

### Phase 1: guard wiki navigation

Implement Decisions 1-3 in `tracker.js` and the i18n tables. Verification:
`go test ./internal/wsdashboard/... ./internal/wsi18n/...` under
`agents-plugin-tool`, plus a headless-browser check on an isolated daemon:
with a dirty edit, following a `wiki:` link, switching wiki, and starting a
new wiki each prompt; declining keeps the draft; with a clean editor none of
them prompt; following a `wiki:` link from an item window and declining
leaves the item window open; saving or creating a page and then navigating
does not prompt. The new string is one `js.tk.wk.*` key in every table.

### Result (38079f5) - 2026-11-10

- `tracker.js`: `wkLeaveOk` asks with `window.confirm(wsT("js.tk.wk.leave"))`
  when `wkDirty(wk().edit)`, before `wkOpenPage` (tree opens and the
  existing-wiki branch of `wkCreate`), `wkOpenLink` (checked before
  `closeItemWindow`), `wk-pick` (only when the pick changes wiki), and
  `wk-newwiki`. `wkOpenPage` became a guarded wrapper over the unchanged body,
  now `wkGoPage`, so `wkOpenLink` asks once. Cancel, Esc, and taking theirs do
  not ask; a save clears the edit, so the reopen after it does not ask.
- Dirty baseline: a new page records its seeded heading in a new `edit.seed`
  field, and `wkDirty` compares against `seed` when present, otherwise `orig`.
  `orig` stays `''` for a new page because the refused-save view uses it as
  the merge base. Edit again compares against theirs text.
- `js.tk.wk.leave` added to all eleven tables; the mockup `i18n-en.js`
  regenerated (`TestTrackerMockupI18nIsCurrent`).
- Test: `TestTrackerWikiNavigationAsksBeforeDroppingAnEdit` runs the real
  `wkShow`..`wkStartEdit` and `wkAct` blocks under node: declined/accepted/
  clean/no-edit for link, tree open, pick, and new wiki; untouched vs typed new
  page; item window kept on decline and closed once on accept; pick of the
  shown wiki; Edit again; after save. Disabling the guard fails it.
- Verification: `go test ./internal/wsdashboard/... ./internal/wsi18n/...`
  ok; `go vet ./internal/wsdashboard/` clean; `node --check tracker.js` ok.
  Headless Chromium on an isolated dev daemon (scratch `WS_CACHE_HOME` /
  `WS_CONFIG_HOME`, port 39561): 22/22 checks pass, covering every case the
  phase lists, plus cancel and Esc not asking. In the probe, wiki: links were
  injected anchors clicked through the document handler, which is the same
  path a rendered link takes.
- Review (lite): clean.
