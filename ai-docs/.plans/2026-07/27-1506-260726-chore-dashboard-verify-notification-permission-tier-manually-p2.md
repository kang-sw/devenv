# Plan: 260726-chore-dashboard-verify-notification-permission-tier-manually — Phase 2: correct the insecure-context guard, its comment, and its spec claim

## Relevant Ticket Contract

- Reorder `currentNotificationAvailability()` to consult `window.isSecureContext`
  before `typeof Notification`, so a plain-http LAN page reports the
  insecure-context state instead of falling through to "denied". Keep the
  `typeof Notification === "undefined"` branch reachable — it is still correct
  for a browser that genuinely omits the global (Safari/Firefox); this is a
  reorder, never a swap.
- Extract the decision as an exported pure function of its three inputs
  (secure-context flag, global presence, permission value) so all four states
  are assertable from `settingsSections.test.ts`.
- Settle whether the checkbox should be offered on an insecure origin; the
  ticket recommends disabling it and requires the choice be recorded either
  way (caller-visible change).
- Spec amendment is required and load-bearing, in two parts: (1) correct the
  false "absent entirely" claim in both `#260726-dashboard-browser-level-attention-cue`
  and `#260722-ws-dashboard-settings-panel` to "un-permissioned and ungrantable",
  keeping the conclusion that Tier 2 cannot work there; (2) state whichever way
  the disabled-control question lands, in the settings-panel anchor. Run
  `ws/spec_index.verify` after editing.
- Deferred scope (do not touch): Tier 1, the edge detector, the
  `new Notification(...)` call site, its `catch`, and anything in `App.tsx`.
- Verification boundary: unit assertions for all four availability states
  (including both insecure variants); `npm run build`; if the control's
  disabled state changes, one browser assertion for it, rebuilt first.

## Out of Scope

- Phase 1 (already landed, commits `87259c93`/`4acbdc98`) and its new spec file
  `e2e/agent-attention-notification.spec.ts` — read only to confirm this phase
  does not regress it, never re-planned.
- The Tier 2 effect in `App.tsx` (edge detector, `new Notification(...)`,
  its `catch`) — explicitly deferred by this phase.
- The six `## Human verification residue` steps — human-only, not this phase's
  job to discharge or record.
- A Playwright gate on a real plain-http LAN origin, or one that drives a real
  native permission dialog — both explicitly rejected alternatives in the
  ticket's Decisions.

## Codebase Findings

- `ws-dashboard/frontend/src/settingsSections.tsx#L101-L116` — the exact
  current function and its backwards-reasoning comment:

  ```
  101  // `window.isSecureContext` and `Notification.permission` are both readable
  102  // with no permission prompt of their own, so the section can show the actual
  103  // limitation up front (ticket text: "Settings copy stating plainly that
  104  // OS-level notification requires localhost or TLS") rather than only
  105  // surprising the user after a click does nothing. Checked in this order
  106  // because a plain-http LAN page lacks the whole `Notification` global, not
  107  // merely a granted permission - `window.isSecureContext` alone would not
  108  // distinguish "insecure" from "secure but denied".
  109  function currentNotificationAvailability(): string {
  110    if (typeof Notification === "undefined") {
  111      return window.isSecureContext
  112        ? "unavailable in this browser"
  113        : "unavailable - this page is not a secure context";
  114    }
  115    return Notification.permission;
  116  }
  ```

  Three branches today: (a) global undefined + insecure -> "unavailable - this
  page is not a secure context" (UNREACHABLE on Chromium per the ticket's
  measurement, since the global IS defined there even when insecure); (b)
  global undefined + secure -> "unavailable in this browser"; (c) global
  defined (any secure-context value) -> live `Notification.permission`. Bug: on
  Chromium, a plain-http LAN page falls into branch (c) and reports "denied",
  which reads as "you denied this" rather than "this page is insecure".

- `ws-dashboard/frontend/src/settingsSections.tsx#L140-L176` — the checkbox
  itself has no `disabled` attribute today; its `onChange` always fires
  `requestPermission()` when checked, regardless of secure-context state. This
  is the second half of the bug: on an insecure Chromium origin the box is
  offered, clickable, calls `requestPermission()`, gets `denied`, and silently
  unchecks itself (`onChange(false)` at line 163) with no explanation.

- `ws-dashboard/frontend/src/settingsSections.tsx#L179-L183` — the JSX that
  embeds the string return value directly: `` Current permission: {currentNotificationAvailability()}. `` — the extracted pure function must keep
  returning a *display string* (not an opaque enum) so this call site needs no
  change beyond the reorder.

