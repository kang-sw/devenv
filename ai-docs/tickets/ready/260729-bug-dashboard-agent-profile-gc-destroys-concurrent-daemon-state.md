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

#### Correction (bff07caf refuted) - 2026-07-29

Adversarial review refuted `bff07caf`. The daemon-side *decision* was right; the
*mechanism* it used to reach the decision was a new silent path into
`kill_shell_if_running()`. Four findings, all confirmed against source and all
fixed. `cargo build` and `cargo test` pass (362 lib tests, was 354, plus every
integration target, 0 failures).

**F9-root - a peer going away must never kill the shell (the real root cause).**
Every write in `handle_connection` propagated with `?`, and
`run_terminal_helper` runs `kill_shell_if_running()` + `delete_registry_entry()`
on ANY return from `serve_connections`. So a daemon that merely connected and
dropped without reading made the helper EPIPE on its own `Handshake` and SIGKILL
the user's shell, erasing the entry that would have let anyone diagnose it. Fixed
by `is_peer_gone` (BrokenPipe / ConnectionReset / ConnectionAborted /
NotConnected / UnexpectedEof, plus the Windows named-pipe raw codes 109/232/233/64):
a peer-gone write - and a peer-gone read - now returns the same
`ConnectionOutcome::Disconnected` the EOF arm returns. All writes go through one
`write_to_daemon!` macro so no site can reintroduce a bare `?`. Deliberately
narrow: a malformed line is `ErrorKind::Other` by `NdjsonReader`'s own contract
and stays fatal. **Helper lifetime is now independent of any daemon's
behaviour** - that is the invariant, stated as such in the code.

**F9-gate - the equivalence HOLDS, so a legacy entry now gets no socket contact
at all.** Asked to verify whether `identity_status == VerifiedOurs` already
implies a live helper process with a matching start time: it does -
`identity_status` returns `VerifiedOurs` only when `process_start_time(pid)`
returns `Some(observed) && observed == start_time`. So for a capability-absent
entry the connect decided nothing: VerifiedOurs => `Unsupported` => `Leave`
regardless, and a dead helper never reaches the probe (`NoSuchProcess` /
`PidReused` / `UnverifiableBoot` are drop-only). The capability gate therefore
moved ahead of the connect in `probe_helper`. `reconcile_entry` is unaffected -
it only probes for `ConnectedButSilent`, which already proves a listener exists.
Two consequences, both accepted deliberately:
- The ticket's "pre-upgrade helpers are spared, with no expiry" widens from
  *while attached* to *unconditionally*: a legacy helper whose socket is gone but
  whose process is alive is no longer reaped by Site A. This is the same trade
  the Decisions already made, extended to the case the connect used to cover; the
  registry entry still clears the moment the process dies. Do not close it with a
  version-based kill.
- `profile_gc_may_reclaim` does not check identity, so a capability-absent entry
  whose process is already dead survives one extra GC pass. Bounded: the sweep
  deletes such an entry within its own 10s period, after which the no-entry rule
  reclaims.

**F10 - the probe no longer shares the connect budget.** New
`PROBE_RESPONSE_IDLE_TIMEOUT`, defined as the helper's own
`PROBE_CONNECTION_TIMEOUT` (5s) rather than as a number, and applied as a
PER-MESSAGE idle timeout instead of one deadline over the whole exchange. Both
halves are load-bearing: an unattached helper answers from `handle_connection`,
which first flushes the entire retained ring (up to `MAX_OUTPUT_CHUNKS` = 1024
chunks, each its own write+flush), so a single 400ms connect-sized deadline was
exhausted by scrollback alone and fell through to `Unanswered` => `KillVerified`
- a live helper killed for being slow. The sizing rationale is in the constant's
comment: past 5s of silence the helper has already abandoned the connection, so
more patience cannot produce an answer.

**F11 - real, narrow, documented rather than fixed.** Confirmed: a connection
accepted by the concurrent arm is served as a probe and nothing else, so a daemon
that connects intending to adopt while a session is attached gets no `Handshake`,
reads `ConnectedButSilent`, and that boot's `boot_reconcile` does not adopt.
Precise trigger, now recorded at `serve_session`: daemon B connects while daemon
A's session is *genuinely still attached* AND A disconnects inside B's
connect/handshake budget. If A has already disconnected, `biased` makes the
finished session future win the select and B's connection is served as a full
session as before. Not fixed because the fix is disproportionate: promoting an
accepted connection to a session is the second attach path the Decisions forbid
by name, and handing it back is useless - B's 400ms budget elapses long before
the arm's 5s timeout would release it. Consequence is bounded: entry, process,
profile and token all survive; only the adoption is missed, and the next daemon
start retries it. The pre-ticket behaviour in that same window was not "adopts
reliably" but "queues, times out, then EPIPEs the helper into killing the shell"
(F9-root).

