# Delegate plan: agents pane rows show cumulative running time; "ago" moves to the meta line

## Outcome

Owner request (dashboard, 2026-11-13, whiteboard topic `agent-time`). In the
session page's agents pane, each agent row (Claude subagent, hosted Pi child
`pi:<id>`):

1. The top-line right time (`dashboard.tmpl` `{{define "agent"}}` ~143, the
   final conditional inside `.am`) shows the agent's **cumulative running
   time**: the sum of the time it was actually working, excluding idle gaps
   between turns. Running rows tick live; finished rows show the static total.
   Today a running row shows wall time since first seen (`dur .StartedAt`,
   includes idle gaps between Pi turns) and a finished row shows `ago .EndedAt`.
2. The "ago" (`{{template "ago" .EndedAt}}`, e.g. "4m 28s 전") moves to the
   second/meta line (`{{define "astat"}}` ~150), right after the `t/s` figure
   (after `.atps`; for finished rows `Rate()` is 0 so it follows the context
   tokens). Running rows show no ago.

## Evidence (investigation, paths under agents-plugin-tool/internal)

- Row template: `wsdashboard/assets/dashboard.tmpl:143` (agent), `:150`
  (astat), `:114-116` (`dur` since-ticker, `ago`, static `span`/`spanText`).
  Client ticker: `assets/dashboard.js` ~40-95 `fmtTime` / `tickTimes`
  (1 s, `time[data-ms][data-f=since|ago|rel]`). astat on a second line only via
  container query (`dashboard.css` ~216-234; dot separators `span + span::before`).
- View: `wsdashboard/pageview.go:179` AgentNode, `:211` IsRunning, `:228` Rate,
  `:236` HasStats (must admit an ago-only finished row).
- State: `wslive/state.go` agent creation ~539-543 (startedAt on first sight),
  applyAgent ~587-596 (running / done / failed transitions, endedAt),
  setTelemetry ~627, SeedFinished ~900-928, Snapshot ~971-985;
  `wslive/snapshot.go:298-320` Agent (StartedAt/EndedAt only). No accumulator
  exists anywhere.
- Claude subagents: `wsagent/claude.go:838` running at task start, `:867` end.
  Single run in practice.
- Pi children: `wsagent/spawn_live.go:11` SpawnAgentEvent maps
  `Status==running || Held>0` to running; no timing fields. Each pi.send starts
  a new turn; `Record.TurnStartedAt` (`wsagent/record.go:127`, set
  `runner.go:1807`) holds only the current turn's start; `settleTurn` is the
  turn end. The record is persisted.

## Settled decisions (lead)

1. **Pi children: durable accumulator on the record.** Add an optional
   `ActiveMs` (accumulated closed-turn ms) to the persisted wsagent record /
   State, adding `now - TurnStartedAt` at turn settle (every outcome). Forward
   to the row via SpawnAgentEvent as two new optional event fields: the
   accumulated ms and, while a turn is actually running, that turn's start
   (so held-but-idle time is not counted). Survives daemon restarts.
2. **Claude subagents: wslive accumulator on status transitions.** In
   applyAgent, a transition into running starts a segment; running -> done /
   failed adds the segment to the agent's accumulated ms. Carry it through
   SeedFinished and Snapshot. For a Pi row, the forwarded fields from
   decision 1 take precedence over the transition accumulator.
3. **Template**: one cumulative-time element at the old spot. Running: a live
   `since` ticker whose base is shifted (`data-ms = now - accumulated -
   (now - currentTurnStart)`), so the existing JS ticker counts it with no JS
   change; the view computes the base. Finished: the static `span` template
   with the total. Move `ago .EndedAt` into astat after `.atps`, and make
   HasStats true for a finished row with EndedAt. Keep the dot-separator
   styling consistent.
4. New fields are optional and omitted when zero: **T0** (additive). Record
   the tier in `## AI Context`. If the persisted record has a
   `schema_version` or strict decode, stop and report rather than bump it.
5. Out of scope: lead row timing, tool rows (`runrow`), any other pane.

## Verification

- Go tests: wslive accumulation across running -> done -> running -> done
  (sum, not wall time) and through SeedFinished; Pi: two turns separated by an
  idle gap produce ActiveMs equal to the two turn durations (model on
  `wsagent/hosted_usage_test.go` Pi row tests); held-but-idle time not
  counted; pageview render: running row has a `data-f="since"` element with
  the shifted base, finished row has the static total and the ago inside
  astat after the t/s / context span. Update `pageview_test.go` ~1209-1238
  astat assertions and `pages_test.go` ~691 if affected.
- `go test ./internal/wslive/ ./internal/wsagent/ ./internal/wsdashboard/` on
  remote w5090:
  `bash D:/sys_lock.sh remote w5090 run <who> <min> --sync 'D:/Workspace/Repos/.ws-worktrees/devenv/guiding-dragonish-consumer' -- 'export PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && go test ./internal/wslive/ ./internal/wsagent/ ./internal/wsdashboard/'`
  (Node-dependent tests locally under `bash D:/sys_lock.sh run <who> <min> -- <cmd>`).
  One passing full check closes the work; a pre-existing failure proven at
  base is excluded and recorded as one Forward.
- LOOK: one screenshot of a rendered agents pane fixture (headless Chrome,
  fake data) with a running Pi row and a finished Claude row, under
  `D:/Temp/agent-active-time/`. One retry on an environmental failure.

## Return boundary

Commit on `impl/develop/agent-active-time` in this worktree (this plan rides
the commit). Do not merge. Do not restart or replace the live dashboard
daemon. Another worker (`impl/develop/lunar-coil-phony`, rail usage gutter)
edits dashboard assets in parallel: keep shared-file diffs minimal. Stop and
report if the change needs a T1/T2 change (store schema bump, protocol,
tracker event type semantics) or edits outside wslive / wsagent / wsdashboard
(and their tests).

Machine rules: temp files only under `D:/Temp/<purpose>/`; never create
folders in C:/ or D:/ roots; stop if D: free < 20 GB; never print credential
tokens.
