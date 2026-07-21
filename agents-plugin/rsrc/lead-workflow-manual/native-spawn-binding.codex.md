### Delegated playbook bindings

`playbook.render` returns the prompt path first, then `recommended-tier` when
declared, optional `recommended-model`, and optional
`recommended-reasoning-effort`. When both optional binding lines are present,
dispatch the native worker with this complete shape:

```text
spawn_agent({
  task_name: "<stable_task_name>",
  message: "Read and execute the complete prompt at <first-line-prompt-path>.",
  fork_turns: "none",
  model: "<recommended-model>",
  reasoning_effort: "<recommended-reasoning-effort>"
})
```

Map only present metadata: include `model` only for a `recommended-model`
line, and include `reasoning_effort` only for a
`recommended-reasoning-effort` line. Omit a parameter whose line is absent;
never infer a missing binding or claim an omitted or rejected value was
applied. Never use `effort` as a spawn parameter. Preserve
`recommended-tier` as the portable capability label; do not pass it to
`spawn_agent`.

Before any degraded retry, decide whether the user or invoking workflow
requires the rendered exact model or effort. When exact fidelity is required,
do not launch a degraded native spawn if a required binding is absent or
rejected.

<!-- ws:full-only:start -->
Use the mercenary fallback described under **Persistent agents** when exact
fidelity prevents native dispatch.
<!-- ws:full-only:end -->
If no exact-binding fallback is available, stop and report the absent or
rejected binding to the invoking lead or user.

When exact fidelity is not required, remove only a field that native spawn
explicitly identifies as rejected, disclose the degraded binding, and retry.
Treat a rejection as ambiguous unless it explicitly names one rejected field;
for an ambiguous rejection, do not retry native or guess which field failed.
Report the unchanged error to the invoking lead or user, or use the fallback
when exact fidelity requires it and the fallback is available.

Retain the returned agent ID or canonical task name. For continuity turns, call
`followup_task(target: "<agent-id-or-canonical-task-name>", message: "<continuity-prompt>")`;
do not call `spawn_agent` again.
