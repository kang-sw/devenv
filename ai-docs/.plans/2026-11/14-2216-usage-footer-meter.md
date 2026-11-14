# Plan: dashboard usage footer meter, phase 1 (owner-approved mockup)

## Contract

The todo ticket `261114-feat-ws-dashboard-usage-footer-meter` (ai-docs/tickets/todo/) is the contract: its Decisions, the approved mockup HTML it preserves, and its compatibility tier (T0, display of the existing latest per-window values only). Owner answers recorded there (732 comment, 737-740) govern. Read it fully first.

Owner's own words for the meter (verbatim, Korean): "그냥 각 하네스 아이콘 옆에 N%(+G%) / M%(-M%) 이것만 있어도 되긴 하는데. % 표시는 작은 글씨 2행으로 그려서요." and "정확히는 2행에서 위 행이 현재 사용량, 아래 행이 상대적으로 더 작은 글씨로 비교 사용량으로 생각한 겁니다. 즉, 5시간 사용량이 있는 하네스는 두 열이 되고, 주간만 있으면 한 열이 되는 식."

## Scope

- Phase 1 only: the bottom-left footer meter. Phase 2 (history storage, trend chart, popover redesign: idea `261114-feat-ws-dashboard-usage-history-trend`) is OUT of scope.
- No new storage, no MCP/protocol/store-shape change. If phase 1 cannot be built from data the dashboard already receives, stop and report what is missing instead of adding a field.
- Dashboard assets under `agents-plugin-tool/internal/` plus their tests. Other sessions edit dashboard.js/css on other branches (TASK3 history redesign, 652 overview, 758 recap clamp); keep the diff surgical and contained (prefer the existing usage asset file over dashboard.js where it lives).
- If the ticket has an open decision or the work needs new structure beyond the ticket, stop and report.

## Verification

Machine-wide heavy-job throttle: do NOT run builds, `go test`, Node tests, or browsers in this turn. Static edit + commits only (tests updated/added statically). Report the exact focused test commands and the browser check (1280 and 360 widths, compare against the mockup PNGs) for the lead to schedule.

## Return

Commits on `impl/develop/usage-footer-meter` with this plan. Report files changed, how each harness renders (one vs two columns), the deferred test/browser commands, and any ticket deviations.
