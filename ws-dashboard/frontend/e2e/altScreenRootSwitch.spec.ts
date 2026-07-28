import { test, expect, type Page } from "@playwright/test";
import { fileURLToPath } from "node:url";
import { mkdtempSync, mkdirSync, writeFileSync, rmSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { startDaemon, type DaemonHandle } from "./daemonHarness.js";

// Regression test for a dogfood report: an alt-screen TUI app (vim/htop/
// tmux) went visibly garbled after switching away from its work root and
// back, only recovering on an explicit browser-window resize. Root cause
// (confirmed empirically via this same script before the fix): a workRoot
// switch toggles `display:none` on Dockview's entire per-root layout
// subtree, and the pane-visible corrective refit in `terminalPaneBody.tsx`
// could read `FitAddon.proposeDimensions()` mid-relayout, briefly getting a
// spuriously tiny size (observed: 10x3) that got forwarded to the daemon as
// a real resize before being corrected ~250ms later - vim received a
// genuine SIGWINCH for the bogus tiny size and never recovered its prior
// viewport on its own. Fixed by deferring the visibility-triggered refit
// through two animation frames and hardening `fitNow`'s degenerate-size
// guard to reject a simultaneous >4x collapse on both axes.

const here = path.dirname(fileURLToPath(import.meta.url));
const artifactsDir = path.join(here, ".artifacts");

function workRootDisplayName(rootPath: string) {
  const normalized = rootPath.replace(/[\\/]+$/, "");
  const match = normalized.match(/[^\\/]+$/);
  return match ? match[0] : normalized;
}

async function openWorkRootInBrowser(page: Page, rootPath: string) {
  const opener = page.locator(
    '[data-command-id="rootPicker.open"]:not(.open-work-root-empty-cta)',
  );
  await opener.click();
  const modal = page.locator(".root-picker-modal");
  await expect(modal).toBeVisible();
  await modal.locator("#root-picker-exact-path").fill(rootPath);
  await modal.locator('[data-command-id="workRoot.open"]').filter({ hasText: "Open" }).click();
  await expect(page.locator(".file-explorer-title")).toContainText(
    workRootDisplayName(rootPath),
  );
  await expect(modal).toHaveCount(0);
}

async function selectWorkRootInBrowser(page: Page, rootPath: string) {
  const label = workRootDisplayName(rootPath);
  const row = page.locator(
    '.resource-row[data-command-id="resource.select"]',
    { hasText: label },
  );
  await row.first().click();
  await expect(page.locator(".file-explorer-title")).toContainText(label);
}

test("alt-screen content after work-root round trip", async ({ page }) => {
  test.setTimeout(180_000);
  mkdirSync(artifactsDir, { recursive: true });

  const resizeFrames: string[] = [];
  page.on("websocket", (ws) => {
    if (ws.url().includes("/socket")) {
      ws.on("framesent", (frame) => {
        const text = String(frame.payload);
        if (text.includes('"resize"')) {
          resizeFrames.push(`${Date.now()} ${text}`);
        }
      });
    }
  });

  const workRootA = mktemp("root-a");
  const workRootB = mktemp("root-b");
  const stateHome = mkdtempSync(path.join(os.tmpdir(), "ws-dash-altscreen-state-"));
  const previousStateHome = process.env.WS_DASHBOARD_STATE_HOME;
  process.env.WS_DASHBOARD_STATE_HOME = stateHome;

  const daemon: DaemonHandle = await startDaemon();

  try {
    await page.setViewportSize({ width: 1440, height: 900 });
    // Force the DOM renderer (gpuAcceleration: false) so the emulator grid
    // is inspectable as real `.xterm-rows > div` DOM text instead of opaque
    // canvas/WebGL pixels - needed to assert on actual rendered content.
    await page.addInitScript(() => {
      window.localStorage.setItem(
        "ws-dashboard.settings.terminal.v1",
        JSON.stringify({
          version: 1,
          value: {
            fontFamilyOverride: "",
            fontSize: 12,
            themeBackground: "#0b0d10",
            gpuAcceleration: false,
            ligaturesEnabled: false,
          },
        }),
      );
    });
    await page.goto(daemon.pairingUrl, { waitUntil: "domcontentloaded" });
    await expect(page.locator(".app-shell")).toBeVisible();

    await openWorkRootInBrowser(page, workRootA);
    await page.locator('[data-command-id="terminal.create"]').click();
    const surfaceA = page.locator(".terminal-surface").first();
    await expect(surfaceA).toBeVisible();
    await expect(surfaceA.locator(".xterm")).toBeVisible();

    // Launch vim (full-screen alt-screen app) in root A's terminal.
    await surfaceA.click();
    await page.keyboard.type("vim -u NONE\n");
    await expect(surfaceA.locator(".xterm-rows")).toContainText("VIM -", {
      timeout: 10_000,
    });
    // Write a distinctive marker into the buffer via insert mode so the
    // screen has content that would visibly corrupt/garble if the emulator
    // grid desyncs from the PTY's real width/height.
    await page.keyboard.type("i");
    for (let i = 0; i < 60; i += 1) {
      await page.keyboard.type(`ALTLINE-${i}-0123456789abcdefghijklmnopqrstuvwxyz\n`);
    }
    await page.keyboard.press("Escape");
    await expect(surfaceA.locator(".xterm-rows")).toContainText("ALTLINE-59");

    await page.screenshot({
      path: path.join(artifactsDir, "repro-vim-before-switch.png"),
    });
    const beforeText = await surfaceA.locator(".xterm-rows").innerText();
    const beforeRows = await surfaceA.locator(".xterm-rows > div").count();
    console.log("rows(dom) before switch:", beforeRows);

    // Open a second, independent work root (already-open-root round trip,
    // matching the report: NOT first-open, a switch between two open roots).
    await openWorkRootInBrowser(page, workRootB);
    await page.waitForTimeout(300);

    // Switch back to root A - this is the exact "round trip" the report
    // describes. Give the 100ms focusWatchdog + Effect A + any debounce time
    // to settle before inspecting.
    await selectWorkRootInBrowser(page, workRootA);
    await page.waitForTimeout(1000);

    await page.screenshot({
      path: path.join(artifactsDir, "repro-vim-after-roundtrip.png"),
    });
    const afterText = await surfaceA.locator(".xterm-rows").innerText();
    const afterRows = await surfaceA.locator(".xterm-rows > div").count();
    console.log("rows(dom) after roundtrip (before ctrl-l):", afterRows);

    // The regression: vim's buffer collapsed to a handful of lines (a
    // spurious tiny resize reached the daemon and vim redrew for it) and
    // never recovered on its own once the correct size was restored.
    expect(afterRows).toBe(beforeRows);
    expect(afterText).toBe(beforeText);

    // Now force vim to actually redraw so any dimension-desync becomes
    // visible: request a full redraw (Ctrl-L is vim/less/tmux's own redraw
    // key) WITHOUT touching the browser window.
    await surfaceA.click();
    await page.keyboard.press("Control+L");
    await page.waitForTimeout(500);
    await page.screenshot({
      path: path.join(artifactsDir, "repro-vim-after-ctrl-l.png"),
    });
    const afterCtrlL = await surfaceA.locator(".xterm-rows").innerText();

    // Read the actual emulator geometry (DOM row count is authoritative for
    // the DOM renderer forced above) to correlate with any visual corruption.
    const rows = await surfaceA.locator(".xterm-rows > div").count();

    console.log("=== REPRO DIAGNOSTICS ===");
    console.log("rows(dom) after ctrl-l:", rows);
    console.log("beforeText === afterRoundtripText:", beforeText === afterText);
    console.log("afterRoundtripText === afterCtrlL:", afterText === afterCtrlL);
    console.log("--- beforeText ---\n" + beforeText.slice(0, 2000));
    console.log("--- afterRoundtripText ---\n" + afterText.slice(0, 2000));
    console.log("--- afterCtrlL ---\n" + afterCtrlL.slice(0, 2000));
    // Direct check on the root cause: no bogus, single-digit-column resize
    // frame should ever reach the daemon during the round trip (the exact
    // pattern that desynced vim's viewport pre-fix - observed as low as
    // 10x3 against a real 151x56 pane).
    const bogusFrames = resizeFrames.filter((frame) => {
      const match = frame.match(/"columns":(\d+),"rows":(\d+)/);
      if (!match) {
        return false;
      }
      return Number(match[1]) < 20 || Number(match[2]) < 10;
    });
    expect(bogusFrames).toEqual([]);

    console.log("--- resize frames sent (all panes, timestamped) ---");
    for (const frame of resizeFrames) {
      console.log(frame);
    }

    // Now do a genuine resize (what the user says "fixes it") and compare.
    await page.setViewportSize({ width: 1441, height: 900 });
    await page.waitForTimeout(500);
    await page.setViewportSize({ width: 1440, height: 900 });
    await page.waitForTimeout(500);
    await page.screenshot({
      path: path.join(artifactsDir, "repro-vim-after-explicit-resize.png"),
    });
    const afterResize = await surfaceA.locator(".xterm-rows").innerText();
    console.log("--- afterExplicitResize ---\n" + afterResize.slice(0, 2000));
  } finally {
    await daemon.stop();
    rmSync(workRootA, { recursive: true, force: true });
    rmSync(workRootB, { recursive: true, force: true });
    rmSync(stateHome, { recursive: true, force: true });
    if (previousStateHome === undefined) {
      delete process.env.WS_DASHBOARD_STATE_HOME;
    } else {
      process.env.WS_DASHBOARD_STATE_HOME = previousStateHome;
    }
  }
});

function mktemp(prefix: string): string {
  const dir = mkdtempSync(path.join(os.tmpdir(), `ws-dash-altscreen-${prefix}-`));
  writeFileSync(path.join(dir, "readme.txt"), "fixture\n");
  return dir;
}
