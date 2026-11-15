# Selection bundle chip and memo popover (owner 1089)

## Authority and scope

MAIN 2026-11-15 09:17:33Z relays owner 1089=ok. Implement after composer1069 head 71323d8b16e957426f630047f22a60ace94df550. Reuse its mention-chip presentation for each AY selection bundle in dashboard-composer.js/dashboard-bundles.js: one compact chip, no outer blue-bar frame or always-open memo textarea. Chip body opens an anchored memo editor popover; Esc and blur save. Only the small × removes the bundle. Non-empty memo gets a subtle note indicator. Existing payload, serialization, one wb:selection per push and composer-text invariance remain unchanged. Withdrawn reveal/focus/caret and note-toggle behavior changes remain absent.

## Bounded implementation

Inspect current matching manuals and source first. Limit changes to existing bundle/composer assets, related styling, focused tests and an existing-style offline visual fixture. Reuse existing popover primitives when suitable; do not add architecture, exported APIs, new persistence, shared runtime behavior or new browser infrastructure. Source screenshot: /Users/kang-sw/.cache/ws@kang-sw-devenv/dashboard/sessions/sweatradartakingoverwrite/inbox/attachments/dfd9864d1597e3f5-1.png.

## Verification and handoff

UI-fast: scoped tests and full wsi18n if strings/template usage affected. Preserve TestSelectionNoteBrowser and TestSelectionCaptureBrowser payload/composer-text assertions; change only DOM selectors where presentation requires. Use at most one bounded independent logic review; Important/Critical block, Minor is a follow-up. No screenshot matrix or broad suites. Provide a ready-to-open fixture page or screenshot paths for MAIN's Claude pre-merge consistency check; do not start shared daemons or use port29417. Every Go invocation must explicitly cd to this checkout's agents-plugin-tool and set GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local, with isolated fixtures and warm caches. No downloads/providers/foreign keys/signals/forks. Return clean committed scoped-tested/review checkpoint before integration. Lead owns current-develop pre-head sync; only affected checks follow. Commit this unchanged plan with the implementation.
