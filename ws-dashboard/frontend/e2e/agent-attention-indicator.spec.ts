import { test, expect, type Page } from "@playwright/test";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { startDaemon, type DaemonHandle } from "./daemonHarness.js";
import {
  terminalCloseEndpoint,
  workRootTerminalsEndpoint,
} from "../src/terminals.js";

// Browser-level acceptance gate for the 260725 Phase 6 tab-label attention
// indicator (`ai-docs/.plans/2026-07/26-0748-pty-agent-tab-indicator.md`).
//
// CONTRACT: a small SIBLING spec with its own daemon/workRoot, following the
// `agent-spawn-profile.spec.ts` precedent (see that file's CONTRACT for why a
// new `test.step` inside `dashboard-acceptance.spec.ts` is the wrong shape -
// that suite's steps share one exact terminal-tab count that many later
// assertions depend on).
//
// What this gate proves, at browser level (the `ws-web-dashboard` mental
// model's binding rule - `tsc`/build/curl do not close UI-facing work):
//   1. A turn-state callback POST (the Phase 4 route, driven directly with a
//      real per-terminal token) makes the indicator APPEAR on that
//      terminal's Dockview tab.
//   2. LOAD-BEARING (plan step 7, the `shouldUpdateDockviewWorkbenchPanelParams`
//      fix): a SECOND transition on the SAME, still-mounted, still-ACTIVE,
//      still-connected tab repaints it (`working` -> `ready`). This is the
//      assertion the param-diff fix exists for: that function's
//      `persistentTerminal` branch returns `false` for a connected terminal,
//      so before the fix a changed attention param never reached Dockview's
//      `updateParameters` at all. A first-transition-only assertion would
//      NOT prove this - the first paint can arrive via an unrelated
//      remount/param change, whereas by the second transition the panel is
//      demonstrably mounted and quiescent.
//   3. Selecting a DIFFERENT tab does not clear this tab's indicator (the ack
//      watermark is per terminal, not global).
//   4. Selecting the terminal's own tab acknowledges and clears it.
//   5. A LATER turn boundary re-raises it (the watermark is keyed by
//      `updatedAtMs`, so acknowledging once does not mute the terminal).
//   6. LOAD-BEARING (review cycle 1, Critical): clicking the tab that is
//      ALREADY Dockview's active panel acknowledges it, with no intervening
//      pane change. Assertions 3-4 above cannot cover this - they select the
//      other terminal first, so the active panel genuinely changes and
//      `onDidActivePanelChange` fires. The feature's PRIMARY flow is the
//      opposite: the agent finishes in the tab the user left focused.
//
// No vendor CLI, credentials, or network are involved: the terminal is
// spawned with the always-compiled-in `"dummy-echo-hooked"` test profile
// (`agent_profile_registry.rs`), whose only relevant property is that it
// carries a hook config, which is what makes `TerminalSession::spawn` mint a
// real callback token. The hook itself can never fire - the dummy command is
// `/bin/sh`, not an agent.
//
// TOKEN HANDLING (ticket hard constraint: the callback token must never
// appear in a URL or a log line): the token is read directly off disk with
// Node `fs` from this run's own state dir, and POSTed in a JSON BODY from
// the Node side of the test - never through `page.evaluate`, never in a
// query string, never printed. Assertion messages below deliberately never
// interpolate it.

function socketSafeTempBase(): string {
  // Mirrors dashboard-acceptance.spec.ts / agent-spawn-profile.spec.ts (same
  // sockaddr_un.sun_path length ceiling on macOS).
  return process.platform === "darwin" ? "/tmp" : os.tmpdir();
}

function workRootDisplayName(rootPath: string) {
  const normalized = rootPath.replace(/[\\/]+$/, "");
  const match = normalized.match(/[^\\/]+$/);
  return match ? match[0] : normalized;
}

// 260725 Phase 7 (nav-row agent counter) adds a SECOND `test(...)` to this
// file rather than a third spec: every helper below (temp state home +
// on-disk token read, the direct turn-state POST, the minimal open/select
// helpers, the tab locators, the unconditional force-close teardown) is
// module-local and unexported, so a sibling file would duplicate them and
// boot a third daemon. What that second test proves, at browser level:
//   1. The counter SPLIT - one `working` agent and one `ready` agent are
//      reported as separate numbers on the same row at the same time.
//   2. NO DOUBLE COUNT - the row reads 2 agents and exactly 1 terminal while
//      three terminal tabs are mounted, so the `1` is an exclusion rather
//      than a stale read.
//   3. A badge on a work root that is NOT the selected one.
//   4. Acknowledging the LAST pending tab clears the row badge, with no
//      nav-row action of any kind - the derived-never-separately-acknowledged
//      rule.
//   5. The pinned server-row roll-up (`ready` outranks `working`).

