# Brief: 260625-refactor-workflow-delegation-config Phase 2

## Intent

Make `"workflow.prefer_subagent"` active during workflow-manual loading by
appending the normally-rendered `lead-prefer-subagent` playbook to
`lead-workflow-manual`, wrapped in the standard XML-style playbook boundary.

## Scope Boundary

Implement Phase 2 only: workflow manual auto-insertion and reusable pragmatic
playbook concatenation helper.

In scope:

- Remove the `DelegationSection` override marker and seeded posture text from
  `lead-workflow-manual`.
- Keep `UserPreferenceSection` unchanged.
- When global/builtin `"workflow.prefer_subagent"` resolves to `on`, append
  `lead-prefer-subagent` every time `lead-workflow-manual` is rendered.
- Render the appended body through the existing harness-aware renderer, prompt
  override lookup, variable substitution, and product-mode pass.
- Wrap the appended body as
  `<playbook name="lead-prefer-subagent" title="Prefer Subagent">...</playbook>`.
- Add a reusable helper for future code-side pragmatic playbook concatenation.
- Regenerate the rsrc manifest and wsflow rsrc mirror after rsrc edits.

Out of scope:

- Do not add duplicate insertion detection.
- Do not implement project- or session-scoped prefer-subagent behavior.
- Do not paste prefer-subagent guidance from `ws.ferrule`.
- Do not rewrite `lead-tune` or tuning catalog text beyond test fallout caused
  directly by removing `DelegationSection`; Phase 3 owns that polish.
- Do not migrate orphaned `prompt.DelegationSection.*` values.

## Caller-Visible Contract

`playbook.print(name: "lead-workflow-manual")`

- With `"workflow.prefer_subagent" == "off"` or unset, renders the workflow
  manual without the strict maximum-delegation posture.
- With `"workflow.prefer_subagent" == "on"`, appends the rendered
  `lead-prefer-subagent` playbook inside the XML-style boundary.
- Keyless calls resolve `"workflow.prefer_subagent"` from global/builtin state
  only.
- Session-keyed calls may still resolve prompt overrides for ordinary override
  markers, but the prefer-subagent decision remains global/builtin.
- The appended playbook must use the same harness-aware render path as standalone
  `playbook.print(name: "lead-prefer-subagent")`.

`config.prompt` and `config.tuning`

- Removing the shipped `DelegationSection` marker naturally removes it from
  marker discovery output.
- Orphaned stored `prompt.DelegationSection.*` values remain ignored.
- `UserPreferenceSection` and `PreferSubagentInvocationGuidance` remain
  discoverable.

## Contract Instructions

- Reuse `renderPlaybookBody`; do not read or concatenate rsrc files directly.
- Add the concatenation helper in the MCP playbook-rendering layer, close to
  `renderPlaybookBody`, because programmatic concatenation is a render concern.
- The helper should accept already-rendered Markdown body text and produce the
  XML-style wrapper. It should derive or accept the human title as explicit data;
  do not parse source frontmatter for the wrapper.
- The `lead-workflow-manual` append path must not require a `session_key`,
  project root, or `ws.ferrule` result.
- Avoid stateful duplicate guards; double insertion is accepted when a caller
  also explicitly invokes `lead-prefer-subagent`.
- Preserve existing `WorkflowLang` and namespace substitution behavior.
- Preserve wsflow product-mode behavior: no bare `ws/`, `ws:`, `ws.mercenary.*`,
  or marker syntax in rendered wsflow manual output.

## Integration Test Instructions

Focused files:

- `agents-plugin-tool/internal/mcp/playbook_tools.go`
- `agents-plugin-tool/internal/mcp/playbook_tools_test.go`
- `agents-plugin-tool/internal/mcp/prompt_override_test.go`
- `agents-plugin-tool/internal/mcp/prefer_mercenary_phase2_test.go`
- `agents-plugin-tool/internal/wsrsrc/wsflow_mirror_test.go`
- `agents-plugin/rsrc/lead-workflow-manual/lead-workflow-manual.md`
- `agents-plugin/rsrc/manifest.json`
- `agents-plugin-wsflow/rsrc/`

