---
title: Re-adopted agent terminals permanently lose profile provenance after a daemon restart
related:
  260725-feat-dashboard-pty-agent-attention-notification: found-during (Phase 2 adopt-arm CONTRACT); its Phase 7 nav counters are what the loss corrupts
  260725-bug-dashboard-terminal-registry-schema-evolution-orphans-helpers: relates - the helper-owned registry hazard this fix deliberately routes around rather than resolving
  260725-feat-dashboard-nav-row-two-line-open-state: owns the second nav line whose terminal count is over-reported by this bug
spec:
  - 260725-ws-web-dashboard-terminal-spawn-profile
related-mental-model:
  - ws-web-dashboard
---

# Re-adopted agent terminals permanently lose profile provenance after a daemon restart

## Background

`260725-feat-dashboard-pty-agent-attention-notification` Phase 2 added
`profile_id: Option<String>` to `TerminalSession` and `TerminalSessionView`
(`terminal.rs:781`, `terminal.rs:835`) to record which vendor profile (e.g.
`claude`) produced a terminal. It is in-memory only.

On daemon restart, `boot_reconcile` re-adopts every still-live helper it can
reach. `reconcile_entry`'s adopt arm rebuilds the session from the on-disk
registry entry and passes a literal `None` for the profile id
(`terminal.rs:344-387`; the `from_connection` call at `:348`, the `None` at
`:383`). Nothing about a live process re-announces which profile spawned it,
so the session reports `profileId: null` for the rest of its lifetime.

Unlike turn state - which adoption defaults to `idle` and the next hook
corrects - profile provenance has no correcting signal at all. The loss is
permanent, not transient, and a reader who pattern-matches the two will
wrongly conclude this one is harmless too.

### Corrections to the captured claims

The original capture was written quickly during a dogfood run. Seven of its
claims were re-checked against source; five needed correction.

1. **Line numbers were stale.** The capture cited `terminal.rs:241-274` /
   `:245` / `:254-272`. The adopt arm is now `terminal.rs:344-387`, the
   `from_connection` call `:348`, the literal `None` `:383`, and the CONTRACT
   comment block `:357-382`.

2. **"Hard no-new-field constraint" overstates the sibling ticket.**
   `260725-bug-dashboard-terminal-registry-schema-evolution-orphans-helpers`
   does not forbid new fields; it documents that an unversioned reader
   *skips* entries it cannot deserialize (`terminal_registry_file.rs:98-110`)
   and names `Option<T>` + `#[serde(default)]` as the working mitigation. The
   decisive reason the registry is the wrong carrier is **ownership, not
   schema**: `TerminalRegistryEntry` is written by the *helper* process
   (`terminal_helper_process.rs:195`), not the daemon, so a profile field
   would have to travel through helper argv and make the helper the author of
   daemon-side metadata.

3. **"Silently falls out of the AGENT counter - an under-count" is half the
   story.** `terminalCountByRoot` excludes agent panes with the predicate
   `pane.session.profileId == null` (`App.tsx:4534-4536`, `:4545-4561`), so a
   re-adopted agent pane is *simultaneously* dropped from the agent counts and
   added to the plain terminal count. The row under-reports agents **and**
   over-reports terminals - a double error in opposite directions, both
   silent.

4. **Phase 7 is not future work.** The parent ticket is closed
   (`.done/`, `completed: 2026-07-26`) and the nav counters shipped. The
   consequence is live, not prospective. `App.tsx:4530-4533` already names
   this ticket stem as a known open gap in a source comment.

5. **The blast radius is narrower than "the terminal loses its agent
   identity".** Two agent surfaces are unaffected and must not be described as
   broken: the per-tab attention indicator and the Phase 8 browser-level cue
   are profile-independent, keying off `attentionByKey` through
   `pendingAttentionStateFor` (`terminalWorkbenchPane.tsx:92-97`,
   `agentAttention.ts:99`); and the callback token *is* recovered on adopt
   (`recover_callback_token`, `terminal.rs:416-438`), so hooks keep
   authenticating and keep posting turn state after a restart. Only the
   nav-row counts are wrong.

