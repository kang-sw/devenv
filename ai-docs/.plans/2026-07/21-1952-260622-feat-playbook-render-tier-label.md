# Plan: 260622-feat-playbook-render-tier-label — Phase 1: Render and teach native spawn bindings

## Relevant Ticket Contract
- Preserve `playbook.render`'s first prompt-path line and exact `recommended-tier: <tier>` line; add `recommended-model` only when the declared tier resolves to a model, then `recommended-reasoning-effort` only when effort is non-empty.
- Resolve the binding at render time through `wsconfig.ResolveAgentForHarnessConfig`; capability tiers remain the host-neutral vocabulary, and user-local harness mappings override defaults.
- Codex-specific workflow guidance must map the rendered values to native `spawn_agent.model` and `spawn_agent.reasoning_effort` (never `effort`), and must report an optional-field rejection honestly while retaining the mercenary exact-binding fallback where applicable.
- Remove delegate-body `Alias model for this role:` echoes, update `mcp-tools.md` and `workflow-skills.md` on contact, and keep canonical rsrc, its manifest, and the byte-identical wsflow mirror synchronized.

## Out of Scope
- Changing tier vocabulary/default mappings, native spawn tool schemas, or the mercenary registration contract.
- Replacing `recommended-tier`, changing the rendered prompt path, or placing concrete provider model names in shared delegate bodies.
- Future ticket phases and any broad rewrite of `playbook.print`; retain its existing tier-only return contract unless a separately accepted contract extends it.

## Codebase Findings
- `agents-plugin-tool/internal/mcp/playbook_tools.go#L86-L99` — `resolveTierModel` already routes a tier through `wsconfig.ResolveAgentForHarnessConfig`, but deliberately discards the returned effort; a sibling result helper can reuse this exact seam without a second resolver implementation.
- `agents-plugin-tool/internal/wsconfig/config.go#L213-L255` — the shared resolver returns `(backend, model, effort)` after harness mapping; `config.go#L308-L358` supplies Codex defaults and Claude mappings naturally leave effort empty.
- `agents-plugin-tool/internal/mcp/server.go#L1300-L1357` — the `playbook.render` dispatch has the session/harness context and currently appends metadata through `withRecommendedTier`; `playbook.print` separately calls that same helper at `server.go#L1265-L1298`, so binding additions must be render-specific to avoid an unapproved print contract change.
- `agents-plugin-tool/internal/mcp/playbook_tools.go#L292-L302` and `agents-plugin-tool/internal/mcp/mercenary_surface_test.go#L637-L648` — the exact stable tier-line formatting is isolated in one helper with focused compatibility coverage.
- `agents-plugin/rsrc/lead-workflow-manual/lead-workflow-manual.md#L94-L117` and `agents-plugin/rsrc/lead-implement/lead-implement.md#L102-L111` — both describe render dispatch; the workflow manual is the selected Codex guidance surface, while `lead-implement` already says dispatch metadata stays out of worker-facing task text.
- `agents-plugin/rsrc/*/*.md` — 12 shipped delegate bodies still contain `Alias model for this role: {{.RoleModel}}.`; `agents-plugin-tool/internal/mcp/playbook_tools_test.go#L1013-L1110` asserts two of those echoes, so remove/update their golden expectations together.
- `agents-plugin-tool/internal/mcp/playbook_tools_test.go#L484-L551`, `mercenary_surface_test.go#L503-L554`, and `server_test.go#L1348-L1416` provide established fixtures for custom config resolution, resolver-error fallback, real shipped render output, and wsflow tool dispatch.
- `agents-plugin-tool/internal/wsrsrc/manifest_shipped_test.go#L17-L105` and `wsflow_mirror_test.go#L47-L110` enforce manifest freshness and an exact canonical-to-wsflow rsrc copy; changing the manual or delegate bodies requires both regeneration steps before verification.

## Implementation Plan
1. In `agents-plugin-tool/internal/mcp/playbook_tools.go`, add a render-metadata resolver/formatter that calls `wsconfig.ResolveAgentForHarnessConfig` once for the playbook's declared tier and emits ordered non-empty model/effort lines after the existing tier line; retain graceful resolver-error and empty-effort behavior, and leave `withRecommendedTier`/print output compatible.
2. In `agents-plugin-tool/internal/mcp/server.go`, wire the new binding metadata only into the `playbook.render` response after the path and stable tier line, using the detected harness and existing empty `wsconfig.Options{}` runtime configuration path; update the tool description if it documents the returned render metadata.
3. Extend `agents-plugin-tool/internal/mcp/playbook_tools_test.go`, `mercenary_surface_test.go`, and the applicable end-to-end server test with default Codex output, harness-local overridden model/effort, Claude model with omitted effort, resolver failure/model omission, exact path+tier compatibility, and unchanged `playbook.print` behavior.
4. Update `agents-plugin/rsrc/lead-workflow-manual/` with Codex-specialized guidance (using the existing overlay/override pattern rather than provider literals in shared text) that passes `recommended-model` and optional `recommended-reasoning-effort` to `spawn_agent.model` and `spawn_agent.reasoning_effort`, reports rejected optional bindings, and identifies the applicable mercenary fallback; preserve the host-neutral shared body and existing `lead-implement` worker-prompt boundary.
5. Remove the `Alias model for this role:` line from every matching canonical delegate file under `agents-plugin/rsrc/`, update render golden assertions so rendered child prompts no longer assert self-reported aliases, regenerate `agents-plugin/rsrc/manifest.json`, then regenerate `agents-plugin-wsflow/rsrc/` from canonical.
6. Update `ai-docs/spec/mcp-tools.md` with the additive render-result contract and resolution/omission behavior, and `ai-docs/spec/workflow-skills.md` with the Codex native dispatch mapping and honest rejection fallback, without redefining capability tiers.

## Verification Plan
- Run focused MCP tests covering the new helper, default/override/Claude/error metadata cases, shipped delegate renders, workflow-manual Codex guidance, and the render dispatch response; then run `cd agents-plugin-tool && go test ./internal/mcp ./internal/wsrsrc`.
- Regenerate artifacts before the drift checks: `cd agents-plugin-tool && WS_REGEN_MANIFEST=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateShippedManifest` and `WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowRsrcMirror`; verify the committed manifest and mirror guards in the full package run.
- Run `python3 -m unittest discover agents-plugin-wsflow/tests` to cover derivative package contracts after the rsrc mirror update.
- Manually inspect a Codex render response for ordered path/tier/model/effort lines and the Codex workflow-manual render for literal `model`/`reasoning_effort` guidance, never `effort` as a spawn parameter.

## Escalations
- None.
