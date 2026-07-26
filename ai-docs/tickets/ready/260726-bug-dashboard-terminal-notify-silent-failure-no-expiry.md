---
title: terminal-notify's deliberate silence has no expiry, no failure counter, and no reader anywhere
related:
  260725-feat-dashboard-pty-agent-attention-notification: parent feature, now closed in .done/ with all 8 phases landed; this ticket fixes an observability gap that survived that ticket's completion rather than a deferred item of it
sage-review-design: completed
sage-review-completeness: completed
---

# terminal-notify's deliberate silence has no expiry, no failure counter, and no reader anywhere

## Background

The dashboard's PTY-agent attention feature exists so that an agent finishing a
turn produces a signal the owner can see from a browser tab, a nav row, or the
browser chrome itself. The whole chain is live today:
`260725-feat-dashboard-pty-agent-attention-notification` closed with all eight
phases landed, so this is not a prospective gap in unfinished work — it is a
gap in shipped behavior.

The chain's first hop is the hidden `ws-dashboard terminal-notify` subcommand
(`terminal_notify.rs`), invoked by a materialized vendor `settings.json` hook on
every agent turn boundary. It POSTs to
`/api/dashboard/terminals/{terminal_id}/turn-state`, one of three route classes
deliberately registered outside the protected router (`router.rs:105-116`),
authorized by a per-terminal opaque token rather than the owner session cookie.

`run_terminal_notify` (`terminal_notify.rs:55-60`) unconditionally returns
`Ok(())` regardless of whether delivery succeeded. On failure it appends one
line to `logs/terminal-notify.log.<date>` under the daemon state dir
(`log_failure`, `terminal_notify.rs:117-145`) instead of writing to
stdout/stderr, and the process exits `0`. Nothing in this path calls
`tracing::` — the subcommand is dispatched in `main.rs:21-28` *before*
`logging::init` runs, which that call site's comment states is deliberate for a
short-lived hook-fired invocation.

**That silence was not arbitrary and this ticket does not ask to revert it.**
The module header (`terminal_notify.rs:6-30`) records a real-PTY measurement: a
non-zero exit with stderr makes Claude Code surface a visible `<Event> hook
error` line plus a persistent "Stop hook error occurred" status-line indicator
on every `UserPromptSubmit` and every `Stop`, for as long as the callback file
is broken. That is unacceptable per-turn noise the user cannot act on.

**The problem is that nothing bounds or surfaces the silence.** All three
captured claims were re-verified against the current tree and all three hold,
with one refinement:

1. **No expiry — with one partial exception that does not cover the live case.**
   `pendingAttentionStateFor` (`agentAttention.ts:99-108`) suppresses a badge
   when the session's daemon-reported status is not `"running"`, which bounds a
   badge stranded on a *dead* session. It applies no age check to
   `entry.updatedAtMs`. A session that is still `running` while its notify path
   is broken therefore holds its last-delivered state indefinitely.
2. **No failure counter.** Grepping the daemon crate and the frontend for
   `consecutive` / `failure_count` / `failureCount` / `failure_streak` returns
   no hit in any attention-path module.
3. **No reader, in either sense.** `logs/terminal-notify.log` is referenced only
   by its writer (`terminal_notify.rs`) and by tests
   (`crates/daemon/tests/terminal_notify.rs:168-177`). `bound_base_url_path` is
   referenced only by its own definition, its writer at `server.rs:98`, and
   tests — no production reader anywhere.

### The two failure classes this produces

Both are live today and neither produces a signal in any place a human looks.

- **Stranded state.** The `working` POST lands, the badge shows `working`,
  delivery then breaks, and the `ready` POST never arrives. The session is still
  `running`, so the liveness gate above does not clear it. The tab shows
  `working` forever for an agent that finished minutes ago — the owner waits on
  a turn that already ended.
- **Never-posted.** The first `working` POST already fails, so
  `AttentionHub::record_and_publish` (`agent_attention.rs:117-135`) never runs
  and the terminal gets no hub entry at all. The browser shows nothing, which is
  byte-identical to "this agent is idle". The feature is silently dead.