- `ws-dashboard/frontend/src/notificationPrefs.ts#L25-L43` — the idiom to
  follow for the extraction: `parseNotificationPrefs(raw: unknown): NotificationPrefs | null`
  is exported specifically "so its edge cases can be asserted directly against
  its own return value" (its own comment), with a thin `loadNotificationPrefs`
  wrapper that supplies the live default source. Same shape as
  `terminalPrefs.ts`'s `parseTerminalFontSizeInput`. Apply the same pattern
  here: an exported pure core plus a thin live-reading wrapper.

- `ws-dashboard/frontend/src/settingsSections.test.ts` (all 129 lines read) —
  today only imports `NotificationSection`, `SETTINGS_SECTIONS`,
  `SettingsNotificationContext`, `SettingsTerminalContext`,
  `TerminalStyleSection` and asserts registry shape (id/title/`Component`
  identity/arity) via a hand-rolled `assertEqual`. No render, no DOM, no
  `window`/`Notification` reference anywhere in the file — confirms the module
  is importable there today with no DOM, and a newly exported pure function
  can be added to the same import list and called directly with literal
  booleans/strings, no environment setup needed.

- `ws-dashboard/frontend/tsconfig.route-tests.json` — `"jsx": "react-jsx"`,
  `target: ES2022`, no explicit `"lib"` override, so default DOM lib types
  (`NotificationPermission`, etc.) are available for the new function's
  signature even though the test never touches an actual DOM at runtime.
  `settingsSections.tsx`/`.test.ts` are both already in its `include` list
  (no tsconfig change needed).

- `ws-dashboard/frontend/package.json#L33` —
  `"test:settings": "tsc -p tsconfig.route-tests.json && node ./node_modules/.tmp/route-tests/settingsStore.test.js && node ./node_modules/.tmp/route-tests/terminalPrefs.test.js && node ./node_modules/.tmp/route-tests/settingsSections.test.js"`.
  This is the exact confirmed unit-test command for this phase.

- `ws-dashboard/frontend/package.json#L8` — `"build": "tsc -b && vite build"`,
  the confirmed `npm run build` command the ticket's verification boundary
  names directly.

- `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L1029-L1049` — an
  existing `test.step("settings modal opens and closes", ...)` inside the one
  giant serial `test("dashboard workRoot UI browser acceptance", ...)`
  (declared `test.describe.configure({ mode: "serial" })` at line 77, only two
  top-level `test()`s in the whole file). This step already opens the Settings
  dialog via `[data-command-id="settings.open"]` and scopes to
  `[role="dialog"][aria-label="Settings"]` — the pattern a new Notifications
  disabled-state step should reuse (click into the Notifications section nav
  button the same way this step finds the Terminal one).

- `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L1059-L1068` — sibling
  precedent for asserting a disabled control: `await expect(submit).toBeDisabled();`
  / `await expect(submit).toBeEnabled();` on the add-server submit button. Use
  the same Playwright locator assertion (`toBeDisabled()`) for the Notifications
  checkbox rather than reading the DOM attribute manually.

- `ws-dashboard/frontend/e2e/agent-attention-notification.spec.ts#L299-L347` —
  Phase 1's precedent for safely faking a browser global from a test: it
  overrides `window.Notification` with `Object.defineProperty(window, "Notification", { configurable: true, writable: true, value: proxied })`
  inside `page.addInitScript`, never a bare stub-class replacement. The same
  `Object.defineProperty` technique (this time on `window.isSecureContext`,
  and via a direct `page.evaluate` rather than an `addInitScript`, since no
  reload/fresh-realm is needed) is the mechanism to reach for to force the
  insecure-context branch without a real plain-http LAN origin. Grep confirms
  `isSecureContext` is not overridden anywhere in `e2e/` today — this would be
  the first use of that specific override, but it directly reuses an
  already-proven technique in the same file family.

- `ws-dashboard/frontend/e2e/agent-attention-notification.spec.ts` (whole file,
  read for constraints) — every test in this file needs `channel: "chromium"`
  and a permission grant, both irrelevant to the disabled-checkbox assertion
  (no `requestPermission()` call ever fires on a disabled control, so nothing
  here needs the full Chromium channel or a `grantPermissions` call). Reusing
  this file's daemon for the new assertion would be free-riding on
  infrastructure the assertion does not need.

