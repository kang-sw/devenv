---
title: Schema-backed tuning knob catalog for lead-tune
related:
  260624-feat-prefer-mercenary-hide-option: prefer_mercenary gained a hide mode that lead-tune prose can miss
  260624-design-session-scope-hide-not-reflected-in-tools-list: adjacent visibility/schema tension for hidden mercenary tools
related-mental-model:
  - mcp-runtime
  - prompt-bundle
  - workflow-skills
---

# Schema-backed tuning knob catalog for lead-tune

## Background

Dogfooding `ws:lead-tune` exposed a stale-guidance risk: prompt overrides are
runtime-discoverable through `config.prompt`, but the non-prompt knobs are
duplicated in prose. The live `ws.lead.prefer_mercenary` schema has
`on`/`off`/`hide`, while the lead-tune playbook describes the older toggle shape.
`config.agents_tier` also carries enum and optional-field details only in the Go
tool schema, so the skill has to repeat them.

The desired shape is a lead-tune flow where the prose owns routing and safety
rules, while the runtime owns the current knob catalog, enum values, default
scope, and accepted fields.

## Prior Art

- `config.prompt` already scans shipped override markers and returns the current
  prompt override points plus stored overrides.
- `config.show` already reports resolved config values and scopes.
- MCP `tools()` already owns the input schemas for `ws.lead.prefer_mercenary`,
  `config.agents_tier`, and `config.prompt.*`.

## Phases

### Phase 1: Expose a schema-backed tuning catalog

Add or extend a read-only MCP surface that returns the lead-tune knob catalog
from runtime metadata instead of requiring the playbook to copy schema details.

Candidate shapes:
- Extend `config.prompt` into a broader `config.tuning` listing that includes
  prompt override points, `prefer_mercenary`, and `config.agents_tier`.
- Or add a small schema lookup helper for selected lead-tune tool names, then
  have `lead-tune` call it before presenting options.

Constraints:
- The playbook should still own judgment/routing text and the "confirm before
  write" rule.
- The catalog must not advertise wsflow-hidden full-ws knobs in agentless mode.
- Avoid copying full raw JSON Schema into normal user-facing output; provide a
  compact LLM-readable summary with JSON available when needed.

Verification:
- Changing the enum values for `ws.lead.prefer_mercenary` or
  `config.agents_tier` should update the lead-tune listing without editing the
  playbook prose.
- Tests should cover full ws and wsflow/no-agent product modes.
