import { test, expect, type Page } from "@playwright/test";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { startDaemon, type DaemonHandle } from "./daemonHarness.js";
import {
  readCallbackToken as readCallbackTokenFrom,
  postTurnState as postTurnStateTo,
} from "./agentTurnState.js";
import {
  terminalCloseEndpoint,
  workRootTerminalsEndpoint,
} from "../src/terminals.js";

// Browser-level acceptance gate for the 260725 Phase 8 Tier 2 cue: the
// `new Notification(...)` the dashboard constructs when the GLOBAL agent
// attention tone rises to `ready`. Before this file, the whole Tier 2
// permission path - `App.tsx`'s effect, `NotificationSection`'s onChange
// handler, and `currentNotificationAvailability()` - had zero coverage of any
// kind; it had only ever been reasoned about.
//
// CONTRACT - why this is a SIBLING file and not a `test.describe` inside
// `agent-attention-indicator.spec.ts`:
//   * Tier 2 needs a real browser build. Playwright's default Chromium
//     download for this project is the headless shell, which reports
//     `Notification.permission === "denied"` no matter what the context
//     grants - under which EVERY negative assertion in this file would pass
//     vacuously. The fix is `channel: "chromium"` (the full Chrome for
//     Testing binary), and `channel` is declared
//     `{ scope: "worker", option: true }`, so it can only be set at FILE
//     level. A `test.describe(...)` with a nested `test.use({ channel })` is
//     a Playwright load error, not a slower path.
//   * Because it is its own file it stands up its own daemon, its own
//     `WS_DASHBOARD_STATE_HOME`, and its own workRoot, and therefore pairs
//     its own owner session: the daemon's pairing URL is ONE-TIME, and this
//     file's daemon has its own unconsumed one. It never borrows another
//     spec file's `ownerCookies`.
//
// ASSERTION-PLACEMENT RULE, load-bearing - read before editing any test here:
// the `Notification` construction recorder installed by
// `installNotificationRecorder` is PER DOCUMENT. A `page.reload()` builds a
// fresh JS realm and the recorded list starts empty again. Two assertions in
// this file ("a second agent reaching ready constructs nothing more", "with
// the pref off a ready edge constructs nothing more") are counted as "the list
// is STILL n", and a reload landing inside their window would satisfy them
// against an empty list no matter what the app did. So: every counted window
// opens and closes inside ONE document, and every `page.reload()` in this file
// happens BEFORE any turn-state POST - i.e. while the aggregate tone is still
// `null`, which also rules out the second hazard (reloading while the
// aggregate is already `ready` re-fires Tier 2 by design and would ADD a
// construction). Each "constructs nothing more" assertion reads a list that
// demonstrably reached a NONZERO length earlier in the same document, so it
// fails just as loudly on 0 as on n+1.
//
// Non-vacuity is asserted, never assumed: every test that expects a fire first
// asserts in-page that `Notification.permission === "granted"`, and the one
// test that expects no fire asserts the inverse.
//
// No vendor CLI, credentials, or network: terminals are spawned with the
// always-compiled-in `"dummy-echo-hooked"` test profile, whose only relevant
// property is that it carries a hook config, which is what makes
// `TerminalSession::spawn` mint a real callback token. The hook itself can
// never fire - the dummy command is `/bin/sh`.
//
// TOKEN HANDLING (ticket hard constraint): tokens are read off disk with Node
// `fs` from this run's own state dir and POSTed in a JSON body from the Node
// side - never through `page.evaluate`, never in a query string, never
// printed. See `./agentTurnState.ts`.

// The full Chrome for Testing build. File level, not describe level - see the
// CONTRACT above.
test.use({ channel: "chromium" });
test.describe.configure({ mode: "serial" });

const NOTIFICATION_TITLE = "ws dashboard";
const NOTIFICATION_BODY = "An agent is ready for your input.";
const NOTIFICATION_PREFS_KEY = "ws-dashboard.settings.notifications.v1";
const PREFS_ENABLED = '{"version":1,"value":{"enabled":true}}';
const PREFS_DISABLED = '{"version":1,"value":{"enabled":false}}';

