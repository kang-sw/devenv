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

### Result

#### Edition (5075f142) - 2026-07-21

Delivered on branch `impl/pwa-installability`.

- Dashboard is now PWA-installable: hand-rolled `manifest.webmanifest`
  (name/short_name `ws-dashboard`, `display: standalone`, `start_url: "/"`,
  theme/bg `#78a9ff`/`#0f1117` sourced from existing CSS tokens), 192/512 PNG
  icons (flat glyph on palette colors), and a no-op passthrough service
  worker (`sw.js` - its fetch listener never calls `respondWith`, so there is
  no stale-asset risk and Ctrl+R reload is unaffected). `index.html` gained
  manifest/icon links; `main.tsx` registers the SW and stashes
  `beforeinstallprompt`. Four new daemon routes
  (`/manifest.webmanifest`, `/sw.js`, `/icon-192.png`, `/icon-512.png`) were
  added to `ws-dashboard/crates/daemon/src/router.rs` via a new
  `serve_root_static_file` helper, staying inside the existing owner-auth
  PROTECTED router block alongside `/assets`.
- Auth interaction: `<link rel="manifest" crossorigin="use-credentials">` so
  the browser's background manifest/icon fetch carries the owner-auth
  session cookie (a default, no-credentials manifest fetch would 401
  against this origin). Documented limitation: a token-only (no-cookie)
  auth boundary could not satisfy a browser-initiated manifest fetch this
  way, but every real browser reaching the shell already holds a
  `/pair`-issued cookie, so this is not a practical gap.
- Chosen approach: hand-rolled manifest/SW instead of `vite-plugin-pwa`, to
  avoid Workbox's default precache, which would risk serving stale assets
  and conflict with the dev-iteration/Ctrl+R concern from this ticket's
  Background.
- Commits: `5075f142` (implementation) + `75285f17` (review fit-fix:
  `sw.js` content-type aligned to `application/javascript`).
- Review: correctness/fit partitioned review. Correctness clean. Fit raised
  1 Important (sw.js content-type divergence from house `.js` convention),
  fixed in `75285f17`; remaining minors accepted by design.
- Spec: added anchor `260721-ws-dashboard-pwa-installability` to
  `ai-docs/spec/ws-web-dashboard/index.md`; updated the static-serving
  bullet in the mental model.
- Plan: `ai-docs/.plans/2026-07/21-1825-260721-phase1-pwa-installability.md`
- Verification: frontend `npm run build`, `cargo build -p
  ws-dashboard-daemon`, and `cargo test --test routes` (158 tests) all
  green. Manual dogfood (real Chrome/Edge "Install app" against a running
  daemon; confirm the standalone window has no tab strip and Ctrl+R still
  reloads) remains the user's step - no live daemon instance was available
  in this implementing session.
- Remaining: Phase 2 (Class-A keydown suppression) is still pending; Phase
  3 (Keyboard Lock) stays deferred per the Delivery-Mode Spectrum above.
  Ticket stays in `ready/` until both are addressed.

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

### Result

#### Edition (9fe2cb4b) - 2026-07-21

Delivered on branch `impl/pwa-keydown-suppression`.

- Global capture-phase `document` keydown suppressor installed near `App()`
  that `preventDefault()`s Class-A browser shortcuts via a new pure
  predicate `shouldSuppressBrowserShortcut` in a new `keydownSuppression.ts`
  module. Suppressed set: Ctrl/Cmd + S/P/F/G/D/O/U/J and zoom
  (+ / = / - / _ / 0), plus Backspace when the event target is NOT
  editable. Whitelisted (never suppressed): Ctrl+R (reload), Ctrl/Cmd +
  C/V/X/A/Z/Y, all plain typing, and Backspace in editable targets
  (input/textarea/contenteditable/select).
- Terminal coexistence: confirmed safe by correctness review against
  xterm.js source. The suppressor is capture-phase on `document` and never
  calls `stopPropagation`; xterm's own capture-phase handler does not gate
  on `defaultPrevented`, so all suppressed control bytes still forward to
  the terminal, and terminal Backspace is treated as editable (not
  suppressed). No terminal-input regression.
- Class-B shortcuts (Ctrl+W/T/N/Tab/1-9) remain browser-reserved and
  unaffected by design - the PWA-standalone install from Phase 1 is what
  removes the tab strip those act on, not this handler.
- Testability: the block/allow logic is a pure DOM-free predicate,
  unit-tested via a new `test:keydown-suppression` script covering
  uppercase/Shift casing, Ctrl+Backspace, Cmd/meta parity, clipboard-allow,
  and editable-vs-not; `SUPPRESSED_CTRL_KEYS` is exported and iterated so
  new keys auto-cover.
- Commits: `9fe2cb4b` (implementation + spec) + `7bcdbcbd` (review relay:
  test hardening + `<select>` editable alignment).
- Review: partitioned correctness/fit/test review. Correctness clean
  (terminal coexistence verified). Fit clean. Test findings
  (casing/Backspace/meta coverage + dedup) relayed and re-reviewed
  RESOLVED. Remaining minors accepted by design.
- Spec: added anchor `260721-ws-dashboard-browser-shortcut-suppression` to
  `ai-docs/spec/ws-web-dashboard/index.md`.
- Plan: `ai-docs/.plans/2026-07/21-1847-260721-phase2-keydown-suppression.md`
- Manual dogfood (real browser: confirm each suppressed shortcut no longer
  fires its browser action, Ctrl+R still reloads, terminal input
  unaffected) remains the user's step - no live daemon instance was
  available in this implementing session.

#### Edition (1cc6dcdf) - 2026-07-21

Reverses this phase's original Ctrl+R whitelist decision, per explicit
owner direction: Ctrl+R will be reused as the in-app reverse-history-search
binding in the terminal and agent chat inputs, so it can no longer be left
unsuppressed at the page level.

- `SUPPRESSED_CTRL_KEYS` now includes `"r"`; the special-case whitelist
  branch (`if (key === "r") return false`) was removed from
  `shouldSuppressBrowserShortcut` in `keydownSuppression.ts`. Ctrl/Cmd+R is
  now suppressed like the rest of the Class-A set.
- Reload is no longer reachable via Ctrl/Cmd+R; F5 and the browser reload
  button remain the reload path (unaffected by this suppressor, out of its
  scope).
- Tests (`keydownSuppression.test.ts`) updated: the Ctrl+R/Cmd+R and
  Ctrl+Shift+R/Cmd+Shift+R cases now assert suppressed (`true`) instead of
  whitelisted (`false`).
- Spec anchor `260721-ws-dashboard-browser-shortcut-suppression` in
  `ai-docs/spec/ws-web-dashboard/index.md` updated to match.

**Ticket closure**: Phases 1-2 are complete and reviewed. Phase 3 (Keyboard
Lock API / Tauri) is intentionally deferred per this ticket's own
Delivery-Mode Spectrum above - it is scoped as optional/deferred, to be
picked up only if Phases 1-2 prove insufficient in practice. Closing this
ticket now; reopen or spin a new ticket if that need materializes.

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
