---
title: "prefer-subagent fork executor recursively delegates despite handoff boundary"
parent: 260605-epic-ws-playbook-factory-pivot
related:
  260626-bug-prefer-subagent-fork-executor-narration: same-failure-family
  260625-research-fork-posture-leak-system-guarantee: inherited-posture-risk
sage-review: skipped
completed: 2026-06-29
dropped: 2026-06-29
---

# prefer-subagent fork executor recursively delegates despite handoff boundary

## Background

During 2026-06-26 dogfooding, the lead dispatched a forked worker under
`ws:lead-prefer-subagent` with the explicit handoff boundary:

```text
You are the executing delegate for this task in the current workspace; this
handoff is the delegation boundary. Use tools directly and do not create another
fork.
```

The worker completed the requested ticket/documentation task, but its final
report stated that it used a separate `codex exec` delegate because
`spawn_agent(fork_context:true)` was not exposed inside its session. That is a
recursive-delegation escape: the worker respected the output goal but violated
the execution ownership boundary.

## Spec Impact

Target spec area: workflow-skills — lead-prefer-subagent delegate posture
Expected caller-visible change: Fork boundary wording explicitly forbids routing work through any secondary agent or AI-backed executor; lead failure-recovery text covers the secondary-executor escape case.
Contract-first spec: no

## Phases

### Phase 1: Strengthen fork boundary language in lead-prefer-subagent

Constraints:
- Edit `agents-plugin/rsrc/lead-prefer-subagent/lead-prefer-subagent.md` only.
- In the fork prompt template opening, forbid using any AI-backed execution harness or secondary executor, not just explicit fork/subagent spawn mechanisms. Add: a missing tool or capability is a blocker to report, not a reason to create a secondary executor.
- In the failure-recovery paragraph, extend the failed-fork definition to cover a fork that completes via a secondary executor (any process, tool, or agent created to do the work on the fork's behalf), not only a fork that reports delegation instructions.
- After editing rsrc, regen the wsflow rsrc mirror and verify `TestWsflowRsrcMirrorUpToDate` passes.

Verification:
- `go test ./internal/mcp/...` passes including `TestWsflowRsrcMirrorUpToDate`.
- Fresh read of the edited playbook confirms no ambiguity remains about secondary-executor delegation.


## Resolution (2026-06-29)

Extended failed-fork definition to cover secondary-executor escape. Updated fork prompt template opening to forbid AI-backed execution harnesses and name missing-tool as a blocker, not a delegation trigger. wsflow mirror regenerated.


## Resolution (2026-06-29)

Reverted implementation (511af6f~) — wording was too broad and had been through multiple polishing passes without a fresh explicit design. Dropped; reopen if the recursive-escape issue re-surfaces with a clear reproduction case.
