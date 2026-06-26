---
kind: print
---

# Workflow Tuning

Topic: tune how the {{.SkillNamespace}} workflow runs to the user's stated preference.

## Invariants

Scope
- Tune only through catalog writer tools; never edit shipped rsrc playbook text to change behavior.
- Confirm the exact change — knob, writer, selector fields, and new value/text — with the user before any write.
- Tuning tools are lead-only and require the lead `session_key`; a delegate or leaf key cannot tune.

Surface
- Treat `config.tuning` as the source of supported knob ids, writer tools, field options, and current values.
- Treat prompt override-point ids as valid only when they appear as `prompt.<pointId>` knobs in `config.tuning`.
- State that any tuning request that does not map to one of this playbook's handlers is not yet supported.

Storage
- For prompt overrides, prefer the catalog's cross-harness selector unless the user names one harness.
- For global-only workflow preferences, use the writer's lead `session_key` only as authority.
- Confirm storage scope before any project or global write.

## On: invoke

1. Call `{{.McpNamespace}}/config.tuning(session_key: <lead key>)` to load supported knobs, writer tools, field options, and current values.
2. If the user states a standing workflow preference but does not explicitly ask to tune, apply `judge: proactive-propose` before selecting a handler.
3. Apply `judge: tune-target` to route the request to one catalog knob.
4. Follow that knob's handler using only catalog-provided writer and field metadata, including required drafting and confirmation before writing.
5. If the target is unclear, show the catalog knob ids with descriptions, ask which to tune, and resume the selected handler after the user answers.

## On: tune prompt override

1. Map the request to a `prompt.<pointId>` knob from the `config.tuning()` catalog; if no listed point matches, show prompt knobs and ask.
2. Draft or restate the override text for user approval, proposing concise text when the user's desired wording is clear. State the model: a stored override replaces that point's seed block for the matching `(pointId, harness)`; a point shipped with an empty seed contributes new text at that point rather than replacing shipped guidance.
3. Choose `harness` and `scope` from the catalog selector fields when present; use `n/a` for selectors the catalog does not expose.
4. Confirm `(knob, writer, harness, scope, text)` per the Tuning Proposal template.
5. Call the catalog writer tool with the knob's fixed arguments, `session_key`, selected selector fields, and the override text in the catalog-specified prompt/text field.
6. Report the stored knob/harness/scope; note it applies at the next playbook render, not retroactively.

Examples:
- Standing communication preferences: map to `prompt.UserPreferenceSection`, draft preference text, confirm the Tuning Proposal, write through the catalog writer, then report the knob and scope changed.

## On: tune subagent posture

1. Map the request to the `"workflow.prefer_subagent"` catalog knob.
2. Choose the new state from the catalog value field.
3. Confirm the Tuning Proposal with the selected value.
4. Call `config.workflow_prefer_subagent` with `session_key` and the selected value.
5. Report the global state and that it applies to the next workflow-manual load.

<!-- ws:full-only:start -->
## On: tune delegation mode

1. Map the request to the `"workflow.prefer_mercenary"` catalog knob.
2. Choose the new state from the catalog value field.
3. Confirm the Tuning Proposal with the selected value.
4. Call `config.workflow_prefer_mercenary` with `session_key` and the selected value.
5. Report the global state and that it controls both mercenary visibility and default render guidance.

## On: tune model tier

1. Map the request to the `agents.tier` catalog knob.
2. Choose the required tier field and any applicable optional `harness`, `backend`, `model`, and `effort` fields from the catalog metadata.
3. Confirm the Tuning Proposal with the selected fields.
4. Call the catalog writer tool with `session_key` and the selected fields.
5. Report the tier and, when returned by the writer/catalog, its resolved backend/model.
<!-- ws:full-only:end -->

## On: unsupported axis

1. State that the request is not a supported tuning knob today.
2. If it is per-role tier tuning (a `(role) -> tier` override), point to research ticket `260611-research-ws-per-role-delegation-tuning-config`.
3. Do not fabricate a tool for an unsupported knob.

## Judgments

### judge: tune-target
- User standing preferences, communication style, language, terminology, or wording conventions -> prompt override (`UserPreferenceSection`).
- Prompt wording or a named manual section -> prompt override for that named override point.
- "delegate more/less" or strict subagent posture -> workflow preference (`"workflow.prefer_subagent"`).
<!-- ws:full-only:start -->
- A preference for mercenary delegation mode, including persistent agents where supported -> workflow preference (`"workflow.prefer_mercenary"`).
- A model, tier, or "cheaper/stronger model" preference -> model tier (`agents.tier`).
<!-- ws:full-only:end -->
- Anything else -> unsupported axis.

### judge: proactive-propose
Propose a tune without being asked when the user states a standing preference about how the workflow runs (for example "you delegate too much"), as opposed to a one-off instruction for the current task. Name the knob and the concrete change, then write only after confirmation; if declined, make no change and report that tuning was not updated.

Do not propose tuning for a one-off task instruction or for a question that only needs a direct answer.

## Templates

### Tuning Proposal

```text
knob:      <catalog knob id>
writer:    <catalog writer tool>
selectors: <selected catalog selector fields, or n/a>
scope:     <selected catalog storage scope, or n/a>
change:    <new prompt text or catalog value>
```

## Doctrine

This skill optimizes for the lead's context window: workflow-tuning guidance
lives in this on-demand entry point, not the always-on `lead-workflow-manual`, so
routing attention for general tasks stays cheap. The user owns their workflow —
confirm the concrete change before writing it. When ambiguous, list the knobs and
ask.
