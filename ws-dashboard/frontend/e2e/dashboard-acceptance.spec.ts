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

const here = path.dirname(fileURLToPath(import.meta.url));
const artifactsDir = path.join(here, ".artifacts");

let daemon: DaemonHandle;
let workRoot: string;
let ownsWorkRoot = false;
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

async function expectDurableDockviewSplitDrop(page: Page) {
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
    .locator('.dockview-workbench-tab[data-workbench-pane-id^="readonly:"]')
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
  await expect(
    page.locator(`.dockview-workbench-tab[data-workbench-pane-id="${paneId}"]`),
  ).toHaveAttribute(
    "data-workbench-group-id",
    /group-[3-9][0-9]*|group-[1-9][0-9]+/,
  );
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
    await expect(page.locator("#open-work-root-path")).toBeVisible();
    await page.locator("#open-work-root-path").fill(workRoot);
    await page.locator('[data-command-id="workRoot.open"]').click();

    await expect(page.locator(".file-explorer-title")).toContainText(
      workRootDisplayName(workRoot),
    );
    await expectDockviewWorkbench(page);
    note(
      "open workRoot: live opened workRoot is selected and shown in the explorer",
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
  await test.step("open read-only file preview", async () => {
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
      "read-only",
    );
    await expect(page.locator(".workbench-pane-header")).toHaveCount(0);
    await expect(page.locator(".workbench-pane-status")).toHaveCount(0);
    await expectDurableDockviewSplitDrop(page);
    note(
      "read-only file: previewable file opens a read-only text pane with content and no generic pane chrome",
    );
  });

  // --- Create a terminal and verify emulator IO ---------------------------
  await test.step("create terminal and run a command", async () => {
    // CONTRACT: This step must prove the live terminal path attaches a
    // WebSocket, does not keep periodic output polling active while connected,
    // and preserves byte-stream input fidelity for Backspace, cursor movement,
    // shell history, Ctrl keys, paste, and prompt editing.
    // HINT: Intercept `/api/dashboard/terminals/*/output` and browser
    // WebSocket events around this block; use a real shell prompt rather than
    // fixture-only assertions.
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
    await page.keyboard.press(
      process.platform === "darwin" ? "Meta+A" : "Control+A",
    );
    await page.keyboard.type(commandPlan.echo("EDITED-OK"));
    await page.keyboard.press("Enter");
    await expect(page.locator(".xterm-rows")).toContainText("EDITED-OK");
    await page.keyboard.press("ArrowUp");
    await page.keyboard.press("Control+C");
    await page.keyboard.insertText("\f");
    await page.locator(".terminal-surface").click();
    await page.keyboard.insertText(commandPlan.echo("PASTE-OK"));
    await page.keyboard.press("Enter");
    await expect(page.locator(".xterm-rows")).toContainText("PASTE-OK");

    expect(
      terminalSocketFrames.some((frame) => frame.includes('"type":"input"')),
    ).toBe(true);
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
        "Ctrl-C, clear-screen control rendering/recovery, paste, and no document scroll",
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
    // workbench pane body, not a fixed constant: a partially filled terminal
    // must fail this regardless of viewport size.
    const paneBody = page.locator(
      '.workbench-pane[data-surface-kind="persistentTerminal"] .workbench-pane-body',
    );
    const bodyBox = await paneBody.boundingBox();
    const surfaceBox = await page.locator(".terminal-surface").boundingBox();
    expect(bodyBox).not.toBeNull();
    expect(surfaceBox).not.toBeNull();
    // The surface occupies the bulk of the pane body; the remainder is only
    // the thin terminal controls bar.
    expect(surfaceBox!.height).toBeGreaterThan(bodyBox!.height * 0.7);
    note(
      `pane fill: terminal surface ${Math.round(surfaceBox!.height)}px of ` +
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
  await test.step("close terminal terminates session", async () => {
    await page.locator(".terminal-surface").click();
    await page.keyboard.press("Control+D");
    await page.locator('[data-command-id="terminal.close"]').click();
    await expect(terminalTabs(page)).toHaveCount(1);
    note(
      "close: Ctrl-D was delivered safely before explicit terminate removed the tab; one terminal session remains",
    );
  });

  // --- Refresh keeps daemon-owned terminal, shows no mock surfaces --------
  await test.step("refresh without mock surfaces", async () => {
    await page.reload({ waitUntil: "domcontentloaded" });
    await expect(page.locator(".app-shell")).toBeVisible();
    await expect(page.locator(".file-explorer-title")).toContainText(
      workRootDisplayName(workRoot),
    );
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
