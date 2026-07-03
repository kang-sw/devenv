# Brief: 260625-feat-lead-tune-schema-backed-knob-catalog

## Intent

Add a read-only `config.tuning` MCP tool that lets `ws:lead-tune` discover
current tuning knobs from runtime metadata instead of copying writer schemas in
playbook prose.

## Scope Boundary

Implement Phase 1 only. Add the catalog MCP surface, update `lead-tune` to read
it, update runtime/package manifests and generated rsrc mirrors as required, and
verify full ws plus wsflow/no-agent behavior. Do not add a generic config setter
or per-role tier tuning.

## Caller-Visible Contract

`config.tuning(session_key?, format?)` is lead-only and read-only.

Default output is compact LLM-readable text grouped by tuning knob. `format:
"json"` returns a stable structured catalog. With `session_key`, entries include
current resolved values/scopes where the existing config resolver can provide
them.

Full ws catalog entries:

- `prompt.<pointId>` for every declared prompt override point from the shipped
  rsrc marker scan. These entries describe `harness`/`scope` selection,
  `prompt` text value, `config.prompt.set` writer, and `config.prompt.unset`
  reset.
- `delegation.prefer_mercenary`, backed by `ws.lead.prefer_mercenary`, exposing
  canonical `value` options derived from the writer schema and hiding legacy
  `enabled`.
- `agents.tier`, backed by `config.agents_tier`, exposing `tier` and optional
  `harness` selectors plus `backend`, `model`, and `effort` value fields.

In wsflow/no-agent mode, omit full-ws-only knobs (`delegation.prefer_mercenary`
and `agents.tier`) while retaining prompt override entries.

## Contract Instructions

Reuse existing MCP schema declarations in `agents-plugin-tool/internal/mcp/server.go`
instead of copying enum/property values into a separate handwritten catalog
schema. A small semantic registry may identify knob ids, writer tools,
canonical/suppressed args, and selector/value grouping.

Prompt override entries must reuse the same scanner path as `config.prompt`.
Do not invent `pointId` values.

`lead-tune` should call `config.tuning` on invoke and stop copying enum/tier
field lists. It still owns routing judgments, unsupported-axis handling, and the
confirm-before-write rule.

Update `agents-plugin/runtime.json` for the new full ws tool. Update
`agents-plugin-wsflow/runtime.json` only if wsflow exposes the tool in no-agent
mode; if exposed, its output must omit full-ws-only knobs.

Because plugin-runtime guidance says all non-mercenary/non-exec MCP tools belong
in both full ws and wsflow contracts, expose `config.tuning` in wsflow unless
implementation discovers a contract blocker.

Update canonical rsrc manifest and byte-identical wsflow rsrc mirror after
editing `agents-plugin/rsrc/lead-tune/lead-tune.md`.

## Integration Test Instructions

Add or extend MCP tests around:

- `config.tuning` text output in full ws mode.
- `config.tuning(format: "json")` in full ws mode.
- wsflow/no-agent mode output omitting full-ws-only knobs while retaining prompt
  override entries.
- `delegation.prefer_mercenary` values coming from writer schema projection, not
  lead-tune prose.
- `lead-tune` rsrc text calling `config.tuning` and no longer duplicating
  `prefer_mercenary` or tier enum lists.

Run focused Go tests for MCP and rsrc changes, then run:

```bash
python3 -m unittest discover agents-plugin-wsflow/tests
```

## Implementation Strategy Decisions

- Implement a read-only catalog projection, not a setter.
- Keep existing writer tools as mutation authority.
- Use semantic registration only for "this writer is a lead-tune knob" metadata.
- Derive enums/properties from existing writer schemas where practical.
- Hide compatibility-only writer args from the catalog when they are not
  canonical tuning syntax.

## Rejected Alternatives

- Manual catalog schema: rejected because it moves the same staleness problem.
- Generic `config.set`: rejected because prompt text, delegation enum, and tier
  table updates have different selector/value shapes.
- Raw `tools/list` passthrough: rejected because it lacks semantic grouping and
  exposes compatibility-only call shapes.

## Approach

- Inspect existing `config.prompt`, `config.agents_tier`, and
  `ws.lead.prefer_mercenary` schema/dispatch tests.
- Add shared helpers for schema property projection if needed.
- Add `config.tuning` to `tools()`, `callTool`, visibility gates, and runtime
  manifests.
- Update `lead-tune` playbook to use the catalog and regenerate rsrc artifacts.
- Remove the planned `🚧` marker from the spec after implementation is verified.

## Constraints

- Do not broaden tool authority; `config.tuning` follows the same lead-only
  `config.*` gate.
- Do not expose full-ws-only knobs in wsflow/no-agent mode.
- Do not make session-scope `hide` tool-list behavior part of this ticket; that
  remains covered by `260624-design-session-scope-hide-not-reflected-in-tools-list`.
- Preserve prompt reset semantics: prompt catalog entries may advertise
  `config.prompt.unset`, but this ticket does not change the existing
  required-harness behavior from `260620-bug-ws-prompt-override-no-unset-path`.

## Out of scope

- Generic config mutation.
- New workflow language setter.
- Per-role tier tuning.
- Changing `prefer_mercenary` runtime semantics.
- Changing prompt override marker grammar.

## Details

Expected JSON shape may be compact but should include stable keys equivalent to:

- `knobs[]`
- `id`
- `kind`
- `description`
- `writer.tool`
- `selector_fields[]`
- `value_fields[]`
- `current` when known

Field entries should include `name`, `required` when derivable, `enum` when
present, and `description` when present.

## Verification Contract

Implementation is acceptable when focused tests pass, wsflow package tests pass,
`config.tuning` is visible/callable in full ws, wsflow/no-agent output is
product-mode filtered, `lead-tune` uses `config.tuning`, and spec/ticket docs are
closed out.

## References

- [Must] `ai-docs/spec/mcp-tools.md` - `260625-tuning-catalog` planned contract.
- [Must] `ai-docs/tickets/ready/260625-feat-lead-tune-schema-backed-knob-catalog.md` - selected phase scope and decisions.
- [Must] `ai-docs/mental-model/mcp-runtime.md` - MCP registry, config, product-mode gate, runtime manifest rules.
- [Must] `ai-docs/mental-model/prompt-bundle.md` - rsrc manifest and wsflow rsrc mirror rules.
- [Must] `ai-docs/mental-model/workflow-skills.md` - lead-tune and skill/playbook routing constraints.
- [Must] `ai-docs/mental-model/plugin-runtime.md` - runtime contract and wsflow package alignment rules.
- [Must] `ai-docs/ref/wsflow-mirroring.md` - wsflow product-mode and package test requirements.
- [Must] `ai-docs/tickets/idea/260605-research-ws-native-subagent-pivot.md` - migration anchor for wsflow/mercenary boundary work.
- [Must] `ai-docs/tickets/idea/260620-bug-ws-prompt-override-no-unset-path.md` - prompt unset and required-harness context.
- [Must] `ai-docs/tickets/idea/260624-feat-prefer-mercenary-hide-option.md` - source context for `prefer_mercenary` values.
