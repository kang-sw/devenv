---
kind: print
---

# Workflow Tuning

Topic: tune how the {{.SkillNamespace}} workflow runs to the user's stated preference.

## Invariants

Scope
- Tune only through catalog writer tools, or, for a team-wide setting, through an ordinary commit of the repo-scope file `config.list` names; never edit shipped rsrc playbook text to change behavior.
- Confirm the exact change — knob, writer, selector fields, storage scope, and new value/text — with the user before any write.
- Write only a value the user chose explicitly. A request that names no value gets the knob explanation instead of an inferred change.
- Tuning tools are lead-only and require the lead `session_key`; a delegate or leaf key cannot tune.

Surface
- Treat `config.list` as the source of supported knob ids, the `config.tune` write contract, field options, current values and scopes, what each knob's values do and cost (its description), whether the repo scope applies to it (`repo_scope`) and under which key (`repo_key`), and the repo-scope file's path and shape.
- Name values exactly as `config.list` accepts them; gloss `off`/`ask`/`auto` as skip/recommend/require.
- Treat prompt override-point ids as valid only when they appear as `prompt.<pointId>` knobs in `config.list`.
- Tune an `adapter_setting` knob (a setting the harness adapter running this session declares) as a scalar knob. Such knobs are listed only in sessions of the adapter that declares them; a request for another harness's adapter setting is tuned from a lead on that harness, so say so instead of writing it.

Storage
- For prompt overrides, choose the catalog option that applies across all harnesses unless the user names one harness.
- For global-only workflow preferences, use the writer's lead `session_key` only as authority.

## On: invoke

1. Call `{{.McpNamespace}}/config.list(session_key: <lead key>)` to load supported knobs, the `config.tune` write contract, field options, current values, and repo-scope information.
2. If the user states a standing workflow preference but does not explicitly ask to tune, apply `judge: proactive-propose` before selecting a handler.
3. Apply `judge: tune-target` to route the request.
4. Follow the selected handler using only catalog-provided writer and field metadata.

## On: explain knobs

1. Identify the catalog knobs the request bears on.
2. For each, explain in the user's language what it controls, each accepted value and what it does, what lowering it loses, and its cost hint, translated from its `config.list` description; then its current value and the scope that value comes from.
3. Explain the scopes the user can choose:
   - session: this work stream only; gone when it ends.
   - project: this project on this machine; persists.
   - repo: the whole team, through the committed repo-scope file; only for a knob whose `repo_scope` is true.
   - global: all of the user's projects. A global-only knob takes only this scope; say so.
   When the user names no scope, suggest session for a one-off request and project for a stated standing preference.
4. Ask which values to set. Run the matching handler for each value the user chooses.

## On: tune scalar knob

1. Take the knob and the value the user chose from its catalog `value` field.
2. Choose the scope the user named; otherwise suggest one by the scope guidance in `On: explain knobs` and confirm it. A repo-scope choice goes to `On: commit repo-scope setting`.
3. Confirm the Tuning Proposal.
4. Call `config.tune` with the knob's id as `key`, `session_key`, the selected scope when the catalog exposes a scope selector, and the value.
5. Report the stored value and scope.

## On: commit repo-scope setting

1. Confirm the knob's `repo_scope` is true; otherwise say it cannot be set team-wide and offer project or global scope.
2. Draft the edit to the repo-scope file at the `config.list` path in its listed shape: add or change only the knob's `repo_key` entry, choosing one listed harness bucket when the key offers several, and keep every existing entry; create the file when it does not exist.
3. If the knob's current value comes from session or project scope, say that value keeps winning on this machine until it is reset.
4. Confirm the Repo-Scope Proposal.
5. Write the file and propose an ordinary commit of it.

## On: tune prompt override

1. Map the request to a `prompt.<pointId>` knob from the `config.list()` catalog; if no listed point matches, show prompt knobs and ask.
2. Draft or restate the override text for user approval, proposing concise text when the user's desired wording is clear. State the model: a stored override replaces that point's seed block for the matching `(pointId, harness)`; a point shipped with an empty seed contributes new text at that point rather than replacing shipped guidance.
3. Choose all exposed selector fields, including `harness` or `scope` when present; use `n/a` for selector fields the catalog does not expose. A repo-scope choice goes to `On: commit repo-scope setting` with the override text as the value.
4. Confirm `(knob, writer, harness, scope, text)` per the Tuning Proposal template.
5. Call `config.tune` with the knob's id as `key`, `session_key`, the selected selector fields, and the override text as `value`.
6. Report the stored knob/harness/scope; note it applies at the next playbook render, not retroactively.

## On: tune model tier

1. Map the request to the `agents.tier` catalog knob.
2. Choose the required tier field and any applicable optional `harness`, `backend`, `model`, and `effort` fields from the catalog metadata. When the user wants a selected project or global scope to inherit its mapping again, offer reset; reset removes only that scope's selected `(tier, harness)` alias. An empty or `none` effort clears only effort and does not reset the alias.
3. Confirm the Tuning Proposal with the selected fields.
4. For a write, call `{{.McpNamespace}}/config.tune` with `key` set to `agents.tier`, `session_key`, and the selected fields (`tier` plus optional `backend`/`model`/`effort`) as the `value` object. For reset, call `{{.McpNamespace}}/config.tune` with `reset: true` and `value` containing only `tier`, retaining the selected `harness` and `scope`.
5. Relay any warnings returned by the write or reset. Report the tier and, when returned by the writer/catalog, its resolved backend/model.

## On: unsupported axis

1. State that the request is not a supported tuning knob today.

## Judgments

### judge: tune-target
- The language the user wants responses in (for example "answer me in Korean") -> scalar knob `workflow.lang`, with the language name as the value.
- User standing preferences, communication style, terminology, or wording conventions -> prompt override (`UserPreferenceSection`).
- Prompt wording or a named manual section -> prompt override for that named override point.
- A named value for a named scalar knob (for example `"workflow.prefer_subagent"`, a Sage review stage, `review_phase`, `bootstrap_alarm`, or an `adapter_setting` knob) -> scalar knob; for the repo scope -> commit repo-scope setting.
- A named tier with a named model or backend -> model tier (`agents.tier`).
- How heavy, slow, costly, or thorough the workflow is, a request spanning several knobs, or one with no clear knob or value -> explain knobs.
- A request no catalog knob bears on -> unsupported axis.

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

### Repo-Scope Proposal

```text
knob:   <catalog knob id>
file:   <repo-scope path from config.list>
change: <key>: <old value or absent> -> <new value>
commit: <proposed commit subject>
```
