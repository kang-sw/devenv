# Brief: lead-sprint-episode-shell

## Intent

Redefine full ws `lead-sprint` as an episode-oriented workflow shell that coordinates discussion, exploration, narrow sprint-edit episodes, and normal workflow handoff without owning branch-based sprint implementation or final wrap-up.

## Scope Boundary

Implement Phase 1 of `260523-refactor-lead-sprint-episode-shell`. Replace full ws `lead-sprint` branch/wrap-up behavior with sprint-edit episode behavior. Update directly affected specs, mental models, index flow text, and wsflow sprint text so no current guidance describes the old full ws sprint branch model as active.

## Caller-Visible Contract

Users invoking `ws:lead-sprint` enter a session loop. The skill does not create `sprint/` branches and does not run a final wrap-up. Small interactive edits may enter `sprint-edit`; each edit commit carries a recoverable episode marker, then the skill asks whether to keep refining the current edit context, wrap it up, or shift direction. Wrapping up an episode runs documentation closure for that episode's marked commit range. Larger implementation routes to the normal workflow.

## Contract Instructions

Update `agents-plugin/skills/lead-sprint/SKILL.md` as the authoritative full ws skill surface. Keep `lead-sprint` responsible for routing and closure, not general implementation. Do not make `lead-implement` sprint-aware. Preserve host-neutral ws notation.

Update `agents-plugin-wsflow/skills/lead-sprint/SKILL.md` only as needed to prevent stale divergence: either mirror the new episode shell using wsflow notation or explicitly document why wsflow intentionally differs. Do not introduce forbidden full ws references into wsflow distributed skill text.

Update `ai-docs/spec/workflow-skills.md`, `ai-docs/mental-model/workflow-skills.md`, and `ai-docs/_index.md` so active guidance matches the new full ws sprint contract.

## Integration Test Instructions

Run the skill/static verification tests that cover ws and wsflow skill bundles:

- `python3 -m unittest discover agents-plugin/tests`
- `python3 -m unittest discover agents-plugin-wsflow/tests`

Also run `ws/spec_index.verify()` after spec edits.

## Implementation Strategy Decisions

Use a strict `judge: sprint-edit` boundary. Sprint-edit allows only one-context, lead-owned, small interactive edits. Public contracts, routing semantics, protocols, ticket phase completion, cross-module new patterns, plan/review allocation, and branch decisions route outside sprint-edit.

Use commit body markers for sprint-edit episodes:

```text
Sprint-Edit: <episode-slug>
Sprint-Edit-Context: <one-line context>
```

Use this English source prompt text, rendered by the model in the user's active language:

```text
[sprint] Should we keep refining <current edit context>, wrap it up here, or shift direction?
```

## Rejected Alternatives

Do not keep `sprint/*` branch semantics. Do not add sprint-awareness to `lead-implement`. Do not treat `lead-sprint` as a smaller `lead-implement` with weaker review or documentation rules. Do not keep one final branch wrap-up pass.

## Approach

- Rewrite full ws `lead-sprint` around episode routing and closure.
- Mirror or deliberately document wsflow sprint changes according to wsflow mirroring rules.
- Replace stale spec, mental-model, and index flow references.
- Verify no stale active full ws sprint branch/wrap-up references remain outside intentional historical material.

## Constraints

`lead-sprint` must stay usable as a session shell after each route. Episode wrap-up returns to the sprint loop. Full workflow handoff remains explicit. All authored docs are English.

## Out of scope

Do not solve the separate wsflow `lead-implement` mirroring gap. Do not remove historical references in archived or presentation material unless tests require it.

## Details

Interpret the post-edit question as:

- keep refining / continue: keep the current sprint-edit episode open;
- wrap it up / done / good: finish the episode and run docs for marked commits;
- shift direction / change focus: decide whether to finish the current episode before starting a new one.

## Verification Contract

The implementation is acceptable when the changed skill text is executable under pressure, active docs describe the new episode shell, wsflow text has no forbidden full ws references, tests pass, and the ticket records the result commit.

## References

- `ai-docs/tickets/ready/260523-refactor-lead-sprint-episode-shell.md` - binding implementation scope.
- `ai-docs/spec/workflow-skills.md` - caller-visible workflow behavior.
- `ai-docs/mental-model/workflow-skills.md` - workflow skill coupling and common mistakes.
- `ai-docs/ref/wsflow-mirroring.md` - wsflow mirror/divergence rules.
- `agents-plugin/skills/lead-skill-authoring/SKILL.md` - skill authoring invariants.
