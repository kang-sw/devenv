---
title: The daemon SIGKILLs live helpers because it cannot tell a busy helper from a dead one
sage-review-design: completed
sage-review-completeness: completed
spec:
  - 260516-ws-web-dashboard-workroot-io-restore-model
  - 260727-dashboard-terminal-notify-failure-visibility
  - 260728-terminal-helper-periodic-reap
---

# The daemon SIGKILLs live helpers because it cannot tell a busy helper from a dead one

> The file stem says `agent-profile-gc` because that is where the first symptom
> was found. Two rounds of review moved the diagnosis twice: first from the 300 s
> profile GC to the 10 s registry sweep, then from "shared state dir" to the
> liveness test both of them get wrong. Stem left unrenamed; the title carries
> the real scope.

## Background

Found by code review of PR #4 (`goal/ws-dashboard-dev/velvet-arbor-quill`, merged
as `1b41a37b`). Every claim below was verified against source.

### The invariant being violated

Helper lifecycle is deliberately decoupled from daemon lifecycle: helpers outlive
their daemon, daemon exit must never kill them, and `boot_reconcile` adopts them
back on the next start. Reclamation is supposed to apply to **dead** terminals
only.

The daemon violates this. It kills helpers that are alive and healthily serving
another daemon — not because the kill policy is wrong, but because its test for
"dead" is wrong. This is a defect in the predicate, not in the intent.

### Why a live helper looks dead

`serve_connections` (`terminal_helper_process.rs`) awaits `handle_connection`
inline in its accept loop, so a helper serves **one connection at a time**. A
healthy helper's single connection is held by its owning daemon for the
terminal's lifetime.

A second daemon probing that helper gets:

- `connect` **succeeds** — the listener is bound once before the loop and stays
  listening while `handle_connection` runs. On Linux `listen(fd, -1)` gives the
  kernel maximum backlog, so the connection is queued, not refused. The Windows
  named-pipe leg arms the next instance before returning the current one, so it
  behaves the same.
- the handshake **never arrives** — the helper is busy and will not reach
  `accept()` until the first daemon disconnects.

So *connect succeeded but nobody answered* is the signature of a **healthy, busy**
helper. The code reads it as death.

### Where the misjudgment turns into a SIGKILL

Two of the four kill sites are defective. The other two are sound and must not be
touched (see below).

**Site A — `sweep_registry_backstop` (`terminal.rs`), every 10 s.** Kills on
`terminal_id ∉ self.live_terminal_ids()` plus `identity_status == VerifiedOurs`,
with **no liveness probe at all** — its CONTRACT says so deliberately
("Deliberately never calls `connect_and_handshake`"). Another daemon's live
helper is absent from this daemon's session map and its pid, start-time and boot
id all genuinely match, so it is killed. The boot-identity gate landed in
`e6caac0d` cannot help: both daemons are on the same boot, so the boot id matches
by construction. This is the fastest and broadest destructive path.

**Site B — `reconcile_entry` at daemon start (`terminal.rs`).** Does probe, but
throws the answer away. `connect_and_handshake` has six distinct failure paths
and returns a bare `None` from all of them; the `Err(_)` arm discards the
`io::Error` entirely. `reconcile_entry` folds every `None` into
`IpcStatus::Unreachable`, and `classify` maps `VerifiedOurs + Unreachable`
straight to `KillVerified`. "Socket file is gone" and "connected fine, helper
busy" become the same value.

The transport does surface the distinction — `terminal_ipc_transport::connect`
returns `io::Result`, so `NotFound` and `ConnectionRefused` are available at the
point where they are dropped. Nothing downstream can recover it, and `IpcStatus`
has no variant to express it.

Site B is why the destruction happens *at daemon start*, not 10 s later. In this
repo's own workflow that is the likely trigger: an agent starts a test daemon
without `WS_DASHBOARD_STATE_HOME` and the user's live terminals die immediately.

There are **four** sites that decide to kill, not five. The other two are sound:

- **Site C** — `TerminalSession::spawn`'s handshake-failure cleanup. Kills a
  helper this daemon *just spawned* under a freshly generated id, so it cannot be
  another daemon's busy helper. Ownership is not in question.
- **Site D** — `TerminalSession::terminate`. Writes `GracefulShutdown`, waits
  200 ms, then kills unconditionally. Identity is handshake-proven and the caller
  explicitly asked for the terminal to end.

