---
title: Terminal ticket accumulation and garbage collection (deferred)
related:
  260924-research-rationale-incremental-history-cache: sibling discussion; the real per-call cost lives there
  260924-feat-origin-ticket-ownership-index: its origin-closed check reads terminal ticket presence
---

# Terminal ticket accumulation and garbage collection (deferred)

## Background

`.done/` and `.dropped/` grow without bound. The user raised whether this
needs attention: for example deleting terminal tickets unreachable by
references after a two-to-three-month margin (git history keeps them anyway)
and teaching lookup tooling such as rationale search to fall back to git
history; or whether an ever-growing flat directory of text files simply does
not matter.

The discussion concluded that GC is **not urgent**: it is not a cost problem
today, and deletion currently breaks landed-state semantics. This ticket
exists so the topic can be picked back up from where the discussion left it,
not as queued work.

## Why Not Now

- **Scale is harmless.** 566 `.done/` + 100 `.dropped/` files, 7.5 MB, about
  five months of work (2026-09-24). Deleting files does not shrink the
  repository, since history retains them, and git and the filesystem are
  unbothered by this file count.
- **Scan cost is small.** Including terminal statuses adds roughly 90ms to a
  `tickets.query`. The expensive path, `rationale.query`'s full history walk,
  scales with commits and is untouched by deletion; see
  `260924-research-rationale-incremental-history-cache`.
- **Search noise is mostly contained.** The dot-prefixed directories are
  skipped by default by ripgrep-style searches.

## What Deletion Would Break Today

Terminal ticket file presence is currently the evidence for "landed":

1. `blocked-by` gating: a prerequisite counts as landed only when found in
   `.done/`; a missing prerequisite yields "resolves to no ticket on the
   board" and blocks permanently (`agents-plugin-tool/internal/wsdoc/tickets_deps.go`,
   `edgeLanded`).
2. The origin-closed check designed in
   `260924-feat-origin-ticket-ownership-index` uses the review-track tree as
   the done list; a deleted landed ticket would read as not landed and become
   re-acquirable. That design's landed-closure pruning uses the same evidence.
3. `rationale.query` ticket records (decision sections) are read from working
   tree files only (`wsrationale/tickets.go`, `scanTickets`); deleted
   tickets' decisions vanish from search.
4. Graph rendering (parent/child listings in `wsdoc/tickets_graph.go`)
   resolves children from the board.

## Reference Reachability (2026-09-24 snapshot)

Of 666 terminal tickets, by stem occurrence:

| Referenced from | Count |
|---|---|
| live tickets, `AGENTS.md`, manuals, or plugin/tool source | 190 |
| only other terminal tickets | 247 |
| nowhere | 229 |

A "delete unreferenced after a margin" policy would start at roughly 229 and
cascade as referrers themselves become eligible.

## Direction Discussed (if revisited)

The lead's proposed ordering, non-authoritative:

1. Decouple landed state from file presence first: an in-tree, one-line-per-
   stem landed ledger (stem, landing commit, date) that `blocked-by` gating
   and the origin-closed check consult. The ownership index cannot be the sole
   landed authority because it is opt-in and legacy projects lack it.
2. Give `rationale.query` a git-history fallback that restores a deleted
   ticket's text from the ledger's landing commit.
3. Only then GC: tool-driven, deleting terminal tickets with no live
   reference past a margin (the user's two-to-three months; ~90 days).

Not recommended: sharding terminal directories by year. It breaks path-based
references and downstream tooling that assumes flat status directories, for
little gain.

## Outcome Ledger

### Verified Findings

- Terminal inventory at 2026-09-24: 666 files, 7.5 MB; `tickets.query` scan
  overhead about 90ms.
- `blocked-by` landed evaluation, rationale ticket records, and board graph
  rendering all depend on terminal ticket files being present.
- Reachability snapshot: 190 live-referenced, 247 terminal-only-referenced,
  229 unreferenced.

### Confirmed Decisions

- Terminal-ticket GC is deferred as not urgent; the incremental rationale
  history cache takes priority.

### Proposals

- Landed ledger → rationale git fallback → margin-based reachability GC, in
  that order (**Direction Discussed**).
- Do not shard terminal directories by year.

### Open Questions

- The trigger for revisiting: a file count, measured scan latency, or
  downstream complaint.
- Whether stem-uniqueness checks at ticket creation scan terminal tickets
  (unverified); deletion could allow stem reuse if so.
- Whether `.dropped/` warrants a shorter margin than `.done/`, since dropped
  tickets are rarely prerequisites.

### Rejected Alternatives
