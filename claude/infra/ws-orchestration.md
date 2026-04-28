# WS Orchestration Primitives

PATH-accessible scripts provided by the `ws` plugin.
All scripts are on `$PATH` after `claude plugin install ws@ws`.

## ws-new-named-agent

```
ws-new-named-agent <agent-name> [--agent <type>] [--system-prompt <path>] [--model <opus|sonnet|haiku>]
```

Creates a named agent registry entry at `.git/ws@<repo-dir>/agents/<name>.json`.

- `--agent <type>` — agent type forwarded to the `claude` CLI.
- `--system-prompt <name-or-path>` — content stored in the registry at registration time.
  Bare name (no path separator): resolved against `infra/` first, then the working directory, then error.
  Explicit path (separator present or absolute): read directly.
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

Queues a message into the named agent's outbox for delivery as a new user turn.
If the agent is running, the PostToolBatch hook stops it at the next tool boundary;
`ws-call-named-agent`'s drain loop then resumes with the message. If the agent is
idle, the message is delivered on the next `ws-call-named-agent` call.

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

## ws-named-agent tail

```
ws-named-agent tail <agent-name> [-<n>]
```

Reads the last N assistant turns from the live session file on disk without invoking
the CLI — safe to call while the agent is running. Defaults to 3 turns.

Output per turn: assistant text preview and tools called, interleaved with tool-result
counts. Use to distinguish frozen from done:

- `[last]` is a tool-results line → agent waiting for next turn (running or stalled)
- `[last]` is assistant with tools → waiting on tool results
- `[last]` is assistant with no tools → agent has concluded its response

```bash
ws-named-agent tail implementer -5
```

## ws-infra-path

```
ws-infra-path <doc-name>
```

Returns the absolute path to an infra doc. Prefer passing bare names directly to
`--system-prompt` instead — `ws-new-named-agent` resolves them automatically.
Use `ws-infra-path` only when a path string is needed outside of `ws-new-named-agent`
(e.g., `cat "$(ws-infra-path implementer.md)"`).

```bash
# preferred — bare name resolved automatically
ws-new-named-agent implementer --system-prompt implementer.md

# use ws-infra-path only when the path itself is needed
cat "$(ws-infra-path implementer.md)"
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
ws-new-named-agent implementer --model sonnet --system-prompt implementer.md
ws-new-named-agent reviewer-corr --agent ws:code-reviewer --system-prompt code-review-correctness.md
ws-new-named-agent reviewer-fit --agent ws:code-reviewer --system-prompt code-review-fit.md

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

# 5. Mid-task interrupt (while implementer runs in background)
ws-interrupt-named-agent implementer "Stop after the current file. Scope reduced to src/foo.ts only."
ws-print-named-agent-output implementer

# 6. Resident searcher — spawn once per domain, reset on domain shift
ws-new-named-agent searcher --system-prompt searcher.md
ws-call-named-agent searcher "Where is the session routing logic in ws-named-agent?"
# Domain shifts: call ws-new-named-agent searcher again to reset context
```
