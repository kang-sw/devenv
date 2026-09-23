---
title: config.tune agents.tier reset and shadowed-write warning
related:
  260923-feat-agent-tier-arbitrary-effort-and-codex-defaults: sibling editing the same agents.tier code; whichever lands second rebases
  260921-feat-config-tune-agents-tier-global-scope: builds on its leaf-overlay precedence
  260814-refactor-config-collapse-tuning-knobs-to-list-tune: agents.tier compound-writer shape
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: ff82bbc4f5c10de5
sage-review-completeness-reviewed: ff82bbc4f5c10de5
---

# config.tune agents.tier reset and shadowed-write warning

## Background

Dogfood surprise while tuning global Claude tier mappings through
`ws:lead-tune`:

- `config.tune(key: "agents.tier", scope: "global", harness: "claude", ...)`
  succeeded, but the devenv project config
  (`~/.cache/ws@<project>/config.json`) already carried explicit
  `model_aliases.<tier>.claude` entries, so the project scope shadowed the new
  global values. `config.resolve_agent` still returned the old mapping, and the
  write result gave no hint of the shadowing.
- `agents.tier` exposes no `reset` in the `config.list` catalog (unlike
  `prompt.*`, `workflow.prefer_subagent`, `sage_review`), so there is no
  catalog path to drop a project-scope tier alias back to the inherited global
  value. The workaround was hand-editing the project config JSON, which the
  `lead-tune` playbook otherwise steers away from.

## Decisions

- Reset call shape: `config.tune(key: "agents.tier", reset: true,
  value: {tier}, harness, scope)`. `tier` stays inside the `value` object as
  for writes; with `reset: true` the value object carries only `tier`
  (`backend`/`model`/`effort` are rejected). Rejected: promoting `tier` to a
  selector field, which would split the compound writer's shape.
- Reset removes the stored `(tier, harness)` alias leaf from the selected
  scope's file only, so resolution falls through to the next scope.
- Shadow warning is limited to `agents.tier`: after a write or reset at global
  scope, warn when the project scope stores a leaf for the same
  `(tier, harness)`, naming the shadowing scope. Harness fallback
  (`default`/`codex` buckets) does not count as shadowing. Rejected for now:
  generalizing the warning to every layered knob and to session/repo scopes.
- The shadow warning is a `warnings` string-array field in the agents.tier
  JSON response; a reset of an absent leaf succeeds as a no-op and says so in
  the same field.
- `lead-tune`'s model-tier handler gains reset guidance (offer reset when the
  user wants a scope to inherit again, state that reset takes only
  `value.tier`, distinguish it from writing effort `""`/`"none"` which clears
  effort only, and relay any `warnings`), mirrored to wsflow. The `agents.tier`
  catalog description also states that reset takes only `value.tier`.

## Constraints

- Convention: `ai-docs/manuals/skill-authoring.md` and `ai-docs/manuals/wsflow-mirroring.md` (declared for `agents-plugin/rsrc/`; the `lead-tune` edit touches `agents-plugin/rsrc/lead-tune/lead-tune.md` and its wsflow mirror)
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)

## Prior Decisions

