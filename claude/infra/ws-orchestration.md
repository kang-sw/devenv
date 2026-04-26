# WS Orchestration Primitives

Bash-callable scripts at `claude/bin/` for Team-free subagent coordination.
All scripts are on `$PATH` via `claude/bin/`.

## ws-new-named-agent

```
ws-new-named-agent <agent-name> [--agent <type>] [--system-prompt <path>] [--model <opus|sonnet|haiku>]
```

Creates a named agent registry entry at `.git/ws@<repo-dir>/agents/<name>.json`.

- `--agent <type>` — agent type forwarded to the `claude` CLI.
- `--system-prompt <path>` — file path; content is stored in the registry at registration time.
- `--model` — model level (`opus`, `sonnet`, `haiku`). Defaults to `sonnet`.

Call at skill start for each agent slot. Overwrites any prior registry entry for
that name, which resets the session.

## ws-call-named-agent

```
ws-call-named-agent <agent-name> <prompt>
```

Calls the registered agent. Reads config from the registry entry; exits 1 with a
clear error if `ws-new-named-agent` has not been called for this name.

Auto-routes the session: `--resume` if a session file exists in `~/.claude/projects/`,
`--session-id` otherwise. Context length is managed transparently — long sessions
are compressed and handed off without caller involvement.

**Output:** agent response text to stdout and to `<registry-dir>/<name>.output.txt`. Exit code 1 on error.

**Bash tool timeout:** Always pass `timeout: 600000` (the 10-minute max) when calling `ws-call-named-agent` via the Bash tool. Agent tasks routinely exceed the 120s default, which causes silent background detachment.

**Background mode:** Pass `run_in_background: true` on the Bash tool call to let the lead agent continue other work while the agent runs. Output is still written to the output file; read it with `ws-print-named-agent-output` after the completion notification arrives.

## ws-interrupt-named-agent

```
ws-interrupt-named-agent <agent-name> <message>
ws-interrupt-named-agent <agent-name> - <<'MSG'
...
MSG
```

Queues a message into the named agent's outbox. The message is delivered as a
new user turn on the agent's next `--resume` call.

**Delivery timing:**

- **Agent running** via `ws-call-named-agent` — the PostToolBatch hook fires after
  each tool batch, detects the non-empty outbox, and exits with code 2. This stops
  the agent cleanly at the next tool boundary. The drain loop in `ws-call-named-agent`
  then resumes with the queued message.
- **Agent idle** — the message waits in the outbox. The next `ws-call-named-agent`
  call picks it up via the same drain loop before returning.

Multiple `ws-interrupt-named-agent` calls append to the outbox. The drain loop
delivers them one at a time in order.

The agent receives the message as a prompt but is not obligated to act on it. Use
for course-correction, cancellation signals, or additional context mid-task.

## ws-print-named-agent-output

```
ws-print-named-agent-output <agent-name>
```

Prints the last response written by the named agent. Use after a background `ws-call-named-agent` completes to read its output.

```bash
# Background spawn — lead continues other work
# (Bash tool call with run_in_background: true)
ws-call-named-agent implementer - <<'PROMPT'
Implement X
PROMPT

# After completion notification:
ws-print-named-agent-output implementer
```

## ws-infra-path

```
ws-infra-path <doc-name>
```

Returns the absolute path to an infra doc. Use for `--system-prompt` arguments where
a path string is required rather than file content.

```bash
ws-new-named-agent implementer --system-prompt "$(ws-infra-path implementer.md)"
```

## ws-review-path

```
ws-review-path <stem> [<stem> ...]
```

Allocates temp file paths for review findings. Generates a pwd-scoped, per-call run-id
so concurrent invocations across projects do not collide. Paths are **not reproducible**
after the call returns — capture all output lines in a single call.

```bash
read -r CORR FIT TEST < <(ws-review-path correctness fit test)
```

Reviewers write findings to their allocated path; the implementer reads them directly.
This avoids token overhead from relaying review text through the lead.

## Usage Pattern

```bash
# 1. Register all agent slots upfront (creates fresh sessions; stores system prompts)
ws-new-named-agent implementer --model sonnet --system-prompt "$(ws-infra-path implementer.md)"
ws-new-named-agent reviewer-corr --agent ws:code-reviewer --system-prompt "$(ws-infra-path code-review-correctness.md)"
ws-new-named-agent reviewer-fit --agent ws:code-reviewer --system-prompt "$(ws-infra-path code-review-fit.md)"

# 2. Call implementer — auto-starts session on first call
ws-call-named-agent implementer - <<'PROMPT'
Implement X
PROMPT

# 3. Parallel reviewers — issue multiple Bash calls in the same response turn
DIFF=$(git diff HEAD~1)
ws-call-named-agent reviewer-corr - <<PROMPT
$DIFF
PROMPT
ws-call-named-agent reviewer-fit - <<PROMPT
$DIFF
PROMPT

# 4. Fix loop — auto-resumes existing sessions
ws-call-named-agent implementer - <<'PROMPT'
Fix these issues: ...
PROMPT
ws-call-named-agent reviewer-corr - <<'PROMPT'
Re-review. Updated diff: ...
PROMPT

# 5. Mid-task interrupt — queue a message while the agent is running in background
#    (Bash tool call with run_in_background: true already issued for implementer)
ws-interrupt-named-agent implementer "Stop after the current file. Scope reduced to src/foo.ts only."

# After the background call completes, the drained message will have been delivered.
ws-print-named-agent-output implementer
```
