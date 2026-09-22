---
title: Allow arbitrary agent tier effort and update shipped Codex defaults
---

# Allow arbitrary agent tier effort and update shipped Codex defaults

## Background

`agents.tier` currently advertises and validates `effort` against a fixed set ending at `xhigh`. That prevents storing a provider's new effort label such as `max` until ws changes its enum. The shipped ws-mcp Codex defaults also differ from the requested GPT-6 tier mapping. The Pi project-local mapping is a separate configuration: it has already been set to the requested GPT-6 models using `xhigh` temporarily for the two tiers that would otherwise use `max`.

## Decisions

- Accept any non-empty effort string in `agents.tier` configuration rather than a ws-owned allowlist. Preserve the existing unset meaning of `""` and `"none"`. Store and propagate the supplied value without substituting another effort label; leave model/backend support validation to agent launch.
- Set the shipped Codex tier models and efforts to:

  | Tier | Model | Effort |
  | --- | --- | --- |
  | small | `gpt-6-luna` | `high` |
  | medium | `gpt-6-luna` | `max` |
  | large | `gpt-6-sol` | `high` |
  | xlarge | `gpt-6-sol` | `max` |

- Update the shipped Codex defaults where the existing ws-mcp default/Codex alias seeding uses them; preserve the existing harness override and fallback behavior. Do not silently change the Pi-specific mapping or any user-stored aliases as part of the shipped-default change. The temporary Pi project mapping uses `xhigh` in place of `max` until arbitrary effort strings can be configured.

## Phases

### Phase 1: Widen effort storage and refresh shipped Codex tiers

Remove the fixed `agents.tier.effort` value restriction from the configuration catalog and write path, while keeping the agreed unset behavior. Carry arbitrary effort values through save, read, resolution, and rendered agent recommendations. Replace the shipped Codex tier defaults with the table above without overriding explicit per-harness/project mappings.

Verify that `config.list` exposes a string effort field rather than the old enum; `config.tune` accepts and round-trips `max` and another non-enumerated non-empty effort label, while `""` and `"none"` remain unset. Cover shipped Codex defaults and existing harness override/fallback precedence in configuration tests, plus the model/effort exposed by the MCP resolution and playbook-render paths. Do not treat configuration storage as proof that a provider will accept an effort at launch.
