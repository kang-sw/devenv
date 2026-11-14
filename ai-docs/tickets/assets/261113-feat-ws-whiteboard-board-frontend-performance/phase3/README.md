# Phase 3 evidence and LOOK blocker

This checkpoint is **not** a completed Phase 3 Result. The implementation and
contract tests landed, but the original long-board LOOK comparison did not
establish parity. The ticket remains open. No live activation or integration
was performed.

## Source identities and verification

- Baseline: `d1c3b9d6baf433c48b4ec61313dbf6c26b577cdf`.
- Candidate implementation: `3ec591d82123fdd07147a39a9e4950c81b0e62a2`.
- Candidate contract tests: `1341947b38ab680761835b5fb65276bffe581c63`.
- The iframe-shell prerequisite `3310356af5ed1105d554bcb718a0af5cc68b4ae5`
  is an ancestor of the baseline.
- Test leaf command, from `agents-plugin-tool`:
  `go test -timeout 20m ./internal/wsdashboard ./internal/wswhiteboard`.
  Complete final output:
  ```text
  ok  github.com/kang-sw/devenv/internal/wsdashboard  140.774s
  ok  github.com/kang-sw/devenv/internal/wswhiteboard  (cached)
  ```
- Worker command, from `agents-plugin-tool`:
  `go test -timeout 20m ./internal/mcp -run 'Dashboard|Whiteboard|Board|Panel|Theme|Closed|Topic|Wiki' -count=1`.
  Complete output:
  ```text
  ok  github.com/kang-sw/devenv/internal/mcp  66.371s
  ```
- `node --check agents-plugin-tool/internal/wsdashboard/assets/dashboard.js`,
  `go test ./internal/wsdashboard -run '^TestSplitBoardTopics$' -count=1`,
  and `git diff --check` passed before the evidence-only checkpoint.
- Independent one-pass lite review: `lite-review.md`. No Critical findings;
  one Important validation gap, unchanged LOOK not established. The reviewer
  ran no tests and established no additional production logic defect.

## Exact commands and original locations

Commands were run from the isolated worktree
`/Users/kang-sw/.ws-worktrees/devenv/dense-hardship-outclass`. The original
artifacts remain at `/tmp/ws-board-topics-phase3.3pBANc`. Files copied here
were checked byte-for-byte against those originals. The baseline source
archive and test binaries are temporary build inputs, not tracked assets.

Baseline extraction and alternating measurement:

```sh
ARTIFACTS=$(mktemp -d /tmp/ws-board-topics-phase3.XXXXXX)
mkdir -p "$ARTIFACTS/before"
git archive d1c3b9d6b -- agents-plugin-tool | tar -x -C "$ARTIFACTS/before"
node scripts/board-perf.mjs run \
  --tree "$ARTIFACTS/before/agents-plugin-tool,$PWD/agents-plugin-tool" \
  --label before,after \
  --out "$ARTIFACTS/before.json,$ARTIFACTS/after.json" \
  --scenarios c --runs 3 --rates 4,6 --gc \
  > "$ARTIFACTS/run.log" 2>&1
node scripts/board-perf.mjs compare \
  /tmp/ws-board-topics-phase3.3pBANc/before.json \
  /tmp/ws-board-topics-phase3.3pBANc/after.json \
  --targets c:ScriptDuration
node /tmp/ws-board-topics-phase3.3pBANc/visual.mjs \
  > /tmp/ws-board-topics-phase3.3pBANc/visual.log 2>&1
node scripts/board-perf.mjs diff \
  /tmp/ws-board-topics-phase3.3pBANc/before-look \
  /tmp/ws-board-topics-phase3.3pBANc/after-look
node /tmp/ws-board-topics-phase3.3pBANc/diagnose.mjs \
  > /tmp/ws-board-topics-phase3.3pBANc/diagnose.log 2>&1
```

`visual.mjs` and `diagnose.mjs` preserve the exact executable paths, browser
setup, viewport, update commands, delays, and observation instrumentation.
They start only `TestBoardPerfServe`, which owns temporary session/project
state and an ephemeral loopback server, and stop it via stdin `quit`.
The harness built the baseline binary at
`/var/folders/qz/mxbhy9fs5zq_ymx5vr1_ztv80000gn/T/board-perf-aQ37QY/mcp.test`
and the candidate at
`/var/folders/qz/mxbhy9fs5zq_ymx5vr1_ztv80000gn/T/board-perf-IBUQ4g/mcp.test`.
Reproduction on another machine must rebuild these identities and substitute
only the temporary executable paths. No launcher or shared daemon is needed.

## Measurements and event identity

`before.json`, `after.json`, and the complete `run.log` hold all samples.
Three runs per side alternate run by run; Chromium 148.0.7778.96; shape
8,6,4,2,60; garbage collection precedes absolute counts.

