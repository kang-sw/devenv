# Survey: 260615-refactor-native-explore-dispatch-skill-guidance

## Reusable Components

- `agents-plugin/rsrc/lead-workflow-manual/lead-workflow-manual.md#L55-L62` - Scoped Exploration primitive: current worktree already states direct host-native Explore/search dispatch with scoped prompt, citations, gaps, follow-up needs, and parallel fan-out.
- `agents-plugin/rsrc/lead-workflow-manual/lead-workflow-manual.md#L193-L200` - Usage Pattern block: current worktree distinguishes direct scoped exploration from bundled delegate prompts that still render through `ws/playbook.render(name: "<delegate>")`.
- `agents-plugin/rsrc/subagent-rules.md#L7-L20` - Exploration Helper: current worktree gives pasteable caller rules for host-native Explore/search workers without rendering `explore`.
- `agents-plugin/rsrc/explore/explore.md#L1-L36` - `explore` fallback playbook: still `kind: render`, `delegates: true`, and variable-driven through `ExploreAgent`/`SpawnIdiom`/`ContinueIdiom`.
- `agents-plugin-tool/internal/wsrsrc/manifest.go#L35-L82` - `GenerateManifest` / `WriteManifest`: deterministic shipped rsrc manifest generation and persistence helpers used by regen tests.
- `agents-plugin-tool/internal/mcp/playbook_tools.go#L13-L39` - Harness terminology table: keeps `explore` fallback rendering harness-aware for Claude, Codex, and neutral harnesses.

## Existing Patterns

- Generic call-site wording: see `agents-plugin/rsrc/lead-discuss/lead-discuss.md#L16-L20`, `agents-plugin/rsrc/lead-sprint/lead-sprint.md#L113-L117`, and `agents-plugin/rsrc/lead-verify-discussion/lead-verify-discussion.md#L20-L27` for direct native Explore/search dispatch phrased without `explore` rendering.
- Purpose-specific query blocks: see `agents-plugin/rsrc/lead-forge-spec/lead-forge-spec.md#L41-L63`, `agents-plugin/rsrc/lead-forge-spec/lead-forge-spec.md#L198-L205`, and `agents-plugin/rsrc/lead-forge-mental-model/lead-forge-mental-model.md#L109-L116` for passing existing query/check blocks as native subagent prompts.
- Documentation reconciliation shape: see `ai-docs/spec/workflow-skills.md#L91-L102`, `ai-docs/mental-model/workflow-skills.md#L24-L28`, and `ai-docs/mental-model/prompt-bundle.md#L43-L46` for the current direct-dispatch/fallback distinction.
- Generated wsflow rsrc sameness: see `agents-plugin-wsflow/rsrc/lead-workflow-manual/lead-workflow-manual.md#L55-L62` and `agents-plugin-wsflow/rsrc/subagent-rules.md#L7-L20`; current worktree mirrors the canonical wording byte-for-byte in these sampled files.
- Skill-authoring audit rule: see `agents-plugin/rsrc/lead-skill-authoring/lead-skill-authoring.md#L46-L52`; procedure playbook edits should preserve the active-conversation ownership boundary and direct Explore/search wording.

## Relevant Interfaces

- `agents-plugin-tool/internal/wsrsrc/manifest_shipped_test.go#L16-L60` - Shipped manifest drift guard and `WS_REGEN_MANIFEST=1 go test ./internal/wsrsrc -run TestRegenerateShippedManifest` regeneration entry point.
- `agents-plugin-tool/internal/wsrsrc/wsflow_mirror_test.go#L47-L113` - Byte-identical wsflow rsrc mirror guard and `WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -run TestRegenerateWsflowRsrcMirror` regeneration entry point.
- `agents-plugin-tool/internal/wsrsrc/wsrsrc_test.go#L736-L762` - Real-tree validation plus older `WSRSRC_REGEN=1` manifest regeneration test; useful because existing docs mention both manifest patterns.
- `agents-plugin-tool/internal/mcp/playbook_tools_test.go#L735-L842` - Golden tests for `explore` fallback rendering across Claude, Codex, unknown, and junk harnesses.
- `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py#L88-L103` - wsflow distributed skill checks forbid full-ws references in skill files and assert manual subagent guidance is present.
- `agents-plugin-wsflow/tests/test_wsflow_runtime_contract.py#L90-L133` - wsflow runtime contract checks `prompt.render` is wsflow-only and hidden from full ws.

## Constraints

- `ai-docs/.plans/2026-06/15-260615-refactor-native-explore-dispatch-skill-guidance.brief.md#L26-L32` - Canonical rsrc files are edited first; manifests and byte-identical wsflow rsrc mirror follow; implementer/reviewer delegate rendering remains through `playbook.render`.
- `ai-docs/.plans/2026-06/15-260615-refactor-native-explore-dispatch-skill-guidance.brief.md#L50-L56` - Do not add `explore.codex.md`, do not add empty `explore.claude.md`, and do not delete `explore.md` or its tests in this phase.
- `ai-docs/ref/wsflow-mirroring.md#L95-L118` - The rsrc subtree is the generated-sameness carve-out; stored wsflow rsrc must be byte-identical to canonical and namespace differences happen at render time.
- `ai-docs/ref/wsflow-mirroring.md#L135-L155` - wsflow package tests make distributed skill drift visible but do not require skill text to be byte-identical; this differs from rsrc mirror rules.
- `agents-plugin-tool/internal/wsrsrc/loader.go#L65-L88` - rsrc loads validate manifest hashes before returning playbook bodies/includes, so stale manifest hashes break runtime loading loudly.

## Risk Signals

- `agents-plugin-tool/internal/mcp/playbook_tools_test.go#L1137-L1139` - Possible doc/test-comment risk: several golden-test comments still say `explore playbook delegation` or `explore playbook spawns`; escaped search now leaves only these test comments, but lead/planner should decide whether comments need fallback wording or can remain as test-scope references.
- `agents-plugin/rsrc/manifest.json#L19-L32` - Possible coordination risk: current worktree already has regenerated canonical manifest hashes for touched rsrc files; preserve these pre-existing edits and avoid re-running stale or alternate regen paths that could overwrite concurrent wording.
- `agents-plugin-wsflow/rsrc/manifest.json#L19-L32` - Possible mirror risk: current worktree also has wsflow rsrc and manifest edits; `TestWsflowRsrcMirrorUpToDate` is the guard that should confirm byte identity rather than manual inspection.
- `ai-docs/mental-model/prompt-bundle.md#L46-L51` - Possible contract risk: `explore` remains in the rsrc tree as fallback/compatibility while other bundled delegates still render normally; broad search replacements must not rewrite implementer/reviewer/reference-discovery render guidance as if all playbook rendering were deprecated.
- `agents-plugin/rsrc/lead-workflow-manual/lead-workflow-manual.md#L47-L53` - Possible ws-tooling risk: root-aware ws tools still require a `session_key`; during this survey, `ws/mental_models.find` could not accept the rendered `session_key` through the exposed MCP schema, so source reads and search carried the doc-gap discovery.

## Opinion

- `ai-docs/tickets/ready/260615-refactor-native-explore-dispatch-skill-guidance.md#L90-L115` - The active worktree appears to have already covered most Phase 1 edit surfaces; remaining implementation work should be validated against the ticket's search, mirror, manifest, test, and doc reconciliation boundaries.
- `ai-docs/.plans/2026-06/15-260615-refactor-native-explore-dispatch-skill-guidance.brief.md#L77-L84` - The highest-signal final checks are escaped search plus focused `internal/wsrsrc`, `internal/mcp`, and wsflow package tests; no research escalation is needed for a reference map.
