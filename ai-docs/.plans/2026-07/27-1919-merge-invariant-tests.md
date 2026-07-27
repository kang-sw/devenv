# Plan: 260727-chore-merge-ws-dashboard-dev-into-goal-branch — Phase 1: pin the surviving invariants as tests, before the merge

## Relevant Ticket Contract

- Scope is `ws-dashboard/crates/daemon/src/terminal.rs` only, plus a
  verification-only check of `settingsSections.test.ts`. All new Rust tests go
  in the existing `#[cfg(test)] mod terminal_portability_skeleton_tests` —
  this file has no `mod tests`.
- Build one shared scanning helper first; invariants 1–3 all consume it.
  Excise rule: walk lines; when a line is exactly `#[cfg(test)]` at column 0,
  skip forward past the next line that is exactly `}` at column 0; otherwise
  keep the line. Then drop lines whose trimmed form starts with `//`. A second
  `flattened()` variant removes ALL whitespace from the comment-stripped text,
  applied AFTER comment-stripping (not before — flattening first collapses
  backticked `` `self.tokens` `` inside CONTRACT comments against the real
  identifier and inflates the count).
- Invariant 1: exactly one production `.env_clear(` in the comment-stripped
  (unflattened — it's single-line) text. Mutation: add/remove a call site,
  observe count move off 1 in both directions; also verify comment-stripping
  itself by temporarily adding a commented `.env_clear()` line and confirming
  the count does not move.
- Invariant 2 is two tests: a behavioral pair
  (`remove_forgets_the_callback_token`,
  `remove_for_work_roots_forgets_the_callback_token`) each needing a
  pre-removal "token resolves" assertion so a vacuous pass is impossible, and
  a structural test (`sessions_write_lock_sites_are_enumerated`) counting
  `self.sessions.write()` in the flattened text, asserting **4** pre-merge
  (becomes 5 post-merge in Phase 2 — do not pre-empt that here). The test's
  comment must state it counts *methods that take the `sessions` write lock*,
  a different count from the existing CONTRACT comment's "FIFTH
  session-removal path" phrasing, and say so explicitly so a reader grepping
  "FIFTH" isn't left arbitrating between two numberings.
- Invariant 3: `tokens_map_access_is_confined_to_its_choke_points`, over the
  flattened text: `.tokens` == 3, `self.tokens.read()` == 1,
  `self.tokens.write()` == 2 (all measured, all on this branch today).
- Invariant 4: no code change. Confirm the four `notificationAvailability`
  assertions in `settingsSections.test.ts` are green pre-merge and record
  that fact; do not touch the frontend.
- Every test needs a named mutation, and the mutation must actually be
  executed (temporarily edit, run the single test, observe the failure site,
  revert) — not merely described.
- Verification boundary (from the ticket): `cargo test -p ws-dashboard-daemon
  --no-fail-fast` with the failure-site list unchanged from a baseline
  recorded BEFORE any edit (Phases 2/3 diff against this baseline),
  `npm run test:settings` green, and every mutation observed to fail at its
  own site.

## Out of Scope

- Phases 2–5 of the ticket (the merge itself, `drain_all`'s fix, `_index.md`
  inventory repair, spec routing) — this plan covers Phase 1 only.
- Any frontend source edit. Invariant 4 is read-only verification.
- Closing `insert`'s eviction-path token gap (explicitly deferred debt,
  unrelated to this phase).
- Anything under `crates/daemon/tests/routes.rs` or other integration test
  files — Phase 1's new tests are unit tests inside `terminal.rs` itself
  (Invariant 5/6 integration tests belong to Phase 3, not this phase).

## Codebase Findings

- `ws-dashboard/crates/daemon/src/terminal.rs` is 3566 lines, has no
  `mod tests` (`grep -c "mod tests"` == 0), and has exactly three
  `#[cfg(test)]` markers at column 0: lines 2302, 3512, 3548, whose matching
  closing `}` lines are 3461, 3539, 3551 respectively — confirmed by direct
  line inspection. Excising exactly these three spans and then dropping
  comment-only lines leaves 2374 lines pre-comment-strip and 1704 after
  (both counts reproduced independently in this survey).
