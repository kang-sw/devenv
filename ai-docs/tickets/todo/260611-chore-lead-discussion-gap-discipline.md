---
title: Lead discussion discipline — readable discussion replies + explore-before-gap-fill
---

# Lead discussion discipline — readable discussion replies + explore-before-gap-fill

## Background

Dogfood surprise (2026-06-11, during the `260611-refactor-ws-tier-taxonomy-delegate-tier-routing`
Phase 1 discussion). The lead agent hit a context gap — it had NOT read the
migration anchor `260605-research-ws-native-subagent-pivot` (which AGENTS.md
mandates for plugin-architecture / spawn-removal / adapter-boundary tasks) nor the
M3 ticket — and **filled the architecture gap with imagination**: wrong premises
that wsflow is "agentless ⇒ no delegation" and that wsprompt retirement was an open
fork. A post-hoc Explore audit over the ticket cascade showed two of those premises
were already documented (the agent simply had not read the sources), while a third
(wsprompt being parallel-maintained, not retired) the agent HAD mentioned but
buried inside a verbose option-dump, so the user missed it. Two workflow-improvement
directions surfaced.

## Phases

### Phase 1: Discussion-mode reply readability + coherence

Improve how the lead replies *during discussion* (the `lead-discuss` surface and the
discussion portions of `lead-proceed`). Reduce verbose gap-filling and exhaustive
option-dumps; lead with the single load-bearing point; keep each claim adjacent to
its evidence; do not bury a statement or decision the user must act on. Target: a
user skimming the reply cannot miss the key point. Likely surface: `lead-discuss`
playbook text + the AGENTS.md / skill "Response Discipline" guidance, possibly a
brevity/structure rule for discussion turns. Verification: review of
representative discussion replies against a readability/coherence rubric.

### Phase 2: Gap → explore-the-cascade-first (no imagination)

When the lead detects it lacks a documented decision or architecture fact, it must
FIRST search the ticket/spec cascade before answering — Explore over
`ai-docs/tickets` + `ai-docs/spec`, `mental_models.find`, and honor the AGENTS.md
migration-anchor read rule — and never present inference as established fact.
Consider enforcing the migration-anchor read in `lead-implement` Prep and/or
`lead-proceed` route context when the task touches plugin architecture,
spawn-removal, or adapter boundaries (the Prep step today calls `mental_models.find`
but does not pull the migration anchor). Verification: a gap-handling rule the lead
follows + (if enforced) a Prep/route step that loads the anchor for in-scope tasks.

### Phase 3: Ticket-write consent gate (persist only confirmed decisions)

Dogfood (2026-06-12, same tier-taxonomy discussion). While parking a follow-up,
the lead wrote a *mechanism* decision (`render-param forwarding`) into the ticket
`## Decisions` BEFORE the user had confirmed it — the user had agreed only to
parking the `ws.mercenary.*` rename, and the mechanism was still under discussion
(it was later superseded by a different design). The lead-write-ticket /
lead-proceed surfaces should gate decision capture on explicit user agreement:
persist only decisions the user has confirmed, and before a ticket-cleanup pass,
surface the full set of open items and get agreement on ALL of them at once rather
than committing a draft decision that then needs correction/revert. Likely
surface: `lead-write-ticket` Apply-Ticket-Content / Intent-Review guidance + a
"confirmed-only" capture rule. Verification: a rule the lead follows + a
representative discussion where unconfirmed decisions are held back until
agreement.

## Notes

- Captured at `todo` (accepted backlog), not spec-addressed; slicing and spec
  addressing happen at ready promotion.
- Standalone workflow-discipline improvement; not part of the playbook-factory epic.
