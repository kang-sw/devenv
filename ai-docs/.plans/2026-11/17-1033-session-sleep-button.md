# Dashboard: Sessions panel "sleep" (moon) button that stops all idle sessions

## Context

Owner 2026-11-17 10:18, mockup approved (1284)=ok 10:32. Add a moon
button to the Sessions panel header, to the right of the chips and next to
refresh. It works as a power-save button: it puts every idle session into
the stopped state. A click first shows a confirmation that lists the target
sessions and explains that their processes end and they go to sleep. The
conversation is kept, and each session resumes on its next message or mail.

Approved mockup source (untracked in the main checkout; copy it into this
worktree at the same path and commit it):
`D:\Workspace\Repos\devenv\ai-docs\assets\mockups\261117\session-sleep\index.html`.
It has states `#a` (button), `#b` (confirm dialog), `#c` (after: toast and
rows in the stopped look) and `#d` (disabled when nothing is idle).
Reference PNGs are `state-a.png` to `state-d.png` beside it. Do not commit
the PNGs.

Facts, verified on develop (paths under `agents-plugin-tool/internal/`):

- Header:
  - `{{define "r-rail"}}` at `wsdashboard/assets/dashboard.tmpl:437`
    (`.rhd`, chips, `.rref#railref`);
  - CSS in `dashboard-rail.css:136-142`;
  - the refresh click handler at `dashboard-rail.js:590`.
- Icons:
  - sprite symbols, `{{icon "NAME"}}` emitting `<use href="#i-NAME">`
    (`wsdashboard/pages.go:392`);
  - the 16-grid stroke style is `.i`;
  - add `i-moon` to `wsdashboard/assets/sprite.svg`; the mockup's path is
    `M8 2a4 4 0 0 0 6 6 6 6 0 1 1-6-6z`.
- Per-session stop:
  - route `POST /agents/<id>/stop` (`wsagent/http.go:238`), behind the
    `leadControl("stop")` trusted gate (`http.go:314`);
  - it calls `Runner.Stop` (`wsagent/runner.go:956`), which runs
    `conv.stopProcess("")`;
  - the session is resumable on its next send.
  - Client side: `dashboard-leads.js:323` and `:378-381`, using `railPost`.
- Idle:
  - the runner's predicate is `idleNow()` (`wsagent/runner_events.go:605-631`);
  - it is false during a turn, a replay, background tasks, or an active child
    conversation.
- Stopped rows already render through `.rail .rs.stopped`.

## Outcome

1. A moon button `.rref.rsleep` appears in the Sessions header, matching
   mockup state a.
   - Tooltip: "유휴 세션 모두 절전".
   - When nothing is idle the button is disabled with the tooltip
     "유휴 세션 없음" (state d).
   - Strings go through i18n (ko and en tables), like the existing header
     strings.
2. A click fetches the idle list from the server and shows the confirmation
   dialog from state b: the title with count N, the explanation sentence,
   and the session list (harness icon, name, workspace · branch, last
   activity).
   - The note counts kept sessions (working or waiting for approval).
   - If sessions the dashboard cannot control (`can_control` false) exist,
     add "제어할 수 없는 세션 N개 제외".
   - Buttons "취소" (default focus) and "절전" (primary).
3. Confirming calls one new server route, for example
   `GET /agents/idle` for the list and `POST /agents/sleep-idle` to act; pick
   names that fit the existing routes.
   - The POST takes the ids the dialog listed. For each id, it re-checks
     `idleNow()` under the runner's lock and stops it exactly as
     `Runner.Stop` does.
   - Sessions that became busy are skipped and returned as skipped.
   - Same trusted gate as stop.
   - Scope: every session this daemon's runners can control, across all
     workspaces. The session the user is viewing is included when idle.
4. Afterwards a toast "세션 N개를 절전 상태로 전환했습니다" appears (state c),
   and the rows refresh to the existing stopped look.
   - When some were skipped, append "(N개는 작업을 시작해 건너뜀)".
   - The "실시간" chip is unchanged.
5. Compatibility: a new route on the daemon that also serves this page is
   T0. State that with source lines; an older daemon simply has no button.

## Constraints

- Read before editing `agents-plugin-tool/`:
  - `ai-docs/manuals/shipped-surface-boundary.md`;
  - `ai-docs/manuals/ws-mcp.md`, only if MCP surfaces are touched;
  - `ai-docs/manuals/release-compatibility.md`.
- Simplest complete change: reuse the existing stop path and dialog styles
  (the delete dialog look, `.pbtn`, `.wstoast`). No new config.
- Other workers are changing `wswhiteboard` (nudge timers, `wb:close`) and
  topic body CSS (content-visibility). Keep this change to the rail header,
  the dialog, and `wsagent` routes.
- NO local `go test`, Go test exes, ws/ws-mcp executables, or launcher on this
  PC (owner AV rule).
  - Do not start the ws daemon or Electron.
  - Go tests run on remote w5090 only:
    `D:/sys_lock.sh remote w5090 run <who> <min> --sync 'D:/Workspace/Repos/.ws-worktrees/devenv/flyaway-fifteen-judgingly' -- 'export PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && go test ./internal/wsagent ./internal/wsdashboard'`.
  - Accepted reds: only the two node-less `TestTopicStateBadge*` tests, if
    `wswhiteboard` is run.
- LOOK:
  - Render the real dashboard rail markup (a test fixture rendered on w5090
    is fine, as the previous worker did) in local headless Chrome.
  - Capture states a, b and d, and c if practical.
  - Compare side by side with the mockup PNGs and fix differences before
    committing.
- Temp files only under `D:/Temp/session-sleep-button/`. Keep the final PNGs
  for the lead; delete everything else.
- Commits carry `## AI Context` and the Co-Authored-By trailer naming your
  model. Do not push.

## Verification

- Go tests:
  - the idle list only returns idle, controllable sessions;
  - sleep-idle stops idle ones and skips one that became busy;
  - the trusted gate is enforced;
  - template renders the button.
- Run any JS tests that exist for the rail.
- LOOK screenshots against the mockup.
- Report:
  - commit(s);
  - files changed;
  - compatibility tier;
  - test results;
  - screenshot paths.
