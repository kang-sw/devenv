# Plan: 260721-feat-dashboard-suppress-browser-shortcuts — Phase 2: Class-A keydown suppression

## Relevant Ticket Contract

- Phase 2 scope (`ai-docs/tickets/ready/260721-feat-dashboard-suppress-browser-shortcuts.md:201-216`):
  a global `keydown` interceptor (capture-phase, installed once near the
  top-level `App` component, `App.tsx:437` — actually `App.tsx:450` in the
  current tree, see Codebase Findings) that calls `preventDefault()` for the
  catchable Class-A browser shortcuts the dashboard wants blocked, while
  explicitly whitelisting Ctrl+R so reload keeps working. Works identically
  in both plain-tab and installed-PWA modes.
- Constraints (`:19-56`): Class A = interceptable via page-level `keydown` +
  `preventDefault()` (Ctrl+S/P/F/G/D/O/U/J, zoom Ctrl+Plus/Minus/0, Backspace
  back-navigation, Ctrl+R). Class B = browser-chrome-reserved, never reaches
  page script regardless of any handler (Ctrl+W/T/N, Ctrl+Shift+T/W/N,
  Ctrl+Tab, Ctrl+1..9, Alt+F4) — nothing to implement for Class B.
- Prior Art (`:58-84`): two precedents in `App.tsx` for the idioms to reuse —
  `dismissOnEscape` (document-level capture-phase listener with cleanup) and
  `keydownFallback` (ctrl/meta-combo detection + `preventDefault` idiom). See
  Codebase Findings for exact current line numbers (they drifted from the
  ticket's citation).
- Verification (`:213-216`): each targeted Class-A shortcut is suppressed (no
  save dialog, no find bar, no zoom) in both a plain tab and the Phase 1
  installed PWA, while Ctrl+R still reloads. The PWA/browser part is manual/
  dogfood; the block/allow decision itself is unit-testable (see Test Plan).
- Spec Impact (`:230-238`): Phase 2's suppression set is expected to be
  spec-addressed alongside or shortly after Phase 1's spec entry.

### Lead-decided block/allow set (encoded here, not reopened)

- **SUPPRESS** (`preventDefault()`) when `ctrlKey || metaKey` is true and the
  (lowercased) key is one of: `s`, `p`, `f`, `g`, `d`, `o`, `u`, `j`, or a zoom
  key `+` / `=` / `-` / `_` / `0`.
- **SUPPRESS** plain `Backspace` (no ctrl/meta) when the event target is NOT
  an editable field (input / textarea / `isContentEditable`).
- **NEVER SUPPRESS**: Ctrl+R (explicit whitelist, checked before the block
  set), Ctrl+C/V/X/A/Z/Y, any plain typing, and Backspace inside an editable
  target.
- Class B combos (Ctrl+W/T/N/Tab/1-9, Alt+F4, Shift variants) — out of scope,
  nothing reaches page script.

## Out of Scope

- Phase 1 (PWA manifest/service worker/install-prompt code) — already
  delivered per `260721-feat-dashboard-suppress-browser-shortcuts.md:153-199`
  ("Result" section, edition `5075f142`). Do not touch
  `ws-dashboard/frontend/public/`, `index.html`, `main.tsx`'s SW/
  `beforeinstallprompt` wiring, or `router.rs`'s manifest/sw/icon routes.
- Phase 3 (Keyboard Lock API / fullscreen residual-key handling,
  `:218-228`) — deferred, not part of this plan.
- Class B shortcuts (Ctrl+W/T/N/Tab/1-9, Alt+F4) — uninterceptable from page
  script by design; no code addresses them.
- Any change to `keydownFallback`'s terminal-forwarding behavior in
  `TerminalPaneBody` (`App.tsx:8428-8828`) beyond what's needed to confirm it
  keeps working unmodified (see Conflict Analysis below) — it is not touched.

## Codebase Findings

### Precedent 1 — `dismissOnEscape` (capture-phase listener + cleanup idiom)

`ws-dashboard/frontend/src/App.tsx:1637-1665`, inside `useDismissableMenu`:

```ts
useEffect(() => {
  if (!open) {
    return;
  }
  const dismissIfOutside = (event: MouseEvent) => { ... };
  const dismissOnEscape = (event: KeyboardEvent) => {
    if (event.key === "Escape") {
      onDismiss();
    }
  };
  document.addEventListener("click", dismissIfOutside);
  document.addEventListener("keydown", dismissOnEscape, true);
  return () => {
    document.removeEventListener("click", dismissIfOutside);
    document.removeEventListener("keydown", dismissOnEscape, true);
  };
}, [containerRef, onDismiss, open]);
```

Idiom to reuse: `document.addEventListener("keydown", handler, true)` (capture
phase, third-arg `true`) inside a `useEffect`, paired with the mirrored
`removeEventListener(..., true)` in the cleanup function. This is the correct
shape for the new global suppressor (ticket explicitly calls for
capture-phase).

### Precedent 2 — `keydownFallback` (ctrl/meta detection + preventDefault idiom)

`ws-dashboard/frontend/src/App.tsx:8595-8652` (ticket cited `8343-8400`; the
file has grown since the ticket was written — current content confirmed by
direct read), inside `TerminalPaneBody` (component starts `App.tsx:8428`):

```ts
const keydownFallback = (event: KeyboardEvent) => {
  if (!container.offsetParent) return;
  if (!liveRef.current.actions.isActivePane(liveRef.current.pane)) return;
  if (event.isComposing || event.key === "Process" || composingInput) return;
  const isMetaLineStart = event.metaKey && event.key.toLowerCase() === "a";
  if (container.contains(document.activeElement) && !isMetaLineStart) return;
  const target = event.target as HTMLElement | null;
  const tagName = target?.tagName.toLowerCase();
  if (target?.isContentEditable || tagName === "input" || tagName === "textarea" || tagName === "select") {
    return;
  }
  let data: string | null = null;
  if (event.ctrlKey || event.metaKey) {
    const key = event.key.toLowerCase();
    if (key === "c") data = "\x03";
    if (key === "l") data = "\x0c";
    if (key === "a") data = "\x01";
    if (key === "u") data = "\x15";
    if (key === "w") data = "\x17";
  } else if (event.key.length === 1) { ... }
  ...
  if (data !== null) {
    event.preventDefault();
    ...
  }
};
window.addEventListener("keydown", keydownFallback); // bubble phase, no capture flag
```

Idioms to reuse: `event.ctrlKey || event.metaKey` as the combo guard
(exactly what the task instructions specify), `event.key.toLowerCase()`
normalization, and the editable-target check
(`target?.isContentEditable || tagName === "input" || tagName === "textarea"`)
as the shape for representing "editable target" in the new predicate.

### Where to install the new global effect

`export function App()` starts at `App.tsx:450` (ticket cites `:437`, which
is now the line just before the function in the current tree — a one-line
drift from a preceding doc-comment edit, not a different function). The first
existing `useEffect` inside `App` is at `App.tsx:689`. The new suppressor
effect should be added as the **first** effect in `App`, immediately after the
block of `useState` declarations and before line 689, matching the ticket's
"installed once near the top-level App component" instruction and keeping it
visually separate from feature-specific effects further down.

`main.tsx` wraps `<App />` in `<StrictMode>` (`main.tsx:6-9`), so this effect
mounts/cleans-up/remounts once in dev — harmless for
`addEventListener`/`removeEventListener`, no special handling needed.

### Conflict analysis — terminal `keydownFallback` vs. the new global suppressor

- **Phase difference**: the new suppressor is a capture-phase listener on
  `document` (`addEventListener(..., true)`); `keydownFallback` is a
  bubble-phase listener on `window` (`addEventListener(..., keydownFallback)`,
  no capture flag). Capture always runs before target/bubble for the same
  event, so the new suppressor's handler runs first on every keydown,
  regardless of DOM focus location.
- **`preventDefault()` does not stop propagation.** Calling
  `event.preventDefault()` in the capture-phase suppressor only cancels the
  browser's default action; it does **not** call `stopPropagation()` /
  `stopImmediatePropagation()`, so `keydownFallback` (and every other
  listener) still receives and fully processes the same event afterward. This
  plan's App-level handler must never call `stopPropagation`/
  `stopImmediatePropagation` — that is the load-bearing reason the two
  listeners can coexist.
- **Overlap key: Ctrl+U.** It is in both sets — the new suppressor's block
  list (browser "view source") and `keydownFallback`'s terminal-forwarding
  map (`u` → `\x15`, clear line). Because `preventDefault()` alone doesn't
  block propagation, when a terminal pane is the active pane and
  `keydownFallback`'s own guards pass (pane active, DOM focus outside the
  terminal container per its fallback condition, target not editable),
  `keydownFallback` still runs, still forwards `\x15`, and still calls its
  own `preventDefault()` — functionally identical to today, just with an
  earlier no-op `preventDefault()` call from the capture phase. No regression.
