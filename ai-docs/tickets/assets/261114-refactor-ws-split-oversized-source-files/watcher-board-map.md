# Split map: Claude mod `watcher.test.ts` (ws + wsflow) and `scripts/board-headless.mjs`

Planning record for `261114-refactor-ws-split-oversized-source-files`. Nothing here
has been executed in the tree. Line ranges are at `616271b51`; both targets are
byte-identical at `f66857935` (checked with `git diff 616271b51 HEAD -- <paths>`).
Re-check against the execution base before moving anything.

Binding rules (from the ticket, applied to TS/JS): move-only, no renames, no
extracted helpers, no logic edits, no test redesign, comments move with their
declarations, only the import/export scaffolding the move needs, one move-only
commit per destination file, compatibility T0, no size-guard limit changes. Every
resulting file stays under 2000 lines.

A and B touch disjoint trees (A: `agents-plugin*/hooks/claude-mod/`, plus
optionally `agents-plugin-pi/workflow-bundles/`; B: `scripts/` only). No file and
no gate is shared, so they can run as **separate phases** in either order.

---

## A. `agents-plugin{,-wsflow}/hooks/claude-mod/watcher.test.ts` (2881 lines each)

### Mechanism facts

- **Mirror:** a hand-synced, byte-identical copy. There is no generator. The drift
  guard is `TestWsflowHooksMirrorUpToDate`
  (`agents-plugin-tool/internal/wsrsrc/wsflow_hooks_mirror_test.go:21`). It walks
  both whole directories (`collectTreeBytes`, `wsflow_mirror_test.go:20`), so it
  reports **missing** and **extra** files as well as byte differences. A new file
  must exist in both packages within the same commit. Resync:
  `rsync -a --delete agents-plugin/hooks/claude-mod/ agents-plugin-wsflow/hooks/claude-mod/`
  (`ai-docs/manuals/wsflow-mirroring.md`, "Claude Mod Mirror").
- **Runner:** `claude plugin test <plugin dir>`. It runs every `*.test.ts` and
  `*.test.tsx` under the directory, one child per file, with the kit imported from
  `'claude-code/testing'`. There is no package.json or tsconfig. Discovery is by
  glob, so new `*.test.ts` files are picked up and need no registration.
  - **Measured:** a `*.test.ts` file that declares no `test()` **fails** the run
    (`(fail) the file did not load — it declares no test(): nothing ran`). So the
    shared harness cannot live in a `*.test.ts` file.
  - Test files run as separate children, so a test file can never import another
    test file. Doing so would register that file's describes a second time.
- **Hook registration:** `hooks/hooks.json` `modules` names only
  `./claude-mod/register.ts`. Two tests pin this:
  `agents-plugin/tests/test_claude_hooks_manifest.py:97` and
  `agents-plugin-wsflow/tests/test_wsflow_claude_hooks_manifest.py:30`.
  `agents-plugin/tests/test_daemon_workflow_bundle.py:154-171` walks the relative
  import closure from register.ts. A test-only module sits outside that closure,
  so none of these change.
- **Packaging:**
  - `install.sh:462-505` copies the whole `agents-plugin/` tree, and the Claude
    marketplace installs the whole package, so test files already ship there.
  - The Pi workflow bundle (`agents-plugin-pi/scripts/prepare-workflow-bundles.py:25`)
    takes `hooks/**` but **excludes `*.test.ts` and `*.test.tsx` only**. The
    bundles are tracked (`agents-plugin-pi/workflow-bundles/{ws,wsflow}.zip`,
    `manifest.json`).
  - CI step "Check workflow bundle freshness" (`.github/workflows/ws-mcp-ci.yml:83-85`)
    and the release workflow (`ws-mcp-release.yml:110`) run `--check`.
  - **Measured:** `--check` passes at HEAD `f66857935`. Adding a non-test `.ts`
    under `hooks/claude-mod/` makes it fail (`workflow bundle stale or invalid: ws.zip`).
    Adding a new `*.test.ts` keeps it current.
  - Native-pi first-party is already stale at HEAD (`ws-pi-payload.py --check-source`
    reports `FIRST-PARTY stale`). It is regenerated at release regardless of this work.

