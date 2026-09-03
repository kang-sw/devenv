# Subagent Dispatch Rules

Rules injected by the caller when spawning general-purpose delegated workers.
Callers paste the relevant sections into the prompt; delegates do not read this
file directly.

## Exploration Helper

For scoped exploration beyond direct file search:

```text
Caller: spawn a host-native exploration worker with an English scoped task prompt.
Caller: collect a concise report with the answer, cited evidence, gaps, and follow-up needs.
```

Prefer direct file reads and search when the target is known. Use a
host-native exploration worker when the investigation requires broad or
multi-step search that would consume too much main-session context, or for broad
cross-module tracing. The worker prompt must include the scoped question,
read-only boundary, expected evidence citations, gaps, and follow-up needs.

## Branches

The caller creates and checks out the working branch before spawning the
delegate. Commit on the current branch at logical checkpoints. Keep commit
messages brief; the caller merges back.

## General Rules

- All output in English regardless of message language.
- Do not modify files outside the task scope without escalating to the caller.
- Do not merge sub-branches; the caller reviews and merges after the report.
