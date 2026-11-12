---
title: "Dashboard: an open tab keeps running the old page script after a daemon upgrade"
related:
  261112-bug-ws-dashboard-interrupt-badge-stuck-on-back-to-back-turn: the fix looked broken live because the tab ran the old script
completed: 2026-11-12
---

# Dashboard: an open tab keeps running the old page script after a daemon upgrade

## Background

Found while dogfooding a daemon-hosted lead session (2026-11-12, reply-id
`autisticcavalierharpistdairy`). The interrupt-badge fix (7eb0e7289) was merged
and the daemon replaced, yet the badge still stuck once on the open page. The
session page inlines `internal/wsdashboard/assets/dashboard.js`, so a tab
opened before the upgrade keeps the old script; the SSE stream reconnects to
the new daemon and keeps swapping fragments into the old page. Only a manual
reload picked up the fix. Peer sessions replace the daemon on every asset
merge, so this recurs often and makes fixes look broken.

The page already has the reload-or-banner pattern: `modeChanged` and `setLang`
in `dashboard.js` reload when `holdsDraft()` reports no unsent work (composer
text, picked board answers, attached images, topic notes) and show a banner
otherwise.

## Decisions

- When the page sees that the daemon serves different page assets than the
  ones it runs, it reloads when `holdsDraft()` is false and otherwise shows a
  banner asking for a reload, as `modeChanged` / `setLang` do. User board (19).
- The asset identity is a content hash of the embedded page assets (script,
  style, template). User board (20).
  Rejected: the daemon build time; a daemon upgrade that leaves the page
  assets unchanged would reload every tab.
- The daemon sends the identity in the SSE stream's first event on every
  (re)connect, and the page compares it with a value rendered into the page.
  User board (21).
  Rejected: polling a version endpoint; the reconnect after a daemon
  replacement is exactly when a mismatch appears, and it costs no extra
  request.

## Constraints

- Compatibility tier: T0 (dashboard asset content plus an additive SSE field).
- Manuals: `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/release-compatibility.md`.
- Dev asset mode (`internal/wsdashboard/devassets.go`) must not reload tabs in
  a loop; the identity there follows the served files.
- Heavy Go builds and tests run through `~/.local/bin/ws-rgo` on this machine.


## Resolution (2026-11-12)

Fixed in 978c3d79a (merge b6d4baf9d): every stream connect first sends the content hash of the page assets, and a page rendered with another one reloads, or banners while holdsDraft() holds unsent work, with a per-tab sessionStorage guard against reload loops. Covered by Go tests and a node-run page test. A live auto reload after an asset-changing daemon replacement was not confirmed; the user judged it unimportant and accepted closing (board (30)).
