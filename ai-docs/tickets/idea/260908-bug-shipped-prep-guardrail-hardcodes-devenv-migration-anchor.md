---
title: "Shipped Prep guardrail and lead-discuss hardcode the devenv migration anchor"
related:
  260605-research-ws-native-subagent-pivot: the devenv-only anchor the shipped text names
  260908-feat-implement-skip-survey-for-localized-ticket-target: surfaced while deciding where anchor constraints go when the survey is skipped
---

# Shipped Prep guardrail and lead-discuss hardcode the devenv migration anchor

## Background

Observed 2026-09-08. The Prep todo instruction every `route.resolve_implement`
verdict emits is a Go string constant
(`agents-plugin-tool/internal/mcp/session_state.go`, `implementPrepInstruction`
guardrails): "read the 260605 migration anchor when target touches plugin
architecture, host-neutral migration, spawn-removal, or adapter boundaries".
It is pinned verbatim in `session_state_test.go`. The shipped
`lead-discuss` playbook likewise says "Architecture/migration/spawn-removal/
adapter-boundary topics -> read
`ai-docs/tickets/idea/260605-research-ws-native-subagent-pivot.md`". Both
ship to every downstream project through the plugin, and
`ai-docs/spec/workflow-skills.md` `{#260513-proceed-ticket-freshness-gate}`
and `{#260519-proceed-implementation-dispatch-precheck}` describe "the
native-subagent pivot anchor" as if it were ws behavior.

`260605` and its four topics are this repository's own migration concern.
A downstream project receives a Prep todo telling it to read a ticket it
does not have, and a `lead-discuss` rule pointing at a path that does not
exist there. The project-neutral hooks that carry binding project docs
already exist in the same guardrail sentence: mental-model lookup and
`infra.read("impl-playbook")`.

## Phases

### Phase 1: Project-declared binding anchor instead of a shipped constant

Replace the hardcoded anchor in the Go guardrail, `lead-discuss`, and the
two spec sentences with a project-declared binding-anchor hook: the
guardrail tells the lead to read the project's declared binding anchors
when the target touches their declared topics, and devenv declares
`260605` with its four topics as project memory. Decide where the
declaration lives: the `AGENTS.md` "Migration anchor" paragraph already
states it for humans and leads, and a `config.list` key would make it
runtime-readable by the Go instruction; pick one and say why the other
lost. Update the `session_state_test.go` pins, the wsflow mirror, and the
two spec anchors. Verification: a downstream-shaped fixture with no
declared anchor renders a Prep instruction that names no ticket stem, and
devenv's render still names `260605` with its topics.
