import { test, expect, type Page, type Locator } from "@playwright/test";
import { fileURLToPath } from "node:url";
import { existsSync, mkdtempSync, mkdirSync, readFileSync, writeFileSync, rmSync } from "node:fs";
import { execFileSync } from "node:child_process";
import os from "node:os";
import path from "node:path";
import { startDaemon, type DaemonHandle } from "./daemonHarness.js";
import {
  terminalCommandPlanForPlatform,
  type TerminalCommandPlan,
} from "../src/terminalCommandPlan.js";
// mirrors src/agentGuiSuspended.ts - agent GUI suspended 2026-07-25 (260713
// family). While true, the agent-GUI acceptance steps below are quarantined.
import { AGENT_GUI_SUSPENDED } from "../src/agentGuiSuspended.js";
import type { TerminalPortabilityEvidence } from "./terminalPortabilityEvidence.js";

// Browser-level acceptance gate for the dashboard workRoot UI.
//
// This gate drives the daemon-served production frontend after owner pairing
// and checks the user-reported failure set: terminal tab selection, non-mock
// initial workbench state, real terminal emulator rendering/input/sizing, and
// conventional read-only file explorer affordances.
//
// CONTRACT: Workbench tab polish evidence belongs in this Playwright gate.
// Coverage must drive hover-only tab close affordances, terminal/agent
// cursor-near Yes/No confirmation popovers, immediate close for reversible
// panes, pinned/opened tab group or chip presentation, and preview-to-pinned
// file behavior against the daemon-served production frontend.

const here = path.dirname(fileURLToPath(import.meta.url));
const artifactsDir = path.join(here, ".artifacts");

let daemon: DaemonHandle;
let workRoot: string;
let secondWorkRoot: string | null = null;
let gitWorkRoot: string | null = null;
let ownsWorkRoot = false;
let ownsSecondWorkRoot = false;
let ownsGitWorkRoot = false;
let ownsStateHome = false;
let stateHome: string | null = null;
let previousStateHome: string | undefined;
let commandPlan: TerminalCommandPlan;
let portabilityEvidence: TerminalPortabilityEvidence | undefined;
let activityFixtureRootId: string | null = null;
let activityRecentPollRequests = 0;
let rootPickerPinEvidenceRecorded = false;
let ownerCookies:
  | Awaited<ReturnType<import("@playwright/test").BrowserContext["cookies"]>>
  | undefined;

const evidence: string[] = [];
function note(line: string) {
  evidence.push(line);
}

test.describe.configure({ mode: "serial" });

test.beforeAll(async () => {
  mkdirSync(artifactsDir, { recursive: true });

  const externalWorkRoot = process.env.WS_DASHBOARD_TEST_WORKROOT;
  if (externalWorkRoot) {
    workRoot = externalWorkRoot;
  } else {
    // A deterministic temporary workRoot keeps explorer assertions stable.
    workRoot = mkdtempSync(path.join(os.tmpdir(), "ws-dash-gate-"));
    ownsWorkRoot = true;
    writeFileSync(
      path.join(workRoot, "gate-readme.txt"),
      "ws-dashboard browser gate fixture\nsecond fixture line\n",
    );
    writeFileSync(
      path.join(workRoot, "gate-long-readonly.txt"),
      Array.from(
        { length: 220 },
        (_, index) => `readonly scroll containment line ${index + 1}`,
      ).join("\n") + "\n",
    );
    writeFileSync(
      path.join(workRoot, "gate-config.toml"),
      [
        "title = \"CodeMirror source viewer\"",
        "",
        "[owner]",
        "name = \"dashboard\"",
        "enabled = true",
        "",
        ...Array.from({ length: 90 }, (_, index) => `source_scroll_line_${index + 1} = ${index + 1}`),
      ].join("\n") + "\n",
    );
    writeFileSync(
      path.join(workRoot, "gate-unknown.workflowx"),
      "plain fallback source viewer fixture\n",
    );
    writeFileSync(
      path.join(workRoot, "gate-document.md"),
      [
        "# Gate Document",
        "",
        "Markdown paragraph line with `inline code`",
        "with soft continuation",
        "",
        "- [x] completed task",
        "- parent item",
        "  - nested item",
        "",
        "5. ordered fifth",
        "6. ordered sixth",
        "",
        "> [!note] Browser note",
        "> callout body",
        "",
        "| Kind | Value |",
        "| --- | --- |",
        "| table | rendered |",
      ].join("\n") + "\n",
    );
    mkdirSync(path.join(workRoot, "gate-subdir"));
    writeFileSync(
      path.join(workRoot, "gate-subdir", "nested.txt"),
      "nested gate file\n",
    );

    // Many root files make the explorer tree far taller than its pane so the
    // viewport-containment assertion below is meaningful.
    for (let index = 0; index < 80; index += 1) {
      writeFileSync(
        path.join(workRoot, `gate-bulk-${String(index).padStart(3, "0")}.txt`),
        `bulk gate fixture ${index}\n`,
      );
    }
  }

  const externalDaemon = Boolean(
    process.env.WS_DASHBOARD_DAEMON_MODE === "external" ||
      process.env.WS_DASHBOARD_DAEMON_BASE_URL ||
      process.env.WS_DASHBOARD_DAEMON_PAIRING_URL,
  );
  if (process.env.WS_DASHBOARD_TEST_GIT_WORKROOT) {
    gitWorkRoot = process.env.WS_DASHBOARD_TEST_GIT_WORKROOT;
  } else if (!externalDaemon) {
    gitWorkRoot = mkdtempSync(path.join(os.tmpdir(), "ws-dash-git-gate-"));
    ownsGitWorkRoot = true;
    initGitFixture(gitWorkRoot);
  }
  previousStateHome = process.env.WS_DASHBOARD_STATE_HOME;
  if (!externalDaemon) {
    stateHome = mkdtempSync(path.join(os.tmpdir(), "ws-dash-state-"));
    ownsStateHome = true;
    process.env.WS_DASHBOARD_STATE_HOME = stateHome;
  }

  daemon = await startDaemon();

  const externalSecondWorkRoot = process.env.WS_DASHBOARD_TEST_SECOND_WORKROOT;
  if (externalSecondWorkRoot) {
    secondWorkRoot = externalSecondWorkRoot;
  } else if (daemon.mode === "spawned") {
    secondWorkRoot = mkdtempSync(
      path.join(os.tmpdir(), "ws-dash-gate-second-"),
    );
    ownsSecondWorkRoot = true;
    writeFileSync(
      path.join(secondWorkRoot, "second-readme.txt"),
      "second ws-dashboard browser gate fixture\n",
    );
  }
  const shellProfileHint = process.env.WS_DASHBOARD_TERMINAL_SHELL_PROFILE;
  const targetPlatform = process.env.WS_DASHBOARD_TERMINAL_PLATFORM;
  if (daemon.mode === "external" && !shellProfileHint && !targetPlatform) {
    throw new Error(
      "external daemon browser gate requires WS_DASHBOARD_TERMINAL_SHELL_PROFILE or WS_DASHBOARD_TERMINAL_PLATFORM so command helpers match the remote daemon shell",
    );
  }
  commandPlan = terminalCommandPlanForPlatform(
    targetPlatform ?? process.platform,
    shellProfileHint,
  );
  portabilityEvidence = {
    os: `${os.type()} ${os.release()}`,
    platform: process.platform,
    shellProfile: commandPlan.profile,
    daemon: {
      mode: daemon.mode,
      command: daemon.command,
      baseUrl: daemon.baseUrl,
      pairingUrlSource: daemon.mode === "spawned" ? "scraped" : "provided",
    },
    forwarding: {
      used: daemon.mode === "external",
      kind: daemon.mode === "external" ? "ssh-local-forward" : undefined,
      localEndpoint:
        daemon.mode === "external" ? new URL(daemon.baseUrl).host : undefined,
      remoteEndpoint:
        daemon.mode === "external" ? "loopback-fixed-endpoint" : undefined,
    },
    readiness: {
      signal: daemon.readinessSignal,
      result: "pass",
      detail:
        daemon.mode === "spawned"
          ? "pairing URL scraped and /healthz reachable"
          : "external /healthz reachable",
    },
    browserGate: {
      result: "skipped",
      commandProfile: commandPlan.profile,
      limitations: [...commandPlan.limitations],
    },
  };
  note(`daemon base URL: ${daemon.baseUrl}`);
  note(`terminal command profile: ${commandPlan.profile}`);
  note(`test workRoot: ${workRootDisplayName(workRoot)}`);
  if (secondWorkRoot) {
    note(`second test workRoot: ${workRootDisplayName(secondWorkRoot)}`);
  } else {
    note("second test workRoot: not configured for external daemon");
  }
  if (stateHome) {
    note("spawned daemon state: isolated temporary state home");
  }
});

test.afterEach(async ({}, testInfo) => {
  if (portabilityEvidence && testInfo.status !== "passed") {
    portabilityEvidence.browserGate.result = "fail";
  }
});

test.afterAll(async () => {
  if (daemon) {
    await daemon.stop();
  }
  if (workRoot && ownsWorkRoot) {
    rmSync(workRoot, { recursive: true, force: true });
  }
  if (secondWorkRoot && ownsSecondWorkRoot) {
    rmSync(secondWorkRoot, { recursive: true, force: true });
  }
  if (gitWorkRoot && ownsGitWorkRoot) {
    rmSync(gitWorkRoot, { recursive: true, force: true });
  }
  if (ownsStateHome && stateHome) {
    rmSync(stateHome, { recursive: true, force: true });
  }
  if (previousStateHome === undefined) {
    delete process.env.WS_DASHBOARD_STATE_HOME;
  } else {
    process.env.WS_DASHBOARD_STATE_HOME = previousStateHome;
  }
  if (portabilityEvidence) {
    writeFileSync(
      path.join(artifactsDir, "terminal-portability-evidence.json"),
      `${JSON.stringify(portabilityEvidence, null, 2)}\n`,
    );
  }
  writeFileSync(
    path.join(artifactsDir, "evidence.txt"),
    `${evidence.join("\n")}\n`,
  );
});

async function terminalSurface(page: Page) {
  const surface = page.locator(".terminal-surface");
  await expect(surface).toBeVisible();
  await expect(surface.locator(".xterm")).toBeVisible();
  return surface;
}

async function terminalInputTarget(page: Page) {
  const surface = await terminalSurface(page);
  const inputTarget = surface.locator(".xterm-helper-textarea");
  await expect(inputTarget).toBeAttached();
  return inputTarget;
}

async function expectTerminalInputFocused(page: Page) {
  await expect
    .poll(() =>
      page.evaluate(() => {
        const element = document.activeElement;
        return {
          className:
            typeof element?.className === "string" ? element.className : "",
          tagName: element?.tagName ?? "",
        };
      }),
    )
    .toMatchObject({
      className: expect.stringContaining("xterm-helper-textarea"),
      tagName: "TEXTAREA",
    });
}


function initGitFixture(rootPath: string) {
  execFileSync("git", ["init"], { cwd: rootPath, stdio: "ignore" });
  execFileSync("git", ["config", "user.email", "ws-dashboard@example.local"], { cwd: rootPath });
  execFileSync("git", ["config", "user.name", "ws dashboard"], { cwd: rootPath });
  writeFileSync(path.join(rootPath, "README.md"), "git browser gate fixture\n");
  execFileSync("git", ["add", "README.md"], { cwd: rootPath });
  execFileSync("git", ["commit", "-m", "seed"], { cwd: rootPath, stdio: "ignore" });
  execFileSync("git", ["branch", "existing-browser-branch"], { cwd: rootPath });
}

function workRootDisplayName(rootPath: string) {
  const normalized = rootPath.replace(/[\\/]+$/, "");
  const match = normalized.match(/[^\\/]+$/);
  return match ? match[0] : normalized;
}

async function runInTerminal(page: Page, command: string) {
  await page.locator(".terminal-surface").click();
  await page.keyboard.type(command);
  await page.keyboard.press("Enter");
}

// 260722-feat-dashboard-which-key-hint-overlay Phase 2: proves a dismissal
// path left the terminal passthrough guard intact - focuses the currently
// active terminal surface and confirms a fresh echo round-trips through it,
// rather than only asserting the overlay's own DOM disappeared.
//
// `.first()`-scoped rather than a bare `.terminal-surface`/`.xterm-rows`
// locator: exactly one terminal pane exists at every call site today, but
// once 260724-idea-dashboard-hotkey-leader-dispatch-gap is fixed, dismissal
// path 1's `<leader> t n` will start actually creating a second terminal
// pane, and a bare locator would become an ambiguous Playwright strict-mode
// multi-match. `.first()` targets whichever terminal surface is currently
// frontmost, matching this helper's intent - prove *the active* terminal
// still accepts input after a dismissal path, not a specific pane identity -
// and stays valid once that bug is fixed.
async function expectTerminalNotBlocked(page: Page, tag: string) {
  const surface = page.locator(".terminal-surface").first();
  await surface.click();
  await page.keyboard.type(commandPlan.echo(tag));
  await page.keyboard.press("Enter");
  await expect(surface.locator(".xterm-rows")).toContainText(tag, {
    timeout: 5_000,
  });
}

function terminalTabs(page: Page) {
  return page.getByRole("tab").filter({ hasText: "Terminal" });
}

async function expectDockviewWorkbench(page: Page) {
  const owner = page.locator(
    '[data-workbench-root-active="true"] [data-workbench-layout-owner="dockview"]',
  );
  await expect(owner).toBeVisible();
  // CONTRACT: The visible workbench must be backed by Dockview, not the retired
  // custom `.workbench-splits > .workbench-group` tab/split shell.
  await expect(owner.locator(".dv-dockview")).toBeVisible();
  await expect(
    page.locator(".workbench-splits > .workbench-group"),
  ).toHaveCount(0);
}

async function expectContextSurfaceHierarchy(page: Page) {
  const hierarchy = await page.evaluate(() => {
    const rootStyle = getComputedStyle(document.documentElement);
    const navStyle = getComputedStyle(document.querySelector(".shell-panel-nav")!);
    const workbenchStyle = getComputedStyle(
      document.querySelector(".shell-panel-workbench")!,
    );
    const toolbarStyle = getComputedStyle(document.querySelector(".workbench-toolbar")!);
    const fileExplorerStyle = getComputedStyle(document.querySelector(".file-explorer")!);
    const fileExplorerHeaderStyle = getComputedStyle(
      document.querySelector(".file-explorer-header")!,
    );
    const layoutStyle = getComputedStyle(
      document.querySelector(".dockview-workbench-layout")!,
    );
    const tabbarStyle = getComputedStyle(
      document.querySelector(".dv-tabs-and-actions-container")!,
    );
    const paneBodyStyle = getComputedStyle(
      document.querySelector(".workbench-pane-body")!,
    );

    return {
      navBackground: navStyle.backgroundColor,
      workbenchBackground: workbenchStyle.backgroundColor,
      toolbarBackground: toolbarStyle.backgroundColor,
      toolbarDivider: toolbarStyle.borderBottomColor,
      fileExplorerBackground: fileExplorerStyle.backgroundColor,
      fileExplorerHeaderBackground: fileExplorerHeaderStyle.backgroundColor,
      dockviewBorderTopWidth: layoutStyle.borderTopWidth,
      dockviewBackground: layoutStyle.backgroundColor,
      tabbarBackground: tabbarStyle.backgroundColor,
      tabbarDivider: tabbarStyle.borderBottomColor,
      paneBodyBackground: paneBodyStyle.backgroundColor,
      structuralBorderWidth: rootStyle
        .getPropertyValue("--ws-border-width-structural")
        .trim(),
      localDivider: rootStyle.getPropertyValue("--ws-color-divider-local").trim(),
      contextDivider: rootStyle.getPropertyValue("--ws-color-divider-context").trim(),
      structuralDivider: rootStyle
        .getPropertyValue("--ws-color-divider-structural")
        .trim(),
      splitGutter: rootStyle.getPropertyValue("--ws-color-split-gutter").trim(),
      splitGutterSize: rootStyle.getPropertyValue("--ws-split-gutter-size").trim(),
    };
  });

  expect(hierarchy.navBackground).not.toBe(hierarchy.workbenchBackground);
  expect(hierarchy.tabbarBackground).not.toBe(hierarchy.toolbarBackground);
  expect(hierarchy.fileExplorerBackground).not.toBe(hierarchy.navBackground);
  expect(hierarchy.fileExplorerHeaderBackground).not.toBe(
    hierarchy.fileExplorerBackground,
  );
  expect(hierarchy.toolbarBackground).not.toBe(hierarchy.paneBodyBackground);
  expect(hierarchy.tabbarBackground).not.toBe(hierarchy.paneBodyBackground);
  expect(hierarchy.toolbarDivider).not.toBe(hierarchy.tabbarDivider);
  expect(hierarchy.localDivider).not.toBe(hierarchy.contextDivider);
  expect(hierarchy.contextDivider).not.toBe(hierarchy.structuralDivider);
  expect(hierarchy.dockviewBorderTopWidth).toBe(hierarchy.structuralBorderWidth);
  expect(hierarchy.dockviewBackground).not.toBe(hierarchy.paneBodyBackground);
  expect(hierarchy.splitGutter).toBeTruthy();
}

async function expectDurableDockviewSplitDrop(
  page: Page,
): Promise<{ paneId: string; groupId: string }> {
  // CONTRACT: Browser acceptance must prove that a Dockview split-drop preview
  // creates or maps a durable dashboard group. After dragging a workbench tab
  // into a new split target, the moved pane keeps a distinct
  // `data-workbench-group-id` after React synchronization, and file/terminal
  // interactions still work in the resulting split layout.
  // Use `.dockview-workbench-tab`, `data-workbench-pane-id`,
  // `data-workbench-group-id`, expectDockviewWorkbench, and settlePastPollCycle.
  // Drag coordinates should target Dockview's split overlay near the workbench
  // body midpoint so the preview and resulting group are both observable.
  const owner = page.locator('[data-workbench-layout-owner="dockview"]');
  const movedTab = page
    .locator('.dockview-workbench-tab[data-workbench-pane-id^="readonly"]')
    .first();
  await expect(movedTab).toBeVisible();
  const paneId = await movedTab.getAttribute("data-workbench-pane-id");
  const originalGroupId = await movedTab.getAttribute(
    "data-workbench-group-id",
  );
  expect(paneId).not.toBeNull();
  expect(originalGroupId).not.toBeNull();

  const sourceBox = await movedTab.boundingBox();
  const ownerBox = await owner.boundingBox();
  expect(sourceBox).not.toBeNull();
  expect(ownerBox).not.toBeNull();

  await page.mouse.move(
    sourceBox!.x + sourceBox!.width / 2,
    sourceBox!.y + sourceBox!.height / 2,
  );
  await page.mouse.down();
  await page.mouse.move(
    ownerBox!.x + ownerBox!.width * 0.95,
    ownerBox!.y + ownerBox!.height * 0.5,
    { steps: 12 },
  );
  await page.mouse.up();

  const movedPane = page
    .locator(`[data-workbench-pane-id="${paneId}"]`)
    .first();
  await expect
    .poll(async () => movedPane.getAttribute("data-workbench-group-id"), {
      timeout: 10_000,
    })
    .not.toBe(originalGroupId);
  await settlePastPollCycle(page);
  const movedGroupId = await page
    .locator(`.dockview-workbench-tab[data-workbench-pane-id="${paneId}"]`)
    .getAttribute("data-workbench-group-id");
  expect(movedGroupId).toMatch(/group-[3-9][0-9]*|group-[1-9][0-9]+/);
  await expectDockviewWorkbench(page);
  await expect(page.locator(".readonly-text-pane")).toBeVisible();
  expect(
    new Set(
      await page
        .locator(".dockview-workbench-tab")
        .evaluateAll((tabs) =>
          tabs.map((tab) => tab.getAttribute("data-workbench-group-id")),
        ),
    ).size,
  ).toBeGreaterThanOrEqual(3);
  return { paneId: paneId!, groupId: movedGroupId! };
}

async function visibleWorkbenchGroupIds(page: Page): Promise<string[]> {
  return page
    .locator('[data-workbench-root-active="true"] .dockview-workbench-tab')
    .evaluateAll((tabs) =>
      Array.from(
        new Set(
          tabs
            .map((tab) => tab.getAttribute("data-workbench-group-id"))
            .filter((groupId): groupId is string => Boolean(groupId)),
        ),
      ).sort(),
    );
}

async function terminalPaneIds(page: Page): Promise<string[]> {
  return page
    .locator('.dockview-workbench-tab[data-workbench-pane-id^="terminal:"]')
    .evaluateAll((tabs) =>
      tabs.map((tab) => tab.getAttribute("data-workbench-pane-id") ?? ""),
    );
}

