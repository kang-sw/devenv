---
title: "Delete the fork delegation construct; reshape lead-prefer-subagent to fresh-spawn + central authoring whitelist"
related:
  260605-research-ws-native-subagent-pivot: native-subagent direction; fork is the delegation substrate this removes
  260625-research-fork-posture-leak-system-guarantee: resolved by deletion — fork inherits the lead's deferral stance
  260629-research-fork-worker-persona-bleed: resolved by deletion — fork inherits lead identity, worse on Opus 4.8
sage-review-design: completed
sage-review-completeness: completed
completed: 2026-07-24
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
  `lead-goal-step` (a one-line pointer). No other playbook uses it. The primary
  edit site is `lead-prefer-subagent/SKILL.md` (~8-22).
- **Idiom precision (verified during design review).** Two spawn idioms must not
  be conflated: `spawn_agent(fork_context:true, ...)` / full-history is the
  **fork to delete**; `fork_turns: "none"` is the **fresh self-contained spawn to
  keep**. `native-spawn-binding.codex.md:7` uses `fork_turns:"none"` — it is the
  surviving fresh-spawn binding, **not** a deletion target.
- **Already retired:** the builtin override default
  `prompt.PreferSubagentInvocationGuidance.codex` returns an empty map today
  (`playbook_tools.go:458-467`); its fork-guidance seed was retired when the
  prefer-subagent body was inlined into SKILL.md. There is nothing live to delete
  there — the earlier "deletion is clean" survey overstated this one point.
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

Primary edit site is `lead-prefer-subagent/SKILL.md` (~8-22):

- **Rewrite the opening invariant, not just excise fork sentences.** SKILL.md:8's
  "no inline reads/edits/... cannot act inline" directly contradicts the new
  lead-inline authoring whitelist; this is a *semantic reversal* of the skill's
  core posture, so the opening invariant is rewritten to admit the
  context-holder-authors carve-out.
- Delete the `fork_context:true` / full-history fork prose and the
  fork-availability posture gate. Reshape to: fresh spawn by default;
  authoring/mutation of durable artifacts stays with the context-holder session
  per a central whitelist.
- **Do not touch `native-spawn-binding.codex.md:7`** — its `fork_turns:"none"` is
  the fresh-spawn binding to keep. **No override to delete** — the
  `PreferSubagentInvocationGuidance.codex` default is already an empty map.
- Drop the `lead-goal-step:124` fork pointer (leave the unrelated git "fork
  point" term at `:90`).
- **Tests:** remove the `fork_context:true` assertions
  (`prefer_mercenary_phase2_test.go:318,385`; `playbook_tools_test.go:832`);
  **keep** `playbook_tools_test.go:592,605` which pin the surviving
  `fork_turns:"none"` fresh-spawn binding.
- Note in `260625` / `260629` that they are resolved-by-deletion.

**Acceptance check:** the Go test suite passes with the updated string
assertions, and a `fork` scan over the rendered `lead-prefer-subagent` SKILL.md,
the codex `native-spawn-binding` source, and the `PreferSubagentInvocationGuidance.codex`
builtin default returns zero remaining fork-delegation references (excluding
unrelated git "fork point" senses). The `lead-goal-step` pointer no longer names
fork.

### Result (d6669070) - 2026-07-24

Deleted the fork delegation construct and reshaped `lead-prefer-subagent` to two
clean poles (fresh spawn + context-holder carve-out); single-phase ticket
complete. Delegated implementer + survey plan + partitioned review + one
fix-relay; landed on `impl/fork-removal-prefer-subagent`, merged into the goal
staging branch.

- Opening invariant reversed (SKILL.md:8): "cannot act inline" → delegate every
  payload to a fresh subagent by default, sole carve-out being durable-artifact
  (ticket/spec) authoring staying with the context-holder session (lead-inline
  or the delegated subagent's own continuing session), never a fresh
  summary-only spawn — a central whitelist overlay owned by the skill, not
  interwoven per-skill. Fork clauses (fresh-vs-fork rule, fork-failure
  re-dispatch, fork definition, `spawn_agent(fork_context:true,...)` guidance,
  fork-availability posture gate) all deleted.
- Kept intact as designed: `native-spawn-binding.codex.md`'s `fork_turns:"none"`
  fresh-spawn idiom (verified untouched); `builtinPromptOverrideDefaults()`
  already returned an empty map (no Go change). Dropped-fork-prompt-authoring
  framing (old SKILL.md:22 anti-persona-bleed wording) removed as a plan-scoped
  judgment call — a fresh spawn inherits no identity, so it has no fresh-spawn
  equivalent; the generic return-format contract was preserved.
- Both edited skills are substitution-mirrored: edited canonical only,
  regenerated the `agents-plugin-wsflow` mirror + skills manifest via the
  env-gated `-count=1` regen tests; drift guards green.
- Tests: removed the 3 `fork_context:true` assertions
  (`prefer_mercenary_phase2_test.go` ×2, `playbook_tools_test.go` ×1) plus stale
  `agent_type` assertions; kept the `fork_turns:"none"` assertions. Full
  `go test ./...` and the wsflow python suite (9) pass; acceptance `fork` scan
  returns zero delegation references.
- Review: correctness / fit / test all clean; two minors fixed in `44541e6d`
  (carve-out wording precision; frontmatter `description` no longer literally
  contradicts the carve-out).
- Doc closeout: implementation corrected three stale sites describing the retired
  `PreferSubagentInvocationGuidance.codex` fork-seeding as live (`mcp-tools.md`
  `{#260619-delegation-section-override-point}`, mental-model `prompt-bundle.md`
  + `workflow-skills.md`). Doc-pre-pass caught one more those missed — the
  `workflow-skills` spec `{#260505-workflow-primitive-reference}` "literal Codex
  `spawn_agent` fallback wording" claim — corrected it and added anchor
  `{#260724-prefer-subagent-fresh-spawn-delegation-posture}` (`6f552285`).
- `260625` / `260629` appended resolved-by-deletion notes. Commit range
  `65bdaf9b`(plan)`..HEAD`.

## Spec Impact

- Target spec area: `workflow-skills` (the `lead-prefer-subagent` delegation
  posture and fresh-vs-fork routing description). Also check whether
  `prompt-bundle` / `mcp-tools` still describe the `PreferSubagentInvocationGuidance.codex`
  override as seeding fork guidance and correct it if stale — the runtime default
  is already an empty map.
- Expected caller-visible change: the documented delegation posture drops the
  fork path; delegates are fresh spawns, with a central authoring/mutation
  whitelist retaining that class in the context-holder session.
- Contract-first spec: no — this simplifies an existing documented behavior; the
  precise reshaped prose will be settled during implementation and the spec update
  is a closeout reflecting the removed path rather than a new contract to stabilize
  in advance.
