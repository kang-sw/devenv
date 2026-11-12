# 0.49 sweep repairs, batch C: tracker handoff accept after close, label toggle race

## Outcome

Fix the batch C findings of the 0.49 pre-ship review sweep (range
62304312b..32b702129). The findings file is
`/home/swkang/.cache/ws@kang-sw-devenv/proj/dac18b1d/review-paths/05aaa35c-01-findings.md`;
read its batch C section first.

1. **C1:** a pending handoff never lapses when its sender closes the work, so
   the receiver's `item.accept` hands over done (or dropped) work. Repro on
   the reference engine: `lena.pc item.handoff {item: maya.pc#1, to: omar}`,
   `lena.pc item.close`, `omar.pc item.accept`: all apply, omar is credited
   with done work. Fix it the same way I4 fixed `item.approve`
   (commits b280fd63b / a355f5998, plan
   `ai-docs/.plans/2026-11/12-1346-tracker-sweep-fixes.md`, `takeWrapped`):
   refuse `item.accept` at check time when the handoff's `handoffOf` record
   is live and wrapped (done/dropped), and fold-flag the concurrent case.
   Places: `agents-plugin-tool/internal/wstracker/check.go` ~1062-1072
   (TypeItemAccept), `state.go` ~1405 (applyAccept), the mirror in
   `ai-docs/ref/tracker-mockup/engine.js`, and the dashboard accept button
   (`agents-plugin-tool/internal/wsdashboard/assets/tracker.js`, hidden or
   disabled with the refusal reason, consistent with how approve is gated).
   Golden files: regenerate through the existing gen-golden harness if the
   change affects them. Tests in Go and the engine/tracker harness.
2. **C2:** `tracker.js` ~4214 / ~4906 (`m-label` -> `labelToggled(it.labels, ...)`):
   two quick label toggles race; the second write builds from the stale
   `it.labels` and silently undoes the first. Compute each toggle from a
   pending local set (or queue the writes) so quick toggles accumulate; add a
   node case in the tracker test harness.

## Boundaries

- Read before editing: `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/release-compatibility.md`. State the tier of the C1
  change in the commit's AI Context (a new check-time refusal plus a fold
  flag; compare the I4 commit, which recorded T0).
- No refactors beyond each item. Never run go build/test/vet locally.

## Verification

- `~/.local/bin/ws-rgo -C agents-plugin-tool -m 15 -- test ./internal/wstracker/... ./internal/wsdashboard/ -count=1`
  from the worktree root: green.
- The engine's own test runner (see `ai-docs/ref/tracker-mockup/`) if it has one.

## Return

Commit(s) on `impl/develop/sweep049-tracker` in
`/home/swkang/.ws-worktrees/devenv/lapped-override-viewing`, this plan file in
the first commit. Do not merge or push. Report commits, what changed, and
test output.
