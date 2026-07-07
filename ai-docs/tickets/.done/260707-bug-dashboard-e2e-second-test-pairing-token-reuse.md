---
title: "Dashboard e2e suite's second test fails: reuses the daemon's one-time pairing URL across an isolated browser context"
related:
  260707-bug-dashboard-terminal-clears-on-tab-switch: related-area
sage-review: completed
completed: 2026-07-07
---

# Dashboard e2e suite's second test fails: reuses the daemon's one-time pairing URL across an isolated browser context

## Background

Discovered as a forward note across two tickets this session
(`260707-bug-dashboard-e2e-multi-root-locator-leakage`,
`260707-bug-dashboard-terminal-clears-on-tab-switch`) while getting
`npm run test:browser` (`e2e/dashboard-acceptance.spec.ts`) fully green for
the first time in this sandbox. It is the last remaining failure blocking a
fully green run of the file.

Confirmed root cause (read `ws-dashboard/crates/daemon/src/auth.rs`): the
daemon's owner pairing token is one-time-use by design
(`consume_pairing_token`, `pairing_consumed` flag) — this is correct,
intentional product security behavior, not a bug. The e2e harness
(`e2e/daemonHarness.ts`) spawns one daemon per spec file (`beforeAll`) and
exposes a single `daemon.pairingUrl` scraped from startup output. The first
test (`dashboard workRoot UI browser acceptance`, `spec.ts:797`) navigates to
it and consumes the token, establishing a session cookie in that test's
Playwright browser context. The second test
(`linked server root picker uses server-scoped local gateway routes`,
`spec.ts:2929`) gets a **fresh, isolated** Playwright browser context (no
shared cookies) and calls `page.goto(daemon.pairingUrl)` again
(`spec.ts:3042`) — the token is already consumed, pairing fails, and
`.app-shell` never renders, failing the `toBeVisible()` assertion at test
start before any of that test's actual routing/gateway assertions run.

Confirmed pre-existing and unrelated to any product code: reproduces
identically on `ws-dashboard-dev` with no other pending changes; the daemon's
one-time-token behavior itself is intentional and out of scope to change.

## Phases

### Phase 1: Fix the e2e harness so a second cookie-scoped test can reuse the paired session

Give the second test (and any future test needing an authenticated session)
a way to reach the app without re-consuming the daemon's one-time pairing
token. Do not weaken or change the daemon's one-time-pairing-token security
behavior (`auth.rs`) — this is a test-harness-only fix. Plausible
approaches (pick after inspecting what's simplest/most idiomatic for this
harness, do not assume): (a) capture the session cookie/auth state after the
first test's pairing and inject it into the second test's context via
`context.addCookies()` / `storageState`; (b) restructure the two tests into
a shared `test.describe.serial` block reusing one `page`/context instead of
Playwright's default per-test isolated context; (c) have the daemon harness
expose a way to mint a fresh pairing token or an equivalent authenticated
entry point per test (e.g. a test-only re-pairing affordance), if the daemon
already supports something like this for other reasons.

Note: the spec file already declares
`test.describe.configure({ mode: "serial" })` (`spec.ts:51`), so approach (b)
only fixes test ordering, not Playwright's per-test isolated
page/context — reusing the first test's page would also carry over its
opened workRoot/terminal state, which could collide with the second test's
`page.route(...)` mocks and assertions. This nudges toward approach (a)
(capture the auth cookie/storageState after the first pairing, inject it
into the second test's fresh context) as the cleaner, isolation-preserving
option, but the implementer should confirm after inspecting the actual
session/cookie mechanism.

Verification: run
the full `e2e/dashboard-acceptance.spec.ts` file (both tests) at least twice
consecutively and confirm both pass every time.

### Result

Implemented approach (a): the first test ("dashboard workRoot UI browser
acceptance") now captures the owner session cookie via
`await page.context().cookies(daemon.baseUrl)` immediately after the
existing "owner pairing" step assertions, storing it in a new module-level
`ownerCookies` variable declared alongside the file's other shared `let`
state. The second test ("linked server root picker uses server-scoped local
gateway routes") injects that cookie into its own fresh, isolated context
via `await page.context().addCookies(ownerCookies)` (with a defensive
fail-fast throw if `ownerCookies` is unset, relying on the file's existing
`test.describe.configure({ mode: "serial" })` ordering), then navigates to
`daemon.baseUrl` instead of the now-consumed `daemon.pairingUrl` before the
unchanged `.app-shell` visibility assertion. No changes were made to
`auth.rs`, `router.rs`, or `daemonHarness.ts` — this is purely spec-file
state threading between the two tests, confirming the survey's finding that
`authenticate_headers` accepts the cookie alone (no re-pairing) once
`pairing_consumed` is `true` for the daemon's lifetime.

Verification: ran `npm run test:browser -- e2e/dashboard-acceptance.spec.ts`
from `ws-dashboard/frontend` twice consecutively. Both runs: `2 passed` (both
`dashboard workRoot UI browser acceptance` and `linked server root picker
uses server-scoped local gateway routes` green, ~32s per full run).

## Spec Impact

Test-harness-only fix; no product-visible behavior changes (the daemon's
one-time pairing token security behavior is explicitly preserved, not
altered). Spec area: none. Contract-first spec: no.


## Resolution (2026-07-07)

Fixed by capturing the owner session cookie after test 1's pairing and injecting it into test 2's isolated context (commit bfb8e271, merged to ws-dashboard-dev). Fit and test partition reviews both clean. Both tests in `e2e/dashboard-acceptance.spec.ts` pass consistently across repeated runs.