- Confirmed via `grep -rn "currentNotificationAvailability\|isSecureContext" src/*.ts src/*.tsx`
  — the function and both raw-string references live ONLY in
  `settingsSections.tsx`; no other call site to update. Confirmed via
  `grep -rn "notification" e2e/*.ts` — no spec file besides
  `agent-attention-notification.spec.ts` and `agentTurnState.ts` touches
  Notifications at all today, so `dashboard-acceptance.spec.ts` has zero
  existing Notifications-section assertions to avoid clobbering.

- `ai-docs/spec/ws-web-dashboard/index.md#L995-L1001` — the
  `#260722-ws-dashboard-settings-panel` sentence to amend (exact text quoted
  below in Implementation Plan step 5).

- `ai-docs/spec/ws-web-dashboard/index.md#L2475-L2477` — the
  `#260726-dashboard-browser-level-attention-cue` sentence to amend (exact text
  quoted below in Implementation Plan step 5).

- Risk signal: overriding `window.isSecureContext` via `Object.defineProperty`
  from the browser gate is a technique with no prior use in this repo for that
  specific global (only `window.Notification` has been overridden before).
  Low risk because the mechanism (own-property shadowing a prototype getter,
  `configurable: true`) is generic and already proven for a harder case
  (shadowing a constructor function, not a boolean getter) in the same file
  family — flagged here for visibility, not escalated.

## Implementation Plan

