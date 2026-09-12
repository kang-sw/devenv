---
title: tickets_query returns every match unbounded; add a default page cap and keyword-ranked matching
related:
  260907-feat-ws-project-tree-parent-nested-ticket-render: sibling symptom found in the same session; that ticket names tickets_query as the on-demand home for the related graph. The "cap lands first" ordering was withdrawn on 2026-09-07 (see Demotion Notes)
  260903-epic-mcp-tool-surface-affordance-reduction: same theme — reduce the resident cost of the MCP tool surface
sage-review-design: blocked
sage-review-completeness: blocked
---

# tickets_query returns every match unbounded; add a default page cap and keyword-ranked matching

## Background

`tickets_query` returns every matching ticket with no result cap and no
pagination footer. Measured on a real session (Pi-track dogfooding,
2026-09-07; the weight is harness-neutral): one broad body-text query matched
98 tickets and returned 47,073 chars / ~11,768 tokens / 326 lines in a single
call, which then stayed resident and was re-billed as a cache read on every
later turn of the session.

Breakdown of that call:

- header lines `[status] stem - title`: 98, ~6,701 tok (57%) — titles average
  ~273 chars, so even a headers-only result of 98 matches is still ~6.7k.
- `snippet:` match-context lines: 196, ~4,587 tok (39%); 44 tickets carried
  3 snippets each.
- `unresolved:` phase lines: 32, ~480 tok (4%).

The lesson is that field trimming alone does not bound the result — match
count does. A precise top-N with a bounded page is the lever; `project_tree`
already has an orphan-count footer, while `tickets_query` has only a
scope-hidden footer (sparse-checkout annotation) and no match-count or
pagination footer.

## Decisions

- **Default page cap with a continuation footer.** A bounded default result
  count (order of 20–30), a `N more — refine the query or page` footer, and an
  explicit page/offset (or cursor) argument to fetch the rest. An explicit
  larger limit stays available for callers that want it.
- **Keyword-ranked matching so the first page is the right page.** Replace
  plain substring matching with a lexical ranker over the ticket corpus —
  BM25/TF-IDF-style, title weighted above body, no embedding engine or
  external dependency — so a free-text keyword query returns a ranked top-N.
  The existing structured filters (`statuses`, exact `ticket_stem`,
  `mentions_ticket_stem`) stay as optional post-filters layered on the ranked
  result; they are not the primary query interface, since a fully specified
  structured query presumes the caller already knows the ticket.
- **Snippet budget follows the page.** Snippets default to at most one
  "why it matched" line per result (or none), with more available on request;
  they are match evidence, not the payload.
- **Rejected: a tool-side "hydrate" tier.** A second mode that returns the
  full related graph / phase list per confirmed stem was considered and
  dropped: the full body is already one file read away, and corpus-scale
  search is cheaper delegated to a low-tier child that returns distilled
  stems than built into the tool. Keep `tickets_query` a thin, deterministic
  search surface for direct small lookups.

## Constraints

- ws-mcp source change to the `tickets_query` tool; harness-neutral, authored
  on `develop` through the normal flow.
- Result line format is unchanged apart from the footer and the snippet
  budget; no schema redesign.
- ~~Land before `project_tree` moves its related graph here (see Related), so
  the bottleneck is not relocated.~~ Withdrawn 2026-09-07: `project_tree`
  relocates discovery to the `mentions_ticket_stem` reverse lookup, whose
  result sets are small, so no bottleneck moves.

## Demotion Notes (2026-09-07)

Promoted to `ready/` and demoted back to `idea/` the same day after a sage
review (both reviewers `block`, see the Blocked section) and an ROI
reassessment. Recorded so a re-open starts here instead of re-deriving.

- **Why demoted.** Unlike `project_tree`, whose cost is structural (fires
  every session, stays resident), this cost is caller-triggered: it appears
  only when a caller issues a broad text query, and it has been observed once.
  Re-open trigger: a second observed broad-query dump in dogfooding — record
  the query and the result size on this ticket.
