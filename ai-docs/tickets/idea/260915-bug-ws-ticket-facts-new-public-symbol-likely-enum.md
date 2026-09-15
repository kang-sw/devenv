---
title: "Harden route-fact enum handling: an out-of-enum scope.new_public_symbol collapses the whole facts table to unknown"
related:
  260915-feat-ws-worktree-pool-default-out-of-tree: same worktree-run dogfood session
---

# Harden route-fact enum handling for out-of-enum values

## Observed

During a parallel `ws:lead-run` batch (2026-09-15), two independently authored
`ready/` tickets carried a `## Route Facts` row
`scope.new_public_symbol | likely`. `likely` is outside the accepted enum
(`yes` / `no` / `unknown`). Both workers stopped `stop: c` at Execute step 1
because `route.resolve_implement`'s verdict reported the route facts as
effectively missing/unreadable — **every** fact resolved to `unknown`, not just
the offending cell.

**The data defect is already fixed inline** on the two tickets during the run
(`likely → yes`, evidence-backed; commits 27e23e15, 69f885d2). What remains is
the robustness question below.

## Code findings (2026-09-15, verified against source)

- The parser `ticketRouteFacts`
  (`agents-plugin-tool/internal/wsdoc/tickets.go:562`) is **tolerant**: it
  reads the first two columns and stores every value **verbatim**
  (`facts["scope.new_public_symbol"] = "likely"`). It does not validate the
  enum and does not reject the table on a bad cell.
- Therefore the "one bad cell → whole table becomes `unknown`" collapse lives
  **downstream in `route.resolve_implement`'s fact normalization**, not in the
  markdown parser. (Confirm the exact normalizer site during implementation.)

## Why it matters

`likely` is a natural hedge an author (or `ticket-fact-populator`) reaches for
when a new symbol is intended but unnamed. Because the downstream normalizer
collapses the *entire* table on a single out-of-enum value, one typo turns a
ready ticket into a hard worker stop and wastes a dispatch — twice, in a
parallel batch.

## Decision needed (this is not a mechanical fix)

Two enforcement points, and the ticket owner must choose one or both:

1. **Authoring-time validation** — have `ticket-fact-populator` / the
   ticket-authoring path constrain `scope.new_public_symbol` (and peers) to the
   enum, rejecting or normalizing `likely`-style hedges before a ticket is
   written.
2. **Route-normalizer tolerance** — change `route.resolve_implement` to fail
   **per-cell** (flag the one bad fact, keep the rest) instead of collapsing the
   whole table to `unknown`. This is a behavior change to the routing contract
   and needs its own care.

A cheap third: a `tickets.verify` lint for out-of-enum Route Facts before
`ready/`.

## Verification (implementation-time)

- Unit: a table with one out-of-enum cell resolves the *other* facts correctly
  (per-cell path), or is rejected at authoring time (validation path).
- Existing `route.resolve_implement` / ticket-fact suites stay green.

## Notes

Captured under the "Dogfood surprises get captured" discipline. Sibling
same-session tickets:
`260915-bug-ws-tickets-close-operates-on-server-cwd-not-worktree` (investigation),
`260915-bug-ws-route-resolve-implement-branch-handling-random-codename`.