let daemon: DaemonHandle;
let workRoot: string;
// Second fixture root (Phase 7). Root A stays the tab-indicator gate's root;
// every Phase 7 terminal is spawned under root B so "a badge on a work root
// that is not selected" is expressible by selecting root A.
let workRootB: string;
let stateHome: string;
let previousStateHome: string | undefined;
// The daemon's owner pairing URL is ONE-TIME, so the second test cannot pair
// again: it reuses the cookie the first test's pairing installed, exactly as
// `dashboard-acceptance.spec.ts` does for its own second test (:4137). This
// is why the file stays in serial mode.
let ownerCookies:
  | Awaited<ReturnType<import("@playwright/test").BrowserContext["cookies"]>>
  | undefined;

test.describe.configure({ mode: "serial" });

test.beforeAll(async () => {
  workRoot = mkdtempSync(path.join(os.tmpdir(), "ws-dash-attention-"));
  writeFileSync(
    path.join(workRoot, "readme.txt"),
    "agent attention indicator browser gate fixture\n",
  );
  workRootB = mkdtempSync(path.join(os.tmpdir(), "ws-dash-navcount-"));
  writeFileSync(
    path.join(workRootB, "readme.txt"),
    "nav row agent counter browser gate fixture\n",
  );
  previousStateHome = process.env.WS_DASHBOARD_STATE_HOME;
  // DEVIATION from the plan's Codebase Findings (recorded deliberately): the
  // plan concluded the harness plumbs no state-dir override and that this
  // spec would therefore have to reconstruct the real per-user default state
  // path to find `terminal-tokens/<id>.json`. It searched for
  // `--state-dir`/`WS_DASHBOARD_STATE_DIR`; the override that DOES exist is
  // `WS_DASHBOARD_STATE_HOME` (`persistent_state.rs::default_state_file`),
  // inherited by the spawned daemon through `process.env` in
  // `daemonHarness.ts`, and already used exactly this way by
  // `agent-spawn-profile.spec.ts`. Using it keeps the token read
  // deterministic AND keeps this gate from writing into the developer's real
  // `~/.local/state/ws-dashboard/` (a pollution problem Phase 3's Result
  // already hit once).
  stateHome = mkdtempSync(
    path.join(socketSafeTempBase(), "ws-dash-attention-state-"),
  );
  process.env.WS_DASHBOARD_STATE_HOME = stateHome;
  daemon = await startDaemon();
});

test.afterAll(async () => {
  if (daemon) {
    await daemon.stop();
  }
  rmSync(workRoot, { recursive: true, force: true });
  rmSync(workRootB, { recursive: true, force: true });
  rmSync(stateHome, { recursive: true, force: true });
  if (previousStateHome === undefined) {
    delete process.env.WS_DASHBOARD_STATE_HOME;
  } else {
    process.env.WS_DASHBOARD_STATE_HOME = previousStateHome;
  }
});

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

function terminalTabsLocator(page: Page) {
  return page.locator(
    '.dockview-workbench-tab[data-workbench-pane-id^="terminal:"]',
  );
}

// `data-workbench-pane-id` embeds a URL-encoded server-scoped identity
// (`terminal:server-local%2Fterm_xxx`), so match by substring on the raw
// terminal id (same approach as agent-spawn-profile.spec.ts).
function terminalTab(page: Page, terminalId: string) {
  return page.locator(
    `.dockview-workbench-tab[data-workbench-pane-id*="${terminalId}"]`,
  );
}

async function currentTerminalPaneId(page: Page): Promise<string | null> {
  const ids = await page
    .locator(".terminal-pane")
    .evaluateAll((panes) =>
      panes.map((pane) => pane.getAttribute("data-terminal-id")),
    );
  return ids[0] ?? null;
}

// Reads the daemon-written per-terminal callback token straight off disk.
// Never over HTTP (there is no route that serves it, by design), never
// logged, never echoed into an assertion message. Path construction mirrors
// `agent_token_store.rs::token_store_path`.
function readCallbackToken(terminalId: string): string {
  const tokenPath = path.join(
    stateHome,
    "terminal-tokens",
    `${terminalId}.json`,
  );
  const parsed = JSON.parse(readFileSync(tokenPath, "utf8")) as {
    token?: string;
  };
  expect(
    typeof parsed.token === "string" && parsed.token.length > 0,
    "the hooked test profile must have made spawn mint a callback token",
  ).toBe(true);
  return parsed.token as string;
}

