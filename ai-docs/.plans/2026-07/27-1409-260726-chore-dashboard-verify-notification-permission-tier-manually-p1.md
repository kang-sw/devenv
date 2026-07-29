# Plan: 260726-chore-dashboard-verify-notification-permission-tier-manually — Phase 1: browser gate for the Tier 2 notification path

## Relevant Ticket Contract

- Add Tier 2 coverage as a NEW sibling spec under `e2e/` (suggested
  `agent-attention-notification.spec.ts`) carrying a FILE-level
  `test.use({ channel: "chromium" })`. Not a `test.describe` inside
  `agent-attention-indicator.spec.ts` — that shape is a Playwright load error
  (`channel` is `{ scope: "worker", option: true }`).
- The new file stands up its own daemon, `WS_DASHBOARD_STATE_HOME`, workRoot,
  and owner pairing, following `agent-spawn-profile.spec.ts` and the
  `beforeAll`/`afterAll` shape of `agent-attention-indicator.spec.ts`. It pairs
  fresh in its first test; it does not borrow another file's `ownerCookies`.
- The two helpers both files need — the on-disk callback-token read and the
  direct turn-state POST — are lifted into a shared module under `e2e/`
  alongside `daemonHarness.ts` rather than copy-pasted.
- The `Notification` global is observed through a `Proxy` with a `construct`
  trap that records arguments then `Reflect.construct`s the real constructor.
  Never a stub class. Installed with `page.addInitScript`, before the first
  `goto`.
- **Assertion placement, load-bearing for assertions 4 and 6:** the recorded
  construction list is per-document; `page.reload()` empties it. Any counted
  window must open and close inside ONE document. A reload while the aggregate
  is already `ready` also re-fires Tier 2 by design, so a mid-window reload can
  both erase and add constructions.
- Non-vacuity is a precondition **assertion**, not an assumption: the gate must
  assert in-page `Notification.permission === "granted"` before asserting a
  fire.
- Verification boundary: Playwright green under `npm run test:browser`, plus a
  per-assertion non-vacuity run. Assertions 4 and 6 mutation runs are
  **mandatory, not sampled**.
- Deferred: no OS-display assertion, no native dialog, no LAN origin, no
  service-worker/push path.

**Operative rule quoted from the `ws-web-dashboard` mental model's
`## Common Mistakes` (index.md:251) — do not re-derive it:**

> `playwright.config.ts` now declares `globalSetup: "./e2e/globalSetup.ts"`,
> which builds the production frontend unconditionally before any test starts
> and hard-fails the run on a non-zero build exit, so the "always rebuild
> manually" discipline is superseded on the ordinary spawned-daemon path
> regardless of how Playwright was invoked. It is NOT superseded on the two
> paths where that setup skips the build [...]: `WS_DASHBOARD_STATIC_DIR` set,
> or external mode (`WS_DASHBOARD_DAEMON_MODE=external` /
> `WS_DASHBOARD_DAEMON_BASE_URL` / `WS_DASHBOARD_DAEMON_PAIRING_URL`). [...]
> Under either skip, a `frontend/src` mutation intended as evidence still needs
> its own `npm run build` first. The staleness is scoped to the daemon-served
> production bundle only — `*.spec.ts` files are read fresh off disk by
> Playwright's own runner on every invocation [...]. The daemon binary keeps the
> original shape of this hazard in full: `cargo build -p ws-dashboard-daemon`
> still lives only in the `test:browser` script.

This **corrects a stale sentence in the ticket's own Constraints** ("Only
`npm run test:browser` chains the build; a bare `npx playwright test` serves
whatever bundle is on disk"). Verified directly against source:
`ws-dashboard/frontend/playwright.config.ts:17` declares the global setup and
`e2e/globalSetup.ts:77-99` runs `npm run build` unconditionally unless one of
the two documented skip conditions fires. Practical effect on this phase: the
mutation protocol below does **not** need a manual `npm run build` between each
mutation and its Playwright run, provided neither skip env var is set.

## Out of Scope

- Phase 2 entirely: the `currentNotificationAvailability()` reorder
  (`settingsSections.tsx:109-116`), its comment, the exported pure availability
  function, the disabled-control decision, and the two spec-anchor amendments.
  Phase 1 depends on none of it, and it must not be touched here.
- The `## Human verification residue` steps 1-6. No agent may mark them
  observed or infer them from a green run. Phase 1 landing does **not** close
  this ticket.
- Any change to Tier 1 (`document.title`/favicon), to
  `shouldFireAttentionNotification`'s semantics, or to the `new Notification`
  call site's load-bearing `catch` (`App.tsx:2252-2265`).
