---
title: ws:lead-tune umbrella workflow-tuning entry skill
parent: 260619-epic-ws-layered-config-prompt-tuning
spec:
  - 260505-lead-skill-namespace-surface
related:
  260619-feat-ws-config-prompt-tool-self-doc: prerequisite — the config.prompt.* data plane this skill drives (list + set)
  260619-feat-ws-prompt-override-marker-engine: prerequisite — the override-points (e.g. DelegationSection) this skill teaches users to tune
  260619-feat-ws-layered-config-scope-substrate: prerequisite — scope model the skill explains (session/project/global)
  260611-research-ws-per-role-delegation-tuning-config: future axis — model_alias/role_tier tuning can later slot into this umbrella
related-mental-model:
  - workflow-skills
  - prompt-bundle
---

# ws:lead-tune umbrella workflow-tuning entry skill

## Background

ws workflow behavior is becoming user-tunable (epic
`260619-epic-ws-layered-config-prompt-tuning`): prompt overrides
(`config.prompt.*`), the `prefer_mercenary` delegation toggle, and model tiers
(`config.agents_tier`) are all knobs. But the capability is undiscoverable — an
agent has no surface that fires when a user says "let's tune the
prompts/workflow" or "make the lead delegate more aggressively," and no manual
for how to reason about scope and harness.

The motivating requirement: an agent should be able to **proactively propose**
tuning when the user signals tuning intent, and a human/agent should find a
single guided entry point for all workflow knobs — without bloating the always-on
`lead-workflow-manual` (whose size taxes general-task routing attention).

## Decisions

These are confirmed in discussion (`lead-discuss`, 2026-06-19).

- **Dedicated entry skill, not workflow-manual prose.** Add `ws:lead-tune` as a
  user-invocable entry skill (the 13th `ws:lead-*`). Its **description is the
  runtime trigger surface** (mental model `workflow-skills` #260508), so the
  harness routes to it on tuning intent and the skill can propose tuning — at
  near-zero cost to general-task routing. `lead-workflow-manual` gets at most a
  one-line pointer (or none); the always-on manual is NOT where tuning guidance
  lives. Rejected: tuning manual inline in `lead-workflow-manual` (always-on
  bloat); tuning manual inline in `config.prompt()` output (bloats the data
  surface, weak trigger).
- **Three planes, kept separate.** (1) Data plane = `config.prompt.*` MCP tools
  (`260619-feat-ws-config-prompt-tool-self-doc`). (2) Procedure/manual plane =
  this skill's rsrc playbook — owns the tuning manual and when/how to propose.
  (3) Discovery plane = the skill description + minimal manual pointer.
  `config.prompt()` returns data + a pointer here; the manual lives here.
- **Umbrella scope, implemented to what exists.** The skill conceptually covers
  all workflow tuning. First build covers:
  - **Prompt overrides** (primary): drive `config.prompt()` to enumerate
    override-points, `config.prompt.set(pointId, harness, prompt, scope?)` to set;
    teach the `(pointId, harness)` axis, the seed/override/extension-slot model,
    and scope (`session/project/global`). `DelegationSection` (lead delegation
    posture) is the worked example.
  - **Delegation mode**: introduce/link `prefer_mercenary` (session-scope
    desired-state toggle) — do not reimplement its set path.
  - **Model tiers**: introduce/link `config.agents_tier` — do not reimplement.
  - Leave a **slot for future axes** (`config.model_alias`, `config.role_tier` —
    the `260611` research axis) without building them now.
- **Naming follows the `lead-*` entry-skill convention** (`ws:lead-tune`,
  directory `agents-plugin/skills/lead-tune/`, playbook
  `agents-plugin/rsrc/lead-tune/lead-tune.md`); user-facing framing is "workflow
  tuning."
- **Not blocking the data plane.** 3a (`config.prompt.*`) ships independently;
  this skill wraps it.

## Phases

### Phase 1: ws:lead-tune entry skill + tuning playbook

Add the entry skill across both required surfaces (per `workflow-skills`
change-recipe): thin-shim `agents-plugin/skills/lead-tune/SKILL.md` (frontmatter +
H1 + single `ws/playbook.print(name: "lead-tune")` execute line) and the full
`kind:print` procedure body `agents-plugin/rsrc/lead-tune/lead-tune.md`. The
playbook is the umbrella tuning manual: prompt-overrides section primary (drives
`config.prompt()`/`config.prompt.set`, teaches `(pointId, harness)` + scope +
seed/override/extension model with `DelegationSection` as the worked example),
plus introduce/link sections for `prefer_mercenary` and `config.agents_tier`, plus
a clearly-marked slot for future `model_alias`/`role_tier`. Add the ≤1-line
pointer in `lead-workflow-manual` (or none). Regenerate `manifest.json` and the
wsflow rsrc mirror. Apply the `lead-skill-authoring` invariant checklist to both
the SKILL.md and the playbook.

Depends on `260619-feat-ws-config-prompt-tool-self-doc` (the data plane it
drives) and `260619-feat-ws-prompt-override-marker-engine` (the points it
documents; done).

Verification: invoking `ws:lead-tune` renders the umbrella manual with the
prompt-override section driving the live `config.prompt.*` tools; the skill
description fires on tuning intent without capturing general work; wsflow
product-mode output stays clean; manifest + mirror tests pass.

## Spec Impact

Ready promotion (todo -> ready) must address the workflow-skills spec: a new
user-invocable entry skill is caller-visible workflow behavior. Add an entry to
`ai-docs/spec/workflow-skills.md` for the `ws:lead-tune` skill (its trigger
description and the umbrella surfaces it exposes). Contract-first spec: no — the
exact manual wording is refined during implementation; a closeout spec entry on
the entry-skill surface is sufficient.
