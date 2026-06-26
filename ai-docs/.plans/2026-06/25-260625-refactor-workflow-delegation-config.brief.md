# Brief: 260625-refactor-workflow-delegation-config Phase 1

## Intent

Refactor workflow delegation posture settings from a freeform prompt override and
legacy `ws.lead.*` tool into explicit workflow-prefixed config keys and writer
tools.

## Scope Boundary

Implement Phase 1 only: config namespace and API refactor.

In scope:

- Add stored config key `"workflow.prefer_subagent"` with values `on|off`,
  builtin default `off`, and global-only/default write behavior.
- Add writer tool `config.workflow_prefer_subagent`.
- Add stored config key `"workflow.prefer_mercenary"` with values `on|off|hide`,
  builtin default `hide`, and global-only/default write behavior.
- Add writer tool `config.workflow_prefer_mercenary`.
- Remove `ws.lead.prefer_mercenary` immediately, with no alias and no migration.
- Leave old `"prefer_mercenary"` stored values orphaned.
- Expose the new workflow knobs through `config.show` and `config.tuning` where
  Phase 1 touches the API surface.

Out of scope for Phase 1:

- Do not remove `DelegationSection` from `lead-workflow-manual`.
- Do not implement workflow-manual auto-insertion.
- Do not add the XML playbook concatenation helper.
- Do not rewrite `lead-tune`.
- Do not close out the ticket/spec/mental-model docs beyond code-adjacent updates
  needed by Phase 1 tests.
- Do not run manifest or wsflow rsrc mirror regeneration unless a Phase 1 source
  change unexpectedly touches rsrc text.

## Branch And Start

- Branch before Prep: `implement/260625-lead-tune-tuning-catalog`.
- Branch after Prep: `implement/260625-workflow-delegation-config`.
- Implementation-start commit: `a5bd6b9dfa1e509b83f7a90288ddca8642cb1a10`.

## Binding Decisions

Use the ready ticket's Sage-resolved decisions over older planned spec notes
that still mention session-default behavior.

- `"workflow.prefer_subagent"` is a global bootstrap preference. It is not
  session-scoped and not project-scoped.
- Keyless reads may resolve `"workflow.prefer_subagent"` from global/builtin
  state only. Phase 1 only needs the config/API substrate; manual insertion is
  Phase 2.
- `"workflow.prefer_mercenary"` is global-only. It is not session-scoped and not
  project-scoped because `hide` affects keyless tool-surface visibility before a
  session key or project root exists.
- Keyless tool-surface visibility and later render guidance must read the same
  global/builtin `"workflow.prefer_mercenary"` value.
- The old unprefixed `"prefer_mercenary"` key is orphaned local state. Do not
  read, write, migrate, or backfill it.
- `ws.lead.prefer_mercenary` is removed from schema, dispatch, runtime metadata,
  tuning catalog, tests, and docs touched by Phase 1. Do not keep an alias.
- Remove the legacy `enabled` compatibility shape with the old tool. The new
  mercenary writer accepts only canonical `value: on|off|hide`.
- `config.*` keyed gate remains the authority boundary for delegate/leaf keys.
  Do not reintroduce `ws.lead.*` as a soft guard for config settings.
- wsflow/no-agent should not expose mercenary controls. `config.tuning` should
  continue omitting full-ws-only mercenary/model-tier knobs in no-agent mode.

## Caller-Visible Contract

`config.workflow_prefer_subagent(value: "on"|"off")`

- Lead-only through the existing `config.*` keyed capability gate.
- Does not require `session_key`.
- Writes `"workflow.prefer_subagent"` to global config.
- Returns compact readable confirmation.
- Its schema is the canonical enum source for `config.tuning`.

`config.workflow_prefer_mercenary(value: "on"|"off"|"hide")`

- Lead-only through the existing `config.*` keyed capability gate.
- Does not require `session_key`.
- Writes `"workflow.prefer_mercenary"` to global config.
- `on` makes implementer/reviewer render guidance prefer the mercenary path.
- `off` keeps native-subagent default guidance while leaving explicit mercenary
  use available where the mercenary surface is visible.
