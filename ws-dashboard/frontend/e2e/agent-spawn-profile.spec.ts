import { test, expect, type Page } from "@playwright/test";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { startDaemon, type DaemonHandle } from "./daemonHarness.js";
import { terminalCloseEndpoint, workRootTerminalsEndpoint } from "../src/terminals.js";

// Browser-level acceptance gate for the 260725 Phase 2 browser-facing agent
// spawn profile path (`ai-docs/.plans/2026-07/26-0130-260725-pty-agent-browser-spawn-profile.md`).
//
// CONTRACT: this is a small SIBLING spec file (plan Verification Plan /
// design answer 3 explicitly sanctions this isolation choice), not a new
// `test.step` inside `dashboard-acceptance.spec.ts`. That suite's steps are
// tightly coupled through a single shared `terminalTabs(page)`/
// `terminalPaneIds(page)` count that many later steps assert exactly
// (`toHaveCount(1)`, `toHaveCount(2)`, ...); inserting extra terminal spawns
// mid-suite would either have to surgically clean up after itself inside a
// 4000-line test or risk destabilizing unrelated assertions. A dedicated
// daemon/workRoot keeps this gate's terminal count entirely self-contained
// while still reusing `daemonHarness.ts` per the plan.
//
// Proves, at browser level (not curl-only, per the `ws-web-dashboard`
// mental model's binding rule):
//   1. A profile-resolved terminal (dummy, no vendor dependency) spawned via
//      a direct authenticated fetch renders as a pane with its profile
//      recorded in the DOM.
//   2. The existing no-profile `terminal.create` toolbar flow is unchanged
//      (profileId absent/null) - the "byte for byte" regression half.
//   3. THE GAP THE PLAN'S OWN BROWSER STEP NEVER EXERCISED: clicking the
//      real "New agent terminal" toolbar button dispatches
//      `terminal.create.agent` (not `agentChat.create`), and the resulting
//      pane is an ordinary `persistentTerminal` recorded with
//      `profileId: "claude"` - never a `data-surface-kind="agentChat"`
//      pane, i.e. it never routes through `registerNewAgentChatPane` (one
//      of the three `AGENT_GUI_SUSPENDED` guard depths). The observable
//      used is the toolbar's own `data-last-command-id` attribute (the
//      dashboard's existing command-observer evidence,
//      `App.tsx`'s `commandLog`/`WorkbenchToolbar`) plus DOM absence of any
//      `agentChat` surface - not a real vendor spawn: the acceptance suite
//      never asserts the underlying `claude` binary starts successfully,
//      only that the browser-visible dispatch and pane provenance are
//      correct, so this suite has no dependency on a vendor binary,
//      credentials, or network (per the ticket's hard constraint).
//   4. (review cycle 1, finding C2) A daemon-side session loss (simulated
//      via a direct DELETE that bypasses the browser's own close-tab flow,
//      leaving the browser's persisted `TerminalRestoreIntent` stale)
//      followed by a reload respawns the terminal through the SAME
//      resolved profile (`profileId: "claude"`), not a silent downgrade to
//      the default shell under an unchanged title - see the
//      "restore-intent respawn preserves profileId..." step below. Reuses
//      this test's own single authenticated `page` rather than a second
//      `test()` block: the daemon's owner-pairing URL is single-use, so a
//      fresh Playwright test (a fresh browser context with no session
//      cookie) cannot re-authenticate against it.
//
// DEVIATION (review cycle 1, finding T1): the plan's Verification Plan step
// 4 asked for this step to skip gracefully when `claude` is not installed.
// No such skip exists here, and this is deliberate, not an oversight: the
// daemon's `create_terminal` HTTP response is already built and returned
// BEFORE the helper process's `spawn_shell` ever attempts to run the
// resolved command (`TerminalSession::spawn` calls
// `connect_and_handshake`, which reads the helper's pre-spawn
// `Handshake`/`Status` messages and sends `HandshakeAck`, and only THEN
// does the helper's `handle_connection` invoke `spawn_shell` - see
// `terminal.rs::connect_and_handshake` and
// `terminal_helper_process.rs::spawn_shell`/`handle_connection`). A failed
// `spawn_command` only flips the helper's internal status to `Error`; it
// cannot retroactively fail the already-returned HTTP response, so this
// step's assertions (pane opens, `profileId: "claude"` recorded) hold
// whether or not `claude` is actually on `PATH`. Verified empirically
// (2026-07-26): with the "claude" profile's `command` temporarily
// repointed at a nonexistent binary
// (`definitely-not-a-real-claude-binary-260725`), `npx playwright test
// --grep "agent spawn profile"` still passed (1 passed, 1.4s, exit 0);
// reverted immediately after. This makes the plan's proposed skip dead code
// rather than a live guard, so it was not added.