// Creates a terminal and returns its pane id by diffing the live terminal
// pane-id set before/after the click, instead of relying on tab DOM order
// (`.first()`/`.last()`/`.nth()`), which is not chronological once terminal
// tabs are split across more than one Dockview group (each group has its own
// tab bar, so page-wide DOM order no longer matches creation order).
async function createTerminalAndGetNewPaneId(page: Page): Promise<string> {
  const before = new Set(await terminalPaneIds(page));
  await page.locator('[data-command-id="terminal.create"]').click();
  await expect
    .poll(async () => (await terminalPaneIds(page)).length, {
      timeout: 10_000,
    })
    .toBeGreaterThan(before.size);
  const created = (await terminalPaneIds(page)).find((id) => !before.has(id));
  expect(created).toBeTruthy();
  return created!;
}

function terminalTabById(page: Page, paneId: string) {
  return page
    .locator(`.dockview-workbench-tab[data-workbench-pane-id="${paneId}"]`)
    .first();
}

// Resolves the bounding box of the `.dv-groupview` ancestor of a tab, not the
// outer `[data-workbench-layout-owner="dockview"]` container. Dockview's 20%
// edge-activation threshold (`droptarget.js` DEFAULT_ACTIVATION_SIZE) is
// evaluated against a *specific group's* rect; dropping at a boundary between
// two side-by-side groups over the owner instead hits the sash and fires no
// move (260720-bug-dashboard-terminal-split-nonhorizontal-snap-back,
// Codebase Findings).
async function groupViewBoundingBox(tab: Locator) {
  const groupView = tab
    .locator(
      "xpath=ancestor::*[contains(concat(' ', normalize-space(@class), ' '), ' dv-groupview ')]",
    )
    .first();
  const box = await groupView.boundingBox();
  expect(box).not.toBeNull();
  return box!;
}

async function dragTabToGroupEdge(
  page: Page,
  tab: Locator,
  targetGroupBox: { x: number; y: number; width: number; height: number },
  fraction: { fx: number; fy: number },
) {
  const sourceBox = await tab.boundingBox();
  expect(sourceBox).not.toBeNull();
  await page.mouse.move(
    sourceBox!.x + sourceBox!.width / 2,
    sourceBox!.y + sourceBox!.height / 2,
  );
  await page.mouse.down();
  await page.mouse.move(
    targetGroupBox.x + targetGroupBox.width / 2,
    targetGroupBox.y + targetGroupBox.height / 2,
    { steps: 6 },
  );
  await page.mouse.move(
    targetGroupBox.x + targetGroupBox.width * fraction.fx,
    targetGroupBox.y + targetGroupBox.height * fraction.fy,
    { steps: 12 },
  );
  await page.mouse.up();
}

// Terminal-pane analog of `expectDurableDockviewSplitDrop` above, generalized
// to an arbitrary target group rect and edge fraction so it can exercise
// vertical, 3-way, and inner-existing-group split gestures against a
// **terminal** pane instead of only the readonly-pane rightward
// outer-container drop
// (260720-bug-dashboard-terminal-split-nonhorizontal-snap-back).
async function expectDurableTerminalSplitDrop(
  page: Page,
  tab: Locator,
  targetGroupBox: { x: number; y: number; width: number; height: number },
  fraction: { fx: number; fy: number },
): Promise<{ paneId: string; groupId: string }> {
  const paneId = await tab.getAttribute("data-workbench-pane-id");
  const originalGroupId = await tab.getAttribute("data-workbench-group-id");
  expect(paneId).not.toBeNull();
  expect(originalGroupId).not.toBeNull();

  await dragTabToGroupEdge(page, tab, targetGroupBox, fraction);

  const movedPane = page
    .locator(`[data-workbench-pane-id="${paneId}"]`)
    .first();
  await expect
    .poll(async () => movedPane.getAttribute("data-workbench-group-id"), {
      timeout: 10_000,
    })
    .not.toBe(originalGroupId);
  await settlePastPollCycle(page);
  const movedTab = terminalTabById(page, paneId!);
  const movedGroupId = await movedTab.getAttribute("data-workbench-group-id");
  expect(movedGroupId).not.toBeNull();
  expect(movedGroupId).not.toBe(originalGroupId);
  await expect(movedTab).toBeVisible();
  return { paneId: paneId!, groupId: movedGroupId! };
}

async function openWorkRootInBrowser(page: Page, rootPath: string) {
  const opener = page.locator(
    '[data-command-id="rootPicker.open"]:not(.open-work-root-empty-cta)',
  );
  await opener.click();
  let modal = page.locator(".root-picker-modal");
  await expect(modal).toBeVisible();
  await expect(modal.locator(".root-picker-title")).toHaveText(
    "Open workRoot on this host",
  );
  await modal
    .locator('[data-command-id="rootPicker.close"]')
    .filter({ hasText: "Cancel" })
    .click();
  await expect(opener).toBeFocused();

  await opener.click();
  modal = page.locator(".root-picker-modal");
  await expect(modal).toBeVisible();
  const parentPath = path.dirname(rootPath);
  await modal.locator(".root-picker-address").fill(parentPath);
  await modal.locator(".root-picker-address").press("Enter");
  const targetRow = modal
    .locator(".root-picker-row", { hasText: workRootDisplayName(rootPath) })
    .first();
  await expect(targetRow).toBeVisible();
  await targetRow.focus();
  await page.keyboard.press(" ");
  await expect(modal.locator(".root-picker-selection")).toContainText(
    workRootDisplayName(rootPath),
  );
  await page.keyboard.press("Enter");
  await expect(modal.locator(".root-picker-current")).toContainText(
    workRootDisplayName(rootPath),
  );
  if (!rootPickerPinEvidenceRecorded) {
    await modal.locator('[data-command-id="rootPicker.pinDirectory"]').click();
    await expect(
      modal.locator(".root-picker-place-row-pinned", {
        hasText: workRootDisplayName(rootPath),
      }),
    ).toBeVisible();
    await page.reload();
    await expect(opener).toBeVisible();
    await opener.click();
    modal = page.locator(".root-picker-modal");
    await expect(
      modal.locator(".root-picker-place-row-pinned", {
        hasText: workRootDisplayName(rootPath),
      }),
    ).toBeVisible();
    rootPickerPinEvidenceRecorded = true;
  }
  await modal.locator("#root-picker-exact-path").fill(rootPath);
  await modal.locator('[data-command-id="workRoot.open"]').filter({ hasText: "Open" }).click();
  await expect(page.locator(".file-explorer-title")).toContainText(
    workRootDisplayName(rootPath),
  );
  await expect(modal).toHaveCount(0);
  await expectDockviewWorkbench(page);
  note(
    "open workRoot: React Aria picker restored opener focus, navigated by address, selected/actioned a row by keyboard, persisted a pinned directory across refresh, and opened exact typed path",
  );
}

async function selectWorkRootInBrowser(page: Page, rootPath: string) {
  const label = workRootDisplayName(rootPath);
  const ids = await resourceIdsForWorkRootLabel(page, label);
  const directRow = page.locator('.resource-row[data-command-id="resource.select"][data-resource-presentation="workRoot"], .resource-row[data-command-id="resource.select"][data-resource-presentation="compactWorkRoot"]', {
    hasText: label,
  });
  if (await directRow.count()) {
    await directRow.first().click();
  } else if (ids?.workspaceId) {
    await page
      .locator(`.resource-row[data-command-id="resource.select"][data-resource-id="${ids.workspaceId}"]`)
      .first()
      .click();
  } else {
    await page
      .locator('.resource-row[data-command-id="resource.select"][data-resource-presentation="workspace"]', {
        hasText: label,
      })
      .first()
      .click();
  }
  await expect(page.locator(".file-explorer-title")).toContainText(
    label,
  );
  await expectDockviewWorkbench(page);
}

async function resourceIdsForWorkRootLabel(
  page: Page,
  label: string,
): Promise<{ workRootId: string; workspaceId: string } | null> {
  return page.evaluate(async (targetLabel) => {
    const response = await fetch("/api/dashboard/resources");
    const resources = (await response.json()) as {
      workspaces?: Array<{
        id?: string;
        workRoots?: Array<{
          id?: string;
          label?: string;
          resourcePath?: { workspaceId?: string; workRootId?: string };
        }>;
      }>;
    };
    for (const workspace of resources.workspaces ?? []) {
      for (const workRoot of workspace.workRoots ?? []) {
        if (workRoot.label === targetLabel) {
          const workspaceId = workRoot.resourcePath?.workspaceId ?? workspace.id ?? null;
          const workRootId = workRoot.resourcePath?.workRootId ?? workRoot.id ?? null;
          return workspaceId && workRootId ? { workspaceId, workRootId } : null;
        }
      }
    }
    return null;
  }, label);
}

async function workRootIdForLabel(page: Page, label: string): Promise<string | null> {
  return page.evaluate(async (targetLabel) => {
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
      for (const workRoot of workspace.workRoots ?? []) {
        if (workRoot.label === targetLabel) {
          return workRoot.resourcePath?.workRootId ?? workRoot.id ?? null;
        }
      }
    }
    return null;
  }, label);
}

// The terminal pane footer renders `<status> · <columns>x<rows>` from the
// daemon-confirmed session size, so it reflects forwarded PTY resizes.
async function terminalColumns(page: Page): Promise<number> {
  const text =
    (await page.locator(".terminal-status-line").first().textContent()) ?? "";
  const match = text.match(/(\d+)x(\d+)/i);
  return match ? Number(match[1]) : Number.NaN;
}

// Sibling of `terminalColumns` above, parsing the daemon-confirmed row count
// from the same `<status> · <columns>x<rows>` footer text.
async function terminalRows(page: Page): Promise<number> {
  const text =
    (await page.locator(".terminal-status-line").first().textContent()) ?? "";
  const match = text.match(/(\d+)x(\d+)/i);
  return match ? Number(match[2]) : Number.NaN;
}

// Independent-of-the-daemon-footer proxy for the emulator's own row count:
// one `.xterm-rows > div` per rendered emulator row.
async function emulatorRowCount(page: Page): Promise<number> {
  return page.locator(".xterm-rows > div").count();
}

// The terminal output poll cycle is ~120ms; a 900ms settle covers several
// cycles and proves a tab selection survives the poll-driven `editorGroups`
// rebuild.
async function settlePastPollCycle(page: Page) {
  await page.waitForTimeout(900);
}

// The shell is viewport-bounded: long pane content scrolls inside its own
// region rather than growing the document and pushing the footer off-screen.
async function documentScrolls(page: Page): Promise<boolean> {
  return page.evaluate(() => {
    const root = document.scrollingElement ?? document.documentElement;
    return root.scrollHeight > root.clientHeight + 1;
  });
}

function linkedServerBrowserServers() {
  return {
    servers: [
      {
        id: "server-local",
        label: "Local ws dashboard",
        kind: "local",
        status: "connected",
        state: {
          status: "connected",
          loading: false,
          stale: false,
          error: null,
        },
        actions: [
          { id: "refresh", label: "Refresh", enabled: true },
          { id: "openRoot", label: "Open root", enabled: true },
        ],
      },
      {
        id: "server-remote",
        label: "Remote fixture",
        kind: "manual",
        status: "connected",
        state: {
          status: "connected",
          loading: false,
          stale: false,
          error: null,
        },
        actions: [
          { id: "refresh", label: "Refresh", enabled: true },
          { id: "openRoot", label: "Open root", enabled: true },
        ],
      },
      {
        id: "server-other",
        label: "Other remote",
        kind: "manual",
        status: "connected",
        state: {
          status: "connected",
          loading: false,
          stale: false,
          error: null,
        },
        actions: [
          { id: "refresh", label: "Refresh", enabled: true },
          { id: "openRoot", label: "Open root", enabled: true },
        ],
      },
    ],
  };
}

function linkedServerBrowserResources(serverRoute: string, workRootId?: string) {
  const hasRoot = Boolean(workRootId);
  return {
    server: {
      id: serverRoute,
      label:
        serverRoute === "server-local"
          ? "Local ws dashboard"
          : serverRoute === "server-remote"
            ? "Remote fixture"
            : "Other remote",
      state: {
        status: "connected",
        loading: false,
        stale: false,
        error: null,
      },
      actions: [
        { id: "refresh", label: "Refresh", enabled: true },
        { id: "openRoot", label: "Open root", enabled: true },
      ],
    },
    workspaces: hasRoot
      ? [
          {
            id: `workspace-${serverRoute}`,
            label: `workspace-${serverRoute}`,
            state: {
              status: "ready",
              loading: false,
              stale: false,
              error: null,
            },
            compactable: false,
            actions: [],
            workRoots: [
              {
                id: workRootId,
                resourcePath: {
                  serverId: serverRoute,
                  workspaceId: `workspace-${serverRoute}`,
                  workRootId,
                  instanceId: null,
                },
                label: "remote-opened",
                kind: "plainDirectory",
                activation: "online",
                availability: "available",
                status: "online",
                state: {
                  status: "ready",
                  loading: false,
                  stale: false,
                  error: null,
                },
                compactable: false,
                mainInstances: [],
                actions: [],
              },
            ],
          },
        ]
      : [],
  };
}

function linkedServerPickerView(currentPath: string, places: unknown[] = []) {
  return {
    currentPath,
    parentPath: currentPath === "/remote/home" ? null : "/remote/home",
    entries: [
      {
        name: "child",
        path: `${currentPath}/child`,
        entryType: "directory",
        selectable: true,
        kindLabel: "Folder",
        modifiedTime: null,
        size: null,
      },
    ],
    places,
  };
}

