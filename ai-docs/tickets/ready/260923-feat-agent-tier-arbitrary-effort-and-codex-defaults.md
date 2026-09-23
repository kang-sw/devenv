---
title: Allow arbitrary agent tier effort and update shipped Codex defaults
related:
  260923-feat-config-tune-agents-tier-reset: sibling editing the same agents.tier code; whichever lands second rebases
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 2230a866feafc62f
sage-review-completeness-reviewed: 2230a866feafc62f
---

# Allow arbitrary agent tier effort and update shipped Codex defaults

## Background

`agents.tier` currently advertises and validates `effort` against a fixed set ending at `xhigh`. That prevents storing a provider's new effort label such as `max` until ws changes its enum. The shipped ws-mcp Codex defaults also differ from the requested GPT-6 tier mapping. The Pi project-local mapping is a separate configuration: it has already been set to the requested GPT-6 models using `xhigh` temporarily for the two tiers that would otherwise use `max`.

## Decisions

- Accept any non-empty effort string in `agents.tier` configuration rather than a ws-owned allowlist. Preserve the existing unset meaning of `""` and `"none"`. Keep the existing trim-and-lowercase normalization of effort labels (for example `Max` is stored as `max`). Store and propagate the supplied value without substituting another effort label; leave model/backend support validation to agent launch.
- Set the shipped Codex tier models and efforts to:

  | Tier | Model | Effort |
  | --- | --- | --- |
  | small | `gpt-6-luna` | `high` |
  | medium | `gpt-6-luna` | `max` |
  | large | `gpt-6-sol` | `high` |
  | xlarge | `gpt-6-sol` | `max` |

- supersedes 260513-feat-agent-tier-effort-config: the fixed portable effort
  vocabulary is replaced by any non-empty label (user-confirmed in 51d9588a).
- supersedes 9bfe7aa3: the xlarge `max` -> `xhigh` substitution is no longer
  needed once `max` is storable.
- Switching the Pi project mapping from `xhigh` to `max` after this lands is a
  manual config.tune step by the user, not part of this ticket.
- Update the shipped Codex defaults where the existing ws-mcp default/Codex alias seeding uses them; preserve the existing harness override and fallback behavior. Do not silently change the Pi-specific mapping or any user-stored aliases as part of the shipped-default change. The temporary Pi project mapping uses `xhigh` in place of `max` until arbitrary effort strings can be configured.

## Constraints

- Convention: `ai-docs/manuals/shipped-surface-boundary.md` (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: `ai-docs/manuals/ws-mcp.md` (declared for `agents-plugin-tool/internal/mcp/`)

## Prior Decisions

- 51d9588a (2026-09-23, commit): "User confirmed unrestricted non-empty agents.tier effort storage with existing empty/none unset behavior and provider support validation at launch." — bearing: supports
- 9bfe7aa3 (2026-07-14, commit): "xlarge \"effort max\" mapped to xhigh (option A): \"max\" is not in the effort vocabulary (normalizeOptionalEffort accepts none/low/medium/high/xhigh ...); user confirmed A rather than extending the enum." — bearing: constrains
- 260513-feat-agent-tier-effort-config (2026-05-13, Decisions): "Support portable effort values that can map cleanly across current runners: `none`, `low`, `medium`, `high`, and `xhigh`." — bearing: constrains
- 260921-feat-config-tune-agents-tier-global-scope (2026-09-21, Decisions): "Read-time merge is a per-(tier, harness) leaf overlay: builtin < global < project. A project entry for a given (tier, harness) overrides the global entry ..." — bearing: constrains
- 726d1dfe (2026-09-21, commit): "Project and global files are read as sparse tier/harness leaves and merged at both independent resolution entry points; applying defaults only after that overlay preserves builtin < global < project precedence." — bearing: constrains
- 260814-refactor-config-collapse-tuning-knobs-to-list-tune (2026-08-14, Decisions): "`agents.tier` is a compound writer, represented faithfully — not flattened." — bearing: constrains
- be1e0125 (2026-06-24, commit): "Effort added to both applyDefaultTiers and defaultModelAliases fallback literals so tierOrDefault picks it up in both code paths." — bearing: constrains
- 0df14498 (2026-05-04, commit): "The final implementation backfills only missing tier entries when loading config, without overwriting user-provided mappings." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsconfig/config.go normalizeOptionalEffort L604-L617 and defaults L479-L529, agents-plugin-tool/internal/mcp/config_registry.go agentsEffortEnum L20 and L179, agents-plugin-tool/cmd/ws-mcp/main.go effort flag help L280 |
| scope.surface | public-interface | config.list catalog effort field Enum and config.tune validation are MCP tool contract; shipped Codex defaults reach downstream installs |
| scope.new_public_symbol | no | none; agentsEffortEnum removal or change only |
| scope.new_type_contract | no | AgentTier.Effort already a string at config.go L63; only the advertised enum is dropped |
| scope.test_surface | existing | agents-plugin-tool/internal/wsconfig/config_test.go, internal/mcp/server_test.go, internal/mcp/playbook_render_surface_test.go, internal/mcp/playbook_tools_test.go, internal/mcp/ticket_review_design_test.go, internal/mcp/prompt_override_test.go, cmd/ws-mcp/main_test.go all assert gpt-5.6 or xhigh |
| complexity.reuse_points | confirmed | normalizeOptionalEffort, applyDefaultTiers, defaultModelAliases with tierOrDefault, LoadAgentTierConfig overlay |
| complexity.side_effect_risk | moderate | shipped Codex defaults change for every downstream project without stored aliases; sibling 260923-feat-config-tune-agents-tier-reset edits the same agents.tier code |
| risk.correctness | moderate | normalizeOptionalEffort lowercases input, and defaults are duplicated in applyDefaultTiers and defaultModelAliases literals that must stay in sync |
| risk.fit | low | existing effort seam and default-literal pattern are reused without new structure |
| risk.test | moderate | many golden assertions pin gpt-5.6 and xhigh across seven test files and TestSetAgentsTierForHarnessRejectsInvalidEffort asserts max is rejected |
| risk.security_or_contract | moderate | removes MCP schema enum from config.list and relaxes config.tune validation, a caller-visible contract change |

## Phases

### Phase 1: Widen effort storage and refresh shipped Codex tiers

Remove the fixed `agents.tier.effort` value restriction from the configuration catalog and write path, while keeping the agreed unset behavior. Carry arbitrary effort values through save, read, resolution, and rendered agent recommendations. Replace the shipped Codex tier defaults with the table above without overriding explicit per-harness/project mappings. Change both default literal sites together (`applyDefaultTiers` and the `default`/`codex` buckets of `defaultModelAliases` in `agents-plugin-tool/internal/wsconfig/config.go`) and the effort list in the `cmd/ws-mcp/main.go` flag help.

Verify that `config.list` exposes a string effort field rather than the old enum; `config.tune` accepts and round-trips `max` and another non-enumerated non-empty effort label, while `""` and `"none"` remain unset. Cover shipped Codex defaults and existing harness override/fallback precedence in configuration tests, plus the model/effort exposed by the MCP resolution and playbook-render paths. Update the existing pinned tests: flip `TestSetAgentsTierForHarnessRejectsInvalidEffort` (it asserts `max` is rejected) and refresh the gpt-5.6/xhigh golden assertions listed in Route Facts; add or keep a test asserting both default paths agree. Do not treat configuration storage as proof that a provider will accept an effort at launch.
