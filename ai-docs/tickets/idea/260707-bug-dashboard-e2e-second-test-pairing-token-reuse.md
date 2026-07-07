---
title: "Dashboard e2e suite's second test fails: reuses the daemon's one-time pairing URL across an isolated browser context"
related:
  260707-bug-dashboard-terminal-clears-on-tab-switch: related-area
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
already supports something like this for other reasons. Verification: run
the full `e2e/dashboard-acceptance.spec.ts` file (both tests) at least twice
consecutively and confirm both pass every time.

## Spec Impact

Test-harness-only fix; no product-visible behavior changes (the daemon's
one-time pairing token security behavior is explicitly preserved, not
altered). Spec area: none. Contract-first spec: no.
