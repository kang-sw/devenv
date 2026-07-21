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

## Delivery-Mode Spectrum (decided direction)

Decided: **PWA install is the chosen first approach.** Effort vs. coverage,
ordered from lowest effort to highest:

- **PWA install** (`display-mode: standalone`) - very low effort (~1 day).
  Removes the browser tab/address-bar chrome entirely, so tab-management
  shortcuts (Ctrl+T, Ctrl+Tab, Ctrl+1..9) become non-applicable - there is no
  tab strip left for them to act on. Ctrl+R (reload) still works. Class-A
  shortcuts remain suppressible the same way, via a `keydown` handler. Does
  **not** block Ctrl+W: it still closes the app window, since that binding is
  browser-reserved and uninterceptable outside fullscreen regardless of
  delivery mode.
- **+ Keyboard Lock API** (`navigator.keyboard.lock()`) - near-zero
  incremental effort on top of the PWA install. Captures residual reserved
  keys (Ctrl+W, Esc), but only while the window is in fullscreen, and only in
  Chromium-based browsers.
- **Tauri/Electron wrapper** - large effort (weeks, plus ongoing packaging,
  signing, and auto-update maintenance). The only path to full keyboard
  sovereignty in a normal windowed (non-fullscreen) app. Since the ws daemon
  is already Rust, Tauri is the better-fit option of the two if this is ever
  pursued, over Electron. Explicitly **out of scope** for this ticket -
  tracked only as a larger future delivery-mode decision, to revisit if the
  PWA path proves insufficient.

Recommended order: PWA install first, Keyboard Lock as a cheap fullscreen-only
follow-up, Tauri/Electron deferred indefinitely unless the first two are
proven insufficient.

## Phases

### Phase 1: PWA installability

Add a web app manifest (name, icons, `start_url`, `display: standalone`) and
the minimal service worker needed to satisfy installability, so Chrome/Edge
offers "Install app" from the served origin (`127.0.0.1:4300` is a secure
localhost context and qualifies). Optionally surface an in-app install
suggestion by listening for the `beforeinstallprompt` event.

**Verification**: the installed standalone window shows no tab strip or
address bar; tab-management shortcuts (Ctrl+T, Ctrl+Tab, Ctrl+1..9) have
nothing to act on; Ctrl+R still reloads the installed app.

### Phase 2: Class-A keydown suppression

Add a global `keydown` interceptor (capture-phase, installed once near the
`App` component - see `App.tsx:437`) that calls `preventDefault()` for the
catchable browser shortcuts the dashboard wants blocked (e.g. Ctrl+S/P/F/G/
D/O, zoom Ctrl+Plus/Minus/0), while explicitly whitelisting Ctrl+R so it
keeps working. The exact block/allow set beyond that whitelist is an
implementation-time decision, not decided by this ticket. Works identically
in both plain-tab and installed modes. Reference precedent for the
ctrl-combo detection + `preventDefault` idiom already in this file:
`App.tsx:8343-8400` (`keydownFallback`).

**Verification**: with the interceptor active, each targeted class-A
shortcut is suppressed (no save dialog, no browser find bar, no zoom, etc.)
in both a plain browser tab and the Phase 1 installed PWA, while Ctrl+R
still reloads.

### Phase 3 (optional/deferred): Residual reserved keys

Evaluate fullscreen + the Keyboard Lock API (`navigator.keyboard.lock()`)
for Ctrl+W/Esc only if Phases 1-2 prove insufficient in practice. Tauri/
Electron remains out of scope per the Delivery-Mode Spectrum above and is
not part of this phase.

**Verification**: deferred - no action required unless a future session
picks this phase up; if picked up, verify Ctrl+W/Esc suppression in
fullscreen on a Chromium-based browser only, with graceful no-op fallback
elsewhere.

## Spec Impact

None yet - no existing spec stem documents dashboard delivery mode (PWA
manifest/installability) or a keyboard-shortcut-suppression contract.
Contract-first spec: yes for Phase 1's installability surface (manifest,
`display: standalone`) once implemented, since it changes how the dashboard
is served/consumed; Phase 2's keydown suppression set is expected to be
finalized and spec-addressed alongside or shortly after Phase 1's spec
entry, per this ticket's `ready/` gate.
