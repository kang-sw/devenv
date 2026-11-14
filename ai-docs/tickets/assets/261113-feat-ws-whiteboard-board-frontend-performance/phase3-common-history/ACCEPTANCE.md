# Common-history Phase 3 acceptance evidence

This records verification, not a Phase 3 Result or independent acceptance.
The original failure remains immutable in the sibling `phase3/` directory.
The required existing independent reviewer has not yet accepted this evidence.

## Frozen inputs

- Baseline: `b84a0de4879168f4c4c0b1bfbd74ceeaad7d7bb7`.
- Candidate: `2b3e54f9ae745bc77f3fdd9840136306593e37ad`.
- Both contain the separately approved history-fit producer `8f24cec5b`
  through integration `28d857622`; iframe-shell `3310356af` is an ancestor.
- Both history streams hash to
  `5a7e86ab9dc8624e2296335d113d69dcd349aaa65c0e0cbb8765aa806634555a`.
- The entire `internal/wswhiteboard` tree, `scripts/board-perf.mjs`, and
  `scripts/board-headless.mjs` have an empty baseline/candidate Git diff.
- Exact archive, source inventory, module tree, executable and environment
  identities are in `manifest.json`, `*-files.sha256.json`, and `README.md`.
  Build/test exports were not edited; no branch integration was performed
  by the worker.
- Fresh test binaries: baseline
  `c1c1d27cd096cc1de64bb2468f7823243d32b14d73ceff57d392edf07710e9d4`;
  candidate
  `43c4befb1a871fbc51cd64ee092515cbc7e85c959f58dc79bd6ab16174e17dc2`.

The isolated LOOK servers used loopback ports 52987 and 53013. The saved
`browser/*-served.html.gz` decompress byte-for-byte to their original
responses. Both contain the exact frozen `stream.js` and side-specific
`dashboard.js` source bytes, checked by the driver. Response identities are:

| Side | Served session HTML SHA-256 | Dashboard JS SHA-256 |
|---|---|---|
| Baseline | 04b7428c74a319a9d1bdce86f04c3b57d85c9e8db04a997f5911d3dc61f08c54 | c2530b85e6e56db5c1609df70ce6697cfef419b52faa7f25358a0c6d56d8f72c |
| Candidate | b4eb6791f3eb35df438edab3f21398a39516ebe50bc7a08ff224892940aa73d4 | d837d8f100fe1f01421d55d67baf119d91cf6f84be56a72c03ab0486b95d4232 |

## Commands and isolation

Original scratch root: `/tmp/ws-board-topics-common-history.EWq6tq`.
`README.md:23-81` and `manifest.json` preserve all fresh build/test commands
and complete outputs. From the retained implementation worktree:

```sh
node --check /tmp/ws-board-topics-common-history.EWq6tq/acceptance.mjs
node /tmp/ws-board-topics-common-history.EWq6tq/acceptance.mjs look \
  > /tmp/ws-board-topics-common-history.EWq6tq/logs/look.log 2>&1
node scripts/board-perf.mjs diff \
  /tmp/ws-board-topics-common-history.EWq6tq/browser/baseline-look \
  /tmp/ws-board-topics-common-history.EWq6tq/browser/candidate-look \
  > /tmp/ws-board-topics-common-history.EWq6tq/logs/look-diff.log 2>&1
node /tmp/ws-board-topics-common-history.EWq6tq/acceptance.mjs sequential \
  > /tmp/ws-board-topics-common-history.EWq6tq/logs/sequential.log 2>&1
node /tmp/ws-board-topics-common-history.EWq6tq/acceptance.mjs perf \
  > /tmp/ws-board-topics-common-history.EWq6tq/logs/perf.log 2>&1
```

`acceptance.mjs` is the exact verification driver. `harness-library.mjs`
extracts the unchanged original `board-perf.mjs` function bodies, removes
only its command dispatcher, and exposes the original measurement helpers.
Performance uses the prebuilt, hash-checked binaries rather than rebuilding
from a moving checkout. The original scenario (c), metric sampling, quiet
wait, forced collection for absolute counts, 4x/6x throttling, synthetic
shape and alternating order remain unchanged.

The driver removes inherited WS variables and supplies owned scratch cache
and config directories. Each process runs only `TestBoardPerfServe`, with
temporary session/project data and an ephemeral port, and ends by stdin
`quit`. No launcher, installed runtime, shared daemon, live data/config,
installer, host setup, source promotion, merge or push was used.

## Fresh contract tests

All commands are uncached (`-count=1`); full output is preserved in `logs/`:

| Identity / test scope | Result |
|---|---|
| Baseline dashboard / whiteboard | PASS, 139.552s / 22.282s |
| Candidate dashboard / whiteboard | PASS, 142.402s / 19.218s |
| Candidate MCP dashboard/whiteboard/board/panel/theme/closed/topic/wiki | PASS, 66.566s |
| Candidate explicit split / incremental stream / client / WebSocket cases | PASS, 10.890s |

The changed-contract tests retain SSE/WebSocket parity, opt-out/full-region
fallbacks, reconnect and held-hash handling, shared render decomposition,
client recovery, accumulation and board/pin identity assertions.

## Matched-state LOOK

Procedure retains the original 1600x1000 light viewport, synthetic board,
natural timestamps, native styles, twenty one-line updates with 350ms gaps,
1200ms minimum stop wait, `scrollTop = i * clientHeight`, and dynamic
end-of-board stop enumeration. The common approved history changes the
board's natural extent: both sides have fifteen stops, 00-14. No old 640px
expectation or old thirteen-stop cap was imposed.