- A real plain-http LAN origin gate, a native permission dialog, and any
  assertion about the OS painting a banner.

## Codebase Findings

### Verification boundary facts (confirmed, not assumed)

- `ws-dashboard/frontend/package.json` — the browser gate script is
  `"test:browser": "npm run build && (cd .. && cargo build -p ws-dashboard-daemon) && playwright test"`,
  run from `ws-dashboard/frontend`. Confirmed against the file; the ticket's
  assumption holds for the script name and location.
- **Chromium cache — VERIFIED PRESENT, no escalation.**
  `playwright-core@1.60.0`'s `browsers.json` pins `chromium` at revision
  **1223**. `~/Library/Caches/ms-playwright/` holds `chromium-1223`,
  `chromium-1228`, `chromium_headless_shell-1223`, `chromium_headless_shell-1228`,
  each with `INSTALLATION_COMPLETE`. `chromium.executablePath({ channel: "chromium" })`
  resolves to
  `~/Library/Caches/ms-playwright/chromium-1223/chrome-mac-arm64/Google Chrome for Testing.app/Contents/MacOS/Google Chrome for Testing`,
  and that binary exists on disk. No download step is required.
- Known pre-existing failure site on this branch:
  `e2e/dashboard-acceptance.spec.ts:3779` (terminal short-viewport row guard).
  Judge the final run by failure SITE, not by exit code.

### Existing precedent the new file must follow

- `ws-dashboard/frontend/e2e/agent-attention-indicator.spec.ts#L127-L196` — the
  exact `beforeAll`/`afterAll` shape to copy. Module-level `let daemon`,
  `let workRoot`, `let stateHome`, `let previousStateHome`;
  `test.describe.configure({ mode: "serial" })` at :148; `beforeAll` mkdtemps
  the workRoot, writes a `readme.txt` marker, saves and overwrites
  `process.env.WS_DASHBOARD_STATE_HOME` with a mkdtemp under
  `socketSafeTempBase()` (`/tmp` on darwin, for the `sockaddr_un.sun_path`
  ceiling), and only then `await startDaemon()`. `afterAll` stops the daemon,
  `rmSync`s every temp dir, and restores or deletes the saved
  `WS_DASHBOARD_STATE_HOME`.
- `e2e/agent-spawn-profile.spec.ts#L91-L123` — the same shape with one workRoot;
  the simpler template if the new file needs only one fixture root.
- **`pairOwner` call shape** — the new file needs the *cookie-capturing* variant
  (`agent-attention-indicator.spec.ts#L198-L203`), not the spawn-profile variant
  (`agent-spawn-profile.spec.ts#L125-L129`), because it has more than one
  `test()` and the daemon's pairing URL is one-time:

  ```ts
  async function pairOwner(page: Page) {
    await page.goto(daemon.pairingUrl, { waitUntil: "domcontentloaded" });
    await expect(page.locator(".app-shell")).toBeVisible();
    expect(new URL(page.url()).pathname).not.toContain("/pair");
    ownerCookies = await page.context().cookies(daemon.baseUrl);
  }
  ```

  plus `attachOwnerSession` (`#L205-L214`) verbatim for tests 2-4.
- `agent-attention-indicator.spec.ts#L216-L241` (`openWorkRootMinimal`,
  `selectWorkRootMinimal`), `#L243-L267` (`resolveWorkRootId`), `#L269-L282`
  (`terminalTabsLocator`, `terminalTab`), `#L339-L378` (`closeTerminalById`,
  `forceCloseTerminals`), `#L385-L424` (`spawnTerminalInRoot`) — copy these into
  the new file. The ticket only mandates lifting the **two** helpers named
  below; these others are cheap, stable, and duplicating them keeps the blast
  radius on the existing passing file minimal.
- `agent-attention-indicator.spec.ts#L1154-L1256` — the Tier 1 test. Closest
  structural precedent for the new file. Note `#L1179`: after
  `spawnTerminalInRoot` (a direct authenticated fetch that bypasses React
  state), a `page.reload()` + `selectWorkRootMinimal` + wait-for-tab is
  **required** before the terminal pane exists in the DOM. **This is exactly the
  reload the ticket's assertion-placement rule is about** — in the new file it
  must land before any counted window opens and before any turn-state POST.
- `agent-attention-indicator.spec.ts#L1205-L1209` — the propagation gate idiom:
  `await expect(terminalTab(page, id)).toHaveAttribute("data-attention-state", "working"|"ready"|"none", { timeout: 20_000 })`.
  Reuse it before every count assertion so the count is not read while the
  transition is still in flight.

