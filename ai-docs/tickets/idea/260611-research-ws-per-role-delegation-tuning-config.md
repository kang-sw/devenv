---
title: Config surface for per-role / per-partition delegation tuning (tier + prompt) at the ws level
related:
  260609-refactor-ws-spawn-runtime-deletion-session-auth: the 2c Edition #2 per-role tier routing is the mechanism this surface would expose as config; this idea generalizes it
  260610-refactor-ws-wordchain-id-generalization: sibling "generalize a per-call concept into a reusable surface" follow-up
  260513-harness-local-agent-tier-config: existing config.agents_tier (tier -> backend/model/effort) that this would extend toward role -> tier
---

# Config surface for per-role / per-partition delegation tuning (tier + prompt) at the ws level

## Background

Surfaced while running a partitioned `lead-implement` review (Phase 3 of M3):
the lead delegated three reviewers (correctness/fit/test) and the user asked for
a tier split (correctness on the deepest tier, fit/test on a mid tier). Today the
only levers are:

1. **Hardcoded skill text** — the `lead-implement` `judge: review-allocation`
   default (being added: `correctness -> deep tier`, `fit/test -> core tier`),
   static guidance the lead follows. Changing it means editing the playbook.
2. **`config.agents_tier`** — maps a tier (`light`/`core`/`deep`) to a
   `{backend, model, effort}` per harness. It is tier-centric, NOT role-centric:
   nothing says "the test reviewer defaults to core."
3. **Per-call native model param** — when the lead delegates via the native
   harness Agent tool it can set a concrete model, but that is per-call and
   harness-specific, not a durable config.
4. The **ws mercenary path cannot carry a per-spawn tier at all** yet (the 2c
   Edition #2 gap — register schema dropped `tier`).

There is no single tunable surface where a user can say, durably and
host-neutrally, "reviewer.test runs at core, reviewer.correctness at deep,
implementer at core, reference-discovery at light" — let alone tune the *prompt*
(verbosity / extra instructions) per delegated role.

## The idea

A ws-level config surface that parameterizes per-role (and ideally
per-partition) delegation, addressable like `reviewer.test.level`,
`reviewer.correctness.level`, `implementer.level`, etc. Minimum: a role -> tier
mapping that the lead consults when delegating. Stretch: role -> prompt tuning
(extra instructions, verbosity, or a prompt fragment) so a user can finely shape
each delegated role without editing playbooks.

Precedence (natural shape): per-call explicit override > this role config >
skill-text default. The `lead-implement` tier default becomes the *fallback
default* of this config rather than hardcoded-only text.

## Why it matters

- Lets users tune cost/rigor per delegated role without forking playbooks.
- Unifies the three scattered levers (skill text, `config.agents_tier`,
  per-call model) into one addressable surface.
- Host-neutral: expressed in ws tier vocabulary (`light/core/deep`); the harness
  adapter maps tier -> concrete model, so it works for native and (once the 2c
  Edition lands) mercenary delegation alike.

## Open questions

- **Granularity:** role only, or `role.partition` (e.g. `reviewer.test`)? Which
  roles are addressable (reviewer partitions, implementer, reference-discovery,
  plan-populator, mental-model-updater)?
- **What is tunable:** tier/model only, or also prompt detail (verbosity, extra
  instructions, a prepended fragment)? Prompt tuning is a much larger surface and
  may warrant its own phase.
- **Config schema:** how it nests under existing `wsconfig`; how it coexists with
  `config.agents_tier` (tier->backend/model) vs this (role->tier); whether a new
  `config.*` MCP tool is needed or it folds into `config.show`/an existing setter.
- **Honoring path:** depends on the 2c Edition #2 per-role tier routing for the
  mercenary path; native path can honor role->tier sooner.
- **Precedence + discoverability:** how the lead surfaces which tier each role
  resolved to (debuggability), and how per-call override is expressed.

## Scope / sequencing notes

- Builds on the 2c Edition #2 per-role tier routing
  (`260609-...session-auth` Phase 2c Edition `0c7c0f50`); the routing mechanism
  must exist before the mercenary path can honor a role config.
- The immediate `lead-implement` skill-text tier default (correctness->deep,
  fit/test->core) is the bootstrap; this ticket would later turn that default
  into a config-backed default.
- Likely a multi-phase actionable ticket once promoted (role->tier config first;
  per-role prompt tuning as a later, larger phase).
