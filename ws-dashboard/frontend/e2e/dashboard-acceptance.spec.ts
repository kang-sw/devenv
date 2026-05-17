import { test, expect, type Page } from "@playwright/test";
import { fileURLToPath } from "node:url";
import { mkdtempSync, mkdirSync, writeFileSync, rmSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { startDaemon, type DaemonHandle } from "./daemonHarness.js";
import {
  terminalCommandPlanForPlatform,
  type TerminalCommandPlan,
} from "../src/terminalCommandPlan.js";
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
let ownsWorkRoot = false;
let ownsSecondWorkRoot = false;
let commandPlan: TerminalCommandPlan;
let portabilityEvidence: TerminalPortabilityEvidence | undefined;

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

function terminalTabs(page: Page) {
  return page.getByRole("tab").filter({ hasText: "Terminal" });
}

async function expectDockviewWorkbench(page: Page) {
  const owner = page.locator('[data-workbench-layout-owner="dockview"]');
  await expect(owner).toBeVisible();
  // CONTRACT: The visible workbench must be backed by Dockview, not the retired
  // custom `.workbench-splits > .workbench-group` tab/split shell.
  await expect(owner.locator(".dv-dockview")).toBeVisible();
  await expect(
    page.locator(".workbench-splits > .workbench-group"),
  ).toHaveCount(0);
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
    .locator(".dockview-workbench-tab")
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

async function openWorkRootInBrowser(page: Page, rootPath: string) {
  await expect(page.locator("#open-work-root-path")).toBeVisible();
  await page.locator("#open-work-root-path").fill(rootPath);
  await page.locator('[data-command-id="workRoot.open"]').click();
  await expect(page.locator(".file-explorer-title")).toContainText(
    workRootDisplayName(rootPath),
  );
  await expectDockviewWorkbench(page);
}

async function selectWorkRootInBrowser(page: Page, rootPath: string) {
  await page
    .locator('.resource-row[data-command-id="resource.select"]', {
      has: page.locator(".row-eyebrow", { hasText: "workRoot" }),
      hasText: workRootDisplayName(rootPath),
    })
    .click();
  await expect(page.locator(".file-explorer-title")).toContainText(
    workRootDisplayName(rootPath),
  );
  await expectDockviewWorkbench(page);
}

// The terminal pane footer renders `<status> · <columns>x<rows>` from the
// daemon-confirmed session size, so it reflects forwarded PTY resizes.
async function terminalColumns(page: Page): Promise<number> {
  const text =
    (await page.locator(".terminal-status-line").first().textContent()) ?? "";
  const match = text.match(/(\d+)x(\d+)/i);
  return match ? Number(match[1]) : Number.NaN;
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

test("dashboard workRoot UI browser acceptance", async ({ page }) => {
  const terminalSocketUrls: string[] = [];
  const terminalSocketFrames: string[] = [];
  let terminalOutputPolls = 0;
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
  });
  // --- Owner pairing against the daemon-served production frontend ---------
  await test.step("owner pairing", async () => {
    await page.goto(daemon.pairingUrl, { waitUntil: "domcontentloaded" });
    await expect(page.locator(".app-shell")).toBeVisible();
    expect(new URL(page.url()).pathname).not.toContain("/pair");
    note("pairing: one-time pairing URL installed owner cookie and left /pair");
  });

  // --- Open a real workRoot through the raw path opener -------------------
  await test.step("open real workRoot", async () => {
    await openWorkRootInBrowser(page, workRoot);
    note(
      "open workRoot: live opened workRoot is selected and shown in the explorer",
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
  await test.step("agent tab close confirmation when a live agent tab exists", async () => {
    const agentTab = page.locator(
      '.dockview-workbench-tab[data-workbench-close-confirmation="confirmSessionClose"]',
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

    const pane = page.locator(".readonly-text-pane");
    await expect(pane).toBeVisible();
    await expect(pane.locator(".readonly-text-content")).toContainText(
      "ws-dashboard browser gate fixture",
    );
    await expectDockviewWorkbench(page);
    await expect(pane.locator(".readonly-text-pane-badges")).toContainText(
      "preview",
    );
    const previewTab = page.locator(
      '.dockview-workbench-tab[data-workbench-pane-id^="readonly-preview:"]',
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
    await expect(pane.locator(".readonly-text-pane-title")).toContainText(
      "gate-bulk-000.txt",
    );
    await expect(pane.locator(".readonly-text-content")).toContainText(
      "bulk gate fixture 0",
    );

    await previewTab.hover();
    await previewClose.click();
    await expect(page.locator(".readonly-text-pane")).toHaveCount(0);

    await fileRow.click();
    await expect(previewTab).toBeVisible();
    await fileRow.dblclick();
    const pinnedTab = page.locator(
      '.dockview-workbench-tab[data-workbench-pane-id^="readonly:"]',
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
    note(
      "read-only file: single click opened a replaceable preview, hover-only close immediately removed it, and double click pinned the file in the opened file group",
    );
  });

  await test.step("long read-only file scroll stays inside the pane", async () => {
    // CONTRACT: Long read-only file content must own its scroll container.
    // Scrolling over `.readonly-text-content` must not move the top-level
    // browser document, displace dashboard chrome, or depend on a future
    // editor-library replacement.
    const longFileRow = page.locator(".file-explorer-row", {
      hasText: "gate-long-readonly.txt",
    });
    await longFileRow.click();
    await expectDockviewWorkbench(page);

    const content = page.locator(".readonly-text-content");
    await expect(content).toContainText(
      "readonly scroll containment line 220",
    );
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
    expect(await documentScrolls(page)).toBe(beforeDocumentScroll);
    expect(beforeDocumentScroll).toBe(false);
    note(
      "read-only file: long file scroll stayed inside the pane without creating top-level document scroll",
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

    const secondFileRow = page.locator(".file-explorer-row", {
      hasText: "second-readme.txt",
    });
    await expect(secondFileRow).toHaveAttribute(
      "data-command-id",
      "fileExplorer.openFile",
    );
    await secondFileRow.click();
    await expect(page.locator(".readonly-text-pane")).toContainText(
      "second ws-dashboard browser gate fixture",
    );
    await expect(
      page.locator(
        '.dockview-workbench-tab[data-workbench-pane-id^="readonly-preview:"]',
      ),
    ).toHaveAttribute("data-workbench-group-id", "group-2");
    expect(await visibleWorkbenchGroupIds(page)).toEqual(["group-1", "group-2"]);

    await selectWorkRootInBrowser(page, workRoot);
    const pinnedReadOnlyTab = page.locator(
      '.dockview-workbench-tab[data-workbench-pane-id^="readonly:"]',
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
    await expect(page.locator(".readonly-text-pane")).toContainText(
      "ws-dashboard browser gate fixture",
    );
    note(
      "dynamic groups: opened-file group placement stayed scoped per workRoot and did not auto-target user-created groups",
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

    await inputTarget.focus();
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
        '.dockview-workbench-tab[data-workbench-close-confirmation="confirmSessionClose"]',
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
    await page.screenshot({
      path: path.join(artifactsDir, "desktop-workbench.png"),
      fullPage: true,
    });
    note(
      "refresh: daemon-owned terminal reconstructs as a selectable tab after reload, no mock surfaces",
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

  if (portabilityEvidence) {
    portabilityEvidence.browserGate.result = "pass";
  }
});
