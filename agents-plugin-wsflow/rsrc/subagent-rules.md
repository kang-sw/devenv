# Subagent Dispatch Rules

Rules injected by the caller when spawning general-purpose delegated workers.
Callers paste the relevant sections into the prompt; delegates do not read this
file directly.

## Exploration Helper

For scoped exploration beyond direct file search:

```text
Caller: run ws/playbook.render(name: "explore").
Caller: pass the rendered brief path and the scoped question to a host-native Explore-capable worker.
Caller: collect a concise report with the answer, cited evidence, gaps, and follow-up needs.
```

Prefer direct file reads and search when the target is known. Use the rendered
`explore` playbook with a host-native Explore-capable worker when sequential
searches would flood context, or for broad cross-module tracing. If passing a
brief file is not practical, call `ws/playbook.print(name: "explore")` and paste
the rendered guidance directly into the delegated worker prompt with the scoped
question.

## Branches

The caller creates and checks out the working branch before spawning the
delegate. Commit on the current branch at logical checkpoints. Keep commit
messages brief; the caller merges back.

## General Rules

- All output in English regardless of message language.
- Do not modify files outside the task scope without escalating to the caller.
- Do not merge sub-branches; the caller reviews and merges after the report.
