---
title: "short-viewport regression gate fails on macOS: fitNow() shrinks to 47 rows instead of holding 120"
sage-review-design: recommended
related:
  260725-feat-dashboard-nav-row-two-line-open-state: found-during
  260707-bug-dashboard-terminal-clears-on-tab-switch: regression-gate-violated
  260725-bug-dashboard-terminal-socket-path-length-unguarded: visibility-precondition
---

# short-viewport regression gate fails on macOS: fitNow() shrinks to 47 rows instead of holding 120

## Background

The macOS browser-acceptance run of
`ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts` fails inside
`test.step("short viewport does not collapse the terminal to 1 row")` at
`ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts:3696`:

```
expect(shortRows).toBe(terminalClearFixTallRows);
Expected: 120
Received: 47
```

That step is the load-bearing assertion of the
`260707-bug-dashboard-terminal-clears-on-tab-switch` regression gate. The
step's own source comment (lines 3691-3695) states the contract precisely:
`fitNow()` must "preserve the current (last-good) emulator size and return
without resizing" on a degenerate resize proposal, and explicitly calls out
that a regression which "let the guard trip too late and shrink partway
(e.g. 54 -> 3 rows)" must fail this strict-equality assertion rather than
pass a weaker `toBeGreaterThan(1)` check. **120 -> 47 is exactly that
partway-shrink failure mode**: the terminal does not collapse to 1 row (so
the historically-weaker check would have passed), but it does not hold the
last-good size either.

Repro sequence (from the existing spec step, unmodified):
1. Viewport is resized to `{ width: 1440, height: 160 }`.
2. `await page.waitForTimeout(800)` lets the ResizeObserver -> `fitNow()` ->
   debounced `forwardSize` cycle settle.
3. `terminalRows(page)` is read and compared against the pre-resize
   `terminalClearFixTallRows` baseline (120).

This ticket does not fix the defect. It only files the finding and captures
the evidence gathered while root-causing it. Do not modify `App.tsx`,
`styles.css`, or any e2e spec file as part of closing this ticket's Phase 1;
scope any fix into a later phase or a follow-up ticket once the root cause
in `fitNow()` is actually located.

## Why this was invisible until today

This step had never actually executed on macOS before. Terminal creation
itself was broken by the macOS 104-byte `sockaddr_un.sun_path` ceiling: the
e2e harness derived `WS_DASHBOARD_STATE_HOME` from `os.tmpdir()`, which on
macOS resolves to a ~48-char `/var/folders/<hash>/T/` path, producing a
108-byte socket path that exceeded the limit. That was fixed today in
commit `b75a7d0c` ("fix(dashboard): scope e2e harness state-home temp dir
short on macOS", path length 108 -> 64 bytes; see
`260725-bug-dashboard-terminal-socket-path-length-unguarded`). The suite
runs `test.describe.configure({ mode: "serial" })`, so every earlier run
died before ever reaching line 3696.

**The socket-path fix did not cause this failure — it revealed it.**
Whether the underlying defect is a genuine regression in `fitNow()`'s
degenerate-resize guard, or whether that guard was never correct on macOS
in the first place, is an **open question** and should not be asserted
either way without further investigation.

## Evidence this is unrelated to the in-flight nav-row work

Controlled experiment on branch `impl/nav-row-two-line-open-state-phase1`,
same build and command, only the diff varied:

| tree state | outcome |
| --- | --- |
| without nav-row e2e fix (2 runs) | dies earlier at `:2880`, never reaches `:3696` |
| with nav-row e2e fix (2 runs) | `:2880` passes; fails `:3696`, 120 vs 47, 20.9s / 20.8s |
| with nav-row e2e fix, nav-row step neutralized (2 runs) | fails `:3696`, 120 vs 47, 20.4s / 20.4s |

The third arm is decisive: neutralizing the nav-row step leaves the failure
bit-identical (same numbers, same timing), so the in-flight
`260725-feat-dashboard-nav-row-two-line-open-state` work is not the cause.
The failure was deterministic across all 4 runs that reached the step —
not a timing flake, despite the fixed `waitForTimeout(800)` making that a
plausible first guess.