### The two helpers to lift (this is the only edit to an existing passing file)

Both are module-local and unexported today, and both close over module state:

- **`readCallbackToken`** —
  `e2e/agent-attention-indicator.spec.ts#L293-L311` (comment block at :293-296,
  function at :297-311). Closes over `stateHome`. Path construction mirrors
  `agent_token_store.rs::token_store_path`:
  ``path.join(stateHome, "terminal-tokens", `${terminalId}.json`)``.
- **`postTurnState`** —
  `e2e/agent-attention-indicator.spec.ts#L313-L337` (comment block at :313-316,
  function at :317-337). Closes over `daemon.baseUrl`. POSTs
  `{ token, state }` as a JSON body to
  `/api/dashboard/terminals/${terminalId}/turn-state` from the Node side
  (never `page.evaluate`, never a query string, never logged) and asserts
  status `204`.

Destination: a new `ws-dashboard/frontend/e2e/agentTurnState.ts`, sibling to
`daemonHarness.ts`. Move the existing comment blocks with the functions — they
carry the token-handling hard constraint and the "registered outside
`require_owner_auth`" rationale.

New signatures (the closed-over values become leading parameters):

```ts
export function readCallbackToken(stateHome: string, terminalId: string): string;
export async function postTurnState(
  baseUrl: string,
  terminalId: string,
  token: string,
  state: "working" | "ready" | "idle",
): Promise<void>;
```

**Call sites in the existing file, and what they become.** There are 6
`readCallbackToken(...)` call sites (:486, :610, :814, :815, :1046, :1173) and 9
`postTurnState(...)` call sites (:487, :649, :673, :710, :840, :841, :947,
:1065, :1199). **Do not rewrite those 15 lines.** The lowest-risk extraction
keeps them byte-identical by leaving two thin module-local binders in place of
the deleted definitions:

```ts
import {
  readCallbackToken as readCallbackTokenFrom,
  postTurnState as postTurnStateTo,
} from "./agentTurnState.js";

// Binds this file's module-local `stateHome` / `daemon.baseUrl`, so the 15
// existing call sites below are unchanged by the extraction.
function readCallbackToken(terminalId: string): string {
  return readCallbackTokenFrom(stateHome, terminalId);
}
async function postTurnState(
  terminalId: string,
  token: string,
  state: "working" | "ready" | "idle",
) {
  await postTurnStateTo(daemon.baseUrl, terminalId, token, state);
}
```

One further edit is required: `readFileSync` becomes unused in
`agent-attention-indicator.spec.ts:3` and must be dropped from that import
(`mkdtempSync`, `rmSync`, `writeFileSync` all remain used).

**Regression risk, stated plainly.** This is the only change to a currently
green 56 KB spec file with 5 tests, and the failure it can produce is quiet: a
wrong `stateHome` binding makes `readCallbackToken` throw `ENOENT` in
`test.step`s far from the extraction, and a wrong `baseUrl` binding makes
`postTurnState` fail its own `expect(response.status).toBe(204)`. Both surface
as failures inside `agent-attention-indicator.spec.ts`, which the ticket
explicitly asks be confirmed unchanged. Mitigations, in order:
1. Prefer the binder shape above (zero call-site edits) over rewriting 15 lines.
2. `stateHome` and `daemon` are assigned in `beforeAll` and must be read lazily
   *inside* the binders, so the binders must stay **functions**, never
   module-initialization-time partial applications.
3. The final `npm run test:browser` must show all 5 tests of that file green;
   any new failure there is the extraction, not the browser build (that file
   stays on the default channel).

`tsconfig.e2e-tests.json` includes only `e2e/daemonHarness.ts` and
`e2e/daemonHarness.test.ts`, so the new `agentTurnState.ts` is **not** covered by
`npm run test:terminals`' tsc pass. Playwright transpiles spec imports itself.
Adding it to that `include` list is optional and not required by this phase.

### The app code under test — literal strings assertion 3 must assert

`ws-dashboard/frontend/src/App.tsx#L2237-L2268` — the entire Tier 2 effect:

```tsx
const previousGlobalAttentionToneRef = useRef<AttentionTone>(null);
useEffect(() => {
  if (
    shouldFireAttentionNotification(
      previousGlobalAttentionToneRef.current,
      globalAttentionTone,
    ) &&
    notificationPrefs.enabled &&
    typeof Notification !== "undefined" &&
    Notification.permission === "granted"
  ) {
    try {
      new Notification("ws dashboard", {
        body: "An agent is ready for your input.",
      });
    } catch {
      /* ... load-bearing, do not remove ... */
    }
  }
  previousGlobalAttentionToneRef.current = globalAttentionTone;
}, [globalAttentionTone, notificationPrefs.enabled]);
```