// Settle budget after the last gated transition of a counted window, before a
// "constructs nothing more" count is read: past the ~1s Tier 1 flash tick and
// any late React commit, so the count is not read in the gap where a fire
// would have been about to happen.
const SETTLE_MS = 1_500;

let daemon: DaemonHandle;
let workRoot: string;
let stateHome: string;
let previousStateHome: string | undefined;
// Captured by the first test's pairing and reused by the rest, because the
// pairing URL is one-time. This is why the file stays in serial mode.
let ownerCookies:
  | Awaited<ReturnType<import("@playwright/test").BrowserContext["cookies"]>>
  | undefined;

function socketSafeTempBase(): string {
  // Mirrors the other browser gates (same sockaddr_un.sun_path length ceiling
  // on macOS).
  return process.platform === "darwin" ? "/tmp" : os.tmpdir();
}

function workRootDisplayName(rootPath: string) {
  const normalized = rootPath.replace(/[\\/]+$/, "");
  const match = normalized.match(/[^\\/]+$/);
  return match ? match[0] : normalized;
}

test.beforeAll(async () => {
  workRoot = mkdtempSync(path.join(os.tmpdir(), "ws-dash-notify-"));
  writeFileSync(
    path.join(workRoot, "readme.txt"),
    "agent attention notification browser gate fixture\n",
  );
  previousStateHome = process.env.WS_DASHBOARD_STATE_HOME;
  // `WS_DASHBOARD_STATE_HOME` (`persistent_state.rs::default_state_file`) is
  // inherited by the spawned daemon through `process.env` in
  // `daemonHarness.ts`. Overriding it keeps the on-disk token read
  // deterministic AND keeps this gate from writing into the developer's real
  // `~/.local/state/ws-dashboard/`.
  stateHome = mkdtempSync(
    path.join(socketSafeTempBase(), "ws-dash-notify-state-"),
  );
  process.env.WS_DASHBOARD_STATE_HOME = stateHome;
  daemon = await startDaemon();
});

test.afterAll(async () => {
  if (daemon) {
    await daemon.stop();
  }
  rmSync(workRoot, { recursive: true, force: true });
  rmSync(stateHome, { recursive: true, force: true });
  if (previousStateHome === undefined) {
    delete process.env.WS_DASHBOARD_STATE_HOME;
  } else {
    process.env.WS_DASHBOARD_STATE_HOME = previousStateHome;
  }
});

// Binds this file's module-local `stateHome` / `daemon.baseUrl`. Both are only
// assigned in `beforeAll`, so these must stay functions that read them per
// call rather than module-initialization-time partial applications.
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

async function pairOwner(page: Page) {
  await page.goto(daemon.pairingUrl, { waitUntil: "domcontentloaded" });
  await expect(page.locator(".app-shell")).toBeVisible();
  expect(new URL(page.url()).pathname).not.toContain("/pair");
  ownerCookies = await page.context().cookies(daemon.baseUrl);
}

async function attachOwnerSession(page: Page) {
  if (!ownerCookies) {
    throw new Error(
      "ownerCookies not captured - the owner-pairing test must run first (serial mode) before this test can reuse the session cookie",
    );
  }
  await page.context().addCookies(ownerCookies);
  await page.goto(daemon.baseUrl, { waitUntil: "domcontentloaded" });
  await expect(page.locator(".app-shell")).toBeVisible();
}

async function openWorkRootMinimal(page: Page, rootPath: string) {
  const opener = page.locator(
    '[data-command-id="rootPicker.open"]:not(.open-work-root-empty-cta)',
  );
  await opener.click();
  const modal = page.locator(".root-picker-modal");
  await expect(modal).toBeVisible();
  await modal.locator("#root-picker-exact-path").fill(rootPath);
  await modal
    .locator('[data-command-id="workRoot.open"]')
    .filter({ hasText: "Open" })
    .click();
  await expect(page.locator(".file-explorer-title")).toContainText(
    workRootDisplayName(rootPath),
  );
  await expect(modal).toHaveCount(0);
}

