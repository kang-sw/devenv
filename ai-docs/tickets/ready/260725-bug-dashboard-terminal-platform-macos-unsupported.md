---
title: dashboard daemon does not build on macOS — the "unix" terminal platform layer is Linux-only
related:
  260723-feat-dashboard-terminal-lifetime-daemon-decouple: introduced-by; landed the detached-helper identity/kill model as "Unix + Windows", but its Unix leg is Linux-only
  260724-bug-dashboard-terminal-dead-shell-undetected-steady-state: sibling platform gap; established the per-platform dead-shell detection story that macOS has never been checked against
  260622-chore-windows-shipping-hardening: precedent for treating a platform as shipping-incomplete until it builds and runs natively
sage-review-design: completed
sage-review-completeness: completed
---

# dashboard daemon does not build on macOS — the "unix" terminal platform layer is Linux-only

## Background

`cargo build -p ws-dashboard-daemon` fails outright on macOS (verified
2026-07-25, aarch64-apple-darwin, cargo 1.95.0, on branch `ws-dashboard-dev`
at `507f99d1`):

```
error[E0425]: cannot find value `SYS_pidfd_open` in crate `libc`
  --> crates/daemon/src/terminal_platform.rs:83:50
error[E0425]: cannot find value `SYS_pidfd_send_signal` in crate `libc`
  --> crates/daemon/src/terminal_platform.rs:96:27
```

`crates/daemon/src/terminal_platform.rs` splits its syscall leaves into a
`#[cfg(unix)]` module and a `#[cfg(windows)]` module, and re-exports the four
names `spawn_detached`, `process_start_time`, `verify_process_identity`, and
`kill_verified` from whichever applies. The `unix` module is **not** portable
Unix — it is Linux-only in two independent ways:

1. `kill_verified` calls `libc::SYS_pidfd_open` / `libc::SYS_pidfd_send_signal`.
   These are Linux syscall numbers and are simply absent from `libc` on macOS,
   so the crate does not compile at all.
2. `process_start_time` parses `/proc/<pid>/stat` field 22. macOS has no
   `/proc`. Even with (1) patched, this returns `None` for every pid, so
   `verify_process_identity` is permanently `false` and the entire identity
   model silently degrades to "never verifies".

Failure mode (2) is the dangerous one, because it is **silent**. Two call
sites would quietly lose their safety property rather than fail loudly:

- `terminal_helper_process.rs:174` records the helper's own identity as
  `process_start_time(pid).unwrap_or(0)`. On macOS every helper would record
  start-time `0`.
- `boot_reconcile` (`terminal.rs:352`) and the fallback kill paths
  (`terminal.rs:261`, `terminal.rs:1038`) gate on that recorded value, so
  helper re-adoption after a daemon restart and identity-verified termination
  would both misbehave rather than error.

Consequence: `260723-feat-dashboard-terminal-lifetime-daemon-decouple` closed
as "both platforms landed (Decision B)", but its real coverage is **Linux +
Windows**. The `unix` module name made macOS look covered when it has never
been compiled. macOS should be feature-complete (owner directive, 2026-07-25),
so this is a shipping gap, not a nice-to-have.

Also Linux-only and in scope: `#[cfg(all(test, unix))] mod unix_tests`
asserts `/proc`-derived behavior (`process_start_time` stable for self, `None`
for an implausible pid) and will fail on macOS as written.

## Decisions

### macOS verified-kill mechanism: verify-then-`kill(2)`, with a post-kill identity re-check

**Decided 2026-07-25 (owner asked for the cleanest option, not an open
choice).** macOS has no `pidfd`, and no macOS primitive provides pidfd's actual
property — *signalling through a stable kernel reference to a process
instance*. Every available option therefore leaves a verify→signal window;
the choice is which one to pay for.

Chosen: verify the recorded start-time immediately before signalling, then
`kill(pid, SIGKILL)`, then re-read identity as a **best-effort** post-kill
check.

