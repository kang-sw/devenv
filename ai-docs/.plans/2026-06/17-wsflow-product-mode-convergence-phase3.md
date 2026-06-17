# Survey: 17-wsflow-product-mode-convergence-phase3.brief

## Reusable Components
- `agents-plugin/skills/lead-discuss/SKILL.md#L1-L9` — Full ws thin shim shape: frontmatter, H1, and one `ws/playbook.print(name: "lead-discuss")` execute-inline instruction; use as the canonical mechanical template with wsflow notation.
- `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py#L27-L59` — Skill inventory and forbidden-reference constants: already encode the shipped wsflow set plus `ws/`, `ws:`, `ws.`, `subquery`, `agents.*`, and excluded-skill scans.
- `agents-plugin-tool/internal/mcp/playbook_tools.go#L277-L319` — Product-mode block selector: strips `ws:full-only` / `ws:wsflow-only` marker comments and keeps only the active mode content before playbook output reaches skills or tests.
- `agents-plugin-tool/internal/mcp/playbook_tools.go#L487-L514` — `printPlaybook` / `renderPlaybook`: existing product-mode rendering path; `printPlaybook` is the shim target and `renderPlaybook` owns the Phase 2 legacy context bridge.
- `agents-plugin-tool/internal/wsrsrc/wsflow_mirror_test.go#L47-L80` — Byte-equality drift guard for `agents-plugin-wsflow/rsrc/` versus canonical `agents-plugin/rsrc/`; use as the existing enforcement instead of changing rsrc files.
- `agents-plugin/rsrc/manifest.json#L14-L32` — Shared lead playbook coverage: every shipped wsflow skill has a same-named `lead-*` rsrc playbook; `lead-salvage` and `lead-skill-authoring` remain canonical-only/excluded.

## Existing Patterns
- Full ws entry shims preserve descriptions and move behavior to playbooks: see `agents-plugin/skills/lead-proceed/SKILL.md#L1-L9` and `agents-plugin/skills/lead-sprint/SKILL.md#L1-L9`.
- Current wsflow skills are curated procedure bodies, not shims: see `agents-plugin-wsflow/skills/lead-discuss/SKILL.md#L6-L37` and `agents-plugin-wsflow/skills/lead-implement/SKILL.md#L6-L24`.
- Existing wsflow tests still assert curated-body details that must become rendered-output or shim-shape checks: see `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py#L99-L140`.
- Product-mode rendered-output checks already live in Go tests for representative lead and delegate playbooks: see `agents-plugin-tool/internal/mcp/playbook_tools_test.go#L570-L599` and `agents-plugin-tool/internal/mcp/playbook_tools_test.go#L690-L719`.
- Phase 2 context-bridge tests already protect prompt.render absorption boundaries: see `agents-plugin-tool/internal/mcp/playbook_tools_test.go#L721-L810`.

## Relevant Interfaces
- `agents-plugin-tool/internal/mcp/server.go#L2148-L2171` — MCP schemas for `playbook.print` and `playbook.render`; `playbook.print` requires only `name`, while `playbook.render` may include `session_key`, `context`, and `root_override`.
- `agents-plugin-tool/internal/mcp/server.go#L2372-L2385` — `LeadToolNames`: runtime capabilities advertise mode-filtered tool names, so wsflow package tests compare against this selected surface.
- `agents-plugin-tool/internal/mcp/server.go#L2555-L2578` — `wsflowOnlyTool` and `wsflowRenderEligibleStems`: `prompt.render` remains wsflow-only and its five-stem allowlist is still live until Phase 4.
- `agents-plugin-tool/internal/mcp/playbook_tools.go#L104-L110` and `agents-plugin-tool/internal/mcp/playbook_tools.go#L171-L175` — Reserved namespace variables: output namespace comes from `McpNamespace` / `SkillNamespace`, and caller context cannot spoof them.
- `ai-docs/ref/wsflow-mirroring.md#L26-L54` — Shipped and excluded wsflow skill inventory; implementation should not add or remove directories.
- `ai-docs/ref/wsflow-mirroring.md#L55-L70` — Distributed wsflow skill text rules: use `wsflow:lead-*` and `wsflow/<tool>`, avoid full-ws references, and keep lead integration responsibility explicit.

## Constraints
- `ai-docs/tickets/ready/260616-refactor-wsflow-product-mode-convergence.md#L161-L176` — Phase 3 changes only skill bodies/tests; Phase 4 owns `prompt.render` removal and stale curated-skill doctrine cleanup.
- `ai-docs/spec/mcp-tools.md#L325-L333` — Product-mode playbook output must use marker selection and reserved namespace variables; literal generic MCP identifiers such as `ws.lead.login` are not namespace-renamed.
- `ai-docs/mental-model/prompt-bundle.md#L59-L68` — `prompt.render` remains live, but `playbook.print` / `playbook.render` now produce wsflow-safe output from product markers and explicit namespace variables.
- `ai-docs/spec/plugin-runtime.md#L92-L113` — The rsrc mirror must stay byte-identical and local verification remains `python3 -m unittest discover agents-plugin-wsflow/tests`.
- `ai-docs/mental-model/mcp-runtime.md#L45-L50` — Product-mode tool visibility and product-mode rendering are separate contracts; do not infer `prompt.render` deletion from shimming skills.

## Risk Signals
- `agents-plugin-wsflow/skills/lead-add-rule/SKILL.md#L1-L3` — Possible contract risk: current wsflow frontmatter names are bare `lead-*`, while the brief's expected shim shows `name: wsflow:lead-*`; lead/planner should confirm whether Codex plugin namespace prefixes are declarative or should be embedded in `name:` before changing all frontmatter.
- `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py#L99-L140` — Possible test risk: several assertions inspect curated workflow-body sentences/templates, so a mechanical shim conversion will need test replacement rather than small string edits.
- `ai-docs/spec/workflow-skills.md#L177-L184` — Possible doc-staleness risk: spec still states wsflow skills are curated semantic rewrites and not text-identical; Phase 3 may intentionally make this stale until Phase 4, but implementer should avoid treating it as current source-of-truth over the ticket.
- `ai-docs/ref/wsflow-mirroring.md#L121-L129` — Possible reuse risk: mirroring docs forbid generated sameness for skills while this brief prefers mechanical generation of shims; this is a doctrine mismatch to leave for doc closeout/Phase 4, not a source-code blocker.
- `agents-plugin-wsflow/skills/lead-workflow-manual/SKILL.md#L47-L60` — Possible stale-body risk: current workflow manual includes full primitive lists and login guidance; after shimming, only the rendered shared playbook must carry equivalent wsflow-safe guidance, so rendered-output coverage should include this playbook.

## Opinion
- `agents-plugin/rsrc/manifest.json#L14-L32` — No missing shared playbook was found for the shipped wsflow skill inventory, so the brief does not need research escalation on playbook coverage.
- `ai-docs/ref/wsflow-mirroring.md#L142-L162` — The package test suite is the right enforcement point, but its documented purpose currently says inventory/forbidden-reference drift rather than shim-shape; expect a focused test-doc/name update if docs are touched in closeout.
