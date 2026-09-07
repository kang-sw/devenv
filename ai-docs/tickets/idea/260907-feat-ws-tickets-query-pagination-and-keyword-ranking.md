---
title: tickets_query returns every match unbounded; add a default page cap and keyword-ranked matching
related:
  260907-feat-ws-project-tree-parent-nested-ticket-render: sibling symptom found in the same session; that ticket names tickets_query as the on-demand home for the related graph, so this cap should land first
  260903-epic-mcp-tool-surface-affordance-reduction: same theme — reduce the resident cost of the MCP tool surface
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
already has a "N hidden" footer, `tickets_query` has none.

## Decisions

- **Default page cap with a continuation footer.** A bounded default result
  count (order of 20–30), a `N more — refine the query or page` footer, and an
  explicit page/offset (or cursor) argument to fetch the rest. An explicit
  larger limit stays available for callers that want it.
- **Keyword-ranked matching so the first page is the right page.** Replace
  plain substring matching with a lexical ranker over the ticket corpus —
  BM25/TF-IDF-style, title weighted above body, no embedding engine or
  external dependency — so a free-text keyword query returns a ranked top-N.
  Structured filters (status, stem/date prefix, parent scope) stay as optional
  post-filters layered on the ranked result; they are not the primary query
  interface, since a fully specified structured query presumes the caller
  already knows the ticket.
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
- Land before `project_tree` moves its related graph here (see Related), so
  the bottleneck is not relocated.

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