- **Consumer survey.** Playbook call sites of `tickets.query` are mostly the
  list shape (`status:`), point-resolve, or `mentions_ticket_stem`, which a
  discovery cap would not touch. Free-text callers: `lead-discuss` cascade
  lookup, `lead-forge-spec` domain query, and `ticket-fact-populator`, whose
  prompt depends on **one** corpus-sweep call returning the whole shortlist.
  Any cap forces that prompt to learn paging or narrower queries.
- **Shared code path.** `wsdoc.TicketsFind` also serves `references.trace`
  (spec-stem query with `Resolve: true`). A cap must live in the MCP
  `tickets.query` handler, never in `TicketsFind`, or `references.trace`
  truncates silently.
- **Policy conflict.** Epic `260903` (decisions recorded in
  `260901-research-enter-tool-direct-call-affordance-rename`, commit
  `15e53f8c`) treats MCP schema growth as clutter drift and sequences work so a
  tool is not expanded under the epic. `offset`/`limit`/cursor are schema
  additions on a tool under that epic; the ticket's "no schema redesign"
  constraint did not account for this.
- **No-parameter alternative to weigh on re-open.** A fixed cap plus a
  `N more — refine the query` footer with no paging argument at all; a caller
  that needs the rest narrows the query. Not decided.
- **Open decisions** are the six items in the Blocked section; none has a
  user decision yet.

### Spec target (draft from the withdrawn promotion)

Target: `ai-docs/spec/mcp-tools.md` `{#260505-ticket-discovery-tools}` (the
`tickets.query` contract). The anchor's search-shape prose must change on three
points. (1) The text-query / filter shape no longer returns every match: a
bounded default page (order of 20–30 results) is returned, followed by a
continuation footer naming the remainder (`N more — refine the query or page`)
when matches exceed the page; an explicit continuation argument (page/offset or
cursor) fetches the next slice, and an explicit larger limit overrides the
default. (2) Matching is keyword-ranked: a free-text `query` is scored by a
title-weighted lexical ranker (BM25/TF-IDF-style, no external dependency) and
results are ordered by score; `statuses`, stem, and `mentions_ticket_stem`
filters act as post-filters on the ranked result and do not reorder it. (3)
`snippet:` match-context lines are budgeted per result (default at most one,
more on request). The point-resolve (`ticket_stem` alone) and list (no
`ticket_stem`, no `query`) shapes are unchanged. No new anchor and no
`spec-remove:`; this revises the existing anchor's prose only.

## Phases

### Phase 1: Page cap, footer, and continuation argument

Add the bounded default, the continuation footer, and the page/offset
argument; apply the per-result snippet budget. Tests: a query with more
matches than the cap returns exactly the cap plus a footer naming the
remainder; paging returns the next slice without overlap; an explicit limit
overrides the default. Verification: rerun the measured 98-match query and
confirm the first page is bounded (~2k tokens order).

### Phase 2: Keyword-ranked matching

Introduce the lexical ranker (title-weighted BM25/TF-IDF over the ticket
corpus) and order results by score, with the existing structured filters as
post-filters. Tests: a multi-keyword query ranks a ticket whose title matches
above one whose body matches; results are deterministic across runs; filters
narrow a ranked result without reordering it. Verification: the measured
query's first page contains the tickets a maintainer would pick by hand.

## Blocked (2026-09-07)

### Design Reviewer — block

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | Ranker replaces matching, but the match predicate is never defined — and it invalidates Phase 1's footer | important | missing |
| 2 | Rank / filter / cap order of operations is unspecified, and one reading silently drops matches | important | autonomous |
| 3 | Filter-only search calls have no ranking signal, yet inherit the cap | important | autonomous |
| 4 | Rejected hydrate tier sits against the sibling ready ticket's stated affordance | minor | autonomous |
| 5 | Default page size left as a range, including in the spec prose | minor | autonomous |

### Completeness Reviewer — block

| # | Title | Severity |
|---|-------|----------|
| 1 | Pagination continuation argument shape undecided (page/offset vs. cursor) | important |
