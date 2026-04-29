---
name: workflow
description: >
  Loads WS orchestration primitives reference into session context.
  Content survives compaction; re-invoke after compact if references are needed.
---

# Workflow

> **Session invariant:** Keep this skill's content active for the entire session.
> After compaction, re-invoke `/workflow` if orchestration API references are needed.
> Do not summarize away the primitive signatures below — they are the operative reference.

## On: invoke

No action required. This skill loads the WS orchestration primitives reference into
session context. Reading it is the act of invocation.

---

# WS Orchestration Primitives

PATH-accessible scripts provided by the `ws` plugin.
All scripts are on `$PATH` after `claude plugin install ws@ws`.

## ws-new-named-agent

```
ws-new-named-agent <agent-name> [-p <prompt>]... [--model <opus|sonnet|haiku>] [--no-doc-system] [--prompt-cond <binary>[=<prompt>]]...
```

Creates a named agent registry entry at `.git/ws@<repo-dir>/agents/<name>.json`.

- `-p <name-or-path>` — resolves against `infra/prompts/` first, then `infra/`, then cwd.
  Multiple `-p` flags are accepted; bodies are concatenated with `---` separators.
  The first document whose frontmatter declares `model:` sets the agent's model tier.
- `--model` — explicit model level; overrides frontmatter model.
- `--no-doc-system` — suppress auto-injection of `workflow-for-agent.md` into the system prompt.
- `--prompt-cond <binary>[=<prompt>]` — append the named prompt only if `<binary>` is found in
  PATH at registration time. Repeatable. Bare form uses binary name as prompt name.
- Legacy flags `--agent <ws:type>` and `--system-prompt <path>` still accepted.

Call at skill start for each agent slot. Overwrites any prior registry entry for
that name, which resets the session.

## ws-call-named-agent

```
ws-call-named-agent <agent-name> <prompt>
ws-call-named-agent <agent-name> - <<'PROMPT'
...
PROMPT
```

Calls the registered agent. Reads config from the registry entry; exits 1 with a
clear error if `ws-new-named-agent` has not been called for this name.

Auto-routes the session: `--resume` if a session file exists in `~/.claude/projects/`,
`--session-id` otherwise. Context length is managed transparently — long sessions
are compressed and handed off without caller involvement.

**Output:** agent response text to stdout and to `<registry-dir>/<name>.output.txt`. Exit code 1 on error.

**Background mode:** Pass `run_in_background: true` to let the lead continue other work.
Read output with `ws-print-named-agent-output` after the completion notification arrives.

## ws-interrupt-named-agent

```
ws-interrupt-named-agent <agent-name> <message>
ws-interrupt-named-agent <agent-name> - <<'MSG'
...
MSG
```

Queues a message into the named agent's outbox for delivery as a new user turn.
If the agent is running, the PostToolBatch hook stops it at the next tool boundary;
the drain loop then resumes with the queued content. If idle, delivered on next call.

## ws-print-named-agent-output

```
ws-print-named-agent-output <agent-name>
```

Prints the last response written by the named agent. Use after a background call completes.

## ws-named-agent erase

```
ws-named-agent erase <agent-name>
```

Removes the named agent's registry entry (`.json`, `.outbox.txt`, `.output.txt`) and
its Claude session file (`~/.claude/projects/*/<uuid>.jsonl`). Exits non-zero if the
agent is not found.

## ws-oneshot-agent

```
ws-oneshot-agent -p <prompt-stem> [-p <stem2>] [--model <tier>] [--no-doc-system] - <<'PROMPT'
...
PROMPT
```

Runs a full-tool agent for a single call, then erases it. Use when a task needs tool
access but no session persistence. Distinct from `ws-subquery` (which has no tool use).

Registry and session files are cleaned up via EXIT trap regardless of call outcome.

## ws-named-agent tail

```
ws-named-agent tail <agent-name> [-<n>]
```

Reads the last N assistant turns from the live session file without invoking the CLI —
safe to call while the agent is running. Defaults to 3 turns.

- `[last]` is a tool-results line → agent waiting for next turn (running or stalled)
- `[last]` is assistant with tools → waiting on tool results
- `[last]` is assistant with no tools → agent has concluded

## ws-review-path

```
ws-review-path <stem> [<stem> ...]
```

Allocates temp file paths for review findings. Paths embed a per-call `run_id` and are
**not reproducible** after the call returns — capture all output lines in a single call.

```bash
read -r CORR FIT TEST < <(ws-review-path correctness fit test)
```

Reviewers write findings to their allocated path; the implementer reads them directly.

## ws-infra-path

```
ws-infra-path <doc-name>
```

Returns the absolute path to an infra doc. Use only when a path string is needed
outside of `ws-new-named-agent` (e.g., `cat "$(ws-infra-path executor-wrapup.md)"`).

## ws-print-infra

```
ws-print-infra <stem>
```

Prints an infra doc to stdout. Accepts bare stem or `.md` extension.

## ws-ask-api

```
ws-ask-api [<domain-hint>] "<prompt>"
ws-ask-api --refresh <domain>
ws-ask-api --check-stale <domain>
ws-ask-api --list
```

Queries the project's `ai-docs/.deps/` external API documentation cache. On first use for
a domain, bootstraps a cache by fetching official docs. Subsequent calls are answered from
the cache; a stale check runs automatically before each answer.

- No hint: pre-router resolves relevant domains from the prompt. Multiple domains are
  dispatched in parallel; results are concatenated.
- With hint matching an existing `.deps/<hint>/` directory: pre-router is skipped.
- `--refresh <domain>`: force re-fetch all doc levels for a domain.
- `--check-stale <domain>`: report whether the cached version matches the project's current dependency.
- `--list`: print all cached domain names.

**Do not use `WebSearch` or `WebFetch` for external library API lookup** when `ws-ask-api`
is available. Use it for any question about a third-party library's API, types, or behavior.

## Usage Pattern

```bash
# 1. Register agent slots upfront
ws-new-named-agent implementer -p implementer
ws-new-named-agent reviewer-corr -p code-reviewer -p code-review-correctness
ws-new-named-agent reviewer-fit  -p code-reviewer -p code-review-fit

# 2. Call implementer
ws-call-named-agent implementer - <<'PROMPT'
Implement X
PROMPT

# 3. Parallel reviewers (multiple Bash calls in same response turn)
ws-call-named-agent reviewer-corr - <<PROMPT
Diff range: $START..HEAD
Write findings to: $CORR_PATH
PROMPT
ws-call-named-agent reviewer-fit - <<PROMPT
Diff range: $START..HEAD
Write findings to: $FIT_PATH
PROMPT

# 4. Fix loop — auto-resumes existing sessions
ws-call-named-agent implementer - <<'PROMPT'
Fix: <relay file paths from reviewer, not content>
PROMPT

# 5. Mid-task interrupt
ws-interrupt-named-agent implementer "Scope reduced to src/foo.ts only."
ws-print-named-agent-output implementer

# 6. Resident searcher — reset on domain shift
ws-new-named-agent searcher -p searcher
ws-call-named-agent searcher "Where is the session routing logic in ws-named-agent?"
```
