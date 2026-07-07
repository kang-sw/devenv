# Plan: 260707-bug-dashboard-e2e-second-test-pairing-token-reuse — Phase 1: Fix the e2e harness so a second cookie-scoped test can reuse the paired session

## Relevant Ticket Contract

- Give the second test (and any future test needing an authenticated session)
  a way to reach the app without re-consuming the daemon's one-time pairing
  token.
- Do not weaken or change the daemon's one-time-pairing-token security
  behavior (`auth.rs`) — this is a test-harness-only fix.
- Ticket steers toward approach (a): capture the session cookie/auth state
  after the first test's pairing and inject it into the second test's context
  via `context.addCookies()` / `storageState`, over approach (b) (reusing the
  first test's page/context, rejected because it would carry over
  opened-workRoot/terminal state that could collide with the second test's
  `page.route(...)` mocks), but says "confirm after inspecting the actual
  session/cookie mechanism" — this survey does that confirmation below.
- Verification: run the full `e2e/dashboard-acceptance.spec.ts` file (both
  tests) at least twice consecutively and confirm both pass every time.

## Out of Scope

- Any change to `ws-dashboard/crates/daemon/src/auth.rs` (pairing token
  one-time-use semantics, `consume_pairing_token`, `pairing_consumed` flag) —
  explicitly out of scope per ticket.
- Any change to `router.rs` route/middleware behavior.
- The unrelated `260707-bug-dashboard-e2e-multi-root-locator-leakage` and
  `260707-bug-dashboard-terminal-clears-on-tab-switch` tickets.

## Codebase Findings

- `ws-dashboard/crates/daemon/src/auth.rs#L8` and `#L216-221` — cookie is
  named `ws-dashboard-owner`, value is a fixed per-daemon-instance
  `session_token` (constant for the daemon's whole lifetime, not
  re-generated per pairing). `as_set_cookie_header()` produces
  `ws-dashboard-owner=<token>; Path=/; HttpOnly; SameSite=Lax` — no `Domain`,
  no `Secure`, no `Expires`/`Max-Age` (session cookie). `HttpOnly` does not
  block Playwright's `context.addCookies()`/`context.cookies()` — those
  operate on the browser's cookie jar via CDP, not `document.cookie`.
- `ws-dashboard/crates/daemon/src/auth.rs#L123-146` (`authenticate_headers`) —
  cookie auth is accepted whenever `pairing_consumed == true` AND the
  request cookie matches the session token; no bearer token needed. Because
  `pairing_consumed` is a single flag on the shared `OwnerAuthState` for the
  one daemon instance (spawned once in `beforeAll` for the whole spec file),
  once test 1 consumes the pairing token, `pairing_consumed` stays `true` for
  the rest of the daemon's life — test 2 does not need to pair again, it
  only needs the matching cookie in its own context.
- `ws-dashboard/crates/daemon/src/router.rs#L339-346` (route `GET /`,
  wrapped by `require_owner_auth` via `from_fn_with_state` when
  `owner_auth_enabled`) and `#L387-408` (`require_owner_auth` calling
  `authenticate_browser_entrypoint`) — confirms `GET /` (the app shell) is
  reachable directly with just the cookie, no need to hit `/pair` again. On
  auth failure it returns a bare `401`/`403` status with no body/redirect
  (no `/pair` fallback to rely on).
- `ws-dashboard/crates/daemon/src/auth.rs#L274-290` (`entrypoint_headers_allowed`,
  `header_values_allowed`) — `Iterator::all` on zero matching headers is
  vacuously `true`, so a `page.goto()` top-level navigation with no `Origin`
  header (typical Playwright behavior) is not blocked by the Origin check;
  only the `Host` header is actually enforced (must be loopback/`localhost`,
  which `daemon.baseUrl` already satisfies since the harness binds
  loopback).
- `ws-dashboard/crates/daemon/src/router.rs#L349,355-374` (`GET /pair`
  handler) — on `PairingOutcome::Paired`, returns `303 See Other` with
  `Set-Cookie: <cookie>` and `Location: /`. This confirms the cookie is set
  by a real navigation response, observable/capturable via Playwright's
  `page.context().cookies()` after test 1's existing pairing step — there is
  no other API that exposes the raw session token value directly.
- `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L797` (test 1
  signature, destructures only `{ page }`) and `#L826-829` (existing
  "owner pairing" step: `page.goto(daemon.pairingUrl, ...)` then
  `expect(page.locator(".app-shell")).toBeVisible()`) — this is where the
  cookie becomes available in test 1's context after a successful pairing.
