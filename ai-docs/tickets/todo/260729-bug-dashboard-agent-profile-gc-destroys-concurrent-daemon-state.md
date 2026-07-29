---
title: agent_profile_gc destroys a concurrent daemon's live profiles and tokens
sage-review-design: required
---

# agent_profile_gc destroys a concurrent daemon's live profiles and tokens

## Background

Found by code review of PR #4 (`goal/ws-dashboard-dev/velvet-arbor-quill`, merged as
`1b41a37b`). Not a merge blocker — it needs a shared-state-dir setup to trigger —
but it is data-destructive and, worse, silent when it fires.

`agent_profile_gc` derives liveness solely from its own registry
(`registry.live_terminal_ids()`), while `agent-profiles/` and `terminal-tokens/`
live under the process-global `persistent_state::default_state_dir()` with no
per-daemon namespacing. The PR itself already named this scenario when it fixed a
shared temp-file race in `agent_callback.rs` ("two daemons sharing a
`WS_DASHBOARD_STATE_HOME`"), so the hazard is recognised elsewhere in the same
change but not closed here.

Failure scenario:

1. A developer has `ws-dashboard serve` running, then starts a second instance in
   another window without setting `WS_DASHBOARD_STATE_HOME`.
2. Every terminal spawned in window B *after* window A's `boot_reconcile` is
   absent from A's live set.
3. A's 300 s sweep `remove_dir_all`s `agent-profiles/<id>/` and deletes
   `terminal-tokens/<id>.json` for those ids.
4. B's hooks break immediately — `callback.json` is gone, so every
   `terminal-notify` fire fails with "callback file not found".
5. On B's next restart `recover_callback_token` returns `None`, so those adopted
   terminals never authenticate again.

The escalation designed to surface exactly this class of breakage cannot fire:
`notify_failure::record_failure` no-ops when the profile directory is absent, and
the profile directory being absent is precisely the failure state. Nothing is
logged on B.

The pre-existing shared `terminals/` registry dir already makes a two-daemon
shared-state-dir setup partly ill-defined; this change is what makes the conflict
destructive rather than merely confusing.

## Decisions

Open. Candidate directions, to be settled at design review:

- Namespace `agent-profiles/` and `terminal-tokens/` per daemon instance so one
  daemon's sweep structurally cannot see another's entries.
- Keep the shared layout but make the sweep refuse to reclaim anything it cannot
  positively attribute to itself (e.g. an owner stamp written at profile
  creation), so an unattributable entry is skipped rather than deleted.
- Detect the shared-state-dir case at startup and refuse to run the GC (or refuse
  to boot the second daemon) with an explicit message.

The third option is the cheapest and is honest about the setup being
unsupported; the second is the only one that also fixes the pre-existing
`terminals/` ambiguity. Do not pick by cost alone — decide whether concurrent
daemons on one state dir are a supported configuration at all, and let that
answer drive the mechanism.

Independently of which is chosen: `record_failure`'s no-op-when-absent behaviour
should be revisited, because a mechanism whose stated job is to bound silence
cannot report its own worst case.

## Phases

### Phase 1: Decide the concurrent-daemon contract and close the destructive path

Settle whether two daemons sharing one state dir is supported, then implement the
matching mechanism from Decisions. Cover with a test that actually reproduces the
cross-daemon deletion (two registries over one state dir, sweep on one, assert the
other's profile and token survive) — a test that passes with the guard reverted is
worthless here.
