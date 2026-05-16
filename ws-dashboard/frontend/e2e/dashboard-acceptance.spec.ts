import { test, expect, type Page } from "@playwright/test";
import { fileURLToPath } from "node:url";
import { mkdtempSync, mkdirSync, writeFileSync, rmSync, appendFileSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { startDaemon, type DaemonHandle } from "./daemonHarness.js";

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

const evidence: string[] = [];
function note(line: string) {
  evidence.push(line);
}

test.describe.configure({ mode: "serial" });

test.beforeAll(async () => {
  mkdirSync(artifactsDir, { recursive: true });

  // A deterministic temporary workRoot keeps explorer assertions stable.
  workRoot = mkdtempSync(path.join(os.tmpdir(), "ws-dash-gate-"));
  writeFileSync(
    path.join(workRoot, "gate-readme.txt"),
    "ws-dashboard browser gate fixture\nsecond fixture line\n",
  );
  mkdirSync(path.join(workRoot, "gate-subdir"));
  writeFileSync(path.join(workRoot, "gate-subdir", "nested.txt"), "nested gate file\n");

  daemon = await startDaemon();
  note(`daemon base URL: ${daemon.baseUrl}`);
  note(`temp workRoot: ${path.basename(workRoot)}`);
});

test.afterAll(async () => {
  if (daemon) {
    await daemon.stop();
  }
  if (workRoot) {
    rmSync(workRoot, { recursive: true, force: true });
  }
  writeFileSync(path.join(artifactsDir, "evidence.txt"), `${evidence.join("\n")}\n`);
});

async function terminalSurface(page: Page) {
  const surface = page.locator(".terminal-surface");
  await expect(surface).toBeVisible();
  await expect(surface.locator(".xterm")).toBeVisible();
  return surface;
}

async function runInTerminal(page: Page, command: string) {
  await page.locator(".terminal-surface").click();
  await page.keyboard.type(command);
  await page.keyboard.press("Enter");
}

function terminalTabs(page: Page) {
  return page.getByRole("tab").filter({ hasText: "Terminal" });
}

// The terminal pane footer renders `<status> · <columns>x<rows>` from the
// daemon-confirmed session size, so it reflects forwarded PTY resizes.
async function terminalColumns(page: Page): Promise<number> {
  const text = (await page.locator(".terminal-status-line").first().textContent()) ?? "";
  const match = text.match(/(\d+)x(\d+)/i);
  return match ? Number(match[1]) : Number.NaN;
}

// One full output-poll cycle is 500ms; settling past it proves a tab
// selection survives the poll-driven `editorGroups` rebuild.
async function settlePastPollCycle(page: Page) {
  await page.waitForTimeout(900);
}

test("dashboard workRoot UI browser acceptance", async ({ page }) => {
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
      path.basename(workRoot),
    );
    note("open workRoot: live opened workRoot is selected and shown in the explorer");
  });

  // --- No mock/placeholder terminal in the freshly opened workbench -------
  await test.step("non-mock initial workbench state", async () => {
    await expect(terminalTabs(page)).toHaveCount(0);
    await expect(page.locator(".terminal-surface")).toHaveCount(0);
    note("initial state: opened workRoot shows no mock or placeholder terminal surface");
  });

  // --- Conventional read-only file explorer -------------------------------
  await test.step("file explorer expansion and refresh", async () => {
    const dirRow = page.locator(".file-explorer-row", { hasText: "gate-subdir" });
    await expect(dirRow).toBeVisible();
    await expect(dirRow).toHaveClass(/file-explorer-row-directory/);
    await expect(dirRow).toHaveAttribute("data-command-id", "fileExplorer.toggleDirectory");
    await expect(dirRow).toHaveAttribute("aria-expanded", "false");

    await dirRow.click();
    await expect(dirRow).toHaveAttribute("aria-expanded", "true");
    await expect(page.locator(".file-explorer-row", { hasText: "nested.txt" })).toBeVisible();

    await page.locator('[data-command-id="fileExplorer.refresh"]').click();
    await expect(page.locator(".file-explorer-row", { hasText: "gate-readme.txt" })).toBeVisible();
    await page.screenshot({ path: path.join(artifactsDir, "file-explorer.png") });
    note("file explorer: directory rows expand on row click and refresh keeps entries visible");
  });

  // --- Open a previewable read-only file ----------------------------------
  await test.step("open read-only file preview", async () => {
    const fileRow = page.locator(".file-explorer-row", { hasText: "gate-readme.txt" });
    await expect(fileRow).toHaveAttribute("data-command-id", "fileExplorer.openFile");
    await fileRow.click();

    const pane = page.locator(".readonly-text-pane");
    await expect(pane).toBeVisible();
    await expect(pane.locator(".readonly-text-content")).toContainText(
      "ws-dashboard browser gate fixture",
    );
    await expect(pane.locator(".readonly-text-pane-badges")).toContainText("read-only");
    note("read-only file: previewable file opens a read-only text pane with content");
  });

  // --- Create a terminal and verify emulator IO ---------------------------
  await test.step("create terminal and run a command", async () => {
    await page.locator('[data-command-id="terminal.create"]').click();
    await terminalSurface(page);
    await expect(terminalTabs(page)).toHaveCount(1);

    await runInTerminal(page, "printf 'GATEOUT-%s\\n' 12345");
    await expect(page.locator(".xterm-rows")).toContainText("GATEOUT-12345");
    note("terminal IO: keyboard input reached the daemon PTY and output rendered in the emulator");
  });

  // --- ANSI color rendering -----------------------------------------------
  await test.step("ANSI color rendering", async () => {
    await runInTerminal(page, "printf '\\033[32mGATE-GREEN\\033[0m\\n'");
    // The output is rendered in a green (palette index 2) emulator span. A
    // plain text check is intentionally avoided here: the PTY-echoed input
    // line also contains `GATE-GREEN`, so only the colored span proves the SGR
    // sequence was interpreted rather than printed raw.
    await expect(
      page.locator(".xterm-rows span.xterm-fg-2", { hasText: "GATE-GREEN" }),
    ).toBeVisible();
    await page.screenshot({ path: path.join(artifactsDir, "terminal-emulator.png") });
    note("ANSI: SGR color sequence rendered as terminal color (xterm-fg-2), not raw text");
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
    await runInTerminal(page, "printf 'SECOND-%s\\n' MARKER");
    await expect(page.locator(".xterm-rows")).toContainText("SECOND-MARKER");

    // Switch back to the first terminal tab and settle past a full output
    // poll cycle: the selection must stick. Without the focus guard the
    // poll-driven editorGroups rebuild steals focus back to the most-recently
    // created terminal, so this fails first if the guard regresses.
    await terminalTabs(page).nth(0).click();
    await settlePastPollCycle(page);
    await terminalSurface(page);
    await expect(page.locator(".xterm-rows")).toContainText("GATEOUT-12345");
    await expect(page.locator(".xterm-rows")).not.toContainText("SECOND-MARKER");

    // Switch to the second terminal tab; it shows only its own output.
    await terminalTabs(page).nth(1).click();
    await settlePastPollCycle(page);
    await terminalSurface(page);
    await expect(page.locator(".xterm-rows")).toContainText("SECOND-MARKER");
    await expect(page.locator(".xterm-rows")).not.toContainText("GATEOUT-12345");
    note("tab selection: tab focus survives a poll cycle; input/output stay isolated per session");
  });

  // --- Close terminates the session ---------------------------------------
  await test.step("close terminal terminates session", async () => {
    await page.locator('[data-command-id="terminal.close"]').click();
    await expect(terminalTabs(page)).toHaveCount(1);
    note("close: terminating a terminal removes its tab; one terminal session remains");
  });

  // --- Refresh keeps daemon-owned terminal, shows no mock surfaces --------
  await test.step("refresh without mock surfaces", async () => {
    await page.reload({ waitUntil: "domcontentloaded" });
    await expect(page.locator(".app-shell")).toBeVisible();
    await expect(page.locator(".file-explorer-title")).toContainText(path.basename(workRoot));
    // The daemon owns the terminal lifecycle, so the surviving session is
    // reconstructed as a selectable tab after reload.
    await expect(terminalTabs(page)).toHaveCount(1);
    await terminalTabs(page).nth(0).click();
    await terminalSurface(page);
    await page.screenshot({ path: path.join(artifactsDir, "desktop-workbench.png"), fullPage: true });
    note("refresh: daemon-owned terminal reconstructs as a selectable tab after reload, no mock surfaces");
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

    await page.screenshot({ path: path.join(artifactsDir, "narrow-workbench.png"), fullPage: true });
    note(
      `narrow viewport: 480px relayout refit the PTY below ${wideColumns} columns ` +
        "via bounded resize forwarding",
    );
    await page.setViewportSize({ width: 1440, height: 900 });
  });
});
