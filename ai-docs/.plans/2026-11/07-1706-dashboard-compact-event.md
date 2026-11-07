# Plan: dashboard - show context compaction in the conversation; refresh status after it

## Outcome

The user (2026-11-07, whiteboard): the dashboard session page gives no sign
that the lead is compacting ("context compacted" in the conversation would
help), and after a compaction the status band's context figure stayed at the
pre-compaction 75% until much later. Confirmed on the board:

- (100) a: both start and end are shown.
  - When a main-conversation compaction starts, the conversation shows a
    system row "컨텍스트 압축 중…" with an in-progress indicator; when it
    lands, the same row becomes "컨텍스트 압축됨 (자동)" / "(수동)" by trigger;
    a failure or veto shows "컨텍스트 압축 실패". The summary text is NOT
    shown.
  - The agents tree's lead row also shows the compacting state while it runs.
  - Both adapters (Claude mod and Pi) feed a new `compact` live event. This
    is the third adapter exception to the epic's cross-child decision
    (`261107-epic-ws-dashboard-messenger-round2`, Cross-Child Decisions).
- (101) ok: status refresh after compaction.
  - Today claude-mod pushes status figures only on `session.measure`
    (register.ts ~1344-1365: main-thread turn end, rate-limit moves). Nothing
    runs right after a compaction, so the page keeps the old context %.
  - When a compaction lands, push the status figures once more, with a fresh
    usage read. Pi does the same on `session_compact`.

## Evidence (lead-verified)

- claude-mod `register.ts` ~1501: an `on('session.compact', ...)` wrapping
  hook. Before `await next(e)` is the start and after it is the landing (a
  `SessionCompactResult`). Today only the compaction and captions features use
  it. A subagent's or fork's compaction (`e.agentId` set) is ignored, as the
  compaction feature does.
- claude-mod `dashboard.ts`: `DashboardEvent` kinds (~line 439), the feed
  buffer, `settleBefore`, and the `feedStatus()` path in register.ts.
- Pi: `agents-plugin-pi/src/dashboard-feed.ts` (the feed);
  `session_before_compact`, `session_compact`, and `session_compact_failed`
  events are used in `goal-loop.ts`, `bridge.ts`, and
  `display-summary-session.ts`.
- Go: `agents-plugin-tool/internal/wslive/state.go` and `snapshot.go` (live
  event kinds and state), plus the `internal/wsdashboard` conversation and
  agents-tree rendering and page script.

## Settled constraints

- The compaction row lives in the live conversation stream and stays after
  the turn. It must survive the page's incremental conversation updates and
  reloads, so it lives in server state like other conversation entries. A
  compaction start with no landing (a crash or host exit) must not spin
  forever: a session reset or the next turn start settles it.
- The event goes through the existing intake path; keep the wire small:
  `{kind: "compact", phase: "start"|"end"|"failed", trigger?: "auto"|"manual", at}`.
  The exact field names are your call if the existing event style differs.
- Observers never change the compaction; a feed failure is logged and
  swallowed, like the other feed hand-offs.
- Update the docs that describe the feed's event list: the
  `agents-plugin/hooks/hooks.json` description (mirror it in
  `agents-plugin-wsflow` per `ai-docs/manuals/wsflow-mirroring.md`), the
  `register.ts` header, the Pi feed doc comments, and `ai-docs/manuals/ws-mcp.md`
  where it lists live event kinds.
- Shipped-surface boundary applies (`ai-docs/manuals/shipped-surface-boundary.md`).
  Page strings are human-facing and may be Korean as shown above; match how
  other page strings are localized if there is a convention.

## Boundary

Commit on branch `delegate/dashboard-compact-event` only; do not merge or
push. Return to the lead if a host lacks the start signal, or if this needs a
protocol decision beyond the above.

## Verification

- agents-plugin-tool: `go build ./... && go vet ./internal/... ./cmd/...`;
  `go test -count=1 ./internal/wslive/ ./internal/wsdashboard/ ./internal/mcp/`.
- claude-mod: `claude plugin test` in agents-plugin and agents-plugin-wsflow,
  with engine tests for start/end/failed events, subagent compaction ignored,
  and the status push after landing.
- Pi: the dashboard-feed tests. The full Pi suite has 34 known
  base failures; compare against base.
- Headless Chromium (cached Playwright under
  ~/.npm/_npx/*/node_modules/playwright): drive the live state with start then
  end events. Check the row changes from in-progress to done, the lead row
  indicator, light and dark themes, and that it survives a reload.
- Report what was not checked.
