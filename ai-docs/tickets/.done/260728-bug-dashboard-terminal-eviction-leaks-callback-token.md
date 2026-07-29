---
title: "TerminalRegistry::insert's eviction path leaks the evicted terminal's callback token"
related-mental-model:
  - terminal
completed: 2026-07-29
---

# TerminalRegistry::insert's eviction path leaks the evicted terminal's callback token

## Background

`TerminalRegistry` has five `self.sessions.write()` call sites (enumerated by
`terminal.rs::sessions_write_lock_sites_are_enumerated`). Four of them are now
consistent about the two removal obligations a dropped session owes:

- `insert_unchecked` adds only, and owes nothing.
- `remove`, `remove_for_work_roots`, and `drain_all` each discharge BOTH the
  callback-token obligation (`forget_token`, which drops the in-memory
  `self.tokens` entry and best-effort deletes `terminal-tokens/<id>.json`) and
  the attention obligation (`attention.forget`).

The fifth, `insert`'s own eviction `retain` at
`ws-dashboard/crates/daemon/src/terminal.rs` (the `evicted_ids` loop, under the
`CONTRACT (260725 Phase 5 review cycle 1, finding A)` comment), discharges only
the attention half. A non-live session evicted there is dropped from `sessions`
while its callback token stays in `self.tokens` and its
`terminal-tokens/<id>.json` file stays on disk.

The eviction path is reached when a terminal exits without ever going through a
browser `DELETE` — tab closed, agent CLI quit, helper crash — and a later
`insert` sweeps it out to make room. The token that survives remains a valid
credential for `POST /api/dashboard/terminals/{id}/turn-state` even though no
session by that id exists any more.

Recorded provenance: the same CONTRACT comment that fixed the attention half
explicitly deferred the token half ("Whether the callback-token half of this
same gap is also worth closing is Phase 4's inherited debt"), and no follow-up
ticket was opened for it. This ticket is that follow-up. The gap predates both
the goal branch and `ws-dashboard-dev`, so it is not merge fallout.

## Constraints

- Do not fold a fix into `260727-chore-merge-ws-dashboard-dev-into-goal-branch`.
  That ticket's Phase 3 deliberately scoped itself to `drain_all` and routed
  this gap here instead; closing it is a behavior change with its own blast
  radius.
- Before implementing, decide whether the surviving token is actually
  exploitable end to end. `post_terminal_turn_state` authenticates against
  `self.tokens` — confirm whether it also requires a live session lookup, since
  that would downgrade this from a live-credential leak to disk/memory hygiene
  and changes how urgent the fix is.
- A fix must keep `insert`'s existing lock discipline: the eviction runs under
  the `sessions` write guard, and `forget_token` does synchronous disk I/O, so
  the token forgets must happen after that guard is released (same
  lock-hold-duration rule the `drain_all` CONTRACT states).
- Any fix updates the enumerating CONTRACT comment on
  `sessions_write_lock_sites_are_enumerated` so the fifth site's discharge
  status stops reading "attention only".

## Open Questions

- Is the leaked token reachable as a live credential, or inert because the
  route also needs a session? (Determines bug severity vs. hygiene.)
- Should the fix reuse `forget_token` per evicted id, or should eviction route
  through a shared removal helper so a sixth removal path cannot reintroduce
  the same asymmetry?


## Resolution (2026-07-29)

Discharged while reconciling `goal/ws-dashboard-dev/copper-heron-vale` with the
newly-merged `ws-dashboard-dev` line (merge `47314c17`).

The reconciliation had to make every session-removal path honour the incoming
line's `forget_token` + `attention.forget` contract, which is exactly this
ticket's ask. Rather than duplicating both calls at `insert`'s retain and at the
new `sweep_evict_expired`'s retain, `evict_and_close` became the single eviction
discharge point — both retains already funnel through it unconditionally — and
`sweep_evict_expired` carries a CONTRACT stating its return value must reach it.

Covered by `eviction_forgets_the_callback_token_and_the_attention_entry`
(`terminal.rs`), which drives `sweep_evict_expired` directly. A sibling test for
the `insert` retain path was also repaired: its fixture marked the session
`Exited` only, which under the corrected `admits_attach()` eviction gate is still
inside its attach grace, so nothing was being evicted and the assertion passed
vacuously. The fixture now expires the grace window and carries a non-vacuity
guard.

The on-disk `terminal-tokens/<id>.json` half is reclaimed by the existing profile
GC once the id leaves the live set. The separate gap that `terminal-tokens/` has
no independent reclaim path (a daemon killed between `remove_dir_all` and
`delete_token` orphans the file forever) is unrelated to eviction and remains
open — it is slow monotonic growth of inert files, not a leak of live
credentials.