| Scenario (c), median | 4x baseline -> candidate | 6x baseline -> candidate |
|---|---|---|
| ScriptDuration ms | 2456 -> 1349 (-45.1%) | 3879 -> 2086 (-46.2%) |
| TaskDuration ms | 5872 -> 3582 (-39.0%) | 8832 -> 5394 (-38.9%) |
| Long-task time ms | 3414 -> 1299 (-62.0%) | 5393 -> 2126 (-60.6%) |
| LayoutCount | 355 -> 353 | 330 -> 335 |
| LayoutDuration ms | 271 -> 256 | 407 -> 384 |
| Nodes | 46602 -> 63164 (+35.5%) | 46251 -> 63252 (+36.8%) |
| Frames built | 20 -> 20 | 20 -> 20 |

The canonical incoming DOM snapshots retain more nodes. The Nodes increase
is disclosed, not hidden: Decision 12 prioritizes CPU over memory. The
harness prints `keep rule: FAIL` because Nodes rose; that automated rule is
Phase 1's rule, not Phase 3's completion criterion. The owner accepted the
CPU/event/identity evidence while keeping this cost disclosed.

`visual.log` records the baseline's 20 full-region updates versus the
candidate's initial full board plus 20 `board-topics` updates, every one
containing only T00. Update payloads fell from approximately 1.14 MB to
95-106 KB. T01 remained the same element on both sides.

## Original LOOK mismatch (not a pass)

Compared `after-look/light-00.png` through `light-12.png` against the
same-named `before-look` captures: **mismatch**. All 26 originals are
preserved here without date, CSS, history, or source changes.

- Stops 00-05 differ by 0.010-0.052% of pixels, in clock labels.
- Stops 06-11 differ by 15.310-52.863% of pixels.
- `visual.json` shows that T04 is 1520.75px tall on the candidate versus
  1301.75px on the baseline, a 219px difference. Later topics are displaced.
- The side-by-side inspection of `before-look/light-06.png` and
  `after-look/light-06.png` shows the candidate exposing the preceding
  panel's clipped tail. It is not a clock-only change.
- Stop 12 differs by 0.437%. Full LOOK parity was never established.

## Diagnosis: a baseline history-snap defect also exists

`diagnose.mjs` observes actual T04 history style writes and ResizeObserver
callbacks on both exact source identities; it does not reset history,
change dates, force frame heights, or alter production functions. Reads can
influence scheduling, so this diagnostic is evidence of the defect, not a
replacement acceptance capture.

Both `baseline-trace.json` and `candidate-trace.json` record:

1. With base/clip 640px and two 219px frames landed, history live height is
   826.484375px. Entry e723 crosses the clip line by 97.390625px.
2. `snap` writes 738px; the crossing disappears.
3. Another 219px frame lands. Live height becomes 1045.484375px while the
   clip is 738px. Entry e723 crosses the new line by 218.390625px.
4. `snap` computes `base + ceil(extra)`, 640 + 219 = 859px, instead of
   extending the current 738px clip. Entry e723 still starts 97.390625px
   above the clip line and ends 191.859375px below it.
5. This partial entry remains after a further three seconds without any
   state reset. Both traces end at base 640, snapped/style/clip 859,
   live 1045.484375, with the same crossing and mounted frame heights.

The source is `agents-plugin-tool/internal/wswhiteboard/assets/stream.js`:
`snap` measures against the **current** clip at lines 53-64 but adds that
extra to `_wbHistoryBase` at line 65; only `.history-live` is observed at
lines 363-378, so changing the clip alone does not schedule another snap.
The file is unchanged by Phase 3. Its SHA-256 on baseline and candidate is
`06c1d085f74939951f8f1dd912c1eaa337016fe21c85f52b9a0f7d81aac9c558`.

**VERIFIED:** the unchanged baseline already exhibits a persistent,
iframe-delivery-order-sensitive partial-entry snap. **INFERRED:** different
frame-height delivery batches account for the original 219px baseline vs
candidate difference. The trace establishes the pre-existing mechanism,
not that Phase 3 can never affect its timing.

## Blocker and proposed resolution

The independent Important finding remains open. The owner directed the
worker to return a blocker if the cause pre-exists or needs another
contract, rather than weakening source, dates, or history fixtures to
certify parity. Therefore Phase 3 is not marked complete and the ticket is
not closed.

Proposed resolution: authorize a separate, regression-tested history-snap
repair for sequential iframe-height deliveries, then resume this Phase 3
from the retained branch and repeat the unchanged long-board LOOK check.
Retain the original mismatch captures. There is no permission here to
activate the daemon, integrate branches, or expand history behavior.