6. **The "sidecar file" candidate is not a new mechanism.** The daemon
   already owns a per-terminal sidecar lane under the state dir
   (`terminal-tokens/<id>.json`, `agent-profiles/<id>/{settings,callback}.json`)
   and already reads it back at adopt time (`terminal.rs:416-438`). That is
   why it becomes the settled choice below rather than one option among four.

7. **The browser cannot repair this.** The frontend does persist `profileId`
   in restore intents (`terminals.ts:95-115`), but that path only fires when
   `listTerminals` returns zero sessions - an adopted session *is* returned,
   so the intent never applies.

Confirmed as captured: `TerminalRegistryEntry` carries no profile field and no
version field (`terminal_registry_file.rs:16-27`); adoption passes `None`; and
the loss is permanent.

## Reproduction

Manual, against a real daemon:

1. Start the daemon and open a work root in the browser.
2. Spawn an agent terminal through the toolbar control that dispatches
   `terminal.create.agent` (`App.tsx:7201`, `:7214` -> `createTerminalPane({
   profileId: "claude" })` at `App.tsx:5959`). The work-root row's second line
   reports one agent and does *not* include the pane in its terminal count.
3. `GET /api/dashboard/work-roots/<id>/terminals` reports
   `"profileId": "claude"` for that session.
4. `SIGKILL` the daemon - not a graceful stop; the detached helper must
   survive. Restart it on a fresh port.
5. Repeat the GET. The same `terminalId` is listed (adoption worked) but now
   reports `"profileId": null`.
6. In the browser, that same pane is now counted as a plain terminal and the
   root reports zero agents.

Automated, and the shortest path to a failing assertion:
`crates/daemon/tests/terminal_notify_callback_restart.rs` already builds this
exact scenario (two real daemon processes, hard `SIGKILL` between them, owner
auth, terminal spawned with `profileId: "claude"`) and already resolves the
adopted entry from daemon #2's terminal listing. Asserting
`adopted["profileId"] == "claude"` at that existing lookup fails today.

## Decisions

**Settled: persist the profile id in the daemon-owned sidecar lane and read it
back on adopt.** A new per-terminal file
`<state_dir>/agent-profiles/<terminal_id>/profile.json`, `0600`, written with
the same atomic temp-rename shape as its siblings:

```json
{ "profileId": "claude" }
```

Written at spawn for **any** resolved profile; read in `reconcile_entry`'s
adopt arm alongside `recover_callback_token`, replacing the literal `None`.
Chosen because this lane already exists, is already daemon-owned, is already
read at adopt, and is already reclaimed by the GC sweep
(`agent_profile_gc.rs::sweep_agent_profiles` removes
`agent-profiles/<orphan>/` wholesale) - so the fix adds a file, not a
lifecycle.

**Kept separate from `callback.json`, deliberately.** Merging them would force
one of two bad outcomes: minting a callback token for a hookless profile
(broadening the credential surface for no reason), or letting `callback.json`
exist without a token - which would break the invariant
`recover_callback_token` depends on, where the *presence* of `callback.json`
is what distinguishes "spawned with hooks" from a plain shell
(`terminal.rs:416-422`).

**Load-bearing ordering change.** `agent-profiles/<terminal_id>/` is currently
created only inside the `hook_config.is_some()` branch, and
`mark_profile_pending` sits inside that same branch (`terminal.rs:1479`).
Writing `profile.json` for a hookless profile (`dummy-echo` is the registered
example) creates that directory on a path the pending mark does not cover,
which reopens exactly the concurrent-spawn GC race `pending_profile_ids`
exists to close (`terminal.rs:162-187`). The mark must therefore be hoisted to
run before the first byte of the directory is written for **any** resolved
profile. The failure paths that clear the mark (`terminal.rs:1564-1566`,
`:1571-1578`) already call unconditionally and need no change.

