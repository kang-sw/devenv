# Offline verification allocations

## Current exact-099 source-only allocation — 2026-11-15

MAIN's 04:57:46 rework grant supersedes the historical batch below: preserve exact
`09998d57986f9d0f716036115452e1cb912d1f28`, then run the same seven Node commands
and both full Go packages offline. MAIN's 05:16:46 update explicitly keeps that
pin despite newer develop heads. No browser/screenshot/Lite rerun or shared-runtime
action is allocated. Existing browser gaps remain failures/omissions, tracked in
`261115-bug-ws-dashboard-remaining-browser-harness-gaps`.

From the repository root, with `GOPROXY=off`, `GOSUMDB=off`,
`GOTOOLCHAIN=local`, and `BOARD_SPLIT_BASELINE` set to that exact pin:

```sh
node --test scripts/board-headless/split.test.mjs
node --check scripts/board-headless.mjs
node --check scripts/board-headless/toolbar-overview.mjs
node --check scripts/board-headless/shared-actions.mjs
node agents-plugin-tool/internal/wsdashboard/testdata/lead-popover.test.cjs
node agents-plugin-tool/internal/wsdashboard/testdata/search-palette.test.cjs
node --test agents-plugin-tool/internal/wsdashboard/testdata/task_badges.test.cjs
cd agents-plugin-tool
go test ./internal/wsdashboard -count=1 -timeout=5m
go test ./internal/wswhiteboard -count=1 -timeout=5m
```

Go uses only its own timeout; no outer test/compiler kill is permitted. Exact
exit codes and tested source head are recorded after execution in the ticket.
Raw output remains private cache evidence, not a committed log bundle.
The [current move map](move-map-09998d579.md) includes the whole-harness inverse
comparison and updated upstream source readers.

## Historical proposed browser allocation

**NOT RUN. Combined browser batch is NOT START-ready.** The split has no actual
MAIN START; H CRLF's slot is separate and grants this worker no execution.
This revision replaces the unsafe unbounded capture command in static preparation
`961c8e3bf`. It addresses static lite review 8092's three Important recipe findings,
not runtime failures. No executable checks or revised-driver imports ran.

## Exact pins and remaining prerequisites

- **Before:** `aac39c69f381a82677f0ee6857e8f8fc5bc89322`, containing landed UI-C2
  history-boundary and UI-C3 topic-unfold/beforeGeometry behavior. `d0ad8a9f1` is
  historical, no longer the baseline for this batch.
- **After:** MAIN records the full reconciled preparation commit before its own
  START. Never follow a moving branch. UI-C2/UI-C3 are part of both comparison
  sides; do not silently import unmerged 777 or UI-T5.
- **Real readiness gap:** the current `serve` entry point has synchronous,
  unbounded build/seeding work; the old capture command had unbounded rAF/close
  awaits. No allocation supervisor implementing the budgets/owned-exit handoff
  below is supplied or executed by this asset-only refactor. Until a coherent
  supervisor/runner is supplied within MAIN's authorization, do not request or
  execute this combined browser batch. This is the existing harness limitation
  tracked by `261115-bug-ws-shared-card-browser-harness-bounded-readiness`, not a
  new feature gate or permission to redesign daemon policy here.
- **shared-controls is prerequisite-blocked:** `serve --shared-actions` seeds
  files/board content, not its required coherent exited terminal. Separate
  browser commands do not inherit another scenario's terminal. Leave this probe
  omitted unless MAIN separately approves a coherent, isolated exited-terminal
  fixture for BOTH pins. Do not create a shell/PTY, substitute a browser-local
  terminal, remove assertions, or claim the later raise/fold coverage.
- Preserve offline cached dependencies. For every Go/build command use
  `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local` and existing absolute `GOPATH`/
  `GOCACHE` (the serving fixture isolates HOME). Use cached Playwright/Chromium;
  a missing dependency stops the batch, never initiates download/install.
- Any eventual START names this single recipe's exact pins, supported subset,
  isolated loopback/build/seeding/browser work and exact owned-process cleanup.
  A source-test-only allocation does not authorize the browser portion. No
  remote/owner install, shared daemon/29417 access, deployment, profiling,
  formatter/generator, other checkout mutation or broad process cleanup.

## Allocation clock, operation budgets and exit handoff

Proposed ceiling: **45 minutes from the actual allocation START**, not from the
first screenshot or from completion of a build. The eventual supervisor records
both absolute UTC and monotonic `T0`, a work deadline `T0 + 37 minutes`, and a
hard handoff deadline `T0 + 45 minutes`. The final eight minutes are reserved for
cleanup/exit accounting. No operation starts if its whole budget cannot fit before
the work deadline. Phase/operation timers do not reset the allocation clock.

