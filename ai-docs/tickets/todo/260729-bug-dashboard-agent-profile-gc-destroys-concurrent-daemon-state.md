---
title: Two daemons sharing a state dir destroy each other's live terminals every 10s
sage-review-design: blocked
sage-review-completeness: blocked
spec:
  - 260727-dashboard-terminal-notify-failure-visibility
  - 260725-ws-web-dashboard-terminal-spawn-profile
---

# Two daemons sharing a state dir destroy each other's live terminals every 10s

> The file stem says `agent-profile-gc` because that is where the first symptom
> was found. Design review showed the profile GC is the *slow* path; the title
> and body below carry the real scope. Stem left unrenamed to keep in-flight
> review references valid.

## Background

Found by code review of PR #4 (`goal/ws-dashboard-dev/velvet-arbor-quill`, merged
as `1b41a37b`), then substantially widened by this ticket's own design review.

Neither daemon namespaces its state. `agent-profiles/`, `terminal-tokens/` and
the pre-existing `terminals/` registry dir all live directly under the
process-global `persistent_state::default_state_dir()`. Two daemons started
without distinct `WS_DASHBOARD_STATE_HOME` values therefore share all three, and
each treats the shared contents as exclusively its own. Call them **A** and **B**
throughout; the roles are symmetric, so everything below runs in both directions
at once.

The trigger needs no unusual setup: a developer has `ws-dashboard serve` running,
then starts a second instance in another window without setting
`WS_DASHBOARD_STATE_HOME`. Every terminal spawned in B after A's
`boot_reconcile` is absent from A's live set, and vice versa. Both paths below
follow from that alone.

### The fast path: a 10-second sweep SIGKILLs the other daemon's live helpers

`TerminalRegistry::sweep_registry_backstop` (`terminal.rs`) runs every
`TERMINAL_REAPER_INTERVAL` (10 s) over the shared `terminals/` dir. For any entry
not in **its own** `live_terminal_ids()` and older than
`connect_timeout + STALE_ENTRY_SWEEP_MARGIN` (~2.4 s), it calls
`kill_verified_and_delete_entry`.

The other daemon's helper is a real, live process, so its recorded pid,
start-time and boot id all match and `identity_status` returns `VerifiedOurs` —
the kill branch. The sweep **deliberately never attempts a handshake** (its
CONTRACT: "a runtime sweep only ever kills, it never adopts"), so there is no
step at which it could discover the helper belongs to someone else.

Verified by reading the code, not inferred.

Result: in the shared-state-dir setup, **daemon B SIGKILLs every one of daemon
A's live terminals within about 10 seconds**, and keeps doing so. The boot-identity
gate added in `e6caac0d` provides no protection here — both daemons are on the
same boot, so the boot id matches by construction.

`boot_reconcile` reaches the same verdict at startup by a different route: the
helper serves one connection at a time and daemon A holds it persistently, so
daemon B's 400 ms `connect_and_handshake` times out, IPC reads as unreachable
with identity `VerifiedOurs`, and B kills A's live terminals at boot.

**This is an amplification of our own change.** Before `260726` Phase 1, only
`boot_reconcile` ran this decision — once per daemon start. The periodic backstop
turned it into a continuous runtime behaviour. This is the same amplification
pattern as the boot-relative `start_time` hazard that `e6caac0d` closed.

### The slow path: the profile GC deletes the other daemon's agent state

`agent_profile_gc` derives liveness solely from `registry.live_terminal_ids()`,
so its 300 s sweep `remove_dir_all`s `agent-profiles/<id>/` and deletes
`terminal-tokens/<id>.json` for the other daemon's terminals. B's hooks then fail
with "callback file not found", and on B's next restart `recover_callback_token`
returns `None`, so those adopted terminals never authenticate again.

In practice the fast path usually gets there first — the terminals are already
dead long before 300 s elapse — but the slow path is what leaves the state
unrecoverable across a restart.

### Why it is silent

The escalation built to surface exactly this class of breakage cannot fire:
`notify_failure::record_failure` no-ops when the profile directory is absent, and
an absent profile directory is precisely the failure state. Nothing is logged on
the victim daemon.

The PR itself already named this scenario when it fixed a shared temp-file race
in `agent_callback.rs` ("two daemons sharing a `WS_DASHBOARD_STATE_HOME`"), so
the hazard is recognised elsewhere in the same change but not closed here.

The pre-existing shared `terminals/` registry dir already makes a two-daemon
shared-state-dir setup partly ill-defined; this change is what makes the conflict
destructive rather than merely confusing.

## Decisions

**Open, and it is a product-scope decision, not an engineering one: are two
daemons on one state directory a supported configuration?** The branches lead to
opposite mechanisms and opposite spec contracts, so this must be answered before
Phase 1 starts.

Whatever is chosen must cover the `terminals/` registry dir, not just
`agent-profiles/`/`terminal-tokens/` — the registry dir is where the fast
destructive path lives.

Each candidate has a trap that design review surfaced. None is a clean pick:

