---
title: "tickets.query point-resolve reports a closed ticket as not found"
---

# tickets.query point-resolve reports a closed ticket as "not found"

## Background

Dogfood surprise, 2026-11-10 (a ticket-fact-populator delegate and the lead
both hit it): `ws/tickets.query(ticket_stem: "<stem>")`, the point-resolve
form, returns `ticket not found: <stem>` for a ticket that exists in
`ai-docs/tickets/.done/`. The same call with `include_done: true` resolves
it (`[.done] <stem> ...`). A caller resolving a `related:` stem reads the
error as a dangling reference.

Expected: either point-resolve finds a stem in any status (it names one
exact ticket, so status filtering adds nothing), or the error says the stem
exists under `.done/` / `.dropped/` and names the flag. Which of the two is
open.
