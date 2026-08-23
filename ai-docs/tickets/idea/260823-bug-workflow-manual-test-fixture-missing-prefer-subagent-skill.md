---
title: "workflow_manual render resolves appended posture off an exe-relative skills root under test binaries"
---

# workflow_manual render resolves appended posture off an exe-relative skills root

## Status

Test symptom RESOLVED (fix (a), commit `4598507b`). This ticket now tracks only
the latent architectural seam (b) below. Original framing ("missing fixture")
was wrong; corrected root cause recorded here.

## Original symptom

Six tests in `agents-plugin-tool/` failed with an identical error:

```
load appended lead-prefer-subagent: .../skills/lead-prefer-subagent/SKILL.md: no such file or directory
```

- `internal/mcp`: `TestWorkflowManualFreshMode`,
  `TestWorkflowManualFreshModeWithRoot`, `TestWorkflowManualContinueMode`,
  `TestWorkflowManualTodoInstructionPreview`,
  `TestWorkflowStateReturnsSessionStateOnly`.
- `cmd/ws-mcp`: `TestCallCommandColdStartWorkflowManualMintsLeadKey`.

Discovered while implementing `260823-feat-notes-postit-discipline` (its
`TestWorkflowStateReturnsSessionStateOnly` coverage was masked by this).

## Corrected root cause

Two independent facts combined:

1. **Config leak.** The six tests did not pin `WS_CONFIG_HOME`, so they read the
   dev machine's real `~/.ws/config.json`, where `workflow.prefer_subagent=on`.
   That turned on the `lead-prefer-subagent` posture append inside
   `playbook.print(name: "lead-workflow-manual")` — which the tests never
   intended to exercise.
2. **Exe-relative seam mismatch (the latent bug (b)).** The append branch in
   `internal/mcp/playbook_tools.go` (`printPlaybook`, ~L888-909) resolves the
   appended posture body through `wsrsrc.ResolveSkillsRoot()` +
   `wsrsrc.LoadSkillBody`, which key off an **exe-relative** skills root — NOT
   the `rsrcRoot` seam the main body loads through (`wsrsrc.Load(rsrcRoot, ...)`
   at ~L740). Under `go test` the exe is a temp test binary with no adjacent
   `skills/` tree, so the append hard-errors even though the seam-pinned main
   render path is fine.

So the failure was environment-dependent (machine config `on`) AND rode a real
seam inconsistency: the manual's main body honors the rsrcRoot seam but its
appended-posture branch bypasses it.

## Fix (a) — landed

`4598507b test(mcp): pin WS_CONFIG_HOME in workflow_manual/state tests for
hermeticity`. Pinned `WS_CONFIG_HOME` to a per-test temp dir in all six tests
(5 in `session_state_test.go`, 1 in `cmd/ws-mcp/main_test.go` subprocess env),
matching the existing sibling convention. `go test ./...` is now green
regardless of the machine's `workflow.prefer_subagent` global setting. Test
files only; no production change.

## Remaining open scope — latent seam (b)

`printPlaybook`'s append branch should resolve the appended posture body through
the same `rsrcRoot` seam the main body uses, instead of exe-relative
`ResolveSkillsRoot()`. Two directions:

- **Thread the seam:** have the append branch load through `wsrsrc.Load(rsrcRoot,
  ...)` (or an equivalent seam-aware loader) so a pinned rsrc root governs both
  the main body and the append uniformly.
- **Graceful degrade:** if the appended posture body cannot be loaded, skip the
  append with a warning rather than hard-erroring — a missing optional posture
  arguably should not fail the whole manual render.

Determine whether production (installed plugin) can hit the exe-relative path
with an absent skill file; if so this is a real robustness bug, not test-only.
This is an AGENTS.md "ask first" change (observable render behavior / seam
semantics) — do not land without sign-off.

## Verification when (b) is addressed

- `printPlaybook` append and main body resolve from the same seam under a pinned
  `rsrcRoot`; a test that pins the rsrc root but leaves the exe with no adjacent
  `skills/` tree still renders the appended posture.
- `cd agents-plugin-tool && go test ./...` stays green.