- **Normal terminal typing (xterm has actual DOM focus)**: xterm.js attaches
  its own key handling directly to its focused helper element (target-phase,
  not `window` bubble). The new capture-phase suppressor runs first but, per
  the same non-propagation-stopping property, does not prevent xterm's own
  target-phase handler from running. `preventDefault()` on Ctrl+U while
  typing in a focused terminal only suppresses the *browser's* view-source
  action (which was never going to fire while xterm has focus and its own
  handler is engaged) — xterm's own onData path is unaffected.
- **Backspace / editable-target interaction**: xterm's actual focusable node
  is a hidden `textarea` in most terminal-emulator implementations of this
  shape; when that element has focus, `target.tagName === "textarea"` is
  true, so the new predicate's `targetIsEditable` check naturally excludes
  Backspace suppression while a terminal is genuinely focused — consistent
  with (not fighting) existing terminal input behavior. This should be
  spot-checked during manual verification (see Verification Plan) but is not
  expected to require special-casing in the predicate.
- **Conclusion**: no genuine conflict. The two listeners operate in different
  phases, neither calls `stopPropagation`, and the one overlapping key
  (Ctrl+U) degrades to a harmless redundant `preventDefault()` call, not a
  functional collision. No `[escalate-to-research]` needed.

### Test-chain wiring precedent

