# Plan: hosted context meter shows the compaction window and refreshes per model call

Ticket: `261112-bug-ws-hosted-context-window-ignores-compaction-window`
(idea/; read it first: background, decisions, constraints).

## Outcome

On a daemon-hosted Claude lead session page:

1. The context meter's maximum is the effective compaction window the harness
   compacts against (the figure the terminal mod shows as `rawMaxTokens`), not
   the model's `modelUsage.contextWindow`. With
   `CLAUDE_CODE_AUTO_COMPACT_WINDOW=300000` it reads 300k, not 1M.
2. The token count refreshes at every lead assistant message carrying `usage`
   (each model call), not only when the turn settles.

## Settled constraints

- Code: `agents-plugin-tool/internal/wsagent/claude.go` (`claudeMapper`:
  `assistant()` ~1019, `status()` ~1084, `result` handling ~846-860), plus the
  runner/control-request plumbing it needs (`runner.go`, wherever interrupt and
  other control requests are sent and their responses read).
- Refresh: emit `m.status()` after a lead (non-subagent, non-alert) assistant
  message that set `m.context`. Keep subagent and alert messages out.
- Window source: the stream-json `get_context_usage` control request
  (the 2.1.322 binary has it; it also has a message
  "get_context_usage is not supported in this context (onGetContextUsage
  callback...)", so verify it answers in `claude -p --input-format stream-json
  --output-format stream-json` mode). Use its `rawMaxTokens` (or the field the
  terminal mod reads; see `agents-plugin/hooks/claude-mod/dashboard.ts` ~606).
  Ask it at a point that costs no extra model turn (for example once after init
  and again after a compaction or model change), and let it override the
  `modelUsage.contextWindow` fallback.
- Probe first: confirm the request shape and response with a small local
  `claude -p` stream-json probe (cheap; a single short prompt is fine). If the
  request is unsupported or unusable in this mode, STOP and report the evidence
  without implementing a settings-file fallback.
- Compatibility tier T0: no snapshot, record, or MCP shape change.
- Heavy `go build`/`go test`/`go vet` only through
  `~/.local/bin/ws-rgo -C agents-plugin-tool -m <min> -- <go args>`; never run
  them locally.

## Verification

- Unit tests in `internal/wsagent` for: status emitted per lead assistant
  usage message (not for subagent/alert ones); the window taken from the
  control response over `modelUsage.contextWindow`; the fallback when no
  response came.
- `go test ./internal/wsagent/... ./internal/wslive/...` and `go vet` through
  ws-rgo.
- `git diff` reviewed against this plan before commit. Commit on
  `impl/develop/hosted-context-meter` with `## Ticket Updates` naming the ticket.
