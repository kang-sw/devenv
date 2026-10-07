---
title: Sage gate keeps a stale skipped design posture after the knob changes
---

# Sage gate keeps a stale skipped design posture after the knob changes

## Background

Dogfood, 2026-10-07. `tickets.sage_gate(landing: "ready")` ran on two todo
tickets while `sage_review_design` resolved to its builtin `off`, and wrote
`sage-review-design: skipped` into each ticket's frontmatter. Before either
ticket was committed in `ready/`, the user set `sage_review_design: auto` at
repo scope (`c8ce680e6`) and the tickets were moved back to `todo/`. A fresh
`sage_gate(landing: "ready")` then returned `action: skip` for both: the
persisted `skipped` posture won over the now-`auto` knob, though the knob's
description says `auto` requires a design pass before a ticket reaches
`ready/`. Deleting the `sage-review-design: skipped` line by hand made the
gate resolve `run` with the design reviewer.

## Question

Should a `skipped` posture that came from the knob (not from a review) be
re-resolved against the current knob on each gate call, so a stricter knob
takes effect for tickets not yet in `ready/`? A `completed` posture backed by
a reviewed digest is a different case and is not in question here.
