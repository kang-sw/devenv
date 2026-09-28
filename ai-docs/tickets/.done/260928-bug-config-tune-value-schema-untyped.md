---
title: config.tune value schema is untyped, so object values arrive as strings
sage-review-design: completed
sage-review-completeness: completed
sage-review-completeness-reviewed: ee53cf284a62f4a7
sage-review-design-reviewed: ee53cf284a62f4a7
completed: 2026-09-28
---

# config.tune value schema is untyped, so object values arrive as strings

## Background

`config.tune` publishes its polymorphic `value` argument through
`anyProperty` (`agents-plugin-tool/internal/mcp/server.go`, search
`anyProperty(`), which emits only a `description` and no JSON-Schema `type`.
The collapse commit `104831dfc` chose this on the premise that an absent
`type` means "any"; it did not consider how model servers emit tool calls for
type-less parameters.

A downstream Pi user serving `Qwen/Qwen3.8-27B-FP8` on vLLM 0.28.0 could not
write `agents.tier`: every call failed with `config.tune: agents.tier value
must be an object with tier/backend/model/effort fields`, because the object
reached ws-mcp as a JSON-encoded string and `tuneAgentsTier` accepts only a
decoded `map[string]any`. The Pi bridge passes MCP input schemas and
arguments through unchanged; the stringification happens in the model
server's tool-call parser, which falls back to string for a parameter with no
declared `type`.

Evidence (lead, 2026-09-28, direct `/v1/chat/completions` probes against that
vLLM endpoint with a `config_tune`-shaped tool, identical prompts):

| `value` schema | object request | scalar request (`off`) |
|---|---|---|
| no `type` (current) | string `"{\"tier\": \"small\", ...}"` | string |
| `anyOf: [{type: string}, {type: object}]` | object | string |
| `type: ["string", "object"]` | object | string |
| `anyOf: [{type: string}, {type: object, properties: {...}}]` | object | string |
| `type: object` only | object | model refused to call |

Every value shape `config.tune` reads today is either a string or an object:
`agents.tier` write/reset reads an object; `prompt.*` reads prompt text;
the scalar knobs (`workflow.prefer_subagent`, `bootstrap_alarm`,
`sage_review`) read enum strings. No key takes a number, boolean, or array.

The inverse defect lives in the same argument: the `prompt.*` and scalar
branches read `value` with `value.(string)` and discard the ok flag, so a
non-string value silently becomes `""` (prompt then reports "must be
non-empty"; a scalar knob reports an enum mismatch on `""`; a non-agents.tier
reset silently accepts a non-string value).

## Decisions

- Publish `value` as `anyOf: [{"type": "string"}, {"type": "object"}]`,
  keeping the existing description on the outer `value` property beside
  `anyOf` (branches stay bare). The object branch declares no `properties`,
  `required`, or `additionalProperties`: field shape stays in the description
  and in the server's existing `tuneAgentsTier`/wsconfig validation. Build it
  with a small named helper in the style of the existing
  `enumStringOrArrayProperty` (outer description, bare `anyOf` branches,
  comment explaining why the type is spelled out), replacing `anyProperty`.
- Rejected: parsing a JSON-string `value` into an object on the server. A
  legitimate string value could be misread, and the schema would again
  disagree with what the server accepts.
- Rejected: a typed object branch with per-field properties/enums. The probe
  showed the outer type alone fixes the delivery; per-field schema would
  duplicate wsconfig validation (tier synonyms, harness-specific models).
- Rejected: `type: ["string", "object"]`. Equivalent in the probe, but some
  provider schema converters accept `anyOf` and not type arrays; `anyOf` is
  the broader-compatibility form.
- For every key other than `agents.tier`, a present `value` that is not a
  string is rejected with an explicit error naming the key and the received
  JSON type, instead of being coerced to `""`. On write branches the error
  says a string is required; on the reset branch it uses the existing
  "value and reset are mutually exclusive" framing, since a string is not
  accepted there either. An explicit JSON `null`
  `value` counts as absent, not as a non-string, matching the existing
  `typedArrayRejection` treatment in the same file; this keeps a
  non-agents.tier reset that sends `null` instead of omitting `value` working,
  and a write with `null` falls through to the existing empty-value errors.

## Constraints

- Read `ai-docs/manuals/shipped-surface-boundary.md` (new agent-facing error
  text) and `ai-docs/manuals/ws-mcp.md` before editing
  `agents-plugin-tool/internal/mcp/`.
- `anyProperty` has no other caller; remove or replace it rather than leaving
  an unused helper with the misleading "absent type means any" rationale.
- Do not change accepted value semantics beyond the non-string rejection:
  agents.tier object validation, reset rules, enum lowercasing, and scope
  handling stay as they are.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)

## Prior Decisions