### Owner/lead decision A-D1 (required before Phase A)

Every describe block except `pure helpers` uses the shared `harness()` (lines
57-278). The only home a move-only split can give that harness is a **non-test**
module. Options:

1. **Recommended: `watcher-harness.ts` in `hooks/claude-mod/`** (prototype verified,
   below). Consequences:
   - The file ships in Claude installs. That is harmless and consistent with the
     test files already shipping.
   - The file enters the Pi workflow bundles: about 258 lines of unused, type-only
     test code.
   - The same series must regenerate the bundles with
     `python3 agents-plugin-pi/scripts/prepare-workflow-bundles.py` (writes 3 files),
     either inside the harness commit or as a separate
     `chore(pi): regenerate workflow bundles` commit, which follows the
     `f66857935` precedent. Otherwise CI's freshness step fails, as it did on
     `616271b51`.
   - T0: nothing loads the file at runtime.
2. Put the harness outside `hooks/` (for example `agents-plugin/tests/claude-mod/watcher-harness.ts`,
   imported as `../../tests/claude-mod/watcher-harness`). The runner accepts this
   (measured: 140 pass), and the file stays out of the Pi bundle. However, it needs
   a second copy in `agents-plugin-wsflow/tests/` that **no drift guard covers**.
   Covering it would mean changing the Go guard, which is not move-only.
   Not recommended.
3. Add a test-helper suffix to the exclusion list in `prepare-workflow-bundles.py`.
   This is a packaging-script change, not move-only. That script is also hashed by
   the native-pi first-party manifest.

The rest of this section assumes option 1.

### Destinations (option 1)

Each destination below is one commit. Each commit applies the identical edit in
both packages, and the wsflow copy is produced by rsync.

| Commit | File | Lines | Tests |
|---|---|---:|---:|
| A1 | `watcher-harness.ts` (new, non-test) | 258 | 0 |
| A2 | `watcher-board.test.ts` (new) | 857 | 46 |
| A3 | `watcher-dashboard.test.ts` (new) | 451 | 22 |
| — | `watcher.test.ts` (retained) | 1326 (after A1: 2624, after A2: 1774) | 72 |

Baseline is 140 tests in 1 file. After the split: 140 tests in 3 files, and the
`describe > test` name set is identical (measured).

**A1 `watcher-harness.ts`**
- Header: three type-only import lines, then a blank line:
  - `import type { OwnPrompt } from './captions'`
  - `import type { InboxCommand, InboxItem } from './dashboard'`
  - `import type { DrainMode, DrainResult, WaitOutcome, WatcherEdges } from './watcher'`
- Moved verbatim: lines 43-289, then a blank line, then 1392-1397.
  - 43-289 contains: `type Deferred`, `deferred`, `tick`, both doc comments at
    53-54 (the first of them is the harness comment, orphaned above `joined`),
    `joined`, `harness`, `RECV`, `mailA`, `mailB`, and `arrive` with its comment.
  - 1392-1397 is `wake` with its comment.
- Scaffolding: an `export ` prefix on exactly these lines and no others:
  - 44 `deferred`
  - 49 `tick`
  - 57 `harness`
  - 280-282 `RECV`, `mailA`, `mailB`
  - 285 `arrive`
  - 1393 `wake`
- `Deferred` and `joined` stay module-private. `joined` is used only inside
  `harness`. Line 36 of the dashboard section says "joined" only in a comment.
- No `claude-code/testing` import is needed, because the harness uses no
  `expect`/`test`.
