# Survey plan — activity-badge fetch timeout (git-diff ticket Phase 2)

Ticket: `260724-bug-dashboard-git-diff-index-lock-stuck-activity-badge` — Phase 2
(Defect 2: frontend fetch has no timeout, so a stalled daemon response wedges the
work-root activity badge — and sibling pollers — at "loading" forever, with an
`inFlight` guard that never clears).

Branch: `impl/activity-badge-fetch-timeout`. Delegated implementer; survey depth.

## Root cause (from ticket + survey)

The frontend polling fetchers issue a bare `fetch()` with no timeout. When the
daemon accepts the connection but never sends a response body (the exact
condition Phase 1's lock contention could induce, and any daemon stall in
general), the promise never settles. Each of these pollers also holds an
`inFlight` boolean guard that is only cleared in the promise's `.finally`/`.then`
— which never runs — so the poller deadlocks permanently: no retry, badge stuck
at "loading". The `.catch` error path that would transition the UI to an error
phase is never reached either.

## Approach

Introduce one small shared helper and route every deadlock-prone poller through
it. No behavior change on the happy path; on a stall the fetch aborts after a
bounded timeout, the promise rejects, `.finally` clears `inFlight`, and the
existing `.catch` handlers transition to their error phase and allow the next
poll tick to retry.

### 1. New helper — `ws-dashboard/frontend/src/fetchWithTimeout.ts`

- Wrap `fetch(input, init)` with an `AbortController`.
- `const t = setTimeout(() => controller.abort(), timeoutMs)` with a default of
  ~8000ms (export the default as a named constant so tests and call sites can
  reference it; allow a per-call override).
- Pass `signal: controller.signal`, merging with any caller-supplied `signal`
  (do not clobber a caller signal — if the survey finds no call site passes one,
  a simple override is acceptable, but prefer merge-safe).
- `clearTimeout(t)` in a `finally` so a fast response does not leave a dangling
  timer.
- On abort the underlying fetch rejects with an `AbortError`; let it propagate so
  existing `.catch` handlers run. Do not swallow it.
- Keep it dependency-free and typed to the same signature shape callers already
  use.

### 2. Register the helper + its test in `tsconfig.route-tests.json`

- Add both `src/fetchWithTimeout.ts` and its new test module to the `include`
  array. Test modules not listed there are not compiled by the route-test tsc
  pass and will silently not run.

### 3. Route the three deadlock-prone poller families through it

Confirm exact line anchors before editing (survey line numbers below may drift):

- **`ws-dashboard/frontend/src/resourceRefresh.ts` (~14-43)** — HIGHEST severity.
  `requestDashboardResources` / `requestDashboardServers` have the identical
  `inFlight`-deadlock shape. Wrap their `fetch` calls with `fetchWithTimeout`.
- **`ws-dashboard/frontend/src/workRootActivity.ts` (~423-436)** —
  `fetchWorkRootActivity`. Wrapping here fixes all **three** App.tsx call sites
  (~4533, ~4606, ~4797) with **zero App.tsx edits** — the timeout lives in the
  shared fetcher. Confirm the App.tsx `.catch` handlers already transition to the
  error phase (survey says they do); if so, no App.tsx change is needed.
- **`ws-dashboard/frontend/src/gitToolbar.ts` (~77-95)** —
  `fetchWorkRootGitStatus` / `fetchWorkRootBranches`. Same wrap.

## Out of scope (do NOT do here)

- **Server-side timeout / daemon-side stall bound.** The survey recommends a
  separate follow-up ticket for a daemon-side response timeout; do not implement
  it in this phase. If confirmed worth tracking, capture an `idea/` ticket and
  mention it in the closeout — do not expand this phase.
- Any App.tsx structural edit beyond what wrapping the shared fetchers requires
  (ideally none).

## Verification

- `cd ws-dashboard/frontend && npm run build` — must be clean.
- `npm run test:work-root-activity`
- `npm run test:git`
- `npm run test:resource-model`
- Do NOT gate on `npm run test:browser` (Playwright) — it is independently RED on
  unrelated bug 260713.
- Add/extend a route test that asserts: given a fetch that never resolves, the
  wrapped fetcher rejects after the timeout and the `inFlight` guard is released
  (i.e. a subsequent call is allowed). Prove non-vacuity: without the timeout
  wrap the test must hang/fail.

## Risk / escalation

- No API/contract change; no cross-module interface change; frontend-only,
  additive. No escalation expected. If wrapping forces a signal-merge change to a
  call site that passes its own `AbortController` (e.g. an unmount abort), keep
  BOTH signals effective and note it in review.
