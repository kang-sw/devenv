---
title: "ws session-state tool ergonomics: enter.* derive + todo.list surface gaps"
parent: 260605-epic-ws-playbook-factory-pivot
related:
  260625-feat-ws-session-state-machine: motivating-feature
---

# ws session-state tool ergonomics: enter.* derive + todo.list surface gaps

## Background

Two concrete usability gaps observed live during the 260625 dogfood. Neither
blocks work, but both make the session-state tools harder to drive correctly.

## Gap A — `enter.*` arguments not reflected in the derived checklist

`ws.enter.implement(review_alloc: "single", ...)` produced a derived todo item
titled `Review (partitioned)`. The `review_alloc` argument was not reflected in
the rendered title — either the parameter does not drive the label at all, or
`partitioned` is a hardcoded default that ignores the input.

Investigate: are `enter.implement` knobs (`review_alloc`, `plan_depth`, etc.)
actually threaded into the derived checklist, or only stored in the agenda blob?
If stored-only, the rendered titles are misleading; if meant to drive rendering,
the wiring is broken.

## Gap B — `todo.list` hides the item keys callers need

`ws.todo.list` (both `summary` and `full` mode) renders item TITLES but not item
KEYS. Yet `ws.todo.check`, `ws.todo.erase`, and `ws.todo.reorder` all require the
`key`. After `ws.enter.implement` auto-derives the checklist, a caller has no
listed way to learn the keys — during the dogfood the keys had to be GUESSED
(confirmed they are the lowercase slug of the title, e.g. `route`, `prep`).

Proposed: expose the key alongside each item in `todo.list` output (both modes),
so the mutation tools are callable without guessing. Document the key-derivation
rule (slug-of-title) explicitly if it is intended to be stable.

## Proposed direction

- Audit `enter.*` derivation to confirm which typed args drive titles vs are
  agenda-only; fix or document accordingly.
- Add keys to `todo.list` rendering; treat key visibility as part of the tool
  contract since the sibling mutation tools depend on it.
