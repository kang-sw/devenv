# Plan: lead holders keep a larger ring, and a wrapped rejoin holds input

Ticket: `261112-bug-ws-hosted-holder-ring-wrap-loses-turn-state` (idea/; read
it first: background with code pointers, settled decisions, constraints).

## Outcome

1. A hosted lead's holder runs with a larger ring than terminal holders: a
   per-launch ring size on the launch call (a `RingBytes` field), passed to
   `holder.py` as the existing `--ring-bytes`. Lead launches use a constant in
   the 32-64 MiB range; terminal holders keep the manager default.
2. A rejoin whose replay window lost records handles it:
   - `holderChild.event` handles the `gap` event: it drops `partial`.
   - When the holder's status reports `FirstSeq > HolderSeq+1`, or a gap
     arrives during the replay, and the replayed window shows no turn boundary,
     the conversation treats the turn as running and submits nothing (held
     messages and mail stay held) until the harness's next idle signal (Claude
     `session_state_changed idle` or `result`; Pi `agent_end`). The loss is
     logged once.

## Settled constraints

- Rejected: raising `Manager.RingBytes` (grows every terminal ring), and
  trusting the replayed window.
- Code: `internal/wsholder` (launch spec / `manager.launch` / `--ring-bytes`),
  `internal/wsagent` (`holderChild`, the lead launch call, the rejoin and
  replay path in `runner.go`), plus tests. `holder.py` already has the flag;
  change it only if a test proves it must.
- Compatibility tier T0: internal Go API only; no record, registry, snapshot,
  holder protocol, or MCP shape change. Stop and report if one is needed.
- Heavy `go build`/`go test`/`go vet` only through
  `~/.local/bin/ws-rgo -C agents-plugin-tool -m <min> -- <go args>`.

## Verification

- Tests: lead launch passes the larger ring size and a terminal launch does
  not; a `gap` drops a partial line; a wrapped replay with no turn boundary
  holds a queued message until the idle signal, then delivers it once; an
  unwrapped replay behaves as before.
- `go test ./internal/wsagent/... ./internal/wsholder/...` and `go vet` through
  ws-rgo; `git diff` reviewed against this plan before commit on
  `impl/develop/holder-ring-wrap`, with `## Ticket Updates` naming the ticket.
