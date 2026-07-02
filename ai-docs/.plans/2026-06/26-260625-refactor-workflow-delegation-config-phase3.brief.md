# Brief: 260625-refactor-workflow-delegation-config Phase 3

## Intent

Finish the workflow delegation config refactor by aligning `lead-tune`, the
tuning catalog, surrounding tests, wsflow rendering expectations, and closeout
documentation with the Phase 1-2 implementation.

## Scope Boundary

Implement Phase 3 only: lead-tune, catalog, docs, tests, and wsflow polish.

In scope:

- Update `lead-tune` so delegation posture tuning routes to
  `"workflow.prefer_subagent"` through `config.workflow_prefer_subagent`.
- Update mercenary tuning guidance to use `"workflow.prefer_mercenary"` through
  `config.workflow_prefer_mercenary`.
- Ensure `config.tuning` surfaces the workflow knobs from writer schemas and no
  longer advertises the removed `prompt.DelegationSection` marker.
- Add or tighten tests for full ws, wsflow/no-agent catalog behavior, prompt
  override catalog behavior, render output, and removed tool visibility.
- Regenerate `agents-plugin/rsrc/manifest.json` and the byte-identical
  `agents-plugin-wsflow/rsrc/` mirror after rsrc edits.
- Update MCP, workflow-skill, prompt-bundle, and runtime mental-model docs plus
  spec closeout text when implementation details require it.
- Run fresh-reader audit for changed skill/playbook text and classify findings
  before editing audit suggestions.

Out of scope:

- Do not reintroduce `DelegationSection` or migrate orphan
  `prompt.DelegationSection.*` values.
- Do not add duplicate insertion detection for the Phase 2
  `lead-prefer-subagent` append.
- Do not implement project- or session-scoped `"workflow.prefer_subagent"`.
- Do not restore `ws.lead.prefer_mercenary`, keep an alias, or read the old
  unprefixed `"prefer_mercenary"` key.
- Do not add a new playbook include/template syntax.

## Caller-Visible Contract

`ws:lead-tune`

- Loads `config.tuning` first and treats that catalog as the source of supported
  knobs, writer tools, field options, and current values.
- Directs strict subagent posture requests to
  `config.workflow_prefer_subagent(value: "on"|"off")`.
- Directs mercenary preference requests to
  `config.workflow_prefer_mercenary(value: "on"|"off"|"hide")`.
- Keeps prompt overrides for freeform user preferences and shipped override
  markers only; it must not tell users to edit `prompt.DelegationSection.*`.
- Mentions real config entry keys in quotes, for example
  `"workflow.prefer_subagent"`, when referring to file-backed entries.

`config.tuning`

- In full ws mode, shows `"workflow.prefer_subagent"` and
  `"workflow.prefer_mercenary"` backed by their writer tool schemas.
- In wsflow/no-agent mode, keeps shared non-agent knobs such as
  `"workflow.prefer_subagent"` and omits full-ws-only mercenary/model-tier
  controls.
- Discovers prompt knobs only from currently shipped override markers, so the
  removed `DelegationSection` marker stays absent even when orphan state exists
  on disk.

## Contract Instructions

- Reuse existing schema-backed tuning catalog machinery; do not hand-maintain a
  separate enum table in `lead-tune`.
- Keep `"workflow.prefer_subagent"` and `"workflow.prefer_mercenary"` global-only
  workflow preferences. The writer tools require a lead `session_key` as
  authority, not as a session scope selector.
- Preserve Phase 2 rendering: `lead-workflow-manual` appends
  `lead-prefer-subagent` only through normal playbook rendering and the XML-style
  `<playbook name="lead-prefer-subagent" title="Prefer Subagent">` boundary.
- Keep wsflow distributed text free of full-ws-only MCP notation and mercenary
  commands after product-mode rendering.
- For changed Invariants or Constraints lines, apply the `lead-skill-authoring`
  checklist before committing.

## Integration Test Instructions

Focused files and surfaces:

- `agents-plugin/rsrc/lead-tune/lead-tune.md`
- `agents-plugin/rsrc/manifest.json`
- `agents-plugin-wsflow/rsrc/`
- `agents-plugin-tool/internal/mcp/server.go`
- `agents-plugin-tool/internal/mcp/prompt_override_test.go`
- `agents-plugin-tool/internal/mcp/prefer_mercenary_phase2_test.go`
- `agents-plugin-tool/internal/mcp/playbook_tools_test.go`
- `agents-plugin-wsflow/tests/`
- `ai-docs/spec/mcp-tools.md`
- `ai-docs/spec/workflow-skills.md`
- `ai-docs/mental-model/mcp-runtime.md`
- `ai-docs/mental-model/prompt-bundle.md`
- `ai-docs/mental-model/workflow-skills.md`

Add or update tests proving:

- `config.tuning` full ws output names the workflow preference knobs and writer
  tools, with enum options coming from writer schemas.
- `config.tuning` wsflow/no-agent output keeps `"workflow.prefer_subagent"` and
  omits `"workflow.prefer_mercenary"` plus agent-tier knobs.
- `config.prompt` / `config.tuning` omit `DelegationSection` after marker
  removal while preserving `UserPreferenceSection` and
  `PreferSubagentInvocationGuidance`.