## Linux reproduction and root cause (2026-07-29)

Reproduced on Linux while restoring the acceptance gate after the terminal
status-bar removal (PR #7, `goal/ws-dashboard-dev/copper-heron-vale`). Same
step, same failure mode, different numbers: `expect(shortRows).toBe(56)` →
received **3**. This answers the first open question below — **the defect is
not macOS-specific**, and the socket-path fix unmasked it rather than the
platform causing it.

Viewport-height sweep on Linux (terminal surface floors at **65px**, the
Dockview minimum):

| viewport height | settled rows |
| --- | --- |
| 200px | 6 |
| 160 / 140 / 120px | 3 |
| 100 / 80 / 60px | **1** |

Two conclusions follow, and they pull in opposite directions:

1. **The step's contract is unsatisfiable as written.** No viewport height
   reproduces "preserve the last-good size" — the terminal always refits.
   Relaxing the strict-equality expectation locally does not rescue the step
   either: at 3 rows the viewport shows only the prompt, so the *next*
   assertion fails because `CLEAR-FIX-LINE-40` has scrolled into scrollback.
   The whole step needs rewriting, not a threshold tweak.
2. **There is a genuine product defect underneath.** At ≤100px the emulator
   still reaches 1 row — precisely the state
   `260707-bug-dashboard-terminal-clears-on-tab-switch` set out to prevent.
   Mechanism located: `fitNow()`'s early-return guard only rejects proposals
   of `rows <= 1`, but the post-fit
   `while (rows > 1 && !terminalScreenFitsVisibleBox(...)) resize(rows - 1)`
   shrink loop runs *after* that guard and is not covered by it. The guard
   protects the proposal and then the loop walks the emulator down anyway.

So the answer to "does the guard trip on the wrong threshold" is: the guard's
threshold is fine, but it is not the only path to a degenerate size.

Resolving this needs a product decision that this filing pass did not make:
whether a pane too short to hold its last-good size should **preserve that
size** (and clip/scroll) or **fit the pane** (and shrink). The pane-fill
assertions elsewhere in the same spec constrain that choice, so the two must
be settled together.

## Impact

This failure red-lights the entire browser acceptance gate for every
subsequent dashboard-frontend change on macOS. Per
`ai-docs/mental-model/ws-web-dashboard/index.md`, browser-level
verification is required to close any visible-UI work, so an unresolved
gate failure blocks future work rather than being merely incidental — this
ticket should be treated as `ready/`-worthy priority. It lands in `todo/`
for now only because the `ready/`-landing sage design-review gate resolved
to `ask` ("Run design review for this ticket?") and this filing pass did
not run that review; promote to `ready/` after clearing that gate (or after
an explicit decision to skip it).

## Phases

### Phase 1: Root-cause and fix the `fitNow()` degenerate-resize guard on macOS

Determine why the short-viewport transition settles at 47 rows instead of
holding the last-good 120-row size, and fix it so the strict-equality
assertion at
`ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts:3696` passes on
macOS. Open questions to resolve as part of this phase:

- ~~Did the `fitNow()` guard regress after
  `260707-bug-dashboard-terminal-clears-on-tab-switch` landed, or was it
  never exercised/correct on macOS (i.e. this is a pre-existing platform
  gap that the socket-path fix simply unmasked)?~~ **Answered 2026-07-29** —
  not platform-specific; reproduces identically on Linux. See the Linux
  reproduction section above.
- ~~Is 47 rows a stable settled value for a 160px-tall viewport (suggesting
  the guard trips on the wrong threshold), or itself a partial/racy state?~~
  **Answered 2026-07-29** — stable and deterministic, and the threshold is
  not the problem: the post-fit shrink loop bypasses the guard entirely.
- **Open, and blocking**: should a pane too short for its last-good size
  preserve that size (clip/scroll) or fit the pane (shrink)? The step's
  strict-equality contract assumes the former; the observed behaviour and the
  spec's own pane-fill assertions assume the latter. Settle this before
  touching either the guard or the gate, and rewrite the step to match —
  relaxing its expectation alone leaves the following assertion failing.

