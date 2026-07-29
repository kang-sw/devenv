---
title: macOS terminal socket path is unguarded, and the EPERM CONTRACT overstates its own risk
sage-review-design: required
spec:
  - 260516-ws-web-dashboard-workroot-io-restore-model
  - 260728-terminal-helper-periodic-reap
related:
  260726-refactor-ws-dashboard-long-uptime-leak-hardening: landed-UnverifiableBoot-and-the-handshake-failure-cleanup-this-ticket-defers-to
---

# macOS terminal socket path is unguarded, and the EPERM CONTRACT overstates its own risk

Found by code review of PR #4's macOS terminal-platform port
(`goal/ws-dashboard-dev/velvet-arbor-quill`, merged as `1b41a37b`), then
corrected by this ticket's own design review. The port itself is sound — it fixed
a state where macOS could never spawn a terminal at all.

## Background

### 1. The terminal socket path has no length guard in production

`tests/terminal_lifetime.rs` and `tests/routes.rs` both document the macOS
`AF_UNIX` `sun_path` ceiling and work around it by forcing a short temp dir.
Production has no equivalent guard: `default_registry_dir()` joins `terminals/`
onto `default_state_dir()`, and the daemon builds the socket path inline as
`registry_dir.join(format!("{id}.sock"))`, where `id` is `term_` + 18 chars = 28
bytes with the extension.

Defaults fit — `$HOME/.local/state/ws-dashboard/terminals/…` is roughly 72 bytes
plus the username, and the `$TMPDIR` fallback about 88 — but
`WS_DASHBOARD_STATE_HOME`, `XDG_STATE_HOME` and `WS_DASHBOARD_STATE_FILE` are
user-supplied and unbounded.

Failure scenario: a macOS user sets a deep `WS_DASHBOARD_STATE_HOME`.
`IpcListener::bind` fails with `EINVAL` inside the detached helper, and the
daemon reports "helper wrote a registry entry but the daemon could not connect or
complete the handshake" — pointing at handshake timing rather than a path-length
limit. Every terminal fails, permanently, with no actionable diagnostic.

**Correction from design review:** an earlier draft of this ticket claimed the
failure also strands a `<id>.json` behind. It does not — the daemon's
handshake-failure cleanup (landed by
`260726-refactor-ws-dashboard-long-uptime-leak-hardening`) reads the entry back
and prunes both files, and the periodic backstop would drop it as
`NoSuchProcess` regardless. **The only residue is the misattributed diagnostic**,
and that is what this fix must be justified by. Do not re-introduce the
stale-entry framing.

### 2. The EPERM branch is safe; its CONTRACT comment is what is wrong

`terminal_platform.rs` documents that `proc_pidinfo` returns `EPERM` for a
cross-user pid — yielding `None`, hence `NoSuchProcess` — where Linux would
report `PidReused`, and calls this "harmless today because both map to
drop-only".

An earlier draft of this ticket claimed the entry is deleted *while the helper
keeps running*, making it permanently unreclaimable. **Design review showed that
state cannot arise, and the code agrees:** the daemon spawns the helper through
`unix::spawn_detached` with no uid change — verified, there is no `setuid`/
`setgid` anywhere in `terminal_platform.rs` — so a *live* helper is always
same-uid and `proc_pidinfo` succeeds against it. A cross-user `EPERM` therefore
means the recorded pid is **no longer our helper**; it was recycled by another
user's process, the helper is already gone, and deleting the entry is the correct
outcome.

So there is no unreclaimable-helper defect here. What remains is that the
CONTRACT comment states a conclusion ("harmless") without stating the reasoning
that makes it true, which is exactly how a future reader talks themselves into
"fixing" a correct branch. This ticket's Phase 2 is that correction, and nothing
more.

## Decisions

**Phase 2 is doc-only. The behavior change is deliberately not built.** The
earlier draft offered "at minimum a CONTRACT edit, better a behavior change";
that framing is withdrawn, because the behavior change would have been built on a
premise the code contradicts.

This also settles the `UnverifiableBoot` question raised during review: there is
no second "cannot determine" state to introduce, so nothing needs reconciling
with the drop-only `UnverifiableBoot` outcome landed in `e6caac0d`. Note that
outcome drops deliberately — the 10s sweep would otherwise re-evaluate a stale
entry thousands of times a day — so a "retain and retry" rule would have
reversed a landed decision without a retention bound. Do not revive it.

