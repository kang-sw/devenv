---
title: "tickets.query point-resolve reports a closed ticket as not found"
completed: 2026-11-13
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
exists under `.done/` / `.dropped/` and names the flag.

## Decisions

- Exact-stem point resolution searches all statuses; discovery defaults remain unchanged. This corrects the existing documented resolution contract (T0: no tool, argument, or storage shape removed).

## Result

Point resolution now includes `.done/` and `.dropped/` regardless of the inclusion flags, and the tool description states this. The related terminal-status ticket is the same issue and is closed with this fix. Regression coverage checks both terminal statuses, text and JSON, discovery filtering, and terminal scan errors. On w5090, `go test ./internal/mcp/...` passed; `go test ./...` passed except the known `TestHostedConnectionSeeingALANChangeReplacesTheDaemon` timeout. Forward: restore dispatch-gate fail-open regression coverage with an error injected after the point-resolution scan.