Add or update tests proving:

- builtin/global `"workflow.prefer_subagent" == "off"` does not append
  `lead-prefer-subagent`;
- global `"workflow.prefer_subagent" == "on"` appends the XML-wrapped playbook
  in keyless `playbook.print`;
- Codex renders the builtin
  `prompt.PreferSubagentInvocationGuidance.codex` guidance inside the appended
  wrapper;
- Claude does not receive Codex-specific invocation guidance;
- removing `DelegationSection` removes it from `config.prompt` /
  `config.tuning` marker discovery while preserving `UserPreferenceSection`;
- no raw override marker syntax or product-mode marker syntax survives;
- wsflow/no-agent manual rendering remains namespace-clean and can append the
  shared `lead-prefer-subagent` posture when the global knob is on.

Commands:

```bash
cd agents-plugin-tool
go test ./internal/mcp -count=1 -run 'Test.*WorkflowPrefer|Test.*PreferSubagent|Test.*DelegationSection|TestConfigTuning|TestConfigPrompt|TestPlaybookPrintWsflow|TestPlaybookPrintGoldenLeadWorkflowManual'
go test ./internal/wsrsrc -count=1 -run 'TestRegenerateManifest|TestRegenerateWsflowRsrcMirror'
WS_REGEN_MANIFEST=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateManifest
WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowRsrcMirror
go test ./internal/mcp ./internal/wsconfig ./internal/wsrsrc ./cmd/ws-mcp -count=1
python3 -m unittest discover ../agents-plugin-wsflow/tests
git diff --check
```

## Implementation Strategy Decisions

- Treat `renderPlaybookBody` as the one normal renderer for both the manual and
  appended playbook.
- Decide the append in `printPlaybook`/print dispatch code, not in rsrc template
  syntax.
- Read `"workflow.prefer_subagent"` through the global-only resolver with an
  empty session key so keyless manual bootstrap works.
- Keep prompt override defaults for `PreferSubagentInvocationGuidance`; this is
  the proof that the append path did not raw-concatenate files.
- Remove only the `DelegationSection` marker from the manual in Phase 2. Leave
  stale `lead-tune` prose for Phase 3 unless tests require a narrow update.

## Rejected Alternatives

- New include/template syntax: rejected by ticket; the append is a code-side
  pragmatic concatenation standard.
- Project/session-scoped prefer-subagent: rejected by Sage review because the
  workflow manual is commonly keyless before `ws.ferrule`.
- Duplicate guards: rejected by ticket; duplicate insertion is accepted.
- Raw file concatenation: rejected because it bypasses harness overlays,
  builtin prompt overrides, and product-mode filtering.

## Constraints

- AI-authored docs, tests, commits, and code comments stay English.
- Prompt/playbook source edits require manifest regeneration and wsflow mirror
  regeneration.
- Skill/playbook text edits require fresh-reader audit before closeout.
- This phase must leave Phase 3 surfaces intentionally unfinished: lead-tune
  routing prose, broader catalog wording, and spec/mental-model final polish.

## References

- [Must] `ai-docs/tickets/ready/260625-refactor-workflow-delegation-config.md`
  - Phase 2 scope, XML wrapper standard, and explicit exclusions.
- [Must] `ai-docs/tickets/idea/260605-research-ws-native-subagent-pivot.md`
  - playbook-factory direction, rsrc render boundaries, and wsflow convergence.
- [Must] `ai-docs/mental-model/prompt-bundle.md` - renderer ordering,
  override-marker behavior, builtin prompt override defaults, and rsrc mirror
  rules.
- [Must] `ai-docs/mental-model/workflow-skills.md` - workflow-manual role,
  prefer-subagent posture, and lead-implement constraints.
- [Must] `ai-docs/mental-model/mcp-runtime.md` - global-only workflow config,
  keyless behavior, and config visibility contracts.
- [Must] `ai-docs/ref/wsflow-mirroring.md` - wsflow rsrc mirror and package
  verification requirements.
- [Must] `ws/infra.read(name: "impl-playbook")` - implementation and verification
  discipline.
