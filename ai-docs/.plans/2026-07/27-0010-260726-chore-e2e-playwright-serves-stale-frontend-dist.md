# Plan: 260726-chore-e2e-playwright-serves-stale-frontend-dist — Phase 1: Build the production bundle in Playwright globalSetup

## Relevant Ticket Contract

- Add `ws-dashboard/frontend/e2e/globalSetup.ts` and wire it into
  `playwright.config.ts` as `globalSetup`. It runs `npm run build` in
  `ws-dashboard/frontend` before any test, derived from the module's own
  location (not `process.cwd()`), through a portable spawn (`shell: true`),
  with a non-zero build exit propagating as a hard `globalSetup` failure.
- Skip the build — announcing the skip and reason on stdout, never silently —
  only when `WS_DASHBOARD_STATIC_DIR` is set or external daemon mode is
  selected (`WS_DASHBOARD_DAEMON_MODE=external` /
  `WS_DASHBOARD_DAEMON_BASE_URL` / `WS_DASHBOARD_DAEMON_PAIRING_URL`). No
  opt-out env var; `test:browser`'s existing `npm run build` stays untouched.
- File ownership boundary: this phase touches only `playwright.config.ts` and
  the new `e2e/globalSetup.ts`. It must not touch `e2e/daemonHarness.ts`'s
  `startDaemon`/stdio drain or `e2e/dashboard-acceptance.spec.ts`'s `afterAll`
  (owned by `260725-bug-dashboard-e2e-harness-destroys-daemon-diagnostics`),
  and must not write under `e2e/.artifacts/`.
- `e2e/globalSetup.ts` must not be named `*.test.ts`/`*.spec.ts` and does not
  need to enter `tsconfig.e2e-tests.json`'s `include` list (Playwright
  transpiles `globalSetup` itself).
- Spec Impact: update the one sentence at
  `ai-docs/spec/ws-web-dashboard/index.md:1684-1687` binding "the gate builds
  the production frontend" to the Playwright run rather than the npm script,
  and note the two skip conditions.
- Mental-model update on contact: the Traps entry in
  `ai-docs/mental-model/ws-web-dashboard/index.md` (the "Mutating
  `frontend/src`..." entry) must be updated, not deleted — superseded on the
  default spawn path, still applicable when a skip condition fires.
- Six-step proof procedure in the ticket's Phase 1 ("How a future session
  proves the guard actually works") is the verification boundary for this
  phase; it is manual/future-session evidence, not something this plan
  executes now.

## Out of Scope

- The daemon binary's identical `cargo build` gap (deliberately deferred per
  Constraints).
- Any freshness/staleness heuristic (rejected design; build is unconditional
  wherever it runs at all).
- `e2e/daemonHarness.ts::startDaemon`'s stdio drain and
  `e2e/dashboard-acceptance.spec.ts`'s `afterAll` teardown (owned by the
  adjacent diagnostics ticket).
- Any unit test for `globalSetup.ts` (out of scope unless a future node-runnable
  test is added; not requested here).
- A `WS_DASHBOARD_SKIP_BUILD`-style opt-out (explicitly rejected).

## Codebase Findings

- `ws-dashboard/frontend/playwright.config.ts:9-25` — `defineConfig` currently
  declares no `globalSetup`/`globalTeardown`/`webServer`; add
  `globalSetup: "./e2e/globalSetup.ts"` (Playwright resolves this path
  relative to the config file; confirmed via
  `node_modules/playwright/types/test.d.ts:1316-1323`, which documents
  `globalSetup?: string|Array<string>` requiring the module to export a single
  function).
- `ws-dashboard/frontend/e2e/daemonHarness.ts:5-7` — the exact idiom to mirror:
  `const here = path.dirname(fileURLToPath(import.meta.url)); const repoRoot =
  path.resolve(here, "..", "..")`. For `globalSetup.ts` (which lives in the
  same `e2e/` directory), the frontend dir is one level up, not two:
  `const frontendDir = path.resolve(here, "..")`.
- `ws-dashboard/frontend/e2e/daemonHarness.ts:91-123` —
  `parseDaemonHarnessConfig(env)` already implements the exact skip-condition
  branch logic this ticket needs and is exported: it returns
  `{ mode: "external", ... }` when `WS_DASHBOARD_DAEMON_MODE=external` or
  `WS_DASHBOARD_DAEMON_BASE_URL`/`WS_DASHBOARD_DAEMON_PAIRING_URL` is set, and
  `{ mode: "spawn", staticDir: env.WS_DASHBOARD_STATIC_DIR, ... }` otherwise.
  `globalSetup.ts` should call this directly (`import {
  parseDaemonHarnessConfig } from "./daemonHarness.js";` — `.js` suffix
  matches every existing intra-`e2e/` import, e.g.
  `dashboard-acceptance.spec.ts:7`, `agent-spawn-profile.spec.ts:5`) rather
  than re-deriving the two skip conditions from `process.env` a second time.
  Skip when `config.mode === "external"`, or when `config.mode === "spawn" &&
  config.staticDir !== undefined`.
- `ws-dashboard/frontend/e2e/daemonHarness.ts:125-129` —
  `dashboardBinaryName`'s win32 branch is the repo's existing precedent for
  portable-spawn/cross-platform handling this ticket's Constraints section
  points at; there is no existing `shell: true` / npm-portable-spawn call
  anywhere in the repo to reuse directly (confirmed via grep across
  `frontend/` and `crates/`), so the spawn wrapper in `globalSetup.ts` is new
  code, not an extraction.