- Pure browser-DOM-free module pattern: `ws-dashboard/frontend/src/workNavOrder.ts`
  (no React import; plain exported functions/types) paired with
  `ws-dashboard/frontend/src/workNavOrder.test.ts` (imports via `./workNavOrder.js`
  per `NodeNext` module resolution; hand-rolled `assertEqual`/`assertDeepEqual`
  helpers; throws on mismatch; no external test framework). Same pairing
  pattern for `resourceModel.ts` / `resourceModel.test.ts`.
- Test-runner chain: `ws-dashboard/frontend/tsconfig.route-tests.json:12-63` —
  an explicit `include` list of every source+test file pair that participates
  in this test family, compiled once via `tsc -p tsconfig.route-tests.json`
  into `node_modules/.tmp/route-tests/`, then each `npm run test:<area>`
  script (`package.json:8-24`, e.g. `test:resource-model`,
  `test:workbench`, `test:git`) runs `tsc -p tsconfig.route-tests.json &&
  node ./node_modules/.tmp/route-tests/<file>.test.js` (chained with `&&` for
  multi-file areas). There is no aggregate "run everything" script and no CI
  workflow invoking these (confirmed: no `.github/workflows` reference to any
  `test:*` script) — each is a standalone, manually-invoked verification
  script per feature area.
- No existing keybinding/shortcut-suppression test area exists (ticket's own
  Prior Art section already confirms no dedicated module). This phase adds a
  new standalone area rather than folding into an unrelated existing chain
  (e.g. `test:resource-model` or `test:workbench` would be a topical
  mismatch).

## Implementation Plan