**Shipped strings, to be asserted literally:**
- title: `"ws dashboard"` (`App.tsx:2249`)
- body: `"An agent is ready for your input."` (`App.tsx:2250`)

Supporting facts:
- `src/browserAttentionCue.ts#L83-L88` —
  `shouldFireAttentionNotification(previousTone, currentTone)` returns
  `previousTone !== "ready" && currentTone === "ready"`.
- `src/App.tsx#L2142-L2165` — `globalAttentionTone` is a `useMemo` over
  `aggregateNavAttentionTone(agentAttentionByRoot, rootKeys)`, deps
  `[serverConnections, resourcesByServer, workNavOrder.hiddenWorktreesByWorkspace, agentAttentionByRoot]`.
- `src/agentAttention.ts#L201-L264` — the aggregate is `ready` if **any** agent
  pane in any visible root has an unacknowledged `ready` entry; `ready` outranks
  `working` outranks none. So a second agent reaching `ready` while one already
  is leaves the aggregate string at `"ready"`.
- **Non-obvious constraint that shapes assertion 4's mutation (below):** because
  `globalAttentionTone` is a memoized *string*, an A-ready → B-ready sequence
  does not change the effect's dependency array at all, so the effect never
  re-runs. The ready→ready suppression is enforced by the memoized aggregate +
  dep array **together with** `previousGlobalAttentionToneRef`, not by
  `shouldFireAttentionNotification` alone. `browserAttentionCue.test.ts` cannot
  reach this; assertion 4 genuinely covers new ground.
- `src/settingsSections.tsx#L129-L186` — `NotificationSection`. Checkbox is
  `label.settings-notification-toggle > input[type="checkbox"]`, `checked={enabled}`.
  Its `onChange` calls `onChange(next)`, then for `next === true` and a defined
  `Notification`, `Promise.resolve(Notification.requestPermission()).then(p => { if (p === "denied") onChange(false); forceRerender(...) })`.
- `src/App.tsx#L2081-L2085` — `handleNotificationPrefsChange` is the single write
  path: `setNotificationPrefs(next)` then `saveNotificationPrefs(next)`.
- `src/notificationPrefs.ts:22` + `src/settingsStore.ts:51-65` — persisted under
  `localStorage["ws-dashboard.settings.notifications.v1"]` as
  `{"version":1,"value":{"enabled":<bool>}}`. `loadNotificationPrefs()` is read
  once in a `useState` initializer (`App.tsx:553-554`), so a reload re-reads it.
- Settings UI path: topbar `[data-command-id="settings.open"]`
  (`App.tsx:3266-3275`) opens `.settings-modal` (`settingsModal.tsx:51`); the
  section nav is `.settings-section-nav-button` with text `Notifications`; close
  via `[data-command-id="settings.close"]`.

### Non-obvious constraints

- Playwright permission grants are per-context and every `test()` gets a fresh
  context — so a granted test and a non-granted test coexist in one file. Fresh
  context also means fresh `localStorage`, so **each test must re-enable the
  Notifications toggle for itself**; the pref does not leak between tests.
- `daemon.baseUrl` is only known after `beforeAll` (port 0), so the grant cannot
  be expressed as a static file-level `test.use({ permissions: [...] })` with an
  origin. Use `await page.context().grantPermissions(["notifications"], { origin: daemon.baseUrl })`
  inside each test that needs it, before the first `goto`.
- The `dummy-echo-hooked` spawn profile is what makes `TerminalSession::spawn`
  mint a callback token; the hook can never fire on its own (the command is
  `/bin/sh`). No vendor CLI, credentials, or network.
- That profile sleeps 180s. `forceCloseTerminals` in a `finally` is mandatory or
  the run orphans `terminal-helper`/`sh`/`sleep` processes — the daemon does not
  terminate live sessions on shutdown.
- `Notification.requestPermission` and `Notification.permission` are **statics**
  read through the `Proxy`. The ticket measured that proxied and unproxied
  `permission` reads agree. A defensive `get` trap that forwards with the real
  constructor as receiver is included below so a static-accessor `this` check
  cannot break `requestPermission()`.

## Implementation Plan

### Step 1 — extract the two shared helpers