**1. Namespace state per daemon instance.**
Collides with the restart-adoption contract. `recover_profile_id` and
`recover_callback_token` locate a re-adopted terminal's state at the *fixed*
paths `<state_dir>/agent-profiles/<terminal_id>/` and
`terminal-tokens/<terminal_id>.json`. Nothing stable identifies a daemon
instance across a restart — the pid changes and the port is ephemeral — so a
namespace keyed to an instance would make **every ordinary single-daemon
restart** fail to find the previous run's profile and token. That produces the
"adopted terminals never authenticate again" outcome this ticket exists to
prevent, unconditionally rather than only under a shared state dir. If this
branch is taken, state what the namespace is keyed by and what happens to it at
boot-reconcile adopt.

**2. Owner stamp, sweep refuses what it cannot attribute.**
Mirror-image trap: if the stamp is per-instance and is never re-stamped at adopt
time, no directory left by a previous run is ever attributable to the running
daemon, so the sweep stops reclaiming the orphans it was built for. Same
question applies — what identity is stamped, and what re-stamps it at adopt.
This is still the only candidate that also resolves the pre-existing `terminals/`
ambiguity rather than working around it.

**3. Detect the shared-state-dir case at startup and refuse.**
Cheapest and honest about the setup being unsupported, but the ticket cannot
currently say *how*: the daemon has no lock file, pid file, or instance marker
today — each binds an ephemeral port and writes a single shared
`bound-base-url.json`. This branch requires inventing an instance lock **plus** a
stale-lock-after-crash rule, neither of which is sketched here. It is also an
observable behaviour change for anyone deliberately running two daemons today.

Do not pick by cost alone.

**Second open decision, independent of the branch:** `record_failure`'s
no-op-when-absent behaviour must be revisited, because a mechanism whose stated
job is to bound silence cannot report its own worst case. This one is **not**
autonomous either. The behaviour is deliberate and spec-stated today
(`{#260727-dashboard-terminal-notify-failure-visibility}`: "The hook process
never creates the profile directory to write it"), so it falls under this repo's
"always ask" approval class — changing protocol semantics.

What replaces it is also unspecified, and the obvious candidate is the one the
spec deliberately rejected: having the hook create the profile directory. If that
is off the table, the failure has to surface somewhere the hook can reach without
the profile dir — a daemon-side log, a process exit signal the caller can see, or
a separate always-present failure sink. Naming which is part of the decision, not
of the implementation.

## Spec Impact

Target area: `ai-docs/spec/ws-web-dashboard/index.md`, alongside
`{#260725-ws-web-dashboard-terminal-spawn-profile}` (which already owns the
per-terminal registry identity contract) and
`{#260727-dashboard-terminal-notify-failure-visibility}` (which owns the
bounded-silence guarantee this defect voids).

No existing stem states what happens when two daemons share one state directory.
Whatever Phase 1 decides is caller-visible and needs a new anchor:

- If concurrent daemons on one state dir become **supported**, the spec must
  state the namespacing or attribution rule that makes one daemon's sweep unable
  to reclaim another's live state.
- If they become **explicitly unsupported**, the spec must state the detection
  point and the observable refusal (which daemon refuses, when, and with what
  message) — silence is what makes the current behaviour dangerous, so the
  refusal is the contract.

Either way, `{#260727-dashboard-terminal-notify-failure-visibility}` needs a
qualifying sentence: the bounded-silence guarantee does not hold when the profile
directory is absent, because `record_failure` no-ops in exactly that state.

## Phases

### Phase 1: Decide the concurrent-daemon contract and close the destructive path

**Blocked on the Decisions section's open question.** The three candidate
mechanisms are structurally different, so a fresh session cannot pick one from
this ticket alone. Settle the contract first; this phase cannot start before
then.

Once settled, implement the matching mechanism. Phase 1's completion boundary
explicitly includes all three of the following — an implementer who closes the
phase with only the first has not discharged it:

1. The chosen mechanism, so one daemon's sweep can no longer reclaim another's
   live profiles or tokens.
2. `record_failure`'s no-op-when-absent behavior. This is in scope regardless of
   which mechanism is chosen, because a bounded-silence guarantee that cannot
   report its own worst case is the reason this defect is invisible rather than
   merely present.
3. The spec anchor work named in Spec Impact — the new anchor for the chosen
   contract, and the qualifying sentence on
   `{#260727-dashboard-terminal-notify-failure-visibility}`.

Verification depends on the branch, so it cannot be fixed here:

- Branches 1 and 2: a test that reproduces the cross-daemon destruction — two
  registries over one state dir, run the 10 s backstop on one, assert the other's
  **live helper process** survives (this is the load-bearing one), then the same
  shape for the profile GC and its token file.
- Branch 3: nothing reaches a second sweep, so that assertion has no subject.
  The test is instead over the detection point and the refusal — that the second
  daemon refuses, that the message names the shared directory, and that a stale
  lock left by a crashed daemon does not wedge the next start.

In every branch, prove the mutation fails: revert the guard and confirm the test
goes red. A test that passes either way does not cover this defect.

## Blocked (2026-07-29)

### Design Reviewer — block

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | Whether two daemons on one state dir are supported is an open product decision the implementer cannot supply, yet it selects the mechanism, the spec contract, and the test shape | critical | missing |
| 2 | record_failure's replacement behavior is in scope but unspecified, and changing it edits a spec-stated contract (always-ask class) | important | missing |
| 3 | Each surviving candidate still has an unclosed sub-design (namespace key + adopt-time behavior for 1 and 2; instance lock + stale-lock rule for 3) | important | missing |

### Completeness Reviewer — pass

| # | Title | Severity |
|---|-------|----------|
