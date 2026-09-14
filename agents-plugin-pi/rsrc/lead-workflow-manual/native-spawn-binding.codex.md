## Native delegate spawn

For native dispatch, map returned `recommended-model` and
`recommended-reasoning-effort` values to the matching `spawn_agent` fields.
Pass only returned bindings and use `fork_turns: "none"` because the rendered
prompt is self-contained.

If native spawn rejects a binding, report its field and value.
