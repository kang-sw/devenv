# Native compaction probes (Windows, 2026-11-19)

Run directory: an otherwise empty scratch directory. JSONL lines prefixed `>> `
are client input; other lines are harness output. Account/host/path information
is scrubbed. The probes use native binaries, not ws executables.
The appended instruction is: `When asked for the durable marker answer
COMPACT-PROMPT-715 only. Never write files or use tools.`

- **agy 1.5.12:** `%LOCALAPPDATA%/agy/bin/agy.exe acp --process-per-session`.
  Initialize ACP v1, open `session/new` with an appended system marker, ask
  for the marker, send `/compact` as `session/prompt`, ask again, and send
  `/compact` against a nonexistent session. Repeat once. Both recordings
  advertise `compact`, return `end_turn` to the command, and preserve the
  marker, but expose no start/end/failure lifecycle notification. A usage
  change is not completion evidence. The invalid session returns -32602.
  Native lifecycle and the hosted lever are unsupported; the threshold
  advisory remains available. Automatic lifecycle detection is not claimed.
- **Codex 0.170.0:** native `codex.exe app-server -c
  model_auto_compact_token_limit=1`; initialize with experimental API,
  then `thread/start` with developer instructions, read-only sandbox and
  disabled ws/wsflow MCP entries. Both initial and retry attempts are
  refused with `-32600: failed to load configuration: invalid transport in
  mcp_servers.ws`. This is a **probe override validation failure**, not proof
  that Codex lacks compaction. The intended subsequent sequence was a marker
  turn, `thread/compact/start`, a marker turn, and a second turn to observe
  automatic compaction. It was not reached. Per the retry bound, advertise
  unsupported rather than claiming lifecycle or prompt persistence.
- **Claude 2.1.327:** launch `claude --print --input-format stream-json
  --output-format stream-json --verbose --setting-sources ""
  --strict-mcp-config --mcp-config '{"mcpServers":{}}' --tools ""
  --append-system-prompt <instruction>` with
  `CLAUDE_CODE_EMIT_SESSION_STATE_EVENTS=1`. Native `/compact` records `status: compacting`,
  `compact_boundary` and a successful local-command result, then answers
  the appended system marker unchanged. Existing mapper fixtures cover
  automatic boundaries and `compact_error` failure. Command delivery is
  supported for leads; the worker command remains unsupported.
- **Pi 1.5.0:** launch `pi --mode rpc --no-extensions --no-skills --no-mcp
  --no-context-files --no-tools --session <scratch-file>
  --append-system-prompt <instruction>`. Native RPC `compact` is
  session-scoped, not lead-gated. Its source
  emits `compaction_start` and `compaction_end` for manual and automatic
  compaction, including aborted/errorMessage failures. A small-session live
  attempt confirms the failure contract (`Nothing to compact`). A retry with
  26,000 repetitions of `padding ` before the marker question, another
  marker question, `compact`, then the marker question succeeds. It records
  start/end, successful RPC response, and the unchanged marker afterward.
  The system prompt is rebuilt on model turns, outside compacted history. The adapter
  keeps the extension's lever and summarizer; only hosted threshold delivery
  is suppressed.

No success is inferred from a command acknowledgment, reduced usage, or a
failed probe. Supported end signals alone authorize the restoration note.