// Drives the Phase 4 callback route the way a vendor hook would: from
// OUTSIDE the browser, with no owner session cookie (that route is
// registered outside `require_owner_auth` and is authorized by the
// per-terminal token alone).
async function postTurnState(
  terminalId: string,
  token: string,
  state: "working" | "ready" | "idle",
) {
  const response = await fetch(
    new URL(
      `/api/dashboard/terminals/${terminalId}/turn-state`,
      daemon.baseUrl,
    ),
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ token, state }),
    },
  );
  expect(
    response.status,
    `turn-state POST for state '${state}' must be accepted`,
  ).toBe(204);
}

async function closeTerminalById(page: Page, terminalId: string) {
  if (!terminalId) {
    return;
  }
  const tab = terminalTab(page, terminalId);
  if ((await tab.count()) === 0) {
    return;
  }
  await tab.hover();
  await tab.locator('[data-command-id="workbench.tab.close"]').click();
  const popover = page.locator('[data-workbench-close-popover="cursor-near"]');
  await expect(popover).toBeVisible();
  await popover
    .locator('[data-command-id="workbench.tab.close.confirm"]')
    .click();
  await expect(popover).toHaveCount(0);
}

// Unconditional teardown for the terminals this gate spawns (review cycle 1,
// Test Minor 2). The daemon does NOT terminate live `TerminalSession`s on
// shutdown (`run_with_shutdown_and_grace`), so anything still running when
// `daemon.stop()` fires survives as an orphaned `terminal-helper`/`sh`/
// `sleep` for the rest of its natural life - and this profile deliberately
// sleeps 180s. The happy-path close step below is the DOM-level evidence
// that the close flow works; this is the belt-and-braces version that also
// runs when an assertion above threw. Direct DELETE rather than the UI close
// flow: after a failed assertion the DOM may be in any state, but the page's
// owner session cookie is still good. Errors are swallowed so a teardown
// hiccup can never mask the real failure.
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
// directly (not the toolbar), which is the only way to put terminals in a
// root that is not the one the browser currently has selected - and the only
// way to choose the spawn profile, which is the pinned agent carrier.
// `profileId: null` produces an ordinary shell terminal.
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

// The nav row for one work root. Both fixture roots are single-workRoot
// workspaces, so each renders as one `compactWorkRoot` row - the
// presentation that carries the reserved second line.
function workRootRow(page: Page, rootPath: string) {
  return page
    .locator('.resource-row[data-resource-presentation="compactWorkRoot"]', {
      hasText: workRootDisplayName(rootPath),
    })
    .first();
}

