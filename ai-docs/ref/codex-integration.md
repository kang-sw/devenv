# Codex CLI Integration Reference

Probed 2026-04-27 against `codex exec` on WSL2/Linux.
Source: https://developers.openai.com/codex/hooks, https://developers.openai.com/codex/config-reference

## Plugin Operations

Probed 2026-05-02 against a repo-local Codex marketplace for
`/Users/kang-sw/devenv`.

Repo-local marketplace registration works through:

```bash
codex plugin marketplace add /Users/kang-sw/devenv
```

Observed Codex config:

```toml
[marketplaces.kang-sw-devenv]
source_type = "local"
source = "/Users/kang-sw/devenv"
```

Local marketplace registration does not install the plugin. The user must install
the listed plugin from the Codex Plugins UI at least once.

No supported CLI-level plugin install, uninstall, or updater command was found for
this local workflow. `codex plugin marketplace upgrade <name>` is for Git-backed
marketplaces and fails for `source_type = "local"`.

Iterative local plugin testing uses UI uninstall/install or a fresh Codex session
after editing the registered local source. Verified after UI uninstall/install:
`$ws:skill-authoring`, `$ws:write-ticket`, and `$ws:discuss` are visible.

Skill invocation is namespaced as `$<plugin-name>:<skill-name>`; for this repo's
candidate plugin the form is `$ws:<skill-name>`.

## Plugin-Managed MCP

Verified 2026-05-03 by checking official `openai/plugins` examples and local Codex
CLI surfaces.

Codex plugin bundles can include MCP server configuration:

```json
{
  "skills": "./skills/",
  "mcpServers": "./.mcp.json"
}
```

The plugin-local `.mcp.json` uses the usual MCP server map shape:

```json
{
  "mcpServers": {
    "server-name": {
      "command": "some-command",
      "args": ["arg1"],
      "startup_timeout_sec": 30,
      "tool_timeout_sec": 600
    }
  }
}
```

Codex configuration supports MCP server timeout fields:
`startup_timeout_sec` overrides the default 10-second server startup timeout,
and `tool_timeout_sec` overrides the default 60-second per-tool timeout. The ws
plugin bundles these fields in `agents-plugin/.mcp.json` to align Codex's
outer MCP timeout with ws's 10-minute named-agent wait/result defaults.

Official examples:

- `openai/plugins/plugins/build-ios-apps` declares `"mcpServers": "./.mcp.json"`
  and uses a stdio MCP server launched through `npx`.
- `openai/plugins/plugins/cloudflare` declares `"mcpServers": "./.mcp.json"` and
  uses an HTTP MCP server.

Observed CLI support remains separate:

```bash
codex mcp add <name> -- <command>...
codex mcp add <name> --url <url>
codex mcp list
codex mcp get <name>
```

For repo-local plugin iteration, changed plugin-managed MCP configuration is not
known to refresh automatically. Treat Codex UI uninstall/install, or a fresh Codex
session after editing the registered local source, as a required human-in-the-loop
cache refresh step before validating plugin-managed MCP changes. Agents should
explicitly ask the user to perform that refresh when a verification step depends on
the installed plugin cache.

For the `ws` plugin candidate's own MCP runtime contract, see
`ai-docs/ref/ws-mcp.md`.

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

Update from 2026-05-04 on Codex CLI 0.128.0 / WSL2 Linux: the inline
`hooks.PostToolUse` form below fires during `codex exec --json`. The important
host difference from the Claude prior art is semantic rather than configurational:
`PostToolUse` `exit 2` injects hook feedback into the next model step instead
of stopping the Codex subprocess and returning control to the wrapper.

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
| `PostToolUse` | After each tool execution | Interrupt/mailbox check (exit 2 injects feedback) |
| `SessionStart` | On session start and resume | Developer context injection |
| `PreToolUse` | Before each tool execution | Blocking commands |
| `Stop` | When turn concludes | Drain-loop alternative via `decision: "block"` |

### Exit Code Semantics

| Exit code | Effect |
|---|---|
| 0 | Continue normally |
| 2 + stderr | For `PostToolUse`, inject stderr as hook feedback into the next model step and continue the turn |

For `PostToolUse`, plain stdout text was ignored in smoke testing. JSON stdout
with `decision: "block"` and `hookSpecificOutput.additionalContext` also reached
the next model step. Use stderr plus exit 2 for the simple mailbox delivery
path unless a structured hook result is needed.

Codex hook commands receive hook metadata as JSON on stdin. The ws Codex
adapter passes the repository root and agent name in the configured hook command
instead of relying on a Claude-style `WS_AGENT_OUTBOX` environment variable.

## Model Flag Behavior

- Do **not** pass `--model codex` or `--model gemini` (backend shorthand names).
  These are not valid model identifiers for their respective CLIs.
- Pass `--model` only for explicit model names: `o3`, `gpt-4.1`, `gemini-2.0-flash`, etc.
- Omitting `--model` uses the CLI default model.

## PATH Inheritance

Codex agents inherit the calling process's PATH. Shared ws workflow behavior
should use MCP tools rather than retired `ws-*` helper scripts.

## Compression Notes

Codex does not support `--session-id` (pre-assigning a UUID before first call).
Thread IDs are always assigned by codex at session creation. Compression handoff
therefore spawns a new codex session and captures the assigned thread_id from
the `thread.started` event.

Intent extraction (step a) uses claude haiku as a backend-agnostic helper.
