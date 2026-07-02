---
title: enter_implement silently ignores caller policy fields outside their applicability window
sage-review: completed
completed: 2026-07-02
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

## Phases

### Phase 1: Add applicability-window feedback note

Add a one-line warning to the `enter_implement` verdict when
`policy.branch.merge_target` is supplied but falls outside its applicability
window (caller is not already on an `implement/*` branch, so the branch
action resolves to `create` and the value is derived from the current branch
instead).

#### Result (2026-07-02)

Implemented in
`agents-plugin-tool/internal/mcp/implement_resolver.go`:
`resolveImplement` now appends a warning
(`policy.branch.merge_target %q ignored (not on an implement/* branch);
derived from current branch %q`) whenever `deriveImplementBranchPlan` returns
`Action == "create"` and the caller supplied a non-empty
`policy.branch.merge_target`. The note surfaces in both the JSON `warnings`
array and the canonical `raw` text output (existing `Warnings:` section),
following the same "ignored because ..." phrasing convention already used in
`proceed_resolver.go`. No change when already on an `implement/*` branch —
`continue`/`rename` paths honor the supplied merge target as before, so no
warning fires there.

Tests added to
`agents-plugin-tool/internal/mcp/implement_resolver_test.go`:
- `TestResolveImplementMergeTargetPolicyIgnoredOutsideImplementBranchWarns`
  reproduces the ticket's dogfooding repro (current branch
  `test/wsflow-smoke`, `policy.branch.merge_target: "master"`) and asserts
  the branch action is `create`, the merge target is derived from the
  current branch, and the warning appears in both `Warnings` and `Raw`.
- `TestResolveImplementMergeTargetPolicyHonoredOnImplementBranchNoWarning`
  asserts no such warning fires when already on `implement/*` and the
  policy value is honored.

Spec updated: `ai-docs/spec/mcp-tools.md`, `enter.implement` bullet, documents
the new one-line warning and its trigger condition.

Verification: `cd agents-plugin-tool && go build ./... && go test ./...
-count=1` — all 11 packages pass
(`cmd/ws-mcp`, `internal/execjob`, `internal/mcp`, `internal/textreader`,
`internal/wsagent`, `internal/wsconfig`, `internal/wsdoc`, `internal/wsgit`,
`internal/wskey`, `internal/wsrsrc`, `internal/wsstate`, `internal/wsstore`).
