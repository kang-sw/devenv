# Dashboard session header: icon-only subagent badge; slash-command completion in hosted sessions

Owner feedback 2026-11-13. Two independent fixes, one commit each.

## 1. Subagent badge shows the icon and count only

Screenshot `D:/Temp/session-header/subagent-badge.png`: the header badge
reads "[icon] 1 서브에이전트" beside a clock badge "2". Drop the word; keep
icon + count, like the neighbouring clock badge. Keep the word as the
tooltip / accessible label (existing i18n string), so nothing is lost for
screen readers. Assets only (`dashboard.js` ~2018-2030 `data-sub`, template
and CSS as needed). T0.

## 2. Slash-command autocomplete does not work in hosted sessions

Owner: in a dashboard-hosted session the composer's slash-command
autocomplete does not work at all (maybe only right after the session is
created, maybe always; unknown for Claude-mod sessions).

- First establish how the composer gets its slash-command list (endpoint,
  the source of the list per harness: hosted Claude, hosted Pi, ordinary
  Claude mod session, ordinary Pi), and why it is empty or not triggered for
  a hosted session. Live data may be read (not written) under
  `C:\Users\user\.cache\ws@kang-sw-devenv\dashboard\sessions\<replyid>\`;
  hosted Claude row `shrankhumpbackuncannyelaborate`, hosted Pi row
  `refutablevarnishmultitudesnowsuit`. You may GET the live daemon at
  `http://127.0.0.1:29417` read-only (no POSTs, no session actions).
- Fix the cause so a hosted session offers the harness's slash commands, also
  right after creation (if the list arrives late, the composer picks it up
  when it arrives, without a page reload). State the root cause in the
  commit's `## AI Context`. If the list source does not exist for hosted
  sessions at all and adding it needs a protocol/store change, stop after the
  diagnosis and report instead of building it.
- Note in the report whether ordinary Claude-mod sessions are affected.

## Out of scope

- Restarting the daemon or any session (the lead does the live check).

## Verification

- `go test ./internal/wsdashboard/... ./internal/wsi18n/... -count=1` (plus any
  package the fix touches) on the remote w5090 (clone note
  `remote-build-w5090`).
- A test covering the hosted completion source; state what was not checked
  live.
