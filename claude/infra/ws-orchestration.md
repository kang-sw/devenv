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

**Output:** Formatted text to stdout.

```
[info] Context window: N% filled [— recommended to refresh this agent]

<agent response text>
```

The info line is always first. A blank line separates it from the agent output. Exit code is 1 when the underlying call reports `is_error`.

Context window percentage: `(input + cache_creation + cache_read) / 150K`. Prefix is `[info]` below 70%, `[warn]` at ≥70% (~105K tokens).

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
  "Implement X"

# 3. Parallel reviewers — issue multiple Bash calls in the same response
ws-call-agent sonnet --agent reviewer-corr \
  --system-prompt claude/infra/code-review-correctness.md \
  "$(git diff HEAD~1)"

# 4. Fix loop — --agent auto-resumes the existing session
ws-call-agent sonnet --agent implementer \
  "Fix these issues: ..."

ws-call-agent sonnet --agent reviewer-corr \
  "Re-review. Updated diff: ..."
```