- `ws.lead.prefer_mercenary` remains absent from tools/list,
  `runtime.capabilities`, runtime manifests, and explicit calls.
- `lead-tune` rendered output no longer recommends `prompt.DelegationSection.*`
  and instead references the workflow writer tools.
- wsflow package tests stay green after rsrc mirror regeneration.

Suggested commands:

```bash
cd agents-plugin-tool
go test ./internal/mcp -count=1 -run 'Test.*WorkflowPrefer|Test.*PreferSubagent|Test.*PreferMercenary|TestConfigTuning|TestConfigPrompt|TestPlaybookPrintWsflow|TestPlaybookPrintGoldenLeadWorkflowManual|TestRuntimeCapabilities|TestLeadToolNames'
WS_REGEN_MANIFEST=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateShippedManifest
WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowRsrcMirror
go test ./internal/mcp ./internal/wsconfig ./internal/wsrsrc ./cmd/ws-mcp -count=1
python3 -m unittest discover ../agents-plugin-wsflow/tests
git diff --check
```

## Implementation Strategy Decisions

- Treat Phase 1-2 code behavior as the baseline. Phase 3 should polish and
  verify the surrounding workflow surfaces, not redesign the config model.
- Prefer compact `lead-tune` prose that points at `config.tuning` and writer
  tools instead of duplicating schema options.
- Preserve the distinction between config entry keys and writer tool names in
  docs: quote entry keys such as `"workflow.prefer_subagent"` and name tools as
  `config.workflow_prefer_subagent`.
- Keep `UserPreferenceSection` as the only standing freeform preference slot in
  the workflow manual.

## Rejected Alternatives

- Freeform delegation prompt overrides: rejected because small posture choices
  belong in schema-backed workflow config keys.
- Session/project scoped prefer-subagent behavior: rejected because workflow
  manual bootstrap is commonly keyless before `ws.ferrule`.
- `ws.ferrule` prompt paste for prefer-subagent: explicitly out of scope.
- Keeping `ws.lead.prefer_mercenary` as a compatibility alias: rejected because
  the feature had not shipped and old state can remain orphaned.

## Approach

- Survey current `lead-tune`, config catalog, render, manifest, and wsflow tests.
- Patch `lead-tune` and any catalog/render tests needed to lock the Phase 3
  behavior.
- Regenerate canonical rsrc manifest and wsflow rsrc mirror.
- Run focused and broader verification.
- Run fresh-reader audit on changed playbook text; fix only accepted findings.
- Update specs, mental models, ticket result, and project index.

## Constraints

- AI-authored docs, plans, commits, tickets, and code comments stay English.
- Use `apply_patch` for manual edits.
- Do not revert unrelated worktree changes.
- Runtime manifests are not updated for rsrc-only text edits unless live tool or
  command inventories change.
- `agents-plugin-wsflow/rsrc/` must stay byte-identical to canonical
  `agents-plugin/rsrc/`.

## Out of scope

- Merge to an integration branch or `main`.
- Version bump.
- New config scopes or migration of old local config entries.
- Broad lead-tune redesign beyond this workflow delegation posture surface.

## Details

Implementation-start commit: `860c400624a230a5470516eadfd0e0f9da991c17`.
Current branch: `implement/260625-workflow-delegation-config`.

Phase 1 result commit: `25c50a7a`.
Phase 2 result commit: `ce23337`.

## Verification Contract

- Focused MCP/config/render tests pass.
- Manifest regeneration updates or confirms `agents-plugin/rsrc/manifest.json`.
- wsflow mirror regeneration updates or confirms `agents-plugin-wsflow/rsrc/`.
- Broader Go package tests pass for `./internal/mcp`, `./internal/wsconfig`,
  `./internal/wsrsrc`, and `./cmd/ws-mcp`.
- wsflow package tests pass.
- `git diff --check` passes.
- `ws/spec_index.verify` passes after doc updates.

## References

- [Must] `ai-docs/tickets/ready/260625-refactor-workflow-delegation-config.md`
  - selected Phase 3 scope, decisions, exclusions, and prior phase results.
- [Must] `ai-docs/tickets/idea/260605-research-ws-native-subagent-pivot.md`
  - migration anchor for playbook-factory direction, native-subagent posture,
  mercenary boundaries, and wsflow convergence.
- [Must] `ai-docs/mental-model/mcp-runtime.md` - config writer, global-only
  workflow preference, product-mode visibility, and runtime capability contracts.
- [Must] `ai-docs/mental-model/prompt-bundle.md` - tuning catalog,
  prompt-override marker discovery, playbook render, rsrc manifest, and wsflow
  mirror contracts.
- [Must] `ai-docs/mental-model/workflow-skills.md` - `lead-tune`,
  `lead-prefer-subagent`, workflow manual, and implementation workflow contracts.
- [Must] `ai-docs/mental-model/plugin-runtime.md` - runtime manifests, wsflow
  package contracts, and capability inventory coupling.
- [Must] `ai-docs/ref/wsflow-mirroring.md` - wsflow rsrc mirror and package
  verification requirements.
- [Must] `ws/playbook.print(name: "lead-skill-authoring")` - prompt/playbook
  authoring layout, invariant checklist, and fresh-reader audit requirements.
- [Must] `ws/infra.read(name: "impl-playbook")` - implementation and verification
  discipline.