| Operation | Wall-clock budget | Timeout disposition |
|---|---:|---|
| All non-browser Node contracts/syntax commands | 2 min total; 30 s each | Record first failure; no next command or retry |
| Each Go package test | Its existing `-timeout=5m`; reserve 6 min before launch | **No outer kill timeout**; Go's own receipt is required. Unexpected overrun goes to MAIN with exact owned PID and pending exit; do not start another job |
| Each revision's serve/build/seeding/readiness | 5 min total | Parent supervisor must remain responsive despite the child's synchronous build; stop only that attributed job under START's named cleanup authority; record build/seeder/daemon exits |
| Browser launch/navigation/action/wait/screenshot/evaluate | 20 s each; 3 min total captures per revision | Host-side monotonic deadline independent of page rAF; no further page/context work after timeout |
| All selected behavior probes | 4 min total across BOTH revisions; 45 s each | First failure/deadline stops the sequence; report every remaining case omitted |
| Page/context/browser close | 10 s per operation, within cleanup reserve | A rejected/timed-out close is **not** an exit receipt; hand off exact outstanding owned children |
| Fixture graceful stop / owned exit accounting | First 2 min of cleanup reserve | Use only the existing owned-supervisor shutdown path; any further exact-owned termination must be named in START, not inferred here |

These are ceilings, not a promise all proposed coverage fits: cumulative accounting
may omit later probes. The eventual allocation must disclose that limited subset;
there is no automatic second slot or retry to force completion. The supervisor
must enforce Node/browser/serve budgets outside their blocked awaits and retain
full logs, operation start/end/exit status and a per-revision progress ledger.

At the work deadline, stop admission of work and enter cleanup. Record the fixture
supervisor, build/seeder, daemon and browser child identities from their creation,
not from a broad name match. For each retain actual exit status/observation, or
`exit pending` with last known identity/state. At the hard handoff deadline, send
MAIN those exact outstanding identities, logs, last operation, issued actions and
remaining cleanup authority. The slot remains owned/pending until exits are
established or MAIN explicitly carries the ownership; signal delivery alone does
not establish quiescence. No broad kill, shared process signal or unowned cleanup.
This supervision/exit integration is a **remaining prerequisite**, not implemented
or proven by this document. Do not hide it behind a shell `timeout` around Go.

## 1. Source and existing contracts (proposed commands only)

From the reconciled preparation checkout, sequentially, inside the eventual
allocation's source-operation budgets:

```sh
BOARD_SPLIT_BASELINE=aac39c69f381a82677f0ee6857e8f8fc5bc89322 node --test scripts/board-headless/split.test.mjs
node --check scripts/board-headless.mjs
node --check scripts/board-headless/toolbar-overview.mjs
node --check scripts/board-headless/shared-actions.mjs
node agents-plugin-tool/internal/wsdashboard/testdata/lead-popover.test.cjs
node agents-plugin-tool/internal/wsdashboard/testdata/search-palette.test.cjs
node --test agents-plugin-tool/internal/wsdashboard/testdata/task_badges.test.cjs
cd agents-plugin-tool
GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test ./internal/wsdashboard -count=1 -timeout=5m
GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test ./internal/wswhiteboard -count=1 -timeout=5m
```

The opt-in pinned contract compares assembled JS/CSS, unsplit cards/stream,
unchanged moved harness source bodies and registrations; it also parses the
assembled single-IIFE script. No isolated runtime JS fragment is a standalone
script. Package tests retain landed UI-C2/UI-C3 assertions plus extraction,
source-reader, missing-fragment/dev-edit and identity contracts. This is not
`go test ./...`. Full output and real exit receipts precede any pass claim.

## 2. Baseline-first fixture and matching captures

Only after the real readiness gap above is settled and START covers this portion,
create a private temporary **archive**, not another checkout:
`git archive aac39c69f381a82677f0ee6857e8f8fc5bc89322 | tar -x -C <empty-baseline-dir>`.
Use one new batch artifact directory for both isolated fixture trees and outputs.
Do not alter old evidence or existing checkouts. Keep after source at its pin.

Serve each source root with its own unchanged command, under the required
responsive allocation supervisor and that revision's five-minute budget:

```sh
GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOPATH=<existing-absolute-cache> GOCACHE=<existing-absolute-cache> \
  node scripts/board-headless.mjs serve --topics 18 --pending 3 --history-entries 180 --shared-actions --keep-tmp
```

Record exact supervisor/build/seeder/daemon identities, temporary roots, port and
printed adapter/board/standalone URLs. Run baseline captures/probes, then establish
its owned exits before starting candidate. No shared daemon, prebuilt/reuse
interface or new fixture is invented by this recipe.

The eventual capture runner uses the SAME driver for both pins, one browser and
serial contexts/pages. The following is its required ordered algorithm, **not an
unbounded runnable replacement for the missing supervisor**:

1. For each width 420/1900, light/dark and reduced-motion reduce/no-preference,
   create one context. Visit adapter, board-only and standalone URLs serially.
   Each operation, including frame lookup and evaluation, has the host-side
   budget above; browser defaults and frame counts alone are not time bounds.
2. After navigation, locate the shell session frame with a bounded wait when the
   root is a shell. Wait for the narrow adapter tab control to be attached, then
   select the board tab **before** waiting for a visible topic:
   ```js
   // Each await is wrapped by the future host-side operation budget.
   if (kind === 'adapter' && width === 420) {
     await page.waitForSelector('#tabbar [data-tab="board"]', { state: 'attached', timeout: 15000 });
     await page.click('#tabbar [data-tab="board"]', { timeout: 15000 });
   }
   await page.waitForSelector('section.topic', { state: 'visible', timeout: 15000 });
   ```
   Apply the same theme/motion settings and wait for relevant frame heights with
   bounded operations. Keep the later narrow conversation/composer capture.
3. Capture top/full toolbar and actual scrolled floating/sticky states, with
   geometry/state metadata establishing those states. Settle sampling must have
   both a finite frame count and a host monotonic timeout independent of rAF;
   a suspended page must expire. No `page.evaluate` Promise is awaited without
   that bound. If it expires, stop, save first-failure/progress, and enter cleanup.
4. For the narrow adapter select Conversation after the board captures and
   capture its composer; wide adapter captures include both columns. Close each
   page/context/browser under the close budgets and require owned-exit accounting
   on failure. Do not claim `finally { await browser.close(); }` is a time bound.

Maximum requested matrix is eight contexts, 24 pages and 52 screenshots per
revision (top/floating for all hosts plus four narrow conversation captures),
subject to the cumulative capture budget. Pair matching route kind, viewport,
state, theme and motion settings in per-revision manifests. Inspect all completed
before/after pairs **side by side** and record
`compared <after-path> vs <before-path>: match|mismatch`. Name any volatile
fixture/time differences; they do not waive unexpected layout/typography/ownership
mismatches. Filename labels alone do not establish floating/sticky coverage.

## 3. Existing behavior probes and explicit omissions

These proposed cases use the SAME revision fixture and current split entry point,
under the single shared probe budget. They are not an implicit broad sweep.

| Scenario | URL | Width/height | Options / status |
|---|---|---|---|
| `keep` | adapter | 1900/900 | `--framed-middle` |
| `history-fit` | adapter | 1900/900 | Landed boundary and reveal behavior; combined fold+republish coverage is not established by separate tests |
| `panel-header` | standalone | 420/900 | Scenario owns its responsive matrix |
| `toolbar` | adapter | 1900/900 | Existing baseline limitations remain |
| `overview` | adapter | 420/900 | `--case anchors` |
| `overview-motion` | adapter | 1900/900 | `--case pins` |
| `shared-controls` | adapter | 1900/900 | **BLOCKED/OMITTED** without separately approved coherent exited-terminal fixture for both pins |
| `shared-actions` | adapter | 420/900 | `--case persistence`; unmerged UI-T5/terminal-reconciliation limitations remain |

Command form for an allocated, unblocked case:
`node scripts/board-headless.mjs <scenario> --url <printed-url> --width <W>
--height 900 --capture-dir <fresh-case-dir> <options>`.
Seven unblocked cases per revision maximum; a separately approved shared-controls
fixture adds only that eighth case within the SAME cumulative budget. No retries,
extra process or permission to start a real terminal follows from this table.

Baseline first-failure is recorded per command/revision with completed and
unvisited captures/probes. An obsolete UI-T5 ticket-badge failure is still a
failure, not an extraction regression or pass. On that or any first failure,
stop as allocated: candidate may be wholly unvisited, and every candidate
capture/probe is then explicitly **omitted: baseline stopped first**. Do not
reorder, weaken predicates, run the candidate anyway, or import UI-T5/777 to
manufacture a comparison. Any different allocation belongs to MAIN. Repetition
of an encountered root cause triggers the worker's non-convergence stop.

## Static review disposition and completion boundary

- Important 1: the prescribed narrow adapter selection now precedes visible-topic
  waiting; the old faulty runnable capture block is removed.
- Important 2: shared-controls is explicitly prerequisite-blocked; no terminal
  is manufactured or its coverage claimed.
- Important 3: the allocation clock, operation ceilings, cleanup reserve and
  exact-owned exit handoff are specified; the absent enforcement runner remains
  a disclosed NOT START-ready gap, rather than an asserted effective bound.

These are static author dispositions only. Lite allocated one pass, so no new
review round is requested here. No tests, captures, acceptance, Phase 1 Result,
closure or new merge/deployment is authorized. MAIN owns the split's eventual
allocation, integration and UI-start pause; H's existing slot is not this slot.