Create `ws-dashboard/frontend/e2e/agentTurnState.ts` containing
`readCallbackToken(stateHome, terminalId)` and
`postTurnState(baseUrl, terminalId, token, state)`, moving the existing comment
blocks with them. Then edit `e2e/agent-attention-indicator.spec.ts`: delete
`:293-337`, add the import plus the two binder functions shown in Codebase
Findings, and drop `readFileSync` from the `node:fs` import on line 3. **Change
nothing else in that file.**

### Step 2 — create `e2e/agent-attention-notification.spec.ts`

File-level, before any `test()`:

```ts
test.use({ channel: "chromium" });
test.describe.configure({ mode: "serial" });
```

Module-level `let daemon / workRoot / stateHome / previousStateHome / ownerCookies`,
plus `beforeAll`/`afterAll` copied from `agent-attention-indicator.spec.ts#L150-L196`
(one workRoot is enough), and `pairOwner` / `attachOwnerSession` /
`openWorkRootMinimal` / `selectWorkRootMinimal` / `resolveWorkRootId` /
`terminalTab` / `closeTerminalById` / `forceCloseTerminals` / `spawnTerminalInRoot`
copied over. Import the two lifted helpers from `./agentTurnState.js` and bind
them the same way.

Add a CONTRACT header comment explaining: why a sibling file
(`channel` is worker-scoped, a `test.describe` `use` is a load error), why it
pairs its own owner session (its own daemon, its own unconsumed one-time
pairing URL), and the assertion-placement rule (counted windows never span a
reload).

### Step 3 — the `Proxy` recorder, installed with `addInitScript`

Call this on the `page` **before the first `goto`** in every test:

```ts
async function installNotificationRecorder(page: Page) {
  await page.addInitScript(() => {
    // Per-document recorder. A page.reload() builds a fresh JS realm and this
    // array starts empty again, which is exactly why every counted window in
    // this file opens and closes inside ONE document.
    const constructions: Array<{ title: unknown; body: unknown }> = [];
    (window as unknown as Record<string, unknown>).__wsNotificationConstructions =
      constructions;
    const real = (window as unknown as { Notification?: unknown }).Notification;
    if (typeof real !== "function") {
      return;
    }
    const proxied = new Proxy(real as unknown as new (...args: unknown[]) => unknown, {
      // Record, then run the REAL constructor: a browser that refuses to
      // construct still throws into App.tsx's existing catch, and the app is
      // never observed talking to a test double.
      construct(target, args, newTarget) {
        constructions.push({
          title: args[0],
          body:
            args[1] && typeof args[1] === "object"
              ? (args[1] as Record<string, unknown>).body
              : undefined,
        });
        return Reflect.construct(target, args, newTarget);
      },
      // Defensive only: forwards static reads (`permission`,
      // `requestPermission`) with the REAL constructor as receiver, so a
      // static accessor or method that checks its `this` cannot observe the
      // Proxy. Measured behaviour is that plain forwarding already agrees;
      // this only removes the remaining failure mode.
      get(target, prop) {
        const value = Reflect.get(target, prop, target);
        return typeof value === "function" ? value.bind(target) : value;
      },
    });
    Object.defineProperty(window, "Notification", {
      configurable: true,
      writable: true,
      value: proxied,
    });
  });
}
```

Reading it out of the page:

```ts
async function recordedNotifications(page: Page) {
  return await page.evaluate(
    () =>
      (window as unknown as {
        __wsNotificationConstructions?: Array<{ title: unknown; body: unknown }>;
      }).__wsNotificationConstructions ?? null,
  );
}
```

A `null` return means the init script never ran for the current document — treat
that as a hard failure, not as "zero notifications". Every count assertion below
must first assert the array is non-`null`.

Shared toggle helper (used by several tests):

```ts
async function openNotificationSettings(page: Page) {
  await page.locator('[data-command-id="settings.open"]').click();
  const modal = page.locator(".settings-modal");
  await expect(modal).toBeVisible();
  await modal
    .locator(".settings-section-nav-button", { hasText: "Notifications" })
    .click();
  const checkbox = modal.locator(
    '.settings-notification-toggle input[type="checkbox"]',
  );
  await expect(checkbox).toBeVisible();
  return { modal, checkbox };
}
```

The caller clicks the checkbox, asserts, then closes with
`modal.locator('[data-command-id="settings.close"]').click()`.

### Step 4 — the seven assertions as a four-test file structure

Reloads are called out explicitly per test. **Every `page.reload()` in this file
happens before any turn-state POST**, i.e. while the aggregate tone is still
`null`, so no reload can either erase a counted construction or add one via the
"reload while already ready re-fires by design" behaviour.

---

**Test 1 — `"permission is granted and the Notifications toggle persists"`**
(assertions **1**, **2**). Reloads: **none**.

