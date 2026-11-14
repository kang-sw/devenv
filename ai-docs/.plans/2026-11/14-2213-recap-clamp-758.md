# Plan: whiteboard recap clamp + fixed-height topic sticky-header recap (758)

## Owner request (verbatim, Korean)

"그리고 이거 카드 sticky header의 recap 사이즈 가변이 지금 이 flickering 원인같은데, recap 자체를 최대 4행 정도로 제한하고 토픽 카드 헤더 recap은 그냥 고정 사이즈로 바꿔주세요. 사이즈 가변도 좀 산만하고, 4줄이 넘어갈 정도면 애초에 recap도 아니고요"

Announced to the owner as board item (758): display-only (T0).

## Outcome

1. Every rendered recap (whiteboard panel/topic recap text wherever the dashboard shows it) is clamped to at most 4 lines with an ellipsis; the full text stays reachable (hover title or equivalent existing affordance).
2. The topic card's sticky header recap slot has a FIXED height (the 4-line height) regardless of recap content length, so the sticky header height never changes when the recap changes.

## Boundaries

- Display only: no change to whiteboard command parsing, `recap` validation, stored board shape, MCP tools, or protocol. Compatibility tier T0 (dashboard asset content).
- Files: dashboard assets under `agents-plugin-tool/internal/` (whiteboard/topic card CSS and the JS that renders the sticky header recap) and their existing tests. Locate exact files yourself; keep the change surgical.
- Do not touch the folded-panel/reopen behavior (owned by another in-flight ticket, TASK3) or overview.js clearance (owned by the 652 branch). If your change must touch those, stop and report the overlap instead.
- Other sessions edit nearby dashboard files on other branches (TASK3 history redesign, 652 overview clearance). Keep the diff minimal to ease merging.

## Verification

- Static: update/add unit tests asserting the clamp/fixed-height rules where the existing test style allows.
- Machine-load rule: builds, `go test`, Node tests, and browser runs are throttled machine-wide (one heavy job at a time, granted by the lead). Do NOT run any of them in this turn. Finish the static edit + commit, then report the exact focused test command(s) and the browser check you would run; the lead will tell you when you may run them.

## Return

Commit on branch `impl/develop/recap-clamp-758` (this plan committed with it). Report: files changed, the clamp/fixed-height values, the focused test commands to run later, and the proposed browser check.