- Changes to `watcher.test.ts`:
  - Remove 43-290 (including the trailing blank) and 1392-1398.
  - Lines 1-17 (the `./register` import) stay byte-identical.
  - Change the `./watcher` import to: `batchLine, batchPrompt, BOARD_FIRST_WAIT_MS,
    COMPACTION_CONTINUATION_WAIT_MS, createWatcher, EMPTY_DRAIN_BACKOFF_MS,
    formatBatch, formatEnvelope, formatNotice, LEVER_HOLD_MS, mailItems,
    mapWaitExit, nextBackoff, parseRecvText, STEER_DRAIN_WAIT_MS, type DrainMode,
    type DrainResult, type WakeKind`.
  - Drop the `./captions` import.
  - Change the `./dashboard` import to `import type { InboxCommand } from './dashboard'`.
  - Add `import { arrive, deferred, harness, mailA, mailB, RECV, tick, wake } from './watcher-harness'`.
- Bundle regeneration goes here or in the chore commit that follows it (A-D1).

**A2 `watcher-board.test.ts`**
- Imports:
  - `import { describe, expect, test } from 'claude-code/testing'`
  - from `./watcher`: `BOARD_FIRST_WAIT_MS, COMPACTION_CONTINUATION_WAIT_MS,
    createWatcher, EMPTY_DRAIN_BACKOFF_MS, formatBatch, LEVER_HOLD_MS,
    STEER_DRAIN_WAIT_MS, type DrainMode`
  - from `./watcher-harness`: `arrive, deferred, harness, mailA, RECV, tick, wake`
- Moved: lines 1399-2241, which are the describes `board messages`,
  `board messages during a compaction`, and
  `board messages behind a requested compaction`.
- Changes to `watcher.test.ts`: remove 1399-2242, and drop the five `*_MS`
  constants and `type DrainMode` from its `./watcher` import.

**A3 `watcher-dashboard.test.ts`**
- Imports:
  - the testing kit (as in A2)
  - from `./watcher`: `createWatcher, formatBatch, type DrainResult, type WakeKind`
  - `import type { InboxCommand } from './dashboard'`
  - from `./watcher-harness`: `arrive, deferred, harness, mailA, mailB, RECV, tick, wake`
- Moved: lines 2243-2687. That is the `interruptWake` helper with its comment
  (used only here), `dashboard slash commands`, and `dashboard interrupt`.
- Changes to `watcher.test.ts`: remove 2243-2688, drop `type WakeKind` and the
  `./dashboard` import, and drop `wake` from the harness import.

**Retained `watcher.test.ts` (1326 lines)**
- Imports:
  - 1-17 unchanged
  - from `./watcher`: `batchLine, batchPrompt, createWatcher, formatBatch,
    formatEnvelope, formatNotice, mailItems, mapWaitExit, nextBackoff,
    parseRecvText, type DrainResult`
  - from `./watcher-harness`: `arrive, deferred, harness, mailA, mailB, RECV, tick`
- Body: lines 291-1390 and 2689-2881. These are the describes
  `a submit that found a main-thread turn started first (captions on)`,
  `mid-turn steer`, `direct mode`, `notice mode`, `lifecycle`, the comment at
  1106-1110 plus `server drop recovery`, `a crashed push loop`, and `pure helpers`.
- Optional, if the lead wants the retained file under about 1200 lines: a fourth
  commit moves 949-1390 (`lifecycle`, `server drop recovery`, `a crashed push loop`;
  442 lines) into `watcher-lifecycle.test.ts`, leaving about 890 lines. It is not
  required.

### Hazards (A)

- `wsflow_hooks_mirror_test.go:21-50`: a mirror commit that misses a new file or
  carries a stale copy fails on missing, extra, or byte-differs. Always rsync with
  `--delete` before committing.
- `prepare-workflow-bundles.py:25`: the non-test harness makes the bundle stale.
  CI `ws-mcp-ci.yml:83-85` fails until the bundles are regenerated (decision A-D1).
- Runner rule: a `*.test.ts` file with no `test()` fails to load, and a test file
  cannot be imported. This is why the harness is a non-test module.
- Type-only imports are erased at runtime, so a missing `type X` import never
  fails a test run. That is why the static unresolved-name gate below is
  mandatory.
- Module-level state: there is no top-level `let`. `mailA` and `mailB` are
  module-level arrays. Each test file runs in its own child, so within a file
  sharing is unchanged and across files nothing was shared before either.