- Reproduced measurements against the live file (all match the ticket's
  numbers exactly): comment-stripped `.env_clear(` == 1;
  flattened(comment-stripped) `.tokens` == 3, `self.tokens.read()` == 1,
  `self.tokens.write()` == 2, `self.sessions.write()` == 4,
  `self.sessions.read()` == 3.
- `terminal.rs:1447` — `helper_command.env_clear().envs(env)`, inside
  `build_helper_command`'s `HelperEnvPlan::ClearAndSet` arm (function starts
  `terminal.rs:1354`) — this is the sole production `.env_clear(` call site.
  All other `env_clear` hits in the file are comments (e.g. lines 1296, 1340,
  1431, 1439, 2892, 2901, 2909, 2921) or the `#[cfg(test)]`-gated positive
  control at line 2917, inside
  `helper_spawn_default_no_command_matches_existing_arg_shape` (`terminal.rs:2856`).
- `ai-docs/mental-model/ws-web-dashboard/terminal.md:64` is the load-bearing
  explanation for invariant 1's shape: `std::process::Command` has no public
  API distinguishing "no env method was ever called" from "`env_clear()`
  called with nothing re-added" — `get_envs()` is the identical empty
  iterator either way. `helper_env_plan` (`terminal.rs:1313`) sidesteps that
  by extracting the decision into an explicit `HelperEnvPlan` value and
  asserting the value directly; that value assertion (already covered by the
  existing `helper_spawn_default_no_command_matches_existing_arg_shape` test)
  is the PRIMARY guard for invariant 1, not the source-count scan. The scan
  covers the one residual the value cannot see: "an `env_clear()` written
  directly into `build_helper_command` outside the single plan-application
  site." State this explicitly in the new test's doc comment so a reader does
  not mistake the scan for the main guard.
- `terminal.rs:558-613` `insert`, `:620-634` `insert_unchecked`, `:641-648`
  `remember_token`, `:653-665` `forget_token`, `:667-680` `remove`,
  `:690-716` `remove_for_work_roots` — the four `self.sessions.write()` call
  sites are at lines 565 (`insert`), 623 (`insert_unchecked`), 669 (`remove`),
  694 (`remove_for_work_roots`); confirmed by direct read. `drain_all` does
  not exist pre-merge (`grep -n "fn drain_all"` in this file returns nothing
  today) — it only appears in the merge-tree preview, so the structural
  test's expected count is 4 in this phase, not 5.
- `terminal.rs:486-492` `token_for` — the sole `self.tokens.read()` site,
  under an explicit "ONLY reader of `self.tokens`" CONTRACT comment.
  `terminal.rs:641-648` `remember_token` and `:653-665` `forget_token` are
  the two `self.tokens.write()` sites.
- `terminal.rs:3203-3288` — the three existing
  `*_forgets_the_attention_entry` tests
  (`remove_forgets_the_attention_entry`,
  `remove_for_work_roots_forgets_the_attention_entry`,
  `insert_forgets_the_attention_entry_of_a_session_its_own_eviction_retain_drops`)
  and the shared `insert_fake_live_session_for_test` helper they all call
  (`terminal.rs:3513-3538`; also called from `agent_profile_gc.rs:319`).
  **Fixture trap 1**: `insert_fake_live_session_for_test` hard-codes
  `callback_token: None` (`terminal.rs:3524`), and `remember_token`
  (`terminal.rs:641`) no-ops when `session.callback_token` is `None` — so
  this exact helper cannot seed a token, and a token test built on it as-is
  would pass its post-removal assertion vacuously. Because the helper is
  shared with `agent_profile_gc.rs`, changing its signature is a
  cross-module edit; add a token-bearing SIBLING function instead (e.g.
  `insert_fake_live_session_with_token_for_test(registry, terminal_id,
  token)`, same body as `insert_fake_live_session_for_test` but with
  `callback_token: Some(token.to_owned())`), do not touch the existing one.
