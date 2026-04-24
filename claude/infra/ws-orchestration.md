# WS Orchestration Primitives

Bash-callable scripts at `claude/bin/` for Team-free subagent coordination.
All scripts are on `$PATH` via `claude/bin/`.

## ws-call-agent

```
ws-call-agent <model> [--agent <name>] [--session-id <uuid>] [--uuid <uuid>]
              [--system-prompt <path>] "<prompt>"
```

Wraps `claude -p` with permission bypass and JSON output.

**Flags:**

| Flag | Maps to | Notes |
|------|---------|-------|
| `--agent <name>` | auto-routes | Computes deterministic UUID via `ws-agent`, then `--session-id` (no file) or `--resume` (file exists) |
| `--session-id <uuid>` | `claude --session-id` | Create-only — errors if UUID already exists |
| `--uuid <uuid>` | `claude --resume` | Continue-only — errors if session absent |
| `--system-prompt <path>` | `claude --system-prompt` | Reads file, injects as system prompt |

**Model routing:** `claude*` / `sonnet` / `haiku` / `opus` → `claude` CLI. `gemini*` → not yet implemented.

**Output:** JSON object to stdout.

```json
{ "session_id": "...", "result": "...", "is_error": false, ... }
```

**Safe parsing patterns:**

```bash
# Pipe-direct — always safe
ws-call-agent sonnet --agent impl "..." | jq -r '.result'

# UUID extraction — ASCII-only, safe via $()
UUID=$(ws-call-agent sonnet --agent impl "..." | jq -r '.session_id')
```

> `$(ws-call-agent ...)` captured to a variable corrupts multi-byte characters
> in `.result`. Use pipe-direct when reading the response text.

## ws-agent

```
ws-agent <name>  →  prints deterministic UUID v5
```

Derives UUID from repo root + git branch + agent name. Same name on the same
branch always produces the same UUID. Output is ASCII-only — safe for `$()`.

## ws-declare-agent

```
ws-declare-agent <name> [<name2> ...]
```

Clears session files for the given names so the next `ws-call-agent --agent`
starts a fresh session. Idempotent — no-op when no session exists.

**Call at skill start** before any `ws-call-agent --agent` call, listing all
agent slots the skill will use.

## Usage Pattern

```bash
# 1. Declare all slots upfront (clears stale sessions from prior runs)
ws-declare-agent implementer reviewer-corr reviewer-fit

# 2. Start sessions — --agent creates fresh after declare
ws-call-agent sonnet --agent implementer \
  --system-prompt claude/infra/implementer.md \
  "Implement X" | jq -r '.result'

# 3. Parallel reviewers — issue multiple Bash calls in the same response
ws-call-agent sonnet --agent reviewer-corr \
  --system-prompt claude/infra/code-review-correctness.md \
  "$(git diff HEAD~1)" | jq -r '.result'

# 4. Fix loop — --agent auto-resumes the existing session
ws-call-agent sonnet --agent implementer \
  "Fix these issues: ..." | jq -r '.result'

ws-call-agent sonnet --agent reviewer-corr \
  "Re-review. Updated diff: ..." | jq -r '.result'
```