`kill_verified_and_delete_entry` is **not** a fifth site — it is the shared
callee of A, B and C, re-applying the boot-identity gate and withholding the
signal on mismatch. It is a guard, not a decision. Counting it as a peer of its
own callers is what made an earlier draft say "five".

### The fourth destructive path: `agent_profile_gc` does not go through any of them

`agent_profile_gc` never calls `kill_verified`, so it is not one of the four kill
sites — but it destroys state using the **same wrong signal**. It derives
liveness from `registry.live_terminal_ids()` directly, so fixing `classify` and
the sweep does not fix it: another daemon's terminals stay absent from this
daemon's session map even after they correctly survive Sites A and B. It must be
gated on the same probe, or Phase 1 leaves the state-destruction half open while
appearing to close the ticket.

### Why the kill path cannot simply be removed

Verified, and it corrects an earlier assumption in this ticket: **an orphaned
helper whose shell is still running never self-exits.**

The helper has two clocks. `NO_HANDSHAKE_TIMEOUT` (10 s) only applies before
`shell_started` flips. `GRACE_WINDOW` (30 s) counts from `exited_at`, which
`SharedState::transition` sets **only when the shell exits** — never on daemon
disconnect. A helper whose daemon crashed while its shell lives has
`exited_at == None` and `shell_started == true`, so it falls through to an
unbounded `IDLE_ACCEPT_POLL` loop forever.

So the daemon-side kill genuinely covers cases nothing else does:

- a wedged helper with a live shell — socket removed, runtime deadlocked, or
  `SIGSTOP`ped: it can never be adopted and never self-exits;