test("dashboard workRoot UI browser acceptance", async ({ page }) => {
  await page.context().grantPermissions(["clipboard-read", "clipboard-write"], {
    origin: daemon.baseUrl,
  });
  const terminalSocketUrls: string[] = [];
  const terminalSocketFrames: string[] = [];
  let terminalOutputPolls = 0;
  let resourceRefreshRequests = 0;
  page.on("websocket", (ws) => {
    if (
      ws.url().includes("/api/dashboard/terminals/") &&
      ws.url().includes("/socket")
    ) {
      terminalSocketUrls.push(ws.url());
      ws.on("framesent", (frame) =>
        terminalSocketFrames.push(String(frame.payload)),
      );
    }
  });
  page.on("request", (request) => {
    const url = request.url();
    if (url.includes("/api/dashboard/terminals/") && url.includes("/output")) {
      terminalOutputPolls += 1;
    }
    if (new URL(url).pathname === "/api/dashboard/resources") {
      resourceRefreshRequests += 1;
    }
  });
  // --- Owner pairing against the daemon-served production frontend ---------
  await test.step("owner pairing", async () => {
    await page.goto(daemon.pairingUrl, { waitUntil: "domcontentloaded" });
    await expect(page.locator(".app-shell")).toBeVisible();
    expect(new URL(page.url()).pathname).not.toContain("/pair");
    note("pairing: one-time pairing URL installed owner cookie and left /pair");
    ownerCookies = await page.context().cookies(daemon.baseUrl);
  });

  // --- Open a real workRoot through the raw path opener -------------------
  await test.step("open real workRoot", async () => {
    await openWorkRootInBrowser(page, workRoot);
    const resourceRows = page.locator(
      '.resource-row[data-command-id="resource.select"]',
    );
    await expect(resourceRows).toHaveCount(1);
    const compactRow = resourceRows.first();
    await expect(compactRow).toHaveAttribute(
      "data-resource-presentation",
      "compactWorkRoot",
    );
    await expect(compactRow).toHaveClass(/resource-row-selected/);
    await expect(compactRow).toContainText(workRootDisplayName(workRoot));
    await expect(compactRow.locator(".resource-row-icon-compact")).toBeVisible();
    await expect(compactRow.locator(".resource-row-icon svg")).toHaveCount(1);
    await expect(compactRow.locator(".state-badge")).toHaveCount(0);
    await expect(compactRow.locator(".meta-chip")).toHaveCount(0);
    await expect(compactRow).toHaveAttribute("title", /directory/);
    await expect(compactRow).toHaveAttribute("title", /availability: available/);
    await expect(compactRow).toHaveAttribute("title", /activation: online/);
    await expectContextSurfaceHierarchy(page);
    note(
      "open workRoot: live opened workRoot is selected, shown in the explorer, " +
        "and rendered as one compact workRoot nav row",
    );
    note(
      "visual hierarchy: nav, workbench topbar, Dockview group, tabbar, and pane body use distinct context surface/divider roles",
    );
  });

  // --- Settings modal open/close (260722 blocker-2: previously human-smoke ----
  // --- only, no automated coverage) ------------------------------------------
  await test.step("settings modal opens and closes", async () => {
    await page.locator('[data-command-id="settings.open"]').click();
    // Scope to role=dialog: the Settings nav button shares aria-label="Settings".
    const dialog = page.locator('[role="dialog"][aria-label="Settings"]');
    await expect(dialog).toBeVisible();
    // Section nav shell and its Terminal section entry both render.
    await expect(dialog.locator('[aria-label="Settings sections"]')).toBeVisible();
    await expect(
      dialog.locator(".settings-section-nav-button", { hasText: "Terminal" }),
    ).toBeVisible();

    await dialog.locator('[data-command-id="settings.close"]').click();
    await expect(
      page.locator('[role="dialog"][aria-label="Settings"]'),
    ).toHaveCount(0);
    note(
      "settings modal: Settings nav button opened the dialog with its section nav and Terminal section, and Close dismissed it",
    );
  });

  // --- Add-server modal open/validate/cancel (260722 blocker-2: previously ----
  // --- human-smoke only, no automated coverage) ------------------------------
  await test.step("add-server modal opens, validates, and cancels", async () => {
    await page.locator('[data-command-id="resource.action.server.add"]').click();
    // Scope to role=dialog: the Add server nav button shares aria-label="Add server".
    const dialog = page.locator('[role="dialog"][aria-label="Add server"]');
    await expect(dialog).toBeVisible();

    // Submit stays disabled until both name and endpoint are non-empty.
    const submit = dialog.locator(
      '[data-command-id="resource.action.server.link.submit"]',
    );
    await expect(submit).toBeDisabled();
    // Add-mode form renders Name, Endpoint, then Passphrase as `.root-picker-input`.
    const fields = dialog.locator(".root-picker-input");
    await fields.nth(0).fill("Gate remote");
    await fields.nth(1).fill("http://127.0.0.1:49999");
    await expect(submit).toBeEnabled();

    // Cancel without submitting to avoid a real server-link side effect.
    await dialog.getByRole("button", { name: "Cancel" }).click();
    await expect(
      page.locator('[role="dialog"][aria-label="Add server"]'),
    ).toHaveCount(0);
    note(
      "add-server modal: Add server nav button opened the dialog, submit stayed disabled until name+endpoint filled then enabled, and Cancel dismissed it without linking",
    );
  });



  await test.step("git workspace overflow adds linked worktree", async () => {
    if (!gitWorkRoot) {
      note("git worktree add: skipped because no daemon-host Git workRoot is configured");
      return;
    }
    await openWorkRootInBrowser(page, gitWorkRoot);
    const gitToolbar = page.locator(".git-toolbar");
    await expect(gitToolbar).toBeVisible();
    await expect(gitToolbar.locator('[data-command-id="git.branchMenu.open"]')).toBeVisible();
    await expect(gitToolbar.locator(".git-status-pill")).toContainText("clean");
    const fetchResponse = page.waitForResponse((response) =>
      response.url().includes("/git/fetch") && response.request().method() === "POST",
    );
    await gitToolbar.locator('[data-command-id="git.fetch"]').click();
    await expect((await fetchResponse).ok()).toBe(true);
    await expect(gitToolbar.locator('[data-command-id="git.branchMenu.open"]')).toBeVisible();
    await gitToolbar.locator('[data-command-id="git.branchMenu.open"]').click();
    await expect(page.locator(".git-branch-menu")).toBeVisible();
    await page.locator('.git-branch-menu [data-command-id="git.branchCreate.open"]').click();
    const branchModal = page.locator(".git-branch-modal");
    await expect(branchModal).toBeVisible();
    await expect(branchModal.locator("select.root-picker-input")).toBeVisible();
    await branchModal.locator('input[placeholder="feature-name"]').fill("browser-toolbar-branch");
    await branchModal.locator('[data-command-id="git.branchCreate.submit"]').click();
    await expect(branchModal).toHaveCount(0);
    await expect(gitToolbar.locator('[data-command-id="git.branchMenu.open"]')).toContainText("browser-toolbar-branch");
    await selectWorkRootInBrowser(page, workRoot);
    await expect(page.locator(".git-toolbar")).toHaveCount(0);
    await selectWorkRootInBrowser(page, gitWorkRoot);
    await expect(page.locator(".git-toolbar")).toBeVisible();
    const gitRow = page.locator(".resource-row", { hasText: workRootDisplayName(gitWorkRoot) }).first();
    await expect(gitRow).toBeVisible();
    const menuButton = gitRow.locator('[data-command-id="workspace.menu.open"]');
    await expect(menuButton).toBeVisible();
    await expect(gitRow.locator('[data-command-id="workspace.remove"]')).toHaveCount(0);
    await menuButton.click();
    const menu = page.locator(".workspace-row-menu");
    await expect(menu).toBeVisible();
    await expect(menu.locator('[data-command-id="workspace.remove"]')).toContainText("Remove workspace...");
    await menu.locator('[data-command-id="gitWorktreeAdd.open"]').click();
    const modal = page.locator(".git-worktree-modal");
    await expect(modal).toBeVisible();
    await modal.locator('input[placeholder="feature-name"]').fill("Browser Gate Branch");
    const preview = modal.locator(".git-worktree-preview");
    await expect(preview).toContainText("new branch will be created");
    await expect(preview).toHaveClass(/git-worktree-preview-willCreateBranch/);
    await modal.locator('[data-command-id="gitWorktreeAdd.submit"]').click();
    await expect(modal).toHaveCount(0);
    const createdRow = page.locator(".resource-row", { hasText: "Browser-Gate-Branch" }).first();
    await expect(createdRow).toBeVisible();
    await expect(createdRow).toHaveClass(/resource-row-selected/);
    await selectWorkRootInBrowser(page, gitWorkRoot);
    const gitPrimaryChildRow = page.locator('.resource-row[data-resource-presentation="workRoot"]', {
      hasText: workRootDisplayName(gitWorkRoot),
    });
    await expect(gitPrimaryChildRow).toHaveCount(0);
    const gitWorkspaceRow = page.locator('.resource-row[data-resource-presentation="workspace"].resource-row-selected').first();
    await expect(gitWorkspaceRow).toBeVisible();
    await selectWorkRootInBrowser(page, workRoot);
    note("git worktree add: workspace overflow preserved remove action, previewed new branch, submitted through daemon resources, and selected daemon-created workRoot id");
  });

  await test.step("git worktree removal modal (clean + dirty) and hide/unhide", async () => {
    if (!gitWorkRoot) {
      note("git worktree remove/hide: skipped because no daemon-host Git workRoot is configured");
      return;
    }
    await selectWorkRootInBrowser(page, gitWorkRoot);
    // The linked worktree created by the add step above.
    const childSelector =
      '.resource-row[data-resource-presentation="workRoot"]';
    const childRow = page
      .locator(childSelector, { hasText: "Browser-Gate-Branch" })
      .first();
    await expect(childRow).toBeVisible();

    // --- B-1 clean state: baseline confirmation copy, NO red data-loss banner. ---
    await childRow.locator('[data-command-id="worktree.menu.open"]').click();
    await childRow.locator('[data-command-id="worktreeRemove.open"]').click();
    const removeModal = page.locator(".git-worktree-modal");
    await expect(removeModal).toBeVisible();
    await expect(removeModal.locator(".git-worktree-remove-copy")).toBeVisible();
    // Clean worktree: the uncommitted-changes data-loss banner must be absent.
    await expect(
      removeModal.getByText("Uncommitted changes will be lost"),
    ).toHaveCount(0);
    await removeModal
      .locator('[data-command-id="worktreeRemove.close"]')
      .filter({ hasText: "Cancel" })
      .click();
    await expect(removeModal).toHaveCount(0);

    // --- B-1 dirty state: an untracked file surfaces the RED data-loss banner. ---
    const worktreeDir = path.join(
      gitWorkRoot,
      ".ws-dashboard",
      "worktrees",
      "Browser-Gate-Branch",
    );
    const scratchFile = path.join(worktreeDir, "scratch-uncommitted.txt");
    writeFileSync(scratchFile, "wip\n");
    await childRow.locator('[data-command-id="worktree.menu.open"]').click();
    await childRow.locator('[data-command-id="worktreeRemove.open"]').click();
    await expect(removeModal).toBeVisible();
    await expect(
      removeModal.getByText("Uncommitted changes will be lost"),
    ).toBeVisible();
    await removeModal
      .locator('[data-command-id="worktreeRemove.close"]')
      .filter({ hasText: "Cancel" })
      .click();
    await expect(removeModal).toHaveCount(0);
    // Leave the worktree on disk for the hide round-trip below.
    rmSync(scratchFile, { force: true });

    // --- B-3 hide → hidden-worktrees submenu → unhide (pure browser-local). ---
    await childRow.locator('[data-command-id="worktree.menu.open"]').click();
    await childRow.locator('[data-command-id="worktree.hide"]').click();
    await expect(
      page.locator(childSelector, { hasText: "Browser-Gate-Branch" }),
    ).toHaveCount(0);
    // Restore via the root workspace row "..." menu's hidden-worktrees section.
    const gitRow = page
      .locator(".resource-row", { hasText: workRootDisplayName(gitWorkRoot) })
      .first();
    await gitRow.locator('[data-command-id="workspace.menu.open"]').click();
    const hiddenSection = gitRow.locator(
      '[data-testid="hidden-worktrees-section"]',
    );
    await expect(hiddenSection).toBeVisible();
    await hiddenSection
      .locator('[data-command-id="worktree.unhide"]')
      .first()
      .click();
    await expect(
      page.locator(childSelector, { hasText: "Browser-Gate-Branch" }),
    ).toBeVisible();
    note(
      "git worktree remove/hide: clean modal hid the data-loss banner, an untracked file surfaced the red banner, and the hide -> hidden-worktrees-submenu -> unhide round trip restored the row",
    );
  });

  await test.step("activation controls are command-routed and update visible state", async () => {
    const metaRow = page.locator(".workbench-toolbar-meta");
    const activationButton = page.locator(
      '.workbench-power-button[data-command-id="workRoot.activation.set"]',
    );
    await expect(metaRow).toHaveAttribute("title", /availability: available/);
    await expect(metaRow).toHaveAttribute("title", /activation: online/);
    await expect(activationButton).toHaveAttribute("title", "Go offline");
    await expect(activationButton).toHaveCSS("background-color", "rgba(0, 0, 0, 0)");

    await activationButton.click();
    await expect(metaRow).toContainText("offline");
    await expect(
      page.locator('.resource-row[data-command-id="resource.select"]').first().locator(".meta-chip, .state-badge"),
    ).toHaveCount(0);
    await expect(page.locator(".workbench-toolbar")).toHaveAttribute(
      "data-last-command-id",
      "workRoot.activation.set",
    );
    await expect(activationButton).toHaveAttribute("title", "Go online");
    await expect(activationButton).toHaveCSS("background-color", "rgba(0, 0, 0, 0)");

    await activationButton.click();
    await expect(metaRow).not.toContainText("offline");
    await expect(activationButton).toHaveAttribute("title", "Go offline");
    note("activation controls dispatch through workRoot.activation.set and refresh visible state");
  });

  await test.step("topbar overflow keeps placeholder toggles command-routed", async () => {
    const more = page.getByRole("button", { name: "More workbench actions" });
    await expect(more).toBeVisible();
    await more.click();
    const menu = page.locator(".workbench-overflow-menu");
    await expect(menu).toBeVisible();
    for (const toggle of ["viewer", "task", "diagnostics", "events", "layout"]) {
      await expect(
        menu.locator(`[data-command-id="workbench.toggle.${toggle}"]`),
      ).toBeVisible();
    }
    await expect
      .poll(() =>
        menu.locator(".workbench-overflow-item span").evaluateAll((nodes) =>
          nodes.every((node) => node.scrollWidth <= node.clientWidth + 1),
        ),
      )
      .toBe(true);
    await page.screenshot({
      path: path.join(artifactsDir, "topbar-overflow.png"),
    });
    await page.keyboard.press("Escape");
    await more.click();
    note(
      "topbar overflow: low-value workbench toggles remain reachable behind the More icon with their command ids and visible labels do not clip",
    );
  });

  // --- Top-bar WorkRoot Activity badge sits in the existing metadata row --
  await test.step("activity badge renders in the toolbar metadata row without growing it", async () => {
    const metaRow = page.locator(".workbench-toolbar-meta");
    const badge = page.locator(
      ".workbench-toolbar-meta .workbench-activity-badge",
    );
    const stateBadge = page.locator(".workbench-toolbar-meta .state-badge");

    // CONTRACT: the badge is a single named-agent summary chip inside the
    // existing metadata row; it must not add a second badge or a new toolbar
    // row.
    await expect(badge).toHaveCount(1);
    await expect(badge).toBeVisible();
    await expect(badge).toContainText(/agent/i);

    const measureToolbar = async () => {
      const metaBox = await metaRow.boundingBox();
      const badgeBox = await badge.boundingBox();
      const stateBadgeBox = await stateBadge.boundingBox();
      const toolbarBox = await page
        .locator(".workbench-toolbar")
        .boundingBox();
      expect(metaBox).not.toBeNull();
      expect(badgeBox).not.toBeNull();
      expect(stateBadgeBox).not.toBeNull();
      expect(toolbarBox).not.toBeNull();
      return {
        metaBox: metaBox!,
        badgeBox: badgeBox!,
        stateBadgeBox: stateBadgeBox!,
        toolbarBox: toolbarBox!,
      };
    };
    const activitySummaryDisplay = async () =>
      page.evaluate(() => {
        const badgeNode = document.querySelector(".workbench-activity-badge");
        if (!badgeNode) return "missing";
        const probe = document.createElement("span");
        probe.className = "workbench-activity-badge-summary";
        probe.textContent = "probe";
        badgeNode.appendChild(probe);
        const display = window.getComputedStyle(probe).display;
        probe.remove();
        return display;
      });

    const assertSingleLineMetaRow = (
      measured: Awaited<ReturnType<typeof measureToolbar>>,
      label: string,
    ) => {
      // The metadata row stays one chip line tall. A wrapped row or a stacked
      // badge would make this a multiple of the chip height.
      expect(
        measured.metaBox.height,
        `${label}: metadata row stays a single chip line`,
      ).toBeLessThanOrEqual(measured.stateBadgeBox.height + 2);
      // The activity badge is vertically inside the metadata row, not a new
      // toolbar row above or below it.
      expect(
        measured.badgeBox.y,
        `${label}: badge starts inside the metadata row`,
      ).toBeGreaterThanOrEqual(measured.metaBox.y - 1);
      expect(
        measured.badgeBox.y + measured.badgeBox.height,
        `${label}: badge ends inside the metadata row`,
      ).toBeLessThanOrEqual(measured.metaBox.y + measured.metaBox.height + 1);
    };

    const wide = await measureToolbar();
    assertSingleLineMetaRow(wide, "1440px viewport");
    expect(await activitySummaryDisplay()).not.toBe("none");
    const wideToolbarHeight = wide.toolbarBox.height;

    // Constrained width: the badge compacts/clips instead of wrapping the
    // metadata row or adding a toolbar row.
    await page.setViewportSize({ width: 480, height: 900 });
    await expect(badge).toBeVisible();
    assertSingleLineMetaRow(await measureToolbar(), "480px viewport");
    expect(await activitySummaryDisplay()).toBe("none");

    await page.setViewportSize({ width: 1440, height: 900 });
    const restored = await measureToolbar();
    assertSingleLineMetaRow(restored, "restored 1440px viewport");
    expect(
      Math.abs(restored.toolbarBox.height - wideToolbarHeight),
      "toolbar height does not increase across the viewport change",
    ).toBeLessThanOrEqual(1);

    note(
      "activity badge: named-agent summary chip renders inside " +
        ".workbench-toolbar-meta; the metadata row stays a single chip line " +
        "at 1440px and 480px without adding a toolbar row",
    );
  });

  // --- Top-bar badge opens/focuses/closes the WorkRoot Activity pane -----
  // CONTRACT: The Activity badge opens or focuses exactly one WorkRoot Activity
  // pane in group 2, duplicate badge clicks do not create duplicate panes, the
  // pane closes immediately with no confirmation popover, and running-command
  // rows stay explicitly empty until the async exec source exists.
  await test.step("activity badge opens, focuses, and closes the WorkRoot Activity pane", async () => {
    const opener = page.locator(
      '[data-command-id="workbench.openActivity"].workbench-activity-badge',
    );
    const activityPane = page.locator(
      '[data-workbench-root-active="true"] [data-surface-kind="workRootActivity"]',
    );
    const activityTab = page.locator(
      '[data-workbench-root-active="true"] .dockview-workbench-tab[data-workbench-pane-id^="workRootActivity-pane:"]',
    );
    await expect(opener).toHaveCount(1);

    // No Activity pane exists until the badge is clicked.
    await expect(activityPane).toHaveCount(0);

    // Badge click opens exactly one Activity pane, and it lands in group 2.
    await opener.click();
    await expect(activityPane).toHaveCount(1);
    await expect(activityPane).toHaveAttribute(
      "data-workbench-group-id",
      "group-2",
    );
    await expect(activityPane).toHaveAttribute(
      "aria-label",
      "Activity: WorkRoot Activity",
    );
    await expectDockviewWorkbench(page);

    // The pane body is the read-only Activity Console projection. The
    // plain-directory gate workRoot has an empty source-neutral feed.
    const paneBody = activityPane.locator(".workroot-activity-pane");
    await expect(paneBody).toBeVisible();
    await expect(
      paneBody.locator('[data-activity-console-state="empty"]'),
    ).toBeVisible();

    // The Activity tab carries the surface title.
    await expect(activityTab).toHaveCount(1);
    await expect(activityTab.locator(".workbench-tab-title")).toHaveText(
      "WorkRoot Activity",
    );

    // A second badge click focuses the existing pane without duplicating it.
    await opener.click();
    await expect(activityPane).toHaveCount(1);
    await expect(activityTab).toHaveCount(1);

    // Close is immediate: the hover-only tab close removes the pane with no
    // cursor-near confirmation popover.
    await activityTab.hover();
    await activityTab
      .locator('[data-command-id="workbench.tab.close"]')
      .click();
    await expect(activityPane).toHaveCount(0);
    await expect(page.locator(".workbench-close-popover")).toHaveCount(0);

    activityFixtureRootId = await workRootIdForLabel(
      page,
      workRootDisplayName(workRoot),
    );
    expect(activityFixtureRootId).toBeTruthy();

    await page.route(
      /\/api\/dashboard\/work-roots\/.*\/activity(?:\?.*)?$/,
      async (route) => {
        const match = new URL(route.request().url()).pathname.match(
          /\/api\/dashboard\/work-roots\/([^/]+)\/activity$/,
        );
        const requestedWorkRootId = match
          ? decodeURIComponent(match[1])
          : "browser-gate-root";
        if (new URL(route.request().url()).searchParams.has("recentLimit")) {
          activityRecentPollRequests += 1;
        }
        if (requestedWorkRootId !== activityFixtureRootId) {
          await route.fulfill({
            status: 200,
            contentType: "application/json",
            body: JSON.stringify({
              workRootId: requestedWorkRootId,
              status: "ok",
              updateMode: "snapshot",
              feedCursor: "browser:second",
              selectedItemId: null,
              summary: {
                total: 0,
                active: 0,
                blocked: 0,
                failed: 0,
                unavailable: 0,
              },
              items: [],
              agents: [],
            }),
          });
          return;
        }
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            workRootId: requestedWorkRootId,
            status: "degraded",
            updateMode: "snapshot",
            feedCursor: "browser:1",
            selectedItemId: null,
            summary: {
              total: 5,
              active: 1,
              blocked: 1,
              failed: 0,
              unavailable: 0,
            },
            items: [
              {
                id: "agent:alpha",
                kind: "namedAgent",
                label: "agent-alpha",
                status: "running",
                live: true,
                attention: false,
                startedAt: "2026-05-17T11:57:00Z",
                updatedAt: "2026-05-17T11:58:00Z",
                finishedAt: null,
                source: {
                  kind: "namedAgent",
                  label: "claude",
                  backend: "claude",
                  harness: "codex",
                  tier: "core",
                  model: "opus",
                },
                transcript: {
                  status: "available",
                  available: true,
                  cursor: "alpha:1",
                },
                diagnostics: ["cache row degraded"],
                metadata: {},
              },
              {
                id: "exec:beta",
                kind: "exec",
                label: "exec-beta-long-label-that-must-truncate-inside-ribbon",
                status: "completed",
                live: false,
                attention: true,
                startedAt: "2026-05-17T11:55:00Z",
                updatedAt: "2026-05-17T11:59:00Z",
                finishedAt: "2026-05-17T11:59:00Z",
                source: {
                  kind: "exec",
                  label: "exec",
                  backend: null,
                  harness: null,
                  tier: null,
                  model: null,
                },
                transcript: {
                  status: "available",
                  available: true,
                  cursor: "beta:1",
                },
                diagnostics: [],
                metadata: {},
              },
              ...["gamma", "delta", "epsilon"].map((name, index) => ({
                id: `agent:${name}`,
                kind: "namedAgent",
                label: `agent-${name}-long-ribbon-label-${index}`,
                status: "completed",
                live: false,
                attention: false,
                startedAt: "2026-05-17T10:00:00Z",
                updatedAt: `2026-05-17T10:0${index}:00Z`,
                finishedAt: `2026-05-17T10:0${index}:30Z`,
                source: {
                  kind: "namedAgent",
                  label: "codex",
                  backend: "codex",
                  harness: "codex",
                  tier: "core",
                  model: null,
                },
                transcript: {
                  status: "empty",
                  available: false,
                  cursor: null,
                },
                diagnostics: [],
                metadata: {},
              })),
            ],
            agents: [
              {
                agentId: "agent-alpha",
                name: "agent-alpha",
                backend: "claude",
                harness: "codex",
                tier: "core",
                model: "opus",
                effort: "high",
                status: "running",
                lastCallAt: "2026-05-17T11:58:00Z",
                sessionPresent: true,
                currentCall: {
                  status: "running",
                  active: true,
                  terminal: false,
                  executionId: "exec-alpha",
                  startedAt: "2026-05-17T11:57:00Z",
                  updatedAt: "2026-05-17T11:58:00Z",
                  finishedAt: null,
                  cleanupNeeded: false,
                  error: "bounded diagnostic",
                },
                detailHints: ["review output ready"],
                diagnostics: ["cache row degraded"],
              },
            ],
          }),
        });
      },
    );
    await page.route(
      /\/api\/dashboard\/work-roots\/.*\/activity\/events(?:\?.*)?$/,
      async (route) => {
        const match = new URL(route.request().url()).pathname.match(
          /\/api\/dashboard\/work-roots\/([^/]+)\/activity\/events$/,
        );
        const requestedWorkRootId = match
          ? decodeURIComponent(match[1])
          : (activityFixtureRootId ?? "browser-gate-root");
        if (activityFixtureRootId && requestedWorkRootId !== activityFixtureRootId) {
          await route.fulfill({
            status: 200,
            contentType: "text/event-stream",
            body: `event: activity\ndata: ${JSON.stringify({
              type: "heartbeat",
              cursor: "browser:second:heartbeat",
            })}\n\n`,
            headers: { "cache-control": "no-cache" },
          });
          return;
        }
        const liveItem = {
          id: "agent:streamed-live",
          kind: "namedAgent",
          label: "agent-streamed-live",
          status: "running",
          live: true,
          attention: false,
          startedAt: "2026-05-17T12:00:00Z",
          updatedAt: "2026-05-17T12:01:00Z",
          finishedAt: null,
          source: {
            kind: "namedAgent",
            label: "codex",
            backend: "codex",
            harness: "codex",
            tier: "core",
            model: null,
          },
          transcript: {
            status: "empty",
            available: false,
            cursor: null,
          },
          diagnostics: [],
          metadata: {},
        };
        await route.fulfill({
          status: 200,
          contentType: "text/event-stream",
          body: [
            {
              type: "itemUpserted",
              cursor: "browser:stream:1",
              item: liveItem,
            },
            {
              type: "modeChanged",
              cursor: "browser:stream:2",
              updateMode: "watch",
            },
            {
              type: "heartbeat",
              cursor: "browser:stream:3",
            },
          ]
            .map((event) => `event: activity\ndata: ${JSON.stringify(event)}\n\n`)
            .join(""),
          headers: {
            "cache-control": "no-cache",
            "x-workroot-fixture": requestedWorkRootId,
          },
        });
      },
    );
    let alphaTranscriptReplaceRequests = 0;
    let showRefreshedAlphaTranscript = false;
    await page.route(
      /\/api\/dashboard\/work-roots\/.*\/activity\/items\/.*\/transcript(?:\?.*)?$/,
      async (route) => {
        const url = new URL(route.request().url());
        const pathMatch = url.pathname.match(
          /\/api\/dashboard\/work-roots\/([^/]+)\/activity\/items\/([^/]+)\/transcript$/,
        );
        const requestedWorkRootId = pathMatch
          ? decodeURIComponent(pathMatch[1])
          : "browser-gate-root";
        const activityId = pathMatch
          ? decodeURIComponent(pathMatch[2])
          : "agent:alpha";
        const cursor = url.searchParams.get("cursor");
        const before = url.searchParams.get("before");
        if (activityId === "agent:alpha" && !cursor && !before) {
          alphaTranscriptReplaceRequests += 1;
        }
        const source =
          activityId === "exec:beta"
            ? {
                kind: "exec",
                label: "exec",
                backend: null,
                harness: null,
                tier: null,
                model: null,
              }
            : {
                kind: "namedAgent",
                label: "claude",
                backend: "claude",
                harness: "codex",
                tier: "core",
                model: "opus",
              };
        const alphaInitialBlocks = [
          {
            cursor: "alpha:1",
            timestamp: "2026-05-17T11:58:00Z",
            renderKind: "markdown",
            title: "assistant",
            text: "selected transcript alpha",
            data: null,
            degraded: false,
          },
          {
            cursor: "alpha:2",
            timestamp: "2026-05-17T11:58:10Z",
            renderKind: "json",
            title: "tool call",
            text: "tool details visible after expansion",
            data: { tool: "read" },
            degraded: false,
          },
          ...Array.from({ length: 28 }, (_, index) => ({
            cursor: `alpha:filler:${index}`,
            timestamp: "2026-05-17T11:58:20Z",
            renderKind: "markdown",
            title: `tail filler ${index}`,
            text: `tail-follow filler line ${index}`,
            data: null,
            degraded: false,
          })),
        ];
        const refreshedAlphaBlocks = [
          ...alphaInitialBlocks,
          {
            cursor: "alpha:refresh-applied",
            timestamp: "2026-05-17T11:59:40Z",
            renderKind: "markdown",
            title: "refresh applied",
            text: "selected transcript refresh applied",
            data: null,
            degraded: false,
          },
        ];
        const blocks =
          activityId === "exec:beta"
            ? [
                {
                  cursor: "beta:1",
                  timestamp: "2026-05-17T11:59:00Z",
                  renderKind: "text",
                  title: "exec output",
                  text: "$ echo browser-gate\nbrowser-gate",
                  data: null,
                  degraded: false,
                },
              ]
            : cursor
              ? [
                  {
                    cursor: "alpha:3",
                    timestamp: "2026-05-17T11:59:30Z",
                    renderKind: "markdown",
                    title: "assistant follow-up",
                    text: "loaded more transcript",
                    data: null,
                    degraded: false,
                  },
                ]
              : before
              ? [
                  {
                    cursor: "alpha:0",
                    timestamp: "2026-05-17T11:59:30Z",
                    renderKind: "markdown",
                    title: "assistant follow-up",
                    text: "loaded more transcript",
                    data: null,
                    degraded: false,
                  },
                ]
              : showRefreshedAlphaTranscript
                ? refreshedAlphaBlocks
                : alphaInitialBlocks;
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            workRootId: requestedWorkRootId,
            activityId,
            status: "available",
            sourceStatus: "ok",
            live: activityId === "agent:alpha",
            source,
            blocks,
            nextCursor:
              activityId === "agent:alpha" && !cursor && !before ? "alpha:2" : null,
            hasMore: activityId === "agent:alpha" && !cursor && !before,
            diagnostics: [],
          }),
        });
      },
    );
    await page.reload({ waitUntil: "domcontentloaded" });
    await openWorkRootInBrowser(page, workRoot);
    await expect(opener).toContainText("5 agents");

    await opener.click();
    await expect(activityPane).toHaveCount(1);
    await expect(activityPane).toHaveAttribute(
      "data-workbench-group-id",
      "group-2",
    );
    await expect(activityTab).toHaveCount(1);
    await expect(activityTab).toHaveAttribute("aria-selected", "true");

    const populatedBody = activityPane.locator(".workroot-activity-pane");
    await expect(populatedBody.locator(".activity-console")).toBeVisible();
    await expect(populatedBody.locator(".activity-ribbon")).toBeVisible();
    await expect(populatedBody.locator(".activity-console-summary")).toHaveCount(0);
    const ribbonItems = populatedBody.locator(".activity-ribbon-item");
    await expect(ribbonItems).toHaveCount(6);
    await expect(
      populatedBody
        .locator('[data-activity-id="agent:alpha"]')
        .locator(".activity-ribbon-meta"),
    ).toHaveText("agent.claude");
    await expect(
      populatedBody
        .locator('[data-activity-id="exec:beta"]')
        .locator(".activity-ribbon-meta"),
    ).toHaveText("cmd.exec");
    await expect(populatedBody).toContainText("agent-streamed-live");
    await expect
      .poll(() => activityRecentPollRequests, { timeout: 500 })
      .toBe(0);
    await expect(ribbonItems.first()).toHaveCSS("min-width", "176px");
    await expect(populatedBody.locator(".activity-ribbon")).toHaveJSProperty(
      "scrollLeft",
      0,
    );
    await expect(
      populatedBody.locator('[data-command-id="activity.selectItem"]'),
    ).toHaveCount(6);
    await expect(
      populatedBody.locator('[data-command-id="activity.refresh"]'),
    ).toHaveCount(1);
    await page.setViewportSize({ width: 760, height: 760 });
    await page.waitForTimeout(100);
    const ribbonMetrics = await populatedBody
      .locator(".activity-ribbon")
      .evaluate((node) => {
        return {
          clientWidth: node.clientWidth,
          scrollWidth: node.scrollWidth,
          childCount: node.children.length,
          childWidths: Array.from(node.children).map(
            (child) => (child as HTMLElement).getBoundingClientRect().width,
          ),
        };
      });
    expect(ribbonMetrics.scrollWidth, JSON.stringify(ribbonMetrics)).toBeGreaterThan(
      ribbonMetrics.clientWidth,
    );
    const itemBox = await ribbonItems.first().boundingBox();
    expect(itemBox?.width).toBeLessThanOrEqual(180);
    const documentHorizontalOverflow = await page.evaluate(() => {
      const root = document.scrollingElement ?? document.documentElement;
      return root.scrollWidth > root.clientWidth + 1;
    });
    expect(documentHorizontalOverflow).toBe(false);
    await page.setViewportSize({ width: 1440, height: 900 });
    await page.waitForTimeout(100);
    await expect(populatedBody).toContainText("$ echo browser-gate");
    await expect(
      populatedBody.locator('[data-block-mode="terminal"]'),
    ).toBeVisible();
    const alphaItem = populatedBody.locator('[data-activity-id="agent:alpha"]');
    await expect(alphaItem).toHaveAttribute("data-dirty", "true");
    await alphaItem.click();
    await expect(alphaItem).toHaveAttribute("data-dirty", "false");
    await expect(page.locator(".workbench-toolbar")).toHaveAttribute(
      "data-last-command-id",
      "activity.selectItem",
    );
    await expect(populatedBody).toContainText("selected transcript alpha");
    const transcriptScroll = populatedBody.locator(".activity-transcript-scroll");
    await expect
      .poll(() =>
        transcriptScroll.evaluate(
          (node) => node.scrollHeight > node.clientHeight + 1,
        ),
      )
      .toBe(true);
    await expect
      .poll(() =>
        transcriptScroll.evaluate(
          (node) =>
            node.scrollHeight - (node.scrollTop + node.clientHeight) <= 8,
        ),
      )
      .toBe(true);
    await transcriptScroll.hover();
    await page.mouse.wheel(0, -600);
    await expect
      .poll(() =>
        transcriptScroll.evaluate(
          (node) => node.scrollHeight - (node.scrollTop + node.clientHeight),
        ),
      )
      .toBeGreaterThan(100);
    await expect(transcriptScroll).toHaveAttribute("data-following-tail", "false");
    const refreshBeforeScrollTop = await transcriptScroll.evaluate((node) => {
      (node as HTMLElement & { __wsStableScroll?: boolean }).__wsStableScroll = true;
      return node.scrollTop;
    });
    const alphaReplaceRequestsBeforeRefresh = alphaTranscriptReplaceRequests;
    showRefreshedAlphaTranscript = true;
    await populatedBody
      .locator('.activity-transcript-head [data-command-id="activity.refresh"]')
      .click();
    await expect(page.locator(".workbench-toolbar")).toHaveAttribute(
      "data-last-command-id",
      "activity.refresh",
    );
    await expect
      .poll(() => alphaTranscriptReplaceRequests)
      .toBeGreaterThan(alphaReplaceRequestsBeforeRefresh);
    await expect(populatedBody).toContainText("selected transcript refresh applied");
    await expect
      .poll(() =>
        transcriptScroll.evaluate((node) =>
          Boolean(
            (node as HTMLElement & { __wsStableScroll?: boolean }).__wsStableScroll,
          ),
        ),
      )
      .toBe(true);
    await expect
      .poll(() =>
        transcriptScroll.evaluate(
          (node, expected) => Math.abs(node.scrollTop - expected),
          refreshBeforeScrollTop,
        ),
      )
      .toBeLessThanOrEqual(2);
    await transcriptScroll.evaluate((node) => {
      node.scrollTop = node.scrollHeight;
    });
    const detailToggle = populatedBody.locator(
      '[data-command-id="activity.detail.toggle"]',
    );
    await expect(detailToggle).toBeVisible();
    await detailToggle.click();
    await expect(page.locator(".workbench-toolbar")).toHaveAttribute(
      "data-last-command-id",
      "activity.detail.toggle",
    );
    await expect(populatedBody).toContainText("tool details visible after expansion");
    const loadMore = populatedBody.locator(
      '[data-command-id="activity.transcript.loadMore"]',
    );
    await expect(loadMore).toBeVisible();
    await loadMore.click();
    await expect(page.locator(".workbench-toolbar")).toHaveAttribute(
      "data-last-command-id",
      "activity.transcript.loadMore",
    );
    await expect(populatedBody).toContainText("loaded more transcript");
    const execItem = populatedBody.locator('[data-activity-id="exec:beta"]');
    await execItem.click();
    await expect(populatedBody).toContainText("$ echo browser-gate");
    await expect(
      populatedBody.locator('[data-block-mode="terminal"]'),
    ).toBeVisible();

    await opener.click();
    await expect(activityPane).toHaveCount(1);
    await expect(activityTab).toHaveCount(1);
    await expect(activityTab).toHaveAttribute("aria-selected", "true");

    await activityTab.hover();
    await activityTab
      .locator('[data-command-id="workbench.tab.close"]')
      .click();
    await expect(activityPane).toHaveCount(0);
    await expect(page.locator(".workbench-close-popover")).toHaveCount(0);

    note(
      "activity pane: badge click opened one WorkRoot Activity pane in group 2, " +
        "empty and populated Activity Console projections rendered, named live stream " +
        "upsert appeared without reload and without healthy-mode recent polling, " +
        "selection, dirty acknowledgement, " +
        "detail toggle, and load-more controls carried " +
        "stable command ids, a second click focused it without duplicating, " +
        "and hover-only close removed it " +
        "immediately with no confirmation popover",
    );
  });

  // --- Long explorer content stays inside its pane, not the document -----
  await test.step("long explorer content stays within the viewport", async () => {
    // The fixture root holds 80+ files, so the explorer tree is far taller
    // than its pane on the default 1440x900 viewport.
    await expect(
      page.locator(".file-explorer-row", { hasText: "gate-bulk-000.txt" }),
    ).toBeVisible();

    const viewport = page.viewportSize();
    expect(viewport).not.toBeNull();

    // The document itself must not scroll: the explorer body owns the overflow.
    expect(await documentScrolls(page)).toBe(false);

    // The app shell stays inside the viewport so the footer is never pushed
    // off-screen by an expanded tree.
    const shellBox = await page.locator(".app-shell").boundingBox();
    expect(shellBox).not.toBeNull();
    expect(shellBox!.height).toBeLessThanOrEqual(viewport!.height + 1);

    // The explorer body is the scroll container that absorbs the long tree.
    const explorerScrolls = await page.evaluate(() => {
      const body = document.querySelector(".file-explorer-body");
      return body ? body.scrollHeight > body.clientHeight + 1 : false;
    });
    expect(explorerScrolls).toBe(true);

    note(
      "explorer containment: 80+ file rows scroll inside .file-explorer-body; " +
        "the document does not scroll and .app-shell stays within the viewport",
    );
  });

  // --- No mock/placeholder terminal in the freshly opened workbench -------
  await test.step("non-mock initial workbench state", async () => {
    await expect(terminalTabs(page)).toHaveCount(0);
    await expect(page.locator(".terminal-surface")).toHaveCount(0);
    note(
      "initial state: opened workRoot shows no mock or placeholder terminal surface",
    );
  });

  // --- Agent tab close uses the same session confirmation contract ---------
  // suspended: agent GUI (260713)
  if (!AGENT_GUI_SUSPENDED) {
  await test.step("agent tab close confirmation when a live agent tab exists", async () => {
    const agentTab = page.locator(
      '[data-workbench-root-active="true"] .dockview-workbench-tab[data-workbench-close-confirmation="confirmSessionClose"]',
      { hasText: "Agent" },
    );
    if (daemon.mode === "external" && (await agentTab.count()) === 0) {
      note(
        "agent close: skipped because the external daemon exposed no live main agent tab",
      );
      return;
    }
    await expect(agentTab).toHaveCount(1);

    await agentTab.first().hover();
    await agentTab
      .first()
      .locator('[data-command-id="workbench.tab.close"]')
      .click();
    const popover = page.locator('[data-workbench-close-popover="cursor-near"]');
    await expect(popover).toBeVisible();
    await popover
      .locator('[data-command-id="workbench.tab.close.cancel"]')
      .click();
    await expect(agentTab).toHaveCount(1);

    await agentTab.first().hover();
    await agentTab
      .first()
      .locator('[data-command-id="workbench.tab.close"]')
      .click();
    await expect(popover).toBeVisible();
    await popover
      .locator('[data-command-id="workbench.tab.close.confirm"]')
      .click();
    await expect(agentTab).toHaveCount(0);
    note(
      "agent close: cursor-near No preserved the agent tab and Yes detached the live agent surface",
    );
  });
  }

  // --- Conventional read-only file explorer -------------------------------
  await test.step("file explorer expansion and refresh", async () => {
    const dirRow = page.locator(".file-explorer-row", {
      hasText: "gate-subdir",
    });
    await expect(dirRow).toBeVisible();
    await expect(dirRow).toHaveClass(/file-explorer-row-directory/);
    await expect(dirRow).toHaveAttribute(
      "data-command-id",
      "fileExplorer.toggleDirectory",
    );
    await expect(dirRow).toHaveAttribute("aria-expanded", "false");

    await dirRow.click();
    await expect(dirRow).toHaveAttribute("aria-expanded", "true");
    await expect(
      page.locator(".file-explorer-row", { hasText: "nested.txt" }),
    ).toBeVisible();

    await page.locator('[data-command-id="fileExplorer.refresh"]').click();
    await expect(
      page.locator(".file-explorer-row", { hasText: "gate-readme.txt" }),
    ).toBeVisible();
    await page.screenshot({
      path: path.join(artifactsDir, "file-explorer.png"),
    });
    note(
      "file explorer: directory rows expand on row click and refresh keeps entries visible",
    );
  });

  // --- Open a previewable read-only file ----------------------------------
  await test.step("preview, close, and pin read-only file tabs", async () => {
    const fileRow = page.locator(".file-explorer-row", {
      hasText: "gate-readme.txt",
    });
    await expect(fileRow).toHaveAttribute(
      "data-command-id",
      "fileExplorer.openFile",
    );
    await fileRow.click();

    const pane = page.locator('[data-workbench-root-active="true"] .readonly-text-pane');
    await expect(pane).toBeVisible();
    const sourceViewer = pane.locator('.document-source-viewer[data-editor-read-only="true"]');
    await expect(sourceViewer).toBeVisible();
    await expect(sourceViewer).toHaveAttribute("data-editor-language", "text");
    await expect(sourceViewer.locator(".cm-lineNumbers")).toBeVisible();
    await expect(sourceViewer.locator(".cm-content")).toContainText(
      "ws-dashboard browser gate fixture",
    );
    await expectDockviewWorkbench(page);
    await expect(pane.locator(".readonly-text-pane-badges")).toContainText(
      "preview",
    );
    const previewTab = page.locator(
      '[data-workbench-root-active="true"] .dockview-workbench-tab[data-workbench-pane-id^="readonly-preview:"]',
    );
    await expect(previewTab).toBeVisible();
    await expect(previewTab).toHaveAttribute(
      "data-workbench-tab-close-affordance",
      "hover-only",
    );
    await expect(previewTab).toHaveAttribute(
      "data-workbench-tab-category-presentation",
      /dockview-category-chip|pinned-left-badge-fallback/,
    );
    await expect
      .poll(() =>
        previewTab
          .locator(".workbench-tab-title")
          .evaluate((node) => getComputedStyle(node).fontStyle),
      )
      .toBe("italic");
    await expect
      .poll(() =>
        previewTab
          .locator("xpath=ancestor::*[contains(@class, 'dv-tab')][1]")
          .evaluate((node) => getComputedStyle(node).boxShadow),
      )
      .toBe("none");
    const previewClose = previewTab.locator(
      '[data-command-id="workbench.tab.close"]',
    );
    await expect
      .poll(() =>
        previewClose.evaluate((node) => getComputedStyle(node).opacity),
      )
      .toBe("0");
    await previewTab.hover();
    await expect
      .poll(() =>
        previewClose.evaluate((node) => getComputedStyle(node).opacity),
      )
      .toBe("1");

    const replacementRow = page.locator(".file-explorer-row", {
      hasText: "gate-bulk-000.txt",
    });
    await replacementRow.click();
    await expect(previewTab).toHaveCount(1);
    await expect(pane.locator(".readonly-text-pane-path")).toContainText(
      "gate-bulk-000.txt",
    );
    await expect(sourceViewer).toHaveAttribute("data-editor-language", "text");
    await expect(sourceViewer.locator(".cm-content")).toContainText(
      "bulk gate fixture 0",
    );

    await previewTab.hover();
    await previewClose.click();
    await expect(
      page.locator('[data-workbench-root-active="true"] .readonly-text-pane'),
    ).toHaveCount(0);

    await fileRow.click();
    await expect(previewTab).toBeVisible();
    await fileRow.dblclick();
    const pinnedTab = page.locator(
      '[data-workbench-root-active="true"] .dockview-workbench-tab[data-workbench-pane-id^="readonly:"]',
    );
    await expect(pinnedTab).toBeVisible();
    await expect(previewTab).toHaveCount(0);
    await expect(pane.locator(".readonly-text-pane-badges")).toContainText(
      "pinned",
    );
    await expect(page.locator(".workbench-pane-header")).toHaveCount(0);
    await expect(page.locator(".workbench-pane-status")).toHaveCount(0);
    await expect(pinnedTab).toHaveAttribute(
      "data-workbench-group-id",
      "group-2",
    );
    await expect(pinnedTab).toHaveAttribute(
      "data-workbench-pane-category",
      "pinned",
    );
    await expect(pinnedTab).toHaveAttribute(
      "data-workbench-tab-category-presentation",
      "pinned-left-badge-fallback",
    );
    await expect
      .poll(() =>
        pinnedTab
          .locator(".workbench-tab-title")
          .evaluate((node) => getComputedStyle(node).fontStyle),
      )
      .toBe("normal");
    note(
      "read-only file: single click opened a replaceable preview, hover-only close immediately removed it, and double click pinned the file in the opened file group",
    );
  });


  await test.step("source CodeMirror viewer renders TOML, fallback text, quiet focus, and edit save", async () => {
    const tomlRow = page.locator(".file-explorer-row", {
      hasText: "gate-config.toml",
    });
    if ((await tomlRow.count()) === 0) {
      note(
        "source CodeMirror viewer: skipped because external daemon workRoot did not provide gate-config.toml",
      );
      return;
    }

    await tomlRow.click();
    const pane = page.locator('[data-workbench-root-active="true"] .document-pane');
    const sourceViewer = pane.locator('.document-source-viewer[data-editor-read-only="true"]');
    await expect(sourceViewer).toBeVisible();
    await expect(sourceViewer).toHaveAttribute("data-editor-language", "toml");
    await expect(sourceViewer.locator(".cm-lineNumbers")).toBeVisible();
    await expect(pane.locator('[data-document-block-kind="heading"]')).toHaveCount(0);
    await expect(sourceViewer.locator(".cm-content")).toContainText("CodeMirror source viewer");
    await sourceViewer.locator(".cm-content").click();
    await expect(sourceViewer.locator(".cm-content")).toBeFocused();
    await expect
      .poll(() =>
        sourceViewer.locator(".cm-editor").evaluate((node) => {
          return getComputedStyle(node).outlineStyle;
        }),
      )
      .toBe("none");
    const readScroller = sourceViewer.locator(".cm-scroller");
    await readScroller.evaluate((node) => {
      node.scrollTop = node.scrollHeight;
    });
    await expect.poll(() => readScroller.evaluate((node) => node.scrollTop > 0)).toBe(true);

    await pane.locator('[data-command-id="document.mode.set"][data-document-mode="edit"]').click();
    const sourceEditor = pane.locator('.document-raw-editor[data-editor-read-only="false"]');
    await expect(sourceEditor).toBeVisible();
    await expect(sourceEditor).toHaveAttribute("data-editor-language", "toml");
    await expect(pane.locator('[data-command-id="document.save"]')).toBeDisabled();
    await sourceEditor.locator(".cm-content").click();
    await page.keyboard.press(process.platform === "darwin" ? "Meta+A" : "Control+A");
    await page.keyboard.insertText(
      [
        "title = \"CodeMirror source viewer edited\"",
        "",
        "[owner]",
        "name = \"dashboard\"",
        "enabled = true",
      ].join("\n") + "\n",
    );
    await pane.locator('[data-command-id="document.save"]').click();
    await expect(pane.locator('[data-document-save-state="saved"]')).toContainText(/saved/i);
    await pane.locator('[data-command-id="document.mode.set"][data-document-mode="view"]').click();
    await expect(sourceViewer).toHaveAttribute("data-editor-language", "toml");
    await expect(sourceViewer.locator(".cm-content")).toContainText("source viewer edited");
    expect(readFileSync(path.join(workRoot, "gate-config.toml"), "utf8")).toContain(
      "CodeMirror source viewer edited",
    );

    const fallbackRow = page.locator(".file-explorer-row", {
      hasText: "gate-unknown.workflowx",
    });
    await fallbackRow.click();
    await expect(sourceViewer).toHaveAttribute("data-editor-language", "text");
    await expect(sourceViewer.locator(".cm-content")).toContainText("plain fallback source viewer fixture");
    note(
      "source CodeMirror viewer: TOML read view used read-only CodeMirror with quiet focus chrome and line numbers, edit mode saved through CodeMirror, and unknown source fell back to text",
    );
  });
  await test.step("markdown document viewer renders structured blocks and pathrefs", async () => {
    const markdownRow = page.locator(".file-explorer-row", {
      hasText: "gate-document.md",
    });
    if ((await markdownRow.count()) === 0) {
      note(
        "markdown document viewer: skipped because external daemon workRoot did not provide gate-document.md",
      );
      return;
    }

    await markdownRow.click();
    const pane = page.locator('[data-workbench-root-active="true"] .document-pane');
    await expect(pane).toBeVisible();
    const previewTab = page.locator(
      '[data-workbench-root-active="true"] .dockview-workbench-tab[data-workbench-pane-id^="readonly-preview:"]',
    );
    await expect(previewTab).toBeVisible();
    const previewTabIdBeforeEdit = await previewTab.getAttribute("data-workbench-pane-id");
    if (!previewTabIdBeforeEdit) {
      throw new Error("document preview tab id missing before edit");
    }
    const previewTabCountBeforeEdit = await previewTab.count();
    await expect(pane.locator('.document-viewer-segment.is-active[data-document-mode="view"]')).toBeVisible();
    await expect(pane.locator('[data-command-id="document.mode.set"][data-document-mode="edit"]')).toBeEnabled();
    await expect(pane.locator('.document-source-viewer[data-editor-read-only="true"]')).toHaveCount(0);
    await expect(pane.locator('[data-document-block-kind="heading"]')).toContainText("Gate Document");
    await expect(pane.locator('[data-document-block-kind="taskItem"] input[type="checkbox"]')).toBeChecked();
    const nestedUnorderedList = pane.locator(".document-list-unordered .document-list-unordered");
    await expect(nestedUnorderedList).toHaveCount(1);
    await expect(nestedUnorderedList).toContainText("nested item");
    await expect(pane.locator(".document-list-ordered")).toHaveAttribute("start", "5");
    await expect(pane.locator("code")).toContainText("inline code");
    await expect(pane.locator(".document-callout-note")).toContainText("Browser note");
    await expect(pane.locator("table")).toContainText("rendered");
    await pane.locator(".document-translation-toggle").click();
    await expect(pane.locator(".document-translation-status")).toContainText(
      /No translation provider configured|Translation partial|Translated to/,
    );

    if (ownsWorkRoot) {
      await pane.locator('[data-command-id="document.mode.set"][data-document-mode="edit"]').click();
      const editor = pane.locator(".document-raw-editor");
      await expect(editor).toBeVisible();
      await expect(editor).toHaveAttribute("data-editor-language", "markdown");
      await expect(editor.locator(".cm-lineNumbers")).toBeVisible();
      const editorContent = editor.locator(".cm-content");
      await editorContent.click();
      await expect(editorContent).toBeFocused();
      await page.keyboard.press(process.platform === "darwin" ? "Meta+A" : "Control+A");
      await page.keyboard.insertText(
        [
          "# Gate Document Edited",
          "",
          "Markdown paragraph line with `inline code`",
          "with soft continuation",
          "",
          "- [x] completed task",
          "- parent item",
          "  - nested item",
          "",
          "5. ordered fifth",
          "6. ordered sixth",
          "",
          "> [!note] Browser note",
          "> callout body",
          "",
          "| Kind | Value |",
          "| --- | --- |",
          "| table | rendered |",
          "",
          ...Array.from({ length: 80 }, (_, index) => `editor internal scroll line ${index + 1}`),
        ].join("\n") + "\n",
      );
      const scroller = editor.locator(".cm-scroller");
      await scroller.evaluate((node) => {
        node.scrollTop = node.scrollHeight;
      });
      await expect.poll(() => scroller.evaluate((node) => node.scrollTop > 0)).toBe(true);
      await pane.locator('[data-command-id="document.save"]').click();
      await expect(pane.locator('[data-document-save-state="saved"]')).toContainText(/saved/i);
      await pane.locator('[data-command-id="document.mode.set"][data-document-mode="view"]').click();
      await expect(pane.locator('[data-document-block-kind="heading"]')).toContainText(
        "Gate Document Edited",
      );
      await expect(previewTab).toHaveCount(previewTabCountBeforeEdit);
      await expect(previewTab).toHaveAttribute("data-workbench-pane-id", previewTabIdBeforeEdit);
      expect(readFileSync(path.join(workRoot, "gate-document.md"), "utf8")).toContain(
        "# Gate Document Edited",
      );
    }

    const paragraphBlock = pane.locator('[data-document-block-kind="paragraph"]').first();
    const taskBlock = pane.locator('[data-document-block-kind="taskItem"]').first();
    await paragraphBlock.click();
    await expect(pane.locator(".document-viewer-action-strip")).toHaveCount(0);
    await expect(pane.locator(".document-selected-toolbar")).toHaveCount(0);
    await expect(paragraphBlock).not.toHaveClass(/is-selected/);
    await paragraphBlock.locator(".document-block-rail-select").click();
    await taskBlock.locator(".document-block-rail-select").click({ modifiers: ["Shift"] });
    await expect(paragraphBlock).toHaveClass(/is-selected/);
    await expect(taskBlock).toHaveClass(/is-selected/);
    const selectedToolbar = pane.locator(".document-selected-toolbar");
    await expect(selectedToolbar).toContainText("2 blocks selected");
    await expect(paragraphBlock.locator(".document-block-rail-actions")).toHaveCount(0);
    await selectedToolbar.locator('button[aria-label="Copy selected pathrefs"]').click();
    await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe(
      ["@gate-document.md#L3-L4", "@gate-document.md#L6"].join("\n"),
    );

    await expectDockviewWorkbench(page);
    await expect(previewTab).toBeVisible();
    await markdownRow.dblclick();
    const markdownPinnedTab = page.locator(
      '[data-workbench-root-active="true"] .dockview-workbench-tab[data-workbench-pane-id^="readonly:"][title="gate-document.md"]',
    );
    await expect(markdownPinnedTab).toBeVisible();
    await markdownPinnedTab.hover();
    await markdownPinnedTab.locator('[data-command-id="workbench.tab.close"]').click();
    await expect(markdownPinnedTab).toHaveCount(0);
    note(
      "markdown document viewer: daemon-served markdown file rendered heading, task, callout, table, raw edit/save, semantic list/code rendering, rail actions, and relative pathref copy while preserving preview-to-pinned tabs",
    );
  });

  await test.step("long read-only file scroll stays inside the pane", async () => {
    // CONTRACT: Long read-only source content must own its CodeMirror scroll
    // container without moving the top-level browser document or displacing
    // dashboard chrome.
    const longFileRow = page.locator(".file-explorer-row", {
      hasText: "gate-long-readonly.txt",
    });
    await longFileRow.click();
    await expectDockviewWorkbench(page);

    const viewer = page.locator(
      '[data-workbench-root-active="true"] .document-source-viewer[data-editor-read-only="true"]',
    );
    await expect(viewer).toHaveAttribute("data-editor-language", "text");
    await expect(viewer.locator(".cm-content")).toContainText(
      "readonly scroll containment line 1",
    );
    const content = viewer.locator(".cm-scroller");
    const scrollBox = await content.boundingBox();
    expect(scrollBox).not.toBeNull();
    await expect
      .poll(() =>
        content.evaluate((node) => node.scrollHeight > node.clientHeight),
      )
      .toBe(true);

    const beforeDocumentScroll = await documentScrolls(page);
    await content.hover();
    await page.mouse.wheel(0, 900);
    await expect
      .poll(() => content.evaluate((node) => node.scrollTop), {
        timeout: 3_000,
      })
      .toBeGreaterThan(0);
    await content.evaluate((node) => {
      node.scrollTop = node.scrollHeight;
    });
    await expect(viewer.locator(".cm-content")).toContainText(
      "readonly scroll containment line 220",
    );
    const scrollTopBeforeRefresh = await content.evaluate((node) => node.scrollTop);
    await page.locator('[data-command-id="fileExplorer.refresh"]').click();
    await expect(page.locator(".workbench-toolbar")).toHaveAttribute(
      "data-last-command-id",
      "fileExplorer.refresh",
    );
    await settlePastPollCycle(page);
    await expect
      .poll(() =>
        content.evaluate(
          (node, expected) => Math.abs(node.scrollTop - expected),
          scrollTopBeforeRefresh,
        ),
      )
      .toBeLessThanOrEqual(2);
    expect(await documentScrolls(page)).toBe(beforeDocumentScroll);
    expect(beforeDocumentScroll).toBe(false);
    note(
      "read-only file: long file scroll stayed inside the pane and survived a split workbench refresh without creating top-level document scroll",
    );
  });

  // --- Dynamic workbench state remains per workRoot ------------------------
  await test.step("dynamic split state is isolated per opened workRoot", async () => {
    if (!secondWorkRoot) {
      note(
        "dynamic groups: second workRoot isolation skipped because external daemon mode did not provide WS_DASHBOARD_TEST_SECOND_WORKROOT",
      );
      return;
    }
    await openWorkRootInBrowser(page, secondWorkRoot);
    expect(await visibleWorkbenchGroupIds(page)).toEqual(["group-1"]);
    await page
      .locator('[data-command-id="workbench.openActivity"].workbench-activity-badge')
      .click();
    const secondActivityPane = page.locator(
      '[data-workbench-root-active="true"] [data-surface-kind="workRootActivity"]',
    );
    await expect(secondActivityPane).toHaveCount(1);
    await expect(
      secondActivityPane.locator('[data-activity-console-state="empty"]'),
    ).toBeVisible();
    await expect(secondActivityPane).not.toContainText("agent-alpha");
    await expect(secondActivityPane).not.toContainText("exec-beta");
    await expect(secondActivityPane).not.toContainText("selected transcript alpha");
    await expect(secondActivityPane).not.toContainText("$ echo browser-gate");
    const secondActivityTab = page.locator(
      '[data-workbench-root-active="true"] .dockview-workbench-tab[data-workbench-pane-id^="workRootActivity-pane:"]',
    );
    await secondActivityTab.hover();
    await secondActivityTab
      .locator('[data-command-id="workbench.tab.close"]')
      .click();
    await expect(secondActivityPane).toHaveCount(0);

    const secondFileRow = page.locator(".file-explorer-row", {
      hasText: "second-readme.txt",
    });
    await expect(secondFileRow).toHaveAttribute(
      "data-command-id",
      "fileExplorer.openFile",
    );
    await secondFileRow.click();
    await expect(
      page.locator('[data-workbench-root-active="true"] .readonly-text-pane'),
    ).toContainText(
      "second ws-dashboard browser gate fixture",
    );
    await expect(
      page.locator(
        '[data-workbench-root-active="true"] .dockview-workbench-tab[data-workbench-pane-id^="readonly-preview:"]',
      ),
    ).toHaveAttribute("data-workbench-group-id", "group-2");
    expect(await visibleWorkbenchGroupIds(page)).toEqual(["group-1", "group-2"]);

    await selectWorkRootInBrowser(page, workRoot);
    const pinnedReadOnlyTab = page.locator(
      '[data-workbench-root-active="true"] .dockview-workbench-tab[data-workbench-pane-id^="readonly:"]',
    );
    await expect(pinnedReadOnlyTab).toHaveAttribute(
      "data-workbench-group-id",
      "group-2",
    );
    expect(await visibleWorkbenchGroupIds(page)).toContain("group-2");
    // The preceding scroll-containment step left a long-file preview pane as
    // this workRoot's active group-2 pane (per-workRoot active state is
    // preserved by design), so activate the pinned tab before inspecting the
    // pinned file content.
    await pinnedReadOnlyTab.click();
    await expect(
      page.locator('[data-workbench-root-active="true"] .readonly-text-pane'),
    ).toContainText(
      "ws-dashboard browser gate fixture",
    );
    await page.unroute(/\/api\/dashboard\/work-roots\/.*\/activity(?:\?.*)?$/);
    await page.unroute(
      /\/api\/dashboard\/work-roots\/.*\/activity\/items\/.*\/transcript(?:\?.*)?$/,
    );
    note(
      "dynamic groups: opened-file group placement stayed scoped per workRoot and did not auto-target user-created groups",
    );
  });

  await test.step("workspace remove is explicit and dashboard-only", async () => {
    if (!secondWorkRoot) {
      note(
        "workspace remove: skipped because external daemon mode did not provide WS_DASHBOARD_TEST_SECOND_WORKROOT",
      );
      return;
    }
    const secondRow = page.locator(".resource-row", {
      hasText: workRootDisplayName(secondWorkRoot),
    });
    await expect(secondRow).toBeVisible();
    const menuButton = secondRow.locator('[data-command-id="workspace.menu.open"]');
    await expect(menuButton).toHaveCSS("border-color", "rgba(0, 0, 0, 0)");
    await menuButton.click();
    const removeButton = page.locator(".workspace-row-menu").locator('[data-command-id="workspace.remove"]');
    await expect(removeButton).toBeVisible();
    page.once("dialog", async (dialog) => {
      expect(dialog.message()).toContain("Files and Git worktrees on disk will not be deleted");
      await dialog.accept();
    });
    await removeButton.click();
    await expect(secondRow).toHaveCount(0);
    expect(existsSync(secondWorkRoot)).toBe(true);
    await selectWorkRootInBrowser(page, workRoot);
    note(
      "workspace remove: confirmed dashboard-only removal, no child workRoot remove control or filesystem deletion",
    );
  });

  // --- Create a terminal and verify emulator IO ---------------------------
  await test.step("create terminal and run a command", async () => {
    // CONTRACT: This step must prove the live terminal path attaches a
    // WebSocket, does not keep periodic output polling active while connected,
    // and preserves byte-stream input fidelity for Backspace, cursor movement,
    // shell history, Ctrl keys, paste, and prompt editing.
    // Page-level request and WebSocket listeners above capture
    // `/api/dashboard/terminals/*/output` polling and socket frames while this
    // block drives a real shell prompt instead of fixture-only assertions.
    await page.locator('[data-command-id="terminal.create"]').click();
    await expectDockviewWorkbench(page);
    await terminalSurface(page);
    await expect(terminalTabs(page)).toHaveCount(1);
    await expect(page.locator(".workbench-pane-header")).toHaveCount(0);
    await expect(page.locator(".workbench-pane-status")).toHaveCount(0);
    await expect(
      page.locator('[data-command-id="terminal.close"]'),
    ).toBeVisible();
    await expect
      .poll(() => terminalSocketUrls.length, { timeout: 10_000 })
      .toBeGreaterThan(0);
    const pollsAfterSocket = terminalOutputPolls;
    await page.waitForTimeout(500);
    expect(terminalOutputPolls).toBe(pollsAfterSocket);

    const start = Date.now();
    await runInTerminal(page, commandPlan.echo("GATEOUT-12345"));
    await expect(page.locator(".xterm-rows")).toContainText("GATEOUT-12345");
    const echoMs = Date.now() - start;
    expect(echoMs).toBeLessThan(2_000);

    await page.locator(".terminal-surface").click();
    await page.keyboard.type("echo BACKSPACE-BAD");
    await page.keyboard.press("Backspace");
    await page.keyboard.press("Backspace");
    await page.keyboard.press("Backspace");
    await page.keyboard.type("OK");
    await page.keyboard.press("Enter");
    await expect(page.locator(".xterm-rows")).toContainText("BACKSPACE-OK");

    await page.keyboard.type("echo CURSOROK");
    await page.keyboard.press("ArrowLeft");
    await page.keyboard.press("ArrowLeft");
    await page.keyboard.press("ArrowRight");
    await page.keyboard.press("ArrowLeft");
    await page.keyboard.type("-");
    await page.keyboard.press("Enter");
    await expect(page.locator(".xterm-rows")).toContainText("CURSOR-OK");

    const historyBefore = (
      (await page.locator(".xterm-rows").textContent()) ?? ""
    ).split("CURSOR-OK").length;
    await page.keyboard.press("ArrowUp");
    await page.keyboard.press("Enter");
    await expect
      .poll(
        async () => {
          const text = (await page.locator(".xterm-rows").textContent()) ?? "";
          return text.split("CURSOR-OK").length;
        },
        { timeout: 5_000 },
      )
      .toBeGreaterThan(historyBefore);

    await page.keyboard.type(commandPlan.longRunningCommand());
    await page.keyboard.press("Enter");
    await page.waitForTimeout(commandPlan.profile === "cmd-exe" ? 1_000 : 200);
    await page.keyboard.press("Control+C");
    await page.keyboard.type(commandPlan.echo("CTRL-C-OK"));
    await page.keyboard.press("Enter");
    await expect(page.locator(".xterm-rows")).toContainText("CTRL-C-OK", {
      timeout: 2_000,
    });

    await page.locator(".terminal-surface").click();
    await page.keyboard.type(commandPlan.clearAndEcho("CTRL-L-OK"));
    await page.keyboard.press("Enter");
    await expect(page.locator(".xterm-rows")).toContainText("CTRL-L-OK");

    await page.locator(".terminal-surface").click();
    await page.keyboard.type("echo BAD");
    await page.keyboard.press("Control+A");
    await page.keyboard.type(commandPlan.echo("EDITED-OK"));
    await page.keyboard.press("Enter");
    await expect(page.locator(".xterm-rows")).toContainText("EDITED-OK");
    await page.keyboard.press("ArrowUp");
    await page.keyboard.press("Control+C");
    await page.keyboard.insertText("\f");
    const inputTarget = await terminalInputTarget(page);
    await inputTarget.focus();
    await expectTerminalInputFocused(page);
    await page.keyboard.type(commandPlan.echo("PASTE-OK"));
    await expectTerminalInputFocused(page);
    await page.keyboard.press("Enter");
    await expect(page.locator(".xterm-rows")).toContainText("PASTE-OK");
    await expectTerminalInputFocused(page);

    await page.locator(".terminal-surface").click();
    const hangulInputTarget = await terminalInputTarget(page);
    await hangulInputTarget.focus();
    await expectTerminalInputFocused(page);
    await page.keyboard.insertText(commandPlan.echo("한글-OK"));
    await expectTerminalInputFocused(page);
    await page.keyboard.press("Enter");
    await expect(page.locator(".xterm-rows")).toContainText("한글-OK");
    await expectTerminalInputFocused(page);

    // CONTRACT: Focused terminal panes preserve native shell line-editing
    // control bytes through the live xterm/WebSocket input path. `ctrl-u`
    // clears the current command line and `ctrl-w` deletes the previous word.
    // Assert both terminal-visible shell effects and raw input frames so a
    // fallback handler cannot swallow or synthesize the wrong path.
    await inputTarget.focus();
    await expectTerminalInputFocused(page);
    await page.keyboard.type(commandPlan.clearAndEcho("CTRL-U-START"));
    await expectTerminalInputFocused(page);
    await page.keyboard.press("Enter");
    await expect(page.locator(".xterm-rows")).toContainText("CTRL-U-START");
    await page.keyboard.type(commandPlan.echo("CTRL-U-BAD"));
    await page.keyboard.press("Control+U");
    await page.keyboard.type(commandPlan.echo("CTRL-U-OK"));
    await page.keyboard.press("Enter");
    await expect(page.locator(".xterm-rows")).toContainText("CTRL-U-OK");
    await expect(page.locator(".xterm-rows")).not.toContainText("CTRL-U-BAD");

    await inputTarget.focus();
    await page.keyboard.type(commandPlan.trailingWordEcho("CTRL-W-BAD"));
    await page.keyboard.press("Control+W");
    await page.keyboard.type("CTRL-W-OK");
    await page.keyboard.press("Enter");
    await expect(page.locator(".xterm-rows")).toContainText("CTRL-W-OK");
    await expect(page.locator(".xterm-rows")).not.toContainText("CTRL-W-BAD");

    // CONTRACT: Browser fallback key handling must not forward IME
    // composition-in-progress keystrokes as raw terminal bytes. Real Korean IME
    // commit evidence may be manual when Playwright cannot drive platform IME,
    // but this synthetic guard keeps fallback behavior observable.
    const framesBeforeComposition = terminalSocketFrames.length;
    await inputTarget.focus();
    await inputTarget.evaluate((node) =>
      node.dispatchEvent(
        new CompositionEvent("compositionstart", {
          bubbles: true,
          data: "ㅎ",
        }),
      ),
    );
    await page.evaluate(() => {
      if (document.activeElement instanceof HTMLElement) {
        document.activeElement.blur();
      }
      window.dispatchEvent(
        new KeyboardEvent("keydown", {
          bubbles: true,
          cancelable: true,
          key: "ㅎ",
        }),
      );
    });
    await inputTarget.evaluate((node) =>
      node.dispatchEvent(
        new CompositionEvent("compositionend", {
          bubbles: true,
          data: "한",
        }),
      ),
    );
    expect(terminalSocketFrames.length).toBe(framesBeforeComposition);

    // CONTRACT: The terminal focus watchdog may restore focus after terminal
    // input/output churn, but it must not steal focus back after an intentional
    // outside focus move.
    await page.locator('[data-command-id="terminal.create"]').focus();
    await page.waitForTimeout(250);
    await expect
      .poll(() =>
        page.evaluate(() => {
          const element = document.activeElement;
          return element instanceof HTMLElement
            ? element.dataset.commandId
            : "";
        }),
      )
      .toBe("terminal.create");

    expect(
      terminalSocketFrames.some((frame) => frame.includes('"type":"input"')),
    ).toBe(true);
    expect(terminalSocketFrames.some((frame) => frame.includes("\\u0015"))).toBe(
      true,
    );
    expect(terminalSocketFrames.some((frame) => frame.includes("\\u0017"))).toBe(
      true,
    );
    expect(
      terminalSocketFrames.some((frame) => frame.includes('"type":"resize"')),
    ).toBe(true);
    expect(terminalOutputPolls).toBe(pollsAfterSocket);

    // The long explorer tree and a live terminal coexist without the document
    // scrolling: both the explorer and the terminal own their own overflow.
    expect(await documentScrolls(page)).toBe(false);
    note(
      `terminal WebSocket: ${terminalSocketUrls[0]} connected; HTTP output polls stayed at ` +
        `${pollsAfterSocket} while connected; input/echo rendered in ${echoMs}ms with Backspace, cursor movement, edit, history, ` +
        "Ctrl-C, ctrl-u, ctrl-w, clear-screen control rendering/recovery, paste, committed Hangul input, IME composition guard, and no document scroll",
    );
  });

  // --- 260722-feat-dashboard-which-key-hint-overlay Phase 2 ---------------
  await test.step("which-key overlay: appearance delay, narrowing, and all four dismissal paths", async () => {
    // CONTRACT: browser-level coverage of the already-shipped Phase 1 overlay
    // (`WhichKeyOverlay.tsx`, `describeLeaderChildren` in `hotkeys.ts`) - one
    // assertion per Phase 2 "Done when" checklist line. Verification-only:
    // no product source changes accompany this step.
    const overlay = page.locator(".which-key-overlay");
    const rows = overlay.locator(".which-key-overlay-row");
    const toolbar = page.locator(".workbench-toolbar");
    // The currently-selected workRoot's own nav row - clicking it re-selects
    // the same workRoot (a real, idempotent, harmless dispatch) purely to
    // set a known `data-last-command-id` value; see dismissal path 1 below.
    const selectedResourceRow = page
      .locator('.resource-row[data-command-id="resource.select"].resource-row-selected')
      .first();

    // Leader-continuation keys and standalone bindings skip capture while
    // focus sits inside an editable element or a `.terminal-pane`
    // (hotkeys.ts `shouldSkipHotkeyCapture`) - the leader-*entry* trigger
    // itself is checked before that guard and is never blocked by terminal
    // focus (see the dedicated regression step below). Every leader press
    // here still refocuses this always-present, non-editable, non-terminal
    // toolbar button first, to keep this step's assertions about narrowing
    // and dismissal paths independent of that separately-covered trigger
    // behavior.
    const focusNeutral = async () => {
      await page.locator('[data-command-id="terminal.create"]').focus();
    };

    // --- Appearance delay: absent before 250ms, present after it ---------
    await focusNeutral();
    await page.keyboard.press("Control+Space");
    // Checked immediately after the leader press (not raced against the
    // 250ms boundary with a fixed intermediate sleep, which would carry
    // CI-jitter flakiness) - at effectively 0ms elapsed the overlay is
    // certainly still inside its appearance delay.
    await expect(overlay).toHaveCount(0);
    await expect(overlay).toBeVisible({ timeout: 1_000 });
    note(
      "which-key overlay: absent immediately after the leader press (still inside the 250ms appearance delay), visible once the delay elapses",
    );

    // --- Rows reflect live registry contents, and narrow on partial input -
    // The `a` agent-chat leader group is absent while the agent GUI is
    // suspended (260713): its only leaf (`a n`) is not registered, so no
    // top-level `a` group renders. Three top-level groups remain: r/t/g.
    await expect(rows).toHaveCount(3);
    await expect(overlay.locator(".which-key-overlay-key")).toHaveText([
      "r",
      "t",
      "g",
    ]);
    await expect(rows.filter({ hasText: "+group" })).toHaveCount(3);
    await page.keyboard.press("r");
    await expect(rows).toHaveCount(3);
    await expect(overlay).toContainText(
      "Open root picker (browse filesystem to add a new root)",
    );
    await expect(overlay).toContainText("Close active work root");
    await expect(overlay).toContainText(
      "Toggle active work root online/offline",
    );
    note(
      "which-key overlay: rows reflected the live binding registry (r/t/g groups; the a agent-chat group is suspended) and narrowed to the r group's 3 children on a partial <leader> r sequence",
    );
    // Reset to idle before the dedicated dismissal-path assertions below so
    // each one starts from the same known idle state (exercises the same
    // Escape path formally covered as dismissal path 3 further down).
    await page.keyboard.press("Escape");
    await expect(overlay).toHaveCount(0);

    // --- Dismissal path 1: a matched full sequence resolves cleanly -------
    // Evidence that the sequence actually *resolved* (rather than merely
    // cancelling) uses the same `.workbench-toolbar[data-last-command-id]`
    // dispatch hook the "activation controls are command-routed" step above
    // already relies on, instead of a downstream terminal-pane-count side
    // effect. 260722-idea-dashboard-hotkey-leader-dispatch-gap (filed
    // alongside this step) found that `<leader> t n` reaches the command bus
    // but its real `terminal.create` side effect does not fire, because the
    // App-level `executeCommand` shared handler map used by the global
    // leader-key listener only wires a handful of commandIds
    // (`workRoot.close`, `workRoot.activation.set`, `gitWorktreeAdd.open`,
    // `workspace.remove`, `worktreeHidden.menu.open`, etc.) - `terminal.create`
    // itself is wired only at its own toolbar button's local `onCommand`
    // call site, not the shared bus the leader listener dispatches through.
    // That gap is a hotkey-config-framework dispatch defect, not a
    // which-key-overlay defect, and out of this ticket's scope to fix.
    //
    // CONTRACT (regression guard): `data-last-command-id` is a single global
    // "most recent dispatched command" value, already left at
    // "terminal.create" by the real terminal-create button click earlier in
    // this test. Asserting it equals "terminal.create" without first
    // resetting it would pass vacuously even if `<leader> t n` dispatched
    // nothing at all - it would not distinguish this path from dismissal
    // path 2 (unmatched key, no dispatch). Re-selecting the already-selected
    // workRoot first is a real, idempotent, harmless dispatch that sets a
    // known-different baseline ("resource.select"), so the assertion below
    // proves a genuine transition caused by the leader sequence.
    await selectedResourceRow.click();
    await expect(toolbar).toHaveAttribute("data-last-command-id", "resource.select");
    await focusNeutral();
    await page.keyboard.press("Control+Space");
    await expect(overlay).toBeVisible({ timeout: 1_000 });
    await page.keyboard.press("t");
    await expect(rows).toHaveCount(1);
    await page.keyboard.press("n");
    await expect(overlay).toHaveCount(0);
    await expect(toolbar).toHaveAttribute("data-last-command-id", "terminal.create");
    note(
      "which-key overlay dismissal path 1 (match): <leader> t n transitioned data-last-command-id from a known " +
        "different baseline (resource.select) to terminal.create, proving it resolved and dispatched through the " +
        "command bus, and left no stale overlay DOM",
    );
    await expectTerminalNotBlocked(page, "WHICHKEY-MATCH-OK");

    // --- Dismissal path 2: an unmatched key cancels (flash-exit) ----------
    await focusNeutral();
    await page.keyboard.press("Control+Space");
    await expect(overlay).toBeVisible({ timeout: 1_000 });
    await page.keyboard.press("q");
    await expect(overlay).toHaveCount(0);
    note(
      "which-key overlay dismissal path 2 (unmatched key cancel): overlay disappeared",
    );
    await expectTerminalNotBlocked(page, "WHICHKEY-CANCEL-OK");

    // --- Dismissal path 3: Escape dismisses -------------------------------
    await focusNeutral();
    await page.keyboard.press("Control+Space");
    await expect(overlay).toBeVisible({ timeout: 1_000 });
    await page.keyboard.press("Escape");
    await expect(overlay).toHaveCount(0);
    note(
      "which-key overlay dismissal path 3 (Escape): overlay disappeared",
    );
    await expectTerminalNotBlocked(page, "WHICHKEY-ESCAPE-OK");

    // --- Dismissal path 4: a second Ctrl+Space dismisses ------------------
    await focusNeutral();
    await page.keyboard.press("Control+Space");
    await expect(overlay).toBeVisible({ timeout: 1_000 });
    await page.keyboard.press("Control+Space");
    await expect(overlay).toHaveCount(0);
    note(
      "which-key overlay dismissal path 4 (second Ctrl+Space): overlay disappeared",
    );
    await expectTerminalNotBlocked(page, "WHICHKEY-SECONDLEADER-OK");
  });

  // --- 260724-bug-dashboard-terminal-focus-swallows-ctrl-space-hotkey -----
  await test.step("which-key overlay: Ctrl+Space opens it even with a terminal pane focused", async () => {
    // CONTRACT: the leader-entry trigger (Ctrl+Space) must be checked in
    // `App.tsx#handleKeydown` before the terminal/editable-target
    // passthrough guard (`shouldSkipHotkeyCapture`), so focusing a terminal
    // pane and pressing Ctrl+Space still opens the which-key overlay instead
    // of leaking the chord as a stray control byte into the terminal
    // session. Regression coverage for the dogfood-observed bug where
    // terminal focus silently swallowed the leader trigger.
    const overlay = page.locator(".which-key-overlay");
    const surface = await terminalSurface(page);
    await surface.click();

    await page.keyboard.press("Control+Space");
    await expect(overlay).toBeVisible({ timeout: 1_000 });
    note(
      "which-key overlay opened on Ctrl+Space while a terminal pane had focus, proving the leader-entry trigger is checked before the terminal-focus passthrough guard",
    );

    await page.keyboard.press("Escape");
    await expect(overlay).toHaveCount(0);

    // Prove no stray control byte (e.g. NUL) leaked into the terminal as a
    // side effect of the leader chord - the terminal still round-trips real
    // input cleanly afterwards.
    await expectTerminalNotBlocked(page, "TERMINAL-FOCUS-LEADER-OK");
  });

  // --- 260711 Phase 1: agent chat tab shell + stub tile launch ------------
  // suspended: agent GUI (260713)
  if (!AGENT_GUI_SUSPENDED) {
  await test.step("open new agent tab and launch a stub harness session", async () => {
    // CONTRACT: the top-right "open new agent tab" button always opens a new
    // empty tab immediately (never a picker), the empty tab's "current
    // conversation" control opens a resume popup listing stub cross-harness
    // history scoped to this work root, and clicking a per-harness tile
    // actually invokes the stub `activity.session.create`/`start` call path
    // (not a UI-only state transition) - assert the resulting synthetic
    // session/transcript renders.
    await page.locator('[data-command-id="agentChat.create"]').click();
    await expectDockviewWorkbench(page);
    const agentChatTab = page
      .getByRole("tab")
      .filter({ hasText: "New agent chat" });
    await expect(agentChatTab).toHaveCount(1);

    const pane = page.locator(
      '.workbench-pane[data-surface-kind="agentChat"] .workbench-pane-body',
    );
    await expect(pane).toBeVisible();
    await expect(pane.locator('[data-testid="agent-chat-tiles"]')).toBeVisible();
    const resumeControl = pane.locator(
      '[data-command-id="agentChat.history.open"]',
    );
    await expect(resumeControl).toContainText("resume a past conversation");

    // Resume popup: opens a cross-harness history list scoped to this work
    // root, then dismisses without picking an entry (leaving the empty tab
    // untouched for the tile-launch assertion below).
    await resumeControl.click();
    const historyPopover = pane.locator(
      '[data-testid="agent-chat-history-popover"]',
    );
    await expect(historyPopover).toBeVisible();
    await expect(
      historyPopover.locator(".agent-chat-history-item"),
    ).not.toHaveCount(0);
    await page.keyboard.press("Escape");
    await expect(historyPopover).toHaveCount(0);

    // Tile launch: clicking a harness tile actually calls the stub
    // activity.session.create/start and renders the resulting synthetic
    // session/transcript - not a UI-only state flip.
    await pane.locator('[data-agent-chat-tile="codex"]').click();
    const transcript = pane.locator('[data-testid="agent-chat-transcript"]');
    await expect(transcript).toBeVisible();
    await expect(transcript).toContainText("stub provider");
    await expect(pane.locator('[data-testid="agent-chat-tiles"]')).toHaveCount(
      0,
    );
    await expect(
      page.getByRole("tab").filter({ hasText: "Codex" }),
    ).toHaveCount(1);
    note(
      "agent chat tab: open-new-tab button never blocked on a picker, resume popup rendered scoped stub history, and the Codex tile click invoked the stub session create/start path and rendered its synthetic transcript",
    );
  });
  }

  // --- 260711 Phase 2: messenger bubbles + streaming rendering ------------
  // suspended: agent GUI (260713)
  if (!AGENT_GUI_SUSPENDED) {
  await test.step("agent chat bubbles render per-turn/tool separation and stream incrementally", async () => {
    // CONTRACT: the same pane/transcript opened in the Phase 1 step above now
    // renders as messenger-style bubbles (user right-aligned, agent/tool
    // left-aligned), a collapsed-by-default thinking block, a collapsed
    // tool-use bubble with click-to-expand detail, a copy-button affordance
    // on every bubble kind, and - the browser-level acceptance evidence for
    // "live streamed turn rendering incrementally" - a growing agent-turn
    // bubble driven by the stub's chunked emission (`activitySessionStub.ts`
    // `stubBeginStreamingTurn`), observed here via repeated text-length polls
    // rather than a single before/after snapshot.
    const pane = page.locator(
      '.workbench-pane[data-surface-kind="agentChat"] .workbench-pane-body',
    );
    const transcript = pane.locator('[data-testid="agent-chat-transcript"]');
    await expect(transcript).toBeVisible();

    const userBubble = transcript.locator('[data-agent-chat-bubble-kind="user"]');
    await expect(userBubble).toHaveCount(1);
    await expect(userBubble).toHaveAttribute("data-agent-chat-bubble-align", "right");
    await expect(userBubble.locator(".agent-chat-bubble-copy")).toBeVisible();

    const toolBubble = transcript.locator('[data-agent-chat-bubble-kind="tool"]');
    await expect(toolBubble).toHaveCount(1);
    await expect(toolBubble).toHaveAttribute("data-agent-chat-bubble-align", "left");
    const toolDetail = toolBubble.locator(".agent-chat-bubble-tool-detail");
    await expect(toolDetail).toHaveCount(0);
    await toolBubble.locator(".agent-chat-bubble-tool-toggle").click();
    await expect(toolDetail).toBeVisible();

    const thinkingBlock = transcript.locator(".agent-chat-thinking-block").first();
    await expect(thinkingBlock).toBeVisible();
    const thinkingDetail = thinkingBlock.locator(".agent-chat-thinking-detail");
    await expect(thinkingDetail).toHaveCount(0);
    await thinkingBlock.locator(".agent-chat-thinking-toggle").click();
    await expect(thinkingDetail).toBeVisible();

    await expect(
      transcript.locator(".agent-chat-bubble-copy").first(),
    ).toBeVisible();

    // Streaming: poll the agent-turn bubble's rendered text length several
    // times and assert it strictly grows, then settles once the stub's
    // chunked emission finishes and renders the final markdown (a semantic
    // list, not raw dashes).
    const streamingBubble = transcript
      .locator('[data-agent-chat-bubble-kind="agentTurn"]')
      .last();
    await expect(streamingBubble).toBeVisible();
    const lengths: number[] = [];
    for (let attempt = 0; attempt < 8; attempt += 1) {
      const text = (await streamingBubble.innerText()).trim();
      lengths.push(text.length);
      await page.waitForTimeout(180);
    }
    const growingPairs = lengths.slice(1).filter((length, index) => length > lengths[index]).length;
    expect(growingPairs).toBeGreaterThan(0);
    await expect(streamingBubble.locator("li")).not.toHaveCount(0);
    note(
      `agent chat bubbles: messenger alignment (user right/agent+tool left), collapsed-by-default thinking and ` +
        `tool-use bubbles with click-to-expand detail, copy-button affordance on every bubble kind, and a streamed ` +
        `agent-turn bubble observed growing over ${lengths.length} polls (lengths ${lengths.join(",")}) before settling into final semantic markdown`,
    );
  });
  }

  // --- 260711 Phase 3: send input, mid-turn queuing, revert, history, ----
  // --- fork/resume-from-here gating ---------------------------------------
  // suspended: agent GUI (260713)
  if (!AGENT_GUI_SUSPENDED) {
  await test.step("agent chat send input, mid-turn queuing, revert, and history traversal", async () => {
    // CONTRACT: the pane opened/streamed in the Phase 1/2 steps above is a
    // Codex session (`capabilities.fork: true`, `capabilities.steer: true`,
    // `capabilities.rewind: false` per `activitySessionStub.ts`'s per-harness
    // table) - let its Phase 2 canned demo stream settle first so it never
    // interferes with the "last agent-turn bubble" queries below.
    const pane = page.locator(
      '.workbench-pane[data-surface-kind="agentChat"] .workbench-pane-body',
    );
    const transcript = pane.locator('[data-testid="agent-chat-transcript"]');
    await page.waitForTimeout(700);

    const promptInput = pane.locator('[data-testid="agent-chat-prompt-input"]');
    const sendButton = pane.locator('[data-command-id="agentChat.prompt.send"]');
    await expect(promptInput).toBeVisible();

    // CONTRACT: a pending bubble intentionally reuses the real user bubble's
    // `data-agent-chat-bubble-kind="user"` styling attribute (see
    // `App.tsx`'s pending-bubble render), so every "is there a *real*
    // transcript bubble for this text" assertion below must explicitly
    // exclude `[data-testid="agent-chat-pending-bubble"]` to avoid matching
    // the pending bubble itself.
    const realUserBubble = (text: string) =>
      transcript.locator(
        '[data-agent-chat-bubble-kind="user"]:not([data-testid="agent-chat-pending-bubble"])',
        { hasText: text },
      );

    // Sending while idle renders a real bubble and starts a streaming reply.
    await promptInput.fill("first phase-3 message");
    await sendButton.click();
    await expect(realUserBubble("first phase-3 message")).toHaveCount(1);
    const firstReplyBubble = transcript.locator('[data-agent-chat-bubble-kind="agentTurn"]').last();
    await expect(firstReplyBubble).toBeVisible();

    // Sending a second message while the first is still streaming renders it
    // as a pending/queued bubble, not a real transcript bubble - this
    // session's Codex capabilities report `steer: true`, so the badge reads
    // "steering…" (see `activitySessionStub.ts`'s per-harness table) and a
    // fire-and-forget `stubSteerActivitySession` call fires alongside the
    // local FIFO queue.
    // Phase 2 (260713-fix usability-polish): shrink the viewport before
    // queuing so the transcript's already-accumulated content (real bubble,
    // tool bubble, thinking block, streaming assistant reply from the steps
    // above) overflows a short pane, pushing the newly-queued pending bubble
    // outside the initially-visible area. This exercises the net-new
    // scroll-into-view effect (`lastPendingBubbleRef`/`pendingCountRef` in
    // `App.tsx`) rather than merely asserting it landed in the DOM.
    await page.setViewportSize({ width: 1440, height: 260 });
    await promptInput.fill("second phase-3 message");
    await sendButton.click();
    const pendingBubble = pane.locator('[data-testid="agent-chat-pending-bubble"]');
    await expect(pendingBubble).toHaveCount(1);
    await expect(pendingBubble).toContainText("second phase-3 message");
    await expect(pendingBubble.locator('[data-testid="agent-chat-pending-badge"]')).toHaveText(
      "steering…",
    );
    await expect(realUserBubble("second phase-3 message")).toHaveCount(0);
    // The queued bubble is scrolled into view automatically (not merely
    // present in the DOM off-screen) - the direct outcome-level assertion
    // for the auto-scroll fix.
    await expect(pendingBubble).toBeInViewport();
    await page.setViewportSize({ width: 1440, height: 900 });

    // Once the first turn's stub stream naturally completes (`onComplete`),
    // the pending bubble's badge clears and a new bubble/streaming reply
    // begins for it - the concrete "queued mid-turn submission landing in
    // the next tool-call batch" acceptance evidence.
    await expect(pendingBubble).toHaveCount(0, { timeout: 5000 });
    await expect(realUserBubble("second phase-3 message")).toHaveCount(1);
    const secondReplyBubble = transcript.locator('[data-agent-chat-bubble-kind="agentTurn"]').last();
    await expect(secondReplyBubble).toBeVisible();
    // Regression check for the `paneRef`/`pendingRef` stale-closure fix
    // (`App.tsx`'s `beginSimulatedTurn`/`onComplete`, Phase 3 review-fix-cycle
    // 1): dequeuing "second phase-3 message" re-invokes `beginSimulatedTurn`
    // from *inside* the first turn's `onComplete` closure, which was created
    // back when the pane's session did not yet carry "first phase-3
    // message"'s real transcript block. If that recursive call read the
    // closed-over `pane`/`pendingMessages` instead of `paneRef.current`/
    // `pendingRef.current`, `sendAgentChatMessage` would append the dequeued
    // text onto that *stale* pre-send session snapshot and
    // `applyAgentChatSession` would overwrite the pane's current session
    // with it - silently wiping out "first phase-3 message"'s already-sent
    // block. Asserting it is still present here (not just immediately after
    // its own send) is what actually falls over if `paneRef`/`pendingRef`
    // were reverted back to the plain closed-over `pane`/`pendingMessages`.
    await expect(realUserBubble("first phase-3 message")).toHaveCount(1);
    note(
      "agent chat mid-turn queuing: a message sent while a turn was in flight rendered as a pending/steering bubble " +
        "with no corresponding real transcript bubble, then cleared into a real bubble with its own streamed reply " +
        "once the in-flight turn's stub stream naturally completed, without clobbering the earlier real send " +
        "(paneRef/pendingRef stale-closure regression check)",
    );

    // Revert: clicking revert on a still-pending bubble removes it with no
    // corresponding new bubble ever appearing for it, and pulls its text
    // back into the editable input.
    await promptInput.fill("third phase-3 message (should be reverted)");
    await sendButton.click();
    await expect(pendingBubble).toHaveCount(1);
    await pendingBubble.locator('[data-command-id="agentChat.pending.revert"]').click();
    await expect(pendingBubble).toHaveCount(0);
    await expect(promptInput).toHaveValue("third phase-3 message (should be reverted)");
    await promptInput.fill("");
    // Give the in-flight turn (still delivering "second phase-3 message")
    // time to complete naturally, then confirm the reverted text was truly
    // never sent - no real bubble ever appears for it.
    await page.waitForTimeout(2500);
    await expect(realUserBubble("third phase-3 message")).toHaveCount(0);
    note(
      "agent chat revert: reverting a still-pending bubble removed it, restored its text into the editable input, " +
        "and no corresponding bubble was ever sent for it",
    );

    // Prompt-box up/down history traversal cycles through previously *sent*
    // message texts (the reverted-and-never-sent third message is correctly
    // absent from history). Consecutive same-direction and direction-switch
    // presses require no intervening "Home" - the caret is reset to column 0
    // after each recall, so repeated arrows keep walking through history.
    await promptInput.press("ArrowUp");
    await expect(promptInput).toHaveValue("second phase-3 message");
    await promptInput.press("ArrowUp");
    await expect(promptInput).toHaveValue("first phase-3 message");
    await promptInput.press("ArrowDown");
    await expect(promptInput).toHaveValue("second phase-3 message");
    await promptInput.press("ArrowDown");
    await expect(promptInput).toHaveValue("");
    note(
      "agent chat prompt history: consecutive up/down arrow presses (no intervening caret reset needed) cycled " +
        "through previously sent message texts only, excluding the reverted-and-never-sent entry",
    );

    // "Fork from here" (shipped live, Codex `capabilities.fork: true`):
    // clicking it opens a *new* tab/pane whose transcript is truncated to
    // exactly the clicked bubble's cut point - it must contain the forked
    // bubble's own text but not text sent afterward in the original pane.
    const forkSourceBubble = realUserBubble("first phase-3 message");
    await expect(
      forkSourceBubble.locator('[data-command-id="agentChat.bubble.forkFromHere"]'),
    ).toBeVisible();
    // "Resume from here" never renders for this (or any) current harness -
    // the load-bearing `capabilities.rewind: false` gate from Resolved
    // Strategic Question 1 (no harness cleanly supports point-based rewind).
    await expect(
      forkSourceBubble.locator('[data-command-id="agentChat.bubble.resumeFromHere"]'),
    ).toHaveCount(0);

    const tabCountBeforeFork = await page.getByRole("tab").count();
    await forkSourceBubble.locator('[data-command-id="agentChat.bubble.forkFromHere"]').click();
    await expect(page.getByRole("tab")).toHaveCount(tabCountBeforeFork + 1);
    const forkedPane = page
      .locator('.workbench-pane[data-surface-kind="agentChat"] .workbench-pane-body')
      .last();
    const forkedTranscript = forkedPane.locator('[data-testid="agent-chat-transcript"]');
    await expect(forkedTranscript).toContainText("first phase-3 message");
    await expect(forkedTranscript).toContainText("Forked from conversation");
    await expect(forkedTranscript).not.toContainText("second phase-3 message");
    note(
      'agent chat "fork from here": clicking it on the Codex session (capabilities.fork: true) opened a new tab ' +
        "whose transcript was truncated to exactly that bubble's cut point (contained the forked bubble's own text " +
        "and a synthetic fork marker, but none of the original pane's later messages)",
    );

    // Cross-harness confirmation (ticket verification boundary): open a
    // second brand-new tab and start a Claude session -
    // `capabilities.fork: false`/`capabilities.rewind: false` per the stub's
    // per-harness table - and confirm neither "fork from here" nor "resume
    // from here" render on its user bubble either, i.e. "resume from here"
    // is verifiably disabled/hidden for every current harness, not just
    // Codex.
    await page.locator('[data-command-id="agentChat.create"]').click();
    const claudePane = page
      .locator('.workbench-pane[data-surface-kind="agentChat"] .workbench-pane-body')
      .last();
    await claudePane.locator('[data-agent-chat-tile="claude"]').click();
    const claudeTranscript = claudePane.locator('[data-testid="agent-chat-transcript"]');
    await expect(claudeTranscript).toBeVisible();
    const claudeUserBubble = claudeTranscript.locator('[data-agent-chat-bubble-kind="user"]').first();
    await expect(claudeUserBubble).toBeVisible();
    await expect(
      claudeUserBubble.locator('[data-command-id="agentChat.bubble.forkFromHere"]'),
    ).toHaveCount(0);
    await expect(
      claudeUserBubble.locator('[data-command-id="agentChat.bubble.resumeFromHere"]'),
    ).toHaveCount(0);
    note(
      '"resume from here" confirmed disabled/hidden on a Claude session too (capabilities.fork: false, ' +
        "capabilities.rewind: false), not just the Codex session exercised above - neither affordance rendered",
    );
  });
  }

  // --- ANSI color rendering -----------------------------------------------
  await test.step("ANSI color rendering", async () => {
    await runInTerminal(page, commandPlan.ansiGreen("GATE-GREEN"));
    if (commandPlan.profile === "cmd-exe") {
      await expect(page.locator(".xterm-rows")).toContainText("GATE-GREEN");
      note(
        "ANSI: cmd.exe profile asserted visible text only; SGR color is recorded as a limitation",
      );
    } else {
      // The output is rendered in a green (palette index 2) emulator span. A
      // plain text check is intentionally avoided here: the PTY-echoed input
      // line also contains `GATE-GREEN`, so only the colored span proves the SGR
      // sequence was interpreted rather than printed raw.
      await expect(
        page.locator(".xterm-rows span.xterm-fg-2", { hasText: "GATE-GREEN" }),
      ).toBeVisible();
      note(
        "ANSI: SGR color sequence rendered as terminal color (xterm-fg-2), not raw text",
      );
    }
    await page.screenshot({
      path: path.join(artifactsDir, "terminal-emulator.png"),
    });
  });

  // --- Scrolled terminal keeps the active bottom row visible ---------------
  await test.step("terminal scrolled bottom row remains visible", async () => {
    await runInTerminal(page, commandPlan.scrollLines("SCROLL-LINE-", 80));
    await expect(page.locator(".xterm-rows")).toContainText("SCROLL-LINE-80");

    const bottomRowVisible = await page.evaluate(() => {
      const surface = document.querySelector(".terminal-surface");
      const rows = Array.from(document.querySelectorAll(".xterm-rows > div"));
      const row = rows.find((element) =>
        (element.textContent ?? "").includes("SCROLL-LINE-80"),
      );
      if (!surface || !row) return false;
      const surfaceBox = surface.getBoundingClientRect();
      const rowBox = row.getBoundingClientRect();
      return rowBox.bottom <= surfaceBox.bottom && rowBox.top >= surfaceBox.top;
    });
    expect(bottomRowVisible).toBe(true);
    note(
      "terminal scroll: SCROLL-LINE-80 active bottom row stayed fully inside .terminal-surface",
    );
  });

  // --- Alternate-screen terminal apps fit inside the visible surface -------
  await test.step("terminal alternate-screen bottom row remains visible", async () => {
    await runInTerminal(
      page,
      commandPlan.alternateScreenBottomRow("TUIBOTTOM"),
    );
    await expect(page.locator(".xterm-rows")).toContainText("TUIBOTTOM");

    const bottomRowVisible = await page.evaluate(() => {
      const surface = document.querySelector(".terminal-surface");
      const rows = Array.from(document.querySelectorAll(".xterm-rows > div"));
      const row = rows.find((element) =>
        (element.textContent ?? "").includes("TUIBOTTOM"),
      );
      if (!surface || !row) return false;
      const surfaceBox = surface.getBoundingClientRect();
      const rowBox = row.getBoundingClientRect();
      return rowBox.bottom <= surfaceBox.bottom && rowBox.top >= surfaceBox.top;
    });
    expect(bottomRowVisible).toBe(true);
    note(
      "terminal alternate-screen: synthetic TUI bottom row stayed fully inside .terminal-surface",
    );
  });

  // --- Terminal fills the pane --------------------------------------------
  await test.step("terminal fills the pane", async () => {
    // Compare the emulator surface against the height of its actual containing
    // Dockview workbench pane body, not a fixed constant: a partially filled
    // terminal must fail this regardless of viewport size.
    const paneBody = page.locator(
      '.workbench-pane[data-surface-kind="persistentTerminal"] .workbench-pane-body',
    );
    const terminalPane = page.locator(".terminal-pane");
    const surface = page.locator(".terminal-surface");
    const controls = page.locator(".terminal-controls");
    const bodyBox = await paneBody.boundingBox();
    const terminalBox = await terminalPane.boundingBox();
    const surfaceBox = await surface.boundingBox();
    const controlsBox = await controls.boundingBox();
    expect(bodyBox).not.toBeNull();
    expect(terminalBox).not.toBeNull();
    expect(surfaceBox).not.toBeNull();
    expect(controlsBox).not.toBeNull();

    const filledHeight = surfaceBox!.height + controlsBox!.height;
    expect(Math.abs(terminalBox!.height - bodyBox!.height)).toBeLessThanOrEqual(
      1,
    );
    expect(Math.abs(filledHeight - bodyBox!.height)).toBeLessThanOrEqual(1);
    expect(Math.abs(terminalBox!.width - bodyBox!.width)).toBeLessThanOrEqual(
      1,
    );
    expect(Math.abs(surfaceBox!.width - bodyBox!.width)).toBeLessThanOrEqual(1);
    expect(
      Math.abs(
        controlsBox!.y + controlsBox!.height - (bodyBox!.y + bodyBox!.height),
      ),
    ).toBeLessThanOrEqual(1);
    note(
      `pane fill: terminal surface+controls ${Math.round(filledHeight)}px of ` +
        `${Math.round(bodyBox!.height)}px containing pane body`,
    );
  });

  // --- Terminal tab selection and per-session isolation -------------------
  await test.step("terminal tab selection isolates sessions", async () => {
    await page.locator('[data-command-id="terminal.create"]').click();
    await expect(terminalTabs(page)).toHaveCount(2);
    await terminalSurface(page);
    await runInTerminal(page, commandPlan.echo("SECOND-MARKER"));
    await expect(page.locator(".xterm-rows")).toContainText("SECOND-MARKER");

    // Switch back to the first terminal tab and settle past a full output
    // poll cycle: the selection must stick. Without the focus guard the
    // poll-driven editorGroups rebuild steals focus back to the most-recently
    // created terminal, so this fails first if the guard regresses.
    await terminalTabs(page).nth(0).click();
    await settlePastPollCycle(page);
    await terminalSurface(page);
    await expect(page.locator(".xterm-rows")).toContainText("SCROLL-LINE-80");
    await expect(page.locator(".xterm-rows")).not.toContainText(
      "SECOND-MARKER",
    );

    // Switch to the second terminal tab; it shows only its own output.
    await terminalTabs(page).nth(1).click();
    await settlePastPollCycle(page);
    await terminalSurface(page);
    await expect(page.locator(".xterm-rows")).toContainText("SECOND-MARKER");
    await expect(page.locator(".xterm-rows")).not.toContainText(
      "GATEOUT-12345",
    );
    note(
      "tab selection: tab focus survives a poll cycle; input/output stay isolated per session",
    );
  });

  // --- Close terminates the session ---------------------------------------
  await test.step("terminal tab close confirms then terminates session", async () => {
    await page.locator(".terminal-surface").click();
    await page.keyboard.press("Control+D");
    const activeTerminalTab = page
      .locator(
        '[data-workbench-root-active="true"] .dockview-workbench-tab[data-workbench-close-confirmation="confirmSessionClose"]',
        { hasText: "Terminal" },
      )
      .last();
    await activeTerminalTab.hover();
    await activeTerminalTab
      .locator('[data-command-id="workbench.tab.close"]')
      .click();
    const popover = page.locator('[data-workbench-close-popover="cursor-near"]');
    await expect(popover).toBeVisible();
    await popover
      .locator('[data-command-id="workbench.tab.close.cancel"]')
      .click();
    await expect(popover).toHaveCount(0);
    await expect(terminalTabs(page)).toHaveCount(2);

    await activeTerminalTab.hover();
    await activeTerminalTab
      .locator('[data-command-id="workbench.tab.close"]')
      .click();
    await expect(popover).toBeVisible();
    await popover
      .locator('[data-command-id="workbench.tab.close.confirm"]')
      .click();
    await expect(terminalTabs(page)).toHaveCount(1);
    note(
      "close: terminal tab close showed cursor-near No/Yes confirmation; No preserved the tab and Yes terminated one daemon session",
    );
  });

  // --- Refresh keeps daemon-owned terminal, shows no mock surfaces --------
  await test.step("refresh without mock surfaces", async () => {
    await page.reload({ waitUntil: "domcontentloaded" });
    await expect(page.locator(".app-shell")).toBeVisible();
    await selectWorkRootInBrowser(page, workRoot);
    // The daemon owns the terminal lifecycle, so the surviving session is
    // reconstructed as a selectable tab after reload.
    await expect(terminalTabs(page)).toHaveCount(1);
    await terminalTabs(page).nth(0).click();
    await terminalSurface(page);
    const restoredPinnedReadOnlyTab = page.locator(
      '[data-workbench-root-active="true"] .dockview-workbench-tab[data-workbench-pane-id^="readonly:"]',
    );
    await expect(restoredPinnedReadOnlyTab).toBeVisible();
    await restoredPinnedReadOnlyTab.click();
    await expect(
      page.locator('[data-workbench-root-active="true"] .readonly-text-pane'),
    ).toContainText(
      "ws-dashboard browser gate fixture",
    );
    await page.screenshot({
      path: path.join(artifactsDir, "desktop-workbench.png"),
      fullPage: true,
    });
    note(
      "refresh: daemon-owned terminal and browser-owned read-only file descriptors reconstruct after reload, no mock surfaces",
    );
  });

  // --- Narrow viewport relayout and bounded PTY resize --------------------
  await test.step("narrow viewport relayout and bounded resize", async () => {
    const wideColumns = await terminalColumns(page);
    expect(wideColumns).toBeGreaterThan(0);

    await page.setViewportSize({ width: 480, height: 900 });
    await expect(page.locator(".app-shell")).toBeVisible();
    await expect(page.locator(".file-explorer")).toBeVisible();

    // The ResizeObserver refits the emulator and debounced resize forwarding
    // updates the daemon-owned PTY logical size; the footer reflects it once
    // the daemon confirms the (bounded) resize.
    await expect
      .poll(() => terminalColumns(page), { timeout: 20_000 })
      .toBeLessThan(wideColumns);

    await page.screenshot({
      path: path.join(artifactsDir, "narrow-workbench.png"),
      fullPage: true,
    });
    note(
      `narrow viewport: 480px relayout refit the PTY below ${wideColumns} columns ` +
        "via bounded resize forwarding",
    );
    await page.setViewportSize({ width: 1440, height: 900 });
  });

  // --- Terminal-clear-on-tab-switch regression gate ------------------------
  // (260707-bug-dashboard-terminal-clears-on-tab-switch, Phase 1). Live repro
  // established the actual trigger is a *visible-but-too-short* pane height
  // reaching `fitNow()` in App.tsx, not the hidden/detached state the
  // ticket's two named repro modes (dockview tab switch, session/workRoot
  // round-trip) were originally assumed to hit directly. The short-viewport
  // transition below is the load-bearing new assertion; the two named modes
  // are kept as non-regression coverage since they already passed pre-fix.
  // Reuses the already-paired `page` and already-open `workRoot`/terminal
  // tab from the steps above instead of a separate `test()`, since the
  // daemon's owner-pairing URL is single-use and a fresh `test()` would need
  // to consume it again.
  let terminalClearFixTallRows = 0;
  await test.step("short viewport does not collapse the terminal to 1 row", async () => {
    await terminalTabs(page).nth(0).click();
    await terminalSurface(page);
    await runInTerminal(page, commandPlan.scrollLines("CLEAR-FIX-LINE-", 40));
    await expect(page.locator(".xterm-rows")).toContainText(
      "CLEAR-FIX-LINE-40",
    );
    const tallRows = await terminalRows(page);
    expect(tallRows).toBeGreaterThan(1);
    terminalClearFixTallRows = tallRows;

    await page.setViewportSize({ width: 1440, height: 160 });
    // Give the ResizeObserver -> fitNow -> debounced forwardSize cycle time to
    // settle; pre-fix this drives the footer to `<cols>x1` and clears the
    // rendered screen within a couple hundred ms.
    await page.waitForTimeout(800);
    const shortRows = await terminalRows(page);
    const shortEmulatorRows = await emulatorRowCount(page);
    // Assert the fix's documented contract exactly (fitNow() "preserves the
    // current (last-good) emulator size and returns without resizing" on a
    // degenerate proposal), not just "didn't floor to 1" - a regression that
    // let the guard trip too late and shrink partway (e.g. 54 -> 3 rows)
    // would still pass a `toBeGreaterThan(1)` check but must fail here.
    expect(shortRows).toBe(terminalClearFixTallRows);
    expect(shortEmulatorRows).toBe(terminalClearFixTallRows);
    await expect(page.locator(".xterm-rows")).toContainText(
      "CLEAR-FIX-LINE-40",
    );
    await page.screenshot({
      path: path.join(artifactsDir, "terminal-short-viewport-guard.png"),
    });
    note(
      `terminal-clear fix: short viewport (160px) preserved ${shortRows} rows ` +
        `(emulator ${shortEmulatorRows} rows), content not cleared, tall baseline was ${tallRows} rows`,
    );
  });

  await test.step("growing the viewport back recovers the full size", async () => {
    await page.setViewportSize({ width: 1440, height: 900 });
    await expect
      .poll(() => terminalRows(page), { timeout: 20_000 })
      .toBe(terminalClearFixTallRows);
    await expect(page.locator(".xterm-rows")).toContainText(
      "CLEAR-FIX-LINE-40",
    );
    note(
      `terminal-clear fix: growing viewport back recovered ${terminalClearFixTallRows} rows`,
    );
  });

  await test.step("dockview tab switch does not collapse the terminal (ticket repro mode 1)", async () => {
    await page.locator('[data-command-id="terminal.create"]').click();
    await expect(terminalTabs(page)).toHaveCount(2);
    await terminalSurface(page);
    await runInTerminal(page, commandPlan.echo("CLEAR-FIX-SECOND-TAB-MARKER"));
    await expect(page.locator(".xterm-rows")).toContainText(
      "CLEAR-FIX-SECOND-TAB-MARKER",
    );

    await terminalTabs(page).nth(0).click();
    await terminalSurface(page);
    await expect(page.locator(".xterm-rows")).toContainText(
      "CLEAR-FIX-LINE-40",
    );
    expect(await terminalRows(page)).toBe(terminalClearFixTallRows);

    await terminalTabs(page).nth(1).click();
    await terminalSurface(page);
    await expect(page.locator(".xterm-rows")).toContainText(
      "CLEAR-FIX-SECOND-TAB-MARKER",
    );

    await terminalTabs(page).nth(0).click();
    await terminalSurface(page);
    await expect(page.locator(".xterm-rows")).toContainText(
      "CLEAR-FIX-LINE-40",
    );
    expect(await terminalRows(page)).toBe(terminalClearFixTallRows);
    note(
      "terminal-clear fix: repeated dockview tab switches left the first terminal's rows/content intact",
    );

    // Close the extra tab so it does not affect steps below.
    await terminalTabs(page).nth(1).click();
    await page.locator(".terminal-surface").click();
    await page.keyboard.press("Control+D");
    const activeTerminalTab = page
      .locator(
        '[data-workbench-root-active="true"] .dockview-workbench-tab[data-workbench-close-confirmation="confirmSessionClose"]',
        { hasText: "Terminal" },
      )
      .last();
    await activeTerminalTab.hover();
    await activeTerminalTab
      .locator('[data-command-id="workbench.tab.close"]')
      .click();
    const closePopover = page.locator(
      '[data-workbench-close-popover="cursor-near"]',
    );
    await expect(closePopover).toBeVisible();
    await closePopover
      .locator('[data-command-id="workbench.tab.close.confirm"]')
      .click();
    await expect(terminalTabs(page)).toHaveCount(1);
  });

  await test.step("session/workRoot round-trip does not collapse the terminal (ticket repro mode 2)", async () => {
    if (!secondWorkRoot) {
      note(
        "terminal-clear fix: second workRoot round-trip skipped (not configured for external daemon)",
      );
      return;
    }
    // The "workspace remove is explicit and dashboard-only" step above
    // already removed `secondWorkRoot` from the dashboard workspace list, so
    // it is no longer a selectable resource here; reopen it by path instead
    // of using `selectWorkRootInBrowser`.
    await openWorkRootInBrowser(page, secondWorkRoot);
    await selectWorkRootInBrowser(page, workRoot);
    await terminalTabs(page).nth(0).click();
    await terminalSurface(page);
    await expect(page.locator(".xterm-rows")).toContainText(
      "CLEAR-FIX-LINE-40",
    );
    expect(await terminalRows(page)).toBe(terminalClearFixTallRows);
    note(
      "terminal-clear fix: workRoot round-trip (open second root, switch back) left the terminal's rows/content intact",
    );
  });

  // --- Terminal split durability across non-horizontal drop positions -----
  // (260720-bug-dashboard-terminal-split-nonhorizontal-snap-back). The
  // confirmed root cause: `movePane` persisted a drag/drop-created dynamic
  // group and its pane order keyed by the bare `workbenchModel.root.id`,
  // while every reader (and every other writer) resolves layout under the
  // server-scoped `serverScopedIdentity(...)` key - so the derived group
  // list never contained the freshly created group and the moved terminal
  // fell back into `groups[0]`, and `syncDockviewWorkbench` reconciled
  // Dockview back to match (the reported snap-back). This only reproduced
  // for a **multi-pane source group** (the realistic way every ticket
  // gesture creates a new group), so each drag below is seeded from a group
  // that still holds >=1 other terminal pane at the moment of the drag.
  await test.step("terminal split durability across non-horizontal drop positions (260720)", async () => {
    await expect(terminalTabs(page)).toHaveCount(1);
    const [firstPaneId] = await terminalPaneIds(page);
    expect(firstPaneId).toBeTruthy();
    const originGroupId = await terminalTabById(
      page,
      firstPaneId,
    ).getAttribute("data-workbench-group-id");
    expect(originGroupId).not.toBeNull();

    // --- Vertical 2-way: seed a 2nd terminal into the origin group (now
    // multi-pane), then split it to the origin group's own bottom edge.
    const secondPaneId = await createTerminalAndGetNewPaneId(page);
    await expectDockviewWorkbench(page);
    const secondGroupIdBeforeMove = await terminalTabById(
      page,
      secondPaneId,
    ).getAttribute("data-workbench-group-id");
    expect(secondGroupIdBeforeMove).toBe(originGroupId);

    const originBoxForVertical = await groupViewBoundingBox(
      terminalTabById(page, secondPaneId),
    );
    const verticalMoved = await expectDurableTerminalSplitDrop(
      page,
      terminalTabById(page, secondPaneId),
      originBoxForVertical,
      { fx: 0.5, fy: 0.95 },
    );
    expect(
      (await visibleWorkbenchGroupIds(page)).length,
    ).toBeGreaterThanOrEqual(2);
    note(
      `terminal split: vertical bottom-edge drop moved terminal ${verticalMoved.paneId} ` +
        `into group ${verticalMoved.groupId} (origin ${originGroupId}); it survived a poll settle`,
    );

    // --- 3-way: a fresh terminal always lands in `groups[0]`
    // (`placeTerminalSessions`/`terminalPlacementState`, which fixes
    // `focusedGroupId` to `groups[0]`, not "whichever tab was last
    // clicked"), and the origin group stays `groups[0]` through the moves
    // above - so the next terminal lands in the (still multi-pane, since T1
    // remains) origin group again. Split it to a fresh (right) edge.
    const thirdPaneId = await createTerminalAndGetNewPaneId(page);
    const thirdGroupIdBeforeMove = await terminalTabById(
      page,
      thirdPaneId,
    ).getAttribute("data-workbench-group-id");
    expect(thirdGroupIdBeforeMove).toBe(originGroupId);

    const originBoxForThreeWay = await groupViewBoundingBox(
      terminalTabById(page, thirdPaneId),
    );
    const threeWayMoved = await expectDurableTerminalSplitDrop(
      page,
      terminalTabById(page, thirdPaneId),
      originBoxForThreeWay,
      { fx: 0.95, fy: 0.5 },
    );
    const groupIdsAfterThreeWay = await visibleWorkbenchGroupIds(page);
    expect(new Set(groupIdsAfterThreeWay).size).toBeGreaterThanOrEqual(3);
    note(
      `terminal split: 3-way split left ${groupIdsAfterThreeWay.length} distinct groups ` +
        `(${groupIdsAfterThreeWay.join(", ")}) standing after a poll settle`,
    );

    // --- Inner-group edge: a 4th terminal again lands in the (still
    // multi-pane) origin group; drop it onto the *vertical split's* existing
    // group - an already-populated inner group, not the outer
    // `[data-workbench-layout-owner]` container. Target the vertical split's
    // group on its right edge, not its top edge: the top edge directly abuts
    // the sash it shares with the origin group from the vertical split
    // above, and (like the outer-owner/sash pitfall noted in the ticket's
    // root-cause research) a drop that close to an existing sash does not
    // register as that group's own edge-split zone.
    const fourthPaneId = await createTerminalAndGetNewPaneId(page);
    const fourthGroupIdBeforeMove = await terminalTabById(
      page,
      fourthPaneId,
    ).getAttribute("data-workbench-group-id");
    expect(fourthGroupIdBeforeMove).toBe(originGroupId);

    const innerTargetGroupBox = await groupViewBoundingBox(
      terminalTabById(page, verticalMoved.paneId),
    );
    const innerMoved = await expectDurableTerminalSplitDrop(
      page,
      terminalTabById(page, fourthPaneId),
      innerTargetGroupBox,
      { fx: 0.95, fy: 0.5 },
    );
    expect(innerMoved.groupId).not.toBe(originGroupId);
    expect(innerMoved.groupId).not.toBe(verticalMoved.groupId);
    const groupIdsAfterInnerEdge = await visibleWorkbenchGroupIds(page);
    expect(new Set(groupIdsAfterInnerEdge).size).toBeGreaterThanOrEqual(3);
    note(
      `terminal split: inner-group-edge drop onto the vertical split's own group ` +
        `(${verticalMoved.groupId}) moved terminal ${innerMoved.paneId} into ` +
        `${innerMoved.groupId}, resolved from that group's own .dv-groupview rect; it persisted`,
    );
  });

  await test.step("bounded resource polling runs while mounted and stops after unmount", async () => {
    const beforePollingWindow = resourceRefreshRequests;
    await expect
      .poll(() => resourceRefreshRequests, { timeout: 7_000 })
      .toBeGreaterThan(beforePollingWindow);

    const beforeUnmount = resourceRefreshRequests;
    await page.goto("about:blank");
    await page.waitForTimeout(5_500);
    expect(resourceRefreshRequests).toBe(beforeUnmount);
    note(
      "resources: mounted dashboard polled /api/dashboard/resources and stopped after page unmount",
    );
  });

  if (portabilityEvidence) {
    portabilityEvidence.browserGate.result = "pass";
  }
});