- Docs citing the test, for note only:
  - `agents-plugin-tool/internal/mcp/lead_mail_test.go:18` cites
    `watcher.test.ts` "the batch prompt, as the daemon reproduces it…". That is
    line 2800, inside `pure helpers`, which **stays** in `watcher.test.ts`, so the
    cite stays correct.
  - Done tickets cite the file by name only.
- Pre-existing, unrelated baseline failure, the same in both packages:
  `hooks/claude-mod/dashboard.test.ts` fails to load with
  `cannot import "node:fs" … a hooks module imports its own files by relative path
  and "claude-code", nothing else`. The kit no longer allows `node:fs` in mod
  tests, so this file's whole suite is not running. Report it separately. It must
  not be fixed inside a move commit.
- Timing: no unmerged local branch touches `watcher.test.ts`. Among open tickets,
  `261116-research-ws-hosted-session-first-tier` only mentions it.

### Gates (A), measured on macOS (node v25.9.0, claude CLI with `plugin test`)

1. Scoped watcher run (fast). In a `/tmp` copy:
   - `git archive <rev> agents-plugin | tar -x -C /tmp/x`
   - delete every other `*.test.ts*` under `hooks/claude-mod/`
   - `nice -n 19 claude plugin test /tmp/x/agents-plugin`

   Measured about 0.8 s. Baseline: **140 pass, 0 fail, 1 file**. Split
   prototype: **140 pass, 0 fail, 3 files**. Compare the sorted `(pass)` name
   lists after stripping timings; they must be identical.
2. Full suites, in-tree. Read-only: `git status` was unchanged by them.
   - `nice -n 19 claude plugin test agents-plugin` and
     `nice -n 19 claude plugin test agents-plugin-wsflow`: about **2 min 23 s
     each**. Baseline **763 pass / 1 fail** (the `dashboard.test.ts` load failure
     above).
   - `dashboard-engine.test.ts` has a known load flake
     (`261110-chore-ws-claude-mod-dashboard-engine-test-load-flake`), so run these
     niced and alone.
3. Mirror. `diff -r agents-plugin/hooks/claude-mod agents-plugin-wsflow/hooks/claude-mod`
   is identical at HEAD. Also run, from `agents-plugin-tool/`,
   `go test ./internal/wsrsrc -count=1 -run TestWsflowHooksMirrorUpToDate`, which
   measured `ok` in about 0.6 s. Go is installed on this Mac. The ticket's "no local
   go" rule was written for the Windows PC.
4. `python3 agents-plugin-pi/scripts/prepare-workflow-bundles.py --check`, about
   0.06 s. It passes at HEAD and must pass again after the bundle regeneration.
5. Static name check, per commit, on every affected file:
   `node /tmp/wsplit/tools/unresolved.mjs <files>` must print nothing; the baseline
   prints nothing. The tool's negative controls catch a dropped value import
   (`deferred`) and a dropped type import (`InboxCommand`).
   `python3 /tmp/wsplit/imports.py <files>` must report `missing=[] unused=[]`.
6. Declaration identity:
   `python3 /tmp/wsplit/identity.py <base watcher.test.ts> -- <all 4 result files>`
   must print `units 26 names 24 OK`. Its negative controls fail on a
   one-character body change and on a deleted declaration.

---

## B. `scripts/board-headless.mjs` (2475 lines)

### What it is and who runs it

- It is a repository-local developer CLI, and its header says it is not shipped
  (lines 2-4). It runs headless Chromium against dashboard pages, using Playwright
  from the npx cache.
- There is no package.json script, no Makefile target, and no CI workflow for it.
- The only automated caller is the opt-in Go test `TestUnifiedHistoryBrowser`
  (`agents-plugin-tool/internal/wsdashboard/history_unified_browser_test.go:24-27`).
  It skips unless `WS_BOARD_ACCEPTANCE_CASE` is set. At `:146` it executes
  `node <harness>/scripts/board-headless.mjs <scenario> …` by path, with
  `cmd.Dir=harness`. The entry path must therefore stay.