In both classes the exit status is `0`, `daemon.log` is silent, and the only
artifact is a log file with no reader.

## Decisions

### The silence stays silent-but-observable, and is bounded by an *unrepaired* failure rather than by an expiry on attention state

Settled under goal-run posture (owner away; reversible, local, recorded here).

The stdio-silence CONTRACT at `terminal_notify.rs:6-30` is kept intact. What
changes is that the silence acquires a bound and a reader:

- `terminal-notify` persists a per-terminal **failure record** alongside the
  callback file it was handed: a consecutive-failure count, the wall-clock
  timestamp of the most recent failure, and that failure's error text. A
  successful delivery clears the record.
- The daemon **reads** that record on its existing periodic
  `sweep_agent_profiles` pass and emits one `tracing::warn!` per terminal once
  the record shows a failure that nothing has repaired. That warning lands in
  `daemon.log`, which is already rotated and pruned by `logging.rs`.

**The bound is on the silence, not on the attention state.** A wall-clock expiry
on the attention state would be actively wrong: `ready` means "your agent is
waiting for you" and is precisely the state that must survive an owner being
away for an hour, and a legitimately long turn can hold `working` for just as
long. Expiring either would silently discard a *correct* signal to fix a
*missing* one — a worse failure than the one being repaired. What must not be
unbounded is the *silence*. The escalation rule below does carry a time term,
but that term only ever delays a log line; it never discards, expires, or
rewrites a delivered attention state. That is the whole distinction from the
rejected wall-clock expiry, and it is why the two are not in tension.

**Silent-but-observable, not user-visible.** The fault class here (stale
`callback.json` after a restart, token mismatch, dead port, unwritable disk) is
an operator/developer fault the end user cannot act on from a terminal tab. A
"hooks broken" badge would occupy the same badge space this feature owns, raised
for exactly the terminals that currently show nothing — a softer repetition of
the per-turn-noise mistake the module CONTRACT already exists to avoid. This
choice is strictly additive to reverse: a UI surface can be layered later on top
of a record that is already written and already read.

**State plainly what this does not fix.** After this phase the stranded badge on
a live session is still stranded — nothing in the presentation layer changes,
and the owner staring at a stale `working` still gets no correction. What
changes is that the operator can now find out, from `daemon.log`, that delivery
is broken for that terminal. Do not let the spec entry, the phase report, or a
follow-up ticket describe this as fixing the stranded-badge case.

### Rejected alternatives

- **Restore a non-zero exit with stderr on failure.** Rejected on the recorded
  real-PTY evidence (`terminal_notify.rs:6-30`). This is the measurement the
  whole silence design rests on; do not re-litigate it without a new
  measurement.
- **Wall-clock expiry on `AttentionHub` entries, or an age check inside
  `pendingAttentionStateFor`.** Rejected — see above. It drops correct `ready`
  signals, which is the opposite of this feature's purpose.
- **A pure consecutive-failure threshold with no time term (originally settled
  here as "3 consecutive failures").** Rejected on arithmetic, not taste — see
  the escalation-rule section above. Two hook events per turn cap the headline
  stranded-state case at one failed delivery, and the owner it strands is
  precisely the owner who submits nothing more, so the count freezes below any
  threshold above 1 and the warning never arrives. Dropping the threshold to 1
  instead would fire on the routine daemon-restart blip the threshold existed to
  absorb. The settled rule keeps a count for reporting and moves the
  discrimination onto "unrepaired", which is observable without the owner
  acting.
- **A user-visible "hook delivery broken" affordance on the tab or nav row.**
  Rejected for now, not overlooked; reversible and additive later.