test("agent attention indicator", async ({ page }) => {
  await pairOwner(page);
  await openWorkRootMinimal(page, workRoot);
  const workRootId = await resolveWorkRootId(page, workRoot);

  let agentTerminalId = "";
  let plainTerminalId = "";
  let token = "";

  try {
    await test.step("spawn a hooked test-profile terminal and make its tab the active one", async () => {
      const endpoint = workRootTerminalsEndpoint(workRootId);
      const created = await page.evaluate(
        async ({ endpoint }) => {
          const response = await fetch(endpoint, {
            method: "POST",
            headers: {
              Accept: "application/json",
              "Content-Type": "application/json",
            },
            body: JSON.stringify({
              columns: 80,
              rows: 24,
              title: "Attention Probe",
              cwdHint: null,
              profileId: "dummy-echo-hooked",
            }),
          });
          return {
            ok: response.ok,
            status: response.status,
            body: await response.json(),
          };
        },
        { endpoint },
      );
      expect(
        created.ok,
        `terminal create with dummy-echo-hooked profile: ${JSON.stringify(created)}`,
      ).toBe(true);
      expect(created.body.profileId).toBe("dummy-echo-hooked");
      agentTerminalId = created.body.terminalId as string;
      expect(agentTerminalId).toBeTruthy();

      token = readCallbackToken(agentTerminalId);

      // The direct fetch above bypassed this tab's React state; reload +
      // reselect so the daemon-known session renders as a real Dockview pane
      // (same restoration path agent-spawn-profile.spec.ts uses).
      await page.reload({ waitUntil: "domcontentloaded" });
      await selectWorkRootMinimal(page, workRoot);
      await expect(terminalTab(page, agentTerminalId)).toHaveCount(1, {
        timeout: 20_000,
      });

      // A second, ordinary terminal exists solely so "select a DIFFERENT tab"
      // is expressible below. Created via the toolbar, which auto-focuses it.
      await page.locator('[data-command-id="terminal.create"]').click();
      await expect(terminalTabsLocator(page)).toHaveCount(2, { timeout: 20_000 });
      await expect
        .poll(() => currentTerminalPaneId(page), { timeout: 20_000 })
        .not.toBe(agentTerminalId);
      plainTerminalId = (await currentTerminalPaneId(page))!;
      expect(plainTerminalId).toBeTruthy();

      // Make the agent terminal the ACTIVE tab and wait until its pane body is
      // actually mounted and its socket connected. This is a precondition of
      // the load-bearing assertion below, not incidental setup: the
      // `shouldUpdateDockviewWorkbenchPanelParams` early return this phase
      // fixes only suppresses updates while `meta[1]` is
      // `"connecting"`/`"connected"`, so a backgrounded (disconnected) tab
      // would repaint anyway and prove nothing.
      await terminalTab(page, agentTerminalId).click();
      await expect(
        page.locator(`.terminal-pane[data-terminal-id="${agentTerminalId}"]`),
      ).toHaveCount(1, { timeout: 20_000 });
      await expect(terminalTab(page, agentTerminalId)).toHaveAttribute(
        "data-attention-state",
        "none",
      );
    });

    await test.step("a turn-state callback raises the indicator on that tab only", async () => {
      await postTurnState(agentTerminalId, token, "working");
      await expect(terminalTab(page, agentTerminalId)).toHaveAttribute(
        "data-attention-state",
        "working",
        { timeout: 20_000 },
      );
      await expect(
        terminalTab(page, agentTerminalId).locator(
          "[data-workbench-tab-attention]",
        ),
      ).toHaveAttribute("data-workbench-tab-attention", "working");
      await expect(terminalTab(page, plainTerminalId)).toHaveAttribute(
        "data-attention-state",
        "none",
      );
    });

    await test.step("a SECOND transition repaints the same mounted, active, connected tab (param-diff fix)", async () => {
      // THE load-bearing assertion of this spec (plan step 7). Nothing between
      // the previous step and this one remounts or reselects the panel: the
      // only path from `working` to `ready` on screen is
      // `shouldUpdateDockviewWorkbenchPanelParams` returning true for a
      // CONNECTED persistentTerminal whose `attentionState` changed. Revert
      // that comparison and this assertion is the one that fails.
      await postTurnState(agentTerminalId, token, "ready");
      await expect(terminalTab(page, agentTerminalId)).toHaveAttribute(
        "data-attention-state",
        "ready",
        { timeout: 20_000 },
      );
    });

    await test.step("selecting a different tab does not acknowledge this one", async () => {
      await terminalTab(page, plainTerminalId).click();
      await expect(
        page.locator(`.terminal-pane[data-terminal-id="${plainTerminalId}"]`),
      ).toHaveCount(1, { timeout: 20_000 });
      await expect(terminalTab(page, agentTerminalId)).toHaveAttribute(
        "data-attention-state",
        "ready",
      );
    });

    await test.step("selecting the terminal's own tab acknowledges and clears the indicator", async () => {
      await terminalTab(page, agentTerminalId).click();
      await expect(terminalTab(page, agentTerminalId)).toHaveAttribute(
        "data-attention-state",
        "none",
        { timeout: 20_000 },
      );
      await expect(
        terminalTab(page, agentTerminalId).locator(
          "[data-workbench-tab-attention]",
        ),
      ).toHaveCount(0);
    });

    await test.step("a later turn boundary re-raises the acknowledged indicator", async () => {
      // The ack watermark is keyed by `updatedAtMs`, not by terminal id: a
      // single acknowledgement must not mute this terminal for the rest of the
      // session. This is also a THIRD repaint of the same connected tab.
      await postTurnState(agentTerminalId, token, "working");
      await expect(terminalTab(page, agentTerminalId)).toHaveAttribute(
        "data-attention-state",
        "working",
        { timeout: 20_000 },
      );
    });

    await test.step("clicking the ALREADY-ACTIVE tab acknowledges it (no intervening pane change)", async () => {
      // THE review-cycle-1 Critical regression guard. Every earlier clear in
      // this spec first selected the OTHER terminal, so Dockview's active
      // panel genuinely changed and `onDidActivePanelChange` -> `selectPane`
      // fired. That masked the feature's PRIMARY flow: the agent finishes in
      // the tab the user left focused, so the badge is raised on the tab that
      // is ALREADY active, and dockview-core emits no active-panel change when
      // that tab is clicked (`doSetGroupActive` compares against the current
      // value; `DockviewGroupPanelModel.openPanel` early-returns for the
      // already-active panel). With `selectPane` as the only ack trigger the
      // badge was unclearable here - permanently so with a single open pane.
      //
      // Preconditions asserted, not assumed: the agent tab is already the
      // selected tab AND already shows the indicator raised by the previous
      // step. Nothing between that step and the click below touches any other
      // pane.
      await expect(terminalTab(page, agentTerminalId)).toHaveAttribute(
        "aria-selected",
        "true",
      );
      await expect(terminalTab(page, agentTerminalId)).toHaveAttribute(
        "data-attention-state",
        "working",
      );

      await terminalTab(page, agentTerminalId).click();

      await expect(terminalTab(page, agentTerminalId)).toHaveAttribute(
        "data-attention-state",
        "none",
        { timeout: 20_000 },
      );
      // The tab never stopped being the active one - this clear came from the
      // tab's own click handler, not from a selection change.
      await expect(terminalTab(page, agentTerminalId)).toHaveAttribute(
        "aria-selected",
        "true",
      );
    });

    await test.step("cleanup: close every terminal this gate spawned", async () => {
      await closeTerminalById(page, agentTerminalId);
      await closeTerminalById(page, plainTerminalId);
      await expect(terminalTabsLocator(page)).toHaveCount(0);
    });
  } finally {
    await forceCloseTerminals(page, [agentTerminalId, plainTerminalId]);
  }
});