**Degrade rules, matching the existing precedents rather than inventing new
ones.** An unresolved `state_dir` writes no sidecar and logs a warning, the
same shape as the hookless-spawn degrade at `terminal.rs:1529-1534`. A
sidecar write failure logs an error and the spawn continues - a terminal must
never fail to start over provenance metadata. A missing or malformed sidecar
reads back as `None`, mirroring `agent_token_store::read_token`'s tolerant
read. In every degrade the observable result is today's behavior, never worse.

**The default no-profile spawn writes nothing.** A create request that names
no profile takes no new branch and creates no directory, preserving the
"unchanged byte for byte from a request that names no profile at all" contract
the spawn-profile spec entry already states.

**Adopt echoes the recorded id without re-validating it against the profile
registry.** The field is provenance - what spawned this terminal - not a live
capability claim, and `TerminalSessionView.profileId` is already an opaque
string to every consumer. Re-resolving through
`agent_profile_registry::resolve` would erase provenance for a still-running
terminal the moment its profile is renamed or retired, which is a worse
outcome than reporting an id that no longer resolves.

**No backfill.** Terminals spawned before this lands have no sidecar and still
report `null` on the first restart after the upgrade. Accepted: the population
is bounded by one restart and self-clears as those terminals are closed.

**Verification stays at daemon level; no browser acceptance is added.** The
frontend is unchanged by this fix, Phase 7's unit tests already pin the
counter's behavior given a non-null `profileId`, and the acceptance harness
serves a prebuilt `frontend/dist`
(`260726-chore-e2e-playwright-serves-stale-frontend-dist`), so a browser run
here would cost time without producing evidence the daemon test does not
already produce.

### Rejected alternatives

- **Add an `Option<String>` field to `TerminalRegistryEntry`.** Technically
  viable with `#[serde(default)]`, and the sibling ticket's hazard is about
  non-`Option` fields. Rejected on ownership: the file is written by the
  helper (`terminal_helper_process.rs:195`), so the daemon would have to pass
  the profile id through helper argv and make a helper the author of
  daemon-side provenance - a wider change than the fix warrants, and it
  couples this bug to an unresolved schema-versioning ticket for no gain.

- **Sniff the adopted process's argv through OS process-inspection APIs.**
  Rejected: three platform implementations for a *guess*. Argv does not map
  back to a profile id unambiguously (`dummy-echo` is `/bin/sh -c ...`), and
  the vendor process is the helper's child, not the helper, so the daemon
  would have to walk the process tree first.

- **Have the browser repair the adopted session from its restore intent.**
  Rejected: it makes a client the authority for daemon state, needs a new
  write route, and the intent store is per-browser `localStorage` - a
  different browser, a cleared profile, or no open tab all lose it. See
  correction 7 for why the existing intent path does not fire here anyway.

- **Accept the uncertainty and render an explicit "unknown" state.** Rejected:
  the spec already asserts no-double-count, and a third segment would land in
  a line Phase 7 measured at 313px against a 225px box. Paying UI budget to
  display a defect we know how to remove is the wrong trade.

### Forward-compatibility guardrail

`profile.json` is daemon-owned and deliberately unversioned, but every field
on its struct is `Option<T>` + `#[serde(default)]` and a malformed file reads
as absent - so it can never become a second instance of the helper-owned
registry's skip-the-whole-entry hazard. It must also never carry a secret: the
callback token stays in `terminal-tokens/` and `callback.json`.

## Constraints

- Do not modify `TerminalRegistryEntry`. This ticket routes around the
  helper-owned registry; it does not resolve
  `260725-bug-dashboard-terminal-registry-schema-evolution-orphans-helpers`,
  which stays open and untouched.
- The GC-sweep-after-`boot_reconcile` ordering
  (`agent_profile_gc.rs` module CONTRACT, wired at `server.rs:130-150`) is
  unchanged and must stay proven - the new hookless profile directory falls
  under the same guarantee.
- Several source comments currently assert the permanence of this loss as a
  design fact. Leaving them in place would be a comment claiming a guarantee
  its code does not provide - the exact defect class the parent ticket's Phase
  7 Result named four times. Sweep them with a search for `profile_id` /
  `profileId` across `crates/daemon/src` and `frontend/src` and check every
  CONTRACT block; the two load-bearing ones are the adopt-arm block
  (`terminal.rs:357-382`) and `recover_callback_token`'s contrast with it
  (`terminal.rs:403-408`). The frontend's known-gap note naming this stem
  (`App.tsx:4530-4533`) must go with them.