1. `await page.context().grantPermissions(["notifications"], { origin: daemon.baseUrl })`
   — must precede the first `goto`; `daemon.baseUrl` is known by now.
2. `await installNotificationRecorder(page)`.
3. `await pairOwner(page)` — this is the file's one pairing, and it captures
   `ownerCookies` for tests 2-4.
4. **Assertion 1 (precondition, asserted not assumed):**
   ```ts
   expect(await page.evaluate(() => typeof Notification)).toBe("function");
   expect(
     await page.evaluate(() => Notification.permission),
     "channel:'chromium' must report a granted notification permission; under the default chromium-headless-shell this reads 'denied' and every negative assertion in this file would pass vacuously",
   ).toBe("granted");
   ```
5. **Assertion 2:** open the section, click the checkbox on; then
   `await expect(checkbox).toBeChecked()` — it must *stay* checked, because with
   the permission pre-granted `requestPermission()` resolves `"granted"` and the
   denied-reconciliation `onChange(false)` must not fire — and
   ```ts
   expect(
     await page.evaluate(() =>
       window.localStorage.getItem("ws-dashboard.settings.notifications.v1"),
     ),
   ).toBe('{"version":1,"value":{"enabled":true}}');
   ```
6. Close the modal.

---

**Test 2 — `"a ready edge fires exactly one notification; a second agent reaching ready fires none"`**
(assertions **1**, **3**, **4**). Reload: **exactly one**, in step 4, before the
counted window opens.

1. `grantPermissions` → `installNotificationRecorder` → `attachOwnerSession`.
2. `openWorkRootMinimal(page, workRoot)`; `resolveWorkRootId`.
3. `spawnTerminalInRoot(..., "dummy-echo-hooked", "Notify Agent A")` and
   `... "Notify Agent B"`; read both callback tokens.
4. **The one reload** (`page.reload({ waitUntil: "domcontentloaded" })`) +
   `selectWorkRootMinimal` + `await expect(terminalTab(page, idA)).toHaveCount(1)`
   and the same for B. Required because the two spawns were direct fetches that
   bypassed React state (precedent: `agent-attention-indicator.spec.ts:1179`).
   **The counted window has not opened yet.**
5. Enable the Notifications toggle, assert checked, close the modal.
   (Fresh context ⇒ fresh `localStorage` ⇒ the pref must be set again here.)
6. **Assertion 1 restated in this document:** `Notification.permission === "granted"`,
   and `recordedNotifications(page)` is a non-`null` array of length `0`.
   **The counted window opens here.**
7. `postTurnState(idA, tokenA, "ready")`; gate on
   `expect(terminalTab(page, idA)).toHaveAttribute("data-attention-state", "ready", { timeout: 20_000 })`.
8. **Assertion 3:**
   ```ts
   await expect
     .poll(async () => (await recordedNotifications(page))?.length, { timeout: 10_000 })
     .toBe(1);
   const [first] = (await recordedNotifications(page))!;
   expect(first.title).toBe("ws dashboard");
   expect(first.body).toBe("An agent is ready for your input.");
   ```
9. `postTurnState(idB, tokenB, "ready")`; gate on B's tab reaching
   `data-attention-state="ready"`. **This gate is what stops assertion 4 from
   passing merely because the second transition never landed** — the second
   `ready` is proven to have reached the browser before the count is re-read.
10. `await page.waitForTimeout(1_500)` (settle past the ~1s Tier 1 flash tick and
    any late React commit).
11. **Assertion 4:**
    ```ts
    const after = await recordedNotifications(page);
    expect(after, "the recorder must still exist in this document").not.toBeNull();
    expect(
      after!.length,
      "a second agent reaching ready while the aggregate is already ready must not construct another Notification",
    ).toBe(1);
    ```
    **Why this cannot pass on an empty array:** the same array, in the same
    document, already had to reach length 1 at step 8 for the test to get here.
    `toBe(1)` fails on `0` as loudly as it fails on `2`. No reload separates
    step 8 from step 11.
12. `finally { await forceCloseTerminals(page, [idA, idB]); }`.

---

**Test 3 — `"ready → working → ready fires a second; turning the toggle off gates a third"`**
(assertions **5**, **6**). Reload: **exactly one**, in step 4, before the counted
window opens.

1-4. Same as Test 2 but with **one** agent terminal (`idA`, `tokenA`).
5. Enable the Notifications toggle, assert checked, close.
6. **Assertion 1 restated**; recorder array non-`null`, length `0`.
   **Counted window opens.**
