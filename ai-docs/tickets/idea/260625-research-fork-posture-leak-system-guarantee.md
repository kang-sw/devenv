---
title: fork posture-leak — prefer-subagent forks echo deferral instead of executing; system-level guarantee needed
related-mental-model:
  - workflow-skills
related:
  260605-research-ws-native-subagent-pivot: subagent-pivot direction; fork/native-subagent delegation is the substrate this affects
---

# fork posture-leak — prefer-subagent forks echo deferral instead of executing; system-level guarantee needed

## Background

Found while dogfooding `lead-prefer-subagent` (maximum-delegation posture)
during `260625-feat-ws-session-state-machine` Phase 1. Under prefer-subagent the
lead delegates all project work to forked subagents that inherit the current
conversation context. Repeatedly, a forked subagent returned **0 tool calls**,
echoing the lead's own deferral narrative ("a fork is running / I'll wait")
instead of executing its directive — the fork inherited the lead's
delegate-and-wait stance and applied it to itself.

Key observations:

- Reproduced multiple times under Opus 4.8. The same override prompts were
  followed reliably under Sonnet 4.6, so this reads as a model-specific
  instruction-following gap, not a prompt-content defect.
- The leak worsens as the session accumulates deferral phrasing: the more the
  lead transcript talks about routing/waiting, the more likely an inheriting
  fork echoes it.
- **Fresh spawns are immune** — a clean-context agent has no inherited deferral
  narrative to echo. The partitioned review in this same session used fresh
  spawns specifically to sidestep the leak.

## Current Mitigation (prompt-strength only, non-deterministic)

The `lead-prefer-subagent` playbook now prescribes a strongly-encoded
fork-awareness header on every fork prompt:

- a long separator token (`====...`),
- an UPPERCASE up-front declaration that the recipient is a forked executor and
  the prefer-subagent posture is suspended for it,
- an instruction that its first action MUST be a real tool call,
- a "a prior fork failed by echoing — do NOT do that" line,
- the mandatory trailing line `**You are a forked agent. Execute all work
  directly — do not sub-delegate.**`

This reduces the failure rate but does not eliminate it. Prompt strength is a
non-deterministic lever: the same header sometimes works and sometimes does not,
and it degrades as the session grows.

## Research Question

Should the runtime provide a **system-level posture-suspension guarantee** for
forked agents rather than relying on prompt strength?

Directions to evaluate:

- An injected fork-role marker the playbook render can key on, so a forked
  agent receives an authoritative "you are a leaf executor; delegation posture
  does not apply to you" frame from a trusted channel rather than from
  lead-authored prose the model may discount.
- Whether the fork boilerplate the harness already injects (the
  `<fork-boilerplate>` "you are a worker fork, execute one directive" block)
  can be strengthened or made authoritative enough to override an inherited
  deferral stance without per-prompt reinforcement.
- Whether prefer-subagent should prefer fresh spawns over forks for any work
  that does not strictly need inherited conversation context, given fresh
  spawns are immune — narrowing fork use to the genuinely context-dependent
  cases (authoring/mutation) where the leak risk is unavoidable.

Cross-reference `260605-research-ws-native-subagent-pivot`: the pivot makes
harness-native subagents (forks included) the sole delegation substrate, so a
reliable fork-execution guarantee is load-bearing for the whole direction, not
just for prefer-subagent.

## Resolved by deletion (260723-refactor-fork-removal-prefer-subagent)

`lead-prefer-subagent` no longer offers a fork delegation path at all —
`260723-refactor-fork-removal-prefer-subagent` Phase 1 deleted the
`spawn_agent(fork_context:true, ...)` construct and the fork/fresh-spawn
decision rule, reshaping the posture to fresh-spawn-by-default plus a narrow
central authoring/mutation whitelist for durable artifacts. With no fork
mechanism left to route through, the posture-leak this ticket investigates has
no surviving reproduction path; the third research direction listed above
("prefer fresh spawns over forks for any work that does not strictly need
inherited conversation context") is effectively the outcome taken, just
without a fork option remaining at all.
