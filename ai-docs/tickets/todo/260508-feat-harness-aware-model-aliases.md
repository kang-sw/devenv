---
title: Harness-aware model aliases
parent: 260503-epic-ws-agent-workflow-stability
related-mental-model:
  - named-agent-runtime
  - mcp-runtime
  - prompt-bundle
  - workflow-skills
---

# Harness-aware model aliases

## Background

Named-agent registration currently exposes both `tier` and `model`. That shape
made sense during the Claude-to-Codex migration, but the practical behavior is
now closer to generalized model aliases: `light`, `core`, and `deep` select a
configured backend/model pair, while concrete model names such as GPT or Claude
model names override the alias for a specific registration.

The caller-facing model selection contract should be simplified around one
concept:

- `model: "light" | "core" | "deep"` means a portable model alias.
- `model: "<concrete model name>"` means a one-off concrete backend model.
- `tier` remains a deprecated compatibility input that behaves like an alias
  selector when `model` is absent.

Aliases should also be harness-aware. A `core` delegate launched from a Codex
MCP session should resolve to the configured Codex default, while the same alias
from a Claude MCP session should resolve to the configured Claude default.

## Decisions

- Prefer MCP payload and initialization heuristics over environment variables
  for harness detection. Environment propagation is known to be brittle in
  delegated contexts and should not be the primary host identity mechanism.
- Treat `light`, `core`, and `deep` as portable model aliases, not as workload
  tiers in new public documentation.
- Preserve existing `tier` inputs for compatibility, but route new examples and
  workflow guidance through `model`.
- Concrete model names always win over aliases and harness defaults.
- Documentation examples that use Claude family names should avoid stale
  versions; the user noted Sonnet, Opus, and Haiku examples should account for
  current 4.6-era naming where exact names are shown.

## Phases

### Phase 1: Normalize model selection semantics

Define and implement the registration precedence in one place:

1. Explicit concrete `model` wins and infers backend from the model family.
2. Explicit alias `model` (`light`, `core`, or `deep`) resolves through the
   harness-aware alias map.
3. Legacy `tier` is accepted only when `model` is absent and is converted to the
   matching alias.
4. Prompt frontmatter alias is used only when neither `model` nor `tier` was
   supplied by the caller.
5. Missing selection defaults to the `core` alias.

Keep status and diagnostics clear enough that callers can see the requested
alias, resolved backend, and resolved concrete model. Avoid making `tier:
sonnet` and `model: sonnet` mean different long-term public behaviors.

### Phase 2: Detect MCP harness from request payloads

Add session-level harness detection to the MCP server. Use high-confidence MCP
payload signals before any environment fallback:

- Inspect `initialize.params`, including `clientInfo`, `_meta`, and raw JSON
  text, for Codex or Claude client markers.
- Treat `tools/call.params._meta.x-codex-turn-metadata` as a strong Codex
  signal; this signal already exists in the root fallback path.
- Add a Claude heuristic only from observed Claude MCP initialization or request
  payloads; do not invent an unverified marker.
- Record conflicts as debug events and diagnostics instead of silently changing
  the stored harness.

Expose the detected harness through an inspection surface such as
`runtime.info`, `session.get_default_root`, `agents.status`, or a more focused
status field if that is cleaner.

### Phase 3: Add harness-aware alias configuration

Replace the single tier-to-backend/model mapping with alias mappings that can
branch by harness while preserving existing config files. A compatible shape is
acceptable as long as it can represent:

- `core` under Codex resolves to a Codex backend/model default.
- `core` under Claude resolves to a Claude backend/model default.
- Unknown harness uses a deterministic default, initially Codex unless the user
  config says otherwise.
- User-local config can override alias mappings without changing shared skills
  or embedded prompt files.

Keep the old `config.agents_tier` surface as a compatibility alias or migration
wrapper. Prefer new documentation and any new tool surface to speak in terms of
model aliases rather than workload tiers.

### Phase 4: Update workflow docs and compatibility guidance

Update workflow manual, MCP reference docs, named-agent runtime docs, prompt
frontmatter guidance, and relevant tests so new examples use:

```text
model: "core"
model: "deep"
model: "gpt-5.5"
model: "claude-sonnet-4.6"
```

Use exact provider model names only where they are necessary to test backend
inference or CLI argument passing. Elsewhere, prefer portable aliases or family
aliases so documentation does not drift every time a provider revises model
versions.
