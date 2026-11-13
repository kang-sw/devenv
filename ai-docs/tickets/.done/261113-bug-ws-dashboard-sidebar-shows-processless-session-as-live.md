---
title: "Dashboard sidebar shows a processless hosted session as live while its popover says no process runs"
related:
  - 261113-bug-ws-daemon-replace-stops-hosted-sessions
  - 261113-bug-ws-launcher-dev-rebuild-replaces-daemon-mid-work
completed: 2026-11-13
---

# Dashboard sidebar shows a processless hosted session as live

## Observed (2026-11-13)

- Right after the dev daemon moved from 0.49.1-dev to 0.50.0-dev (pid 4832),
  several dashboard-hosted sessions looked alive in the left sidebar.
- Their settings popover showed Stop disabled with "No process runs. The next
  message or mail starts one." (ko: "실행 중인 프로세스가 없습니다...").
- The backend knows there is no process; the sidebar does not say so.

## Evidence

- Popover: `agents-plugin-tool/internal/wsdashboard/assets/dashboard.js:6463-6464`
  disables Stop and picks i18n key `js.lead.stopped` when `!st.process`
  (`wsi18n/tables/ko.json:1566`). `process` is `c.proc != nil`, the runner's
  in-memory state (`wsagent/runner.go:1495`, field at `runner.go:1010`).
- Sidebar: rows come from `railRow` (`wsdashboard/pageview_rail.go:485`).
  `Live` is `Registration.Live(now)` (`registry.go:195-204`): not ended and
  heartbeat within 5 minutes. It never reads `process`/`HostPID`.
- The runner keeps heartbeating a processless hosted row
  (`wsagent/record.go:471`, `runner.go:1557`), so `Live` stays true.
- The only asleep mark is `RailRow.Stopped()` (`pageview_rail.go:523-525`):
  true only for exit reason `stop` or `killed`. It drives the moon icon
  (`dashboard.tmpl` `rrow`, `{{if .Stopped}}<span class="zz">`).
- Live state (read-only, 10:12Z): hosted rows `impulsiveobtrusive...` and
  `levitateonboard...` have `host_stopped: true`, `host_exit_reason:
  "child_exited"`, heartbeat 10:10:09Z, no `ended_at`. Row `antiquitytrade...`
  has the same shape with reason `stop` (this one gets the moon).

## Likely cause

- The sidebar derives liveness from the registration heartbeat, not process
  presence, and treats only user-initiated stops as asleep.
- A daemon swap kills holders/children with another reason (`child_exited`),
  so `Stopped()` is false: the row stays "live" with no moon, while the popover
  reads the real `process` flag.
- Not a stale-status or restore bug: `host_stopped` is correctly recorded.

## Owner direction

A session with no running process, where Stop is disabled, should display as
asleep/dormant in the sidebar.

Fix shape: backend/template, not JS-only. Widen `Stopped()` to any hosted row
with `HostStopped` set (keep the reason line for non-stop reasons), or add a
processless flag to `RailRow`. The rail is server-rendered Go template output,
so no dashboard.js state is involved.

## Related

- `261113-bug-ws-daemon-replace-stops-hosted-sessions` (masked exit reason:
  why the swap leaves `child_exited` rather than `stop`).
- `261113-bug-ws-launcher-dev-rebuild-replaces-daemon-mid-work` (the trigger).


## Resolution (2026-11-13)

Fixed by a97c95953 (merge 05ddba10d): RailRow.Stopped() is now true for any live hosted row with a host exit, whatever the reason; the old stop/killed check is UserStopped(). The moon icon was removed (owner: gray text suffices) and the harness badge of stopped rows is dimmed. child_exited rows keep their reason line; only user-stopped rows collapse to one line. The four-state lifecycle ticket may later replace Stopped/UserStopped.