async function selectWorkRootMinimal(page: Page, rootPath: string) {
  const label = workRootDisplayName(rootPath);
  const row = page.locator('.resource-row[data-command-id="resource.select"]', {
    hasText: label,
  });
  await row.first().click();
  await expect(page.locator(".file-explorer-title")).toContainText(label);
}

async function resolveWorkRootId(page: Page, rootPath: string): Promise<string> {
  const label = workRootDisplayName(rootPath);
  const workRootId = await page.evaluate(async (targetLabel) => {
    const response = await fetch("/api/dashboard/resources");
    const resources = (await response.json()) as {
      workspaces?: Array<{
        workRoots?: Array<{
          id?: string;
          label?: string;
          resourcePath?: { workRootId?: string };
        }>;
      }>;
    };
    for (const workspace of resources.workspaces ?? []) {
      for (const candidate of workspace.workRoots ?? []) {
        if (candidate.label === targetLabel) {
          return candidate.resourcePath?.workRootId ?? candidate.id ?? null;
        }
      }
    }
    return null;
  }, label);
  expect(workRootId, "resolved workRootId for the fixture workRoot").toBeTruthy();
  return workRootId as string;
}

// `data-workbench-pane-id` embeds a URL-encoded server-scoped identity
// (`terminal:server-local%2Fterm_xxx`), so match by substring on the raw
// terminal id.
function terminalTab(page: Page, terminalId: string) {
  return page.locator(
    `.dockview-workbench-tab[data-workbench-pane-id*="${terminalId}"]`,
  );
}

// Unconditional teardown. The daemon does NOT terminate live
// `TerminalSession`s on shutdown, and the hooked test profile deliberately
// sleeps 180s, so anything still running when `daemon.stop()` fires survives
// as an orphaned `terminal-helper`/`sh`/`sleep`. Direct DELETE rather than the
// UI close flow: after a failed assertion the DOM may be in any state, but the
// page's owner session cookie is still good. Errors are swallowed so a
// teardown hiccup can never mask the real failure.
async function forceCloseTerminals(page: Page, terminalIds: string[]) {
  for (const terminalId of terminalIds.filter(Boolean)) {
    try {
      await page.evaluate(async (endpoint) => {
        await fetch(endpoint, { method: "DELETE" });
      }, terminalCloseEndpoint(terminalId));
    } catch {
      // Page already closed/crashed - nothing more this side can do.
    }
  }
}

// Spawns one terminal into a NAMED work root through the daemon route
// directly, which is the only way to choose the spawn profile - the pinned
// agent carrier.
async function spawnTerminalInRoot(
  page: Page,
  workRootId: string,
  profileId: string | null,
  title: string,
): Promise<string> {
  const endpoint = workRootTerminalsEndpoint(workRootId);
  const created = await page.evaluate(
    async ({ endpoint, profileId, title }) => {
      const response = await fetch(endpoint, {
        method: "POST",
        headers: {
          Accept: "application/json",
          "Content-Type": "application/json",
        },
        body: JSON.stringify({
          columns: 80,
          rows: 24,
          title,
          cwdHint: null,
          profileId,
        }),
      });
      return {
        ok: response.ok,
        status: response.status,
        body: await response.json(),
      };
    },
    { endpoint, profileId, title },
  );
  expect(
    created.ok,
    `terminal create (${title}, profile ${profileId ?? "none"}): ${JSON.stringify(created)}`,
  ).toBe(true);
  expect(created.body.profileId ?? null).toBe(profileId);
  const terminalId = created.body.terminalId as string;
  expect(terminalId).toBeTruthy();
  return terminalId;
}

