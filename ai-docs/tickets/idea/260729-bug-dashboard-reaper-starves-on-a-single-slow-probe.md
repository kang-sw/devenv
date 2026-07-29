---
title: One slow helper starves the whole terminal reaper, then the backlog fires back-to-back
related:
  260729-bug-dashboard-agent-profile-gc-destroys-concurrent-daemon-state: surfaced-by
---

## Symptom

`sweep_registry_backstop` is serial over registry entries and each entry may now
cost up to `connect_timeout + PROBE_EXCHANGE_TOTAL_TIMEOUT` (~21 s) before the
probe is abandoned. One entry that reliably hits that bound therefore delays every
later entry on every tick.

`terminal_reaper.rs` drives the sweep with `tokio::time::interval` at 10 s and
leaves the default `MissedTickBehavior::Burst`, so once a tick overruns, the
accumulated ticks fire immediately one after another - re-hitting the same slow
entry with no spacing.

`sweep_evict_expired` runs first inside `sweep_once`, so it is starved too, not
just the backstop.

## Finding

Surfaced by round-4 review of Phase 1 of
`260729-bug-dashboard-agent-profile-gc-destroys-concurrent-daemon-state`. That
phase deliberately made `ProbeVerdict::Abandoned` **not** authorize reclaim - the
daemon hitting its own bound says nothing about the helper, and turning impatience
into a SIGKILL is the defect that ticket exists to remove. That decision is
correct and should not be revisited here.

Its consequence is what this ticket owns: an entry that always yields `Abandoned`
is permanently unreclaimable *and* permanently expensive. That ticket records the
no-reclaim half as intentional; the reaper-timing half was never anywhere.

The equivalent startup path was bounded in the same phase
(`BOOT_RECONCILE_TOTAL_BUDGET`, 30 s aggregate). The periodic path got no
equivalent.

## Directions worth considering

Not a decision, and not ordered by preference:

- An aggregate budget per sweep pass, mirroring what `boot_reconcile` received.
  Entries not reached are left untouched - they must not be reclaimed for having
  been skipped.
- `MissedTickBehavior::Delay` or `Skip` so an overrun does not produce a burst.
  Cheap and independent of the budget question.
- Probing concurrently rather than serially, which changes the cost model from
  sum to max but introduces a fan-out this ticket has not sized.
- Remembering entries that abandoned recently and backing off on them
  specifically, rather than paying full price every 10 s.

## Before deciding

Establish whether a real helper can produce a sustained `Abandoned` at all, or
whether it requires a wedged/pathological peer. If only the latter, the priority
is low and `MissedTickBehavior` alone may be the whole fix. If a healthy helper
with a very large retained ring can do it under load, the budget matters.
