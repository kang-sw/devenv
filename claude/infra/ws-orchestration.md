# WS Orchestration Primitives

Bash-callable scripts at `claude/bin/` for Team-free subagent coordination.
All scripts are on `$PATH` via `claude/bin/`.

## ws-new-agent

```
ws-new-agent <agent-name> [--agent <type>] [--system-prompt <path>] [--model <opus|sonnet|haiku>]
```

Creates a named agent registry entry at `.git/ws@<repo-dir>/agents/<name>.json`.

- `--agent <type>` — agent type forwarded to the `claude` CLI.
- `--system-prompt <path>` — file path; content is stored in the registry at registration time.
- `--model` — model level (`opus`, `sonnet`, `haiku`). Defaults to `sonnet`.

Call at skill start for each agent slot. Overwrites any prior registry entry for
that name, which resets the session.

## ws-call-agent

```
ws-call-agent <agent-name> <prompt>
```

Calls the registered agent. Reads config from the registry entry; exits 1 with a
clear error if `ws-new-agent` has not been called for this name.

Auto-routes the session: `--resume` if a session file exists in `~/.claude/projects/`,
`--session-id` otherwise.

**Token tracking:** writes `input_tokens + cache_creation_input_tokens` to `token_count`
in the registry after each call. Emits a context fill line to stderr when fill ≥ 25%:

```
[info] Context: N% filled (M tokens)
```

**Auto-compression:** when `token_count > 100K`, compresses the existing session via
`agent-compression.md` and hands off to a fresh agent (3-call flow). The immediate
next call is the handoff; re-compression is suppressed on that call.

**Output:** agent response text to stdout. Exit code 1 on error.

## Usage Pattern

```bash
# 1. Register all agent slots upfront (creates fresh sessions; stores system prompts)
ws-new-agent implementer --model sonnet --system-prompt "$(ws-infra-path implementer.md)"
ws-new-agent reviewer-corr --agent ws:code-reviewer --system-prompt "$(ws-infra-path code-review-correctness.md)"
ws-new-agent reviewer-fit --agent ws:code-reviewer --system-prompt "$(ws-infra-path code-review-fit.md)"

# 2. Call implementer — auto-starts session on first call
ws-call-agent implementer "Implement X"

# 3. Parallel reviewers — issue multiple Bash calls in the same response turn
ws-call-agent reviewer-corr "$(git diff HEAD~1)"
ws-call-agent reviewer-fit "$(git diff HEAD~1)"

# 4. Fix loop — auto-resumes existing sessions
ws-call-agent implementer "Fix these issues: ..."
ws-call-agent reviewer-corr "Re-review. Updated diff: ..."
```
