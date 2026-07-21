---
title: Suppress interfering browser keyboard shortcuts in ws-dashboard
---

# Suppress interfering browser keyboard shortcuts in ws-dashboard

## Background

`ws-dashboard.exe` serves the dashboard as a normal web app opened in a
browser tab. Standard browser keyboard shortcuts (Ctrl+S, Ctrl+P, Ctrl+F,
Ctrl+Plus/Minus/0, Backspace navigation, etc.) can fire while the dashboard
has focus and interfere with the dashboard UX (e.g. triggering the OS save
dialog, browser find bar, page zoom, or navigating away). The user wants
these suppressed, while keeping Ctrl+R (reload) working, since reload is
useful during dogfooding/dev iteration.

Requested during live dogfooding on 2026-07-21.

## Constraints

Browser keyboard shortcuts split into two classes with fundamentally
different interceptability from a served (non-installed) browser tab:

- **Class A - interceptable**: shortcuts the page can catch via a
  `keydown` listener + `preventDefault()` before the browser acts on them.
  Examples: Ctrl+S (save), Ctrl+P (print), Ctrl+F (find), Ctrl+G (find
  next), Ctrl+D (bookmark), Ctrl+O (open file), Ctrl+U (view source),
  Ctrl+Plus/Minus/0 (zoom), Ctrl+J (downloads), Backspace-as-back-navigation,
  etc. Ctrl+R (reload) is also in this class technically, but the ask is to
  explicitly *not* block it — it should keep working.
- **Class B - browser-reserved, NOT interceptable**: shortcuts the browser
  chrome consumes before any page-level `keydown` handler runs, regardless
  of `preventDefault()`. Examples: Ctrl+W (close tab), Ctrl+T (new tab),
  Ctrl+N (new window), Ctrl+Shift+T/W/N, Ctrl+Tab (tab switch), Ctrl+1..9
  (jump to tab N), Alt+F4. The user already knows Ctrl+W is uncatchable from
  a page context — the same root cause (browser owns these bindings ahead
  of page script) applies to this entire class.

Consequence: "block them all except Ctrl+R" is only **partially**
achievable while the dashboard runs as an ordinary served browser tab.
Class A can be intercepted and blocked (with Ctrl+R whitelisted through).
Class B cannot be blocked from that delivery mode at all — no amount of
JS-level `keydown` handling reaches it.

Fully blocking class B would require changing how the dashboard is
delivered/hosted, not just adding a handler:

- Installed PWA running in `display-mode: standalone` (its own top-level
  window, no visible browser tab strip), or
- An Electron/kiosk-style wrapper.

In either of those contexts, shortcuts that are normally browser-chrome-only
become page/window-interceptable because there is no shared tab strip
consuming them first. That is a larger scope / delivery-mode change and a
separate consideration from an in-page keydown handler — do not conflate
the two in scoping or estimation.

## Prior Art

The frontend does not currently have a global, app-wide browser-shortcut
suppressor. Two existing narrower keydown patterns in
`ws-dashboard/frontend/src/App.tsx` are relevant as precedent/starting
points, not as the implementation itself:

- `ws-dashboard/frontend/src/App.tsx:1497-1507` - a scoped `dismissOnEscape`
  keydown listener (capture phase, `document`-level) used by a
  dismiss-on-outside-click/Escape hook; shows the existing pattern for a
  document-level capture-phase listener with cleanup.
- `ws-dashboard/frontend/src/App.tsx:8343-8400` - `keydownFallback`, a
  `window`-level keydown listener scoped to an active terminal pane. It
  already inspects `event.ctrlKey || event.metaKey` plus `event.key` and
  calls `event.preventDefault()` for specific combinations (e.g. ctrl-c,
  ctrl-l, ctrl-u, ctrl-w) before forwarding raw bytes to the terminal. This
  is terminal-input-forwarding, not shortcut suppression, but demonstrates
  the ctrl-combo detection + `preventDefault` idiom already in use in this
  file.
- `ws-dashboard/frontend/src/App.tsx:437` - `export function App()`, the
  top-level component. A global suppressor (if built) would most naturally
  live as an effect near the top of this component (or a small dedicated
  hook/module alongside it), not nested inside a pane-scoped handler like
  the terminal fallback above.

No existing `beforeunload`, app-wide Ctrl+S/Ctrl+P handler, or dedicated
keybinding/shortcut module was found in `ws-dashboard/frontend/src`.

## MVP Scope (realistic)

A global `keydown` interceptor (likely a `window`/`document`-level capture
listener installed once near the `App` component) that:

- Calls `event.preventDefault()` for the class-A combinations the dashboard
  wants to suppress.
- Explicitly whitelists Ctrl+R (never `preventDefault()`s it) so reload
  keeps working.
- Leaves class-B combinations alone entirely (nothing to do — they never
  reach page script).

Exact catchable set to block vs. deliberately allow (e.g. whether Ctrl+F
find-in-page should stay usable for the user while iterating) is an
implementation-time decision, not decided by this ticket.

## Larger-Scope Option (separate consideration)

Deliver the dashboard as an installed PWA (`display-mode: standalone`) or
via an Electron/kiosk-style wrapper so that class-B shortcuts also become
interceptable/suppressible. This is a distinct, larger-scope delivery-mode
change (manifest + install flow, or a new packaging target) and should be
scoped and decided independently of the MVP keydown interceptor above.
