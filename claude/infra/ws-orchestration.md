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
`--session-id` otherwise. Context length is managed transparently — long sessions
are compressed and handed off without caller involvement.

**Output:** agent response text to stdout. Exit code 1 on error.

## ws-infra-path

```
ws-infra-path <doc-name>
```

Returns the absolute path to an infra doc. Use for `--system-prompt` arguments where
a path string is required rather than file content.

```bash
ws-new-agent implementer --system-prompt "$(ws-infra-path implementer.md)"
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
