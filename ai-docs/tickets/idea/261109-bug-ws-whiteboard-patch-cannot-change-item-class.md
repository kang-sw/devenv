---
title: "whiteboard: wb:patch cannot change an item's class, so a reclassified item still shows as technical"
---

# whiteboard: wb:patch cannot change an item's class, so a reclassified item still shows as technical

## Context

Dogfood surprise (2026-11-09). The lead-ticket playbook says an announced or
technical item the user objects to, or one a later finding undercuts,
"becomes a policy question under its existing ID". On the whiteboard the lead
re-opened items (112) and (113) with `wb:patch status="open"` plus options,
a recommendation and a new rationale, intending them as policy questions.
`wb:patch` has no `class` attribute, so the store kept `[technical]`. The user
reported the board still showed them as resolved technical items ("지금 제
화면에는 112 113 114가 이미 resolve된 technical로 보입니다"). The lead worked
around it by superseding (112)-(114) and adding fresh items (116)-(118),
renumbering the ticket queue.

## Questions

- Should `wb:patch` accept `class` (open items only), so the playbook's
  "reclass under its existing ID" rule is expressible on the board?
- Why did the page show an `open` technical item that once had an answer as
  resolved? Check how the page renders a technical item whose status went
  back to `open` after an `ok` answer (stale answer chip, or collapsed
  settled rendering).
