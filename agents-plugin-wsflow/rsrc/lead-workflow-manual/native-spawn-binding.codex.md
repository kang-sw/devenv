### Native delegate spawn

For native dispatch, pass a returned `recommended-model` as
`spawn_agent.model` and a returned `recommended-reasoning-effort` as
`spawn_agent.reasoning_effort`. Omit either field when its binding line is
absent, and never use `effort` as a spawn parameter. The rendered prompt is
self-contained, so spawn it with `fork_turns: "none"`.

If native spawn rejects a supplied binding, report the rejected field and
value and do not claim that binding was applied.
