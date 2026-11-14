# Approved history-fit correction

## Authority
Owner641/642 approved inherited correction, relayed by MAIN and scoped by explore-1. Read `/Users/kang-sw/.cache/ws@kang-sw-devenv/proj/17da6bdc/shared/docs/handoff-ux.md:16-37` and `handoff-main.md:70` for exact context. Completed ticket `261113-feat-ws-whiteboard-topic-history-load-more` is historical reference only; do not edit completed phase text.

## Contract
Newest K panels plus intervening logs, trying K=3, then 2, then 1 until height is <=1000px; always one panel even if oversized. Boundary is immediately above oldest included panel; older logs belong to next chunk. Cache known heights and measure unknown height once. Already expanded chunks are never re-cut by late measurements or resize callbacks. Log-only stretches use 30 lines. Preserve off-screen folding, swap restoration, and existing interactions. Do not implement open history-unify643.

## Scope
Prefer agents-plugin-tool/internal/wswhiteboard/assets/stream.js and page.css, stream_script_test.go, and scripts/board-headless.mjs history scenarios. Source currently uses STEP=640 and cumulative snapping; verify actual source before editing. Read applicable AGENTS.md manuals before changes. Use simplest local implementation; if new public API/type/module or materially broader structure is needed, stop with evidence for ticket routing, not scope expansion.

## Verification and completion
Test 3/2/1 selection, oversized singleton, exact intervening/older log boundaries, log-only30, cached/late heights without re-cut, swap restoration, and off-screen collapse. Use isolated fixture servers/source builds if useful; never invoke dev launcher, installed launcher, shared daemon, snapshots, or production runtime. Run appropriate Go/Node/headless regressions. No network dependency installation. Commit one verified logical unit with AI Context and this plan on fix/history-fit-panels. No merge/rebase/pull/push/release or live config/runtime mutation. Report exact commit, tests, independent-review need and limitations using concise English VERIFIED/INFERRED bullets and file:line citations. If nested reviewer provenance fails, stop retrying and request a lead-rendered reviewer.