test("nav row agent counter", async ({ page }) => {
  await attachOwnerSession(page);
  await openWorkRootMinimal(page, workRoot);
  await openWorkRootMinimal(page, workRootB);
  const workRootBId = await resolveWorkRootId(page, workRootB);

  // Literal expected strings, not `formatOpenSurfaceCounts(...)` calls: this
  // gate must fail if the formatter's own contract drifts, and comparing a
  // row against the very function that rendered it could not.
  const bothPending = "1 terminal, 0 documents · 2 agents: 1 working, 1 ready";
  const bothReady = "1 terminal, 0 documents · 2 agents: 0 working, 2 ready";
  const oneAcknowledged =
    "1 terminal, 0 documents · 2 agents: 0 working, 1 ready";
  const allAcknowledged =
    "1 terminal, 0 documents · 2 agents: 0 working, 0 ready";

  let plainTerminalId = "";
  let agentOneId = "";
  let agentTwoId = "";

  try {
    let agentOneToken = "";
    let agentTwoToken = "";

    await test.step("spawn one shell terminal and two agent terminals under work root B", async () => {
      plainTerminalId = await spawnTerminalInRoot(
        page,
        workRootBId,
        null,
        "Nav Counter Shell",
      );
      agentOneId = await spawnTerminalInRoot(
        page,
        workRootBId,
        "dummy-echo-hooked",
        "Nav Counter Agent 1",
      );
      agentTwoId = await spawnTerminalInRoot(
        page,
        workRootBId,
        "dummy-echo-hooked",
        "Nav Counter Agent 2",
      );
      agentOneToken = readCallbackToken(agentOneId);
      agentTwoToken = readCallbackToken(agentTwoId);

      // Same restoration path the first test uses: the direct POSTs above
      // bypassed this tab's React state.
      await page.reload({ waitUntil: "domcontentloaded" });
      await selectWorkRootMinimal(page, workRootB);
      await expect(terminalTabsLocator(page)).toHaveCount(3, {
        timeout: 20_000,
      });

      // Park the ACTIVE tab on the shell terminal, so nothing that follows
      // can acknowledge an agent tab as a side effect of selection.
      await terminalTab(page, plainTerminalId).click();
      await expect(
        page.locator(`.terminal-pane[data-terminal-id="${plainTerminalId}"]`),
      ).toHaveCount(1, { timeout: 20_000 });
      await expect(workRootRow(page, workRootB)).toHaveAttribute(
        "data-row-attention",
        "none",
      );
    });

    await test.step("counter split: one working and one ready are reported as separate numbers", async () => {
      // Holding BOTH states live at once is what makes this non-vacuous - a
      // single-state fixture would pass against a collapsed single total.
      await postTurnState(agentOneId, agentOneToken, "working");
      await postTurnState(agentTwoId, agentTwoToken, "ready");
      // Deliberately the AGENT SEGMENT only, not the whole line: this step
      // owns the split, and the surrounding terminal count is the next step's
      // subject. Keeping the two assertions disjoint is what makes each
      // mutation fail at its own site instead of both tripping the first one.
      await expect(
        workRootRow(page, workRootB).locator(".resource-row-counts"),
      ).toContainText("2 agents: 1 working, 1 ready", { timeout: 20_000 });
    });

    await test.step("no double count: 2 agents and exactly 1 terminal while three tabs are mounted", async () => {
      // The mounted-tab count is asserted in the SAME step as the row text,
      // so the `1 terminal` is proven to be an exclusion of the two agent
      // panes rather than a stale read taken before they existed.
      await expect(terminalTabsLocator(page)).toHaveCount(3);
      await expect(
        workRootRow(page, workRootB).locator(".resource-row-counts"),
      ).toHaveText(bothPending);
    });

    await test.step("server-row aggregation: ready outranks working across the server's roots", async () => {
      await expect(page.locator(".server-row").first()).toHaveAttribute(
        "data-row-attention",
        "ready",
      );
    });

    await test.step("a badge survives on a work root that is NOT the selected one", async () => {
      await selectWorkRootMinimal(page, workRoot);
      const rowA = workRootRow(page, workRoot);
      const rowB = workRootRow(page, workRootB);
      await expect(rowA).toHaveClass(/resource-row-selected/);
      await expect(rowB).not.toHaveClass(/resource-row-selected/);
      // Root B's workbench instance is still mounted but no longer the
      // active one (App.tsx renders inactive roots with display:none), which
      // is exactly the state a selected-root-only aggregation would drop.
      await expect(
        page
          .locator('[data-workbench-root-active="true"]')
          .locator(
            `.dockview-workbench-tab[data-workbench-pane-id*="${agentOneId}"]`,
          ),
      ).toHaveCount(0);
      await expect(rowB).toHaveAttribute("data-row-attention", "ready");
      await expect(rowB.locator(".resource-row-counts")).toHaveText(
        bothPending,
      );
    });

    await test.step("acknowledging the LAST pending tab clears the row badge, with no nav-row action", async () => {
      await selectWorkRootMinimal(page, workRootB);
      // Park the active tab on the shell terminal again, so the two tab
      // clicks below are the only acknowledgements in this step.
      await terminalTab(page, plainTerminalId).click();
      await expect(
        page.locator(`.terminal-pane[data-terminal-id="${plainTerminalId}"]`),
      ).toHaveCount(1, { timeout: 20_000 });

      await postTurnState(agentOneId, agentOneToken, "ready");
      const rowB = workRootRow(page, workRootB);
      await expect(rowB.locator(".resource-row-counts")).toHaveText(bothReady, {
        timeout: 20_000,
      });
      await expect(rowB).toHaveAttribute("data-row-attention", "ready");

      // Two agents, acknowledged one at a time: this proves the badge clears
      // when NO child is still pending, not merely on any acknowledgement.
      await terminalTab(page, agentOneId).click();
      await expect(rowB.locator(".resource-row-counts")).toHaveText(
        oneAcknowledged,
        { timeout: 20_000 },
      );
      await expect(rowB).toHaveAttribute("data-row-attention", "ready");

      await terminalTab(page, agentTwoId).click();
      await expect(rowB.locator(".resource-row-counts")).toHaveText(
        allAcknowledged,
        { timeout: 20_000 },
      );
      await expect(rowB).toHaveAttribute("data-row-attention", "none");
      // The agent panes are still mounted - the badge cleared because every
      // child terminal is acknowledged, not because the panes went away.
      await expect(terminalTabsLocator(page)).toHaveCount(3);
      await expect(page.locator(".server-row").first()).toHaveAttribute(
        "data-row-attention",
        "none",
      );
    });

    await test.step("cleanup: close every terminal this gate spawned", async () => {
      await closeTerminalById(page, agentOneId);
      await closeTerminalById(page, agentTwoId);
      await closeTerminalById(page, plainTerminalId);
      await expect(terminalTabsLocator(page)).toHaveCount(0);
      await expect(
        workRootRow(page, workRootB).locator(".resource-row-counts"),
      ).toHaveText("no open surfaces");
    });
  } finally {
    await forceCloseTerminals(page, [
      plainTerminalId,
      agentOneId,
      agentTwoId,
    ]);
  }
});
