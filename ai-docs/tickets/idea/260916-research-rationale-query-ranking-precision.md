---
title: rationale.query ranking precision — embeddings trigger and path/pickaxe crowding
related:
  260915-feat-ws-rationale-query-and-prior-decisions: source ticket; Phase 3 validation surfaced the two findings below
---

# rationale.query ranking precision — embeddings trigger and path/pickaxe crowding

## Background

260915's Phase 3 ran the tool's three populator-recipe query shapes against
this repository's own recorded reversals and measured, for each, whether the
known target lands in the top 8 threads. 4 of 6 named sub-targets landed; the
two misses are dogfood surprises captured here per AGENTS.md's dogfood-capture
rule, since Phase 3's own ticket is closing to `.done/` and its Follow-up
candidates list is then immutable.

Both misses trace to literal Tool Specification contract text (BM25 formula,
tokenizer, `order: time` default, thread-widening rule), not to an
implementation bug — Phase 3 fixed the one real bug it found (ordered-list
bullet extraction) directly in 260915. These are findings for a future
ticket to weigh, not a defect to patch in passing.

## Finding 1: query-only misses meet Decision 2's embeddings-reconsideration trigger

260915 Decision 2: "Embeddings are reconsidered only if Phase 3 measures a
miss rate that path and site addressing do not cover."

Measured: query `session.note` (no paths, ~6600 commits scanned) failed to
surface two known targets in the top 8 threads even after fixing the
ordered-list extraction bug:
- The refoundation epic's Cross-Child Decision 16 (`260909-epic-ws-worker-interpreter-refoundation`,
  "Notification-driven wait; `session.note` is a carry-over record") ranked
  ~34th of ~46 threads at `limit: 100`.
- The lead-run diet ticket's decision declaring the record's removal
  (`260915-refactor-lead-run-playbook-diet-drop-assignment-note`, "The
  `session.note` assignment record is removed from `lead-run` entirely.")
  ranked ~14th.

Neither miss is rescued by path or site addressing: a cross-cutting prose
decision in a `## Cross-Child Decisions` section has no single owning file or
line range to address by `paths:`/`site:`. The populator's own second recipe
call (`paths: agents-plugin/rsrc/lead-run`, no query) recovers only the
third, unrelated target (the implementation commit) — not these two.

Root cause: `tokenize()` splits `session.note` into two generic tokens
(`session`, `note`) with low IDF across a corpus where both words are common
(session keys, `note.*` tooling), so BM25 cannot discriminate a specific
decision from dozens of unrelated mentions. This is the literal, specified
tokenizer and BM25 formula (Tool Specification "Ranking" section) — not a bug.

## Finding 2: path/pickaxe crowding when a broad commit touches the queried file

Measured: `paths: ["agents-plugin-tool/internal/wsdoc/tickets_mutate.go"]`,
no query — target was the commit that actually retired the ready spec-address
gate (`91621687`, thread `260909-refactor-retire-spec-mental-model-layers`,
2026-09-10). It was absent even at `limit: 100` (574 records past limit).

Root cause: two things compound. (a) the Record model's thread-widening rule
("a ticket record matches when...a commit in its thread touched [the path]")
pulls a matching ticket's *entire* decision/result history into the ranked
set once any one commit in its thread touches the path; (b) the spec's
`order: time` default (no query given) then sorts purely by recency. A single
later, unrelated "landing" commit that happens to also touch the queried file
as part of a broad multi-ticket commit (e.g. `203e555c`, "Goal run ...
drained the ready queue: two tickets landed") pulls its own ticket's full
35-record history ahead of the smaller, more specific, and actually
on-topic older thread — burying it past the fixed 100-record global cap.
Both (a) and (b) are literal spec text (Record model, Ranking section), so
this is a design tension between "broad recall via thread-widening" and
"precision on a simple file lookup," not an extraction bug.

## Outcome Ledger

### Verified Findings

- Query-only lookups over short, generic, dotted compound terms
  (`session.note`) do not reliably surface a known target in the top 8
  threads on this repository's real corpus, even after fixing the
  ordered-list extraction bug. Path/site addressing does not rescue these
  particular misses (no natural path or site owns a cross-cutting prose
  decision).
- Path-only, no-query lookups on a file touched by both a narrow authoritative
  commit and a later broad multi-ticket "landing" commit can bury the
  authoritative commit past the 100-record cap, because thread-widening pulls
  in the broad commit's entire ticket history at full weight and `order: time`
  ranks it first purely on recency.

### Proposals

- Reconsider embedding-based (or hybrid lexical+embedding) ranking per
  Decision 2's own trigger, now measured as met.
- For path/pickaxe crowding: consider a per-thread record budget (round-robin
  allocation across matching threads before the global cap) instead of a pure
  global sort-then-group, so one broad commit's thread cannot consume the
  entire limit; or down-weight a ticket record's path-match when it derives
  only from a commit that touched many unrelated files.
- A standalone `rationale-discovery` delegate (already a 260915 follow-up
  candidate) could partially mitigate both findings by running several query
  variants with judgment, rather than relying on a single mechanical populator
  recipe call.

### Open Questions

- Whether the precision gap is severe enough on downstream, smaller
  repositories to justify the added complexity of embeddings or per-thread
  budgeting, versus accepting the current lexical-only, recency-ordered
  design and documenting the limitation for callers.

### Rejected Alternatives

(none yet — this ticket is diagnostic, not a design proposal)
