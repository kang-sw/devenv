---
title: "playbook.render root_override resolves manifest from repository root"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 6265b8e4e1309ed4
sage-review-completeness-reviewed: 6265b8e4e1309ed4
---

# playbook.render root_override resolves manifest from repository root

## Background

During Phase 2 of `260730-chore-ws-dashboard-drop-sweep`, a lead session bound
to a linked worktree rendered `plan-populator-survey` with `root_override` set
to that worktree. The call failed with:

```text
rsrc manifest missing at <worktree>/manifest.json
```

`root_override` is documented as selecting delegate include resolution and the
child-key root; it should not make the renderer look for the plugin resource
manifest in the repository worktree.

## Direction

Separate the plugin resource root from the delegate worktree root in
`playbook.render` without adding another public parameter:

- `root_override` selects the worktree used for the generated prompt artifact
  and the root bound to a render-minted child session key.
- The playbook, its manifest, and its includes resolve through the normal plugin
  resource root (`WS_RSRC_ROOT` when configured, otherwise the executable-derived
  installed bundle), regardless of `root_override`.
- Calls that omit `root_override` keep their current behavior.

## Non-Scope

- Worktree acquisition, release, pooling, or parallel ticket execution.
- A new `playbook.render` parameter or a change to `playbook.read`.

## Verification

- Add a regression test with separate resource and linked-worktree roots where
  only the resource root contains `manifest.json`; rendering with
  `root_override` must load the playbook from the resource root, write the
  artifact under the worktree-scoped cache, and bind the child key to the
  overridden worktree.
- Keep existing no-override, resource-integrity, include-resolution, and
  child-key tests green, then run the full Go suite.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/mcp/server.go, agents-plugin-tool/internal/mcp/playbook_tools.go, existing MCP tests, and shipped workflow guidance |
| scope.surface | public-interface | playbook.render's existing root_override contract is published in its MCP schema (agents-plugin-tool/internal/mcp/server.go#L3719-L3730) |
| scope.new_public_symbol | no | no new parameter or symbol is requested |
| scope.new_type_contract | no | the existing root_override parameter changes resolution behavior only |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/playbook_tools_test.go and agents-plugin-tool/internal/mcp/playbook_render_surface_test.go exercise render path allocation and child-key binding |
| complexity.reuse_points | confirmed | renderPlaybook already takes separate rsrcRoot and worktreeRoot arguments (agents-plugin-tool/internal/mcp/playbook_tools.go#L861-L887) |
| complexity.side_effect_risk | moderate | the same override currently controls resource lookup, artifact allocation, and child-key binding |
| risk.correctness | moderate | incorrect root separation can load the wrong resource tree or bind a child to the wrong worktree |
| risk.fit | moderate | the published schema and shipped workflow guidance must match the split contract |
| risk.test | moderate | a dispatch-level split-root regression must preserve existing no-override and child-key behavior |
| risk.security_or_contract | high | root_override is an existing public MCP parameter whose documented behavior changes |

## Phases

### Phase 1: Separate resource lookup from worktree binding

Make `playbook.render` resolve the resource tree independently of
`root_override`, while retaining the override for prompt-path allocation and
child-key binding. Align the published schema description and shipped workflow
guidance with that contract, add the split-root regression test, and verify the
full Go suite plus affected plugin surface tests.