- `hide` is the builtin default and hides `ws.mercenary.*` from keyless tool
  visibility plus suppresses mercenary render content.
- Its schema is the canonical enum source for `config.tuning`.

`config.show`

- Should enumerate both registered workflow keys even when unset, resolving them
  from builtin defaults.
- Keyless and session-keyed output should agree that these keys are global/builtin
  items, not session items.

`config.tuning`

- In full ws mode, expose semantic workflow knobs backed by the new writer tools.
- Do not expose `delegation.prefer_mercenary` backed by `ws.lead.prefer_mercenary`.
- Derive enum/value metadata from `config.workflow_prefer_subagent` and
  `config.workflow_prefer_mercenary` schemas.
- In wsflow/no-agent mode, omit the mercenary workflow knob if it is considered
  full-ws-only; keep prompt override entries as before. `workflow.prefer_subagent`
  is acceptable in no-agent only if the implementation confirms it has no hidden
  agent-backed dependency.

## Migration Anchor Constraints

The migration anchor `260605-research-ws-native-subagent-pivot` is binding for
this phase because the ticket changes workflow routing, host-neutral surfaces,
wsflow convergence, and mercenary/native delegation boundaries.

Apply these constraints:

- Preserve the playbook-factory direction: MCP should expose host-neutral
  playbook/config surfaces rather than spawning ws-owned subprocess agents for
  ordinary workflow routing.
- Keep `playbook.print` and `playbook.render` as distinct surfaces; Phase 1 must
  not blur keyless manual loading with render-time session/root behavior.
- Keep harness-aware behavior as data/config and runtime selection, not
  hard-coded Claude/Codex prose in shared configuration code.
- Keep wsflow/no-agent convergence in mind: full-ws-only mercenary surfaces must
  remain hidden in wsflow/no-agent mode, while shared config surfaces should be
  exposed only when their semantics make sense without agents.
- Do not restore old spawn/profile authority paths. The keyed capability gate is
  the server-side permission authority; `WS_MCP_TOOL_PROFILE` is retired.

## Implementation Notes

Likely implementation shape:

- Add constants such as `ItemWorkflowPreferSubagent` and
  `ItemWorkflowPreferMercenary` in `internal/wsconfig/scope.go`.
- Register both new workflow keys with `ScopeGlobal`.
- Provide builtin defaults for these keys where resolver callers need defaults:
  `"workflow.prefer_subagent" -> "off"` and
  `"workflow.prefer_mercenary" -> "hide"`.
- Update mercenary visibility/read paths to use `"workflow.prefer_mercenary"`
  with an empty session key and global/builtin resolution.
- Update render-time mercenary guidance to read the same global/builtin key, not
  the lead session key's `"prefer_mercenary"` override.
- Add dispatch and schemas for the two new `config.workflow_*` writer tools.
- Remove schema and dispatch for `ws.lead.prefer_mercenary`.
- Update `LeadToolNames`, `filteredTools`, `toolAllowed`, `noAgentHiddenTool`,
  and runtime manifests so the removed tool is gone and new tools appear in the
  correct product mode.
- Update tuning catalog ids/writers/currents so schema projection comes from the
  new writer tools.

## Integration Test Instructions

Focused test paths:

- `agents-plugin-tool/internal/wsconfig/scope_test.go`
- `agents-plugin-tool/internal/mcp/server_test.go`
- `agents-plugin-tool/internal/mcp/mercenary_surface_test.go`
- `agents-plugin-tool/internal/mcp/prefer_mercenary_phase2_test.go`
- `agents-plugin-tool/internal/mcp/prompt_override_test.go`
- `agents-plugin-tool/internal/mcp/session_auth_test.go`
- `agents-plugin-wsflow/tests/test_wsflow_runtime_contract.py`

Add or update tests for:

- default scope for both workflow keys is global;
- keyless resolver reads return builtin `off`/`hide` when unset;
- writer tools validate enum values and write global config without `session_key`;
- delegate/leaf keys cannot call the new `config.workflow_*` tools via the
  existing `config.*` gate;
- `ws.lead.prefer_mercenary` is absent from `tools/list`, `LeadToolNames`, and
  runtime manifests, and explicit calls fail as unknown;
- `ws.mercenary.*` visibility follows global `"workflow.prefer_mercenary"`:
  hidden at builtin/`hide`, visible at `on` and probably `off` only if the
  existing explicit-mercenary availability contract requires it;
- implementer/reviewer render guidance uses global `"workflow.prefer_mercenary"`
  and does not depend on session overrides;
- `config.show` reports `"workflow.prefer_subagent"` and
  `"workflow.prefer_mercenary"` with builtin/global scopes;
- `config.tuning` uses `config.workflow_prefer_*` writer schemas and no longer
  references `ws.lead.prefer_mercenary`.

Suggested commands after implementation:

```bash
cd agents-plugin-tool
go test ./internal/wsconfig -count=1
go test ./internal/mcp -count=1 -run 'Test.*WorkflowPrefer|Test.*PreferMercenary|TestConfigTuning|TestNoAgent|TestRuntimeCapabilities|TestLeadToolNames'
go test ./internal/mcp ./internal/wsconfig ./cmd/ws-mcp -count=1
python3 -m unittest discover ../agents-plugin-wsflow/tests
git diff --check
```

If runtime manifests change, also run the relevant runtime contract tests through
the Go/package tests above and confirm `agents-plugin/runtime.json` and
`agents-plugin-wsflow/runtime.json` match live capabilities.

## Known Drift To Handle

Some current spec and mental-model text still says `prefer_mercenary` or the new
workflow keys are session-default items. That text is stale relative to the ready
ticket and must not drive implementation. If Phase 1 implementation edits docs,
update the touched stale lines to global-only. If docs are deferred to Phase 3,
mention the stale lines in the implementation closeout.

Existing tests intentionally cover the old `ws.lead.prefer_mercenary` session
contract. They should be rewritten or removed; do not preserve the old behavior
just to keep those tests green.

## Out Of Scope

- `lead-workflow-manual` `DelegationSection` removal.
- Automatic `lead-prefer-subagent` append.
- XML `<playbook>` concatenation wrapper.
- `lead-tune` prose rewrite.
- Migration from old `"prefer_mercenary"` values.
- Project- or session-scoped prefer-subagent behavior.
- `ws.ferrule` prompt paste.

## References

- [Must] `ai-docs/tickets/ready/260625-refactor-workflow-delegation-config.md` -
  Phase 1 selected scope, Sage-resolved global-only decisions, and orphan/no-alias
  rules.
- [Must] `ai-docs/tickets/idea/260605-research-ws-native-subagent-pivot.md` -
  migration anchor for playbook-factory direction, native-subagent pivot, and
  wsflow convergence.
- [Must] `ai-docs/spec/mcp-tools.md` - config tools, tuning catalog, layered
  config scope model, playbook tools, and mercenary delegation surface anchors.
- [Must] `ai-docs/spec/workflow-skills.md` - lead-tune and workflow primitive
  planned behavior, with Phase 1 docs drift noted above.
- [Must] `ai-docs/mental-model/mcp-runtime.md` - MCP schema/dispatch coupling,
  config resolver, product-mode gates, runtime manifest rules, and keyed
  capability gate.
- [Must] `ai-docs/mental-model/prompt-bundle.md` - playbook render/config
  boundaries, prompt override/tuning catalog behavior, and wsflow/no-agent
  product-mode expectations.
- [Must] `ai-docs/mental-model/workflow-skills.md` - implementation workflow,
  lead-implement brief requirements, and migration-anchor handling.
- [Must] `ws/infra.read(name: "impl-playbook")` - implementation invariants,
  test-failure diagnosis, verification, and deviation protocol.