1. **Pure predicate module** — create
   `ws-dashboard/frontend/src/keydownSuppression.ts` (no React/DOM imports,
   mirroring `workNavOrder.ts`'s pure-module shape):

   ```ts
   // Pure predicate for Phase 2 of
   // 260721-feat-dashboard-suppress-browser-shortcuts: decides whether a
   // Class-A browser-shortcut keydown should be preventDefault()-ed. Kept
   // DOM-free so the block/allow set is unit-testable without jsdom; the
   // caller (App.tsx) is responsible for reading real DOM state into the
   // minimal shape below.

   export type SuppressibleKeydownEvent = {
     readonly ctrlKey: boolean;
     readonly metaKey: boolean;
     readonly key: string;
     readonly targetIsEditable: boolean;
   };

   const SUPPRESSED_CTRL_KEYS = new Set([
     "s", "p", "f", "g", "d", "o", "u", "j",
     "+", "=", "-", "_", "0",
   ]);

   export function shouldSuppressBrowserShortcut(
     evt: SuppressibleKeydownEvent,
   ): boolean {
     const ctrlOrMeta = evt.ctrlKey || evt.metaKey;
     if (ctrlOrMeta) {
       const key = evt.key.toLowerCase();
       if (key === "r") {
         return false; // explicit reload whitelist — never suppress
       }
       return SUPPRESSED_CTRL_KEYS.has(key);
     }
     if (evt.key === "Backspace") {
       return !evt.targetIsEditable;
     }
     return false;
   }
   ```

   `targetIsEditable` is a caller-supplied boolean rather than a DOM node so
   the predicate itself never touches `HTMLElement`/`document`, keeping it
   importable and testable in plain Node.

2. **App-level wiring** — in `ws-dashboard/frontend/src/App.tsx`, add a new
   `useEffect` as the first effect inside `App()` (before the existing effect
   at `:689`), importing `shouldSuppressBrowserShortcut` from
   `./keydownSuppression.js`:

   ```ts
   useEffect(() => {
     const suppressBrowserShortcut = (event: KeyboardEvent) => {
       const target = event.target as HTMLElement | null;
       const tagName = target?.tagName.toLowerCase();
       const targetIsEditable =
         Boolean(target?.isContentEditable) ||
         tagName === "input" ||
         tagName === "textarea";
       if (
         shouldSuppressBrowserShortcut({
           ctrlKey: event.ctrlKey,
           metaKey: event.metaKey,
           key: event.key,
           targetIsEditable,
         })
       ) {
         event.preventDefault();
       }
     };
     document.addEventListener("keydown", suppressBrowserShortcut, true);
     return () => {
       document.removeEventListener("keydown", suppressBrowserShortcut, true);
     };
   }, []);
   ```

   Empty dependency array: the predicate and handler are pure/stable and do
   not close over component state, so the listener installs exactly once for
   the app's lifetime (mirroring the ticket's "installed once" framing).
   Critically, this handler must never call `stopPropagation()` /
   `stopImmediatePropagation()` (see Conflict Analysis) — only
   `preventDefault()`.

3. **Test file** — create
   `ws-dashboard/frontend/src/keydownSuppression.test.ts` (same
   `assertEqual`-style harness as `workNavOrder.test.ts`), covering:
   - Each suppressed ctrl combo → `true`: `s`, `p`, `f`, `g`, `d`, `o`, `u`,
     `j`, `+`, `=`, `-`, `_`, `0` (with `ctrlKey: true`).
   - Ctrl+R → `false` (explicit whitelist), including with `metaKey: true`
     instead of `ctrlKey` (mac Cmd+R).
   - Ctrl+C / Ctrl+V / Ctrl+X / Ctrl+A / Ctrl+Z / Ctrl+Y → `false` (normal
     editing/clipboard combos untouched).
   - Plain key with no modifier (e.g. `"a"`, `"Enter"`) → `false`.
   - Backspace with `targetIsEditable: true` → `false`; Backspace with
     `targetIsEditable: false` → `true`.
   - Meta-key (Cmd) variants of at least one suppressed combo (e.g.
     `metaKey: true, key: "s"`) → `true`, confirming the `ctrlKey || metaKey`
     guard covers both.
   - A ctrl combo not in the suppressed set (e.g. Ctrl+K) → `false`, to pin
     the allow-by-default behavior for anything not explicitly listed.