**Start-time source: `proc_pidinfo(pid, PROC_PIDTBSDINFO, ...)` →
`proc_bsdinfo.pbi_start_tvsec` / `pbi_start_tvusec`.** Not `sysctl`
`KERN_PROC_PID` → `kinfo_proc.kp_proc.p_starttime`, which was this ticket's
first draft: verified against the pinned `libc 0.2.186`, its apple target
defines `proc_pidinfo`, `proc_bsdinfo`, `pbi_start_tvsec`, and
`PROC_PIDTBSDINFO`, but defines **no** `kinfo_proc`, `extern_proc`, or
`p_starttime`. The sysctl route would therefore require hand-transcribing a
large, layout-sensitive, kernel-ABI-offset-dependent struct in a crate that
hand-declares no other FFI layouts. `proc_bsdinfo` yields the same value with
no hand-declared struct, and additionally carries `pbi_status`, which the
post-kill check below needs.

**What the post-kill re-check does and does not buy.** It is *not* a reliable
mis-kill detector, and this ticket should not be read as claiming otherwise.
Walk the actual failure: verify passes against the right process → that
process exits → the pid is recycled → `kill` hits the new occupant → the
re-read most often sees the pid simply gone, which is indistinguishable from
a correct kill. Detection only fires in the sub-case where the victim is still
observable afterwards. The check is worth keeping because it is nearly free
and turns *some* otherwise-invisible failures into logged ones — it is
best-effort hardening, not a guarantee, and the spec amendment below is worded
accordingly.

The re-check must distinguish three post-kill outcomes rather than treating
"identity still readable" as failure:

- pid gone → normal success.
- identity still readable **with the original start-time** → check
  `pbi_status` for `SZOMB`; an unreaped zombie of our own target is a normal
  success, not a mismatch. This case is on Phase 1's critical path: the
  existing test `kill_verified_kills_on_matching_identity`
  (`terminal_platform.rs:414`) spawns `sleep 5` as a **direct child** and only
  calls `child.wait()` *after* `kill_verified` returns, so the post-kill read
  legitimately observes a zombie carrying the expected start-time. A naive
  "still readable ⇒ mismatch" rule would fail that test and violate this
  phase's own green-`cargo test` exit criterion.
- identity readable with a **different** start-time → a genuine pid-reuse
  signal; log it.

**Channel for "report loudly": a log emission, not the return value.** Both
fallback call sites discard the result (`let _ = tokio::task::spawn_blocking(…)`
at `terminal.rs:261` and `terminal.rs:1038`), so nothing observes
`io::Result<bool>` today. Changing the return type would pull caller updates
into scope that this ticket does not budget; emit a log instead.

Rejected alternatives, with reasons a future session should not have to
re-derive:

- **kqueue `EVFILT_PROC` / `NOTE_EXIT`.** Superficially the closest analogue,
  and genuinely stronger than a bare start-time re-read in one respect:
  registration binds to the *process instance*, so a pending `NOTE_EXIT` after
  a successful registration unambiguously means that instance exited. But it
  is an **exit-notification filter, not a signalable handle** — there is no
  `kevent`-based send-signal path, so the terminating `kill(2)` is still a
  separate non-atomic syscall. It narrows the window; it does not close it.
  Rejected because it buys a kqueue fd lifecycle, `ESRCH`-on-registration
  handling, and a more complex signature for a guarantee it does not actually
  deliver.
- **`task_for_pid` (Mach task port).** Would be a genuine stable reference,
  but obtaining a task port for a non-descendant process is gated by
  `taskgated` / entitlements and typically requires code signing or root. Not
  viable for a locally built, unsigned personal daemon. Rejected.
- **Stop double-forking so the daemon stays the helper's parent** (a parent's
  unreaped zombie holds the pid, which would close the window for free).
  Rejected: `spawn_detached`'s `setsid()` + double fork is the mechanism that
  makes the helper outlive the daemon, which is the entire point of
  `260723`. Recorded here only so it is not re-proposed.

### The absolute spec guarantee becomes platform-tiered

