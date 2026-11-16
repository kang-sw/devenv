---
title: "Fix stale dashboard usage windows and restore stacked indicator layout"
related:
  261116-bug-ws-exe-launch-elimination: "Item 4 Codex REST usage work in flight with Exec1 may overlap the usage data path"
  261114-feat-ws-dashboard-usage-footer-meter: "Original approved usage indicator design and reference"
  261113-feat-ws-dashboard-rail-usage-gutter: "Usage cache and acquisition contract"
---

# Fix stale dashboard usage windows and restore stacked indicator layout

## Background

Owner report, 2026-11-16 12:45 KST: after switching Claude from Pro to Max,
the dashboard still shows a stale 5-hour window; Max has no 5-hour window. The
usage indicator also renders the current and red/green offset percentages
side-by-side instead of vertically stacked. Screenshot (external, not copied):
`C:/Users/user/.cache/ws@kang-sw-devenv/dashboard/sessions/renewaldevoteetranspirefencing/inbox/attachments/33c583d5ce62f2d1-1.png`.

The original design is the A section of the
[approved usage-meter mockup](../assets/261114-feat-ws-dashboard-usage-footer-meter/mockup.html),
recorded in `261114-feat-ws-dashboard-usage-footer-meter` Decisions (732):
current percentage on top, smaller pace delta below. Preserve its stale and
weekly-only treatments. Coordinate with Exec1's item 4 Codex REST usage work.

## Decisions

- A window absent from the latest usage response, or invalidated by an account
  identity change, must disappear rather than retain its prior value. Do not
  treat an incomplete or failed refresh as a successful empty response.
- Compatibility tier **T0** for an internal fix without changing stored usage
  shape. If a stored shape change is necessary, reassess and record the tier
  with reader/writer source lines before implementing it.

## Phases

### Phase 1: Drop stale usage windows in the data path

Fix the usage cache/store to remove windows absent on successful refresh and
clear old-account windows when identity changes. Preserve valid current windows
and error handling. Add unit coverage for Pro-to-Max, absent windows, identity
change and failed refresh, including a subsequent refresh of the new account.

### Result - 2026-11-16

`pushClaude` now removes cached kinds absent from a newer nonempty, valid
Claude limits snapshot, including when the retained window's value is unchanged;
the removal is persisted and advances the entry timestamp so an older snapshot
cannot immediately restore it. Per-window stale/monotonic guards still apply.
Missing/empty or invalid limits are not treated as a successful empty refresh.
No account identity is tracked in this account-blind store; no credential or
identity tracking was added. Snapshots carry only their general `UpdatedAt`,
not an authoritative usage-response timestamp or completeness marker: later
session activity can refresh that timestamp, and independently stale sessions
may still repopulate old windows. Empty authoritative responses cannot be
identified separately from absent status data. These remain limitations, not
claims of full identity-change or empty-response handling.

Compatibility: **T0**, no stored shape change; existing `planUsageEntry` and
`planUsageWindow` JSON and `saveLocked`/`newPlanUsageStore` reader/writer are
unchanged (`agents-plugin-tool/internal/wsdashboard/plan_usage.go:21-65`).
The regression covers {5h, 7d} -> {7d}, persistence, older snapshot replay,
and empty/invalid status. Phase 2 UI is untouched.

Remote w5090 `go test ./internal/wsdashboard/... -run "TestPlanUsage"` passed
(job `6621982d7b5540bc849681d14f9abcd1`). The initial full-package run (job
`0f0d194b9e3d461f84ea2b944d88bf8e`) failed unrelated tracker/toast page asset
assertions in `TestPagesCarryTheTracker` and
`TestPagesCarryTheToastStackAndTitleHost`; no UI files were changed.

### Phase 2: Restore vertical indicator layout

Restore current percentage above its red/green offset in each window column;
retain weekly-only and stale styling. Verify with a side-by-side LOOK capture
against the approved mockup A at matching dimensions, including narrow layout.
This UI phase may be assigned to a Claude worker.
