import { defineConfig } from "@playwright/test";

// Browser acceptance gate configuration for the dashboard workRoot UI.
//
// The gate exercises the daemon-served production frontend (the daemon serving
// `frontend/dist`) after owner pairing. It is intentionally separate from the
// pure-TypeScript `test:*` route tests: Vite builds and helper tests do not by
// themselves close UI-facing dashboard work.
export default defineConfig({
  testDir: "./e2e",
  // Builds the production frontend before any test starts, so every
  // invocation path - `npm run test:browser`, a bare `npx playwright test`, a
  // single-spec run, an IDE runner - serves the current `frontend/src` rather
  // than whatever bundle happens to sit in `dist/`. It skips the build, saying
  // so on stdout, only where the harness does not construct the served
  // directory itself (`WS_DASHBOARD_STATIC_DIR`, or external daemon mode).
  globalSetup: "./e2e/globalSetup.ts",
  fullyParallel: false,
  workers: 1,
  retries: 0,
  // The gate boots a daemon, opens a real workRoot, and drives PTY terminals,
  // so the per-test budget is generous.
  timeout: 180_000,
  expect: { timeout: 20_000 },
  reporter: [["list"]],
  use: {
    headless: true,
    viewport: { width: 1440, height: 900 },
    screenshot: "only-on-failure",
    trace: "retain-on-failure",
  },
});
