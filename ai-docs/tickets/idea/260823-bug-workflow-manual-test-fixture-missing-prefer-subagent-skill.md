---
title: "workflow_manual tests fail on missing lead-prefer-subagent SKILL.md fixture"
---

# workflow_manual tests fail on missing lead-prefer-subagent SKILL.md fixture

## Problem

Six tests in `agents-plugin-tool/` fail with an identical error:

```
load appended lead-prefer-subagent: .../skills/lead-prefer-subagent/SKILL.md: no such file or directory
```

- `internal/mcp`: `TestWorkflowManualFreshMode`,
  `TestWorkflowManualFreshModeWithRoot`, `TestWorkflowManualContinueMode`,
  `TestWorkflowManualTodoInstructionPreview`,
  `TestWorkflowStateReturnsSessionStateOnly`.
- `cmd/ws-mcp`: `TestCallCommandColdStartWorkflowManualMintsLeadKey`.

Confirmed pre-existing and unrelated to any single feature branch: the same six
failures reproduce on a clean base checkout in an isolated worktree. Discovered
while implementing `260823-feat-notes-postit-discipline` (its
`TestWorkflowStateReturnsSessionStateOnly` coverage was masked by this).

## Suspected cause

`playbook.print(name: "lead-workflow-manual")` appends the `lead-prefer-subagent`
posture when the `workflow.prefer_subagent` global preference resolves `on`
(see mcp-runtime `{#260610-mercenary-delegation-surface}` neighborhood). The
render path resolves the appended playbook from a skills tree that, in the test
rsrc/fixture environment, does not contain
`skills/lead-prefer-subagent/SKILL.md`. So the failures are environment/config
dependent: they appear when the global config on the dev machine has
`prefer_subagent=on` (as in the current dogfood setup) and the test rsrc bundle
lacks that skill file.

## Open questions / directions

- Should the workflow-manual render degrade gracefully (skip the append with a
  warning) when the `lead-prefer-subagent` body cannot be loaded, rather than
  hard-erroring? A missing optional posture append arguably should not fail the
  whole manual render.
- Or is the real fix that the test rsrc/fixture tree must bundle
  `lead-prefer-subagent/SKILL.md` (test-fixture completeness), and/or that these
  tests should pin `prefer_subagent` to a known scope instead of reading ambient
  global config?
- Determine whether production (installed plugin) is affected or whether this is
  strictly a test-fixture gap. If production can hit it when the skill file is
  absent, this is a real robustness bug, not just a test issue.

## Verification when addressed

- `cd agents-plugin-tool && go test ./...` is green regardless of the machine's
  `workflow.prefer_subagent` global setting.