4. **Wire into the test-config include list and a new npm script**:
   - `ws-dashboard/frontend/tsconfig.route-tests.json` — add
     `"src/keydownSuppression.ts"` and `"src/keydownSuppression.test.ts"` to
     the `include` array (alongside the `workNavOrder.ts`/`.test.ts` pair at
     `:32-33`).
   - `ws-dashboard/frontend/package.json` — add a new script following the
     existing chain pattern:
     ```json
     "test:keydown-suppression": "tsc -p tsconfig.route-tests.json && node ./node_modules/.tmp/route-tests/keydownSuppression.test.js"
     ```
     A new standalone script (not folded into `test:resource-model` or
     `test:workbench`) since this is a topically distinct area with no
     existing home, matching how `test:git`, `test:document-viewer`, etc.
     are each their own script.

5. **Spec step** — add a new sibling anchor to
   `ai-docs/spec/ws-web-dashboard/index.md`, placed directly after the "PWA
   Installability" section (`:716-742`) since it is the same ticket family
   and delivery-mode concern but a materially different behavioral contract
   (client-side input handling vs. static serving/installability — existing
   spec practice keeps these as separate anchors, e.g. "Protected Frontend
   Shell" vs. "PWA Installability" are already siblings, not merged). Do NOT
   extend the existing `260721-ws-dashboard-pwa-installability` anchor's
   prose — add a new heading:

   ```md
   ## Browser Shortcut Suppression {#260721-ws-dashboard-browser-shortcut-suppression}

   The dashboard installs one capture-phase `document`-level `keydown`
   listener at the top-level `App` component that calls `preventDefault()`
   for a fixed set of Class-A browser shortcuts (Ctrl/Cmd+S/P/F/G/D/O/U/J,
   zoom Ctrl/Cmd+Plus/Minus/Equals/Underscore/0, and Backspace-as-back-
   navigation when the focused target is not an editable field), while
   explicitly never suppressing Ctrl/Cmd+R (reload) or normal editing/
   clipboard combos (Ctrl+C/V/X/A/Z/Y). The block/allow decision is a pure,
   DOM-free predicate (`keydownSuppression.ts`) so the exact set is
   unit-tested without a browser DOM; the App effect only reads real event/
   target state into the predicate's input shape. This suppressor targets
   Class A shortcuts only (interceptable via page-level `keydown`) — Class B
   browser-chrome-reserved shortcuts (Ctrl+W/T/N/Tab/1-9) are not addressed
   here since they never reach page script in any served mode. It runs
   identically in a plain browser tab and the Phase 1 installed PWA
   (`260721-ws-dashboard-pwa-installability`).
   ```

   Mental-model touch: not required. The mental-model doc's static-serving
   bullet (`ai-docs/mental-model/ws-web-dashboard.md:86`) documents *serving*
   contracts; this is a client-side input-handling behavior with no serving
   or module-boundary implication, so no existing mental-model bullet goes
   stale from this change. No mental-model edit needed.

## Verification Plan

- `cd ws-dashboard/frontend && npm run build` (`tsc -b && vite build`) must
  stay green after adding `keydownSuppression.ts` and the `App.tsx` effect.
- `npm run test:keydown-suppression` (new script) must pass, exercising every
  case in the Test File section above.
- Manual/dogfood (matches the ticket's own stated verification boundary,
  `:213-216`): with the interceptor active in a running dashboard (plain tab
  and the Phase 1 installed PWA), confirm each suppressed combo produces no
  browser side effect (no save dialog on Ctrl+S, no find bar on Ctrl+F, no
  zoom on Ctrl+Plus/Minus/0, no back-navigation on Backspace outside an
  input) while Ctrl+R still reloads; separately confirm terminal panes still
  forward Ctrl+C/L/A/U/W to the shell as before (spot-checking the Ctrl+U
  overlap noted in Conflict Analysis) and that normal typing/Backspace inside
  text inputs and the terminal is unaffected.

## Escalations

- None. The one apparent overlap (Ctrl+U used by both the new suppressor and
  the terminal's `keydownFallback`) resolves cleanly because
  `preventDefault()` does not stop event propagation — no design blocker.
