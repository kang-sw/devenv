# Codex CLI Integration Reference

Probed 2026-04-27 against `codex exec` on WSL2/Linux.
Source: https://developers.openai.com/codex/hooks, https://developers.openai.com/codex/config-reference

## Invocation

```bash
# New session
codex exec --dangerously-bypass-approvals-and-sandbox --json [OPTIONS] PROMPT < /dev/null

# Resume existing session
codex exec resume --dangerously-bypass-approvals-and-sandbox --json [OPTIONS] THREAD_ID PROMPT < /dev/null
```

`< /dev/null` is **required** in non-interactive contexts. Without it, codex reads from stdin
and blocks indefinitely. The "Reading additional input from stdin..." message on stderr is
cosmetic; it does not indicate an error when stdin is /dev/null.

## JSONL Output Format (`--json`)

Each line is a JSON event. Events emitted per turn:

```jsonl
{"type":"thread.started","thread_id":"019dce01-..."}
{"type":"turn.started"}
{"type":"item.started","item":{"id":"item_0","type":"command_execution",...}}
{"type":"item.completed","item":{"id":"item_0","type":"command_execution","aggregated_output":"...","exit_code":0,...}}
{"type":"item.completed","item":{"id":"item_1","type":"agent_message","text":"..."}}
{"type":"turn.completed","usage":{"input_tokens":13367,"cached_input_tokens":11648,"output_tokens":5,"reasoning_output_tokens":0}}
```

On stderr (non-fatal, ignore): `ERROR codex_core::session: failed to record rollout items: thread ... not found`

### Extraction

| Value | Path |
|---|---|
| Thread ID | `thread.started` → `thread_id` |
| Agent response | Last `item.completed` where `item.type == "agent_message"` → `item.text` |
| Token usage | `turn.completed` → `usage.input_tokens + cached_input_tokens + output_tokens` |

## Session File Format

Session files at `~/.codex/sessions/YYYY/MM/DD/rollout-<timestamp>-<thread_id>.jsonl`
use a **different format** from `--json` stdout. Each line is:

```json
{"timestamp":"<ISO>","type":"<event_type>","payload":{...}}
```

Relevant event types for parsing session history:

| `type` | `payload.type` | Meaning | Key field |
|--------|---------------|---------|-----------|
| `event_msg` | `task_started` | Turn begins | `turn_id` |
| `event_msg` | `task_complete` | Turn ends | `last_agent_message` |
| `event_msg` | `agent_message` | Assistant response | `payload.message` |
| `response_item` | `function_call` | Tool invocation | `payload.name`, `payload.arguments` |
| `response_item` | `function_call_output` | Tool result | `payload.output` |

Turn grouping: `event_msg{task_started}` → `event_msg{task_complete}`. An in-progress
turn has `task_started` with no matching `task_complete` yet.

## Session Management

- Sessions stored at: `~/.codex/sessions/YYYY/MM/DD/rollout-<timestamp>-<thread_id>.jsonl`
- Session detection: `find ~/.codex/sessions -name "rollout-*-<thread_id>.jsonl"`
- First call: codex assigns `thread_id` (UUID v7); extract from `thread.started` event
- Resume: `codex exec resume ... <thread_id> <prompt>` — same `thread_id` in `thread.started`
- Resume of nonexistent thread_id: exits non-zero with `Error: thread/resume failed: no rollout found`

## System Prompt Injection

Use `-c model_instructions_file=<path>` to inject a system prompt:

```bash
codex exec -c model_instructions_file="/tmp/prompt.txt" ...
```

This **replaces** codex default instructions. Do not pass when system prompt is empty.

`developer_instructions` via `-c` was tested but did not reliably inject content.

`SessionStart` hook with plain text stdout also works (additive developer context),
but `model_instructions_file` is simpler.

## Hook Configuration

Enable via `-c features.codex_hooks=true`.

### Injecting Hooks via `-c`

Hooks can be configured inline using TOML inline-table syntax:

```bash
codex exec \
  -c 'features.codex_hooks=true' \
  -c 'hooks.PostToolUse=[{hooks=[{type="command",command="/abs/path/cmd",timeout=5}]}]'
```

**Critical**: use PascalCase event names (`PostToolUse`, `SessionStart`, etc.).
Lowercase (`postToolUse`) is silently ignored.

Hooks config **cannot** be injected via `-c` using dotted-path nested syntax
(e.g., `hooks.PostToolUse.hooks=[...]` fails with "expected a sequence" error).
The full event key must take an array value directly.

### Hook Event Types

| Event | Fires | Useful for |
|---|---|---|
| `PostToolUse` | After each tool execution | Interrupt/mailbox check (exit 2 stops turn) |
| `SessionStart` | On session start and resume | Developer context injection |
| `PreToolUse` | Before each tool execution | Blocking commands |
| `Stop` | When turn concludes | Drain-loop alternative via `decision: "block"` |

### Exit Code Semantics

| Exit code | Effect |
|---|---|
| 0 | Continue normally |
| 2 + stderr | Blocks the tool; agent sees "blocked by environment hook" and stops turn |

The `WS_AGENT_OUTBOX` env var is inherited by the hook shell. Hook scripts can
read it directly without any per-call configuration.

## Model Flag Behavior

- Do **not** pass `--model codex` or `--model gemini` (backend shorthand names).
  These are not valid model identifiers for their respective CLIs.
- Pass `--model` only for explicit model names: `o3`, `gpt-4.1`, `gemini-2.0-flash`, etc.
- Omitting `--model` uses the CLI default model.

## PATH Inheritance

Codex agents inherit the calling process's PATH. Tools like `ws-infra-path`,
`ws-print-infra`, and other devenv scripts are available inside codex sessions
without additional configuration.

## Compression Notes

Codex does not support `--session-id` (pre-assigning a UUID before first call).
Thread IDs are always assigned by codex at session creation. Compression handoff
therefore spawns a new codex session and captures the assigned thread_id from
the `thread.started` event.

Intent extraction (step a) uses claude haiku as a backend-agnostic helper.
