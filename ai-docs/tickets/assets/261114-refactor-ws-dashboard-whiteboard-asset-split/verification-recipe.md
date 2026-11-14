# One proposed offline verification batch

**NOT RUN. Requires MAIN's actual heavy-slot START.** H CRLF has first priority.
This is one requested serial slot (maximum 45 minutes), not standing permission,
not a deployment procedure and not a claim of process quiescence. Stop on timeout,
unexpected failure, unavailable offline dependency or mismatched source pin; retain
all output. Diagnose implementation/test/environment before changing anything.
Do not retry a failure with an already encountered root cause.

## Pins and bounds

- Before: full source `d0ad8a9f1`, not a peer's tested head.
- After: MAIN records the full static-preparation commit OID before START.
  No peer integration is part of this batch. If MAIN changes that pin, regenerate
  the allocation; do not silently test a moving branch.
- One Go test process or one fixture/browser operation at a time. No full suite,
  dependency install/download, formatter, generator, remote/owner install, shared
  daemon/port 29417 access, profiling, or process-wide cleanup.
- Go environment for every command: `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`.
  Carry the existing absolute `GOPATH` and `GOCACHE` into fixture serving because
  its existing isolated HOME otherwise changes the cache defaults. Resolve these
  only after START. Use the already cached Playwright directory and browser;
  missing caches stop the batch, not trigger a download.
- START must name this recipe and include isolated loopback fixtures/browser
  work and cleanup of exactly their owned supervisor/daemon/browser processes.
  A test-only slot does not imply that authority. No daemon lifecycle code changes
  or shared daemon activation are requested.
- Save separate full stdout/stderr and exit receipts for every command. Read the
  complete outputs before interpreting a result. No outer kill timeout around Go
  tests; use their own `-timeout`. Retain failed receipts and screenshots.

## 1. Source and focused contracts

From the preparation checkout, in this order:

```sh
BOARD_SPLIT_BASELINE=d0ad8a9f1 node --test scripts/board-headless/split.test.mjs
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

The first command checks exact assembled JS/CSS bytes against the explicit pin,
assembled single-IIFE syntax, moved harness source-body equality, unchanged
scenario registration and factory dependency contracts. Individual runtime JS
fragments are **not** standalone scripts; do not syntax-check them independently.
Go package tests cover embedded availability, loading/order, asset identities,
dev-source propagation and existing extracted-function behavior. This is not
`go test ./...` and does not change their predicates.

## 2. Matching before/after fixtures and captures

After source/contracts succeed, create a private temporary **archive**, not a
second checkout: `git archive d0ad8a9f1 | tar -x -C <empty-baseline-dir>`. Both that
archive and all outputs live under one new batch artifact directory. Do not
modify any existing checkout or earlier evidence. Keep the current checkout at
the after pin. The exact current capture driver below is used for both revisions;
the old source's harness is used only to serve its own fixture.

Run one fixture at a time, first baseline, then candidate, from its own root:

```sh
GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOPATH=<existing-absolute-cache> GOCACHE=<existing-absolute-cache> \
  node scripts/board-headless.mjs serve --topics 18 --pending 3 --history-entries 180 --shared-actions --keep-tmp
