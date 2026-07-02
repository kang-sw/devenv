---
title: "Disambiguate skill-surface routing for saving a user preference"
parent: 260605-epic-ws-playbook-factory-pivot
related:
  260625-feat-ws-session-state-machine: surfaced-during
sage-review: required
completed: 2026-06-30
---

# Disambiguate skill-surface routing for saving a user preference

## Background

When a user asks to persist a standing preference, the entry point is ambiguous.
Observed live during the 260625 dogfood: a request to "save a user preference"
was routed to `ws:lead-add-rule`, which turned out to be the wrong surface, and
the work had to pivot to `config.prompt.set`.

## The two surfaces

- `ws:lead-add-rule` routes ONLY to repo rule docs — `CLAUDE.md
  ## Architecture Rules` or a mental-model `## Domain Rules`. It has no path to
  the user-preference layer.
- The user-preference layer is the `UserPreferenceSection` prompt override-point,
  owned by `ws:lead-tune` / `config.prompt.set` (a `prompt_override` knob in
  `config.tuning`). It also carries a harness-scoping dimension
  (`claude` / `codex` / `*`) that `lead-add-rule` does not model.

A user saying "remember this preference" cannot tell which skill is correct, and
`lead-add-rule`'s description ("persist a user-requested workflow rule") reads
like it should handle preferences too.

## Proposed direction

- Either teach `lead-add-rule` to recognize a user-preference target and route to
  the prompt-override layer (`UserPreferenceSection`, with harness scope), or
- Add a cross-reference so the two skills disambiguate up front: rule docs
  (durable repo invariants) vs. user preferences (prompt-override layer,
  harness-scoped).
- Clarify the boundary in both skill descriptions so the routing decision is
  obvious from the request shape.

## Note

`config.prompt.set` REPLACES the whole override value for a `(pointId, harness)`
pair, so a naive preference write can clobber existing preferences — whichever
surface owns this should preserve-and-append, not overwrite.

## Decision (260629 sweep)

Fix: Clarify routing boundary in lead-discuss and lead-sprint. lead-tune routes ws workflow preferences (delegation posture, model tier, mercenary on/off, prompt overrides). lead-add-rule routes project-level coding/architecture rules and conventions. Add a cross-reference note in each skill's description so the distinction is visible at invocation time. No new behavior; prose-only clarification.


## Resolution (2026-06-30)

Added routing preference disambiguation note to lead-discuss and lead-sprint invariants: "save a preference / remember a setting" routes to lead-tune; lead-add-rule is for repo-level rules only.