- Tickets and docs cite `node scripts/board-headless.mjs …` commands.
- Guard coverage: `git check-attr` reports no `linguist-*` and no `ws-size-guard`
  attributes, and `mjs` is in `defaultMergeSizeExtensions`
  (`git_merge_size.go:47`). The file is covered.
- Precedent: an earlier split already produced `scripts/board-headless/toolbar-overview.mjs`
  (1240 lines) and `shared-actions.mjs` (304), with `split.test.mjs` (88) as their
  opt-in proof. It used **factory DI wrappers** (`export function
  createToolbarOverview({assert, log, …}) { …verbatim col-0 bodies…; return {…} }`,
  bound at `board-headless.mjs:1066-1072`). `scripts/board-history-unified.mjs`
  uses the same injected-helper style.

### Constraints that shape any split

- **Top-level side effect:** `main();` at line 2475. A sibling module that imports
  the entry would run the CLI whenever that sibling is loaded first, for example
  by a test.
- **`import.meta.url` is location-dependent:**
  - line 183 `REPO = path.resolve(dirname(import.meta.url), "..")`
  - line 1450 `scenarioHistory` re-executes **itself** via
    `fileURLToPath(import.meta.url)` (`spawnSync(process.execPath, argv)`)

  Moving `REPO` or `TOOL_DIR`, or any code that uses them (`seedSession`/`serve`
  251-435 and `openBoard` 691-764, at line 733), or moving `scenarioHistory`
  (1445-1903), into `scripts/board-headless/` silently changes behavior. These must
  stay in the entry. Otherwise the line would have to be edited, which is a logic
  change.
- Because every scenario calls `openBoard`, which must stay, **moving any
  scenario requires one of two things**: (a) importing back from the entry, which
  creates a cycle, or (b) a factory wrapper.

### Owner/lead decision B-D1 (required before Phase B)

1. **B-cycle (strictly move-only; prototype verified).**
   - Siblings `import { … } from "../board-headless.mjs"`.
   - The entry gains one `export { … }` list, so the declaration bytes stay
     untouched.
   - This is safe when the file runs as `node scripts/board-headless.mjs`, which
     is the only way anything runs it today:
     - ESM evaluates the siblings first, and their top level holds only function
       declarations and uninvoked `const X = () => {…}` page functions.
     - `Failure`, `BROWSERS`, and `qa` keep one module instance.
   - Hazard: a future direct import of `motion.mjs` or `history.mjs`, from a test
     for example, would run `main()`. Add a one-line comment above the export list,
     as in the prototype. That comment is the only non-import line and needs the
     lead's approval.
2. **B-factory (follows the precedent, not strictly move-only).**
   - Wrap each moved group in `export function createX({deps}) { … return {…} }`.
     Bodies stay byte-identical at column 0.
   - Bind each group in the entry with a destructuring line, the same way
     `createToolbarOverview` is bound.
   - There is no cycle, and the groups are unit-testable without running `main`.
   - It adds a wrapper, which the ticket's "no wrappers" rule forbids unless the
     owner makes an exception.
3. Leave the file whole and exempt it. That needs a `.gitattributes ws-size-guard`
   entry, which is a guard-limit change and is forbidden here. Owner only.

### Destinations (B-cycle; same ranges apply to B-factory)

| Commit | File | Lines | Content (616271b51 ranges) |
|---|---|---:|---|
| B1 | `scripts/board-headless/in-page.mjs` (new; leaf, no cycle) | 245 | 438-680: `INIT` and `IN_PAGE` with their comments + `export { INIT, IN_PAGE };` |
| B2 | `scripts/board-headless/motion.mjs` (new) | 490 | 861-1064: `scenarioKeep`, `scenarioObserver`; 1074-1354: `BANDS_PAGE` with its comment, `mermaidDrawn`, `scenarioBands`, `scenarioPill` |
| B3 | `scripts/board-headless/history.mjs` (new) | 514 | 1905-2411: `scenarioHistoryRegressions`, `scenarioPanelHeader`, `scenarioHistoryFit`, each with its comment |
| — | `scripts/board-headless.mjs` (retained) | 1242 (after B1 about 2232, after B2 about 1750) | 1-437, 682-860, 1066-1073, 1356-1904, 2413-2475 |