- `ws-dashboard/frontend/e2e/daemonHarness.ts:41-56` — `scrubDiagnosticText` /
  `diagnosticCommand` exist specifically because this harness family has a
  standing rule (mental-model Common Mistakes,
  `ai-docs/mental-model/ws-web-dashboard/index.md:236`: "Letting daemon
  harness startup/readiness failures print raw command arguments, forwarded
  endpoints, pairing URLs, hostnames, or static-dir paths") against leaking
  raw paths/URLs into harness stdout/error text. **Risk signal**: the skip
  announcement this ticket requires must name *which* condition fired
  (static-dir override vs. external mode) without printing the actual
  `WS_DASHBOARD_STATIC_DIR` value or the external base/pairing URL, to stay
  consistent with that existing rule rather than accidentally reintroducing
  the class of leak this file was already hardened against.
- `ws-dashboard/frontend/package.json:27` — `test:browser` is `npm run build
  && (cd .. && cargo build -p ws-dashboard-daemon) && playwright test`; leave
  unchanged per Decisions ("keeps its leading `npm run build`").
- `ws-dashboard/frontend/tsconfig.e2e-tests.json:14-17` — `include` currently
  lists only `e2e/daemonHarness.ts` and `e2e/daemonHarness.test.ts`; per
  Constraints, do not add `globalSetup.ts` to this list (no unit test is being
  added in this phase).
- `ws-dashboard/frontend/e2e/.gitignore:1` — only `.artifacts/` is ignored
  under `e2e/`; no ignore-list change needed since `globalSetup.ts` is a
  tracked source file, not generated output.
- `ai-docs/spec/ws-web-dashboard/index.md:1684-1687` — exact sentence to edit:
  "The frontend package exposes this gate through `npm run test:browser`. The
  gate builds the production frontend, serves it through the dashboard
  daemon, pairs as owner through the startup pairing URL, and records textual
  evidence plus regenerable screenshot artifacts outside tracked source."
- `ai-docs/mental-model/ws-web-dashboard/index.md:247` — the Traps entry
  ("Mutating `frontend/src` to prove a browser assertion is non-vacuous and
  then running Playwright without rebuilding first...") is the exact entry the
  ticket's Spec Impact section requires updating in place.

## Implementation Plan

1. Create `ws-dashboard/frontend/e2e/globalSetup.ts`:
   - Derive `frontendDir` via `fileURLToPath(import.meta.url)` +
     `path.resolve(here, "..")`, mirroring `daemonHarness.ts:5-7`'s idiom (one
     level up, since this file already lives in `frontend/e2e`).
   - Call `parseDaemonHarnessConfig()` (imported from `./daemonHarness.js`) to
     get the same mode/staticDir decision `startDaemon` will use.
   - If `config.mode === "external"`: `console.log` a stdout line stating
     external daemon mode was selected and the build is skipped, then return
     without spawning anything.
   - Else if `config.staticDir !== undefined`: `console.log` a stdout line
     stating a custom static dir was supplied (env var name only, not the
     path value) and the build is skipped, then return.
   - Else: spawn `npm run build` with `cwd: frontendDir`, `shell: true`,
     `stdio: "inherit"` (so build output/errors surface directly in the
     Playwright run), wrapped in a `Promise` that resolves on `close` code `0`
     and rejects (propagating out of the exported `globalSetup` function) on
     any non-zero code or `error` event — matching the "failing build must
     fail the run" constraint and the "portable spawn" constraint together.
   - Export the function as `export default async function globalSetup()`
     (Playwright's required single-function-export shape per
     `node_modules/playwright/types/test.d.ts:1323`).
2. Edit `ws-dashboard/frontend/playwright.config.ts`: add
   `globalSetup: "./e2e/globalSetup.ts"` inside the `defineConfig({...})`
   object (near the existing `testDir` line, before `use`).
3. Edit `ai-docs/spec/ws-web-dashboard/index.md:1684-1687`: rephrase so the
   build-before-serve property is stated as belonging to the Playwright run
   itself (via `globalSetup`), not to the `test:browser` npm script, and add
   the two documented skip conditions (`WS_DASHBOARD_STATIC_DIR` set; external
   daemon mode selected) so the spec does not overclaim unconditional building.
4. Edit `ai-docs/mental-model/ws-web-dashboard/index.md:247`: update the Traps
   entry text so it states the "always rebuild manually" discipline is now
   superseded on the default spawn invocation path (Playwright `globalSetup`
   builds unconditionally), but still fully applies whenever
   `WS_DASHBOARD_STATIC_DIR` or external mode makes `globalSetup` skip the
   build. Do not delete the entry — its mechanism explanation (why staleness is
   silent, why `*.spec.ts` is never stale) remains correct background for the
   skip-path case.

## Verification Plan

- No automated test is added in this phase (Constraints: no unit test unless
  a future node-runnable test is written for `globalSetup.ts`).
- Manual verification is the ticket's own six-step proof procedure (Phase 1,
  "How a future session proves the guard actually works"): mutate a
  `frontend/src` file to break one existing e2e assertion, run `npx playwright
  test e2e/<that>.spec.ts` from `ws-dashboard/frontend` without a manual
  `npm run build`, and confirm the run now fails on the mutation (previously a
  false pass); revert and confirm green again; then run once with
  `WS_DASHBOARD_STATIC_DIR` set and once with `WS_DASHBOARD_DAEMON_MODE=external`
  alone, confirming the skip line and reason appear on stdout in both cases
  (the external-mode run is expected to end red via `startDaemon`'s own
  "external daemon mode requires ..." error, `daemonHarness.ts:199-201`, after
  the skip line has already been emitted — a red run there is not a guard
  failure).
- Record the measured `globalSetup` build overhead on a warm tree in the
  Phase 1 Result, per the ticket's own step 6, so the ~2.4 s premise is
  re-checkable later rather than inherited as folklore.

## Escalations

- None.
