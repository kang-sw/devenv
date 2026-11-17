# Whiteboard: stop repeating lead nudges until the board changes

## Context

Owner 2026-11-17 09:48, (1276)=a. The daemon's timed whiteboard lead nudges
(the hourly "Whiteboard lead nudge (hourly)" open-topic list, sent while any
implementing or waiting topic is open, plus ETA-overdue and stale-topic
nudges) kept arriving all night while nothing was being worked: the same list
repeated every hour with no board change. The owner chose the automatic
behaviour: after a nudge is sent, do not send the same kind again until the
board is updated.

The behaviour is described for leads in the whiteboard playbook section
"Timed mailbox nudges" (hourly repeat while an open implementing/waiting
topic exists; ETA overdue with a 30-minute debounce per topic; deadlines and
receipts survive restarts).

## Outcome

1. Hourly open-topic list: sent at most once per board revision. After it is
   sent, no further hourly list goes out until the board has a new
   lead/user-originated change (any successful `whiteboard.update` that
   changes the board, or a user answer arriving from the dashboard); then the
   normal hourly cadence resumes.
2. ETA-overdue nudge: a given overdue topic is reported once; it is repeated
   only after the board changes (e.g. the lead extends the ETA, logs, or
   changes state) and the topic is still overdue. A newly overdue topic still
   triggers a nudge.
3. Stale-topic nudges follow the same rule (no repeat without a board change).
4. Waiting -> ready "unblocked" nudges are unchanged (already deduped).
5. Server-generated events (system log entries, reconciliation, timer
   receipts) do not count as a board change, otherwise the nudge would
   unmute itself.
6. Receipts survive daemon restarts as today; an older daemon reading the
   store simply keeps its old repeat behaviour (state the compatibility tier
   with source evidence; expected T0).

## Constraints

- Read `ai-docs/manuals/shipped-surface-boundary.md` and
  `ai-docs/manuals/release-compatibility.md` before editing
  `agents-plugin-tool/`.
- Do NOT edit `agents-plugin/rsrc/whiteboard/whiteboard.md` (owner rule).
  If the "Timed mailbox nudges" text there becomes inaccurate, report the
  exact sentences that drift; the lead handles it.
- NO local `go test`, Go test exes, ws/ws-mcp executables, or launcher on this
  PC (owner AV rule). Go tests only on remote w5090:
  `D:/sys_lock.sh remote w5090 run <who> <min> --sync 'D:/Workspace/Repos/.ws-worktrees/devenv/porthole-diabetic-banjo' -- 'export PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && go test ./internal/wswhiteboard ./internal/mcp'`
  (add the package that owns the nudge timer if different). Accepted reds:
  only TestTopicStateBadgeResearchingMetadata and
  TestTopicStateBadgeClockAndReconciliation (need Node).
- Simplest complete change: one "last notified board revision" (or
  equivalent) per nudge kind in the existing receipt record; no new config.
- Temp only under `D:/Temp/board-nudge-quiet/`, deleted after.
- Commits carry no Co-Authored-By trailer. Do not push develop or main.

## Verification

- Go tests on w5090 for: hourly list sent once then suppressed with no board
  change; resumes after a board update; overdue topic not repeated without a
  change, repeated after a change while still overdue; newly overdue topic
  still nudges; server/system events do not unmute; restart keeps
  suppression.
- R1 review; fixes-only R2 for substantive findings.
- Report: files changed, compatibility tier with source lines, test results,
  and any playbook text drift.
