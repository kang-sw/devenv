# Plan: 260727-chore-merge-ws-dashboard-dev-into-goal-branch — Phase 3: discharge drain_all's removal obligations and cover the kill-all endpoint

## Relevant Ticket Contract

- Fix shape, mirroring `remove_for_work_roots`: drop the `sessions` write lock
  first, THEN loop over the drained sessions calling `forget_token` and
  `attention.forget` for each. Do not call either while still holding the
  `sessions` write lock.
- Add a CONTRACT comment on `drain_all` itself, mirroring the two on `remove`.
- `sessions_write_lock_sites_are_enumerated`'s expected count stays **5** (Phase 2
  already moved it there); only its enumerating comment's `drain_all` clause
  changes, from "discharges neither" to "discharges both".
- Invariant 5: integration test `close_all_terminals_revokes_callback_tokens` in
  `crates/daemon/tests/routes.rs` — create a terminal with a callback token, POST
  its turn-state and assert **204** (non-vacuity guard, load-bearing per the
  ticket), POST kill-all, POST the same turn-state again and assert **401**, and
  assert `terminal-tokens/<id>.json` is gone. Mutation check: reverting `drain_all`
  to its merged (broken) form must make the post-kill POST return 204 and leave
  the token file present, while the pre-kill control stays green.
- Invariant 6: unit test `drain_all_forgets_the_attention_entry`, modeled directly
  on `remove_forgets_the_attention_entry`, using `insert_fake_live_session_for_test`.
  Assert the attention entry is present before the drain and absent after.
  Mutation check: deleting the `attention.forget` call from the fixed `drain_all`
  must fail this test at its own site while the token test stays green.
- Same phase must widen the spec sentence's parenthetical under
  `{#260726-dashboard-terminal-attention-event-stream}` to name kill-all as a
  third close path.
- Deferred scope (explicit non-goal): this phase does NOT close `insert`'s
  eviction-path token gap. If no ticket already tracks it, open an `idea/` one
  rather than folding the fix in here.
- Verification boundary: `cargo test -p ws-dashboard-daemon --no-fail-fast`, both
  mutations run and observed to fail at their own sites, and the failure-site list
  still matching Phase 1's baseline (re-measured fresh, not diffed against a
  stale recorded list — machine load from leaked helper processes has produced
  spurious extra failures in `routes.rs`/`terminal_lifetime.rs`/
  `terminal_notify_callback_restart.rs` in this ticket's own history).

## Out of Scope

- Phase 1's five tests (already landed, `### Result (3c6b465f)`).
- Phase 2's merge resolution and companion edits (already landed,
  `### Result (62a9bc3c)`) — do not re-touch `settingsSections.tsx`,
  `settingsSections.test.ts`, `server.rs`, `styles.css`, `_index.md`, or the
  research ticket.
- `insert`'s eviction-retain token gap itself (only the routing/idea-ticket
  decision is in scope, not the fix).
- Phase 4 (`_index.md` inventory parity) and Phase 5 (routing the Advanced-panel
  spec debt to `260725-feat-dashboard-graceful-shutdown-from-settings`) — separate
  commits, not this phase's work.
- Authoring new spec text beyond widening the one existing parenthetical; the
  token half of Phase 3 (`## Spec Impact` item 2) is explicitly deferred to "doc
  closeout", not this implementation pass.

## Codebase Findings

