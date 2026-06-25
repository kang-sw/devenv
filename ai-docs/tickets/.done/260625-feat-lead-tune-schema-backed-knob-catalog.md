---
title: Schema-backed tuning knob catalog for lead-tune
sage-review: completed
related:
  260624-feat-prefer-mercenary-hide-option: prefer_mercenary gained a hide mode that lead-tune prose can miss
  260624-design-session-scope-hide-not-reflected-in-tools-list: adjacent visibility/schema tension for hidden mercenary tools
spec:
  - 260625-tuning-catalog
related-mental-model:
  - mcp-runtime
  - prompt-bundle
  - workflow-skills
completed: 2026-06-25
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

## Decisions

- Add a new read-only `config.tuning` MCP tool rather than widening
  `config.prompt`. `config.prompt` remains the prompt override scanner; the new
  tool is a tuning catalog that can include prompt and non-prompt knobs.
- Keep existing writer tools as the mutation authority. `config.tuning` never
  writes config and never replaces `config.prompt.set`,
  `ws.lead.prefer_mercenary`, or `config.agents_tier`.
- Use a small semantic registry to decide which writer tools are visible as
  lead-tune knobs. The registry may name a canonical argument set and hide legacy
  arguments, but it must not duplicate enum/property schema that already belongs
  to the writer tool.
- Treat catalog output as a projection for `lead-tune`: default text is compact
  and LLM-readable; JSON is available for stable callers.
- Honor product mode. Full-ws-only agent-backed knobs such as
  `prefer_mercenary` and `config.agents_tier` are omitted from wsflow/no-agent
  catalog output.

## Rejected Alternatives

- **Manual catalog schema:** rejected because it recreates the staleness problem
  in a new file.
- **Generic `config.set`:** rejected for this slice because prompt text,
  delegation enum, and tier-table updates have different selector/value shapes
  and already have working writer tools.
- **Expose raw `tools/list` schema directly:** rejected because it leaks
  compatibility-only arguments such as legacy `enabled` and gives `lead-tune`
  too little semantic grouping.

## Phases

### Phase 1: Expose a schema-backed tuning catalog

Add `config.tuning` as a lead-only read-only MCP tool that returns the
lead-tune knob catalog.

Required catalog entries in full ws mode:

- `prompt.<pointId>` entries for every prompt override point returned by the
  existing override-marker scan. Each entry exposes `harness` and `scope` as
  selector/storage fields, `prompt` as the text value, and
  `config.prompt.set`/`config.prompt.unset` as writer/reset tools.
- `delegation.prefer_mercenary`, backed by `ws.lead.prefer_mercenary`. It
  exposes canonical `value` options derived from the writer schema
  (`on|off|hide`) and hides the legacy `enabled` argument from the catalog.
- `agents.tier`, backed by `config.agents_tier`. It exposes `tier` and
  optional `harness` as selectors, plus `backend`, `model`, and `effort` as
  value fields, with enum values derived from the writer schema.

Required behavior:

- Default output is concise text grouped by knob; `format: "json"` returns a
  stable structured catalog.
- `session_key` is optional but, when supplied, current values and scopes are
  resolved the same way `config.prompt`/`config.show` do.
- wsflow/no-agent mode omits full-ws-only knobs while retaining prompt override
  entries.
- `lead-tune` starts by calling `config.tuning` and removes duplicated enum and
  field lists from prose. The playbook continues to own request routing,
  confirmation-before-write, unsupported-axis handling, and the Tuning Proposal
  template.

Verification:

- MCP tests cover text and JSON output for full ws mode.
- MCP tests cover wsflow/no-agent product mode omitting `prefer_mercenary` and
  `agents.tier`.
- A test proves `prefer_mercenary` catalog values are derived from the writer
  schema by observing `on`, `off`, and `hide` in `config.tuning` without a
  hardcoded playbook prose dependency.
- Existing prompt override tests still prove override point discovery is
  marker-driven.
- Lead-tune rsrc tests or snapshots confirm the playbook calls
  `config.tuning` and no longer copies the `prefer_mercenary` or tier enum
  lists.

### Result (f171bd2) - 2026-06-25

Implemented `config.tuning` as a read-only MCP catalog backed by existing writer
tool schemas and live prompt override-point discovery. Full ws catalog output
includes prompt override knobs, `delegation.prefer_mercenary`, and `agents.tier`;
wsflow/no-agent catalog output keeps prompt knobs and omits full-ws-only knobs.

Updated `lead-tune` to load `config.tuning` first and use catalog-provided
writer/field metadata for proposals instead of duplicating enum lists in prose.
Regenerated the canonical rsrc manifest and byte-identical wsflow rsrc mirror.

Verification:

- `go test ./internal/mcp ./internal/wsrsrc ./cmd/ws-mcp -count=1`
- `python3 -m unittest discover ../agents-plugin-wsflow/tests`
- `git diff --check`


## Resolution (2026-06-25)

Implemented schema-backed config.tuning catalog and updated lead-tune to consume catalog metadata. Verification passed with focused Go MCP/wsrsrc/ws-mcp tests and wsflow package tests.
