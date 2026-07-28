---
title: duplicate ticket stems across status directories resolve silently
related:
  260726-feat-verify-ticket-graph-advisories: introduced byStem and the first-wins duplicate resolution this ticket questions
---

# duplicate ticket stems across status directories resolve silently

## Background

`agents-plugin-tool/internal/wsdoc/tickets_graph.go:97-106`: `loadTicketGraph`
builds `byStem` as a first-wins map over tickets sorted by status openness
(`ready` → `todo` → `idea` → `.done` → `.dropped`). If the same stem exists as
a file in two status directories at once, `byStem` silently keeps "the most-open
copy" and drops the other from the graph entirely.

The code's own comments name this scenario explicitly:

> scanTickets is sorted by ticketStatusRank, so first-wins keeps the most-open
> copy when the same stem exists in two status directories. That direction
> matters: an **abnormal** duplicate then degrades toward "still open" rather
> than producing a false closure nudge.

So the resolution direction (prefer open) was a deliberate choice to fail safe
toward "don't falsely claim done." But "abnormal" is acknowledged and nothing
detects or reports the abnormality itself.

## Measured 2026-07-28

Reproduced by experiment: created copies of the same stem under `todo/` and
`ready/` simultaneously (a state `git mv`-based moves should never produce, but
which unrelated tooling, manual edits, or a bad merge could). `tickets.verify`
against either copy returned `OK=true` with no finding, warning, or advisory
naming the duplicate. The graph silently resolved to one copy and proceeded as
if the board were normal.

This is new code with no prior `main` behavior to regress from — it is a gap in
day-one coverage, not a regression.

## Why this is worth deciding rather than just fixing

A duplicate stem is exactly the kind of cross-file integrity condition
`260726-feat-verify-ticket-graph-advisories` exists to surface — the ticket
that added `byStem` also added the advisory framework that could report this.
Before wiring a detection: is a same-stem duplicate across status directories
something that can occur through normal tooling at all (if `git mv` is the only
sanctioned move path and is atomic, is this purely an out-of-band-edit
scenario), and if so, should detection block the commit or advise only — the
existing graph-advisory pattern in this file is non-blocking by design
(`tickets_graph.go:9-16`: "every output here is non-blocking, because a commit
is reversible"). That precedent argues for an advisory, not a hard fail, but it
should be a stated decision rather than an assumption carried into
implementation.

## Non-Scope

- Does not propose the specific advisory text or detection mechanism.
- Does not investigate whether any tool in the current tree can actually
  produce this state through normal use; the experiment above forced it
  directly by creating both files.
