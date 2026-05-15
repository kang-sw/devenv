# Brief: 260513-feat-agent-tier-effort-config Phase 3

## Intent

Close the remaining documentation and runtime-metadata verification slice for
named-agent effort configuration. Phase 1 implemented alias effort storage and
metadata visibility. Phase 2 implemented Codex and Claude runner application.
Phase 3 should make the durable user-facing references match that behavior and
verify the plugin runtime contract is not stale.

## Approach

- Update user-facing configuration guidance so model aliases are the single
  route for named-agent effort selection.
- Document the portable effort values and no-override behavior in the runtime
  reference surfaces that explain `config.agents_tier`, CLI mirrors, and
  `agents.register`.
- Verify `agents-plugin/runtime.json` and wsflow runtime metadata against the
  actual public tool and command surfaces. Edit runtime metadata only if a real
  contract drift exists.
- Keep wsflow agentless behavior unchanged unless runtime contract validation
  shows drift.

## Constraints

- Implement only Phase 3. Do not change runner behavior, alias resolution,
  config storage, or MCP schemas unless verification finds a real runtime
  metadata mismatch.
- Do not add direct effort input to `agents.register`, `subquery`, prompt
  frontmatter, or workflow skill calls.
- Preserve the default no-override behavior: empty, omitted, or `none` effort
  means no backend effort argument.
- Keep documentation in English.
- Use `model aliases`, not workload tiers, for new guidance. The compatibility
  name `config.agents_tier` and CLI command `config agents-tier` may remain as
  literal API names.

## Out Of Scope

- Releasing or bumping the ws version.
- Adding Gemini effort support.
- Changing wsflow's no-agent surface.
- Re-running Phase 1 or Phase 2 implementation.

## Details

- `ai-docs/ref/ws-agent-runtime.md` should describe alias effort configuration,
  the accepted effort values, no-override clearing behavior, runner application,
  and the CLI `--effort` option for `config agents-tier`.
- `ai-docs/ref/ws-mcp.md` should describe the `effort` input field on
  `ws/config.agents_tier`, the stored no-override behavior, and the fact that
  `agents.register` has no direct effort input.
- `agents-plugin/runtime.json` should remain unchanged if the only changes are
  schema/detail behavior under existing `config.agents_tier`, `agents.register`,
  Codex, and Claude surfaces.
- If runtime metadata is unchanged, record that in the completion report and
  ticket result instead of manufacturing a metadata diff.

## References

- [Must] `ai-docs/tickets/ready/260513-feat-agent-tier-effort-config.md` -
  Phase 3 acceptance criteria.
- [Must] `ai-docs/ref/ws-agent-runtime.md` - durable named-agent runtime and
  CLI mirror guidance.
- [Must] `ai-docs/ref/ws-mcp.md` - MCP tool reference for `config.agents_tier`
  and `agents.register`.
- [Must] `agents-plugin/runtime.json` - full ws runtime compatibility contract.
- [Must] `agents-plugin-wsflow/runtime.json` - wsflow agentless compatibility
  contract.
- [Must] `ai-docs/spec/plugin-runtime.md` and
  `ai-docs/mental-model/plugin-runtime.md` - runtime metadata coupling and
  launcher compatibility rules.
- [Must] `ai-docs/spec/mcp-tools.md` and
  `ai-docs/spec/named-agent-runtime.md` - implemented effort behavior already
  recorded by the Phase 2 doc pipeline.
- [Must] `ai-docs/mental-model/mcp-runtime.md` and
  `ai-docs/mental-model/named-agent-runtime.md` - operational effort behavior
  already recorded by the Phase 2 doc pipeline.