7. `postTurnState(idA, tokenA, "ready")`; gate `data-attention-state="ready"`;
   `expect.poll(length).toBe(1)`.
8. `postTurnState(idA, tokenA, "working")`; gate `data-attention-state="working"`.
   Gating on `working` explicitly is required — without it, the `ready` wait in
   step 9 would pass immediately against the stale `ready` from step 7 and the
   edge would never be proven to have happened.
9. `postTurnState(idA, tokenA, "ready")`; gate `data-attention-state="ready"`.
10. **Assertion 5:** `await expect.poll(length, { timeout: 10_000 }).toBe(2)` —
    the edge detector is not a one-shot latch.
11. Open the section again, click the checkbox off; assert
    `await expect(checkbox).not.toBeChecked()` and that the persisted value is now
    `'{"version":1,"value":{"enabled":false}}'`; close the modal.
    **No reload here** — the counted window is still open.
12. `postTurnState(idA, tokenA, "working")` (gate `working`) then
    `postTurnState(idA, tokenA, "ready")` (gate `ready`) — a genuine
    `working → ready` edge with the permission still granted and the pref off.
13. `await page.waitForTimeout(1_500)`.
14. **Assertion 6:**
    ```ts
    const after = await recordedNotifications(page);
    expect(after).not.toBeNull();
    expect(
      after!.length,
      "with the Notifications pref off (permission still granted) a ready edge must not construct a Notification",
    ).toBe(2);
    ```
    **Why this cannot pass on an empty array:** the same array, same document,
    already reached length 2 at step 10. A recorder that silently stopped
    working, or a reload that emptied the list, produces `0` and fails here.
    The permission is still `"granted"` in this document (asserted at step 6 and
    never revoked), so this isolates the pref as the gate rather than the
    permission.
15. `finally { await forceCloseTerminals(page, [idA]); }`.

---

**Test 4 — `"without a grant the toggle reconciles to unchecked and a ready edge fires nothing"`**
(assertion **7**). Reload: **exactly one**, before the turn-state POST.

1. **No `grantPermissions` call.** `installNotificationRecorder` →
   `attachOwnerSession`.
2. `openWorkRootMinimal`; `resolveWorkRootId`; spawn one hooked terminal; read
   its token.
3. Reload + `selectWorkRootMinimal` + wait for the tab.
4. **Inverse precondition, asserted:**
   `expect(await page.evaluate(() => Notification.permission)).not.toBe("granted")`
   (Playwright's measured value here is `"denied"`; assert `not "granted"` so the
   test does not become brittle on `"default"`).
5. Click the toggle on. `requestPermission()` resolves `"denied"`, which fires
   `onChange(false)`.
   **Assertion 7a:** `await expect(checkbox).not.toBeChecked({ timeout: 10_000 })`
   — a polled form, because the uncheck arrives asynchronously after the promise
   settles.
   **Assertion 7b:** persisted value is `'{"version":1,"value":{"enabled":false}}'`.
   Close the modal.
6. `postTurnState(id, token, "ready")`; gate `data-attention-state="ready"`;
   `await page.waitForTimeout(1_500)`.
   **Assertion 7c:** recorder array non-`null` and length `0`.
   Stated honestly: 7c is the one assertion in the file that cannot be made
   non-vacuous by construction — the whole point is that nothing fires. Its
   value comes from the non-`null` recorder check plus the fact that the
   identical recorder demonstrably records in tests 2 and 3. The load-bearing,
   mutation-provable halves of assertion 7 are 7a and 7b.
7. `finally { await forceCloseTerminals(page, [id]); }`.

## Verification Plan

### Baseline

From `ws-dashboard/frontend`, capturing exit status on the line **after** the
redirect, in the same bash invocation, never through a pipe:

```
npm run test:browser > /tmp/tier2-baseline.log 2>&1
echo $?
```

Expected: every test green except the known pre-existing site
`e2e/dashboard-acceptance.spec.ts:3779`. Judge by failure SITE. This run also
builds the daemon binary (`cargo build -p ws-dashboard-daemon`), which the
mutation runs below rely on and do not rebuild.

### Per-assertion non-vacuity protocol

Every mutation run is:

```
<apply the one mutation>
npx playwright test e2e/agent-attention-notification.spec.ts > /tmp/tier2-mutN.log 2>&1
echo $?
<revert the mutation>
```