test("linked server root picker uses server-scoped local gateway routes", async ({
  page,
}) => {
  const remoteGatewayRequests: string[] = [];
  let localRootPickerHits = 0;
  let remoteResourcesRefreshes = 0;

  await page.route("**/api/dashboard/servers", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(linkedServerBrowserServers()),
    });
  });
  await page.route("**/api/dashboard/resources", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(linkedServerBrowserResources("server-local")),
    });
  });
  await page.route("**/api/dashboard/root-picker**", async (route) => {
    localRootPickerHits += 1;
    await route.fulfill({
      status: 500,
      contentType: "application/json",
      body: JSON.stringify({ error: "local root picker should not be used" }),
    });
  });
  await page.route(
    "**/api/dashboard/servers/server-remote/resources",
    async (route) => {
      remoteResourcesRefreshes += 1;
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(
          linkedServerBrowserResources("server-remote", "remote-root-opened"),
        ),
      });
    },
  );
  await page.route(
    "**/api/dashboard/servers/server-remote/root-picker**",
    async (route) => {
      const url = new URL(route.request().url());
      remoteGatewayRequests.push(
        `${route.request().method()} ${url.pathname}${url.search}`,
      );
      if (url.pathname.endsWith("/directories")) {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            name: "new-child",
            path: "/remote/target/new-child",
            entryType: "directory",
            selectable: true,
            kindLabel: "Folder",
          }),
        });
        return;
      }
      if (url.pathname.endsWith("/pins")) {
        const places =
          route.request().method() === "POST"
            ? [
                {
                  id: "pin-remote-target",
                  label: "target",
                  path: "/remote/target",
                  kind: "pin",
                  source: "pin",
                  available: true,
                },
              ]
            : [];
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({ places }),
        });
        return;
      }
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(
          linkedServerPickerView(url.searchParams.get("path") ?? "/remote/home"),
        ),
      });
    },
  );
  await page.route(
    "**/api/dashboard/servers/server-remote/work-roots/open",
    async (route) => {
      remoteGatewayRequests.push(
        `${route.request().method()} ${new URL(route.request().url()).pathname}`,
      );
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        headers: {
          "x-ws-dashboard-opened-work-root-id": "remote-root-opened",
        },
        body: JSON.stringify(
          linkedServerBrowserResources("server-remote", "remote-root-opened"),
        ),
      });
    },
  );

  if (!ownerCookies) {
    throw new Error(
      "ownerCookies not captured - the owner-pairing test must run first (serial mode) and capture the session cookie before this test can reuse it",
    );
  }
  await page.context().addCookies(ownerCookies);
  await page.goto(daemon.baseUrl, { waitUntil: "domcontentloaded" });
  await expect(page.locator(".app-shell")).toBeVisible();

  const remoteRow = page.locator(".server-row", { hasText: "Remote fixture" });
  await remoteRow.locator('[data-command-id="rootPicker.open"]').click();
  const modal = page.locator(".root-picker-modal");
  await expect(modal.locator(".root-picker-title")).toHaveText(
    "Open workRoot on Remote fixture",
  );
  await expect(modal.locator(".root-picker-current")).toContainText(
    "/remote/home",
  );

  await modal.locator(".root-picker-address").fill("/remote/target");
  await modal.locator(".root-picker-address").press("Enter");
  await expect(modal.locator(".root-picker-current")).toContainText(
    "/remote/target",
  );
  await modal.locator("#root-picker-create-name").fill("new-child");
  await modal.locator('[data-command-id="rootPicker.createDirectory"]').click();
  await expect(
    modal.locator(".root-picker-row", { hasText: "new-child" }),
  ).toBeVisible();
  await modal.locator('[data-command-id="rootPicker.pinDirectory"]').click();
  await expect(
    modal.locator(".root-picker-place-row-pinned", { hasText: "target" }),
  ).toBeVisible();
  await modal.locator('[data-command-id="rootPicker.unpinDirectory"]').click();
  await expect(
    modal.locator(".root-picker-place-row-pinned", { hasText: "target" }),
  ).toHaveCount(0);
  await modal.locator("#root-picker-exact-path").fill("/remote/opened");
  await modal
    .locator('[data-command-id="workRoot.open"]')
    .filter({ hasText: "Open" })
    .click();

  await expect(modal).toHaveCount(0);
  await expect(remoteRow).toHaveClass(/server-row-selected/);
  await expect(
    page.locator('[data-resource-id="remote-root-opened"]'),
  ).toHaveClass(/resource-row-selected/);
  expect(localRootPickerHits).toBe(0);
  expect(remoteResourcesRefreshes).toBeGreaterThanOrEqual(1);
  expect(remoteGatewayRequests).toEqual(
    expect.arrayContaining([
      "GET /api/dashboard/servers/server-remote/root-picker",
      "GET /api/dashboard/servers/server-remote/root-picker?path=%2Fremote%2Ftarget",
      "POST /api/dashboard/servers/server-remote/root-picker/directories",
      "POST /api/dashboard/servers/server-remote/root-picker/pins",
      "DELETE /api/dashboard/servers/server-remote/root-picker/pins",
      "POST /api/dashboard/servers/server-remote/work-roots/open",
    ]),
  );

  // --- 260711 Phase 4: agent chat pane identity is server-route-scoped ----
  // suspended: agent GUI (260713)
  if (!AGENT_GUI_SUSPENDED) {
  await test.step("agent chat tab on a linked remote server behaves identically to the local case and is server-route-scoped", async () => {
    // CONTRACT: `createAgentChatPane` (`App.tsx`) reads `serverRoute` from
    // the real UI state (`workbenchModel.root.resourcePath.serverId`), now
    // "server-remote" after the work root opened above, and threads it
    // through `agentChatPaneId`/`agentChatPaneLogicalKey`
    // (`agentChatSessions.ts`) into the rendered pane's
    // `data-workbench-pane-id`. `activitySessionStub.ts` is in-memory only
    // (no `fetch`), so this asserts server-scoping via that DOM identity
    // rather than a network mock, while asserting the same rendered
    // transcript/tile behavior as the local flow (`#L2534-L2589`).
    await expectDockviewWorkbench(page);
    await page.locator('[data-command-id="agentChat.create"]').click();
    const agentChatTab = page
      .getByRole("tab")
      .filter({ hasText: "New agent chat" });
    await expect(agentChatTab).toHaveCount(1);

    const pane = page.locator(
      '.workbench-pane[data-surface-kind="agentChat"]',
    );
    await expect(pane).toBeVisible();
    const paneId = await pane.getAttribute("data-workbench-pane-id");
    expect(paneId).toContain(encodeURIComponent("server-remote"));

    const paneBody = pane.locator(".workbench-pane-body");
    await expect(paneBody.locator('[data-testid="agent-chat-tiles"]')).toBeVisible();
    await paneBody.locator('[data-agent-chat-tile="codex"]').click();
    const transcript = paneBody.locator('[data-testid="agent-chat-transcript"]');
    await expect(transcript).toBeVisible();
    await expect(transcript).toContainText("stub provider");
    await expect(
      page.getByRole("tab").filter({ hasText: "Codex" }),
    ).toHaveCount(1);
    note(
      `agent chat on a linked remote server: the new tab/pane rendered and the Codex tile click invoked the stub ` +
        `session create/start path identically to the local flow, and the pane's identity (${paneId}) was scoped ` +
        "to the remote Server Route rather than server-local",
    );
  });
  }

  await page.unroute("**/api/dashboard/servers/server-remote/root-picker**");
  let releaseFirstPicker: ((value: void) => void) | null = null;
  let remoteStalePickerRequests = 0;
  await page.route(
    "**/api/dashboard/servers/server-remote/root-picker**",
    async (route) => {
      remoteStalePickerRequests += 1;
      if (remoteStalePickerRequests === 1) {
        await new Promise<void>((resolve) => {
          releaseFirstPicker = resolve;
        });
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify(linkedServerPickerView("/remote/stale")),
        });
        return;
      }
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(linkedServerPickerView("/remote/fresh")),
      });
    },
  );
  await page.route(
    "**/api/dashboard/servers/server-other/root-picker**",
    async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(linkedServerPickerView("/other/home")),
      });
    },
  );

  await remoteRow.locator('[data-command-id="rootPicker.open"]').click();
  let staleModal = page.locator(".root-picker-modal");
  await expect(staleModal.locator(".root-picker-title")).toHaveText(
    "Open workRoot on Remote fixture",
  );
  await expect(staleModal.locator(".root-picker-current")).toContainText(
    "Loading directories from Remote fixture",
  );
  await staleModal
    .locator('[data-command-id="rootPicker.close"]')
    .filter({ hasText: "Cancel" })
    .click();
  releaseFirstPicker?.();
  await page.waitForTimeout(100);

  await remoteRow.locator('[data-command-id="rootPicker.open"]').click();
  staleModal = page.locator(".root-picker-modal");
  await expect(staleModal.locator(".root-picker-title")).toHaveText(
    "Open workRoot on Remote fixture",
  );
  await expect(staleModal.locator(".root-picker-current")).toContainText(
    "/remote/fresh",
  );
  await expect(staleModal.locator(".root-picker-current")).not.toContainText(
    "/remote/stale",
  );
  await staleModal
    .locator('[data-command-id="rootPicker.close"]')
    .filter({ hasText: "Cancel" })
    .click();

  await page.unroute("**/api/dashboard/servers/server-remote/root-picker**");
  let releaseOpenRacePicker: ((value: void) => void) | null = null;
  let openRacePickerRequests = 0;
  await page.route(
    "**/api/dashboard/servers/server-remote/root-picker**",
    async (route) => {
      openRacePickerRequests += 1;
      if (openRacePickerRequests === 1) {
        await new Promise<void>((resolve) => {
          releaseOpenRacePicker = resolve;
        });
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify(
            linkedServerPickerView("/remote/stale-after-open"),
          ),
        });
        return;
      }
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(
          linkedServerPickerView("/remote/fresh-after-open"),
        ),
      });
    },
  );

  await remoteRow.locator('[data-command-id="rootPicker.open"]').click();
  staleModal = page.locator(".root-picker-modal");
  await expect(staleModal.locator(".root-picker-title")).toHaveText(
    "Open workRoot on Remote fixture",
  );
  await expect(staleModal.locator(".root-picker-current")).toContainText(
    "/remote/fresh",
  );
  await staleModal.locator(".root-picker-address").fill("/remote/open-race");
  await staleModal.locator(".root-picker-address").press("Enter");
  await expect.poll(() => openRacePickerRequests).toBe(1);
  await staleModal
    .locator("#root-picker-exact-path")
    .fill("/remote/opened-after-stale");
  const openWhileLoadingResponse = page.waitForResponse((response) => {
    const url = new URL(response.url());
    return (
      url.pathname === "/api/dashboard/servers/server-remote/work-roots/open" &&
      response.request().method() === "POST"
    );
  });
  await staleModal
    .locator('[data-command-id="workRoot.open"]')
    .filter({ hasText: "Open" })
    .click();
  await openWhileLoadingResponse;
  releaseOpenRacePicker?.();
  await expect(staleModal).toHaveCount(0);
  await page.waitForTimeout(100);

  await remoteRow.locator('[data-command-id="rootPicker.open"]').click();
  staleModal = page.locator(".root-picker-modal");
  await expect(staleModal.locator(".root-picker-title")).toHaveText(
    "Open workRoot on Remote fixture",
  );
  await expect(staleModal.locator(".root-picker-current")).toContainText(
    "/remote/fresh-after-open",
  );
  await expect(staleModal.locator(".root-picker-current")).not.toContainText(
    "/remote/stale-after-open",
  );
  await staleModal
    .locator('[data-command-id="rootPicker.close"]')
    .filter({ hasText: "Cancel" })
    .click();

  await page
    .locator(".server-row", { hasText: "Other remote" })
    .locator('[data-command-id="rootPicker.open"]')
    .click();
  staleModal = page.locator(".root-picker-modal");
  await expect(staleModal.locator(".root-picker-title")).toHaveText(
    "Open workRoot on Other remote",
  );
  await expect(staleModal.locator(".root-picker-current")).toContainText(
    "/other/home",
  );
  await expect(staleModal.locator(".root-picker-address")).toHaveValue(
    "/other/home",
  );
  await expect(staleModal.locator("#root-picker-create-name")).toHaveValue("");
});