**F12 - the probe spawn is bounded, as backpressure not shedding.**
`MAX_CONCURRENT_PROBE_CONNECTIONS` (8) permits, acquired BEFORE `accept()`, so at
capacity the listener simply is not drained and surplus connections wait in the
backlog. Dropping them would have been worse than the unbounded spawn: a dropped
probe reads daemon-side as `Unanswered`, which authorizes a SIGKILL - i.e. load
shedding would manufacture the exact false-death signal this ticket removes.

**Tests - the previous suite could not see any of this, which was the real
defect.** `FakeHelperBehaviour::SilentForever` writes nothing on accept (a real
legacy helper writes `Handshake` immediately) and every fixture write is
`let _ = ...`, so a write error was unobservable by construction. Added a
REAL-helper fixture (`real_helper_test_support::RealHelper` - the actual
`run_terminal_helper`, with its real registry write, real listener, real PTY and
a real shell child) plus:
- `a_real_helper_survives_a_daemon_that_connects_and_drops_without_reading` -
  helper alive, `<id>.json` intact, and NEW shell output past the pre-drop
  sequence (so "the shell lives" is observed, not inferred);
- `a_peer_gone_write_returns_the_real_helper_to_its_accept_loop` - a full
  adoption sequence still completes afterwards;
- `only_peer_gone_io_errors_are_downgraded` - non-vacuity for the narrow
  classification, including a malformed-line error staying fatal;
- `every_probe_is_answered_even_past_the_concurrent_probe_bound` - 3x the cap,
  all answered (guards the drop-instead-of-queue implementation of F12; it does
  not distinguish bounded from unbounded, which is stated rather than claimed);
- `a_slow_ring_flush_cannot_exhaust_the_probe_timeout` - scripted drip fixture,
  the deterministic instrument for the F10 policy;
- `a_real_helper_with_a_retained_ring_answers_a_probe_without_being_reclaimed` -
  the same scenario end-to-end with a >1MB real ring and the production connect
  budget. Honest scope note in the test: on an unloaded machine a real helper
  outruns any budget, so this is an integration guard, not the F10 mutation
  instrument;
- `a_capability_absent_entry_is_never_probed_even_when_nothing_is_listening` -
  F9-gate, including that the connect loop's budget is not spent;
- `probe_response_idle_timeout_is_sized_against_the_helpers_own_bound`.

`probe_helper_reports_a_reclaimable_verdict_when_nothing_is_listening` lost its
`declares_capability = false` leg, which is exactly the F9-gate behaviour change;
the replacement test above asserts the new outcome directly.

**Mutation results.** New: making `is_peer_gone` always false kills the real
helper - the reconnect fails with `NotFound` because the helper has exited and
unlinked its socket (this is the bff07caf behaviour, reproduced); restoring the
shared deadline turns `a_slow_ring_flush_cannot_exhaust_the_probe_timeout` red;
connecting before the capability gate turns
`a_capability_absent_entry_is_never_probed_even_when_nothing_is_listening` red.
Re-ran the four original checks and all still hold: reverting the concurrent arm
turns `a_probe_is_answered_while_a_session_is_attached` red; reverting
`classify`'s `ConnectedButSilent` arm or the sweep's probe gate turns
`a_helper_attached_to_another_daemon_survives_boot_reconcile_and_the_sweep` red;
reverting the GC gate turns
`the_profile_gc_spares_a_helper_that_is_attached_to_another_daemon` red.

#### Correction (0ff45251 refuted) - 2026-07-29

A third adversarial review refuted `0ff45251`. Round 2 fixed peer-caused *I/O*
errors and left every other peer-caused error fatal. `cargo build` and
`cargo test` pass (377 lib tests, was 362, plus every integration target, 0
failures). Six findings fixed, two recorded.

