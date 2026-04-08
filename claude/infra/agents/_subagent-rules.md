# Subagent Dispatch Rules

Rules injected by the caller when spawning general-purpose (non-native,
non-team) subagents. Callers paste the relevant sections into the spawn
prompt — subagents do not read this file directly.

## Exploration Helper

For scoped exploration beyond direct Read/Grep/Glob tools:

```bash
bash ~/.claude/infra/ask.sh "<question>"                  # haiku
bash ~/.claude/infra/ask.sh --deep-research "<question>"  # sonnet
```

Prefer direct Read/Grep/Glob when the target is known. Use `ask.sh`
when sequential searches would flood the subagent's context, or for
external lookups. `--deep-research` for cross-module tracing, API-usage
nuance, or when strict cited output is needed.

## Branches

The caller creates and checks out the working branch before spawning
the subagent. Commit on the current branch at logical checkpoints. Keep
commit messages brief; the caller merges back.

## General Rules

- All output in English regardless of message language.
- Do not modify files outside the task scope without escalating to the caller.
- Do not merge sub-branches — the caller reviews and merges after the report.
