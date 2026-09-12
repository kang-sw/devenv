---
summary: Probed Codex CLI behavior
---

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
`$ws:lead-write-ticket` and `$ws:lead-discuss` are visible; that skill is
`$ws:lead-ticket` since the 2026-09-09 lead-surface collapse renamed it. The probe also
covered `$ws:lead-skill-authoring` until 2026-07-28, when that skill was
relocated out of the plugin surface to `ai-docs/manuals/skill-authoring.md`.

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

For launcher and verification runbook steps, see `ai-docs/manuals/ws-mcp.md`.

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

Per official docs (`developers.openai.com/codex/hooks`, checked 2026-07-08):
hooks are **enabled by default**; disable via `[features] hooks = false` in
`config.toml` (admins can force this via `requirements.toml`). The
`-c features.codex_hooks=true` flag documented below was empirically required
on Codex CLI 0.128.0 (2026-05-04) and may be stale for current CLI versions —
re-verify the enablement flag against the installed CLI version before relying
on either form.

Hooks can be bundled at multiple layers, loaded together (higher-precedence
layers do not replace lower ones):

- User-level: `~/.codex/hooks.json` or inline `[hooks]` in `~/.codex/config.toml`
- Project-level: `<repo>/.codex/hooks.json` or inline `[hooks]` in `<repo>/.codex/config.toml`
- **Plugin-bundled**: `hooks/hooks.json` inside the plugin root (or a path named
  in the plugin manifest) — same shape as Claude's plugin-level hook bundling.
- Managed (Enterprise): `requirements.toml` `[hooks]` tables

`SessionStart` runs at thread (main-session) scope; subagent/sub-process
initialization fires a separate `SubagentStart` event instead — mirroring the
Claude-side split where `SessionStart` never reaches Task-tool subagents.

Update from 2026-05-04 on Codex CLI 0.128.0 / WSL2 Linux: the inline
`hooks.PostToolUse` form below fires during `codex exec --json`. The important
host difference from the Claude prior art is semantic rather than configurational:
`PostToolUse` `exit 2` injects hook feedback into the next model step instead
of stopping the Codex subprocess and returning control to the wrapper.
**Superseded on Codex 0.154.0 — see the 2026-09-13 re-probe below; on 0.154.0
`PostToolUse` no longer steers the model at all.**

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

### Re-probe 2026-09-13 (Codex CLI 0.154.0, macOS)

Re-verified the hook path on a current CLI in an isolated `codex exec`
(`--ignore-user-config --ephemeral --skip-git-repo-check`, inline `-c` hooks,
no plugin). Findings that supersede the 0.128.0 notes above:

- **Hook trust is now gated.** 0.154.0 requires *persisted hook trust*; enabled
  hooks do not run non-interactively without it. Ad-hoc/inline hooks need
  `--dangerously-bypass-hook-trust` for automation, or the hook source must be
  persisted as trusted. A real adapter deployment persists trust rather than
  passing the dangerous flag.
- **`Stop` + `decision: block` steers the model — confirmed.** A `Stop` hook
  returning `{"decision":"block","reason":"<instruction>"}` re-invokes the model
  with `reason` as an instruction (the model ran the injected command), then
  concludes. The re-entry `Stop` fires with `stop_hook_active: true` (loop
  guard). This is the load-bearing turn-boundary wake/drain mechanism.
- **`PostToolUse` output never reaches the model.** The hook still *fires* (it
  can observe and produce side effects), but nothing it emits reaches the model
  on 0.154.0: not `exit 2` + stderr, not JSON `decision: block` + `reason`, not
  `hookSpecificOutput.additionalContext`. Verified even with a *passive* note
  (exit 0, `additionalContext` asking the model to append a codeword to its
  final message) — the codeword never surfaced, so the content did not reach the
  model at all. Asymmetry: `Stop`'s `reason` reaches the model, `PostToolUse`'s
  does not. A Codex `PostToolUse` hook is therefore side-effect-only (write a
  marker, check state); it cannot notify or steer the model mid-turn. This
  reverses the 2026-05-04 `exit 2` finding. Anything that must reach the model
  mid-turn has to travel through a channel the model reads (e.g. an MCP tool
  response), not a `PostToolUse` hook.
- **Hook payload carries no agent classifier.** `Stop` stdin is
  `{session_id, turn_id, transcript_path, cwd, hook_event_name, model,
  permission_mode, stop_hook_active, last_assistant_message}`; `PostToolUse`
  adds `{tool_name, tool_input, tool_response, tool_use_id}`. There is no
  `agent_id`/`agent_type`/agent-name field, so a Codex hook cannot structurally
  tell a main turn from a subagent turn from the payload alone (contrast Claude,
  whose `Stop`/`SubagentStop` split plus optional `agent_id`/`agent_type`
  does). Owner/main-turn context must be baked into the hook command args by the
  adapter, per the note above. Payload is delivered on **stdin**, not argv.
- **Subagent spawn needs the app-server daemon.** In `--ephemeral` `codex exec`
  the base agent has a subagent-spawn tool but the spawn fails
  (`no thread with id ...`); subagent turns (and thus any `SubagentStart`/
  `SubagentStop` firing) could not be observed without the shared local
  app-server daemon (`codex agents`). Whether Codex subagents fire hooks at all
  remains open and needs a daemon-backed probe.
- Useful isolation flags on `codex exec`: `-C/--cd`, `--ignore-user-config`
  (auth still uses `CODEX_HOME`), `--ephemeral`, `--skip-git-repo-check`.

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