- **A daemon-side "this terminal has never posted" liveness probe** (the
  original capture's first candidate direction). Rejected as the primary
  mechanism: the daemon cannot distinguish "delivery is broken" from "the agent
  genuinely has not reached a turn boundary yet" without knowing the vendor's
  turn cadence, so it false-positives on every long turn. A failure count has no
  such ambiguity — it increments only when a delivery was actually attempted and
  actually failed.
- **Reusing `bound-base-url.json` as the failure surface.** Rejected.
  `agent_callback.rs:89-100` carries a standing do-not-read CONTRACT on that
  file, and the multi-daemon-steal argument behind it is unchanged.
- **A new periodic daemon task for the read side.** Rejected as redundant:
  `sweep_agent_profiles` already `read_dir`s `agent-profiles/` on a schedule
  (`server.rs:36` — 300 s; wiring at `server.rs:159-172`) and already holds the
  live-id set the reader needs.

### The escalation rule, and why a bare count threshold cannot bind

**The arithmetic that rules out a bare count.** The Claude hook config registers
exactly two events (`agent_profile_registry.rs:96-97` —
`UserPromptSubmit`→`working`, `Stop`→`ready`), so one turn produces exactly two
notify invocations and no more. Count what the two failure classes actually
generate: the stranded-state case corrupts delivery mid-turn and produces
**one** failed invocation (the `Stop` hook); the never-posted case corrupts
before the prompt and produces **two**. Any threshold above 2 is unreachable by
either class on its own evidence.

Worse, the headline case is structurally incapable of advancing. In the
stranded-state case the owner is *by definition* waiting on a badge that reads
`working`, so they submit nothing — no further hook fires, and the count freezes
at 1 for as long as the owner is away. A count-only threshold therefore
escalates only once the owner returns and drives roughly one and a half more
turns, i.e. only in the case where the owner is already interacting and would
notice the missing badge themselves. It binds least where it is needed most.

**The rule.** The daemon warns for a **live** terminal when all three hold:

1. A failure record exists with a count of at least 1.
2. Its most recent failure is at least one `AGENT_PROFILE_GC_SWEEP_PERIOD`
   (300 s, `server.rs:36`) old — the grace window in which a self-heal would
   have shown up as a success clearing the record.
3. `callback.json`'s mtime is **not** newer than the recorded failure timestamp:
   the daemon has not rewritten the callback target since the failure was
   recorded. A missing or unreadable `callback.json` supersedes nothing and
   satisfies this condition.

Condition 2 is what the threshold of 3 was reaching for — "the self-heal did not
happen" — expressed in the one dimension that keeps moving while the owner is
idle, which a count does not. Condition 3 is a *precise* discriminator for the
one legitimate single failure that motivated the threshold of 3: a daemon
restart rebinds an ephemeral port and the boot-reconcile adopt arm rewrites
`callback.json` with the fresh `base_url` (`agent_callback.rs:141-156`), so a
hook firing inside that window failed against a target that has since been
replaced. Comparing the failure timestamp against that rewrite *names* the
self-heal directly instead of inferring it from a count that will never advance.

**Why this binds in the idle-owner case.** One failed `Stop` delivery is enough.
The record is written by the failing hook process itself, so it exists the
moment the breakage occurs; conditions 2 and 3 are then evaluated by a sweep
that runs on the daemon's own clock, requiring nothing from the owner. Worst
case the warning lands two sweep periods (~600 s) after the break: one to pass
the grace window, one for the next tick to observe it.

**Residual false positive, accepted and named.** If the adopt rewrite lands
*before* an already-in-flight hook process finishes failing against the stale
value it had already read, condition 3 does not suppress that failure, and with
an idle owner no later success clears it — so one spurious warning can reach
`daemon.log`. The window is sub-second, the cost is one line in an
operator-facing log that is warn-once per terminal, and the alternative is never
warning at all in the case this ticket exists for. Both timestamps originate on
the same host's clock, so no skew term is needed.

**The count is reported, not gating.** Past "at least 1" the count does not
change the decision; it is carried into the warning because "failed once" and
"failed fourteen times consecutively" are different operator situations. This
also makes the writer's non-atomic read-modify-write benign: two overlapping
hook processes can lose an increment, but an undercount can never drop below 1,
so it can never suppress a warning.

- **Policy lives in the reader, not the writer.** `terminal-notify` only
  records; the daemon decides what is alarming. The writer is a short-lived
  process with no policy context, and a record is strictly more informative than
  a boolean.
- **Warn once per terminal per daemon lifetime.** The sweep runs every 300 s; a
  permanently broken terminal would otherwise emit the same warning forever.
  Keep an in-memory already-warned set of terminal ids. Drop an id when it
  leaves the live set, **and** when it is next observed with no failure record
  or a count of 0. A terminal that crossed the bound, was warned, self-healed,
  and then broke again has suffered a second, distinct breakage and must warn
  again; dropping only on the live-set exit would silently swallow it.

## Constraints

- **Do not extend `CallbackTarget`.** The failure record is writer-owned state
  belonging to the notify process; `callback.json` is daemon-owned state the
  notify process only reads. Folding one into the other would have the hook
  process rewriting a file the daemon authors. Use a separate sibling file.
- **The file and the shape are named here, not invented per module.** Both the
  writer (`terminal_notify.rs`) and the reader (`agent_profile_gc.rs`) bind to
  the same two names, mirroring how `agent_callback.rs` names both
  `callback_path` and `CallbackTarget` explicitly:
  - Path helper `notify_failure_path(profile_dir) ->
    <profile_dir>/notify-failures.json`.
  - Shape `NotifyFailureRecord { count: u32, last_failure_at_ms: u64,
    last_error: String }`, serialized `camelCase` like `CallbackTarget`
    (`count`/`lastFailureAtMs`/`lastError`).
  - Home: a small new module rather than `agent_callback.rs`, whose module
    CONTRACT is scoped to the callback target and its fresh-read guarantee.
- **`last_error` reuses `log_failure`'s own error string verbatim.** `deliver`
  returns `Result<(), String>` and `log_failure` is handed exactly that string
  (`terminal_notify.rs:56-57`); record the same value rather than inventing an
  error-kind tag or a second formatting, so the `daemon.log` warning and the
  `logs/terminal-notify.log.<date>` line name the same fault in the same words.
  Truncate to 512 bytes on a `char` boundary — the record is rewritten in place
  on every failure and must not grow with a pathological error string.
- **The already-warned set is an explicit parameter, not a module static.**
  `sweep_agent_profiles` is a free function whose blocking body runs inside
  `spawn_blocking` (`agent_profile_gc.rs:60-68`), so borrowed state cannot cross
  that boundary. Have the blocking pass *return* owned observations — the
  `(terminal_id, Option<NotifyFailureRecord>, Option<callback_mtime>)` triples
  for the live directories it skipped — and apply the escalation rule as a pure
  function on the async side against a `&mut NotifyFailureWatch` threaded
  through the signature. The watch is owned by the existing sweep loop in
  `server.rs:159-172`, so it lives exactly one daemon lifetime and dies with the
  task on shutdown-abort. A module static would leak warn-once state across this
  crate's in-process tests and would hide the very dependency the verification
  boundary wants exercised as pure logic. The four existing
  `sweep_agent_profiles` call sites in `agent_profile_gc.rs`'s tests
  (`:186`, `:216`, `:269`, `:295`) each gain a `&mut NotifyFailureWatch::default()`
  argument; that churn is accepted deliberately in exchange for explicit state.
- **Key the record by path sibling, not by parsed terminal id.** The most
  likely failure class is exactly "the callback file is missing, unreadable, or
  unparseable", in which case no terminal id can be parsed. The sibling path is
  derivable from `args.callback` alone, so this class stays counted.
- **The record writer must not create the profile directory.** If the parent
  directory is absent, skip the record write and let the existing log line
  stand as the only trace. `create_dir_all` here would resurrect a directory
  `sweep_agent_profiles` just reclaimed for a dead terminal, defeating the GC
  contract at `agent_profile_gc.rs:1-25`.
- **A record-write failure obeys the same silence rule as `log_failure`.**
  Swallow it; never produce stdout/stderr, never change the exit code
  (`terminal_notify.rs:104-106`).
- **`0600`, atomic temp-then-rename, with a UNIQUE temp name.** Follow
  `write_callback_target` (`agent_callback.rs:157-182`) for the
  create-at-`0600` sequence *only*: it creates the temp file at mode `0600`
  directly via `agent_token_store::create_new_file_at_mode_0600` and then
  renames. Do not copy the write-then-chmod sequence from
  `write_bound_base_url`.
- **Do not inherit `write_callback_target`'s fixed temp name.** Its doc comment
  justifies the fixed `callback.json.tmp` by its writers never overlapping (one
  daemon process, a per-spawn-unique `terminal_id` directory). That argument
  does not transfer: this record's writers are short-lived hook processes in
  *one* profile directory, and the module CONTRACT at `terminal_notify.rs:6-30`
  explicitly names a second concurrent invocation context (a developer running
  the command by hand while hooks fire). Two overlapping writers would
  truncate-then-write the same fixed temp path before renaming and publish a
  torn file — which reads back as "no record" and silently restores the exact
  silence this phase removes. This is the same exposure `unique_temp_path`
  (`agent_callback.rs:189`) was added to fix for `write_bound_base_url`; reuse
  it (it is currently private and needs `pub(crate)`).
- **The GC sweep's ordering contract is untouched.** The new read applies to
  directories the sweep currently *skips* (live ids). Do not let the read
  influence which directories are deleted.

## Prior Art

- `agent_profile_gc.rs` — the periodic pass this reader extends; already scans
  the right directory, already holds the live-id set, already `tracing::warn!`s.
- `agent_callback.rs::write_callback_target` (`:157-182`) — the atomic,
  create-at-`0600` per-terminal writer to mirror, but only for the
  create-at-mode sequence; see `## Constraints` on why its fixed temp name must
  not come with it.
- `agent_callback.rs::unique_temp_path` (`:189`) — the collision-free temp-name
  helper the record writer reuses, added for exactly this class of overlapping
  writers.
- `logging.rs::build_file_appender` — already reused by `log_failure` for
  bounded rotation; the daemon's own `daemon.log` shares that policy.
- `crates/daemon/tests/terminal_notify_end_to_end.rs` — drives the compiled CLI
  against a real daemon through a transparent TCP relay and asserts a real
  `204 No Content`. This is the delivery-assertion the original capture listed
  as a candidate direction; it already exists, so the new tests extend it rather
  than duplicate it.

## Corrections to the original capture

Recorded so a fresh session does not re-derive them or preserve a false premise.

- **Line references had drifted.** `run_terminal_notify` is at
  `terminal_notify.rs:55-60` (captured as `:37-42`); the log path and writer are
  at `:99-152` (captured as `:72-98`). `cli.rs:34-35` (`hide = true` above
  `TerminalNotify`) and `main.rs:21-28` still hold.
- **The parent ticket is closed, not mid-flight.** The capture framed the
  consequence as "once Phase 4 makes notifications real". Phases 4 through 8 all
  landed and the ticket sits in `.done/`. The gap is live in shipped behavior.
- **"No expiry" needed refinement, not retraction.** A session-liveness gate
  does exist (`agentAttention.ts:99-108`) and bounds the dead-session case. It
  does nothing for a live session, which is the case this ticket is about.
- **Related Finding 1 (write-then-chmod) is resolved for the credential-bearing
  case and must not be re-raised as open.** `write_callback_target`
  (`agent_callback.rs:157-182`) creates its temp file at `0600` directly at
  `:179`, proven by `write_callback_target_writes_at_mode_0600`
  (`agent_callback.rs:392-409`). The write-then-chmod sequence survives only in
  the two secret-free writers (`agent_callback.rs:131-136`,
  `agent_hook_config.rs:74-79`) — exactly the case the finding itself called
  acceptable.
- **Related Finding 2's risk did not materialize; its invariant stands.**
  `server.rs:80-87` computes `base_url` once from the in-memory `bound_addr` and
  threads it into both `write_bound_base_url` and `boot_reconcile`, with a
  CONTRACT comment forbidding a re-read. `bound-base-url.json` still has no
  production reader. Keep it that way — this ticket's rejected alternatives
  restate it.
- **An end-to-end delivery test already exists**
  (`crates/daemon/tests/terminal_notify_end_to_end.rs`), closing the capture's
  third candidate direction.

## Reproduction

A fresh session must confirm the failure before fixing it. Both levels are
worth running; the CLI level is deterministic and fast, the end-to-end level
proves the user-visible consequence.

**CLI level — proves the silence.**

1. Build the daemon binary and pick a scratch directory.
2. Write `callback.json` there with a well-formed but unusable target:
   `{"baseUrl":"http://127.0.0.1:1","terminalId":"t1","token":"wrong"}` (port 1
   refuses immediately, so the run is bounded by `CONNECT_TIMEOUT`,
   `terminal_notify.rs:52`).
3. Run `ws-dashboard terminal-notify --callback <that path> --state ready`,
   capturing stdout and stderr.
4. Observe: exit status `0`, stdout empty, stderr empty.
5. Observe: one new line in `<state dir>/logs/terminal-notify.log.<date>`, and
   **no** `notify-failures.json` beside `callback.json` — the failure leaves no
   state anything can act on.
6. Run it four more times. Observe: five lines, no escalation, no threshold, no
   change in exit status, and nothing anywhere else in the tree that reads those
   lines.

   *After the fix, the same six steps become the writer-side verification:*
   step 5 additionally yields `notify-failures.json` with `count: 1`, a
   `lastFailureAtMs` matching the log line's timestamp, and a `lastError`
   carrying the same text as the log line; step 6 leaves `count: 5`; and
   re-pointing `baseUrl` at a live daemon with a valid token and running once
   more clears the record. This level is deliberately writer-only — the
   escalation rule is the daemon's, and is exercised end-to-end below and as
   pure logic in unit tests.

**End-to-end level — proves the stranded-state consequence.**

1. Start the daemon and spawn an agent terminal from the browser with the Claude
   profile. Note its terminal id.
2. Send a prompt and confirm the tab badge goes `working`, then `ready` at the
   turn boundary. The chain is healthy.
3. Send another prompt. While the badge reads `working`, corrupt
   `<state dir>/agent-profiles/<terminal_id>/callback.json` — change `token` to
   a wrong value. The daemon holds the real token in memory
   (`terminal.rs::token_for`) and never re-reads this file, so the route now
   returns `401` to the hook.
4. Let the turn finish, then **submit nothing further** — this is the whole
   point of the case. The owner is waiting on the badge, so exactly one notify
   invocation (the `Stop` hook) fails and no further hook ever fires.
5. Observe the defect: the tab badge stays `working` indefinitely. The session
   status is still `running`, so the liveness gate does not clear it. `daemon.log`
   says nothing. The only trace is one more unread line in
   `logs/terminal-notify.log.<date>`.
6. Wait out two sweep periods (~10 minutes) with the daemon still running and
   the terminal still open, and confirm `daemon.log` stays silent for the whole
   window. That wait is what makes the fix's escalation observable at this
   level; before the fix it produces nothing.

**Do not shortcut step 6 by restarting the daemon** to force an immediate boot
sweep. The restart's boot-reconcile adopt arm rewrites `callback.json` with the
real token, which both repairs delivery and bumps the file's mtime past the
recorded failure — condition 3 of the escalation rule then correctly suppresses
the warning, and the shortcut reads as a broken fix rather than as the
self-heal it actually is.

For the never-posted class, corrupt the token *before* sending the prompt: no
badge ever appears, indistinguishable from an idle agent. This class produces
two failed invocations rather than one; neither class produces three, which is
why `## Decisions` does not gate on a count.

## Spec Impact

Contract-first spec: no.

The change is operator-facing and daemon-internal — no HTTP route, wire shape,
or browser-visible affordance changes — so a phase-owned spec entry is
proportionate. The entry is a merge gate for the phase, not a follow-up.

Target area: `ai-docs/spec/ws-web-dashboard/index.md`, in the attention
neighborhood immediately after
`#260726-dashboard-terminal-attention-event-stream`.

- **NEW entry — turn-state hook delivery-failure contract.** No existing stem
  covers the hook-to-daemon leg's failure behavior; the three existing attention
  sections all describe the daemon-to-browser leg. The entry must state: the
  hook process never writes stdout/stderr and never exits non-zero regardless of
  outcome (existing behavior, currently recorded only in a source comment);
  every failure is appended to a rotated daemon-side log; each failure is
  recorded per terminal as a count, a timestamp, and its error text, cleared on
  success; and the daemon surfaces, once per terminal, a failure that has gone
  unrepaired across a grace window and has not been superseded by a rewrite of
  the callback target. State the bound as "an unrepaired delivery failure
  becomes visible to the operator" rather than as a count threshold, since a
  single failure is sufficient and is the common case. It must also state what
  deliberately does *not* happen: no wall-clock expiry of attention state, and
  no user-facing affordance.
- **AMEND `#260726-dashboard-terminal-tab-attention-indicator`.** That section
  currently reads "This is a presentation gate, not a daemon guarantee, and it is
  deliberately the only defense". Tier the sentence rather than deleting it, and
  **do not let the tiering overclaim**: the badge itself gains no new defense
  from this change. On a live session a stranded badge stays stranded
  indefinitely — the liveness gate remains the only thing that ever clears a
  badge, and it only ever fires on a dead session. What the new entry adds is an
  operator-visible signal in `daemon.log` that delivery is broken, with no
  presentation-layer consequence. The amended sentence must say both halves
  explicitly, so a reader cannot conclude the stranded-badge case is now
  handled.

No `spec-remove:` entries.

## Phases

One phase, deliberately. The three captured findings are not three slices:

- The failure record and its reader are inseparable. A record with no reader is
  literally the defect this ticket names, so "add a counter" cannot be a
  reviewable slice — its completed behavior would reproduce the bug.
- The expiry is not a separate mechanism once it is correctly expressed. Per
  `## Decisions`, the bound belongs on the silence (an unrepaired failure), not
  on the attention state (a timer), and the escalation rule reads nothing but
  the record and the callback file's mtime. Splitting it off would leave an
  empty phase.

The resulting surface — the record write/clear in the notify path, a small
shared file shape, and an extension of an existing periodic sweep — is one
reviewable slice with no frontend change.

### Phase 1: Bound the silence with a per-terminal failure record and give it a daemon-side reader

**Completed behavior.** `terminal-notify` maintains a per-terminal failure
record — `NotifyFailureRecord { count, lastFailureAtMs, lastError }` in
`notify-failures.json`, written as a sibling of the callback file it was handed
and cleared on a successful delivery. The daemon's existing
`sweep_agent_profiles` pass reads that record for each live profile directory,
applies the three-condition escalation rule from `## Decisions`, and emits one
`tracing::warn!` per terminal naming the terminal id, the count, and the last
recorded error. After this phase a broken notify path is discoverable from
`daemon.log` within two sweep periods — including when the owner is idle and
exactly one delivery ever failed — without any UI change and without any new
background task.

Implement to the settled choices in `## Decisions` and the boundaries in
`## Constraints` — in particular: the escalation policy lives entirely in the
reader; the record is keyed by path sibling rather than parsed terminal id; the
writer never creates the profile directory and uses a unique temp name; the
already-warned set is threaded explicitly rather than kept in a module static;
and a record-write failure is swallowed exactly as `log_failure`'s failures are.

While in `terminal_notify.rs`, correct the now-stale error text at
`terminal_notify.rs:66-72` ("Phase 4 has not populated this callback target
yet") — Phase 4 shipped, and that message would misdirect the next person
reading a log line that contains it. This is incidental cleanup on contact, not
a separate concern.

**Deferred scope.**

- Any user-visible affordance for a broken hook path. Rejected in
  `## Decisions`; reversible and additive on top of what this phase builds.
- Any change to the frontend, to `pendingAttentionStateFor`, or to the attention
  wire shapes. This phase adds no browser-visible behavior.
- Any wall-clock expiry of attention state, in the daemon or the browser.
  Rejected in `## Decisions`.
- A never-posted liveness probe distinct from the failure counter. Rejected in
  `## Decisions`; the "never posted because the first delivery failed" case is
  already covered by the counter, and the "never posted because no hook ever
  fired" case has no unambiguous daemon-side signal.
- Retiring or finding a reader for `bound-base-url.json`. Out of scope, and its
  do-not-read invariant is restated here so this phase does not weaken it.

**Verification boundary.**

- Unit coverage for the record writer: increments the count across consecutive
  failures, clears the record on success, carries `lastError` forward as the
  same text `log_failure` receives (truncated at the stated bound), stamps
  `lastFailureAtMs`, tolerates an absent parent directory without creating it,
  and lands at mode `0600` on Unix.
- Unit coverage for the escalation rule as a **pure function** over
  `(record, callback_mtime, now)` plus the warn-once set — no filesystem, no
  live sweep. It must cover each condition failing independently: count `0`;
  a failure younger than the grace window; a `callback.json` mtime newer than
  the failure (the self-heal suppression); a missing `callback.json` (not
  suppressed); warn-once suppression on a second observation of the same
  unrepaired record; and the drop rule — warned, then observed with count `0`,
  then broken again must warn a second time.
- One test proving the idle-owner case specifically: a **single** recorded
  failure, aged past the grace window, with an unchanged callback target,
  produces a warning. This is the case the mechanism exists for and a threshold
  on count alone would fail it.
- One test that drives the compiled CLI against a deliberately-broken callback
  target and asserts the silence contract still holds after this change: exit
  status `0`, empty stdout, empty stderr. Extend the existing
  `crates/daemon/tests/terminal_notify_end_to_end.rs` harness rather than
  standing up a second one. This assertion is the regression guard on the
  contract this phase is most at risk of breaking.
- Daemon-side suite: `cargo test -p ws-dashboard-daemon`.

  **Two failures are KNOWN and pre-existing — judge by failure SITE, not by exit
  code.** Verified on this branch at `8bbb1f6d` before any change to this
  ticket's scope:

  - `dashboard_resources_refresh_prunes_workspace_without_available_work_roots`
    at `crates/daemon/tests/routes.rs:1066` — "pruned workspaces reappear only
    after an explicit open"
  - `online_missing_work_root_returns_bounded_unavailable_without_path_leak`
    at `crates/daemon/tests/routes.rs:1383` — `left: 409, right: 404`

  Baseline totals at that commit: `174 passed; 2 failed` in the `routes`
  integration target, all other targets green. The suite exits non-zero with or
  without this phase's changes, so the exit code carries no signal — read the
  log's `failures:` block and compare against the two sites above. A third
  failure site, or a change to either message, is this phase's regression.

- **Exit-status capture discipline (repo rule, and it bites here).** Run
  `cmd > file 2>&1` on one line and `echo $?` on the **next** line. Never
  `cmd | tee`, `cmd | tail`, or `cmd; echo $?` — the pipe forms report the
  pipeline's status and the semicolon form was observed during this ticket's own
  verification to report `0` for a run whose log clearly showed
  `test result: FAILED`. The log file is the source of truth, not the status.

- Manual confirmation against the end-to-end reproduction above: after the fix,
  the wait at step 6 produces exactly one `daemon.log` warning naming the
  terminal id, the count (`1`), and the last error — within two sweep periods,
  from the single failed delivery of step 4, with the owner submitting nothing.
  Then keep waiting one more sweep period and confirm the warning does **not**
  repeat. Heed the restart warning under that step: a daemon restart repairs the
  callback target and legitimately suppresses the warning, so it cannot be used
  to shorten this wait.