**A - the invariant, stated in its real form.** Round 2's `is_peer_gone` was an
allow-list of five `io::ErrorKind`s plus four Windows raw codes. An allow-list
cannot be shown to be complete, and every kind missing from it is a dead shell.
It was missing the *most common* case of all: a daemon SIGKILLed mid-`write_all`
leaves a truncated final line, `Lines::next_line()` hands those trailing bytes
back as `Some(line)` at EOF, and the resulting decode failure was
`ErrorKind::Other`, which the list rejected. So the ordinary crash of the
ordinary peer SIGKILLed the user's shell. Also missing: invalid UTF-8 in that
same tail (`InvalidData`), an unknown variant tag from a newer daemon,
`ErrorKind::WriteZero` (what `write_all` actually returns on a short write), and
Windows `ERROR_OPERATION_ABORTED` (995).

`is_peer_gone` is **deleted**. Classification moved into the transport and is
**by source**, not by kind: `read_message` returns
`terminal_helper_ipc::PeerFault` (`Io` / `InvalidUtf8` / `Malformed`) - every way
reading can fail is the peer's, so reading has one error type and it is always a
peer fault - and `write_ndjson` returns `WriteFault::{Peer, Serialize}`, where
`Peer` is every I/O kind and `Serialize` (this process failing to serialize its
own message) stays fatal. There is nothing left to enumerate. The invariant now
reads, in the code: *no error attributable to the peer's connection may end the
helper process or kill its shell; it ends the connection and the helper returns
to its accept loop.*

This also makes the unknown-variant defence **structural**. The Decisions made
`supportsLivenessProbe` the defence; from this build onwards the guarantee is in
the transport and the flag is defence-in-depth. It stays load-bearing, and the
CONTRACTs at `terminal_registry_file.rs` / `terminal_helper_protocol.rs` now say
why: the population the flag protects is exactly the one that does *not* have
the structural guarantee - helpers already running from the previous binary.

`only_peer_gone_io_errors_are_downgraded` pinned the too-narrow list as
intentional and asserted `InvalidData`/`Other` stay fatal. It is **replaced**.
The replacement is built so it can catch a missing case, which the old one
structurally could not: `every_peer_connection_fault_ends_only_the_connection`
asserts the classification ignores the kind (feeding it fourteen kinds including
ones nobody enumerated, plus raw code 995) and that `Serialize` still separates;
and `a_real_helper_survives_every_malformed_thing_a_peer_can_put_on_the_wire`
drives all four real byte sequences (truncated line, mid-codepoint cut, garbage
line, future variant) into a REAL attached helper and asserts new shell output
afterwards.