```

Keep its supervisor PID, printed daemon PID/port/temp directory and printed
adapter/board/standalone URLs in the batch manifest. Do not use any other daemon.
Use those three URLs as `ADAPTER_URL`, `BOARD_URL`, `STANDALONE_URL`, a fresh
per-revision `CAPTURE_DIR`, and the cached absolute `BOARD_HEADLESS_PLAYWRIGHT`
package directory in the following **single serial capture command**, run from
the preparation checkout. This code is proposed only; it has not been executed.

```sh
node --input-type=module <<'JS'
import fs from 'node:fs';
import path from 'node:path';
import { pathToFileURL } from 'node:url';
const { chromium } = await import(pathToFileURL(path.join(process.env.BOARD_HEADLESS_PLAYWRIGHT, 'index.mjs')).href);
const dir = process.env.CAPTURE_DIR;
fs.mkdirSync(dir, { recursive: true });
const manifest = [], errors = [];
const browser = await chromium.launch({ headless: true });
try {
  for (const width of [420, 1900]) for (const colorScheme of ['light', 'dark']) for (const reducedMotion of ['reduce', 'no-preference']) {
    const context = await browser.newContext({ viewport: { width, height: 900 }, colorScheme, reducedMotion });
    try {
      for (const [kind, url] of [['adapter', process.env.ADAPTER_URL], ['board', process.env.BOARD_URL], ['standalone', process.env.STANDALONE_URL]]) {
        const tab = await context.newPage();
        try {
          tab.on('pageerror', error => errors.push({ kind, width, colorScheme, reducedMotion, error: String(error) }));
          await tab.goto(url, { waitUntil: 'load', timeout: 20000 });
          const page = tab.frames().find(frame => frame.parentFrame() === tab.mainFrame() && /[?&]frame=1(&|$)/.test(frame.url())) || tab;
          await page.waitForSelector('section.topic', { timeout: 15000 });
          await page.evaluate(theme => { document.documentElement.dataset.theme = theme; }, colorScheme);
          if (kind === 'adapter' && width === 420) await page.click('#tabbar [data-tab="board"]');
          await page.waitForFunction(() => [...document.querySelectorAll('iframe.doc-frame')].every(frame => frame.style.height), null, { timeout: 15000 });
          const stem = `${kind}-${width}-${colorScheme}-${reducedMotion}`;
          const settle = () => page.evaluate(() => new Promise((resolve, reject) => {
            let last = '', stable = 0, frames = 0;
            const tick = () => {
              const nodes = [...document.querySelectorAll('.board-toolbar,.topic,.tophead')].filter(node => node.getClientRects().length);
              const value = JSON.stringify(nodes.map(node => { const r = node.getBoundingClientRect(); return [r.top, r.height]; }));
              stable = value === last ? stable + 1 : 0; last = value;
              if (stable >= 6) resolve(); else if (++frames >= 300) reject(Error('capture geometry did not settle')); else requestAnimationFrame(tick);
            };
            requestAnimationFrame(tick);
          }));
          const capture = async state => {
            await settle();
            const file = path.join(dir, `${stem}-${state}.png`);
            await tab.screenshot({ path: file });
            manifest.push({ kind, width, colorScheme, reducedMotion, state, url, file });
          };
          await capture('top');
          await page.evaluate(() => {
            const col = document.getElementById('bcol');
            const scroller = col && col.getClientRects().length && /auto|scroll/.test(getComputedStyle(col).overflowY) ? col : document.scrollingElement;
            scroller.scrollTop = Math.min(450, scroller.scrollHeight - scroller.clientHeight);
          });
          await capture('floating-sticky');
          if (kind === 'adapter' && width === 420) {
            await page.click('#tabbar [data-tab="convo"]');
            await capture('conversation-composer');
          }
        } finally { await tab.close(); }
      }
    } finally { await context.close(); }
  }
  if (errors.length) throw Error(JSON.stringify(errors));
} finally {
  fs.writeFileSync(path.join(dir, 'capture-manifest.json'), JSON.stringify({ captures: manifest, errors }, null, 2));
  await browser.close();
}
JS
```

This is bounded to eight contexts and 24 pages per revision, sequentially, with
52 screenshots per revision (top/floating for all hosts, plus four narrow
conversation/composer captures). Check that each named floating/sticky capture
actually reaches its state; otherwise record a coverage gap, not a match.
The wide adapter captures include conversation/composer beside the board.

Inspect each matching before/after pair **side by side**, using the manifests to
pair kind/viewport/theme/motion/state, including full/floating toolbar, topic/card
headers, stream/panel, sticky head, and conversation/composer. Record
`compared <after-path> vs <before-path>: match|mismatch` for every pair. Explicitly
attribute any volatile timestamps/fixture identifiers; do not hide a geometry,
typography, action-ownership or unexpected pixel difference behind that caveat.
No earlier mockup or peer PASS substitutes for the actual baseline capture.

## 3. Bounded existing behavior probes

With the same per-revision fixture (no additional build), run each selected
scenario once through the preparation checkout's current entry point. Each
command supplies its fresh `--capture-dir` under that revision. Compare baseline
and candidate receipts; never edit a predicate merely to make the split pass.

| Scenario | URL | Width/height | Additional options |
|---|---|---|---|
| `keep` | adapter | 1900/900 | `--framed-middle` |
| `history-fit` | adapter | 1900/900 | none |
| `panel-header` | standalone | 420/900 | none (scenario owns its responsive matrix) |
| `toolbar` | adapter | 1900/900 | none |
| `overview` | adapter | 420/900 | `--case anchors` |
| `overview-motion` | adapter | 1900/900 | `--case pins` |
| `shared-controls` | adapter | 1900/900 | none |
| `shared-actions` | adapter | 420/900 | `--case persistence` |

Command form: `node scripts/board-headless.mjs <scenario> --url <printed-url>
--width <W> --height 900 --capture-dir <fresh-case-dir> <options>`.
Eight commands per revision maximum, no broad acceptance sweep or automatic
retry. UI-T5's obsolete linked-ticket predicates are intentionally retained;
shared-controls/shared-actions can stop there on this baseline. The existing
open-picker/card-maximum limitations and any observed pre-existing failure stay
failures/coverage gaps, never passes. Re-encountering a root cause triggers the
worker's non-convergence stop; remaining probes are reported omitted. UI-T5,
UI-C3 and 777 fixes require separate root integration authority and a repinned
recipe, not a silent patch inside this batch.

Stop only the exact owned fixture supervisor using the harness's existing
shutdown path, wait for its own daemon and browser children to exit, and record
that disposition before starting the other revision or relinquishing the slot.
No broad kill/process matching or assumption that a signal proves termination.
Retain both isolated fixture trees, source pins, logs, captures and manifests.

## Completion boundary

After the allocated batch, independent one-pass lite review still runs before
recording Phase 1. Any missing capture/known failure is listed explicitly. No
Result, closure, merge, deployment or UI-start resume is authorized by preparation
or this recipe; MAIN owns integration, semantic preview and the pause boundary.
