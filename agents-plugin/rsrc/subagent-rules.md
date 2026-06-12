# Subagent Dispatch Rules

Rules injected by the caller when spawning general-purpose delegated workers.
Callers paste the relevant sections into the prompt; delegates do not read this
file directly.

## Exploration Helper

For scoped exploration beyond direct file search:

```text
ws/subquery(question: "<question>")
ws/subquery(deep_research: true, question: "<question>")
```

Prefer direct file reads and search when the target is known. Use `ws/subquery`
when sequential searches would flood context, or for broad cross-module tracing.

## Branches

The caller creates and checks out the working branch before spawning the
delegate. Commit on the current branch at logical checkpoints. Keep commit
messages brief; the caller merges back.

## General Rules

- All output in English regardless of message language.
- Do not modify files outside the task scope without escalating to the caller.
- Do not merge sub-branches; the caller reviews and merges after the report.
