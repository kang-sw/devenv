---
title: "Delete the fork delegation construct; reshape lead-prefer-subagent to fresh-spawn + central authoring whitelist"
related:
  260605-research-ws-native-subagent-pivot: native-subagent direction; fork is the delegation substrate this removes
  260625-research-fork-posture-leak-system-guarantee: resolved by deletion — fork inherits the lead's deferral stance
  260629-research-fork-worker-persona-bleed: resolved by deletion — fork inherits lead identity, worse on Opus 4.8
sage-review-design: required
sage-review-completeness: required
---

# Delete the fork delegation construct; reshape lead-prefer-subagent to fresh-spawn + central authoring whitelist

## Background

`lead-prefer-subagent` (maximum-delegation posture) routes conversation-dependent
work to a **fork** — a subagent that inherits the current conversation context.
Evidence shows fork is the least reliable delegation mechanism, and it fails
precisely at its one job:

- **posture-leak** (`260625`): a fork inherits the lead's deferral stance and
  returns 0 tool calls, echoing lead narration instead of executing.
- **persona-bleed** (`260629`): a fork inherits the lead's identity/voice and
  returns lead-voice narration instead of executing; **higher failure rate on
  Opus 4.8**, and forks are tier-locked to the parent model so it cannot be
  worked around by model choice.
- Two tickets (`260626-bug-prefer-subagent-fork-executor-narration`,
  `260626-bug-prefer-subagent-recursive-delegate-escape`) were **dropped** with
  the conclusion that Opus fork narration-vs-execution is model-level and
  resistant to prompt fixes — no actionable playbook change exists.
- **Detection gap**: a failed fork produces `tool_uses > 0` + lead narration, so
  it looks successful and can mislead the lead into a false-complete state.

Survey of the live tree confirms deletion is clean:

- **fork has zero Go implementation.** `fork_context` / `fork_turns` appear only
  as prompt text the host model is told to pass to its native `spawn_agent`, plus
  test string-assertions. Deletion touches prompt text + a few test assertions,
  not runtime code.
- Only two live skills mention fork: `lead-prefer-subagent` (the rule) and
  `lead-goal-step` (a one-line pointer). No other playbook uses it. The rendered
  sources are `lead-prefer-subagent/SKILL.md` (~12-22),
  `agents-plugin/rsrc/lead-workflow-manual/native-spawn-binding.codex.md`, and the
  builtin override default `prompt.PreferSubagentInvocationGuidance.codex`.
- The one substantive rule already routes standing-role authoring
  (implementer/reviewer) to fresh spawn unconditionally; fork remains only for a
  vague "other work" carve-out whose own failure-recovery clause already re-runs
  the same work as a fresh spawn. No use-site was found that genuinely needs
  context inheritance with no fresh/lead mapping.

## Decisions

- **Delete the fork construct entirely.** Do not retain fork behind a flag or
  narrow it; the reliability problem was already declared unfixable at prose level.
- **Two clean delegation poles remain**: fresh (stateless, artifact-forwarded)
  and lead-inline (full context). Fresh spawns are immune to posture-leak and
  persona-bleed.
- **Central authoring/mutation whitelist, owned by `lead-prefer-subagent` as an
  overlay.** The rule: *authoring/mutation of durable artifacts (tickets, specs)
  is performed by the session that already holds the authoritative context —
  never handed off to a fresh, context-less agent.* In practice: discussion in
  the lead → lead authors inline; research delegated to a subagent → that same
  subagent authors within its own session (not a fork, just continuation). The
  forbidden move is the lead spawning a fresh agent to author from a summary.
- **Overlay, not interwoven.** The whitelist lives centrally in
  `lead-prefer-subagent`; individual skills (e.g. `lead-write-ticket`) stay
  agnostic about who executes them. This preserves prefer-subagent as a posture
  that *sits on top of* the normal workflow rather than being wired into each skill.
- **Remove the posture-gated-on-fork-availability clause.** With fork gone,
  `lead-prefer-subagent` no longer needs to ask the user to suspend the posture
  when no fork mechanism exists.

### Rejected alternatives

- *Keep fork, add a per-skill carve-out for authoring* — rejected: adds an
  ambiguous branch (the "settled only in this conversation" test has no
  distinguishing example) and interweaves posture into skills.
- *Try-fork-then-fallback-to-lead on failure* — rejected: fork failure is not
  reliably detectable (looks successful), so the fallback would not fire.

## Phases

### Phase 1: Remove fork from prefer-subagent and reshape to fresh + central whitelist

Delete fork prose from `lead-prefer-subagent/SKILL.md`, the codex
`native-spawn-binding` source, and the builtin override default; drop the
`lead-goal-step` pointer. Rewrite the posture to: fresh spawn by default;
authoring/mutation of durable artifacts stays with the context-holder session per
a central whitelist; remove the fork-availability posture gate. Update the
test string-assertions that pin fork rendering. Note in `260625` / `260629` that
they are resolved-by-deletion.

## Spec Impact

- Target spec area: `workflow-skills` (the `lead-prefer-subagent` delegation
  posture and fresh-vs-fork routing description), plus the `prompt-bundle` /
  `mcp-tools` mention of the `PreferSubagentInvocationGuidance.codex` override
  default that seeds fork guidance.
- Expected caller-visible change: the documented delegation posture drops the
  fork path; delegates are fresh spawns, with a central authoring/mutation
  whitelist retaining that class in the context-holder session.
- Contract-first spec: no — this simplifies an existing documented behavior; the
  precise reshaped prose will be settled during implementation and the spec update
  is a closeout reflecting the removed path rather than a new contract to stabilize
  in advance.