- 260921-feat-config-tune-agents-tier-global-scope (2026-09-21, Decisions): "Read-time merge is a per-(tier, harness) leaf overlay: builtin < global < project. A project entry for a given (tier, harness) overrides the global entry for the same (tier, harness)..." — bearing: supports
- 260921-feat-config-tune-agents-tier-global-scope (2026-09-21, Decisions): "Keep the compound (non-resolver) writer; add a scope axis to it. Do not fold agents.tier into wsconfig.Resolver. The structured ModelAliases shape is incompatible with the resolver's flat overlay" — bearing: constrains
- 260921-feat-config-tune-agents-tier-global-scope (2026-09-21, Decisions): "Scopes: project and global only. session is not meaningful for agents.tier ... repo remains hand-edited/read-only as for every other key." — bearing: constrains
- 726d1dfe (2026-09-21, commit): "The project writer now persists only its changed alias leaf, so a later global override can supply untouched harnesses." — bearing: supports
- 260814-refactor-config-collapse-tuning-knobs-to-list-tune (2026-08-23, Result d722e864): "Phase 2 builds config.list/config.tune on the registry's GlobalOnly()/DefaultScope() methods ...; the agents.tier adapter (ResolverBacked:false) must stay a compound writer, not be flattened." — bearing: constrains
- 104831df (2026-08-23, commit): "The tuning catalog's Writer.Tool is now uniformly config.tune with a fixed key argument per knob (Reset adds reset:true)." — bearing: supports
- 260814-refactor-config-collapse-tuning-knobs-to-list-tune (2026-08-14, Decisions): "harness is load-bearing for the keys that use it; a warning only where it genuinely does not apply. ... agents.tier (harness selects which per-harness model-alias slot is written)" — bearing: constrains
- 260906-feat-ws-config-tune-agents-tier-returns-only-the-written-harness (2026-09-06, commit 9769d142, dropped): "the agents.tier branch of the config.tune handler passes the full wsconfig.Config ... to toolJSONResponse, while every other key answers with a one-line key/value/scope summary." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/mcp/config_registry.go#L143-L185, agents-plugin-tool/internal/mcp/server.go#L869-L983, agents-plugin-tool/internal/mcp/server.go#L2280-L2290, agents-plugin-tool/internal/wsconfig/config.go#L249-L343 |
| scope.surface | public-interface | MCP config.tune reset branch and config.list tuning catalog Reset writer for agents.tier; warning text on config.tune write responses |
| scope.new_public_symbol | yes | an exported wsconfig agents.tier leaf unsetter is needed beside SetAgentsTierForHarness and SetGlobalAgentsTierForHarness since server.go calls wsconfig directly |
| scope.new_type_contract | yes | agents.tier config.tune responses (full JSON Config today) gain a `warnings` field for project-scope shadowing; other knobs are unchanged |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/server_test.go agents.tier config.tune cases, agents-plugin-tool/internal/wsconfig/config_test.go, agents-plugin-tool/internal/mcp/prompt_override_test.go |
| complexity.reuse_points | confirmed | tuningWriter Reset field server.go#L2164 and the Reset wiring pattern at server.go#L2223-L2270; Resolver.Unset resolver.go#L198; loadProjectConfig, loadGlobalConfig, save, saveGlobal in wsconfig |
| complexity.side_effect_risk | moderate | reset rewrites user project or global config files; the warning touches only the agents.tier write and reset branch |
| risk.correctness | moderate | shadow detection follows the agents.tier leaf overlay builtin < global < project only; removing a tier's last alias leaf can change legacy `tiers` seeding in effectiveAgentConfig (config.go#L104-L136) |
| risk.fit | moderate | reset currently rejects non-string value and agents.tier carries tier inside the value object, so the reset argument shape must fit the existing mutual-exclusion check at server.go#L875-L879 |
| risk.test | moderate | needs new cases for reset per scope and harness, fall-through resolution, empty-map cleanup and legacy `tiers` interaction, and agents.tier warning presence or absence |
| risk.security_or_contract | moderate | changes the observable config.tune and config.list contract consumed by the shipped lead-tune playbook |

## Phases

### Phase 1: agents.tier reset and shadow warning

Add a reset path to the `agents.tier` writer per Decisions: accept
`reset: true` with a `value` object carrying only `tier`, plus `harness` and
`scope`, and delete that alias leaf from the selected scope's config file
(no-op reported in `warnings` when the leaf is absent). When the removal
empties a tier's alias map, delete the map instead of persisting `{}`. Advertise the reset in
the `config.list` catalog entry for `agents.tier`. After a global-scope write
or reset, include a warning naming the project scope when it stores a leaf for
the same `(tier, harness)`. Update the model-tier handler in
`agents-plugin/rsrc/lead-tune/lead-tune.md` to offer reset and relay the
warning, and mirror it to wsflow.

Verify: `config.list` shows a reset writer for `agents.tier`; a project-scope
reset of one `(tier, harness)` leaf makes `config.resolve_agent` return the
global value while other leaves stay untouched; reset with `backend`/`model`/
`effort` in `value` is rejected; a global write shadowed by a project leaf
returns the warning and an unshadowed write does not; a leaf reached only
through harness fallback produces no warning; removing a project's last
alias leaf for a tier while it also stores a legacy `tiers` entry resolves as
expected and leaves no empty map. For the playbook edit, the wsflow skill-shim
and mirror drift tests pass and a `playbook.render` of `lead-tune` shows the
reset guidance.