// Observes the real `Notification` global through a `Proxy` with a `construct`
// trap. Deliberately NOT a stub class: the trap records the arguments and then
// `Reflect.construct`s the REAL constructor, so a browser that refuses to
// construct still throws into `App.tsx`'s existing (load-bearing) catch, and
// the app is never observed talking to a test double.
//
// Must be called BEFORE the page's first `goto`.
async function installNotificationRecorder(page: Page) {
  await page.addInitScript(() => {
    // Per-document recorder. A `page.reload()` builds a fresh JS realm and
    // this array starts empty again, which is exactly why every counted
    // window in this file opens and closes inside ONE document.
    const constructions: Array<{ title: unknown; body: unknown }> = [];
    (window as unknown as Record<string, unknown>).__wsNotificationConstructions =
      constructions;
    const real = (window as unknown as { Notification?: unknown }).Notification;
    if (typeof real !== "function") {
      return;
    }
    const proxied = new Proxy(
      real as unknown as new (...args: unknown[]) => unknown,
      {
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
        // Proxy. Plain forwarding already agrees in practice; this only
        // removes the remaining failure mode.
        get(target, prop) {
          const value = Reflect.get(target, prop, target);
          return typeof value === "function" ? value.bind(target) : value;
        },
      },
    );
    Object.defineProperty(window, "Notification", {
      configurable: true,
      writable: true,
      value: proxied,
    });
  });
}

// `null` means the init script never ran for the CURRENT document. That is a
// hard failure, never "zero notifications" - every count assertion below
// asserts non-`null` before reading a length.
async function recordedNotifications(page: Page) {
  return await page.evaluate(
    () =>
      (
        window as unknown as {
          __wsNotificationConstructions?: Array<{
            title: unknown;
            body: unknown;
          }>;
        }
      ).__wsNotificationConstructions ?? null,
  );
}

async function inPagePermission(page: Page): Promise<string> {
  return await page.evaluate(() => Notification.permission as string);
}

// Asserts the precondition the whole file rests on, restated in EVERY document
// that expects a fire. Without it a broken grant would make every negative
// assertion here pass vacuously.
async function assertPermissionGranted(page: Page) {
  expect(
    await page.evaluate(() => typeof Notification),
    "the Notification global must exist in this document",
  ).toBe("function");
  expect(
    await inPagePermission(page),
    "channel:'chromium' must report a granted notification permission; under the default chromium-headless-shell this reads 'denied' and every negative assertion in this file would pass vacuously",
  ).toBe("granted");
}

// Opens Settings and switches to the Notifications section, returning the
// modal and its single checkbox. The caller clicks the checkbox, asserts, then
// closes with `closeSettings`.
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

async function closeSettings(page: Page) {
  const modal = page.locator(".settings-modal");
  await modal.locator('[data-command-id="settings.close"]').click();
  await expect(modal).toHaveCount(0);
}

async function persistedNotificationPrefs(page: Page): Promise<string | null> {
  return await page.evaluate(
    (key) => window.localStorage.getItem(key),
    NOTIFICATION_PREFS_KEY,
  );
}

// Playwright permission grants are per CONTEXT and every `test()` gets a fresh
// one, so a granted test and a non-granted test coexist in this file. Fresh
// context also means fresh `localStorage`: each test must re-enable the
// Notifications toggle for itself, the pref never leaks between tests.
// `daemon.baseUrl` is only known after `beforeAll` (the daemon binds port 0),
// which is why this cannot be a static file-level
// `test.use({ permissions: [...] })`.
async function grantNotifications(page: Page) {
  await page
    .context()
    .grantPermissions(["notifications"], { origin: daemon.baseUrl });
}

test("permission is granted and the Notifications toggle persists", async ({
  page,
}) => {
  await grantNotifications(page);
  await installNotificationRecorder(page);
  // This file's one pairing; it captures `ownerCookies` for the tests below.
  await pairOwner(page);

  await test.step("the granted permission is asserted, not assumed", async () => {
    await assertPermissionGranted(page);
  });

  await test.step("enabling the toggle keeps it checked and persists the opt-in", async () => {
    const { checkbox } = await openNotificationSettings(page);
    await expect(checkbox).not.toBeChecked();
    expect(await persistedNotificationPrefs(page)).not.toBe(PREFS_ENABLED);

    await checkbox.click();
    await expect(checkbox).toBeChecked();

    // With the permission pre-granted, `requestPermission()` resolves
    // "granted", so the denied-reconciliation `onChange(false)` must NOT
    // fire. Settle past the promise before re-asserting, otherwise a
    // reconciliation that DID fire would arrive after the assertion.
    await page.waitForTimeout(SETTLE_MS);
    await expect(
      checkbox,
      "a granted requestPermission() must leave the box checked",
    ).toBeChecked();
    expect(await persistedNotificationPrefs(page)).toBe(PREFS_ENABLED);

    await closeSettings(page);
  });
});

