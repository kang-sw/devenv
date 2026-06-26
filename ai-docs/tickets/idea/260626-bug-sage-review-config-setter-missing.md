---
title: "sage_review config has no lead-facing setter or tuning catalog knob"
parent: 260605-epic-ws-playbook-factory-pivot
related:
  260622-feat-sage-review-ticket-gate: introduced sage_review config substrate
  260626-feat-surface-sage-review-posture: depends on clear sage_review posture control
---

# sage_review config has no lead-facing setter or tuning catalog knob

## Problem

During dogfood, the user asked to set `sage_review` to `ask` through
`ws:lead-tune`. `config.show` exposes `sage_review` as a registered resolved
override, and ticket move/create behavior consults it, but `config.tuning` does
not list a `sage_review` knob and no lead-facing MCP setter is exposed for it.

The only working path was to edit the config JSON manually under the correct
`overrides` map. That is too implementation-specific for a workflow preference
that directly affects ticket promotion and sage-review routing.

## Evidence

- `config.show(session_key, format: json)` reports `sage_review` in
  `resolved_overrides`.
- `config.tuning(session_key, format: json)` lists prompt overrides,
  `workflow.prefer_subagent`, `workflow.prefer_mercenary`, and `agents.tier`,
  but no `sage_review` or `sage_review_*` writer.
- `ws-mcp config --help` exposes only `show` and `agents-tier`.
- A top-level `"sage_review": "ask"` config edit is ignored; the resolver reads
  scoped keys from `config.overrides["sage_review"]`.

## Direction

- Add a lead-facing writer for `sage_review` with accepted values
  `off|ask|auto`.
- Surface it through `config.tuning` so `ws:lead-tune` can route the request
  without manual file edits.
- Make scope explicit. `sage_review` defaults to project scope today, but users
  may reasonably request global defaults; the writer/catalog should expose the
  supported scope choices or deliberately reject unsupported ones with a clear
  message.
- Consider including the adjacent `sage_review_design_tier`,
  `sage_review_completeness`, and `sage_review_completeness_tier` keys in the
  same tuning family if that keeps the review posture surface coherent.

## Open questions

- Should `sage_review` remain project-default by default while permitting an
  explicit global override, or should it become global-only like other standing
  workflow preferences?
- Should `lead-tune` treat sage review as its own handler, or should the generic
  config catalog be rich enough that no playbook-specific handler is needed?
