---
title: The e2e build step lives only in the test:browser npm script, so a direct npx playwright test bypasses it and serves a stale bundle
related:
  260725-feat-dashboard-pty-agent-attention-notification: Phase 7 ran bare npx playwright test as evidence throughout, including reviewers; one mutation run passed against the pre-mutation bundle
  260725-bug-dashboard-e2e-harness-destroys-daemon-diagnostics: adjacent harness work — disjoint file ownership, see Constraints
spec:
  - 260516-ws-web-dashboard-browser-ui-acceptance-gate
related-mental-model:
  - ws-web-dashboard
---

# The e2e build step lives only in the test:browser npm script, so a direct npx playwright test bypasses it and serves a stale bundle

## Background

Found on 2026-07-26. The dashboard e2e suite does have a build step, but it
sits in exactly one place, and the obvious shortcut around it is unguarded.

Re-verified against source on 2026-07-26 in `ws-dashboard/frontend`:

- `package.json:27` — `test:browser` is
  `npm run build && (cd .. && cargo build -p ws-dashboard-daemon) && playwright test`.
  This is the safe entry point and it works. (The original capture cited
  `package.json:25`; that line is `test:browser-attention-cue`.)
- `package.json` declares no `test` script at all, so there is no script for a
  `pretest` hook to attach to. More decisively: `npx playwright test` never
  enters npm, so *no* npm lifecycle hook can guard the bypass path. This is a
  mechanism fact, not a preference — see Decisions.
- `playwright.config.ts:9-25` — the exported `defineConfig` declares no
  `webServer`, no `globalSetup`, and no `globalTeardown`.
- Nothing under `e2e/` invokes a build. `daemonHarness.ts:216-217`'s
  `startDaemon` points the daemon at `<repoRoot>/frontend/dist` via
  `--static-dir` unless `staticDir` overrides it (`daemonHarness.ts:120` maps
  that to `WS_DASHBOARD_STATIC_DIR`); `repoRoot` is `ws-dashboard/`
  (`daemonHarness.ts:7`).
- Not in the original capture: `startDaemon` has a *second* path that never
  touches `frontend/dist`. `daemonHarness.ts:103` selects external mode from
  `WS_DASHBOARD_DAEMON_MODE=external`, `WS_DASHBOARD_DAEMON_BASE_URL`, or
  `WS_DASHBOARD_DAEMON_PAIRING_URL`, and that branch returns before reaching
  the `--static-dir` construction at all.

So the protection is carried entirely by which command you happen to type.
`npx playwright test <spec>` — the natural move when iterating on a single spec
file, and what was actually used throughout Phase 7 of
`260725-feat-dashboard-pty-agent-attention-notification`, reviewers included —
runs the browser against whatever `dist/` already sits on disk.

The sharp edge is not "the harness never builds." It is that a footgun which
only fires when you take the obvious shortcut is arguably worse than no
protection at all: the presence of `test:browser` makes the suite *look*
build-safe, so nobody re-derives the risk when they drop to a direct
invocation.

The concrete damage is to mutation-based non-vacuity evidence. Mutate
`frontend/src` to prove an assertion catches a regression, run Playwright
directly, and the browser serves the pre-mutation bundle — the run passes, and
a passing assertion and a wrong build look identical from the terminal. Phase 7
hit this: an implementer's first mutation run passed for this reason, and
because the mismatch leaves no trace in output, the bare `npx playwright test`
runs used as evidence throughout that phase cannot be re-established after the
fact.

The existing mitigation is procedural and already recorded: the Traps section
of `ai-docs/mental-model/ws-web-dashboard/index.md` carries an entry requiring
`npm run build` after any `frontend/src` mutation used as evidence. This ticket
exists because the discipline is unenforced on the bypass path, not because the
hazard is unaddressed.

## Decisions

**Settled: build the production bundle unconditionally in a Playwright
`globalSetup`, so every invocation path — `npm run test:browser`, bare
`npx playwright test`, a single-spec run, an IDE runner — builds before the
daemon is pointed at `dist/`.**

The tradeoff that kept this ticket in `idea/` was the assumption that moving the
build onto every invocation would tax the fast single-spec loop the bypass
exists to serve. That assumption was measured rather than argued.

Measured `npm run build` in `ws-dashboard/frontend` on a warm tree
(`tsc -b` incremental + `vite build`), three consecutive runs, the third taken
immediately after touching a `src/` file so it reflects the real
mutate-then-rerun case:

| run | condition | wall |
| --- | --- | --- |
| 1 | warm, no `src` change | 2.78 s |
| 2 | warm, no `src` change | 2.40 s |
| 3 | warm, after `touch src/main.tsx` | 2.34 s |

(`vite build` itself reports 191–261 ms; the rest is `tsc -b`.)

Against that, `playwright.config.ts:16-17` sets a 180 s per-test timeout and a
20 s expect timeout with `workers: 1`, and `startDaemon` allows a 60 s daemon
readiness budget. A single-spec run is dominated by daemon boot, owner pairing,
and browser drive. A ~2.4 s unconditional build is a few percent of the loop it
was feared to ruin, so the speed objection does not survive measurement and the
correctness argument decides the question.

