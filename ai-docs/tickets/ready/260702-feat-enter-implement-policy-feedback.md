---
title: enter_implement silently ignores caller policy fields outside their applicability window
sage-review: completed
---

# enter_implement silently ignores caller policy fields outside their applicability window

## Context

Found during a v0.31.1 dogfooding pass. Called `enter_implement` with
`policy.branch.merge_target: "master"` while on branch
`test/wsflow-smoke`. The returned verdict derived the merge target from the
observed current branch instead of honoring the policy value. Per the tool's
own schema this is correct behavior — `policy.branch.merge_target` only
applies when the caller is already on an `implement/*` branch — but the
response gave no indication that the supplied policy field was seen and
deliberately ignored. A caller who does not have the schema's fine print
memorized reasonably expects either the value to apply or an explanation of
why it didn't.

## Suggestion

When a policy field is present in the input but does not apply given current
state, add a one-line note to the verdict output, e.g.: "merge_target policy
ignored (not on an implement/* branch); derived from current branch." This
keeps the correct behavior but closes the feedback gap.

## Spec Impact

Target: `ai-docs/spec/mcp-tools.md`. Caller-visible change: `enter_implement`
verdict includes a one-line note when a caller-supplied policy field was
outside its applicability window and was ignored. Contract-first spec: no.