**Open for Phase 1: guard, or remove the constraint?** The ticket commits to
diagnose-and-fail, which leaves terminals non-functional on an affected host,
just with a better message. The alternative is to decouple the socket path from
the user-supplied state home — a short socket dir with the `.json` still in the
state home — which is what the test suite already does as a workaround. That
removes the failure mode instead of reporting it. Weigh this before implementing;
the guard is still needed either way as a backstop, but it should not be the
whole answer if the decoupling is cheap.

## Constraints

Neither area is reproducible on Linux CI. Any test must either be
`#[cfg(target_os = "macos")]`-gated and honestly labelled as unexecuted
elsewhere, or restructured so the pure decision function is testable without the
platform syscall. Given the macOS-only constraint, naming that pure function is
what keeps this from becoming "untested in practice".

## Spec Impact

Target area: `ai-docs/spec/ws-web-dashboard/index.md`.

- **Phase 1** adds a stated bound on the terminal socket path and the observable
  failure when a user-supplied state home exceeds it. Caller-visible because the
  current failure is *misattributed* — the fix changes what the operator is told.
  Nearest existing anchor is `{#260725-ws-web-dashboard-terminal-spawn-profile}`,
  which owns the helper's spawn-side contract.
- **Phase 2** corrects prose, and the prose to correct is **not** in
  `260725`. The caller-observable boot-reconcile outcomes live under
  `{#260516-ws-web-dashboard-workroot-io-restore-model}`, and the periodic-reap
  paragraph is `{#260728-terminal-helper-periodic-reap}`.

While in the restore-model section: its sentence currently reads "kill a helper
whose identity cannot be verified …", which **misstates the code**. Kill applies
to verified-ours + IPC-dead; an unverified identity is never killed. Phase 2 is
the natural place to correct that, and it is a real spec/code divergence rather
than a wording preference.

## Phases

### Phase 1: Guard the terminal socket path length

The guard must run **daemon-side, before `spawn_detached`**, and feed the
create-terminal error. Placement matters more than it looks:

- A check inside the helper emits nothing an operator can see — the helper is
  spawned with all three streams to `/dev/null` and dispatches before
  `logging::init`, so it would surface as the same generic 400 plus the same
  misattributed handshake log.
- A check in `terminal_registry_file.rs`'s `registry_socket_path` covers
  **cleanup only** and changes nothing about the reported failure, because that
  helper is not on the spawn path — the daemon builds the socket path inline.

Use the correct maxima: `sun_path` is 104 bytes on Darwin and 108 on Linux
**including the NUL terminator**, and Rust's `UnixListener::bind` rejects
`path.len() >= sun_path.len()`, so the usable maxima are **103 and 107**. A guard
written literally from 104/108 admits exactly the boundary path that still fails
to bind — reproducing the misattributed diagnostic this phase exists to
eliminate. Windows has no such ceiling (`IpcListener` is a named pipe derived
from the path stem), so the guard must be `cfg`-gated, not applied uniformly.

Done when a state home deep enough to overflow the ceiling produces an error
naming the limit, the measured length, and the offending path, instead of the
handshake message.

Verification: a unit test over the pure length-check function at both ceilings
**and at the off-by-one boundary** (103/104 on macOS, 107/108 on Linux), driven
by a constructed path rather than a real bind so it runs on Linux CI. Reverting
the guard must fail the boundary case specifically — a test that only covers an
obviously-too-long path would still pass against the 104/108 mistake.

### Phase 2: Correct the EPERM CONTRACT and the restore-model spec sentence

Doc-only, per Decisions. Two edits:

1. `terminal_platform.rs`'s EPERM CONTRACT: state *why* the branch is harmless —
   a live helper is always same-uid because the daemon spawns it with no uid
   change, so `EPERM` implies the pid is no longer ours and dropping the entry is
   correct. Replace the bare "harmless today" conclusion with that reasoning.
2. `{#260516-ws-web-dashboard-workroot-io-restore-model}`: correct the "kill a
   helper whose identity cannot be verified" sentence to match the code — kill
   applies to verified-ours with a dead IPC channel; an unverified or
   undeterminable identity is dropped, never signalled.

No behavior change, no new identity state, no test beyond confirming the existing
suite still passes.
