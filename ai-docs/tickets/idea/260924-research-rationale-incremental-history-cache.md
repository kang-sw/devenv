---
title: Incremental commit-history cache for rationale.query
related:
  260924-research-terminal-ticket-gc: sibling discussion; GC was found not to reduce this cost
---

# Incremental commit-history cache for rationale.query

## Background

While discussing whether terminal tickets (`.done/`, `.dropped/`) should be
garbage-collected because they accumulate without bound, the actual per-call
cost was measured. The terminal-ticket file scan turned out to be cheap; the
dominant cost of `rationale.query` is its full commit-history walk, which
grows with commit count, not ticket count, and which deleting tickets does
not touch. The user judged this cache potentially more urgent than the GC and
asked for it to be captured as research.

## Measurements (2026-09-24, this repository, macOS)

Wall times via `ws-cli call`, so each includes CLI process startup:

| Call | Time |
|---|---|
| `tickets.query` (no match), live statuses only | 0.09s |
| `tickets.query` (no match), with `.done/` + `.dropped/` | 0.18s |
| raw read of all 666 terminal ticket files (`cat`) | 0.04s |
| `rationale.query(query, kinds: [commit])` | 1.43s |
| `rationale.query(query, kinds: [ticket])` | 2.02s |
| `git log --name-only --format=%H%n%B HEAD` alone | 1.14s |
| `git log --format=%H%n%B HEAD` alone (no paths) | 0.13s |

History depth at measurement: 6,899 commits; 566 `.done/` + 100 `.dropped/`
tickets (7.5 MB).

## Current Mechanism

- `queryScan` (default, path, and text-query modes) always runs
  `scanCommits` and then `scanTickets`
  (`agents-plugin-tool/internal/wsrationale/rationale.go`, `queryScan`).
  `kinds` filters at assembly, so `kinds: [ticket]` still pays the full commit
  walk.
- `scanCommits` runs `git log --date=short --diff-merges=first-parent
  --name-only <format> -n <scanCap+1>` on every call
  (`wsrationale/commits.go`). `--name-only` forces a tree diff per commit,
  which is the measured ~1.1s.
- `scanCap = 20000`: a longer history is truncated and reported as
  `truncated at 20000`.
- `scanTickets` reads every file under every status directory on every call
  (`wsrationale/tickets.go`); measured cost is small at current scale.
- Pickaxe mode (`git log -S`) and site mode (`git log -L`) are separate paths
  whose results depend on diff content, not only on per-commit metadata.

## Direction Discussed

The lead's proposal from the discussion, recorded as non-authoritative:

- Cache per-commit parse output (record text: `## AI Context`,
  `## Ticket Updates`, date, subject; plus touched paths) keyed by commit OID.
  Commits are immutable, so entries never go stale.
- Store the last observed tip; on each call walk only `<cached_tip>..HEAD`
  and append. If the cached tip is not an ancestor of `HEAD` (rewrite, other
  branch), fall back to a walk whose already-cached OIDs are skipped, or a
  full rebuild.
- Location: under the git common dir (e.g. `<git-common-dir>/ws-cache/`),
  so every worktree of one clone shares it; the common dir is untracked by
  construction, so no exclude entry is needed. The cache is opaque, derived,
  and safe to delete.
- Format: a single append-only file (JSONL or gob) rather than a database;
  BM25 ranking already runs in memory, and the cache only replaces the
  `git log` parse. Concurrent writers use write-temp-then-rename; a corrupt
  or unreadable cache is rebuilt.
- Side effect: an incremental cache makes the `scanCap` truncation largely
  moot, which matters most for long-history downstream projects.
- Leave the ticket-file scan uncached for now (a blob-OID-keyed cache via
  `git ls-files -s` with dirty-file re-read is possible but ~90ms does not
  justify the invalidation logic).

## Outcome Ledger

### Verified Findings

- The terminal-ticket inventory scan costs roughly 90ms per `tickets.query`
  at 666 files; it is not the bottleneck.
- `rationale.query` spends about 1.1s of its 1.4–2.0s in
  `git log --name-only` over full history; without `--name-only` the same
  walk takes 0.13s.
- Every `queryScan` call re-walks full history regardless of `kinds`.
- History walk cost scales with commit count; deleting terminal tickets does
  not reduce it.
- History beyond 20,000 commits is truncated today.

### Confirmed Decisions

- Pursue the incremental history cache as research ahead of terminal-ticket
  GC; the user judged it the more urgent of the two.

### Proposals

- The commit-OID-keyed, tip-incremental, common-dir, append-only-file design
  in **Direction Discussed**.
- Leave `scanTickets` uncached.
- Lift or relax `scanCap` once the cache exists.

### Open Questions

- Why `kinds: [ticket]` measured slower (2.02s) than `kinds: [commit]`
  (1.43s): run-to-run noise, `buildRecords` thread matching, or something
  else. Profile before designing.
- Remaining non-git cost after caching (record building, BM25 indexing):
  measure to confirm the expected drop to tens of milliseconds; that figure
  is an estimate, not a measurement.
- Whether other history walkers (ticket-thread `git log --grep` lookups,
  verification or route resolvers) should share the same cache, and whether
  that widens the cached record shape.
- Whether pickaxe or site modes can use the cache at all (their answers depend
  on diff content), or stay uncached.
- How the cache interacts with shallow clones and grafted/replaced history.
- Cache format versioning so a ws upgrade that changes parsing invalidates
  old entries.

### Rejected Alternatives