## Spec Impact

Two existing entries in `ai-docs/spec/ws-web-dashboard/index.md` are addressed;
no new spec entry is needed.

- `260725-ws-web-dashboard-terminal-spawn-profile` (index.md:2124) - **text
  changes.** It currently asserts the bug as intended behavior: "that
  provenance does not survive a daemon restart: a session reattached during
  boot reconciliation is rebuilt from the on-disk terminal registry alone,
  which never carries a profile id." After the fix, a reattached session
  reports the profile it was spawned with; the on-disk terminal registry still
  never carries a profile id, and the provenance is restored from daemon-owned
  per-terminal state instead. The remaining honest caveat to state: a terminal
  spawned before this behavior existed, or one whose daemon had no resolvable
  state directory, still reattaches without provenance.
- `260725-nav-row-open-surface-counts-and-open-state` (index.md:1087) - **no
  text change.** It already asserts "An agent terminal is reported by the agent
  counts only and is never also included in the terminal count, so no open
  surface is counted twice." That sentence is currently false after a daemon
  restart; the fix restores conformance rather than changing the contract.

Contract-first spec: no. The behavior is a restoration of an already-specified
contract plus an amendment to an entry that describes a limitation being
removed, so the spec edit follows the implementation in the same phase.

## Phases

### Phase 1: persist spawn-profile provenance daemon-side and restore it on boot-reconcile adopt

Single phase - the write, the read, and the comment sweep are one reviewable
slice with no sequential dependency between them.

**Completed behavior.** A terminal spawned with any resolved profile records
that profile id in `<state_dir>/agent-profiles/<terminal_id>/profile.json`, and
a session re-adopted by `boot_reconcile` reports that same profile id in
`TerminalSessionView.profileId` instead of `null`. Consequently a work-root
nav row that survives a daemon restart counts a re-adopted agent terminal in
the agent segment only, as
`260725-nav-row-open-surface-counts-and-open-state` already requires. Includes
hoisting `mark_profile_pending` to cover the hookless-profile directory
creation, the degrade rules above, the spec amendment, and the comment sweep
named in `## Constraints`.

**Deferred scope.** No backfill for terminals spawned before this lands. No
change to `TerminalRegistryEntry` and no work on the registry schema-versioning
ticket. No other provenance in the sidecar - profile id only, not turn state,
not title, not cwd. No change to the nav-row presentation, the counter
derivation, the attention stream, or the acknowledgement watermark. No new
registered profile.

**Verification boundary.** Daemon-level only, no browser acceptance (see the
decision above).

- Extend the existing two-real-daemon `SIGKILL` restart harness
  (`crates/daemon/tests/terminal_notify_callback_restart.rs`) at its existing
  adopted-entry lookup to assert the adopted terminal reports
  `profileId: "claude"`. Non-vacuous by construction: that assertion fails on
  the current tree.
- Cover the hookless branch at daemon lib level rather than through the
  restart harness: a `boot_reconcile` test in the style of this file's
  existing `boot_reconcile_drops_entry_*` tests, driving the real async path
  against a pre-written sidecar, plus a spawn-side test that a resolved
  profile with `hook_config: None` creates `profile.json`. Reason for the
  split rather than spawning `dummy-echo` through the restart harness: that
  profile's process lives 30s, which is not a dependable budget for a
  two-process kill-and-rebind test - the parent ticket already paid for this
  once when Phase 6 needed a 180s variant. Adding a third test-only profile
  purely to reach the harness was rejected as disproportionate.
- Keep the harness's GC-ordering regression assertion true for the hookless
  directory as well: a directory created for a profile with no hook config
  must also survive daemon #2's sweep.
- Confirm the degrade paths: no `state_dir` and a failed sidecar write both
  leave spawn succeeding and adoption reporting `null`, exactly as today.