`ai-docs/spec/ws-web-dashboard/index.md`
(`#260516-ws-web-dashboard-workroot-io-restore-model`) currently states the
fallback kill is

> "never a bare-pid re-resolve, so a pid reused by an unrelated process after
> the helper already exited is never mistakenly killed."

On Linux that is structurally true (pidfd). Under the decision above it is
**not** unconditionally true on macOS. Implementing the macOS leg without
amending that sentence would put the implementation in direct contradiction
with the spec, so the amendment is part of the deliverable, not optional
closeout polish: the guarantee must be restated as a platform tier (Linux and
Windows: structurally closed; macOS: verified immediately before the signal,
with best-effort post-kill detection). Do not word the amendment as if macOS
reliably detects a mis-kill — per the Decisions entry above it does not, and
an overclaiming spec sentence would recreate exactly the problem this
amendment exists to fix.

## Constraints

- The registry's recorded start-time field is a `u64` and is written by the
  helper, read by the daemon. Keep the field platform-opaque rather than
  introducing a per-platform registry schema: macOS can pack
  `pbi_start_tvsec`/`pbi_start_tvusec` into the same `u64`. Changing the
  registry schema would break `boot_reconcile` against registry files written
  by an older build.
- Do not "fix" the build by widening `#[cfg(unix)]` to a runtime branch.
  Follow the file's own established shape — syscall leaves live in
  cfg-gated submodules behind cfg-independent re-exported names.
- `process_start_time(pid).unwrap_or(0)` at
  `terminal_helper_process.rs:174` is a silent-degradation hazard on any
  platform where the lookup can fail. Whatever macOS does here, a failed
  identity lookup must not be recorded as a valid identity of `0`.
- `spawn_detached` (`setsid()` + double fork + reap the middle process) is
  already portable POSIX and is expected to work unchanged on macOS; it is
  not part of the port beyond verification.

## Spec Impact

Target spec area: `ai-docs/spec/ws-web-dashboard/index.md` —
`#260516-ws-web-dashboard-workroot-io-restore-model` (the fallback-kill
guarantee sentence quoted above, restated as a platform tier),
`#260516-ws-web-dashboard-terminal-registry-pty-spawn` (helper identity is
pid + process start-time; note that the start-time *source* is
platform-specific while the recorded value stays opaque), and
`#260516-ws-web-dashboard-terminal-cross-platform-evidence` (macOS becomes a
recorded supported environment, or an explicit stated gap).

Expected caller-visible change: none to any HTTP/WS route or payload. The
change is to a documented durability/safety guarantee — terminals surviving a
daemon restart, and the never-kill-a-recycled-pid promise — which becomes
tiered by platform instead of absolute.

Contract-first spec: no. The mechanism is fully pinned by the Decisions
section above, so planned spec text would restate the ticket rather than
stabilize an unsettled contract; the amendment lands with Phase 1 closeout.
The contradiction risk the gate exists to catch is handled explicitly by the
second Decision entry, which names the exact sentence and the required
amendment.

## Phases

### Phase 1: Make the daemon build and verify identity correctly on macOS

Split the Linux-only leaves out of the `unix` module and add the macOS leg so
`cargo build`/`cargo test -p ws-dashboard-daemon` are green on
aarch64-apple-darwin.

- Introduce the macOS start-time source
  (`proc_pidinfo`/`PROC_PIDTBSDINFO` → `proc_bsdinfo.pbi_start_tvsec` +
  `pbi_start_tvusec`), packed into the existing opaque `u64` registry field.
- Implement macOS `kill_verified` per the Decisions section: verify →
  `kill(pid, SIGKILL)` → best-effort re-read, discriminating the three
  post-kill outcomes (gone / zombie carrying the original start-time, via
  `pbi_status` `SZOMB` / different start-time), logging only the last as a
  pid-reuse signal. The return type stays `io::Result<bool>`.