test("a ready edge fires exactly one notification; a second agent reaching ready fires none", async ({
  page,
}) => {
  await grantNotifications(page);
  await installNotificationRecorder(page);
  await attachOwnerSession(page);
  await openWorkRootMinimal(page, workRoot);
  const workRootId = await resolveWorkRootId(page, workRoot);

  let idA = "";
  let idB = "";

  try {
    let tokenA = "";
    let tokenB = "";

    await test.step("spawn two hooked agent terminals and restore them into the DOM", async () => {
      idA = await spawnTerminalInRoot(
        page,
        workRootId,
        "dummy-echo-hooked",
        "Notify Agent A",
      );
      idB = await spawnTerminalInRoot(
        page,
        workRootId,
        "dummy-echo-hooked",
        "Notify Agent B",
      );
      tokenA = readCallbackToken(idA);
      tokenB = readCallbackToken(idB);

      // THE one reload of this test. Required because both spawns were direct
      // fetches that bypassed React state. It happens here, before the
      // counted window opens and before any turn-state POST, so it can
      // neither erase a counted construction nor add one.
      await page.reload({ waitUntil: "domcontentloaded" });
      await selectWorkRootMinimal(page, workRoot);
      await expect(terminalTab(page, idA)).toHaveCount(1, { timeout: 20_000 });
      await expect(terminalTab(page, idB)).toHaveCount(1, { timeout: 20_000 });
    });

    await test.step("enable the Notifications pref in THIS document", async () => {
      const { checkbox } = await openNotificationSettings(page);
      await checkbox.click();
      await expect(checkbox).toBeChecked();
      await closeSettings(page);
    });

    // ---- the counted window opens here and does not close until step 11 ----
    await test.step("precondition: permission granted and nothing recorded yet", async () => {
      await assertPermissionGranted(page);
      const initial = await recordedNotifications(page);
      expect(
        initial,
        "the recorder init script must have run for this document",
      ).not.toBeNull();
      expect(initial!.length).toBe(0);
    });

    await test.step("agent A reaching ready constructs exactly one Notification with the shipped strings", async () => {
      await postTurnState(idA, tokenA, "ready");
      await expect(terminalTab(page, idA)).toHaveAttribute(
        "data-attention-state",
        "ready",
        { timeout: 20_000 },
      );

      await expect
        .poll(async () => (await recordedNotifications(page))?.length, {
          timeout: 10_000,
        })
        .toBe(1);
      const [first] = (await recordedNotifications(page))!;
      expect(first.title).toBe(NOTIFICATION_TITLE);
      expect(first.body).toBe(NOTIFICATION_BODY);
    });

    await test.step("agent B reaching ready while the aggregate is already ready constructs nothing more", async () => {
      await postTurnState(idB, tokenB, "ready");
      // Gating on B's own tab is what stops the assertion below from passing
      // merely because the second transition never landed: the second `ready`
      // is proven to have reached the browser before the count is re-read.
      await expect(terminalTab(page, idB)).toHaveAttribute(
        "data-attention-state",
        "ready",
        { timeout: 20_000 },
      );
      await page.waitForTimeout(SETTLE_MS);

      const after = await recordedNotifications(page);
      expect(
        after,
        "the recorder must still exist in this document",
      ).not.toBeNull();
      // Cannot pass on an empty list: the SAME array in the SAME document
      // already had to reach length 1 above for this step to be reached, and
      // no reload separates the two. `toBe(1)` fails on 0 as loudly as on 2.
      expect(
        after!.length,
        "a second agent reaching ready while the aggregate is already ready must not construct another Notification",
      ).toBe(1);
    });
  } finally {
    await forceCloseTerminals(page, [idA, idB]);
  }
});

