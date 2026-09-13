---
title: Pi agent spawn cannot select a concrete model without mutating tier config
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: ae63aa4a07f2428b
sage-review-completeness-reviewed: ae63aa4a07f2428b
completed: 2026-09-13
---

# Pi agent spawn cannot select a concrete model without mutating tier config

## Background

`ws-agent-spawn` currently exposes `model_name`, but accepts only the configured capability aliases `small`, `medium`, `large`, and `xlarge`. A caller that needs a one-off concrete Pi model must temporarily rewrite the project-scoped `agents.tier` mapping, spawn, and restore it. This is global mutable configuration for a per-dispatch choice and can interfere with concurrent dispatches.

Live dogfood hit this while comparing `openrouter/inception/mercury-2.5` with the configured `openai-codex/gpt-5.6-luna`: the concrete ID made its fixed-tier `config.resolve_agent` lookup fail, and the spawner then inherited the parent model (agents-plugin-tool/internal/wsconfig/config.go#L258-L270; agents-plugin-pi/src/spawner.ts#L441-L454), so the benchmark required a temporary small-tier mutation.

`model_effort` already exists and accepts explicit Pi thinking levels; the missing behavior is concrete model selection and an explicit default-effort value, not an effort field itself.

## Decisions

- `model_name` accepts either a configured tier alias or a concrete Pi model ID. Existing tier inputs remain backward compatible.
- Concrete IDs pass through the same Pi model-catalog and authentication validation as configured tiers and fail before child allocation when unknown or unavailable; no silent fallback to an inherited or tier model.
- `model_effort` accepts `"default"` in addition to its current explicit values. `"default"` means no caller-level effort override: inherited-model dispatch keeps inherited effort, tier dispatch uses the configured tier effort, and concrete-model dispatch uses Pi/the selected model's default effort. An explicit supported effort value overrides those defaults.
- Omitting model and effort preserves the existing parent-model inheritance behavior.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/spawner.ts, agents-plugin-pi/src/model-catalog.ts, agents-plugin-pi/test/spawner.test.ts |
| scope.surface | public-interface | ws-agent-spawn model_name and model_effort tool-schema behavior at agents-plugin-pi/src/spawner.ts#L3612-L3634 |
| scope.new_public_symbol | no | extends existing ws-agent-spawn parameters and resolver |
| scope.new_type_contract | yes | model_name accepts a tier alias or concrete Pi model ID; model_effort gains default semantics |
| scope.test_surface | existing | agents-plugin-pi/test/spawner.test.ts has resolver, spawn-guard, effort, and --thinking coverage |
| complexity.reuse_points | confirmed | resolveModelForAliasViaWsMcp and modelCatalogFromToolCtx implement existing tier resolution, catalog membership, and auth checks |
| complexity.side_effect_risk | moderate | model selection occurs before persistent child allocation and RPC startup |
| risk.correctness | high | a concrete ID must not silently resolve to an unintended inherited or tier model |
| risk.fit | moderate | the existing fixed-tier resolver, tool schema, and Pi lead guidance all state the tier-only contract |
| risk.test | moderate | existing unit seams cover resolution and spawn guards; the requested concurrent-dispatch regression needs new coverage |
| risk.security_or_contract | high | public model selection must preserve catalog and configured-auth validation before child allocation |

## Phases

### Phase 1: Support per-dispatch concrete model and default effort

Extend `ws-agent-spawn` model resolution and its public tool schema/help so callers can choose a tier alias or concrete Pi catalog ID without mutating shared tier configuration. Add the `model_effort: "default"` semantics above while preserving current explicit effort handling, authentication checks, pre-allocation failure, and inherited-model behavior.

Verify tier, concrete, omitted, unknown, unauthenticated, explicit-effort, and default-effort paths. Include a concurrency regression proving a concrete one-off dispatch does not alter the model selected by simultaneous tier-based dispatches.

### Result (41dcbb31) - 2026-09-13

`ws-agent-spawn` now distinguishes configured tier aliases from concrete catalog `provider/id` selections, validates both through one live catalog/auth primitive, and rejects invalid concrete selections before allocating an owned child home. Concrete selection bypasses `config.resolve_agent`, so simultaneous tier dispatch remains isolated from the one-off choice.

`model_effort: "default"` now preserves source-specific behavior: inherited dispatch keeps the captured parent effort, tier dispatch keeps configured effort, and concrete dispatch leaves effort unset for the selected model's default. Omitted effort retains its previous behavior. The tool schema and injected Pi lead guidance now defer to the authoritative tier-or-concrete contract.

Verification: `node --test test/spawner.test.ts` passed 329/329; `npm test -- --test-reporter=dot --test-concurrency=1` passed the full suite; `git diff --check` passed. The default parallel full-suite run had one unrelated launcher-error assertion observe `write EPIPE`; its isolated test passed 5/5, and the serial full suite passed.

Round-one Important findings were fixed and round-two correctness, fit, and test verification was clean. The fit review's Minor terminology note remains recorded: internal `TierResolution`/`tierResolution` names now cover concrete and inherited selection as well as tiers; renaming was not expanded into this behavior fix.


## Resolution (2026-09-13)

Implemented per-dispatch concrete Pi model selection with live catalog/auth validation, source-aware `model_effort: "default"` behavior, schema and lead-guide alignment, concurrency coverage, and clean two-round partitioned review.
