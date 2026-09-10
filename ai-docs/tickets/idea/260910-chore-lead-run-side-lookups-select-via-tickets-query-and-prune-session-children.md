---
title: "lead-run's side lookups cost more lead context than the run itself: Explore-based selection and an unpruned session.children listing"
related:
  260910-bug-lead-run-worker-key-lookup-ignores-delegate-scope: sibling; fixes the correctness of the worker-key lookup, this ticket addresses its volume
---

# lead-run's side lookups cost more lead context than the run itself: Explore-based selection and an unpruned session.children listing

## Background

Measured on 2026-09-10 while running one ticket through the installed 0.45.2
plugin (promote, run, merge): the lead spent roughly 88k tokens of context
for the whole cycle, and the worker's terminal report — the only input the
run step actually needs — was a small fraction of it. The rest was side
lookups the playbooks mandate or invite:

- **Selection through an Explore spawn.** `lead-run` tells the lead to spawn
  the Explore agent to pick the next ticket and never list `ready/` itself.
  With one ticket in `ready/` that spawn cost about 15k subagent tokens and a
  round trip to return the only possible answer. The rule exists so the lead
  does not read ticket *files*; `tickets.query(statuses: ["ready"])` in its
  compact text form returns stems, titles, and which phases carry a Result
  without opening any file, which is enough to apply the three selection
  rules (in-progress first, then named prerequisite, then oldest) in the
  lead's own turn for a few hundred tokens. Only the `## Blocked (...)` skip
  needs a body read, and the query can report that too.
- **`session.children` returns the whole subtree, every time.** The lead key
  had 26 children after one epic — every finished worker and every disposable
  delegate (populator, sage reviewers, code reviewers) — each with its note
  body. Two calls cost about 5k tokens. Nothing prunes a child whose ticket
  has resolved, and there is no filter by `scope` or by "note absent", so the
  lead-run step that reads the fresh worker key pays for the full history to
  find one row.

## Phases

### Phase 1: Select from tickets.query and let session.children answer the one question asked

1. Rewrite `lead-run`'s **Select** paragraph to resolve candidates with
   `tickets.query(statuses: ["ready"])` (compact form) and apply the three
   rules inline; keep the Explore spawn only as the fallback when the query
   cannot expose a needed fact. If the query does not already surface a
   `## Blocked (...)` marker per ticket, add that field rather than reading
   the body. Rejected: keeping the Explore spawn unconditionally — it re-reads
   files the tool already indexes and adds a round trip per cycle.
2. Add `scope` and `unnoted_only` (or equivalent) filters to
   `session.children`, and have `lead-run` step 2 call it with both so the
   answer is one row. Rejected: pruning resolved keys automatically — the
   notes on finished workers are the carry-over record a restarted lead
   rebuilds from, so they must stay queryable; the fix is to stop returning
   them by default, not to delete them.
3. Regenerate the rsrc manifest and the wsflow mirror; update the pinned
   tests; verify with one installed-build cycle that the lead's per-cycle
   context for select + key lookup drops to well under 1k tokens.