- `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L2929-2931` (test 2
  signature, destructures only `{ page }`) and `#L3041-3042` (currently
  re-navigates to the same consumed `daemon.pairingUrl`, then asserts
  `.app-shell` visible) — this is the failing call site; test 2's
  `page.route(...)` mocks are registered at `#L2936-3039`, all before this
  navigation, so switching the navigation target does not disturb the mock
  setup.
- `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L30-44` — module-level
  `let` state (e.g. `daemon`, `workRoot`, ...) shared across tests in this
  serial-mode file; the plan adds one more module-level variable
  (`ownerCookies`) following this existing pattern.
- `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L51` —
  `test.describe.configure({ mode: "serial" })` already guarantees test 1
  runs before test 2 in file order, which the cookie-capture-then-reuse
  approach depends on.
- `ws-dashboard/frontend/e2e/daemonHarness.ts#L30-38` (`DaemonHandle` type) —
  exposes `baseUrl: string` and `pairingUrl: string`; `baseUrl` is already
  used elsewhere in the spec (e.g. `dashboard-acceptance.spec.ts#L799`,
  `origin: daemon.baseUrl`), so it is directly reusable as the `url` for
  `context.addCookies()` and as the navigation target for test 2 (instead of
  the now-consumed `pairingUrl`).
- Repo-wide `grep` for `addCookies`, `storageState`, `context.cookies(` in
  `ws-dashboard/frontend/e2e/` found no existing precedent — this is a new
  pattern for the suite, but Playwright's `BrowserContext.cookies()` /
  `BrowserContext.addCookies()` are stable public APIs, no library gap.

## Implementation Plan

1. In `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts`, add a
   module-level variable near the other shared `let` state (after
   `#L44`, alongside `daemon`, `workRoot`, etc.):
   `let ownerCookies: Awaited<ReturnType<import("@playwright/test").BrowserContext["cookies"]>> | undefined;`
   (or a locally-typed `Cookie[] | undefined` — reuse whatever cookie type
   Playwright exports, e.g. `import type { Cookie } from "@playwright/test"`
   if available, otherwise inline the array type from `context.cookies()`'s
   return type).
2. In test 1 (`dashboard workRoot UI browser acceptance`,
   `#L797`), immediately after the existing pairing assertions inside the
   `"owner pairing"` step (after `#L829`), add:
   `ownerCookies = await page.context().cookies(daemon.baseUrl);`
   This captures the `ws-dashboard-owner` cookie (plus any others) set by
   the real `/pair` redirect response, scoped to the daemon's origin.
3. In test 2 (`linked server root picker uses server-scoped local gateway
   routes`, `#L2929-2931`), replace the re-pairing navigation at
   `#L3041-3042`:
   - Before navigating, assert `ownerCookies` is defined (fail fast with a
     clear message if test 1 didn't run/capture it — defensive, since
     serial-mode ordering is relied on).
   - Call `await page.context().addCookies(ownerCookies);` to inject the
     captured session cookie into test 2's fresh, isolated context.
   - Change the navigation target from `daemon.pairingUrl` to
     `daemon.baseUrl` (i.e. `await page.goto(daemon.baseUrl, { waitUntil:
     "domcontentloaded" });`) — navigating to the already-consumed
     `pairingUrl` would hit `/pair` again and fail with
     `PairingOutcome::AlreadyUsed`; the cookie alone is sufficient to reach
     `/` per `authenticate_headers` (`auth.rs#L123-146`).
   - Keep the existing `await expect(page.locator(".app-shell")).toBeVisible();`
     assertion unchanged (`#L3042`) as the post-navigation check.
4. Do not touch `page.route(...)` mock registrations (`#L2936-3039`) or any
   assertion logic after the navigation in test 2 — only the
   auth/navigation lines change.
5. Do not touch `daemonHarness.ts` or `auth.rs`/`router.rs` — no daemon or
   harness-API change is needed; this is purely spec-file-local state
   threading between the two tests.

## Verification Plan

- Run `npm run test:browser -- e2e/dashboard-acceptance.spec.ts` (or the
  project's equivalent full-file Playwright command) from
  `ws-dashboard/frontend`, at least twice consecutively, and confirm both
  tests pass every run (per the ticket's stated verification boundary).
- If only a quick focused check is wanted before the full run, temporarily
  scope with `--grep` to run just the two named tests
  (`"dashboard workRoot UI browser acceptance"` and `"linked server root
  picker uses server-scoped local gateway routes"`) — but the ticket's
  verification boundary is the full file, so the final check must be the
  full unfiltered run, twice.

## Escalations

- None.