The additional required state gate observes geometry until unchanged for
750ms within a 15s bound, with no running animations, unsized mounted frame,
or pending near-topic selection. Far, unmounted topics retain their normal
provisional state; it must match between sides. Each corresponding snapshot
records all topic/frame boxes, frame attributes/heights, history chunk IDs,
pending/queued flags, clip/live geometry, boundary exposure and observer
errors **before** its screenshot. The comparison tolerance is 1px; the
actual maximum numeric difference across all fifteen pairs is **0px**, and
all nonnumeric fields match. No heights or captures were normalized.

`browser/look.json` contains every raw snapshot, event, source identity and
height-message trace. Both sides retained T01's original element identity.
The baseline sent twenty full board regions. The candidate sent an initial
full board and twenty `board-topics` updates, each containing only T00.
Both recorded zero ResizeObserver errors.

Compared `browser/candidate-look/light-00.png` through `light-14.png` against
the same-named `browser/baseline-look` files:

- Thirteen pairs are exactly pixel-identical.
- Stop 04 differs by 0.298%; stop 05 by 0.625%.
- Worker side-by-side inspection of both nonidentical pairs finds only
  natural relative-time labels (`just now` versus `1 min ago`) in the topic
  headers/recaps. Their rectangles and every recorded frame/history box
  remain identical. No date freezing, masking or pixel normalization was
  used. `logs/look-diff.log` preserves the complete raw pixel comparison.
- Stop 06 was also inspected side by side and is pixel-identical.

This is evidence of unchanged Phase 3 layout on the common-history stack,
not a claim that the raw captures are all pixel-identical. Independent
acceptance remains required.

## Actual three-frame 219px replay and late growth

`sequential` is a separate timing probe, not a modification of the LOOK
capture procedure. It routes only the unchanged frame-script requests of
T04 entries e723/e725/e727, holding their network continuations. It releases
the first two, waits for their **real** native 219px height reports to land,
then releases the third. No message/height is forged, no body/style is
rewritten, and no selection/cache property is assigned by the probe.

`browser/sequential.json` records both source identities' matching states:

| Native state | Boundary | Pending | Clip px | Boundary top relative to clip px | Live/clip bottom gap px |
|---|---|---|---|---|---|
| Three frame scripts still held | e723 | yes | 300 | 0.609375 | 0 |
| First two native 219px heights applied | e723 | yes | 738 | 0.609375 | 0 |
| Third native 219px height applied | e723 | no | 957 | 0.609375 | 0 |
| Historical version opened; actual late frame rendered | e723 | no | 1215 | 0.609375 | 0 |

The third report arrives about 41ms after the first pair on both sides.
Unlike the original base/current-clip mismatch, each fit exposes the whole
saved boundary. After finalization, the probe uses native clicks to open
the versions group and its historical panel, causing genuine later layout
growth. The selected chunk ID and first-usable-height cache remain unchanged
while the clip grows from 957 to 1215px. Both sides' full recorded T04
geometry matches at every corresponding stage; observer errors are empty.

### Preserved failed driver attempt

The first sequential command exited 1 with a 10s timeout **after** the
successful real 219/219/219 delivery and 957px finalization. The driver
opened the versions group but wrongly expected its historical panel's frame
to mount while that second disclosure was still closed. Historical entries
are intentionally not among the initially opened latest panels
(`stream_view.go:244-255`).

The exact failing script, raw states and full failure log are preserved in
`browser/sequential-attempt1/`. The sole correction adds a native click to
open that existing nested disclosure. Source, dates, heights, timeouts and
assertions are unchanged. The owner explicitly accepted that single input
correction. The second command passed on both exact identities; this does
not erase or relabel the failed attempt.

## New scenario (c) measurements and Nodes cost

Three alternating runs per side, Chromium 148.0.7778.96, common history,
shape 8,6,4,2,60; complete samples and output are in `browser/*-perf.json`
and `logs/perf.log`.

| Metric, median | 4x baseline -> candidate | 6x baseline -> candidate |
|---|---|---|
| ScriptDuration ms | 2438 -> 1275 (-47.7%, 8% spread) | 3414 -> 1841 (-46.1%, 9% spread) |
| TaskDuration ms | 5652 -> 3335 (-41.0%, 7% spread) | 7728 -> 4674 (-39.5%, 5% spread) |
| Long-task time ms | 3316 -> 1097 (-66.9%, 20% spread) | 4796 -> 1780 (-62.9%, 7% spread) |
| LayoutCount | 296 -> 297 | 286 -> 279 |
| LayoutDuration ms | 225 -> 206 | 299 -> 291 |
| Nodes | 46492 -> 63230 (+36.0%) | 46492 -> 63230 (+36.0%) |
| Frames / frames built | 15 / 20 -> 15 / 20 | 15 / 20 -> 15 / 20 |

The canonical incoming snapshot still retains additional DOM nodes. Its
**+36.0% Nodes** cost remains disclosed under Decision 12's CPU-over-memory
priority. The original **+35.5%/+36.8%** measurements belong only to the old
identities and remain historical evidence, not measurements of this stack.
The harness prints Phase 1's `keep rule: FAIL` for the Nodes increase; Phase
3 does not inherit that Phase 1-only keep rule. The CPU target improvement
is well outside its within-side spread. No new history or memory policy is
selected by this evidence.

## Limits and remaining gate

The original 219px LOOK failure stays failed, with its original timing
attribution inferred rather than retroactively proved. The new common-stack
comparison does not claim old-stack parity. These runs cover Chromium and
the original light-mode failing capture procedure, not other browsers,
Windows, or a new dark-mode capture campaign. No new source changes were
needed. Phase 3 completion/closure waits for the same independent reviewer
to accept this evidence against the existing Important finding.
