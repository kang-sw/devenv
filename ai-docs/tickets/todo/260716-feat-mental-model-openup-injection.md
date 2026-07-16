---
title: Deterministic mental-model pointer-injection at delegation open-up
sage-review-design: required
related:
  260716-feat-ws-doc-condition-diagnostics: prerequisite — injection telemetry must exist before injection becomes the primary delivery path
  260716-feat-sage-related-mental-model-curation: association source — consumes the sage-curated related-mental-model frontmatter
---

# Deterministic mental-model pointer-injection at delegation open-up

## Background

The 2026-07-16 evaluation showed mental-model docs are accurate and updated
reliably at closeout, but delivery at open-up is probabilistic: it depends on
a discovery-tool query the agent must remember to make, multiplied by reading
discipline. The storage side is sound; the bridge is brittle. The fix follows
the established Lever B pattern: convert "agent should remember to search"
into deterministic tool behavior.

## Decisions

- **Pointer injection, not content inlining.** Mental-model docs run 9-75KB;
  inlining them into delegate briefs is a context tax of the same species as
  the bloat this system diets elsewhere. The injected artifact is a mandatory
  reading list — doc path plus section anchors (`{#YYMMDD-slug}`) where
  applicable — with an instruction to read before starting. Lead context
  cost ~0; the delegate reads in its own context.
- **Association source is ticket frontmatter first.** `related-mental-model:`
  entries — curated by the sage design reviewer per
  `260716-feat-sage-related-mental-model-curation` — are consumed
  mechanically at `enter.implement` time and injected into the generated
  instruction/brief; the same list rides into sage reviewer briefs.
- **Path→domain fallback is a later slice.** For ticket-less flows, mapping
  touched paths to domains via mental-model frontmatter `sources:` globs is
  the candidate mechanism; the first slice may accept the gap (monitored by
  the doc-condition diagnostics) rather than build the mapping.
- **Telemetry is a precondition, not an option.** Once injection exists,
  agents will stop searching manually, making injection quality a single
  point of failure whose false negatives are silent. Injection hit/miss
  telemetry lands on the counters substrate of
  `260716-feat-ws-doc-condition-diagnostics`; this ticket must not land
  before that substrate exists.

## Phases

### Phase 1: Frontmatter-driven pointer injection

`enter.implement`-generated instructions and sage reviewer brief rendering
include a mandatory-reading pointer list derived from the ticket's
`related-mental-model:` frontmatter (doc paths resolved via discovery tools;
missing stems degrade to a warning, never a hard failure). Injection events
(per doc stem, injected vs. resolved-missing) are recorded through the
consumption-counter substrate. Verification: an implement run on a ticket
with populated frontmatter shows the pointer list in the generated
instruction and matching counter increments; a ticket with a dangling stem
produces the warning path without blocking.