`e2e/globalSetup.ts` rebuilds `frontend/dist` unconditionally at the start of
each of these runs (see the quoted rule above), so **no manual `npm run build`
is needed** — but only while `WS_DASHBOARD_STATIC_DIR` and the three
external-mode variables are all unset. Confirm each run's stdout carries
`[e2e globalSetup] building the production frontend ...` and **not** a
`skipping the frontend build` line; if a skip line appears, the run is
invalid — unset the env var and rerun. Pass criterion for a mutation run: the
targeted assertion fails **at its own site**, and no unrelated assertion in the
file becomes the first failure.

| # | Assertion | Mutate | Expected failing site |
|---|---|---|---|
| 1 | permission is `"granted"` | **Spec file only**: delete `test.use({ channel: "chromium" })` from `e2e/agent-attention-notification.spec.ts`. No `frontend/src` edit; `*.spec.ts` is read fresh so no rebuild question arises. | Test 1's precondition assertion; the default `chromium-headless-shell` reports `"denied"`. |
| 2 | toggle-on persists | `src/App.tsx:2084` — delete `saveNotificationPrefs(next);` | Test 1's `localStorage` assertion. |
| 3 | exactly one, literal strings | `src/App.tsx:2250` — change the body literal to e.g. `"MUTATED"` | Test 2 step 8's `expect(first.body).toBe(...)`. |
| **4** | ready→ready adds nothing (**mandatory**) | `src/App.tsx`, **two lines**, see note below: (a) `:2268` → `}, [globalAttentionTone, notificationPrefs.enabled, agentAttentionByRoot]);`; (b) delete `previousGlobalAttentionToneRef.current = globalAttentionTone;` at `:2267` | Test 2 step 11 — the list reaches 2. |
| 5 | ready→working→ready fires again | `src/browserAttentionCue.ts:87` — change to `return previousTone === null && currentTone === "ready";` (a one-shot latch) | Test 3 step 10 — the list stays at 1. Test 2 still passes (its first fire has `previousTone === null`), which is the point. |
| **6** | pref gates the fire (**mandatory**) | `src/App.tsx:2244` — delete `notificationPrefs.enabled &&` | Test 3 step 14 — the list reaches 3. |
| 7 | denied reconciliation | `src/settingsSections.tsx:163` — delete `onChange(false);` | Test 4's 7a (`not.toBeChecked`) and 7b (persisted `enabled:true`). |

**Note on the assertion-4 mutation, stated rather than hidden.** It is the only
two-line mutation in the table, and that is a property of the code, not
sloppiness: as recorded in Codebase Findings, an A-ready → B-ready sequence
leaves `globalAttentionTone` at the same memoized string, so the Tier 2 effect
never re-runs and no single-line change to the predicate can make it fire twice.
The guard assertion 4 targets is the conjunction {memoized aggregate in the dep
array, `previousGlobalAttentionToneRef`}; mutating both members is the minimum
that isolates it. Record this in the phase Result — it is also the reason
assertion 4 covers ground `browserAttentionCue.test.ts` structurally cannot.

### Final

Revert every mutation, confirm `git status` shows none of them surviving, then:

```
npm run test:browser > /tmp/tier2-final.log 2>&1
echo $?
```

Required: the 5 tests in `agent-attention-indicator.spec.ts` are green (this is
the extraction regression check — that file stays on the default channel, so any
failure there is the helper lift, not the browser build), the 4 new tests are
green, and `dashboard-acceptance.spec.ts` shows no failure site other than the
known `:3779`.

### Budget

**9 Playwright invocations total: 1 baseline full run + 7 single-file mutation
runs + 1 final full run.** Six of the seven mutation runs touch `frontend/src`
(assertion 1's touches only the spec file). Each single-file run costs one
`npm run build` (paid automatically by `globalSetup`), one daemon boot, and the
4 tests of the new file — it does **not** re-run `dashboard-acceptance.spec.ts`,
which is where the wall-clock cost lives. No subset needs to be dropped: the
full seven-assertion protocol is affordable at this shape, and assertions 4 and
6 — the two the ticket marks mandatory — are both single-file runs.

## Escalations

- None. The one factual risk the survey was asked to settle — whether the full
  Chromium build required by `channel: "chromium"` is present locally — is
  confirmed present at `chromium-1223` (revision 1223, matching
  `playwright-core@1.60.0`'s `browsers.json`), with the
  `Google Chrome for Testing` binary on disk and `INSTALLATION_COMPLETE`
  written. No download step, no unattended-install problem.
- One documentation drift worth recording in the phase Result: the ticket's
  Constraints section states "Only `npm run test:browser` chains the build",
  which `playwright.config.ts:17` + `e2e/globalSetup.ts:77-99` have since
  superseded on the spawned-daemon path. The mental model (index.md:251) is
  current; the ticket text is not. The plan follows the code.
