---
title: Pi agent spawn cannot select a concrete model without mutating tier config
---

# Pi agent spawn cannot select a concrete model without mutating tier config

## Background

`ws-agent-spawn` currently exposes `model_name`, but accepts only the configured capability aliases `small`, `medium`, `large`, and `xlarge`. A caller that needs a one-off concrete Pi model must temporarily rewrite the project-scoped `agents.tier` mapping, spawn, and restore it. This is global mutable configuration for a per-dispatch choice and can interfere with concurrent dispatches.

Live dogfood hit this while comparing `openrouter/inception/mercury-2.5` with the configured `openai-codex/gpt-5.6-luna`: passing the concrete model directly was rejected, and the benchmark required a temporary small-tier mutation.

`model_effort` already exists and accepts explicit Pi thinking levels; the missing behavior is concrete model selection and an explicit default-effort value, not an effort field itself.

## Decisions

- `model_name` accepts either a configured tier alias or a concrete Pi model ID. Existing tier inputs remain backward compatible.
- Concrete IDs pass through the same Pi model-catalog and authentication validation as configured tiers and fail before child allocation when unknown or unavailable; no silent fallback to an inherited or tier model.
- `model_effort` accepts `"default"` in addition to its current explicit values. `"default"` means no caller-level effort override: inherited-model dispatch keeps inherited effort, tier dispatch uses the configured tier effort, and concrete-model dispatch uses Pi/the selected model's default effort. An explicit supported effort value overrides those defaults.
- Omitting model and effort preserves the existing parent-model inheritance behavior.

## Phases

### Phase 1: Support per-dispatch concrete model and default effort

Extend `ws-agent-spawn` model resolution and its public tool schema/help so callers can choose a tier alias or concrete Pi catalog ID without mutating shared tier configuration. Add the `model_effort: "default"` semantics above while preserving current explicit effort handling, authentication checks, pre-allocation failure, and inherited-model behavior.

Verify tier, concrete, omitted, unknown, unauthenticated, explicit-effort, and default-effort paths. Include a concurrency regression proving a concrete one-off dispatch does not alter the model selected by simultaneous tier-based dispatches.