- a hung-but-connected helper that buffers `GracefulShutdown` without processing
  it (`terminate()`'s own comment names this);
- a live-shell helper whose daemon-side IPC broke and was evicted.

**This also falsifies a CONTRACT in the tree.** `terminal.rs` states the helper
"is the authoritative timer … and self-exits/deletes its registry entry
independently of whatever the daemon believes here". That holds only for the
shell-exited case. The sweep's own rationale leans on the same overstatement.

### The slow path, and why it is silent

`agent_profile_gc` derives liveness from the same `live_terminal_ids()`, so its
300 s sweep `remove_dir_all`s another daemon's `agent-profiles/<id>/` and deletes
`terminal-tokens/<id>.json`. In practice the terminals are already dead by then;
what this adds is that the state is unrecoverable across a restart, because
`recover_callback_token` returns `None`.

Nothing is logged on the victim. `notify_failure::record_failure` no-ops when the
profile directory is absent, and an absent profile directory is precisely the
failure state.

## Decisions

**The fix is to make "is this helper reachable?" answerable, not to partition the
state directory.** Three candidates were carried in an earlier draft —
per-instance namespacing, an owner stamp, and startup refusal. All three are
withdrawn as workarounds: each partitions state so that one daemon never sees
another's entries, which hides the wrong predicate instead of correcting it, and
each carried an unclosed trap (namespacing breaks ordinary single-daemon restart
adoption, since `recover_profile_id`/`recover_callback_token` resolve fixed
paths; an owner stamp not re-stamped at adopt stops the sweep reclaiming real
orphans; startup refusal needs an instance lock and a stale-lock rule that do not
exist today).

### Why splitting the connect error is necessary but not sufficient

Distinguishing "no listener" from "connected, unanswered" stops the *false*
kills only if the two populations are separable, and they are not. A **wedged**
helper also accepts into the backlog and never answers. Its signature is
identical to a healthy-but-busy helper's. Error-splitting alone therefore either
keeps killing live helpers or stops reclaiming wedged ones.

Only the helper can break the tie, because only it knows whether it is serving
someone.

### The mechanism

Give the helper a concurrent liveness probe: a lightweight request it can answer
**while a session is attached**, reporting that it is alive, whether it is
currently attached, and — if not — **how long it has been unattached**.

The kill predicate is then three-way, not two-way. Getting this wrong in the
obvious direction is why an earlier draft of this section was rejected:

| probe result | outcome | why |
| --- | --- | --- |
| no answer within the timeout | **kill** | wedged, deadlocked, or gone; nothing else reclaims it |
| answers, attached | **never kill** | someone owns it and is using it |
| answers, unattached < grace | **leave** | its daemon is restarting, or is mid-`boot_reconcile` |
| answers, unattached ≥ grace | **kill** | a real orphan: live shell, no daemon coming back |

**The naive predicate "kill only when the probe does not answer" is wrong**, and
it fails on the exact case the Background says nothing else covers. An orphaned
helper with a live shell keeps its listener bound and sits in `IDLE_ACCEPT_POLL`
forever, so it *answers*. Under the naive rule it becomes unreclaimable and its
shell and PTY leak permanently and silently. The attached bit is not decoration;
it is the predicate.

### "Leave" must become a real outcome, not the absence of a kill

`ReconcileRow` has no leave-alone variant today: every row either adopts or
deletes, `kill_verified_and_delete_entry` deletes the entry unconditionally even
when it withholds the signal, and the sweep's non-`VerifiedOurs` arm deletes too.

So "do not kill" cannot be expressed by picking an existing row. Mapping it onto
a drop-only row spares the helper **and deletes its registry entry**, which is
worse than the bug: the helper keeps running, but its owning daemon can no longer
adopt it at the next `boot_reconcile`, so it becomes a permanent orphan holding a
PTY and a shell. Phase 1 must add an explicit `Leave` outcome — entry untouched,
process untouched — and the sweep must skip rather than fall through to a delete.

**The unattached grace must be measured by the helper, not the daemon.** The
helper is the only party that knows when its last daemon disconnected; a
daemon-side timer cannot distinguish "unattached for an hour" from "I just
started and have not adopted it yet". This is the same principle the existing
design already applies to the post-shell-exit grace. The grace must comfortably
exceed a daemon restart, including the window during which daemon A's own
`boot_reconcile` has not yet re-adopted its helpers — otherwise daemon B's 10 s
sweep reaps A's terminals during A's restart, which is this ticket's bug in a
narrower form.

This is correct by construction for the concurrent-daemon case without any
namespacing: a helper attached to daemon A answers daemon B's probe, so B never
kills it. It also repairs Site A, which today has no probe at all.

The predicate must reach `agent_profile_gc` too, which is not a kill site but destroys
state off the same signal and does not route through `classify`. It needs the
probe wired in explicitly or it will keep deleting a live terminal's profile and
token after the terminal itself has been correctly spared.

**A new message kind alone is not enough, and this is easy to under-scope.**
`serve_connections` awaits `handle_connection` inline and never polls `accept()`
while a session is attached, so a probe message would sit unread in the backlog —
exactly today's failure. The accept loop must gain a **concurrent accept arm**
(e.g. `select!` over `listener.accept()`) that serves probe connections only.

What is forbidden is promoting that arm into a general dispatch: do not
implement it by spawning `handle_connection` as a task, which *is* a second full
attach path and breaks the one-session-at-a-time guarantee this mechanism
otherwise leaves intact. Probe-only accept, probe-only response.

### The new message kinds must never reach a helper that predates them

Helpers outlive their daemon by design, so a daemon carrying the probe will meet
helpers spawned by a binary that has never heard of it. This was verified against
source, and the failure mode is worse than it looks.

The wire format is NDJSON with `#[serde(tag = "type", rename_all = "camelCase")]`
on both enums, so variant tags are **names, not ordinals**. Appending — or even
inserting — a variant never renumbers anything. As a type change this is safe.

Sending one is not. `read_message` converts a parse failure into an `io::Error`
by deliberate contract ("a malformed peer is treated as a transport failure, not
silently skipped"); the helper's read site propagates it with `?`; and
`run_terminal_helper`'s exit path then runs `kill_shell_if_running()` **and**
`delete_registry_entry()`. An unknown variant does not disconnect the helper — it
makes the helper **SIGKILL the user's shell and erase its own registry entry**.
That is strictly worse than the daemon-side kill this ticket exists to remove,
because it leaves nothing behind to diagnose it with.

There are no forward-compat affordances on these enums: no protocol version, no
`#[serde(other)]`, and `HandshakeAck` is a payload-less unit variant with nowhere
to put one. The tree does practise this discipline elsewhere —
`terminal_registry_file.rs` carries `#[serde(default)] boot_id: Option<String>`
under a "HARD backward-compatibility requirement" comment — it was simply never
applied to IPC.

Two rules follow, and Phase 1 must satisfy both:

- **Declare the capability in the registry file, not only in the handshake.**
  Adding a `#[serde(default)]` field to `Handshake` is compatible, but it arrives
  too late for the site that needs it most: the sweep probes a helper it has
  never handshaked with, and a *busy* helper answers nothing at all — so there is
  no handshake to read a version out of. All four sites already read
  `<registry_dir>/<id>.json`; the helper must record its probe capability there
  at startup, so any daemon knows before it connects whether a probe is
  answerable. Absent field = predates the probe. This mirrors `boot_id` exactly.
- **New variants are strictly request/response-gated and never sent unsolicited.**
  This is what keeps rollback safe: a new helper emits the probe response only in
  reply to a probe request, and an old daemon never sends one. Without this rule
  the reverse direction is fatal too — an unknown variant reaching an old daemon
  at boot lands on the `None` path and escalates all the way to `kill_verified`.

**Today's restart is unaffected, and must stay that way.** A new daemon adopting
an old helper sends only `HandshakeAck` and reads only `Handshake` plus
`Status`/`Exit` — all of which every existing helper knows. Appending variants
does not perturb this path. Any implementation that changes what the *adoption*
sequence puts on the wire has broken the one case that works today.

### An old helper cannot answer the probe, and must not be killed for it

This is the leg that closes the loop back onto the original bug. An old helper
has no concurrent accept arm, so while it is attached its listener queues the
probe and answers nothing — which is predicate leg (a), *kill*. Left unhandled,
the fix reproduces the exact defect it was written to remove, for every helper
alive across the upgrade.

So the three-way table applies **only when the peer is known to speak the probe**.
When the registry entry declares no probe capability, "connected but silent" is
undecidable by construction and the outcome is **`Leave`**. Kill only on positive
absence — no listener, connect refused, socket gone.

### Three consequences, accepted deliberately

- **A helper that wedges *while attached* becomes unreclaimable by Sites A and
  B.** If the helper is the wedged side and never observes the daemon's EOF, it
  reports itself attached forever. This narrows one of the three cases the kill
  path was justified by. Explicit `terminate()` (Site D) still reclaims it, and
  the alternative — killing on a self-reported attached state — is the bug this
  ticket exists to remove. State this in the phase result rather than discovering
  it later.
- **The unattached grace supersedes `EVICTION_BACKSTOP_GRACE` (30 s).** Any grace
  large enough to clear a daemon restart is longer, so an evicted helper that
  fails to self-exit lingers for the new grace instead of 30 s. That is the cost
  of not reaping during a restart; do not "fix" it by shortening the grace below
  a restart window.
- **Pre-upgrade helpers become unreclaimable by Sites A and B while attached, and
  this does not time out.** A helper that predates the probe never restarts on
  its own, so the `Leave` rule above has no expiry. The exposure is bounded by
  what it replaces, not by a clock: before this ticket those helpers were killed
  *wrongly*, and afterwards they are spared *unconditionally*. Explicit
  `terminate()` (Site D) still reclaims them, and they clear naturally as
  terminals end. Do not add a version-based kill to close this — "old, therefore
  dead" is the same unsound inference in a new costume.

### The `record_failure` question is deliberately not in Phase 1

`record_failure`'s no-op-when-absent behaviour bounds silence exactly where
silence is most dangerous, but it is **spec-stated** behaviour
(`{#260727-dashboard-terminal-notify-failure-visibility}`: "The hook process
never creates the profile directory to write it"), so changing it is a protocol
change under this repo's always-ask class. Its replacement is also undesigned:
the obvious candidate — let the hook create the profile directory — is the one
the spec explicitly rejected. It is Phase 3 and blocked; Phases 1 and 2 do not
depend on it.

### Convention, as hygiene rather than defence

Independently of the fix, agent-spawned daemons should set a session-scoped
`WS_DASHBOARD_STATE_HOME`. That is cheap and worth doing, but it is **not** the
remedy: it asks an AI agent to remember something whose forfeit is the user's
entire live session, and it does nothing for the wedged-helper misjudgment that
exists with a single daemon. Record it as workflow guidance, not as the close-out
of this ticket.

## Constraints

Sites C and D must keep killing unchanged, and the identity gate inside
`kill_verified_and_delete_entry` must stay. Only Sites A and B have the wrong
predicate. A change that makes reclamation conditional everywhere would reopen
the leaks the kill path exists to bound.

Every daemon-initiated kill must continue to route through
`terminal_platform::kill_verified`; never a bare pid kill.

## Spec Impact

Target area: `ai-docs/spec/ws-web-dashboard/index.md`.

- **New anchor needed.** No existing stem states when a helper may be signalled.
  The caller-visible rule this ticket establishes is: *a helper is killed only on
  positive evidence that it cannot be reached — never merely because this daemon
  does not have it in its session map, and never because a probe went
  unanswered while the socket accepted the connection.* Nearest existing
  neighbours are `{#260516-ws-web-dashboard-workroot-io-restore-model}` (owns
  boot-reconcile outcomes) and `{#260728-terminal-helper-periodic-reap}` (owns
  the periodic sweep).
- **`{#260728-terminal-helper-periodic-reap}` must be amended, not just
  supplemented.** It currently says the daemon, "if no daemon-side connection to
  it remains, kills it directly" — which is exactly the kill Phase 1 forbids,
  since *this* daemon having no connection says nothing about whether another
  daemon does. Replace that condition with the three-way predicate: unreachable,
  or attached to nobody for longer than the helper-measured grace.

  **Coordinate:** this anchor is also touched by ready/
  `260729-bug-dashboard-macos-terminal-socket-path-and-eperm-gaps` Phase 2, which
  restates the reap predicate as "verified-ours + IPC-dead". That restatement
  becomes stale the moment this phase lands. Whichever lands second owns
  reconciling both.
- **The same amendment must carry the compatibility qualifier.** The predicate is
  not universal: it holds for helpers that can answer a probe, and a helper that
  predates the probe is left alone on silence rather than reaped. A reader who
  takes the three-way predicate as unconditional will conclude the daemon reaps
  unreachable helpers when in fact it declines to. State the exception where the
  predicate is stated.
- **`{#260727-dashboard-terminal-notify-failure-visibility}`** needs a qualifying
  sentence: the bounded-silence guarantee does not hold when the profile
  directory is absent. This lands with Phase 3, not Phase 1.

Note the restore-model sentence "kill a helper whose identity cannot be verified
…" is already claimed by
`260729-bug-dashboard-macos-terminal-socket-path-and-eperm-gaps` Phase 2. Do not
edit it here; coordinate if both land together.

## Phases

### Phase 1: Kill only on positive evidence of unreachability

Three changes, in order:

1. **Helper-side probe, plus the capability declaration that gates it.** A new
   message kind, answered ahead of the session dispatch so it works while a
   session is attached, reporting alive, attached-or-not, and
   unattached-duration. Not a second attach path — see Decisions.

   The helper must also track **when its last daemon disconnected**, which no
   state records today: `exited_at` is set only on shell exit, and the
   daemon-disconnect arm of the read loop stores nothing. Without a new
   `unattached_since`, the probe has no duration to report and the predicate
   collapses to two-way.

   And it must record its probe capability in `<registry_dir>/<id>.json` at
   startup, `#[serde(default)]`, absent = predates the probe. Both directions of
   this are load-bearing: the sweep has no handshake to read a version from, and
   sending the probe to a helper that cannot parse it makes that helper kill the
   user's shell.
2. **Split the connect failure.** Stop discarding the `io::Error` in
   `connect_and_handshake`, and give `IpcStatus` a variant that distinguishes
   "no listener / socket gone" from "connected, unanswered". Feed the probe
   result into `classify` so `KillVerified` requires the three-way predicate from
   Decisions, not `None`-of-any-kind.

   The three-way predicate applies only to helpers whose registry entry declares
   the probe capability. For every other helper, "connected, unanswered" resolves
   to `Leave`; only positive absence kills.
3. **Give Site A a probe.** `sweep_registry_backstop` must probe before
   signalling. Its CONTRACT's "never adopts" property stays — probing is not
   adopting — but "never checks" must go, and the CONTRACT text must be rewritten
   to say so rather than left contradicting the code.
4. **Gate `agent_profile_gc` on the same probe.** It bypasses every kill site and
   reads `live_terminal_ids()` directly, so steps 1-3 do not reach it. Without
   this step the helper processes survive but their `agent-profiles/<id>/` and
   `terminal-tokens/<id>.json` are still deleted, which is the original symptom
   this file is named after.

   It has no socket path to probe with — profile directories are named by bare
   terminal id. It must look up `<registry_dir>/<id>.json` to find one, **and it
   needs an explicit rule for the no-entry case: no registry entry at all means
   no helper, so reclaim.** Omitting that rule stops the GC reclaiming genuine
   orphans, which is the opposite failure.

Done when all four legs of the predicate hold:

- a helper attached to a second daemon survives that daemon's `boot_reconcile`
  and its 10 s sweep **with its registry entry, profile directory and token file
  all intact** — the registry entry matters as much as the process, because
  deleting it makes the surviving helper unadoptable;
- a helper unattached for less than the grace likewise survives all three paths,
  so a daemon restart does not lose its own terminals to another daemon's sweep;
- a helper unattached past the grace is reclaimed;
- a helper with no listener behind it is reclaimed;
- a helper whose registry entry declares no probe capability is **never
  signalled on silence** by Sites A or B, and its shell is never killed by
  anything this phase puts on the wire.

Verification, in order of load-bearing weight:

- Two registries over one state dir, one live helper attached to registry A; run
  registry B's `boot_reconcile` **and** its backstop sweep; assert the **helper
  process is still alive** afterwards **and that `<registry_dir>/<id>.json` still
  exists**. Both halves are load-bearing and fail independently: sparing the
  process while deleting the entry passes a process-only assertion and still
  destroys the terminal, because daemon A can no longer adopt it.
- The same fixture, run through B's `agent_profile_gc`: assert
  `agent-profiles/<id>/` and `terminal-tokens/<id>.json` still exist. This is a
  separate assertion from the one above and fails independently if step 4 is
  skipped.
- **The daemon-restart case, which is the predicate leg easiest to get wrong.** A
  helper that answers but reports unattached for *less* than the grace must
  survive every path — both kill sites and the GC. Build it as: attach a helper,
  drop the daemon-side connection, immediately run a second registry's
  `boot_reconcile`, sweep and `agent_profile_gc`, and assert the helper, its
  profile and its token all survive. Without this the implementer can satisfy
  every other bullet with a two-way predicate and reintroduce this ticket's bug
  in its narrower form — reaping another daemon's terminals during that daemon's
  own restart.
- **Two** non-vacuity guards, and the second is the one an earlier draft of this
  phase was missing:
  - a helper with no listener behind it is still killed by both kill paths, and
    its profile and token still reclaimed by the GC;
  - a helper that **answers the probe but reports unattached past the grace** is
    also still killed. Without this case the "kill only when the probe fails"
    regression passes the suite by construction, because the orphan it leaks is
    precisely the one that answers.
- **The upgrade case, which is the one the user is standing in.** A registry
  entry written *without* the capability field, backed by a listener that accepts
  and never answers, must survive `boot_reconcile`, the sweep and the GC with its
  process, entry, profile and token intact. Build the fixture from a
  hand-written `<id>.json` that omits the field rather than from a
  round-tripped struct, so the assertion is about the absent-field decode and not
  about a default the writer supplied.
- **The adoption sequence must stay byte-compatible.** Assert that adopting a
  helper puts nothing on the wire beyond `HandshakeAck` and reads nothing beyond
  `Handshake` + `Status`/`Exit`. This is the path every live helper depends on
  across the upgrade, and it is the one an implementer is most likely to "tidy"
  while adding the probe.
- Reverting each of the four changes independently must turn one of the tests
  above red. A change that can be reverted with the suite still green is not
  covered.

### Result (bff07caf) - 2026-07-29

All four steps landed in `ws-dashboard/crates/daemon/`. `cargo build` and
`cargo test` pass (354 lib tests + all integration targets, 0 failures).

**What the mechanism ended up being.** `DaemonToHelperMessage::LivenessProbe`
/ `HelperToDaemonMessage::LivenessProbeResponse { attached,
unattached_for_ms }`; a concurrent probe-only accept arm (`serve_session`)
that `select!`s `listener.accept()` alongside the pinned `handle_connection`
future and spawns `serve_probe_connection` — never `handle_connection` — for
each accepted probe; new `SharedState::unattached_since`, flipped to `None`
only by `HandshakeAck` and back by an `AttachmentGuard` on every exit route
including the `?` ones; and `supportsLivenessProbe` in the registry entry,
`#[serde(default)]`, absent = predates the probe.

Daemon side: `connect_and_handshake` returns `HandshakeOutcome`
(`Connected`/`NoListener`/`ConnectedButSilent`) instead of a bare `Option`;
`IpcStatus::Unreachable` split into `NoListener` + `ConnectedButSilent`;
`classify` takes `Option<ProbeVerdict>` and gained `ReconcileRow::Leave`;
`probe_authorizes_reclaim` is the single predicate shared by `classify`, the
sweep and the GC. `UNATTACHED_GRACE` is 120s.

**Two things the plan did not anticipate.**

- `handle_connection` had to answer the probe as well. When no session is
  attached there is no session future for the concurrent arm to run
  alongside, so the probe lands as an ordinary connection — and that is
  exactly the case the "unattached past the grace" leg depends on being
  answerable at all. Consequence: the grace-window "one reattach, then
  self-exit" break is now gated on `attached`, so another daemon's probe
  cannot consume the reattach the helper's real owner is coming back for.
- `#[serde(rename_all = "camelCase")]` on these tagged enums renames
  *variants* only, not struct-variant fields, so the wire has always carried
  `start_time`/`next_sequence` in snake_case. Pinned literally by
  `pre_probe_variant_tags_are_unchanged_by_the_appended_probe_variants`.

**Non-vacuity.** Each of the four changes was mutation-verified: reverting
the concurrent arm turns `a_probe_is_answered_while_a_session_is_attached`
red; reverting `classify`'s `ConnectedButSilent` arm or the sweep's probe
gate turns
`a_helper_attached_to_another_daemon_survives_boot_reconcile_and_the_sweep`
red; reverting the GC gate turns
`the_profile_gc_spares_a_helper_that_is_attached_to_another_daemon` red.

**The three consequences Decisions asked to be stated here, restated as
landed:**

1. A helper that wedges *while attached* reports itself attached forever and
   is now unreclaimable by Sites A and B. Explicit `terminate()` (Site D)
   still reclaims it. This narrows one of the three cases the kill path was
   justified by, and is the deliberate price of not killing on a
   self-reported attached state.
2. `UNATTACHED_GRACE` (120s) supersedes `EVICTION_BACKSTOP_GRACE` (30s): an
   evicted helper that fails to self-exit lingers for the longer window.
3. Pre-upgrade helpers are spared unconditionally while attached, with no
   expiry, and nothing this phase puts on the wire can reach them —
   `a_helper_that_predates_the_probe_is_left_alone_and_never_sent_one`
   asserts zero bytes received. Do not close this with a version-based kill.

**Still open (not this phase):** Phase 2 owns both falsified CONTRACTs — only
the sweep's *liveness* CONTRACT was rewritten here, as step 3 required; the
`DAEMON_GRACE_WINDOW_MS` "authoritative timer ... self-exits independently"
claim and the sweep's eviction-grace rationale are untouched. The Spec Impact
section is also untouched: the new anchor and the
`{#260728-terminal-helper-periodic-reap}` amendment (with its compatibility
qualifier) still need writing, and must be reconciled with
`260729-bug-dashboard-macos-terminal-socket-path-and-eperm-gaps` Phase 2,
which restates the same reap predicate.

### Phase 2: Correct the two CONTRACTs this ticket falsified

Doc-only, no behaviour change.

1. `terminal.rs`'s "the helper is the authoritative timer … self-exits
   independently" CONTRACT: state that this holds only after the shell exits, and
   that a helper whose daemon vanished while its shell lives loops indefinitely.
   That is the fact the daemon-side kill exists for, and the current text denies
   it.
2. The sweep's rationale, which leans on the same overstatement to argue the
   eviction path is safe.

Verification: existing suite still passes. No new test.

### Phase 3: Decide what replaces `record_failure`'s no-op-when-absent

**Blocked — do not start.** Per Decisions, this is an always-ask contract change
with an undesigned replacement. Phases 1 and 2 are independent of it and may land
first. Once decided, this phase also carries the qualifying sentence on
`{#260727-dashboard-terminal-notify-failure-visibility}`.