test("ready -> working -> ready fires a second; turning the toggle off gates a third", async ({
  page,
}) => {
  await grantNotifications(page);
  await installNotificationRecorder(page);
  await attachOwnerSession(page);
  await openWorkRootMinimal(page, workRoot);
  const workRootId = await resolveWorkRootId(page, workRoot);

  let idA = "";

  try {
    let tokenA = "";

    await test.step("spawn one hooked agent terminal and restore it into the DOM", async () => {
      idA = await spawnTerminalInRoot(
        page,
        workRootId,
        "dummy-echo-hooked",
        "Notify Agent Repeat",
      );
      tokenA = readCallbackToken(idA);

      // THE one reload of this test, again before the counted window opens
      // and before any turn-state POST.
      await page.reload({ waitUntil: "domcontentloaded" });
      await selectWorkRootMinimal(page, workRoot);
      await expect(terminalTab(page, idA)).toHaveCount(1, { timeout: 20_000 });
    });

    await test.step("enable the Notifications pref in THIS document", async () => {
      const { checkbox } = await openNotificationSettings(page);
      await checkbox.click();
      await expect(checkbox).toBeChecked();
      await closeSettings(page);
    });

    // ---- the counted window opens here and does not close until step 14 ----
    await test.step("precondition: permission granted and nothing recorded yet", async () => {
      await assertPermissionGranted(page);
      const initial = await recordedNotifications(page);
      expect(
        initial,
        "the recorder init script must have run for this document",
      ).not.toBeNull();
      expect(initial!.length).toBe(0);
    });

    await test.step("the first ready edge constructs one Notification", async () => {
      await postTurnState(idA, tokenA, "ready");
      await expect(terminalTab(page, idA)).toHaveAttribute(
        "data-attention-state",
        "ready",
        { timeout: 20_000 },
      );
      await expect
        .poll(async () => (await recordedNotifications(page))?.length, {
          timeout: 10_000,
        })
        .toBe(1);
    });

    await test.step("a SECOND ready edge after dropping back to working fires again", async () => {
      // Gating on `working` explicitly is required, not decorative: without
      // it the `ready` wait below would pass immediately against the stale
      // `ready` above and the edge would never be proven to have happened.
      await postTurnState(idA, tokenA, "working");
      await expect(terminalTab(page, idA)).toHaveAttribute(
        "data-attention-state",
        "working",
        { timeout: 20_000 },
      );

      await postTurnState(idA, tokenA, "ready");
      await expect(terminalTab(page, idA)).toHaveAttribute(
        "data-attention-state",
        "ready",
        { timeout: 20_000 },
      );

      // The edge detector is not a one-shot latch.
      await expect
        .poll(async () => (await recordedNotifications(page))?.length, {
          timeout: 10_000,
        })
        .toBe(2);
    });

    await test.step("turning the toggle off persists the opt-out - no reload, the counted window stays open", async () => {
      const { checkbox } = await openNotificationSettings(page);
      await expect(checkbox).toBeChecked();
      await checkbox.click();
      await expect(checkbox).not.toBeChecked();
      expect(await persistedNotificationPrefs(page)).toBe(PREFS_DISABLED);
      await closeSettings(page);
    });

    await test.step("with the pref off, a genuine ready edge constructs nothing more", async () => {
      await postTurnState(idA, tokenA, "working");
      await expect(terminalTab(page, idA)).toHaveAttribute(
        "data-attention-state",
        "working",
        { timeout: 20_000 },
      );
      await postTurnState(idA, tokenA, "ready");
      await expect(terminalTab(page, idA)).toHaveAttribute(
        "data-attention-state",
        "ready",
        { timeout: 20_000 },
      );
      await page.waitForTimeout(SETTLE_MS);

      const after = await recordedNotifications(page);
      expect(after).not.toBeNull();
      // Cannot pass on an empty list: the SAME array in the SAME document
      // already reached length 2 above. The permission is still "granted" in
      // this document (asserted above, never revoked), which is what isolates
      // the PREF as the gate rather than the permission.
      expect(
        after!.length,
        "with the Notifications pref off (permission still granted) a ready edge must not construct a Notification",
      ).toBe(2);
      expect(
        await inPagePermission(page),
        "the permission must still be granted, so this step isolates the pref",
      ).toBe("granted");
    });
  } finally {
    await forceCloseTerminals(page, [idA]);
  }
});