- 260923-feat-config-tune-agents-tier-reset (2026-09-23, Decisions): "Reset call shape: `config.tune(key: \"agents.tier\", reset: true, value: {tier}, harness, scope)`. `tier` stays inside the `value` object as for writes; with `reset: true` the value object carries only `tier`" — bearing: constrains
- ee431485 (2026-09-23, commit): "Keep compound agents.tier reset's value {tier} shape distinct from scalar resets; wsconfig validates legacy tier synonyms and deletes only the selected persisted harness leaf." — bearing: constrains
- 260905-feat-ws-pi-harness-config-layer (2026-09-06, Result): "`config.tune` lowercases the harness before the enum check, normalizes an empty harness to `default` only when the key's enum contains `default`" — bearing: constrains
- 104831df (2026-08-23, commit): "agents.tier follows Decision 9: tier/backend/model/effort travel inside the value object; harness stays the outer selector. Non-project scope rejected." — bearing: supports
- 104831df (2026-08-23, commit): "Version strings deliberately untouched; the merge-into-develop bump owns them." — bearing: constrains
- 260814-refactor-config-collapse-tuning-knobs-to-list-tune (2026-08-14, Decisions): "`agents.tier` is a compound writer, represented faithfully — not flattened. Unlike the scalar knobs (alarm booleans, `prefer_*` on/off/hide, prompt text), `agents.tier`'s value is multi-field" — bearing: supports
- 260814-refactor-config-collapse-tuning-knobs-to-list-tune (2026-08-14, Decisions): "`config.list` returns as much as is applicable. ... list returns the full per-key schema (value domain / legal-value hint, required-vs-optional fields, allowed and default scopes, harness applicability)" — bearing: supports
- 260625-feat-lead-tune-schema-backed-knob-catalog (2026-06-25, commit 4107c4f1): "The design keeps existing writer tools as mutation authority and adds config.tuning as a read-only projection rather than a hand-maintained schema copy." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/mcp/server.go plus its tests in agents-plugin-tool/internal/mcp/server_test.go, agents_tier_reset_test.go, prompt_override_test.go |
| scope.surface | public-interface | config.tune inputSchema published through tools/list at agents-plugin-tool/internal/mcp/server.go#L3803-L3817 |
| scope.new_public_symbol | no | none; anyProperty at server.go#L5007-L5016 is unexported and removed or replaced |
| scope.new_type_contract | yes | config.tune value property changes from untyped to anyOf string or object |
| scope.test_surface | existing | toolPropertiesByName in agents-plugin-tool/internal/mcp/server_test.go#L67-L87; config.tune call tests in agents_tier_reset_test.go, prompt_override_test.go, workflow_prefer_subagent_test.go, bootstrap_alarm_test.go |
| complexity.reuse_points | confirmed | tuneAgentsTier server.go#L2377-L2381 unchanged; jsonValueTypeName and typedArrayRejection pattern for type-accurate rejection in server.go |
| complexity.side_effect_risk | moderate | the tools/list schema ships to every host adapter and model provider schema converter, and non-string values previously coerced to empty string now error |
| risk.correctness | moderate | reset branch server.go#L908-L912 and write branches server.go#L945-L989 must reject non-string values while keeping absent and string values unchanged |
| risk.fit | low | follows existing stringProperty and objectProperty helper style and the typedArrayRejection error pattern in the same file |
| risk.test | low | schema helper and per-key config.tune server tests already exist to extend |
| risk.security_or_contract | moderate | changes a published MCP tool input schema and narrows accepted inputs; agents-plugin/runtime.json declares a config.tune version range |

## Phases

### Phase 1: Type config.tune value as string or object

Change the `config.tune` input schema's `value` to the `anyOf` form above and
add the explicit non-string rejection for non-`agents.tier` keys (write
branches and the non-agents.tier reset branch).

Verification (required):
- A schema test asserting the published `config.tune` `value` property is
  exactly `anyOf` of string and object with the description preserved.
- Server-path tests: agents.tier write and reset with an object succeed as
  before; a scalar knob and a `prompt.*` key with an object `value` return
  the new explicit error; a non-agents.tier reset with a non-string `value`
  is rejected; existing string paths are unchanged.
- A non-agents.tier reset with `value: null` succeeds like an omitted
  value; a scalar or `prompt.*` write with `value: null` returns the existing
  empty-value error, not the new non-string error.
- Any existing schema snapshot or tool-inventory test updated to the new
  shape; the `toolPropertiesByName` test helper may need to inspect `anyOf`.
- Version strings and the `config.tune` version ranges in
  `agents-plugin/runtime.json` / `agents-plugin-wsflow/runtime.json` stay
  untouched; the merge-into-develop bump owns them (104831df).

Verification (optional, when a vLLM endpoint with a Qwen tool-call parser is
reachable): repeat the object/scalar probe from Background against the built
schema and record the result.

### Result (d67dce5d0) - 2026-09-28

- `config.tune` publishes `value` as `anyOf: [{type: string}, {type: object}]`
  through the new `stringOrObjectProperty` helper (outer description kept,
  bare branches); `anyProperty` is removed.
- Keys other than `agents.tier` reject a present non-null non-string `value`
  via `tuneStringValueRejection`: write branches report
  `config.tune: <key> value must be a string; got <type>`; the reset branch
  reports `config.tune: value and reset are mutually exclusive; <key> reset
  takes no value, got <type>`. Explicit `null` is treated as absent on both.
- `jsonValueTypeName` now names `array` and `null` so the new errors never
  print a Go type name.
- Tests: `agents-plugin-tool/internal/mcp/config_tune_value_type_test.go`
  pins the exact `anyOf` schema and description, and covers object/boolean/
  array/number rejection on scalar and `prompt.*` write and reset, `null`
  write falling to existing empty-value errors, `null` reset succeeding,
  unchanged string paths (including enum lowercasing), and `agents.tier`
  object write/reset.
- Verification: `go vet ./internal/mcp` clean; `go test ./...` in
  `agents-plugin-tool` passes; `scripts/smoke-ws-mcp.sh ..` passes. Single
  independent review: clean. The optional vLLM re-probe was not run (no
  endpoint available to this worker).
- Version strings and `runtime.json` `config.tune` ranges untouched.