1. In `ws-dashboard/frontend/src/settingsSections.tsx`, replace the
   `currentNotificationAvailability()` block (lines 101-116) with an exported
   pure function plus a thin live wrapper, following the
   `parseNotificationPrefs`/`loadNotificationPrefs` idiom:

   ```ts
   // Exported so all four (isSecureContext, hasNotificationGlobal, permission)
   // states are assertable directly from settingsSections.test.ts, without a
   // DOM or a real browser — this is the cheap substitute for a LAN browser
   // gate. Order is load-bearing: on Chromium a plain-http LAN page still has
   // the `Notification` global defined, so `isSecureContext` — not
   // `typeof Notification` — is what distinguishes "insecure" from "secure but
   // denied". Checking `hasNotificationGlobal` second keeps the
   // undefined-global branch reachable for a browser that genuinely omits the
   // global on an insecure origin (Safari, Firefox may) — this is a reorder,
   // never a swap.
   export function notificationAvailability(
     isSecureContext: boolean,
     hasNotificationGlobal: boolean,
     permission: NotificationPermission,
   ): string {
     if (!isSecureContext) {
       return "unavailable - this page is not a secure context";
     }
     if (!hasNotificationGlobal) {
       return "unavailable in this browser";
     }
     return permission;
   }

   function currentNotificationAvailability(): string {
     const hasNotificationGlobal = typeof Notification !== "undefined";
     return notificationAvailability(
       window.isSecureContext,
       hasNotificationGlobal,
       hasNotificationGlobal ? Notification.permission : "default",
     );
   }
   ```

   Four-state mapping (this IS the reorder, stated explicitly):
   - `isSecureContext=false, hasNotificationGlobal=false` -> `"unavailable - this page is not a secure context"` (Safari/Firefox-shaped insecure LAN page).
   - `isSecureContext=false, hasNotificationGlobal=true` -> `"unavailable - this page is not a secure context"` (Chromium-shaped insecure LAN page — the exact case the ticket proves was mishandled before; this is the state that changes from today's "denied" output).
   - `isSecureContext=true, hasNotificationGlobal=false` -> `"unavailable in this browser"`.
   - `isSecureContext=true, hasNotificationGlobal=true` -> `permission` passed through verbatim (`"default" | "granted" | "denied"`).

   Return type stays plain `string` (not a new enum) because the call site at
   line 182 embeds the return value directly into user-facing copy — no
   display-layer indirection needed, matching the original function's own
   return type.

2. In the same file, wire the disabled-control decision into `NotificationSection`
   (currently lines 129-186): compute `const insecureContext = !window.isSecureContext;`
   once at the top of the component, add `disabled={insecureContext}` to the
   `<input type="checkbox">` (line ~142), and update the note paragraph so the
   disabled state is stated in copy, not just inferred from the permission
   string — e.g. append a short clause when `insecureContext` is true (exact
   wording left to the implementer; keep it short and truthful, e.g. "the
   toggle above is disabled here"). No change to the `onChange` handler itself:
   a disabled `<input>` never fires `onChange`, so the existing
   `requestPermission()`/reconciliation logic becomes naturally unreachable on
   an insecure context — no extra guard needed inside the handler.

   Disabled-control decision, recorded: DISABLE the control on an insecure
   context (`disabled={!window.isSecureContext}`), per the ticket's own
   recommendation. Confirmed safe: `grep -rn "toBeDisabled\|toBeEnabled\|notification-toggle\|NotificationSection" e2e/*.ts` and
   `grep -n "notification" e2e/*.ts` show no existing test asserts the checkbox
   is enabled or interacts with it outside `agent-attention-notification.spec.ts`,
   and every test in that file runs against `localhost` (a secure context by
   spec regardless of TLS), so `insecureContext` is `false` there and none of
   Phase 1's four tests are affected by adding `disabled`.

3. Update the two stale comments in the same file: the block comment above
   `currentNotificationAvailability()`/`notificationAvailability()` (already
   rewritten in step 1) and the `CONTRACT` comment above `NotificationSection`
   (lines 122-128), which currently says "Guarded on `typeof Notification === "undefined"` rather than `window.isSecureContext` alone, because a plain-http LAN page lacks the whole API, not just permission." — this restates the same backwards reasoning the ticket found wrong and must be corrected in the same edit (state that `isSecureContext` is checked first and is what distinguishes the two insecure-vs-denied cases).

4. Add four assertions to `ws-dashboard/frontend/src/settingsSections.test.ts`
   (append after the existing Notifications-registry block, before the final
   `assertEqual(true, true, ...)` sentinel at line 128), importing
   `notificationAvailability` alongside the existing named imports from
   `./settingsSections.js`:

   ```ts
   assertEqual(
     notificationAvailability(false, false, "default"),
     "unavailable - this page is not a secure context",
     "insecure context, no global (Safari/Firefox-shaped): reports insecure, not browser-unsupported",
   );
   assertEqual(
     notificationAvailability(false, true, "granted"),
     "unavailable - this page is not a secure context",
     "insecure context, global present (Chromium-shaped): reports insecure, NOT the raw permission - this is the ticket's core fix",
   );
   assertEqual(
     notificationAvailability(true, false, "default"),
     "unavailable in this browser",
     "secure context, no global: reports browser-unsupported",
   );
   assertEqual(
     notificationAvailability(true, true, "denied"),
     "denied",
     "secure context, global present: passes the live permission through verbatim",
   );
   ```

5. Amend the spec, in lockstep, both edits in
   `ai-docs/spec/ws-web-dashboard/index.md`:

   **Edit A** — `#260726-dashboard-browser-level-attention-cue`, current text
   at lines 2475-2477:

   > The dashboard is routinely reached over plain http on a LAN, where the
   > page is not a secure context and the browser's `Notification` API is
   > absent entirely — not merely un-permissioned. Tier 1 therefore uses only
   > what any page may do unasked:

   Draft replacement:

   > The dashboard is routinely reached over plain http on a LAN, where the
   > page is not a secure context. The browser's `Notification` API is
   > un-permissioned and ungrantable there, not absent entirely: on Chromium
   > the global itself is still defined and `window.isSecureContext` is the
   > property that distinguishes this case from a granted/denied secure
   > context, though a browser that genuinely omits the global on an insecure
   > origin (Safari, Firefox may) reaches the same ungrantable outcome by a
   > different path. Tier 2 cannot work on this class of origin either way, so
   > Tier 1 therefore uses only what any page may do unasked:

   **Edit B** — `#260722-ws-dashboard-settings-panel`, current text at lines
   995-1001:

   > The section's copy states plainly that OS-level notification requires a
   > secure context (`localhost` or a TLS origin): a plain-http LAN page lacks
   > the whole `Notification` global, not merely a granted permission, so the
   > section reads `window.isSecureContext` and `Notification.permission` live
   > (both readable with no permission prompt of their own) and shows the
   > current state — including an explicit "unavailable, insecure context"
   > message — rather than only surprising the user after an unresponsive
   > click.

   Draft replacement (folds in the disabled-control decision from step 2):

   > The section's copy states plainly that OS-level notification requires a
   > secure context (`localhost` or a TLS origin): a plain-http LAN page's
   > `Notification` API is un-permissioned and ungrantable there, not absent —
   > the global itself may still be defined — so the section reads
   > `window.isSecureContext` before `typeof Notification`, and (both readable
   > with no permission prompt of their own) shows the current state,
   > including an explicit "unavailable, insecure context" message, rather
   > than only surprising the user after an unresponsive click. On an insecure
   > context the checkbox itself is also disabled, since no click there can
   > ever change the permission.

   After both edits, run `ws/spec_index.verify` (MCP tool
   `mcp__plugin_ws_ws__spec_index_verify`) and fix any reported anchor/index
   drift before moving on.

6. Add one browser assertion for the new disabled state to
   `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts`, as a new
   `test.step` immediately after the existing "settings modal opens and
   closes" step (after line 1049), inside the same giant serial test — NOT in
   `agent-attention-notification.spec.ts` (that file's `channel: "chromium"`
   and permission grant exist only to make `requestPermission()` resolve
   `"granted"`; the disabled-state assertion never calls `requestPermission()`
   at all, so riding that file's heavier daemon/channel setup would be
   free-riding on infrastructure this assertion does not need). Sketch:

   ```ts
   await test.step("insecure context disables the Notifications toggle", async () => {
     await page.evaluate(() => {
       Object.defineProperty(window, "isSecureContext", {
         configurable: true,
         value: false,
       });
     });
     await page.locator('[data-command-id="settings.open"]').click();
     const dialog = page.locator('[role="dialog"][aria-label="Settings"]');
     await expect(dialog).toBeVisible();
     await dialog
       .locator(".settings-section-nav-button", { hasText: "Notifications" })
       .click();
     const checkbox = dialog.locator(
       '.settings-notification-toggle input[type="checkbox"]',
     );
     await expect(checkbox).toBeDisabled();
     await expect(dialog.locator(".settings-field-note")).toContainText(
       "not a secure context",
     );
     await dialog.locator('[data-command-id="settings.close"]').click();
     // Restore, since the rest of this giant serial test's later steps share
     // this same document/page and must not silently run under a faked
     // insecure context.
     await page.evaluate(() => {
       Object.defineProperty(window, "isSecureContext", {
         configurable: true,
         value: true,
       });
     });
   });
   ```

   Confirm the exact section-nav button selector/text and note-paragraph class
   against the rendered Settings modal (`.settings-section-nav-button`,
   `.settings-field-note` — both read from `settingsSections.tsx`/existing
   `dashboard-acceptance.spec.ts` usage above) before finalizing; adjust
   locator text only if the actual rendered DOM differs.

## Verification Plan

- `cd ws-dashboard/frontend && npm run test:settings` (confirmed command: runs
  `settingsStore.test.js`, `terminalPrefs.test.js`, `settingsSections.test.js`
  after `tsc -p tsconfig.route-tests.json`) — must pass, including the four
  new `notificationAvailability` assertions. Capture exit status per the
  caller's discipline: `cmd > file 2>&1` then `echo $?` on the next line of
  the same invocation.
- `cd ws-dashboard/frontend && npm run build` — must succeed (`tsc -b && vite build`).
- `cd ws-dashboard/frontend && npm run test:browser` — chains `npm run build`
  and `cargo build -p ws-dashboard-daemon` before `playwright test`, so the new
  disabled-checkbox step in `dashboard-acceptance.spec.ts` runs against a fresh
  bundle. Per the mental-model's Common Mistakes and this ticket's own
  Constraints: check stdout for exactly one
  `[e2e globalSetup] building the production frontend` line before trusting
  the result — a bare `npx playwright test` risks serving a stale bundle (two
  skip conditions: `WS_DASHBOARD_STATIC_DIR` set, or external-daemon mode).
  Confirm the existing "settings modal opens and closes" step and the rest of
  the giant serial test still pass (no new failure site besides the one
  pre-existing known failure at `dashboard-acceptance.spec.ts:961` ->
  `:3779`, byte-identical to baseline per Phase 1's Result).
- Per-assertion non-vacuity: mutate `notificationAvailability`'s branches
  (e.g. swap the order back, or hardcode a return) and confirm each of the
  four unit assertions fails at its own site. For the browser step, mutate the
  `disabled={insecureContext}` binding away (or drop the `isSecureContext`
  override in the test) and confirm the new `toBeDisabled()` assertion fails
  there, not vacuously.
- Confirm the four existing tests in `agent-attention-notification.spec.ts`
  stay green and unaffected (they run on `localhost`, a secure context, so
  `insecureContext` is `false` there and the new `disabled` attribute never
  activates in that file).

## Escalations

- None.
