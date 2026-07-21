### Delegated playbook bindings

After `playbook.render`, pass its first-line prompt path to the native
subagent. Map `recommended-model: <model>` to `spawn_agent.model`; map an
optional `recommended-reasoning-effort: <effort>` to
`spawn_agent.reasoning_effort`. Never use `effort` as a spawn parameter.

Treat both concrete bindings as optional. If native spawn rejects one, report
the rejected field and value, then retry without that field; do not claim the
rejected binding was applied. Preserve `recommended-tier` as the portable
capability label.

<!-- ws:full-only:start -->
When native spawn cannot honor the configured binding and the exact local
mapping matters, use the mercenary fallback: pass the rendered prompt as
`system_prompt_text` and its `recommended-tier` as `tier` to
`mercenary.register`, then call the registered mercenary. That path resolves
the same local tier mapping.
<!-- ws:full-only:end -->
