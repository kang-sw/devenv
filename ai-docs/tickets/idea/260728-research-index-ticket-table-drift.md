---
title: The _index.md ticket table stopped tracking new tickets a month ago, and
  AGENTS.md points at a section that does not exist
related:
  260728-research-parallel-workflow-guide-divergence: same shape - a hand-maintained
    surface with no mechanism keeping it true
---

# The project-memory ticket table has silently stopped being true

## Background

`ai-docs/_index.md`'s `## Tickets` section says "This index lists active tickets
only". Measured 2026-07-28:

- 88 ticket files live in `ready/`, `todo/`, and `idea/`.
- The table has 74 rows.
- The newest stem in any row is `260627`. Nothing from `260721` onward appears —
  a month of tickets, including every ticket created by the two epics currently
  in flight.
- Several rows carry `done`, contradicting the section's own "active only"
  statement.

So the deficit is not 14. Rows for closed tickets mask a larger number of
missing live ones, and a session that reads project memory to learn the backlog
gets a picture that is a month stale and silently wrong rather than visibly
incomplete.

Separately and definitely broken: `AGENTS.md:199` instructs every session to
"Check `## Ticket Focus` in `ai-docs/_index.md` before starting implementation".
`_index.md` has no `## Ticket Focus` heading. Its headings are `Repo`,
`Current Branch Rules`, `Plugin Topology`, `Read Before Editing`,
`Runtime Surfaces`, `MCP Runtime Notes`, `Prompt And Agent Inventory`,
`Skill Inventory`, `Canonical Flows`, `Specs`, `Tickets`, `Session Notes`. A
session following the instruction finds nothing and has to guess whether the
`## Tickets` table is the intended substitute.

## Why this is worth deciding rather than just fixing

Adding the 40-odd missing rows is an afternoon of work that buys nothing durable,
because nothing would stop the table drifting again the same way. The table has
no generator, no test, and no step in any playbook that updates it. That is the
same shape as `260728-research-parallel-workflow-guide-divergence`: a
hand-maintained surface asserted to be true with no mechanism keeping it so.

The prior question is what the table is *for*. Three readings, and they imply
different fixes:

- **An exhaustive mirror of the ticket directories.** Then it should be
  generated, not written — `ws/tickets.list` already produces exactly this, and
  a hand-maintained copy of a tool's output is pure liability.
- **A curated attention list.** Then "lists active tickets only" is the wrong
  sentence and the drift is not drift at all — it is a curator who stopped
  curating, and the fix is to say what the curation rule is.
- **Vestigial.** Then delete it and point readers at `ws/tickets.list`, which is
  the only surface that cannot be stale.

The `AGENTS.md:199` pointer is a defect under all three readings and can be
fixed independently of the decision.

## Direction

- Settle which of the three readings is intended. The `## Tickets` preamble
  ("Reference tickets by stem. This index lists active tickets only; completed or
  dropped tickets live in hidden archive dirs and git history") reads as the
  exhaustive-mirror intent, which argues for generating it or deleting it.
- If generated: check whether `ws/tickets.list` output can be embedded, and what
  refreshes it — a regen test in the `WSRSRC_REGEN` family is the obvious home,
  since that machinery already exists and already fails loudly when a generated
  artifact is stale.
- Fix `AGENTS.md:199` either way, to name whatever section actually carries the
  focus list.
- Worth checking at the same time whether any other `AGENTS.md` or playbook
  instruction points at an `_index.md` section that no longer exists. This one
  was found by following the instruction literally; nothing checks these
  pointers.

## Prior art

Found while closing `260726-refactor-retire-spec-planned-marker-mechanism`,
whose own stem is absent from the table despite being in `ready/` through two
implementation phases.