**Rejected: detect-and-refuse** — a `globalSetup` that compares `dist`
freshness against `src` and fails the run with a clear message when stale.
It loses on failure asymmetry. It still has to carry a freshness heuristic, and
that heuristic's bad direction is silent: a false "fresh" verdict reinstates
exactly the bug being fixed, with exactly the property that made the bug
expensive in the first place — nothing in the output distinguishes it from a
real pass. It also leaves a manual step on the path taken by people in a hurry,
who are precisely the population that already skipped `test:browser`. Paying
2.4 s to delete a class of silent wrong answers is the better trade.

**Rejected: conditional build** — build only when `dist` is stale. It carries
the *same* freshness heuristic as detect-and-refuse, with the same silent
false-fresh failure mode, and buys back only the ~2.4 s measured above. Not
worth a correctness risk at that price. `tsc -b` and Vite already do their own
incremental work inside the build, so the interesting part of the saving is
already being captured by the unconditional option.

**Rejected on mechanism: a `pretest`-style npm hook.** This one is not a
tradeoff, it is non-responsive. npm lifecycle hooks only fire when the runner
is invoked through npm; the reported bypass is `npx playwright test`, which
does not. The hook would guard only paths that are already guarded. Recorded
here so it is not re-proposed.

**Settled: `test:browser` keeps its leading `npm run build`.** After this
change that build is a ~2.4 s incremental no-op, but the script stays
self-describing and the change stays purely additive; if `globalSetup` is ever
misconfigured, the documented gate entry point is unchanged. Removing it would
be a wider edit for no correctness gain.

**Settled: no opt-out.** No `WS_DASHBOARD_SKIP_BUILD`-style escape hatch is
added. An opt-out is the bypass path this ticket closes; adding one recreates
the ticket.

## Constraints

- **Scope is the `dist/`-vs-`src/` relationship only.** Playwright's own runner
  reads `*.spec.ts` fresh off disk on every invocation, so edits inside the e2e
  test files are never stale and need no guard.
- **The daemon binary has the identical structural gap and is deliberately out
  of scope.** `resolveDaemonBinary` resolves `<repoRoot>/target/debug/ws-dashboard`
  (`daemonHarness.ts:215`), and `cargo build -p ws-dashboard-daemon` also lives
  only in `test:browser` (`package.json:27`), so a bare `npx playwright test`
  can serve a stale daemon too. Not addressed here: a Rust build is a different
  cost class than 2.4 s and the measurement that settles the frontend question
  does not transfer. Recorded so a future session knows it was seen, not missed.
- **Skip the build only where `frontend/dist` is provably not what gets
  served.** Two such cases exist and both must skip, or the run pays for a build
  nobody consumes: `WS_DASHBOARD_STATIC_DIR` is set (`daemonHarness.ts:120`,
  `:216`), or external mode is selected (`daemonHarness.ts:103`:
  `WS_DASHBOARD_DAEMON_MODE=external`, `WS_DASHBOARD_DAEMON_BASE_URL`, or
  `WS_DASHBOARD_DAEMON_PAIRING_URL`). Skips must announce themselves on stdout;
  a silent skip is the failure mode this ticket is about.
- **A failing build must fail the run.** A non-zero build exit propagates out of
  `globalSetup` as a hard stop, never a warning that lets Playwright proceed
  against the previous bundle.
- **File ownership vs `260725-bug-dashboard-e2e-harness-destroys-daemon-diagnostics`
  (todo/).** That ticket's Phase 1 edits `e2e/daemonHarness.ts::startDaemon`
  (the stdout/stderr no-op drain) and `e2e/dashboard-acceptance.spec.ts`'s
  `afterAll` teardown, and writes captured daemon output under `e2e/.artifacts/`.
  This ticket must not touch either of those files: it owns `playwright.config.ts`
  and the new `e2e/globalSetup.ts` only. There is no ordering dependency in
  either direction. Two adjacencies to expect rather than be surprised by — if
  the diagnostics ticket later needs a `globalTeardown`, it adds a sibling key
  next to this ticket's `globalSetup` key in `playwright.config.ts`; and this
  ticket writes nothing under `e2e/.artifacts/` (build output goes to `dist/`,
  setup logging goes to stdout), so the artifact directory stays wholly that
  ticket's.
- **`tsconfig.e2e-tests.json`'s `include` list stays as-is** (`e2e/daemonHarness.ts`,
  `e2e/daemonHarness.test.ts`). Playwright transpiles `globalSetup` itself; the
  new file only enters that tsconfig if a node-runnable unit test is added for
  it. Name the file `globalSetup.ts`, not `*.test.ts`/`*.spec.ts`, so neither
  Playwright's `testDir: "./e2e"` discovery nor the route-test runner picks it
  up as a test.

## Spec Impact

