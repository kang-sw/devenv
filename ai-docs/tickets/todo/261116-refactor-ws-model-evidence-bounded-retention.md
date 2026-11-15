---
title: Bound model-evidence retention to first, change points and the most recent N
related:
  261115-feat-ws-dashboard-observed-session-model: introduced the model-evidence directory and its Phase 1 Edition (7ad4a0627) records the current prune
---

# Bound model-evidence retention to first, change points and the most recent N

## Background

### Compatibility surface (stated first)

`model-evidence/` is a shared on-disk store under each dashboard session
directory (`<cache>/dashboard/sessions/<reply-id>/model-evidence/*.json`), so
any retention change is a cross-version surface under AGENTS.md
`### Compatibility Tiers`. Who touches it on develop `76d8f7971`:

- Writers, all through `wslive.AppendObserved`
  (`agents-plugin-tool/internal/wslive/observed.go:255`), which publishes
  write-once files with first-write-wins identity and prunes after a write
  (`observed.go:265-269`):
  - the MCP live layer: `internal/mcp/live_snapshot.go:594`;
  - hosted Pi live: `internal/mcp/pi_live.go:132`;
  - the agent runner's record writer: `internal/wsagent/record.go:476`.
- Readers:
  - `wslive.ReadObserved` (`observed.go:314`), which tolerates missing files
    and folds records through `SeedObserved` (`observed.go:149`) into
    `summarizeObserved` (`observed.go:191`);
  - `ObservedCache.Load` (`internal/wslive/observed_cache.go:98`, used by
    `internal/wsdashboard/snapshot_cache.go:133`);
  - the MCP snapshot seed (`internal/mcp/live_snapshot.go:221`);
  - the session fingerprint in `internal/wsdashboard/agent_sessions.go:224`,
    which lists names only.
- `summarizeObserved` yields per agent `Current` (the latest record) and
  `History` (the first record plus every model change). It is unchanged since
  4dc1e7c35, which added the directory. Binaries before 7ad4a0627 never prune.

### What exists today

7ad4a0627 (merged in 34ae81150) added `pruneObserved`
(`internal/wslive/observed_prune.go:51`). The prune runs only once the
directory holds more than `observedPruneLimit = 2256` records
(`observed_prune.go:30`). Per agent it keeps the first record, every model
change and the latest record. It deletes another record only when its `At`
and its file mtime are both more than `observedPruneAge = 1h` older than that
agent's newest record (`observed_prune.go:33`, `:139-145`). It prunes at most
once per directory per process every 10 min (`observed_prune.go:39`).

### Measured growth (2026-11-16, this machine, read-only)

- 8 session evidence directories hold 10,307 files in total. Each file uses
  one ~4 KB block on disk (13.6 MB for 3,403 files).
- The largest, MAIN's lead session, holds 3,430 records over a 13.6 h span,
  from 128 agents: the lead's 679 records plus 2,751 records from 127
  children. Children live a median of 2 min and at most 30 min.
- Under the current rule, **0** of those 3,430 records can be pruned, although
  only 256 are a first, a change point or a latest record:
  - **Children (2,751 records):** every child repeat lies within 1 h of that
    child's own newest record, permanently. A short-lived agent's repeats are
    therefore never pruned.
  - **Lead:** 576 lead repeats have an `At` older than 1 h but a file mtime
    under 1 h old, so the mtime guard holds them.
  - **Burst (INFERRED cause):** 557 files landed in one 10-minute bucket about
    35 min before measurement. A replay re-adding records that an earlier
    prune deleted would match this, the churn case 7ad4a0627's tests cover.
- Net effect: the directory stays above the limit, and growth is bounded
  only by each agent's own 1-hour window.

## Decisions

- Retention keeps, per agent, the first record, every model-change point, and
  the most recent N records; everything else is deletable. (Settled in the
  owner discussion 2026-11-15; N itself is open below.)
- The ticket states its compatibility tier before implementation, citing the
  writer/reader lines above for which version writes and reads what, and
  whether reopening the new version recovers. (AGENTS.md
  `### Compatibility Tiers`.)

