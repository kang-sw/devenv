# Tracker repairs from the 00fd2e5d9..62304312b review sweep (I4, I5, I8)

## Goal

Repair three Important findings of the review sweep (findings file:
/home/swkang/.cache/ws@kang-sw-devenv/proj/dac18b1d/review-paths/769fe701-01-findings.md;
partition reports beside it, `d169a2be-0{1..6}-sweep-*.md`). The user
approved fixing them locally.

- I4 `agents-plugin-tool/internal/wstracker/check.go:928-946`,
  `state.go:1893-1915` (commits 745939377, 6915ebd0f): `item.approve` hands
  over an assignment that is already done or dropped, because pending
  hand-over/assignment requests never lapse.
- I5 `agents-plugin-tool/internal/wstracker/merge.go:426-450`: a merging
  body edit cannot remove an attachment (the merge unions attachments over
  the surviving writes, so a removal is lost). Related idea ticket:
  `261111-bug-ws-tracker-body-merge-minor-gaps` (read it; close or narrow it
  only if this fix fully covers one of its items, and say so in the commit).
- I8 test gaps: `agents-plugin-tool/internal/wsdashboard/assets/tracker.js`
  ~823-860 `moveWhy`/`moveRoots` (90cd1c3ee) and ~800-813
  `canOpen`/`canUnopen`/`reqTargets`/`canAnswer` (86c2daeda) copy store rules
  with no tests.

## Decisions

- I4: refuse `item.approve` (and any other path that applies a pending
  request) when the target assignment/item is done or dropped, with a clear
  refusal message; mirror the check in the client rule copy (`engine.js` or
  wherever the tracker client duplicates check.go) and add a golden/conformance
  case where the store's rule cases live. Rejected: lapsing pending requests
  on wrap-up, since that adds a new state transition (an event-semantics
  change) where a refusal needs none.
- I5: a merging body edit that removes an attachment keeps the removal:
  merge attachments three-way against the common base (removed by one side
  and untouched by the other = removed), not as a union of the surviving
  writes. Keep every other merge behaviour.
- I8: pin `moveWhy`/`moveRoots` and `canOpen`/`canUnopen`/`reqTargets`/
  `canAnswer` in the existing node-driven `tracker_quickadd_test.go` harness,
  with cases derived from the Go rules in check.go they copy (one allowed and
  one refused case per rule at least). Tests only; change tracker.js only if
  a test exposes a real divergence from check.go (then fix tracker.js to match
  and say so).
- Compatibility: no store shape or event type change (T0). If you find either
  is needed, stop and report instead.

## Constraints

- Read `ai-docs/manuals/release-compatibility.md` and
  `ai-docs/manuals/shipped-surface-boundary.md` before editing (AGENTS.md
  Implementation Conventions rows for `agents-plugin-tool/`).
- A user policy change to tracker assignment completion rules is being
  discussed separately; do not anticipate it. Implement only the above.
- Heavy Go test runs only through
  `~/.local/bin/ws-rgo -C agents-plugin-tool -m 10 -- test -count=1 ./internal/wstracker/... ./internal/wstrackerhost/... ./internal/wsdashboard/... ./internal/mcp/...`;
  never run go build/test/vet locally. `internal/mcp`
  `TestDashboardReadPageServesTheBoard` already fails on develop; ignore it.
- One commit per finding on branch `delegate/tracker-sweep-fixes` in this
  worktree; do not merge or push.
