import { spawn } from "node:child_process";
import { fileURLToPath } from "node:url";
import path from "node:path";
import { parseDaemonHarnessConfig } from "./daemonHarness.js";

const here = path.dirname(fileURLToPath(import.meta.url));
// frontend/e2e -> frontend. Derived from this module's own location rather
// than `process.cwd()`, because Playwright runs `globalSetup` with whatever
// cwd the runner was launched from and the whole point of this file is that
// invocation paths vary (`npm run test:browser`, a bare `npx playwright test`,
// a single-spec run, an IDE runner started at the repo root).
const frontendDir = path.resolve(here, "..");

const LOG_PREFIX = "[e2e globalSetup]";

/**
 * Run `npm run build` in `ws-dashboard/frontend`, resolving only on exit code
 * 0. `shell: true` is required for portability: on native Windows `npm`
 * resolves to `npm.cmd`, and `child_process.spawn` does not consult
 * `PATHEXT`, so a bare `spawn("npm", ...)` throws `ENOENT` there. Combined
 * with the hard-fail contract below, an unguarded spawn would kill every
 * Windows run inside `globalSetup` before a single test could execute.
 * (Spawning `npm.cmd` explicitly instead is not an alternative: Node blocks
 * direct `.cmd`/`.bat` spawns without a shell since the CVE-2024-27980 fix.)
 * The command is passed as one string rather than as an argv array so Node
 * does not emit its DEP0190 "args are concatenated, not escaped" warning into
 * every run; the string is a fixed literal with no interpolated input.
 */
function runFrontendBuild(): Promise<void> {
  return new Promise<void>((resolve, reject) => {
    const child = spawn("npm run build", {
      cwd: frontendDir,
      // Build output and diagnostics go straight to the Playwright run's own
      // streams; a failing build must be readable without extra plumbing.
      stdio: "inherit",
      shell: true,
      windowsHide: true,
    });
    child.once("error", (error) => {
      const detail = error instanceof Error ? `: ${error.message}` : "";
      reject(new Error(`${LOG_PREFIX} could not start the frontend build${detail}`));
    });
    child.once("close", (code, signal) => {
      if (code === 0) {
        resolve();
        return;
      }
      reject(
        new Error(
          `${LOG_PREFIX} frontend build failed (code ${code}, signal ${signal}); the run is stopped rather than served the previous bundle`,
        ),
      );
    });
  });
}

/**
 * Build the production frontend before any test starts, so no Playwright
 * invocation path can point the daemon at a stale `frontend/dist`.
 *
 * The build is unconditional on the path where the harness constructs the
 * static dir itself. It is skipped only where `frontend/dist` is not that
 * constructed static dir - a custom `WS_DASHBOARD_STATIC_DIR`, or external
 * daemon mode, both read through `parseDaemonHarnessConfig` so this file and
 * `startDaemon` cannot drift apart on which branch is taken. Neither skip
 * condition proves `frontend/dist` is unused (`WS_DASHBOARD_STATIC_DIR` may
 * point at exactly that directory, and an external daemon may have been
 * started against it); the skip is a mechanical trigger on env shape, not a
 * freshness proof, so it announces itself and hands those two paths back to
 * the manual-rebuild discipline.
 *
 * Skip announcements name which condition fired without echoing the
 * `WS_DASHBOARD_STATIC_DIR` value or the external base/pairing URL, matching
 * this harness family's existing `scrubDiagnosticText` rule against leaking
 * static-dir paths and endpoints into harness output.
 */
export default async function globalSetup(): Promise<void> {
  const config = parseDaemonHarnessConfig();

  if (config.mode === "external") {
    console.log(
      `${LOG_PREFIX} skipping the frontend build: external daemon mode is selected (WS_DASHBOARD_DAEMON_MODE / WS_DASHBOARD_DAEMON_BASE_URL / WS_DASHBOARD_DAEMON_PAIRING_URL), so this run does not build the static dir it serves. Rebuild manually if that daemon serves a build of this tree.`,
    );
    return;
  }

  if (config.staticDir !== undefined) {
    console.log(
      `${LOG_PREFIX} skipping the frontend build: WS_DASHBOARD_STATIC_DIR overrides the harness-constructed static dir. Rebuild manually if that directory is a build of this tree.`,
    );
    return;
  }

  console.log(`${LOG_PREFIX} building the production frontend before the daemon is pointed at frontend/dist`);
  const startedAt = Date.now();
  await runFrontendBuild();
  const elapsedSeconds = ((Date.now() - startedAt) / 1000).toFixed(2);
  console.log(`${LOG_PREFIX} frontend build finished in ${elapsedSeconds}s`);
}