## Open Decision Queue

last-id: ODQ-5
Board ids are minted when MAIN raises these to the owner.

(ODQ-1) [open] [policy] The value of N (most recent records kept per agent)
- Context: `Current` needs only the latest record. Any N >= 1 keeps
  `summarizeObserved` output identical, so N buys only debugging/forensic
  depth and a margin for late replays.
- Alternative: a small fixed N (for example 8 or 32).
- Alternative: N tied to the existing ring bound.
- Alternative: N configurable via a config knob.
> Recommendation pending owner input; not decided by this capture.

(ODQ-2) [open] [policy] Whether N is per agent or per directory
- Context: the settled direction reads "the most recent N". 127 short-lived
  children dominate the measured directory, so a per-agent N multiplies by
  the agent count, while a per-directory N could drop a child's only latest
  record. That would change `Current` for that child unless the latest is
  always kept.
- Alternative: per agent.
- Alternative: per directory, with each agent's latest always kept.
> Not decided by this capture.

(ODQ-3) [open] [policy] Where and when pruning runs
- Context: today it runs after a write in `AppendObserved` above 2256
  records, at most every 10 min per directory per process. Other places it
  could run: the daemon (single owner, no cross-process race), a dashboard
  sweep, or session close/expiry. It could also run unconditionally on each
  append once over N, rather than only above a directory limit.
- Alternative: keep append-time, drop the 2256 threshold.
- Alternative: move to a daemon sweep.
- Alternative: run at session close.
> Not decided by this capture.

(ODQ-4) [open] [policy] Whether the 1 h At-and-mtime guard stays
- Context: the guard (`observed_prune.go:139-145`) is what makes every repeat
  of a short-lived agent permanent, and fresh mtimes from a replay burst hold
  old lead repeats. It exists to avoid re-dating a change when a late record
  lands inside a pruned span. Keeping the most recent N per agent may cover
  the same risk for in-order replays.
- Alternative: keep as is.
- Alternative: drop it.
- Alternative: keep only the `At` half.
- Alternative: shorten it.
> Not decided by this capture.

(ODQ-5) [open] [policy] The compatibility tier of the change
- Context: the readers listed in Background fold records through the
  unchanged `summarizeObserved`. Pre-7ad4a0627 binaries never prune and
  tolerate missing files (`observed.go:312-314`). If the keep set always
  contains each agent's first, change points and latest, old and new
  binaries show identical `Current` and `History`, which supports a T0
  reading as in 7ad4a0627. Any variant from (ODQ-2)/(ODQ-4) that can drop an
  agent's latest record, or re-date a change, changes what an old binary
  shows and needs its own verdict.
- Alternative: T0, the keep set always a superset of first + changes +
  latest.
- Alternative: T1 or higher, if a chosen variant can change displayed
  `Current`/`History`.
> Not decided by this capture; settle together with (ODQ-2) and (ODQ-4).

## Constraints

- Every reader must keep seeing identical `Current` and `History` for every
  agent before and after a prune, unless (ODQ-5) settles otherwise. The
  7ad4a0627 equivalence tests (one agent, three agents, child replay then a
  second prune, concurrent reads, failed deletes) remain the baseline and
  must not be weakened.
- Deletes stay limited to regular `.json` files directly inside
  `model-evidence/`; unreadable, invalid or identity-losing files are never
  deleted (`observed_prune.go:46-50`).
- Manuals: `ai-docs/manuals/release-compatibility.md` (cross-version store
  shape) applies to `agents-plugin-tool/internal/`.

## Phases

### Phase 1: Retain first, change points and the most recent N

Implement the retention settled in `## Decisions` with the values the Open
Decision Queue settles, replacing or adjusting the current rule in
`internal/wslive/observed_prune.go`. Verification: extend the existing prune
tests with a short-lived-agent case (many repeats within minutes) and a
replay-burst case (fresh mtimes on old `At`). Both must end with at most
first + changes + N per agent, while the `SeedObserved`/overlay equivalence
assertions still hold. Measure the largest local evidence directory before
and after, read-only, as in Background.
