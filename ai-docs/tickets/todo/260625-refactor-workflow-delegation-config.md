---
title: Refactor workflow delegation posture into config keys
sage-review: pending
related-mental-model:
  - mcp-runtime
  - prompt-bundle
  - workflow-skills
---

# Refactor workflow delegation posture into config keys

## Background

Workflow delegation posture is currently split across freeform prompt overrides
and a legacy lead-only tool namespace. `lead-workflow-manual` exposes a
`DelegationSection` prompt override point, while mercenary preference is
controlled through `ws.lead.prefer_mercenary` and stored under the unprefixed
`"prefer_mercenary"` key.

That shape makes posture tuning too easy to break with freeform text and keeps a
soft-guard setting in the `ws.lead.*` namespace even though ferrule/session-key
gates now protect `config.*` from delegate and leaf keys. Delegation posture
should instead be expressed as small workflow config keys, with the strict
subagent posture reusing the existing `lead-prefer-subagent` playbook.

## Decisions

- Remove the `DelegationSection` prompt override point from
  `lead-workflow-manual`.
- Keep `UserPreferenceSection` as the freeform user preference override point.
- Accept legacy orphan state without migration. Existing
  `prompt.DelegationSection.*` prompt override entries and old
  `"prefer_mercenary"` entries may remain unused because this behavior has not
  shipped.
- Add stored config key `"workflow.prefer_subagent"` with values `on|off`,
  builtin default `off`, and session default scope.
- Add writer tool `config.workflow_prefer_subagent`.
- When `"workflow.prefer_subagent" == "on"`, `lead-workflow-manual` always
  appends the rendered `lead-prefer-subagent` playbook. Do not add a duplicate
  insertion guard.
- Rename/refactor mercenary preference into stored config key
  `"workflow.prefer_mercenary"` with values `on|off|hide`, preserving builtin
  default `hide`.
- Add writer tool `config.workflow_prefer_mercenary`.
- Remove `ws.lead.prefer_mercenary` immediately. Do not keep an alias and do not
  migrate old stored values.
- Do not use `ws.lead.*` for soft-guard config settings now that keyed gates
  protect `config.*`. Keep true auth/session primitives such as `ws.ferrule` and
  `session.children` separate from workflow config.

## Constraints

- The automatic `lead-prefer-subagent` append must render through the existing
  harness-aware playbook renderer, prompt override resolver, and product-mode
  filtering pipeline. Raw file concatenation is forbidden.
- Standalone `playbook.print` and `playbook.render` output remains Markdown.
- Do not introduce a new template include syntax for this behavior.
- The accepted behavior allows double insertion if a user enables
  `"workflow.prefer_subagent"` and also explicitly invokes `lead-prefer-subagent`.
- Implementation should update runtime manifests, wsflow mirror content, tests,
  specs, and mental models in the relevant phase.

## Prior Art or Existing Patterns

- `"workflow.lang"` already models workflow-level render behavior as a stored
  config key rather than prompt text.
- The existing mercenary preference path is code-side render guidance, not a
  template language feature.
- Prompt override markers are suitable for user preference prose, but not for
  small boolean or enum workflow posture state.
- Existing playbook `includes:` are static fragments around one playbook, not a
  contract for conditionally rendering another playbook.

## Pragmatic Playbook Concatenation Standard

When code-side rendering appends one playbook body to another, wrap the appended
body in an XML-style boundary:

```xml
<playbook name="lead-prefer-subagent" title="Prefer Subagent">
...
</playbook>
```

The `name` attribute is the runtime playbook id. The `title` attribute is the
human-readable playbook title, normally derived from the appended playbook's H1.
The wrapped body must be the already-rendered Markdown produced by the normal
playbook renderer.

This standard applies only to programmatic playbook concatenation. It is not a
new source template syntax and does not change standalone playbook export
format.

## Risks

- Claude leakage is possible if the implementation raw-includes
  `lead-prefer-subagent` and bypasses the harness-aware renderer. The append
  path must preserve harness-specific builtin overrides, including Codex-only
  invocation guidance.
- `config.prompt` may continue to show orphaned prompt entries if a local config
  file already contains `prompt.DelegationSection.*`; they should not be
  reachable from current shipped override markers or tuning catalog output.
- no-agent/wsflow behavior must be checked explicitly. The new config keys and
  playbook append behavior should match the product-mode visibility decisions
  documented for workflow skills and MCP tools.
- Double insertion is accepted by design. Avoid adding stateful guards or
  suppression logic unless a later ticket changes this decision.

## Spec Impact

Before promoting this ticket to `ready`, spec-address the caller-visible
behavior in `mcp-tools` and `workflow-skills`.

Expected spec changes include:

- config writer tools and stored key semantics for `"workflow.prefer_subagent"`
  and `"workflow.prefer_mercenary"`;
- removal of `ws.lead.prefer_mercenary` from the public MCP tool contract;
- `lead-workflow-manual` automatic append behavior when
  `"workflow.prefer_subagent" == "on"`;
- the XML-style wrapper standard for code-side pragmatic playbook
  concatenation;
- `config.tuning` catalog behavior after `DelegationSection` removal and new
  workflow config key registration.

Contract-first spec: yes.

## Phases

### Phase 1: Config namespace and API refactor

Implement the workflow config keys and public writer tool changes:

- register `"workflow.prefer_subagent"` with values `on|off`, builtin default
  `off`, and session default scope;
- register `"workflow.prefer_mercenary"` with values `on|off|hide`, preserving
  builtin default `hide`;
- add `config.workflow_prefer_subagent`;
- add `config.workflow_prefer_mercenary`;
- remove `ws.lead.prefer_mercenary` immediately, with no alias and no migration;
- leave old `"prefer_mercenary"` stored values orphaned.

Verification should cover keyed access control, default resolution, config
show/list behavior, and schema projection for the new writer tools.

### Phase 2: Workflow manual auto-insertion and wrapper helper

Refactor `lead-workflow-manual` rendering:

- remove the `DelegationSection` prompt override point;
- keep `UserPreferenceSection`;
- when `"workflow.prefer_subagent" == "on"`, append
  `lead-prefer-subagent` every time the manual is rendered;
- render the appended playbook through the normal harness-aware renderer,
  prompt override lookup, and product-mode pass;
- wrap the appended body in
  `<playbook name="lead-prefer-subagent" title="Prefer Subagent">`;
- add a reusable helper for future code-side pragmatic playbook concatenation;
- do not add duplicate insertion detection.

Verification should prove Codex receives the Codex invocation guidance through
the builtin override, Claude does not receive Codex-specific guidance, and raw
file concatenation is not used.

### Phase 3: Lead-tune, catalog, docs, tests, and wsflow polish

Align the surrounding workflow surfaces:

- update `lead-tune` so delegation posture tuning routes to
  `"workflow.prefer_subagent"` instead of `DelegationSection`;
- update mercenary tuning guidance to use `"workflow.prefer_mercenary"` and
  `config.workflow_prefer_mercenary`;
- ensure `config.tuning` shows the new workflow knobs and no longer advertises
  `prompt.DelegationSection` when the marker is removed;
- update runtime manifests and the wsflow rsrc mirror;
- update MCP, workflow-skill, prompt-bundle, and runtime mental-model docs;
- add or update tests for full ws, wsflow/no-agent expectations, prompt override
  catalog behavior, render output, and removed tool visibility.

This phase should also include the ready-promotion spec updates noted in
`## Spec Impact` if they were not already completed before implementation.
