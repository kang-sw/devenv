---
title: "Dashboard: following a wiki: link drops an unsaved wiki edit without asking"
---

# Dashboard: following a wiki: link drops an unsaved wiki edit without asking

## Background

Reported as a minor by the tracker wiki Phase 2 worker (2026-11-09) and kept
out of the 0.48.13 gate fixes. In the tracker page, a click on a rendered
`wiki:<name>/<path>` link (the daemon marks it `a[data-wiki][data-page]`) goes
straight to `wkOpenLink` (`agents-plugin-tool/internal/wsdashboard/assets/tracker.js`,
the document click handler near L4084-L4086). When the wiki panel holds an
unsaved edit, for example a link followed from the editor's preview or from an
item window while a page is being edited, that edit is replaced by the
target page with no prompt.

Reasonable expectation: the same guard the wiki editor uses elsewhere for
leaving a dirty page (confirm, or keep the draft) applies to link navigation.

## Idea

Route `wkOpenLink` through the wiki panel's existing leave-with-unsaved-edit
check (or add one if leaving a dirty page has none yet), so following a link
asks first or keeps the draft. Verify which paths reach `wkOpenLink`
(editor preview, item window body, comments) and whether any other
navigation (page picker, back) already guards the draft.