test("without a grant the toggle reconciles to unchecked and a ready edge fires nothing", async ({
  page,
}) => {
  // Deliberately NO grantPermissions call - this is the inverse gate.
  await installNotificationRecorder(page);
  await attachOwnerSession(page);
  await openWorkRootMinimal(page, workRoot);
  const workRootId = await resolveWorkRootId(page, workRoot);

  let id = "";

  try {
    let token = "";

    await test.step("spawn one hooked agent terminal and restore it into the DOM", async () => {
      id = await spawnTerminalInRoot(
        page,
        workRootId,
        "dummy-echo-hooked",
        "Notify Agent Denied",
      );
      token = readCallbackToken(id);

      await page.reload({ waitUntil: "domcontentloaded" });
      await selectWorkRootMinimal(page, workRoot);
      await expect(terminalTab(page, id)).toHaveCount(1, { timeout: 20_000 });
    });

    await test.step("inverse precondition: the permission is NOT granted in this context", async () => {
      expect(
        await page.evaluate(() => typeof Notification),
        "the Notification global must exist even without a grant (this is a secure context)",
      ).toBe("function");
      // Asserted as "not granted" rather than "denied" so the gate does not
      // become brittle if the measured value is "default".
      expect(await inPagePermission(page)).not.toBe("granted");
    });

    await test.step("checking the toggle reconciles back to unchecked and persists the opt-out", async () => {
      const { checkbox } = await openNotificationSettings(page);
      await expect(checkbox).not.toBeChecked();
      await checkbox.click();

      // The uncheck arrives asynchronously, after `requestPermission()`
      // settles to "denied". Settle FIRST and only then assert: a bare
      // `not.toBeChecked()` here would race the initial checked render and
      // could pass before the box was ever checked at all.
      await page.waitForTimeout(SETTLE_MS);
      await expect(
        checkbox,
        "a denied requestPermission() must reconcile the box back to unchecked",
      ).not.toBeChecked({ timeout: 10_000 });
      // ORDERING DEPENDENCY - do NOT drop the next line as redundant. The
      // unchecked assertion above is, on its own, satisfiable by a click that
      // did nothing at all: the box starts unchecked and that is asserted a
      // few lines up. What closes that hole is the persisted-value assertion
      // below - a fresh context has no `ws-dashboard.settings.notifications.v1`
      // key, so `getItem` returns null, and only `onChange(true)` FOLLOWED BY
      // the reconciling `onChange(false)` can leave it at PREFS_DISABLED.
      expect(await persistedNotificationPrefs(page)).toBe(PREFS_DISABLED);

      await closeSettings(page);
    });

    await test.step("a ready edge with no permission constructs nothing", async () => {
      await postTurnState(id, token, "ready");
      await expect(terminalTab(page, id)).toHaveAttribute(
        "data-attention-state",
        "ready",
        { timeout: 20_000 },
      );
      await page.waitForTimeout(SETTLE_MS);

      const after = await recordedNotifications(page);
      // Stated honestly: this is the one assertion in the file that cannot be
      // made non-vacuous by construction - the whole point is that nothing
      // fires. Its value comes from the non-`null` recorder check plus the
      // fact that the identical recorder demonstrably records in the two
      // tests above. The mutation-provable halves of this test are the
      // reconciliation assertions in the previous step.
      expect(
        after,
        "the recorder must exist in this document, otherwise a length of 0 proves nothing",
      ).not.toBeNull();
      expect(after!.length).toBe(0);
    });
  } finally {
    await forceCloseTerminals(page, [id]);
  }
});
