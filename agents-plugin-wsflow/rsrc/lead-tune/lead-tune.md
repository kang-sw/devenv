---
kind: print
---

# Workflow Tuning

Topic: tune how the ws workflow runs to the user's stated preference.

## Invariants

Scope
- Tune only through config tools; never edit shipped rsrc playbook text to change behavior.
- Confirm the exact change — every field that applies (target, harness, scope, text) — with the user before any write.
- Tuning tools are lead-only and require the lead `session_key`; a delegate or leaf key cannot tune.

Surface
- Treat the `config.prompt` listing as the only source of valid override-point ids; never invent a `pointId`.
- Name any tuning request that does not map to a handler below as not yet supported.

Storage
- `harness` is `claude`, `codex`, or `*` (all hosts); use `*` unless the user names one host.
- `scope` is `session` (this worktree), `project` (this repo, the default), or `global` (every repo); confirm scope before a project or global write.

## On: invoke

1. Call `{{.McpNamespace}}/config.prompt(session_key: <lead key>)` to list override-points, their descriptions, and any current overrides.
2. Apply `judge: tune-target` to route the request to one knob.
3. Run that knob's handler; if the target is unclear, show the step-1 knobs and ask which to tune.

## On: tune prompt override

1. Map the request to a `pointId` from the `config.prompt()` listing; if no listed point matches, show the list and ask.
2. Draft the override text with the user. State the model: a stored override replaces that point's seed block for the matching `(pointId, harness)`; a point shipped with an empty seed is a pure extension slot, so its override only appends.
3. Confirm `(pointId, harness, scope, text)` per the Tuning Proposal template.
4. Call `{{.McpNamespace}}/config.prompt.set(session_key: <lead key>, pointId, harness, prompt, scope)`.
5. Report the stored `pointId`/`harness`/`scope`; note it applies at the next playbook render, not retroactively.

Worked example — making the lead delegate less: override the `DelegationSection` point with the user's posture text.

<!-- ws:full-only:start -->
## On: tune delegation mode

1. `prefer_mercenary` is a session-scope desired-state toggle: when on, implementer and reviewer roles prefer the mercenary spawn idiom over native subagents — distinct from the `DelegationSection` prompt override, which tunes delegation posture text.
2. Set it with `{{.McpNamespace}}/ws.lead.prefer_mercenary`; do not reimplement its set path here.
3. Report the new state and that it is session-scoped.

## On: tune model tier

1. `config.agents_tier` maps a capability tier (`small`/`medium`/`large`/`xlarge`) to a backend, model, and effort for a harness; `light`/`core`/`deep` and `haiku`/`sonnet`/`opus` are accepted as read-compat synonyms.
2. Set it with `{{.McpNamespace}}/config.agents_tier`; do not reimplement its set path here.
3. Report the tier and its resolved backend/model.
<!-- ws:full-only:end -->

## On: unsupported axis

1. State that the request is not a supported tuning knob today.
2. If it is per-role tier tuning (a `(role) -> tier` override), point to research ticket `260611-research-ws-per-role-delegation-tuning-config`.
3. Do not fabricate a tool for an unsupported knob.

## Judgments

### judge: tune-target
- Prompt wording, a named manual section, or "delegate more/less"/posture -> prompt override (`DelegationSection` for delegation posture).
<!-- ws:full-only:start -->
- A preference for mercenary or persistent agents over native spawning -> delegation mode (`prefer_mercenary`).
- A model, tier, or "cheaper/stronger model" preference -> model tier (`config.agents_tier`).
<!-- ws:full-only:end -->
- Anything else -> unsupported axis.

### judge: proactive-propose
Propose a tune without being asked when the user states a standing preference about how the workflow runs (for example "you delegate too much"), as opposed to a one-off instruction for the current task. Name the knob and the concrete change, then write only after confirmation.

Does NOT fire for a one-off task instruction or a question a direct answer resolves.

## Templates

### Tuning Proposal

```text
knob:    <the knob being tuned>
target:  <pointId | toggle | alias>
harness: <claude | codex | * | n/a>
scope:   <session | project | global | n/a>
change:  <new text or state>
```

## Doctrine

This skill optimizes for the lead's context window: workflow-tuning guidance
lives in this on-demand entry point, not the always-on `lead-workflow-manual`, so
routing attention for general tasks stays cheap. The user owns their workflow —
confirm the concrete change before writing it. When ambiguous, list the knobs and
ask.