- **Fixture trap 2**: all three existing `*_forgets_the_attention_entry`
  tests construct via `TerminalRegistry::default()`
  (`terminal.rs:3204/3223/3250`), whose `Default` impl
  (`terminal.rs:218-235`) sets `state_dir` to
  `crate::persistent_state::default_state_dir()` — the developer's real
  state directory. `forget_token` (`terminal.rs:653-665`) calls
  `crate::agent_token_store::delete_token(state_dir, terminal_id)` whenever
  it removed an in-memory token entry, so a token test built on `Default`
  would create/delete a real file under the real `terminal-tokens/` on
  every `cargo test` run and pass silently. The one existing test in this
  module that constructs via `TerminalRegistry::new(...)` with an explicit
  temp `state_dir` is
  `spawn_marks_the_profile_pending_before_writing_the_first_sidecar_byte`
  (`terminal.rs:3353-3367`): it builds
  `std::env::temp_dir().join(format!("ws-dashboard-profile-pending-order-{unique}"))`,
  derives `state_dir`/`registry_dir` under it, and calls
  `TerminalRegistry::new(helper_binary, registry_dir, connect_timeout,
  Some(state_dir), base_url)`. The new callback-token tests must copy this
  exact shape (temp base dir, explicit `state_dir`, cleanup via
  `std::fs::remove_dir_all` at the end).
- `ws-dashboard/crates/daemon/src/agent_token_store.rs:32-38` —
  `token_store_dir(state_dir)` = `state_dir.join("terminal-tokens")`;
  `token_store_path(state_dir, terminal_id)` = that dir joined with
  `"{terminal_id}.json"`; both `pub fn`, reachable as
  `crate::agent_token_store::token_store_path(...)`. `write_token` (line
  ~44) and `delete_token` (used at `terminal.rs:662`) are the write/delete
  entry points. Because `insert`/`insert_unchecked`/`remember_token` never
  write the on-disk file themselves (only `TerminalSession::spawn`'s real
  hook-materialization path does, at `terminal.rs:1650`), the new tests must
  explicitly call `crate::agent_token_store::write_token(&state_dir,
  terminal_id, &token)` after inserting the fake session, to put a real file
  on disk for `forget_token`'s best-effort delete to act on and for the test
  to assert against.
- `terminal.rs:2459-2484` `fake_terminal_session()` — the sibling fixture
  pattern for constructing a raw `TerminalSession` (used by
  `insert_forgets_the_attention_entry_of_a_session_its_own_eviction_retain_drops`
  as the "unrelated live session" trigger). Not directly needed for the two
  new token tests, but useful if the structural test needs an extra
  live session as a non-interfering control.
- `ws-dashboard/frontend/src/settingsSections.test.ts` lines 140, 146, 152,
  158 — the four `notificationAvailability(...)` assertions the ticket
  requires stay pinned and verified green pre-merge.
- Self-referential source scan pattern: this codebase already uses
  `include_str!` with a path relative to the current file for embedding
  sibling text (`terminal.rs`'s neighbors `events.rs:28`,
  `mock.rs:16`, both do `include_str!("../tests/fixtures/....json")`). The
  shared scanning helper should read `terminal.rs`'s own source the same way
  — `include_str!("terminal.rs")` from within `terminal.rs` itself resolves
  to the file's own path and needs no `CARGO_MANIFEST_DIR`/`file!()`
  plumbing.

## Implementation Plan