Existing names in `scripts/board-headless/` are `shared-actions.mjs`,
`split.test.mjs`, and `toolbar-overview.mjs`; none collide.

**What stays in the entry, and why**
- The header and section comments, including `// --- page helpers` (436) and
  `// --- scenarios` (859).
- `REPO`, `TOOL_DIR`, `LOGIC_ONLY`, `parseArgs`, `Failure`, `assert`, `sleep`,
  `log`, the Playwright loader, `childEnv`, `run`, `topicCommands`,
  `seedSession`, `waitFor`, `serve`, `BROWSERS`, `openBoard`, `waitQuiet`,
  `scrollToMiddle`, `growFrame`, `segments`, `assertChanged`, and `trace`. These
  stay because of `REPO` and `BROWSERS` lifetime.
- The factory binding at 1066-1072.
- `HISTORY_PAGE`, `growEntry`, `arrivalChecks`, and `scenarioHistory` (1356-1903),
  because of the `import.meta.url` self-exec.
- `SCENARIOS` and `main` (2413-2475).

**Scaffolding per commit**
- **B1:** the entry adds `import { INIT, IN_PAGE } from './board-headless/in-page.mjs';`
  after line 179.
- **B2:**
  - `motion.mjs` line 1: `import { Failure, assert, assertChanged, growFrame, log,
    openBoard, qa, scrollToMiddle, segments, sleep, trace, waitQuiet } from
    "../board-headless.mjs";`
  - The tail of `motion.mjs` is
    `export { scenarioKeep, scenarioObserver, scenarioBands, scenarioPill };`.
  - The entry adds the motion import and `export { Failure, assert, assertChanged,
    growFrame, log, openBoard, qa, scrollToMiddle, segments, sleep, trace,
    waitQuiet };`.
- **B3:**
  - `history.mjs` imports `fs`, `os`, and `path`, copying the entry's
    `"node:…"` lines, plus `{ HISTORY_PAGE, LOGIC_ONLY, assert, log, openBoard,
    sleep, waitQuiet }` from the entry.
  - It ends with an export list.
  - The entry adds the history import and appends `HISTORY_PAGE, LOGIC_ONLY` to
    its export list.

### Hazards (B)

- `board-headless.mjs:1450`: the self-re-exec path. `scenarioHistory` must not
  leave the entry.
- `:183-184`: `REPO`/`TOOL_DIR` are derived from the entry's location.
- `:2475`: `main()` runs on load. Under B-cycle, only the entry may be loaded
  first.
- `:1068`: `qa` is a destructured `const` from `createToolbarOverview`. The
  siblings read it only at call time, so there is no TDZ problem. Keep it that
  way: never reference an imported binding at a sibling's top level.
- `Failure` identity: `main` checks `e instanceof Failure` (line 2468). The class
  is shared only while the entry has a single module instance; Node realpaths
  both the main entry and the imports.
- `scripts/board-headless/split.test.mjs:14-49`: the opt-in
  `BOARD_SPLIT_BASELINE` test reconstructs the whole entry from the earlier split.
  It is commented "not a permanent freeze", and it will not match after this
  split. It is skipped by default, and its 4 default tests still pass on the
  prototype. Note only. Do not redesign it.
