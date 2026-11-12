# Plan: hosted Pi subagents appear in the dashboard agents panel

Ticket: `261112-bug-ws-hosted-pi-subagents-missing-from-dashboard` (idea/;
read it first: `## Diagnosis` has the code pointers, `## Decisions` the
settled contract).

## Outcome

In a dashboard-hosted Pi lead (daemon-owned `pi --mode rpc` under a holder),
native Pi children spawned with `ws-agent-spawn` show in the dashboard agents
panel with the same rows a CLI Pi session's feed produces: id, parent, label,
reply id / session key, status (running/done/failed), model, effort, context
tokens, output rate, batches, and permission state.

1. Pi extension (`agents-plugin-pi/src`): in hosted mode, a child-only
   channel replaces the suppressed feed for children. It reuses
   `dashboardAgentEntries` and the feed's change detection, 100 ms throttle,
   and 1 s poll while children run (extract shared code from
   `dashboard-feed.ts` rather than duplicating it), and emits
   `ctx.ui.setWidget("ws-dashboard-agents", [<JSON event string>, ...])`
   carrying only child `agent` and `permission` events, including a done for
   a child that left the registry while running. While any child exists, it
   resends the full child set without batches every 30 s. It never sends
   reset, turn, or lead events. Wire `agentDashboardRefreshRef` to it near
   `index.ts:935`. CLI (non-hosted) behaviour is unchanged.
2. Daemon (`agents-plugin-tool/internal/wsagent/pi.go`): the Pi mapper
   decodes `extension_ui_request` `setWidget` with that exact key into
   `wslive` agent/permission events, accepts only those with a non-empty
   agent id, and ignores other keys, other kinds, and malformed entries.
   Reuse the event shape the hosted Claude path already feeds the runner
   (`claude.go` `task_started` mapping) so existing projection, `endAgents`
   on process exit, and replay handle the rows.

## Settled constraints

- Rejected: deriving rows from spawn results / settled pushes in the RPC
  stream; routing child events through `dashboard.intake`.
- Compatibility tier T0: the reserved widget key is the only new contract.
  No MCP tool/argument, store shape, tracker event type, holder protocol, or
  snapshot shape change. Stop and report if one is needed.
- If the wslive event a batch update needs does not exist for agent rows, or
  `ctx.ui` captured at `session_start` is unusable after `/reload` in RPC
  mode, handle it within these files or stop and report.
- Manuals: `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/release-compatibility.md`.
- Heavy `go build`/`go test`/`go vet` only through
  `~/.local/bin/ws-rgo -C agents-plugin-tool -m <min> -- <go args>`. TS tests
  in `agents-plugin-pi` may run locally with its own test command.

## Verification

- Go (`pi_test.go`): a widget line maps to agent events (status, model,
  label, parent); events without an agent id, other kinds, other widget keys,
  and malformed JSON are ignored; a permission event sets and clears; Pi rows
  end on process exit like `TestLiveWriterEndAgents`; a runner-level test in
  the pattern of the hosted Claude subagent test.
- TS: the channel with a fake `ui.setWidget` (change-only emission,
  gone-while-running becomes done, throttle, periodic full resend without
  batches, nothing lead-level); `hosted-session.integration.test.ts` still
  sees zero intakes and now the reserved widget lines; `dashboard-feed`
  tests unchanged.
- `go test ./internal/wsagent/...` and `go vet` through ws-rgo; the Pi
  package's test suite; `git diff` reviewed against this plan before commit
  on `impl/develop/pi-hosted-child-rows`, with `## Ticket Updates` naming the
  ticket.