1. In `terminal.rs`'s `mod terminal_portability_skeleton_tests`, add a small
   private helper section (near the top of the module, before the first
   test) providing:
   - `const SOURCE: &str = include_str!("terminal.rs");`
   - `fn production_text() -> String` — apply the excise rule (skip from a
     line exactly `"#[cfg(test)]"` past the next line exactly `"}"`) over
     `SOURCE.lines()`, then drop lines whose `.trim()` starts with `"//"`,
     rejoin with `"\n"`.
   - `fn flattened(text: &str) -> String` — remove all whitespace
     characters from `text` (e.g. `text.chars().filter(|c|
     !c.is_whitespace()).collect()`), called only on the output of
     `production_text()` (comment-stripped first, flattened second — never
     the reverse).
   Doc-comment the module explaining these are source scans, not behavioral
   proofs, and why (the `env_clear`/`self.tokens` observability gap from
   `terminal.md:64`).
2. Add `terminal_rs_has_exactly_one_production_env_clear`: assert
   `production_text().matches(".env_clear(").count() == 1`. Doc-comment: the
   primary guard is `helper_env_plan`'s existing value-assertion test
   (`helper_spawn_default_no_command_matches_existing_arg_shape`); this scan
   only covers a second call site written directly into
   `build_helper_command` outside the plan-application arm.
3. Add the token-bearing fixture sibling
   `insert_fake_live_session_with_token_for_test(registry: &TerminalRegistry,
   terminal_id: &str, token: &str)` next to `insert_fake_live_session_for_test`
   (`terminal.rs:3513`), identical body but with `callback_token:
   Some(token.to_owned())`. Do not modify the existing helper or its call
   sites (`terminal.rs:3205/3225/3251`, `agent_profile_gc.rs:319`).
4. Add `remove_forgets_the_callback_token` and
   `remove_for_work_roots_forgets_the_callback_token`, each:
   - Build a temp base dir under `std::env::temp_dir()` (unique per
     process+timestamp, mirroring
     `spawn_marks_the_profile_pending_before_writing_the_first_sidecar_byte`'s
     `base`/`state_dir` construction), and construct the registry via
     `TerminalRegistry::new(default_helper_binary(), registry_dir,
     DEFAULT_CONNECT_TIMEOUT, Some(state_dir.clone()), String::new())` —
     never `TerminalRegistry::default()`.
   - Insert a fake session via the new token-bearing sibling helper with a
     literal token string.
   - Call `crate::agent_token_store::write_token(&state_dir, terminal_id,
     token)` to place the on-disk file (the fake-insert path never writes
     it).
   - Pre-removal assertion (non-vacuity guard): `registry.token_for(terminal_id)
     == Some(token.to_owned())` and
     `agent_token_store::token_store_path(&state_dir, terminal_id).exists()`.
   - Call `registry.remove(terminal_id)` (first test) /
     `registry.remove_for_work_roots(&BTreeSet::from([work_root_id]))`
     (second test, using the same `work_root_id` the fake session was
     inserted under).
   - Post-removal assertion: `registry.token_for(terminal_id).is_none()` and
     the token file no longer exists.
   - Clean up the temp base dir with `std::fs::remove_dir_all` at the end.
5. Add `sessions_write_lock_sites_are_enumerated`: assert
   `flattened(&production_text()).matches("self.sessions.write()").count()
   == 4`. Doc-comment enumerating all four sites and each one's discharge
   status today: `insert_unchecked` (adds only, owes nothing), `insert`'s
   eviction retain (discharges attention only, per its own CONTRACT-recorded
   deferral), `remove` and `remove_for_work_roots` (discharge both). State
   in the comment that this test counts *methods that take the `sessions`
   write lock*, distinct from the existing CONTRACT comment's "FIFTH
   session-removal path" numbering, and that Phase 2 will move this count to
   5 when `drain_all` lands (with its own "discharges neither" line) and
   Phase 3 will rewrite that line to "discharges both". Make the assertion
   failure message name what changed and what to do (add/update the
   enumerating comment and the expected count).
6. Add `tokens_map_access_is_confined_to_its_choke_points`: three assertions
   against `flattened(&production_text())`: `.tokens` occurs 3 times,
   `self.tokens.read()` occurs 1 time, `self.tokens.write()` occurs 2 times.
   Doc-comment naming the three choke points (`token_for` for the read;
   `remember_token`, `forget_token` for the writes) and that no behavioral
   check exists because an extra reader is runtime-indistinguishable from
   `token_for`.
