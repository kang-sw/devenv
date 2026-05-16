import { defineConfig } from "@playwright/test";

// Browser acceptance gate configuration for the dashboard workRoot UI.
//
// The gate exercises the daemon-served production frontend (the daemon serving
// `frontend/dist`) after owner pairing. It is intentionally separate from the
// pure-TypeScript `test:*` route tests: Vite builds and helper tests do not by
// themselves close UI-facing dashboard work.
export default defineConfig({
  testDir: "./e2e",
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