**B - `accept()` failures no longer kill.** Both accept sites returned `Err`,
which `run_terminal_helper` turns into `kill_shell_if_running()`. EMFILE/ENFILE
(someone else exhausted the machine's fd table), ECONNABORTED and the Windows
pipe-connect failures are all transient and peer- or environment-influenced. New
`AcceptFailures`: exponential backoff from 20ms to a 1s ceiling, shared across
both sites so neither can reset the other's budget, and only
`MAX_CONSECUTIVE_ACCEPT_FAILURES` (64, ~1 minute of unbroken failure) is treated
as a permanently broken listener. Deliberately no per-kind "is this transient?"
test - that is finding A's mistake in a new place, and the safe default here is
retry. In the concurrent probe arm the backoff is awaited inside a `select!`
with the session so an accept storm cannot starve the attached daemon.

**C - the probe exchange is now totally bounded, and the bound never kills.**
Per-message idle timeouts alone left the exchange unbounded: one line every 4.9s
held the call forever, and `boot_reconcile` is awaited *before* the router binds
while the same call runs on the reaper's 10s Burst-catch-up tick. Two bounds,
because they fail independently - `MAX_PROBE_EXCHANGE_MESSAGES`
(`MAX_OUTPUT_CHUNKS + 16` = 1040) catches a fast flood, and
`PROBE_EXCHANGE_TOTAL_TIMEOUT` (20.8s = 1040 x a pessimistic 20ms/message, two
orders of magnitude above a local socket round trip) catches a slow drip. Both
are sized against the legitimate worst case, a full retained-ring flush ahead of
the answer; `the_probe_exchange_bounds_clear_a_full_ring_flush` pins the
derivation rather than the numbers. The idle window is clamped to what remains
of the total, so the total is hard rather than "total plus one more idle
window".

**Neither bound produces a kill**, and this is the load-bearing half. Hitting
one yields the new `ProbeVerdict::Abandoned`, which does not authorize reclaim:
these bounds protect the *daemon*, they measure nothing about the helper, and
turning the daemon's own impatience into positive evidence of death is exactly
the F10 mistake one level up.

**D - `profile_gc_may_reclaim` now establishes `VerifiedOurs`.** It read the
entry off disk and went straight to the probe - the "GC bypasses everything"
pattern the Background names - and it was wrong in both directions: a pid
recycled by an unrelated process was reasoned about as though it were the
helper, and a stale capability-absent entry resolved to `Unsupported`, i.e.
never reclaimed, so its profile and token leaked. It now matches
`reconcile_entry` and the sweep: an entry that cannot be verified as ours
describes no helper of ours, which is the no-entry case - reclaim. (Never a
kill; the GC has no kill path.) This closes the disk-state leak round 2 recorded
against F9-gate.

**E - the Windows F12 inversion, fixed in the transport where the defect is.**
`IpcListener` kept exactly one armed instance, re-armed only inside `accept()`,
which made "armed" conditional on `accept()` being *polled* - and `serve_session`
deliberately stops polling it at probe capacity, which is its backpressure
design. On Unix the surplus parks in the kernel backlog; on Windows it got
`ERROR_PIPE_BUSY`, the daemon's 400ms connect budget expired, and the outcome
inverted into `NoListener -> KillVerified` (unconditional - positive absence
needs no probe verdict) or `Unanswered -> KillVerified`. A healthy helper,
SIGKILLed for being popular. Fixed with a real backlog: `PIPE_BACKLOG` (16,
above `MAX_CONCURRENT_PROBE_CONNECTIONS` + the session) instances armed up
front, restored on every accept, so unconnected instances exist whether or not
anyone is polling; `accept()` polls all of them concurrently via
`select_all`, because polling one while a client sits connected on another is
head-of-line blocking - the same "nobody answered" signature by a different
route. `NamedPipeServer::connect` is documented cancel-safe, so the losing
futures being dropped loses no connection. Load-shedding was not used, per the
ticket: a dropped probe reads as `Unanswered`, which authorizes a SIGKILL.

**F - the tests now exercise a LEGACY-SHAPED helper.** Every real-helper test ran
the *current* binary, which by construction has all the fixes, so the at-risk
population was untested - the same fixture-is-not-the-population blind spot as
round 1, one layer up. New test-only seam `HelperShape` (`CURRENT` / `LEGACY`;
production constructs only `CURRENT`) reproduces a pre-upgrade helper in all
three respects at once: no peer-fault downgrade, no concurrent probe arm, and
`supportsLivenessProbe` absent from its registry entry. `LivenessProbe` is also
modelled as the decode failure it would be on that build, so a legacy fixture
cannot cheerfully answer a probe it could never have parsed.

What it establishes, as assertions of *damage* rather than of hope:
- `a_legacy_shaped_helper_really_is_destroyed_by_what_a_daemon_used_to_do` - the
  entry decodes capability-absent; a bare connect-and-drop makes it exit and
  erase its own entry; and sending it a probe SIGKILLs the shell.
- `a_legacy_shaped_helper_cannot_answer_anything_while_a_session_is_attached` -
  a second daemon's connect is accepted and answered with nothing, reproducing
  "healthy, busy helper looks dead" against a real helper.
- `a_legacy_real_helper_survives_because_the_daemon_never_touches_it` - the
  daemon side of the same story: `probe_helper` resolves `Unsupported` with no
  connect, and the helper is alive with its entry intact afterwards. The sweep
  itself is deliberately not driven against a `RealHelper` (it runs in-process,
  so its entry names the test binary's pid and a regression reaching
  `kill_verified` would SIGKILL the test runner rather than fail an assertion).

**RECORDED, NOT FIXED (1) - review finding 2c, the one-time upgrade cost.**
`connect_and_handshake` connects unconditionally, because adoption is what
`boot_reconcile` exists to do and there is no way to adopt without connecting.
Against an *attached* pre-upgrade helper that connect is queued, times out into
`ConnectedButSilent`, and the stream is dropped; the helper then serves that
dead connection on its next accept, EPIPEs on its own `Handshake`, and
self-kills. Unfixable daemon-side - the defect is in the already-running binary.

Before/after, which is why it is accepted rather than blocking:

| path | before this ticket | after |
| --- | --- | --- |
| `boot_reconcile` vs attached pre-upgrade helper | killed outright: `Unreachable -> KillVerified` | not killed by the daemon; self-kills on its next accept after the adoption connect |
| 10s sweep vs pre-upgrade helper | killed outright, every 10s | **no contact at all** - `probe_helper` short-circuits on the absent capability flag before connecting |
| profile GC vs pre-upgrade helper | profile + token deleted | spared while the process is verifiably ours |

So it is strictly better, not a regression, and the repeating destructive path
(the sweep) is genuinely gone. Asserted, not assumed, by
`a_legacy_helper_still_self_kills_on_the_adoption_connect_accepted_cost`, which
exists so the cost is measured and so it goes red the day someone believes it
has been fixed. It clears as terminals end and helpers are respawned from the
new binary; do not close it with a version-based kill.

The related disk-state leak, now narrowed by finding D: a capability-absent entry
whose helper process is still verifiably ours is never GC-reclaimed
(`Unsupported` does not authorize reclaim), so its `agent-profiles/<id>/` and
`terminal-tokens/<id>.json` persist for as long as that helper lives. Bounded by
the helper's own lifetime, and the state is the live terminal's own - it is
retention, not orphaning. Once the process is gone, finding D's identity check
reclaims on the next GC pass.

**RECORDED (2) - review finding 3: `VerifiedOurs` does NOT imply alive.** Round
2's equivalence claim was too strong and is withdrawn. `VerifiedOurs` means "a
process object with this pid and this start time is still visible to the OS",
which includes a Linux zombie (`/proc/<pid>/stat` exists in state `Z`), a macOS
`SZOMB` process (`read_bsdinfo` still returns `Some`), and a Windows terminated
process whose object is kept alive by an open handle.

Assessed against every decision that consumes it; **no decision changes**:
- No kill is affected. `kill_verified` re-verifies identity and signals through
  a pidfd/handle; signalling a zombie or an already-terminated process is a
  no-op, and start-time verification is what stops a recycled pid being hit.
- The reachability answer is unchanged: a zombie has no listener, so the connect
  F9-gate removed would have returned `NoListener` - which for a
  capability-absent entry is still not a licence to probe, and connecting is
  what kills that population.
- What remains is a bounded disk-state lag, not a leaked shell. On Unix the
  window is microseconds: `spawn_detached`'s `setsid()` + double fork reparents
  every helper to init/launchd, which reaps immediately, so no helper of ours
  has a parent that can leave it a zombie. On Windows it lasts as long as some
  process holds a handle, and nothing in this tree holds one after spawn.

A per-platform "is it a zombie?" check was considered and rejected: new
per-platform code for a microsecond window, whose failure direction is the
dangerous one - reading "cannot determine state" as dead would authorize a kill.
Recorded at `probe_helper`'s CONTRACT, replacing the overstated claim.

**Mutation results.** New, all confirmed red: reinstating round 2's narrowness on
the read side (downgrade only `PeerFault::Io`) turns
`a_real_helper_survives_every_malformed_thing_a_peer_can_put_on_the_wire` red;
making every accept failure immediately fatal turns
`accept_failures_back_off_and_only_a_sustained_run_is_fatal` red; removing the
total deadline turns `a_dripping_peer_cannot_hold_the_probe_past_its_total_budget`
red; removing the message cap turns
`a_flooding_peer_cannot_hold_the_probe_past_its_message_budget` red (this test
was initially non-discriminating - `Abandoned` alone does not say WHICH bound
fired, and it passed under the mutation 30s later via the total deadline; an
elapsed-time assertion was added and the mutation re-run); mapping `Abandoned`
onto reclaim turns `probe_authorizes_reclaim_matches_the_three_way_predicate_exactly`
red; deleting the GC identity check turns
`the_profile_gc_reclaims_state_whose_helper_process_is_gone` red; collapsing
`HelperShape::LEGACY` into `CURRENT` turns
`a_legacy_shaped_helper_really_is_destroyed_by_what_a_daemon_used_to_do` red.

Round 2's three re-run and all still hold: making peer write faults fatal kills
the real helper (`a_real_helper_survives_a_daemon_that_connects_and_drops_without_reading`
red); restoring one shared deadline over the whole exchange turns
`a_slow_ring_flush_cannot_exhaust_the_probe_timeout` red; connecting before the
capability gate turns
`a_capability_absent_entry_is_never_probed_even_when_nothing_is_listening` red.
Round 1's four re-run and all still hold: reverting the concurrent arm turns
`a_probe_is_answered_while_a_session_is_attached` red; reverting `classify`'s
`ConnectedButSilent` arm or the sweep's probe gate turns
`a_helper_attached_to_another_daemon_survives_boot_reconcile_and_the_sweep` red;
reverting the GC probe gate turns
`the_profile_gc_spares_a_helper_that_is_attached_to_another_daemon` red.

**Verification scope, stated honestly.** Finding E has no runtime coverage: this
session has no Windows host, so the named-pipe backlog is cross-compile-checked
(`cargo check --target x86_64-pc-windows-gnu`) and reviewed against the
documented `ConnectNamedPipe` / cancel-safety semantics only. That is the same
scope limit `terminal_ipc_transport.rs`'s Stage-2 header already carries, and it
is unchanged by this fix.

#### Correction (8e9e5134 refuted) - 2026-07-29

A fourth adversarial review refuted `8e9e5134`. Two of its decisions are SCOPE
REDUCTIONS - machinery withdrawn rather than repaired. `cargo build` and `cargo
check --target x86_64-pc-windows-gnu` pass clean; `cargo test` passes every
integration target and 378 of 379 lib tests, the one failure being a
PRE-EXISTING flake unrelated to this ticket (evidence under Verification below).
Lib tests went 377 -> 379.

**WITHDRAWN (1) - F12, and the Windows transport rewrite it caused.** Round 3
bounded the concurrent probe arm's spawn with `MAX_CONCURRENT_PROBE_CONNECTIONS`
(8) permits taken BEFORE `accept()`, so the listener was not drained at
capacity. That made "is the listener armed" conditional on this helper's own
load, which on a Windows named pipe - no backlog - inverted into
`ERROR_PIPE_BUSY` -> `NoListener` -> `KillVerified`. Undoing THAT needed
`PIPE_BACKLOG` + `select_all` in `terminal_ipc_transport.rs`, which cannot be
runtime-verified in this environment and produced two defects in one round: a
dropped already-connected client, and a backlog that shrinks to empty and panics
`select_all`.

Both are gone. The transport is back to its pre-`8e9e5134` single-armed-instance
shape (verbatim, plus a note saying why it is sound: `accept()` is polled
unconditionally again, and stopping that poll is what must never happen without
restoring a real backlog first). The permit-before-accept gating is gone with it.

F12 is not reimplemented, and that is the accepted state: probe tasks are one
line in, one line out, hard-bounded by `PROBE_CONNECTION_TIMEOUT` (5s), on a
local-only socket, so the reachable steady state is "however many probes a local
process opened in the last 5 seconds". An unbounded spawn of 5s-bounded tasks on
a local socket is accepted; a local process that wants to exhaust this machine
has cheaper ways. Any future bound MUST NOT be built by withholding the
`accept()` poll - the rationale is recorded where the constant used to be.
`every_probe_is_answered_even_past_the_concurrent_probe_bound` outlived its
subject and is renamed
`many_simultaneous_probes_are_all_answered_while_a_session_is_attached`: it never
distinguished bounded from unbounded, and what it does assert - every probe gets
an answer - is still exactly what matters.

**Finding 6 - the daemon side of the invariant this work declares closed.** The
`_` arm in `probe_helper_reachability` swallowed `Ok(Err(PeerFault))` into
`ProbeVerdict::Unanswered`, which authorizes reclaim. So a decode fault on the
DAEMON's side of the probe SIGKILLed a healthy helper - the exact mirror of what
round 3 made structurally safe helper-side, in the one direction round 3 did not
check. Reachable today, not theoretically: `HelperToDaemonMessage` has no
`#[serde(other)]`, so a NEWER helper's variant is undecodable by an OLDER daemon,
and a daemon rollback is the realistic trigger.

Fixed with a new sibling verdict, `ProbeVerdict::PeerFaulted`, which does not
authorize reclaim. A sibling rather than reusing `Abandoned` because the two have
different causes and want different logs: `Abandoned` is this daemon's own
budgets firing, `PeerFaulted` is protocol skew or a framing desync, and the
latter is logged at `warn!`. I/O faults ride along with decode faults
deliberately - splitting them would mean deciding by `io::ErrorKind` again, which
is round 3 finding A's mistake one level up, and nothing is lost because a helper
that is genuinely gone still yields a clean EOF (`Unanswered`) or no listener on
the next sweep. Pinned by
`a_probe_reply_this_daemon_cannot_decode_never_authorizes_a_kill`, whose fixture
answers with a raw undecodable line rather than a serialized message; non-vacuity
is the `SilentForever` leg of
`probe_helper_writes_nothing_to_a_peer_that_does_not_declare_the_capability`,
which still resolves to `Unanswered`.

**Finding 5 - accept failures are never fatal.** `MAX_CONSECUTIVE_ACCEPT_FAILURES`
(64) and the fatal path are DELETED; the backoff caps at its 1s ceiling forever
and the log is now `warn!`. The budget was a timer on this ticket's own subject:
EMFILE/ENFILE are properties of the machine, so one fd exhaustion made every
helper on the box kill its shell inside the same second. Self-killing bought no
cleanup either - a genuinely dead listener is already reclaimed daemon-side via
`HandshakeOutcome::NoListener` -> `KillVerified` - and it removed the only chance
of recovery. `record` now returns `Duration` rather than `Option<Duration>`, so
"there is no fatal path" is a property of the type; the success reset is kept.
Test renamed to `accept_failures_back_off_forever_and_never_become_fatal`.

**Finding 8 - `boot_reconcile` is bounded in aggregate.** Per-entry worst case is
~21.6s (400ms handshake connect + 400ms probe connect + 20.8s exchange) and the
loop is serial and awaited before the router binds (`server.rs`), so ten
pathological entries held the daemon off HTTP for ~3.6 minutes. New
`BOOT_RECONCILE_TOTAL_BUDGET` (30s), checked BEFORE each entry.

Sizing, stated rather than asserted: 30s admits one entry consuming its entire
worst case plus several hundred ordinary ones (a healthy helper handshakes in
milliseconds; only `ConnectedButSilent` entries reach the probe at all), and is
the outer edge of what a user waiting for the dashboard will sit through. The
check is deliberately NOT a timeout AROUND an entry - that would drop a
connection mid-adopt, which is the connect-and-drop pattern rounds 2 and 3 were
spent removing - so the true worst case is budget + one entry (~52s).

A skipped entry is UNTOUCHED: not adopted, and explicitly not reclaimed. Running
out of startup budget is a fact about the daemon, exactly like `Abandoned`.
`entries_boot_reconcile_never_reaches_are_left_completely_untouched` asserts this
against entries `reconcile_entry` WOULD delete, so "untouched" is observable, and
its second leg reclaims those same entries under a real budget so the assertion
is about the budget rather than about unreclaimable entries.

**CONSEQUENCE, accepted:** a skipped entry is never adopted until the next daemon
start - nothing else adopts, the reaper only reclaims. If such a helper is a
genuine orphan the sweep reclaims it on the ordinary predicate, which is correct;
if it belongs to a daemon that is restarting, it can be reaped as
`UnattachedPastGrace` once its 120s grace elapses. This is bounded by the fact
that the entries that BURN the budget are precisely the ones that cannot be
adopted anyway (`ConnectedButSilent`), so a healthy entry is only stranded when
it queues behind many pathological ones.

**RECORDED, NOT FIXED - finding 7, the reaper-starvation half.** An entry that
always yields `Abandoned` costs up to its connect budget plus
`PROBE_EXCHANGE_TOTAL_TIMEOUT` (20.8s) on EVERY 10s sweep tick, and
`tokio::time::interval`'s default `MissedTickBehavior::Burst` then fires the
missed ticks back to back. Recorded at `terminal_reaper.rs`'s spawn. Not fixed
here because the safe fixes (a `Delay`/`Skip` miss behaviour, or an aggregate
sweep bound) change reap TIMING, which is observable behaviour belonging to a
phase that owns the reap predicate. Exposure is bounded: a starved sweep delays
reclamation, it never reclaims wrongly.

**RECORDED, NOT FIXED - finding 9, `HelperShape::LEGACY` fidelity.** The fixture
is faithful for the three behaviours its two tests assert and NOT beyond them.
Two divergences would let a FUTURE test pass while the real pre-upgrade binary
failed, and are now named at the constant so nobody leans on it for them:
- accept backoff is not shape-gated (`AcceptFailures` applies to both shapes; a
  real legacy helper died on its FIRST accept error);
- the grace-window self-exit is not shape-gated (the break is gated on
  `attached`; a real legacy helper's was unconditional, so any connection at all
  consumed the one post-shell-exit reattach).
D3 as well: the fixture writes `"supportsLivenessProbe": false` where a real
pre-upgrade entry OMITS the key. Decode-equivalent through `#[serde(default)]`,
so no daemon-side decision differs, but the absent-key path is never exercised by
this fixture - it is covered by the hand-written-JSON upgrade test in
`terminal.rs`, and `spawn_legacy`'s doc no longer claims otherwise.

**RECORDED - finding 10, `WriteFault::Serialize` is unreachable.** Every
`HelperToDaemonMessage` variant is a struct of integers, strings, bools, options
and vectors of the same, with string map keys only; `serde_json`'s failure modes
(non-string map keys, a fallible `Serialize` impl, non-finite floats) are all
absent. So "internal failures stay fatal" is decorative today - the split earns
its keep only when a future message type introduces a fallible `Serialize`.
Stated at `write_fault_ends_only_the_connection` rather than restating the
guarantee, and it is why that test constructs a tuple-keyed map by hand.

**Finding 11 - framing faults are visible.** `PeerFault::Malformed` /
`InvalidUtf8` now log at `warn!` (helper-side read arm, and the daemon-side probe
arm). `PeerFault::Io` stays at `debug!` - a peer going away is ordinary. A later
framing desync must not be invisible at default levels merely because it is no
longer fatal.

**Finding 12 - two stale `is_peer_gone` references** (deleted in round 3) removed
from `serve_session`'s and `probe_helper`'s CONTRACTs, both now pointing at
`write_fault_ends_only_the_connection`.

**Mutation results.** New, all confirmed red: routing `Ok(Err(PeerFault))` back
to `Unanswered` turns `a_probe_reply_this_daemon_cannot_decode_never_authorizes_a_kill`
red; mapping `PeerFaulted` onto reclaim turns
`probe_authorizes_reclaim_matches_the_three_way_predicate_exactly` red; removing
the `boot_reconcile` budget check, AND making a skipped entry delete its registry
file instead, both turn
`entries_boot_reconcile_never_reaches_are_left_completely_untouched` red (the
second is the one that matters - it is the difference between "bounded" and
"bounded by reclaiming").

Every prior check re-run and all still hold. Round 3's seven: reinstating round
2's read-side narrowness turns
`a_real_helper_survives_every_malformed_thing_a_peer_can_put_on_the_wire` red;
removing the total deadline turns `a_dripping_peer_cannot_hold_the_probe_past_its_total_budget`
red; removing the message cap turns
`a_flooding_peer_cannot_hold_the_probe_past_its_message_budget` red; mapping
`Abandoned` onto reclaim turns the predicate test red; deleting the GC identity
check turns `the_profile_gc_reclaims_state_whose_helper_process_is_gone` red;
collapsing `HelperShape::LEGACY` into `CURRENT` turns
`a_legacy_shaped_helper_really_is_destroyed_by_what_a_daemon_used_to_do` red.

The seventh - "making every accept failure immediately fatal" - CHANGED CHARACTER
and is reported as such rather than as a pass. The mutation is no longer
expressible at `record`, whose return type is now `Duration`; reinstating the
budget in full (Option return + both call sites' fatal branches) fails to
COMPILE, taking the test with it. That is a weaker instrument than an assertion
failure, and it is the honest state: the property is now enforced by the type,
and no test drives a REAL accept failure (no test may exhaust the machine's fd
table). Round 3 had the same limitation - its check was also only against
`record`, never against the call sites.

Round 2's three: making peer write faults fatal turns
`a_real_helper_survives_a_daemon_that_connects_and_drops_without_reading` red;
restoring ONE shared connect-sized deadline over the whole exchange turns
`a_slow_ring_flush_cannot_exhaust_the_probe_timeout` red; connecting before the
capability gate turns `a_capability_absent_entry_is_never_probed_even_when_nothing_is_listening`
red. Round 1's four: reverting the concurrent arm turns
`a_probe_is_answered_while_a_session_is_attached` red; reverting `classify`'s
`ConnectedButSilent` arm or the sweep's probe gate turns
`a_helper_attached_to_another_daemon_survives_boot_reconcile_and_the_sweep` red;
reverting the GC probe gate turns
`the_profile_gc_spares_a_helper_that_is_attached_to_another_daemon` red.

**Verification scope, stated honestly.**
- The Windows leg has no runtime coverage, as before - but the surface is now
  SMALLER than at round 3, not larger: the reverted transport is the shape that
  shipped before this ticket, and finding E's untestable machinery is gone rather
  than being carried unverified.
- `git_exec::tests::capture_refuses_to_spawn_once_the_outstanding_reader_cap_is_reached`
  fails on this machine: it asserts that 32 spawned git readers all stay wedged
  and observes 28-30. Confirmed PRE-EXISTING and unrelated, three ways: it fails
  identically at `8e9e5134` with these changes checked away (376 passed / 1
  failed there, 378 / 1 here - the same single failure); it PASSES when run
  alone, so it is order- or load-sensitive within the binary; and it passed in
  the first full runs of this session, before the machine accumulated load. It
  touches no code this ticket changes. It deserves its own ticket - a test that
  is green alone and red in the suite will eventually be read as a real
  regression.

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