Addressed through existing stem `260516-ws-web-dashboard-browser-ui-acceptance-gate`
(`ai-docs/spec/ws-web-dashboard/index.md:1675`, "Browser UI Acceptance Gate").
That entry already states: "The frontend package exposes this gate through
`npm run test:browser`. The gate builds the production frontend, serves it
through the dashboard daemon, pairs as owner..." — so the specified behavior is
already "the gate builds the production frontend." The implementation only
satisfies it on one entry path. This ticket is therefore a **conformance fix**
to an existing entry, not new behavior.

- **Target spec area:** `260516-ws-web-dashboard-browser-ui-acceptance-gate`.
- **Expected caller-visible change: none.** This is developer-facing harness
  tooling. Nothing changes in the daemon HTTP API, the contents of the served
  bundle, or any dashboard UI surface. The only observable deltas are to a
  developer running the gate: a ~2.4 s build on invocation paths that
  previously skipped it, and a run that can no longer serve a stale bundle.
  Stating this explicitly is how this ticket satisfies the `ready/` spec gate —
  the gate is satisfied by an existing entry plus a declared no-product-behavior
  result, not by omission.
- **Spec text delta:** one clarifying sentence in that entry, binding "the gate
  builds the production frontend" to the Playwright run rather than to the npm
  script, since today the sentence reads as a property of `test:browser` and the
  fix makes it a property of the gate itself. Note the two documented skip
  conditions there too, so the spec does not overclaim.
- **Contract-first spec:** no. The entry exists and the behavior is already
  described; there is no contract to design ahead of implementation.
- **Mental-model update on contact:** the Traps entry in
  `ai-docs/mental-model/ws-web-dashboard/index.md` describing this exact hazard
  (mutate `frontend/src`, run Playwright without rebuilding) becomes partially
  obsolete. It must be **updated, not deleted**: the "always rebuild manually"
  discipline is superseded on the default spawn path, but still applies whenever
  `WS_DASHBOARD_STATIC_DIR` or external mode makes `globalSetup` skip the build.

## Phases

### Phase 1: Build the production bundle in Playwright `globalSetup`

Close the bypass so no invocation path can serve a stale `frontend/dist`.

**Completed behavior.** `playwright.config.ts` declares `globalSetup` pointing
at a new `ws-dashboard/frontend/e2e/globalSetup.ts`. That setup runs
`npm run build` in `ws-dashboard/frontend` before any test starts, and a
non-zero exit propagates out of `globalSetup` as a run-ending failure. It skips
the build — announcing the skip and the reason on stdout — only when
`WS_DASHBOARD_STATIC_DIR` is set or external mode is selected via
`WS_DASHBOARD_DAEMON_MODE=external` / `WS_DASHBOARD_DAEMON_BASE_URL` /
`WS_DASHBOARD_DAEMON_PAIRING_URL`, mirroring the branch selection at
`daemonHarness.ts:103` and the `staticDir` default at `daemonHarness.ts:216`.
No skip-build escape hatch is introduced. `test:browser` in `package.json` is
left unchanged. The spec sentence and the mental-model Traps entry named in
`## Spec Impact` are updated in the same phase.

**Deferred scope.** The daemon binary's identical gap (`cargo build` also lives
only in `test:browser`) is not addressed. `e2e/daemonHarness.ts` and
`e2e/dashboard-acceptance.spec.ts` are not touched — they belong to
`260725-bug-dashboard-e2e-harness-destroys-daemon-diagnostics`. No freshness or
staleness heuristic is written, in this phase or later; the settled decision is
that the build is unconditional on the paths where it runs at all.

**Verification boundary.** This phase proves only that the daemon-served bundle
matches current `frontend/src` on every Playwright invocation path. It proves
nothing about daemon binary freshness, and nothing about `*.spec.ts` staleness
(which cannot occur).

**How a future session proves the guard actually works.** The demonstration is
the original failure, re-run:

1. Modify a `frontend/src` file so that an assertion in one existing e2e spec
   must fail — e.g. change a rendered string that spec asserts on.
2. Do **not** run `npm run build`. Invoke the e2e entry point directly:
   `npx playwright test e2e/<that>.spec.ts` from `ws-dashboard/frontend`.
3. The run must now surface the mutation — `globalSetup` builds, the daemon
   serves the mutated bundle, and the assertion fails on the mutation. Before
   this change the identical command passes, because the pre-mutation bundle is
   served. The proof is that the same command flips from false pass to
   mutation-driven failure; a green run at this step means the guard did not
   engage and the phase is not done.
4. Revert the `src` mutation and confirm the same command returns to green.
5. Prove both skip conditions announce and do not rebuild: run once with
   `WS_DASHBOARD_STATIC_DIR` pointed at a prebuilt directory, and once with
   `WS_DASHBOARD_DAEMON_MODE=external`, and check the skip line and reason
   appear on stdout.
6. Record the measured `globalSetup` build overhead on a warm tree in the
   Result, so a future reader can re-check the ~2.4 s premise this decision
   rests on rather than inheriting it as folklore.