- Header comment lines 164-169 ("Adding a scenario … using the shared helpers
  below … add it to SCENARIOS") become slightly loose. Note only.
- Docs citing `file:line`, which will drift (note only):
  - `ai-docs/tickets/todo/261114-feat-ws-whiteboard-compact-floating-toolbar.md:84`
    (`board-headless.mjs:1370`)
  - dropped tickets and whiteboard panels (`:1187`, `:1200`, `:1228`)
  - untracked `shared/docs/handoff-ux.md:106`
- **Parallel work and timing:**
  - Ready ticket `261115-feat-ws-dashboard-board-reorder-hold-rewrite` (:202-205)
    plans to port new `hold*` scenarios and a `board-headless.test.mjs` onto
    "develop's `scripts/board-headless/` layout".
  - Idea tickets `261115-bug-board-headless-scenario-gaps` and
    `261114-bug-ws-dashboard-history-paint-resizeobserver-loop` target this file.
  - Unmerged local branch `impl/develop/perm-unlit-icon` (last commit 2026-11-14)
    has 7 commits touching it.
  - Run the B split at a quiet point and land it or park it against those.

### Gates (B), measured on macOS (node v25.9.0)

1. `node --check` on the entry and every new sibling: about 0.05 s each.
2. Usage smoke: `node scripts/board-headless.mjs` with no arguments loads the
   whole module graph, including the cycle and the factory binding. It prints the
   usage text listing all 21 `SCENARIOS` keys and exits 2. The output must be
   byte-identical before and after (verified on the prototype). It takes about
   0.07 s and writes nothing.
3. Stub-browser smoke: `/tmp/wsplit/tools/smoke.sh <root> <out>`.
   - It runs all 21 scenarios with `PLAYWRIGHT_MODULE` pointing at a stub whose
     `chromium.launch` throws, using `--url http://127.0.0.1:9/`, a private
     `TMPDIR`, and cwd `/tmp`.
   - Each scenario crosses the module boundary into `openBoard`/`loadPlaywright`.
   - Baseline: 19 scenarios report `Chromium failed to launch`, `overview-final`
     fails on missing args, and `history-unified` requires `--fixtures`.
   - The output must match the baseline after stack `:line:col` numbers are
     normalized (verified). About 2 s total.
4. `node --test scripts/board-headless/split.test.mjs`: 4 pass, 1 skipped, about
   0.17 s. It must stay the same.
5. Static name check: `node /tmp/wsplit/tools/unresolved.mjs <entry> <siblings>`.
   The union of unresolved names must stay within the baseline set
   `{__bh, process}`; these are page and Node globals. Its negative control
   catches a dropped `qa` import.
6. Declaration identity:
   `python3 /tmp/wsplit/identity.py <base entry> -- <entry> <siblings>` must print
   `units 41 names 43 OK`. Import lines and comment-prefixed `export {…}` lists are
   treated as scaffolding.
7. Optional, real browser: `node scripts/board-headless.mjs serve …` plus one
   scenario from each moved group (`keep`, `bands`, `history-fit`) against both
   page kinds. Only if Playwright and Chromium are present. Not run in this pass.

---

## Declaration-identity proof method (both targets)

All scripts live in `/tmp/wsplit/` and nothing is in the tree. `/tmp` is
ephemeral, so recreate them from these descriptions if they are gone.

- `split.py` and `split_bh.py` regenerate the exact destination files from
  `git show 616271b51:<path>` using the ranges above. Diffing an implementer's
  commit against their output is the fastest review.
- `identity.py BASE -- OUT...`
  - Splits each file into top-level units, which are paragraphs that start after
    a blank line followed by a column-0 line.
  - Drops import units, `} from` units, and units that are only comments followed
    by an `export {…}` list.
  - Strips a leading `export ` from column-0 lines.
  - Asserts that the before and after unit multisets are equal byte for byte, and
    that the multisets of declared names and `describe(` titles are equal.
- `tools/unresolved.mjs FILE...`
  - Uses the TypeScript 5.9.3 compiler API from
    `~/.npm/_npx/5abcb55351612887/node_modules/typescript` with
    `allowJs`/`checkJs`, lib `es2022` and `dom`, and no types.
  - Lists every `Cannot find name` identifier (TS2304/2552/258x/259x).
  - Any name not unresolved in the baseline is a missing import. This covers
    erased type-only imports as well.
- `imports.py FILE...` reports, per file, imported names that are unused and
  referenced names that are not imported. It strips strings and comments.
- Functional check: the test-name set (A) and the usage and stub-smoke output (B)
  must match the baseline, as listed in the gates above.