- **Make a failed self-identity lookup fail loudly rather than recording a
  sentinel.** At `terminal_helper_process.rs:174`, have the helper return
  `Err` from `run_terminal_helper` *before* `write_registry_entry` when it
  cannot read its own start-time. Do **not** take the "record an explicit
  unverifiable state" route: `IdentityStatus` (`terminal_reconcile.rs:51`) has
  exactly three variants and no unverifiable case, so a recorded sentinel
  classifies a live healthy helper as `PidReused` → `DropPidReused` — registry
  entry dropped and a running helper orphaned forever, never killed and never
  re-adopted. Adding a fourth variant would also ripple into the reconcile
  table that spec `#260516-ws-web-dashboard-workroot-io-restore-model`
  describes by name as "a 6-row decision table", which `## Spec Impact` does
  not budget for.
- Re-gate `mod unix_tests`: its `/proc`-shaped assertions are Linux
  assertions. The platform-independent properties (start-time is stable for a
  live process; an implausible pid yields no identity) should hold on macOS
  too and are worth keeping as shared coverage.
- Keep the re-export block (`terminal_platform.rs:354-357`) exhaustive. It is
  total today only because `#[cfg(unix)]` is a catch-all; splitting into
  `target_os = "linux"` / `"macos"` makes any other unix target fail as a pile
  of unresolved-name errors. Add an explicit `compile_error!` arm so an
  unsupported target fails with one honest message.
- Amend the three spec anchors named in `## Spec Impact`.

Exit criterion is a green native macOS `cargo build` + `cargo test -p
ws-dashboard-daemon`, plus **named evidence** that Linux did not regress. The
Linux half needs stating because this change *moves* the Linux leaves out of
the `unix` module, so a mis-scoped `cfg` breaks Linux silently — and no Linux
target is installed on the dogfood host (`rustup target list --installed`
reports only `aarch64-apple-darwin`, `wasm32-unknown-unknown`,
`x86_64-pc-windows-msvc`). Acceptable evidence: `cargo check --target
x86_64-unknown-linux-gnu` after adding that target, or a container run. If
neither is available, record an explicit deferral naming the gap, following
the cross-compile-check-only precedent set by `260723`'s Windows leg — do not
let an untested Linux leg pass as verified. Note that this is the first
time this crate has ever been type-checked on macOS, so the two known errors
above may not be the only ones — treat newly surfaced macOS-only compile or
test failures as in-scope for this phase rather than deferring them, and
record any that are genuinely separate concerns as follow-up tickets.

Rejected approach: patching only the two compile errors and leaving
`/proc` parsing in place. That produces a daemon that builds on macOS but
whose helper re-adoption and verified kill are permanently inert — strictly
worse than the current honest build failure.

### Phase 2: Native macOS runtime acceptance for the helper lifecycle

Phase 1 proves the code compiles and unit-verifies; it does not prove the
detached-helper lifecycle actually works on macOS. This phase is separated
because its verification boundary is different — it needs a live run on a real
macOS host, and (following the Windows precedent in
`260724-chore-dashboard-windows-terminal-reaper-native-acceptance`) a
non-vacuity proof that the assertions can fail.

Cover the lifecycle legs that the Linux and Windows evidence already cover:
terminal spawn through the detached helper, helper survival across a daemon
restart with `boot_reconcile` re-adopting it under the same terminal id,
identity-verified termination on explicit close, and dead-shell detection
(macOS PTYs are expected to deliver EOF like Linux rather than needing the
Windows-style reaper — confirm rather than assume).

Record the outcome under
`#260516-ws-web-dashboard-terminal-cross-platform-evidence`, including any
OS-scoped limitation found. Depends on Phase 1.

Exit criterion: all four lifecycle legs (spawn, daemon-restart re-adopt,
identity-verified close, dead-shell detection) pass on a real macOS host, with
a non-vacuity proof that the assertions can fail, and the outcome recorded
under the cross-platform-evidence anchor. A leg that does not pass is recorded
as an explicit OS-scoped limitation under that same anchor — never silently
dropped, and never satisfied by a Linux run standing in for macOS.