- `ws-dashboard/crates/daemon/src/terminal.rs#L718-L735` — current `drain_all`.
  Doc comment lines 718-722 state the kill obligation; lines 723-728 are the
  `PARITY BROKEN (260727 Phase 2)` clause that must be replaced (it currently
  says `drain_all` "still forgets neither" and "Phase 3 owns closing that gap; do
  not fix it here" — both become stale the moment this phase lands). The fn body
  (729-735) currently just takes `self.sessions.write()` and drains, discharging
  nothing:
  ```rust
  pub fn drain_all(&self) -> Vec<Arc<TerminalSession>> {
      let mut sessions = self.sessions.write().expect("terminal registry lock poisoned");
      sessions.drain().map(|(_, session)| session).collect()
  }
  ```
- `ws-dashboard/crates/daemon/src/terminal.rs#L690-L716` — `remove_for_work_roots`,
  the pattern to mirror: takes `sessions.write()`, `retain`s out the matching
  sessions into a `removed` Vec, **`drop(sessions)`** (line 707), THEN loops
  (708-714) calling `self.forget_token(&session.id)` and
  `self.attention.forget(&session.id)` per drained session, then returns
  `removed`. `drain_all`'s fix must reproduce exactly this shape: collect the
  drained `Vec<Arc<TerminalSession>>` first, drop the sessions write guard, loop
  over the vec calling both forgets, then return the vec unchanged (callers —
  `close_all_terminals` — still need the full list to `.terminate()` each).
- `ws-dashboard/crates/daemon/src/terminal.rs#L653-L665` — `forget_token`: its
  body takes **`self.tokens.write()`** (655-658, a *different* `RwLock` from
  `self.sessions`), then best-effort deletes the on-disk file. Why the drop-then-
  loop matters: `self.sessions` and `self.tokens` are separate locks, so nesting
  them (sessions held while acquiring tokens) is not a same-thread self-deadlock
  today — no other path in this file acquires `tokens` then later needs
  `sessions` (checked: `token_for` only reads `tokens`; `remember_token`/
  `forget_token` only write `tokens`). The concrete hazard the drop-then-loop
  actually guards against is **lock-hold duration**: `forget_token` does
  synchronous disk I/O (`agent_token_store::delete_token`), so calling it once
  per drained session while still holding `self.sessions.write()` would block
  every other reader/writer of the sessions map (every terminal list/create/
  attach/close HTTP request) for the entire kill-all sweep, proportional to
  terminal count. `remove_for_work_roots`'s existing drop-then-loop is the
  precedent that avoids this; nesting the locks would also remove the standing
  invariant that `sessions` and `tokens` are never held simultaneously by the
  same call path, which is what keeps a future reverse-order acquisition
  elsewhere from becoming a real ABBA deadlock.
- `ws-dashboard/crates/daemon/src/terminal.rs#L667-L680` — `remove`, whose two
  CONTRACT comments the new `drain_all` doc comment must mirror:
  1. `forget_token`'s own CONTRACT at **L650-L652** ("deletes the matching
     `self.tokens` entry AND best-effort deletes the on-disk token file - the
     other of the four lockstep choke points").
  2. The inline CONTRACT directly above `self.attention.forget(terminal_id)` at
     **L674-L677** ("mirrors `forget_token` exactly - the other lockstep choke
     point a closed terminal's attention snapshot entry must be forgotten at, or
     a reconnect's snapshot would show a phantom terminal after close").
- `ws-dashboard/crates/daemon/src/terminal.rs#L2431-L2465` —
  `sessions_write_lock_sites_are_enumerated`. The enumerating CONTRACT comment
  (2431-2450) currently reads, for `drain_all`: "(260727 Phase 2, arrived with the
  ws-dashboard-dev merge: it takes the write lock and empties the whole map, yet
  discharges NEITHER obligation ... That 'discharges neither' state is knowingly
  landed here and is Phase 3's to fix; Phase 3 rewrites this line to 'discharges
  both' once it does". This phase rewrites exactly that clause to state
  `drain_all` now discharges both obligations, alongside `remove` and
  `remove_for_work_roots`. The test body (2452-2465) asserts count == 5 — **do
  not change the asserted number**, only the prose above it.
- `ws-dashboard/crates/daemon/src/terminal.rs#L3411-L3428` —
  `remove_forgets_the_attention_entry`, the model for the new
  `drain_all_forgets_the_attention_entry` unit test: builds
  `TerminalRegistry::default()`, seeds via
  `insert_fake_live_session_for_test(&registry, "term_forget_on_remove")`,
  publishes an attention entry via `registry.attention.record_and_publish(...)`,
  asserts `snapshot().len() == 1`, calls `registry.remove(...)`, asserts
  `snapshot().is_empty()`. The new test swaps `registry.remove(id)` for
  `registry.drain_all()` and otherwise copies this shape — attention-only, no
  token needed, so `insert_fake_live_session_for_test` (not the token-bearing
  sibling) is correct here, matching the ticket's own Invariant 6 wording.
  Because it only reads the in-memory `attention` hub (no disk assertions),
  `TerminalRegistry::default()` is safe to reuse for this one test (the
  real-state-dir trap below applies only to on-disk token assertions).
- `ws-dashboard/crates/daemon/src/terminal.rs#L3839-L3909` —
  `insert_fake_live_session_for_test` (hard-codes `callback_token: None`, so it
  cannot seed a token — `remember_token` no-ops on `None`) and its sibling
  `insert_fake_live_session_with_token_for_test` (identical except
  `callback_token: Some(token.to_owned())`), added in Phase 1 specifically for
  the token tests. Both call `registry.insert_unchecked(session)`. Per
  `ai-docs/mental-model/ws-web-dashboard/terminal.md`'s "Common Mistakes"
  section: (a) `TerminalRegistry::default()` resolves
  `crate::persistent_state::default_state_dir()` — the developer's **real** state
  directory — so any test asserting on-disk state must build the registry via
  `TerminalRegistry::new(...)` with an explicit temp `state_dir` instead (as
  `remove_forgets_the_callback_token`, L3516-L3563, already does); (b) neither
  `insert`/`insert_unchecked` writes `terminal-tokens/<id>.json` —
  `remember_token` only populates the in-memory map — so a fixture needing the
  on-disk file present must call `agent_token_store::write_token` directly (as
  `remove_forgets_the_callback_token` does at L3535-L3536). These two traps are
  specific to the **unit** tests in `terminal.rs`; the new integration test
  (Invariant 5) sidesteps both by going through the real HTTP create-terminal
  path with a hook-bearing profile, which does write the real token file via
  `TerminalSession::spawn` (see below), and by using
  `test_terminal_registry_with_state_dir()`, never `TerminalRegistry::default()`.
- `ws-dashboard/crates/daemon/src/router.rs#L441-L444` — the kill-all route:
  `.route("/api/dashboard/terminals/kill-all", post(crate::terminal::close_all_terminals))`.
  This route is registered inside the `protected` router built at L155 and merged
  behind `require_owner_auth` at L528 (`protected.layer(from_fn_with_state(state.clone(), require_owner_auth))`)
  — **unlike** the turn-state route, which `build_router`'s own CONTRACT comment
  (L146-148) states is deliberately registered in the outer chain "before
  `.merge(protected)`... so it sits outside `require_owner_auth`". Consequence for
  the new integration test: the kill-all POST needs the owner-session cookie
  (same `cookie` already obtained via `pair_and_cookie`), while the turn-state
  POSTs (both before and after the kill) must NOT carry it, per the existing
  turn-state tests' own pattern.
- `ws-dashboard/crates/daemon/src/terminal.rs#L1535-L1549` — `close_all_terminals`:
  `state.terminals.drain_all()` then `for session in sessions { session.terminate().await; }`,
  returning `Json({"closed": closed})`. No change needed here — it already treats
  `drain_all`'s return value as "every session this caller must independently
  kill", exactly the shape `drain_all`'s fix preserves.
- `ws-dashboard/crates/daemon/tests/routes.rs#L16619-L16746` — the exact fixture
  chain to reuse for the new integration test, all pre-existing:
  `test_terminal_registry_with_state_dir()` (L161-171, returns
  `(TerminalRegistry, PathBuf)` pointed at an isolated temp state dir — never
  `TerminalRegistry::default()`), `app_state_with_terminal_registry(registry)`
  (L16619-16631, re-points `state.attention` at the incoming registry's hub —
  load-bearing per its own CONTRACT comment, or the drain's `attention.forget`
  calls land in a hub the test's assertions never observe),
  `create_terminal_with_profile_for_test(app, cookie, work_root_id, "claude")`
  (L16642-16679, passes `profileId: "claude"` so `TerminalSession::spawn`'s
  hook-config branch actually materializes a callback token + writes
  `terminal-tokens/<id>.json` — a plain shell terminal via the ordinary
  `create_terminal_for_test`, L13459, never gets a token at all),
  `read_callback_token_from_disk(&state_dir, &terminal_id)` (L16685-16697, reads
  `state_dir/terminal-tokens/<id>.json`'s `"token"` field directly off disk — no
  public API surface exposes it), `turn_state_request(app, terminal_id, token,
  state)` (L16699-16719, POSTs `{token, state}` to
  `/api/dashboard/terminals/{id}/turn-state` with no cookie, returns the raw
  `StatusCode`). The model test
  `turn_state_route_accepts_a_valid_token_with_owner_auth_enabled_and_no_cookie`
  (L16721-16746) chains all of these and is the closest existing template for
  the pre-kill 204 half of the new test; `turn_state_route_rejects_a_wrong_token`
  (L16748-16768) is the template for the 401 assertion shape.
- `ws-dashboard/crates/daemon/src/agent_token_store.rs#L35-L45` — `fn token_dir`
  returns `state_dir.join("terminal-tokens")`; `token_store_path(state_dir,
  terminal_id)` (L38) is `token_dir(state_dir).join(format!("{terminal_id}.json"))`.
  The new integration test asserts
  `!agent_token_store::token_store_path(&state_dir, &terminal_id).exists()`
  after the kill, mirroring `remove_forgets_the_callback_token`'s existing
  on-disk assertion shape (`terminal.rs#L3557-L3560`).
- `ai-docs/spec/ws-web-dashboard/index.md#L2492-L2495` — the exact sentence to
  widen, quoted verbatim: *"A terminal's attention entry is removed from the
  snapshot the moment its underlying terminal session closes (explicit close or
  owning workRoot/workspace removal), so a reconnect never reports state for a
  terminal that no longer exists."* The parenthetical `(explicit close or owning
  workRoot/workspace removal)` is the two-wide enumeration the ticket says must
  become three-wide, naming kill-all. No other wording in the sentence needs to
  change.
- Idea-ticket search for `insert`'s eviction-path token gap (Phase 3's stated
  deferred scope): grepped `ai-docs/tickets/idea/` and `ai-docs/tickets/todo/`
  for `callback.token`, and the wider tree for the CONTRACT comment's own
  language (`insert`'s eviction retain / FIFTH session-removal path / "the
  callback-token half of this same gap is Phase 4's inherited debt"). **No
  ticket anywhere under `idea/`, `todo/`, or `ready/` tracks this gap** — the
  only hits are the ticket-under-execution itself and the `.done/` ticket that
  originally deferred it (`260725-feat-dashboard-pty-agent-attention-notification.md`,
  whose CONTRACT comment on `insert`'s eviction retain records the deferral but
  opens no follow-up ticket for it). Per the ticket, Phase 3 must open one.

## Implementation Plan

1. **Fix `drain_all`** in `ws-dashboard/crates/daemon/src/terminal.rs` (currently
   L718-735). Replace the doc comment's `PARITY BROKEN (260727 Phase 2)` clause
   (L723-728) with a CONTRACT comment mirroring `forget_token`'s (L650-652) and
   `remove`'s attention CONTRACT (L674-677) — stating that `drain_all` now
   forgets both the callback token and the attention entry for every drained
   session, at the same two choke points `remove`/`remove_for_work_roots` use.
   Rewrite the body to the `remove_for_work_roots` drop-then-loop shape:
   ```rust
   pub fn drain_all(&self) -> Vec<Arc<TerminalSession>> {
       let drained: Vec<Arc<TerminalSession>> = self
           .sessions
           .write()
           .expect("terminal registry lock poisoned")
           .drain()
           .map(|(_, session)| session)
           .collect();
       for session in &drained {
           self.forget_token(&session.id);
           self.attention.forget(&session.id);
       }
       drained
   }
   ```
   (The write guard is a temporary dropped automatically at the end of the
   `.drain()...collect()` statement, matching `remove_for_work_roots`'s explicit
   `drop(sessions)` in effect — either an explicit `drop` or scoping the guard to
   end before the loop is acceptable, as long as `forget_token`/`attention.forget`
   run with no `sessions` guard alive.)

2. **Update the enumerating CONTRACT comment** on
   `sessions_write_lock_sites_are_enumerated` (`terminal.rs#L2431-2450`): change
   the `drain_all` clause from "discharges NEITHER obligation... Phase 3's to
   fix; Phase 3 rewrites this line to 'discharges both' once it does" to state it
   now discharges both, alongside `remove` and `remove_for_work_roots`. Do
   **not** change the asserted count (stays 5, `terminal.rs#L2456-2464`).

3. **Add unit test `drain_all_forgets_the_attention_entry`** in
   `terminal_portability_skeleton_tests` (colocate near
   `remove_forgets_the_attention_entry`, `terminal.rs#L3411-3428`), copying its
   shape: `TerminalRegistry::default()`, `insert_fake_live_session_for_test`,
   `record_and_publish`, assert `snapshot().len() == 1`, call
   `registry.drain_all()`, assert `snapshot().is_empty()`.

4. **Add integration test `close_all_terminals_revokes_callback_tokens`** in
   `ws-dashboard/crates/daemon/tests/routes.rs`, placed near the other
   turn-state-route tests (after L16768 area). Shape, built entirely from
   existing helpers:
   - `test_terminal_registry_with_state_dir()` -> `(registry, state_dir)`.
   - `app_state_with_terminal_registry(registry)` -> `state`; capture
     `state.auth.pairing_token()...` before `build_router(state)` moves it, as
     the existing tests do.
   - `pair_and_cookie`, `open_work_root_for_test` against a fresh
     `temp_fixture_path(...)` workRoot.
   - `create_terminal_with_profile_for_test(app.clone(), cookie, work_root_id, "claude")`
     -> `terminal_id`.
   - `read_callback_token_from_disk(&state_dir, &terminal_id)` -> `token`.
   - POST turn-state via `turn_state_request(app.clone(), &terminal_id, &token, "working")`
     and **assert `StatusCode::NO_CONTENT`** — the non-vacuity guard the ticket
     calls load-bearing, not a courtesy.
   - POST `/api/dashboard/terminals/kill-all` **with the owner cookie**
     (`.header(header::COOKIE, cookie)`, since this route sits behind
     `require_owner_auth` unlike turn-state) and assert a 2xx status.
   - POST the same turn-state again via `turn_state_request(app.clone(), &terminal_id, &token, "working")`
     and assert `StatusCode::UNAUTHORIZED`.
   - Assert `!ws_dashboard_daemon::agent_token_store::token_store_path(&state_dir, &terminal_id).exists()`.
   - Clean up `remove_static_fixture(&root)` and `fs::remove_dir_all(&state_dir)`
     as the sibling tests do.
   (Check whether `agent_token_store` is already imported/used in `routes.rs`
   elsewhere — if referenced only via `crate::agent_token_store::...` inside
   `terminal.rs`'s own test module, this integration test crate will need the
   fully-qualified `ws_dashboard_daemon::agent_token_store::token_store_path`
   path, matching how other cross-module daemon items are referenced in this
   same test file, e.g. `ws_dashboard_daemon::discovery::GitProbeCache`.)

5. **Widen the spec sentence** at `ai-docs/spec/ws-web-dashboard/index.md#L2492-2495`:
   change the parenthetical `(explicit close or owning workRoot/workspace
   removal)` to a three-item form naming kill-all (e.g. `(explicit close, owning
   workRoot/workspace removal, or a kill-all sweep)`), preserving the rest of the
   sentence verbatim. This is closeout wording only — the behavior is already
   fixed by step 1; no other spec edit in this phase (the token half, `## Spec
   Impact` item 2, is explicitly deferred to doc closeout, not this pass).

6. **Open an `idea/` ticket** for `insert`'s eviction-path callback-token gap,
   since the survey found none tracking it. Scope: record that `insert`'s
   eviction `retain` (the CONTRACT comment above `evicted_ids`,
   `terminal.rs#L569-581`) forgets a dropped session's attention entry but not
   its callback token/on-disk file, that this predates both merged branches, and
   that closing it is a behavior change with its own blast radius (per this
   ticket's own framing, do not fold a fix into this ticket).

## Verification Plan

- `cargo test -p ws-dashboard-daemon --no-fail-fast` (both the new unit test and
  the new integration test must be visible in the run; `--lib` alone is not
  sufficient since Invariant 5 lives in `tests/routes.rs`).
- Re-measure the failure-site baseline immediately before this run (per the
  ticket's own Constraints — do not diff against Phase 1/2's recorded lists,
  which are known unstable under leaked-process load on this machine) and
  confirm no *new* failure site beyond the pre-existing ones.
- Run both mutations named in the ticket and confirm each fails at its own site:
  (a) revert `drain_all` to its Phase-2-landed (broken) body — the new
  integration test's post-kill 204/file-still-exists assertions must fail, while
  its pre-kill 204 control stays green; (b) delete the `attention.forget` call
  from the fixed `drain_all` — `drain_all_forgets_the_attention_entry` must fail
  at its own site while the token test stays green.
- Confirm `sessions_write_lock_sites_are_enumerated` still asserts count == 5 and
  stays green (this phase must not move that count).
- No frontend/browser verification needed — this phase is Rust-only (backend fix
  + backend tests + one spec sentence).

## Escalations
- None.
