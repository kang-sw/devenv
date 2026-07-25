import { test, expect, type Page } from "@playwright/test";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { startDaemon, type DaemonHandle } from "./daemonHarness.js";
import { workRootTerminalsEndpoint } from "../src/terminals.js";

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
    // as the explicit BEFORE baseline for the AFTER assertion below.
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

    // The central constraint, proven directly rather than only inferred
    // from the command id: the click never registered (even transiently)
    // an agentChat surface - `registerNewAgentChatPane` was never reached.
    await expect(page.locator('.workbench-pane[data-surface-kind="agentChat"]')).toHaveCount(0);
  });

  await test.step("cleanup: close every terminal this gate spawned", async () => {
    // This spec owns a dedicated daemon/workRoot (see the file-level
    // CONTRACT comment), so cleanup only needs to leave no live child
    // processes behind for `daemon.stop()` - it does not need to restore
    // any shared cross-file terminal count.
    await closeTerminalById(page, dummyTerminalId);
    await closeTerminalById(page, plainTerminalId);
    await closeTerminalById(page, agentTerminalId);
    await expect(terminalTabsLocator(page)).toHaveCount(0);
  });
});