function socketSafeTempBase(): string {
  // Mirrors dashboard-acceptance.spec.ts's `socketSafeTempBase` (same
  // sockaddr_un.sun_path length ceiling on macOS - see that file's CONTRACT
  // comment for the full explanation).
  return process.platform === "darwin" ? "/tmp" : os.tmpdir();
}

function workRootDisplayName(rootPath: string) {
  const normalized = rootPath.replace(/[\\/]+$/, "");
  const match = normalized.match(/[^\\/]+$/);
  return match ? match[0] : normalized;
}

let daemon: DaemonHandle;
let workRoot: string;
let stateHome: string;
let previousStateHome: string | undefined;

test.describe.configure({ mode: "serial" });

test.beforeAll(async () => {
  workRoot = mkdtempSync(path.join(os.tmpdir(), "ws-dash-agent-spawn-"));
  writeFileSync(
    path.join(workRoot, "readme.txt"),
    "agent spawn profile browser gate fixture\n",
  );
  previousStateHome = process.env.WS_DASHBOARD_STATE_HOME;
  stateHome = mkdtempSync(
    path.join(socketSafeTempBase(), "ws-dash-agent-spawn-state-"),
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

async function pairOwner(page: Page) {
  await page.goto(daemon.pairingUrl, { waitUntil: "domcontentloaded" });
  await expect(page.locator(".app-shell")).toBeVisible();
  expect(new URL(page.url()).pathname).not.toContain("/pair");
}

// Minimal workRoot open - this gate does not re-test root-picker UI polish
// (cancel/pin/reload persistence are already covered exhaustively by
// dashboard-acceptance.spec.ts); it only needs a working open to reach the
// workbench.
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
  const row = page.locator(
    '.resource-row[data-command-id="resource.select"]',
    { hasText: label },
  );
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

// Dockview only mounts a group's ACTIVE tab's pane body
// (`.terminal-pane[data-terminal-id=...]`) - every other tab in the group
// stays a tab-bar entry with no mounted body. A tab created via the "New
// terminal"/"New agent terminal" toolbar buttons is auto-focused by
// `createTerminalPane` (`setFocusedTerminalPaneId`/`setActiveTerminalPaneRequest`
// in App.tsx) so its body mounts immediately, but a session that only
// EXISTS on the daemon (created via direct fetch, or restored after a
// reload/reselect) is not automatically the active tab - it must be
// clicked first. `paneId` embeds a URL-encoded server-scoped identity
// (`terminal:server-local%2Fterm_xxx`), so match by substring on the raw
// terminal id rather than an exact `data-workbench-pane-id` value.
async function focusTerminalTab(page: Page, terminalId: string) {
  const tab = page.locator(
    `.dockview-workbench-tab[data-workbench-pane-id*="${terminalId}"]`,
  );
  await expect(tab).toHaveCount(1);
  await tab.click();
}

// `data-workbench-pane-id` embeds a URL-encoded server-scoped identity
// (`terminal:server-local%2Fterm_xxx`, via `resourceModel.ts`'s
// `serverScopedIdentity`), not the bare terminal id - so this reads the
// raw id straight off the newly-AUTO-FOCUSED pane's own
// `.terminal-pane[data-terminal-id=...]` instead of trying to parse the
// tab's encoded pane id. `createTerminalPane` (`App.tsx`) focuses every
// pane it creates (`setFocusedTerminalPaneId`/`setActiveTerminalPaneRequest`),
// and Dockview mounts only the active tab's pane body per group, so
// exactly one `.terminal-pane` is mounted at a time in this single-group
// layout - diffing its `data-terminal-id` before/after the click is enough.
async function currentTerminalPaneId(page: Page): Promise<string | null> {
  const ids = await page
    .locator(".terminal-pane")
    .evaluateAll((panes) => panes.map((pane) => pane.getAttribute("data-terminal-id")));
  return ids[0] ?? null;
}

async function createTerminalAndGetNewTerminalId(page: Page): Promise<string> {
  const before = await currentTerminalPaneId(page);
  await page.locator('[data-command-id="terminal.create"]').click();
  await expect
    .poll(() => currentTerminalPaneId(page), { timeout: 10_000 })
    .not.toBe(before);
  const created = await currentTerminalPaneId(page);
  expect(created, "a new terminal pane must become active").toBeTruthy();
  return created!;
}

async function createAgentTerminalAndGetNewTerminalId(
  page: Page,
): Promise<string> {
  const before = await currentTerminalPaneId(page);
  await page.locator('[data-command-id="terminal.create.agent"]').click();
  await expect
    .poll(() => currentTerminalPaneId(page), { timeout: 10_000 })
    .not.toBe(before);
  const created = await currentTerminalPaneId(page);
  expect(created, "a new agent terminal pane must become active").toBeTruthy();
  return created!;
}

async function closeTerminalById(page: Page, terminalId: string) {
  if (!terminalId) {
    return;
  }
  const tab = page.locator(
    `.dockview-workbench-tab[data-workbench-pane-id*="${terminalId}"]`,
  );
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

test("agent spawn profile", async ({ page }) => {
  await pairOwner(page);
  await openWorkRootMinimal(page, workRoot);
  const workRootId = await resolveWorkRootId(page, workRoot);

  let dummyTerminalId = "";
  await test.step("dummy profile spawns via direct API and renders with its recorded profile", async () => {
    // Direct authenticated fetch, exactly like the fixture-setup pattern
    // already used in dashboard-acceptance.spec.ts
    // (`resourceIdsForWorkRootLabel`) - `fetch` from inside `page.evaluate`
    // inherits the page's owner-session cookie for free. `"dummy-echo"` is a
    // real always-registered daemon profile that is never exposed to any
    // user-facing control (plan design answer 3) - only this test and
    // `agent_profile_registry.rs` know its id.
    const endpoint = workRootTerminalsEndpoint(workRootId);
    const created = await page.evaluate(
      async ({ endpoint, workRootId }) => {
        const response = await fetch(endpoint, {
          method: "POST",
          headers: {
            Accept: "application/json",
            "Content-Type": "application/json",
          },
          body: JSON.stringify({
            columns: 80,
            rows: 24,
            title: "Dummy Profile Probe",
            cwdHint: null,
            profileId: "dummy-echo",
          }),
        });
        const body = await response.json();
        return { ok: response.ok, status: response.status, body, workRootId };
      },
      { endpoint, workRootId },
    );
    expect(created.ok, `terminal create with dummy-echo profile: ${JSON.stringify(created)}`).toBe(true);
    expect(created.body.profileId).toBe("dummy-echo");
    dummyTerminalId = created.body.terminalId as string;
    expect(dummyTerminalId).toBeTruthy();

    // The direct fetch above bypassed this browser tab's own React state -
    // reload and reselect the workRoot (the same daemon-owns-the-lifecycle
    // restoration path `dashboard-acceptance.spec.ts`'s "refresh without
    // mock surfaces" step already exercises) so the daemon-known session is
    // re-listed and rendered as a real Dockview pane.
    await page.reload({ waitUntil: "domcontentloaded" });
    await selectWorkRootMinimal(page, workRoot);
    await focusTerminalTab(page, dummyTerminalId);

    const pane = page.locator(
      `.terminal-pane[data-terminal-id="${dummyTerminalId}"]`,
    );
    await expect(pane).toHaveCount(1);
    await expect(pane).toHaveAttribute("data-profile-id", "dummy-echo");
  });

  let plainTerminalId = "";
  await test.step("existing terminal.create toolbar flow is unchanged - profileId stays absent (regression)", async () => {
    plainTerminalId = await createTerminalAndGetNewTerminalId(page);
    await focusTerminalTab(page, plainTerminalId);
    const pane = page.locator(
      `.terminal-pane[data-terminal-id="${plainTerminalId}"]`,
    );
    await expect(pane).toHaveCount(1);
    // TerminalSessionView.profileId is `string | null`; the frontend renders
    // `pane.session.profileId ?? ""` for the unchanged default-shell path.
    await expect(pane).toHaveAttribute("data-profile-id", "");
  });

  let agentTerminalId = "";
  await test.step("New agent terminal toolbar button dispatches terminal.create.agent, not the suspended agent-chat surface", async () => {
    // The button must exist and be clickable regardless of
    // AGENT_GUI_SUSPENDED - it is a parallel path through
    // `terminal.create`-family plumbing, not one of the three suspended
    // agent-GUI depths (toolbar button, `a n` hotkey,
    // `registerNewAgentChatPane` itself - see `agentGuiSuspended.ts`).
    const agentButton = page.locator('[data-command-id="terminal.create.agent"]');
    await expect(agentButton).toBeVisible();
    await expect(agentButton).toBeEnabled();

    // No agent-chat surface exists anywhere before the click (expected,
    // since AGENT_GUI_SUSPENDED hides that button entirely) - recorded here
    // as the explicit BEFORE baseline for the AFTER assertion below. Note
    // (T-Minor, review cycle 1): this DOM-absence check alone cannot
    // distinguish correct routing from a mis-wired button, because
    // `registerNewAgentChatPane` already no-ops under `AGENT_GUI_SUSPENDED`
    // regardless of which command fired it - see the AFTER assertion's own
    // note below for the load-bearing check.
    await expect(page.locator('.workbench-pane[data-surface-kind="agentChat"]')).toHaveCount(0);
    await expect(page.locator('[data-command-id="agentChat.create"]')).toHaveCount(0);

    agentTerminalId = await createAgentTerminalAndGetNewTerminalId(page);

    // Command-observer evidence (the dashboard's existing recent-command
    // mechanism, `App.tsx`'s `commandLog` surfaced as
    // `data-last-command-id` on `.workbench-toolbar`): the toolbar's own
    // click handler dispatched `terminal.create.agent`, not `agentChat.create`
    // and not a bare `terminal.create`.
    await expect(page.locator(".workbench-toolbar")).toHaveAttribute(
      "data-last-command-id",
      "terminal.create.agent",
    );

    // Pane-level provenance: an ordinary persistentTerminal pane recorded
    // with profileId "claude" - this does NOT wait for or assert that the
    // underlying `claude` binary actually starts successfully (Phase 3+
    // owns hook/behavior verification); only the browser-visible dispatch
    // and pane provenance are asserted, so this suite acquires no
    // dependency on a vendor binary, credentials, or network.
    await focusTerminalTab(page, agentTerminalId);
    const pane = page.locator(
      `.terminal-pane[data-terminal-id="${agentTerminalId}"]`,
    );
    await expect(pane).toHaveCount(1);
    await expect(pane).toHaveAttribute("data-profile-id", "claude");
    const paneWrapper = page.locator(".workbench-pane", { has: pane });
    await expect(paneWrapper).toHaveAttribute("data-surface-kind", "persistentTerminal");

    // Corroborating evidence, not the load-bearing check (T-Minor, review
    // cycle 1): no agentChat surface exists after the click either. This
    // alone does NOT prove `registerNewAgentChatPane` was never reached -
    // that function already no-ops under `AGENT_GUI_SUSPENDED`
    // (`App.tsx:5420`) regardless of which command routed to it, so even a
    // hypothetical mis-wiring to `agentChat.create` would still produce
    // zero agentChat panes here. The `data-last-command-id` assertion above
    // is what actually distinguishes correct routing (`terminal.create.agent`)
    // from a regression, since it reads the command id directly rather than
    // an effect the suspension guard would swallow either way.
    await expect(page.locator('.workbench-pane[data-surface-kind="agentChat"]')).toHaveCount(0);
  });

  let respawnedAgentTerminalId = "";
  await test.step("restore-intent respawn preserves profileId after a daemon-side session loss (C2)", async () => {
    // Close the two non-agent terminals through the normal UI close flow
    // first - that flow itself recomputes and re-saves this workRoot's
    // restore intents from the panes that remain
    // (`persistTerminalPanesForWorkRoot`), so it correctly drops their
    // intents rather than leaving unrelated stale entries behind. Only
    // `agentTerminalId`'s intent (profileId "claude") should survive past
    // this point.
    await closeTerminalById(page, dummyTerminalId);
    await closeTerminalById(page, plainTerminalId);
    // `closeTerminalById` only waits for the confirm popover to dismiss,
    // not for the underlying async `closeTerminal().then(setTerminalPanes)`
    // chain (the write that narrows this workRoot's persisted restore
    // intents down to just the agent terminal) to settle. Wait for the
    // DOM to reflect exactly one remaining tab before proceeding, so the
    // direct-delete + reload below race against a stable, fully-persisted
    // intent set rather than a still-in-flight one.
    await expect(terminalTabsLocator(page)).toHaveCount(1, { timeout: 10_000 });

    // Simulate the daemon losing the live agent session (e.g. a restart
    // the registry didn't survive) WITHOUT going through the browser's own
    // close-tab flow, so the browser-side restore intent written when the
    // agent terminal was spawned is left stale in localStorage exactly as
    // it would be after a real daemon restart that dropped this terminal -
    // mirrors the direct-fetch bypass pattern the dummy-echo step above
    // already uses for the opposite direction (daemon knows, browser
    // doesn't).
    const closeEndpoint = terminalCloseEndpoint(agentTerminalId);
    const deleted = await page.evaluate(async (endpoint) => {
      const response = await fetch(endpoint, { method: "DELETE" });
      return { ok: response.ok, status: response.status };
    }, closeEndpoint);
    expect(
      deleted.ok || deleted.status === 404,
      `direct terminal delete: ${JSON.stringify(deleted)}`,
    ).toBe(true);

    // Reload + reselect: `listTerminals` now returns zero sessions for this
    // workRoot (dummy/plain were closed normally above, and the agent
    // terminal was just deleted directly), so the browser's
    // restore-intent effect (`App.tsx`'s `restoredTerminalIntentRoots`)
    // fires and respawns from the stale intent.
    await page.reload({ waitUntil: "domcontentloaded" });
    await selectWorkRootMinimal(page, workRoot);

    await expect(terminalTabsLocator(page)).toHaveCount(1, { timeout: 10_000 });
    const respawnedId = await currentTerminalPaneId(page);
    expect(
      respawnedId,
      "the restore-intent respawn must produce a mounted pane",
    ).toBeTruthy();
    expect(respawnedId).not.toBe(agentTerminalId);
    respawnedAgentTerminalId = respawnedId!;

    // The C2 assertion: before the fix, this respawned terminal silently
    // ran the default shell (profileId absent/null) under an unchanged
    // title - the browser had no way to tell the user the wrong process
    // was now running behind an agent-terminal-looking tab.
    await expect(
      page.locator(`.terminal-pane[data-terminal-id="${respawnedId}"]`),
    ).toHaveAttribute("data-profile-id", "claude");
  });

  await test.step("cleanup: close every terminal this gate spawned", async () => {
    // This spec owns a dedicated daemon/workRoot (see the file-level
    // CONTRACT comment), so cleanup only needs to leave no live child
    // processes behind for `daemon.stop()` - it does not need to restore
    // any shared cross-file terminal count. `dummyTerminalId`/
    // `plainTerminalId` were already closed in the step above, and the
    // original `agentTerminalId` was already deleted directly (the
    // respawned terminal has a different id) - `closeTerminalById` is a
    // no-op for a terminal id with no matching tab, so re-listing the
    // originals here is harmless.
    await closeTerminalById(page, dummyTerminalId);
    await closeTerminalById(page, plainTerminalId);
    await closeTerminalById(page, agentTerminalId);
    await closeTerminalById(page, respawnedAgentTerminalId);
    await expect(terminalTabsLocator(page)).toHaveCount(0);
  });
});