7. Run every mutation named below, confirm each fails at the stated site,
   then revert:
   - Invariant 1: temporarily add a second `.env_clear()` call anywhere in
     production code in `terminal.rs` — the new scan test must fail with
     count 2. Then instead delete the sole call site — must fail with count
     0. Then instead add a *commented* `.env_clear()` line somewhere — the
     scan test must NOT fail (proves comment-stripping works). Revert all
     three after observing.
   - Invariant 2 (behavioral): temporarily delete the
     `self.forget_token(terminal_id)` call from `remove` — confirm
     `remove_forgets_the_callback_token` fails at its post-removal
     assertion while `remove_forgets_the_attention_entry` stays green.
     Revert. (Do the same for `remove_for_work_roots`'s
     `self.forget_token(&session.id)` call against the sibling test, or note
     if time-boxed to running only one of the two symmetric mutations —
     state which in the Result.)
   - Invariant 2 (structural): temporarily add any new method to
     `TerminalRegistry` that takes `self.sessions.write()` — confirm
     `sessions_write_lock_sites_are_enumerated` fails, count 5. Revert.
   - Invariant 3: temporarily add a second `self.tokens.read()` call
     anywhere else in the `impl TerminalRegistry` block — confirm the test
     fails on both the total (4) and the read-count (2) assertions. Revert.
   - Invariant 4: this phase adds no code, so no fresh mutation run is
     required; the ticket text notes the mutation (swap the secure-context
     check back to global-first in `settingsSections.tsx`) was already run
     in `260726-chore-dashboard-verify-notification-permission-tier-manually`
     Phase 2. Confirm `settingsSections.tsx` is unchanged since that phase
     (e.g. `git log -1 --format=%H -- ws-dashboard/frontend/src/settingsSections.tsx`
     against that ticket's recorded commit); only re-run the mutation if it
     changed.
8. Record the pre-change Rust failure-site baseline BEFORE step 1 (see
   Verification Plan) so Phase 1's own new-test additions can be judged
   against it, and so the Result section can hand Phase 2/3 a clean
   baseline.

## Verification Plan

- Before making any edit, capture the baseline. Run, as one bash invocation:
  `cd ws-dashboard && cargo test -p ws-dashboard-daemon --no-fail-fast >
  <scratchpad>/baseline.log 2>&1` on one line, then `echo $?` as the very
  next line of the SAME invocation (never `| tee`, never `| tail`, never
  `cmd; echo $?`) — use the scratchpad directory for the log file, not
  `/tmp`. Extract every failing test's file:line from the output; this is
  the baseline failure-site list (expected to include
  `crates/daemon/tests/routes.rs:1066` and `:1383` per the ticket's stated
  pre-existing conditions — confirm this by reading the output, don't
  assume it).
- After all six new tests are added: re-run
  `cargo test -p ws-dashboard-daemon --no-fail-fast` the same way. The
  six new tests must be green, and the failure-site list for everything
  else must be identical to the baseline (same file:line set — judge by
  site, not by exit code; exit 101 is expected both before and after
  because of the pre-existing `routes.rs` failures).
- `npm run test:settings` (in `ws-dashboard/frontend`) must be green,
  confirming the four `notificationAvailability` assertions in
  `settingsSections.test.ts` pass pre-merge.
- Every mutation listed in Implementation Plan step 7 must actually be
  executed against a single target test (e.g. `cargo test -p
  ws-dashboard-daemon <test_name> --no-fail-fast`), its failure site
  observed and recorded, then reverted before moving to the next mutation.
  A mutation that was only described and not run does not satisfy the
  ticket's verification boundary.
- Record the final baseline failure-site list and the mutation-run log in
  this phase's `### Result` — Phase 2 and Phase 3 compare their own
  post-change failure-site lists against this recorded baseline, not
  against a fresh assumption.

## Escalations

- None.
