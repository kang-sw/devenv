# Plan: 260726-bug-dashboard-agent-profile-provenance-lost-on-restart — Phase 1: persist spawn-profile provenance daemon-side and restore it on boot-reconcile adopt

## Relevant Ticket Contract

- Settled design (not open for re-litigation): a new daemon-owned sidecar
  `<state_dir>/agent-profiles/<terminal_id>/profile.json`, `0600`, atomic
  temp-rename, content `{ "profileId": "claude" }`. Written at spawn for
  **any** resolved profile (hooked or hookless); read in `reconcile_entry`'s
  adopt arm alongside `recover_callback_token`, replacing the literal `None`.
- Kept as a separate file from `callback.json` — never merge them, never mint
  a token for a hookless profile.
- **Load-bearing ordering change**: `mark_profile_pending` must run before the
  first byte of `agent-profiles/<terminal_id>/` is written for ANY resolved
  profile, not only inside the `hook_config.is_some()` branch (today's bug:
  `terminal.rs:1479`). The two failure-path `clear_profile_pending` calls
  (`terminal.rs:1564-1566`, `:1571-1578`) already run unconditionally and need
  no change.
- Degrade rules (must match, not invent): no `state_dir` → no sidecar, log a
  warning (same shape as `terminal.rs:1529-1534`); a sidecar write failure →
  log an error, spawn continues; missing/malformed sidecar on read → `None`,
  mirroring `agent_token_store::read_token`'s tolerant read. Every degrade's
  observable result is today's behavior, never worse.
- A create request naming no profile writes nothing new (byte-for-byte
  unchanged spawn path).
- Adopt echoes the recorded id verbatim, no re-validation against
  `agent_profile_registry::resolve`.
- No backfill for terminals spawned before this lands (accepted, self-clears).
- Verification boundary is daemon-level only:
  - Extend the two-real-daemon restart harness
    (`crates/daemon/tests/terminal_notify_callback_restart.rs`) at its
    existing adopted-entry lookup to assert `profileId: "claude"`.
  - Cover the hookless branch **spawn-side only** (directory creation on a
    path the pending mark did not previously cover) — do NOT drive
    `dummy-echo` through the two-process restart harness (30s process
    lifetime, disproportionate for a third test-only profile).
  - **Do NOT add** an adopt-side lib test for the hookless case (no scaffold
    exists to reach the real `AdoptLive` arm at lib level; the two
    `boot_reconcile_drops_entry_*` tests at `terminal.rs:2547`/`:2600`
    short-circuit before IPC and cannot be reused for this).
  - **Do NOT add** a new GC-ordering assertion for the hookless directory
    (the sweep is directory-name-based and already covered by
    `agent_profile_gc.rs:246`'s existing pending test; the existing restart
    harness's ordering assertion stays as-is).
- Comment sweep is in scope for this phase: every comment across
  `crates/daemon/src` and `frontend/src` asserting the loss is permanent must
  be corrected, named ones being `terminal.rs:357-382`, `terminal.rs:403-408`,
  and `App.tsx:4530-4533`.
- Spec: exactly one entry gets text changes —
  `260725-ws-web-dashboard-terminal-spawn-profile`
  (`ai-docs/spec/ws-web-dashboard/index.md:2131-2139`,
  `{#260725-ws-web-dashboard-terminal-spawn-profile}`). The other two named
  entries (`260725-nav-row-open-surface-counts-and-open-state`,
  `260726-dashboard-browser-level-attention-cue`) take no text change — the
  fix restores conformance to their existing text.
- Constraint: do not modify `TerminalRegistryEntry`
  (`terminal_registry_file.rs:16-27`) — this ticket routes around the
  helper-owned registry entirely.

## Out of Scope

- `TerminalRegistryEntry` schema / the helper-owned registry-versioning
  ticket (`260725-bug-dashboard-terminal-registry-schema-evolution-orphans-helpers`).
- Any browser/frontend behavior change — the frontend is a pure function of
  `profileId != null` and is already covered by existing unit tests
  (`agentAttention.test.ts`, `browserAttentionCue.test.ts`); no Playwright run
  is part of this phase.
- Backfill for pre-upgrade terminals.
- Any other sidecar content (turn state, title, cwd) — profile id only.
- Registering a new agent profile.
- GC sweep logic changes (`agent_profile_gc.rs` is unaffected — see Codebase
  Findings).

## Codebase Findings

- `crates/daemon/src/terminal.rs:1442-1536` — the current
  `if let (Some(hook_config), Some((_, args))) = (hook_config, command.as_mut())`
  branch is the ONLY place `agent-profiles/<terminal_id>/` gets created, and
  `registry.mark_profile_pending(&id)` (`:1479`) sits inside it. A hookless
  resolved profile (`dummy-echo`, `hook_config: None`) never enters this
  branch today, so a hookless spawn currently creates no directory at all —
  after this fix it must, and the pending mark must cover that new path too.
- `crates/daemon/src/terminal.rs:1564-1566` and `:1571-1578` — the two
  `clear_profile_pending` calls on `spawn`'s own failure paths (spawn error,
  handshake timeout) already run unconditionally regardless of which branch
  marked pending; confirmed no change needed here.
- `crates/daemon/src/terminal.rs:1015-1043` (`create_terminal` route handler)
  — `request.profile_id` is the same already-registry-validated string passed
  into `TerminalSession::spawn`'s `profile_id` parameter (`:1424`,
  `:1619`/`:1634`/`:1645`/`:1675`) and is `Some` for BOTH hooked and hookless
  resolved profiles (validated by `resolve_create_command`, `:1381-1404`,
  which errors out before this point for an unknown id). This means
  `profile_id.is_some()` — not `hook_config.is_some()` — is the correct gate
  for the new sidecar write and the hoisted `mark_profile_pending` call.
- `crates/daemon/src/terminal.rs:343-401` (`reconcile_entry`'s adopt arm) —
  `recover_callback_token(&entry.terminal_id)` is called at `:347`, then the
  literal `None` at `:383` is the exact site to replace with a new
  `self.recover_profile_id(&entry.terminal_id)` call.
- `crates/daemon/src/terminal.rs:403-438` (`recover_callback_token`) — the
  pattern to mirror for a new `recover_profile_id` method: `let state_dir =
  self.state_dir.as_deref()?;` then join `agent-profiles/<terminal_id>`, then
  delegate to the new store module's tolerant read. No re-validation against
  `agent_profile_registry::resolve` (matches the settled "echo verbatim"
  decision).
- **Atomic 0600 temp-rename write helper — reusable as-is, no widening
  needed.** `crate::agent_token_store::create_new_file_at_mode_0600`
  (`agent_token_store.rs:110-132`, `pub(crate)`) is already shared by
  `agent_token_store::write_token` and `agent_callback::write_callback_target`
  (`agent_callback.rs:157-182`) specifically because a prior review cycle
  rejected per-module duplication of this exact sequence
  (`agent_callback.rs:97-109`'s comment on the shared helper). A third
  reuse for `profile.json` is the same shape as those two, not a new
  mechanism — no signature change, no visibility change.
  `agent_callback::write_callback_target(profile_dir, ...)` is the closest
  structural template (create_dir_all the already-existing `profile_dir`,
  build the temp path via `.with_extension("json.tmp")`, serialize, write via
  the shared 0600 helper, rename).
- `crates/daemon/src/agent_token_store.rs:65-90` (`read_token`) — the
  tolerant-read pattern to mirror for `read_profile_id`: missing file →
  `None` silently; malformed JSON → `None` + `tracing::warn!`; never panics.
- `crates/daemon/src/agent_profile_gc.rs:60-110` — confirmed NO change
  needed. `sweep_agent_profiles_blocking` keys purely on directory name vs.
  `live_terminal_ids()` and `remove_dir_all`s the whole orphaned directory —
  adding `profile.json` inside it changes nothing about what gets swept or
  when.
- `crates/daemon/src/lib.rs:1-8` — module list is alphabetical
  (`agent_attention`, `agent_callback`, `agent_env_profile`,
  `agent_hook_config`, `agent_profile_gc`, `agent_profile_registry`,
  `agent_token_store`, ...); a new `agent_profile_store` module sits between
  `agent_profile_registry` and `agent_token_store`.
- **Comment sweep sites** (grep confirmed: `profile_id`/`profileId` across
  `crates/daemon/src` and `frontend/src`), each currently asserting or
  implying the loss is permanent:
  - `crates/daemon/src/terminal.rs:357-382` — the adopt-arm CONTRACT block
    (load-bearing, named in ticket).
  - `crates/daemon/src/terminal.rs:403-408` — `recover_callback_token`'s
    "unlike `profile_id` (permanently lost...)" contrast (load-bearing, named
    in ticket).
  - `crates/daemon/src/terminal.rs:777-781` — `TerminalSession.profile_id`
    field doc: "NOT persisted... does not survive a daemon restart" (found
    during this survey, not explicitly named in the ticket but the same
    defect class the `## Constraints` sweep targets).
  - `crates/daemon/src/terminal.rs:831-835` — `TerminalSessionView.profile_id`
    field doc: "`null` for the unchanged default-shell path and for any
    adopted (post-restart) session" (same class, found during this survey).
  - `frontend/src/App.tsx:4530-4533` — the "Known gap, deliberately not
    papered over here" note naming this ticket's stem (named in ticket).
  - `frontend/src/terminals.ts:19-23` — `TerminalSessionView.profileId`'s
    frontend-side doc, same permanence claim mirrored from the daemon type
    (found during this survey — the ticket's sweep instruction scopes to
    both `crates/daemon/src` AND `frontend/src`).
  - `frontend/src/terminals.ts:101-114` — `TerminalRestoreIntent.profileId`'s
    CONTRACT names this ticket's idea-stem directly and describes
    "`reconcile_entry`'s own profile-provenance loss" as a present-tense
    fact; lower-severity than the others (it is a comparison against a worse
    frontend failure mode, not a bare permanence assertion), but the phrase
    should be softened to reflect the loss is now residual (no-backfill /
    no-`state_dir` cases only), not general.
  - `frontend/src/terminalPaneBody.tsx:777-784` — checked, NOT a permanence
    claim (only documents the DOM attribute mirroring `profileId`, including
    its `null`/empty-string mapping); no edit needed.
  - `crates/daemon/src/terminal_registry_file.rs:1-27` — checked, NOT a
    permanence claim (only documents that `TerminalRegistryEntry` itself
    carries no profile field, which remains true and unmodified); no edit
    needed.
- **Existing test with a literal-string dependency on the current warning
  message — must not silently break.**
  `crates/daemon/tests/agent_hook_missing_state_dir.rs:213-218` asserts real
  daemon stdout contains the exact substring `"no persistent state directory
  resolved"` plus the terminal id, using a request with `profileId: "claude"`
  and no resolvable `state_dir` (env-scrubbed subprocess). This test already
  exercises the "profile resolved, no state_dir" degrade path end-to-end and
  must keep passing unchanged; if the `None`-arm warning text is reworded to
  also mention profile provenance, keep the substring `"no persistent state
  directory resolved"` intact (or update this test's assertion in the same
  change).
- `crates/daemon/tests/terminal_notify_callback_restart.rs:386-394` — the
  exact adopted-entry lookup the ticket names: `adopted` is
  `Option<&serde_json::Value>` found by `terminalId` match; the new assertion
  goes immediately after the existing `assert!(adopted.is_some(), ...)` at
  `:391-394`.
- `crates/daemon/tests/routes.rs:161-170` (`test_terminal_registry_with_state_dir`)
  and `:15876-15913` (`create_terminal_with_profile_for_test`) — the reusable
  lib-level harness for the new hookless spawn-side test: both already exist
  and are used today with `"claude"`; the new test can call
  `create_terminal_with_profile_for_test(app, cookie, work_root_id,
  "dummy-echo")` against a real `state_dir` from
  `test_terminal_registry_with_state_dir()`, then assert
  `<state_dir>/agent-profiles/<terminal_id>/profile.json` exists with
  `{"profileId":"dummy-echo"}`. `agent_profile_registry.rs:117`
  (`id: "dummy-echo"`) confirms this profile has `hook_config: None`
  (`agent_profile_registry.rs:197-199`'s `dummy_echo_profile_has_no_hook_config`
  test).
- `crates/daemon/src/agent_profile_gc.rs:245-282`
  (`sweep_agent_profiles_never_touches_a_directory_whose_terminal_is_pending_but_not_yet_live`)
  — the existing technique for proving the pending-before-directory ordering
  (there, synthetically via `mark_profile_pending_for_test`); the new
  hookless spawn-side test should prove the same ordering against the REAL
  production `spawn` path instead of the synthetic helper, e.g. by running
  the real create-terminal call and polling for `profile.json` to appear
  while confirming the id is not yet in `sessions` (only in
  `pending_profile_ids`/`live_terminal_ids()`) at that moment.
- `ai-docs/spec/ws-web-dashboard/index.md:2131-2139` — current text to amend:
  "The resolved profile — if any — is recorded read-only on the session for
  provenance, but that provenance does not survive a daemon restart: a
  session reattached during boot reconciliation is rebuilt from the on-disk
  terminal registry alone, which never carries a profile id." Replace per the
  ticket's `## Spec Impact` first bullet (reattached session now reports its
  spawn profile; registry still never carries one; caveat for pre-upgrade or
  unresolved-`state_dir` terminals).

## Implementation Plan

1. Add `crates/daemon/src/agent_profile_store.rs` (new module, mirrors
   `agent_token_store.rs`'s shape): a `ProfileFile { profile_id: Option<String>
   }` struct (`#[serde(default)]` on the field per the ticket's
   forward-compatibility guardrail), `profile_path(profile_dir: &Path) ->
   PathBuf` (`profile_dir.join("profile.json")`), `write_profile(profile_dir:
   &Path, profile_id: &str) -> io::Result<()>` (create_dir_all, temp-rename,
   `crate::agent_token_store::create_new_file_at_mode_0600` for the 0600
   write — same shape as `agent_callback::write_callback_target`), and
   `read_profile_id(profile_dir: &Path) -> Option<String>` (tolerant read:
   missing → `None`, malformed → `None` + `tracing::warn!`, mirroring
   `agent_token_store::read_token`). Register `pub mod agent_profile_store;`
   in `lib.rs` between `agent_profile_registry` and `agent_token_store`.

2. In `terminal.rs::TerminalSession::spawn` (~1442-1536), restructure so the
   sidecar write and the pending mark are gated on `profile_id.is_some()`
   (not `hook_config.is_some()`):
   - Outer: `if profile_id.is_some() { match state_dir { Some(state_dir) => {
     let profile_dir = state_dir.join("agent-profiles").join(&id);
     registry.mark_profile_pending(&id); /* hoisted here — runs before ANY
     sidecar byte, hooked or hookless */ if let Some(id_str) =
     profile_id.as_deref() { if let Err(error) =
     crate::agent_profile_store::write_profile(&profile_dir, id_str) {
     tracing::error!(...) } } /* existing hook_config branch nests here,
     unchanged except: drop its own now-redundant `mark_profile_pending` call
     and reuse `profile_dir` instead of recomputing it */ if let
     (Some(hook_config), Some((_, args))) = (hook_config, command.as_mut()) {
     ...token/callback/materialize_hook_config, unchanged... } } None =>
     tracing::warn!(...) } }`.
   - Keep the existing `None`-arm warning's substring `"no persistent state
     directory resolved"` intact (see Codebase Findings — an existing test
     depends on it verbatim), extending its wording to also mention profile
     provenance if desired.
   - Do not touch the two `clear_profile_pending` call sites (`:1564-1566`,
     `:1571-1578`) — already correct and unconditional.

3. In `TerminalRegistry` (near `recover_callback_token`, `:403-438`), add
   `fn recover_profile_id(&self, terminal_id: &str) -> Option<String>`:
   `let state_dir = self.state_dir.as_deref()?; let profile_dir =
   state_dir.join("agent-profiles").join(terminal_id);
   crate::agent_profile_store::read_profile_id(&profile_dir)`. No
   re-validation against `agent_profile_registry::resolve`.

4. In `reconcile_entry`'s adopt arm (`:343-386`), add
   `let profile_id = self.recover_profile_id(&entry.terminal_id);` alongside
   the existing `recover_callback_token` call, and pass `profile_id` instead
   of the literal `None` at `:383` into `TerminalSession::from_connection`.

5. Comment sweep (text-only, no behavior change) at every site listed in
   Codebase Findings: `terminal.rs:357-382`, `terminal.rs:403-408`,
   `terminal.rs:777-781`, `terminal.rs:831-835`, `App.tsx:4530-4533`,
   `terminals.ts:19-23`, and reword `terminals.ts:101-114` to describe the
   loss as residual (no-backfill / no-`state_dir` only) rather than general.
   Re-grep `profile_id`/`profileId` across `crates/daemon/src` and
   `frontend/src` afterward to confirm no remaining hit describes the bug as
   live/current behavior.

6. Spec amendment: `ai-docs/spec/ws-web-dashboard/index.md:2131-2139` — edit
   the sentence identified in Codebase Findings per the ticket's `## Spec
   Impact` first bullet. Leave the other two named spec entries untouched
   (no text change needed).

7. Tests:
   - `crates/daemon/tests/terminal_notify_callback_restart.rs` — immediately
     after the existing `assert!(adopted.is_some(), ...)` at `:391-394`, add
     `assert_eq!(adopted.unwrap()["profileId"], "claude", "the adopted
     terminal must report its spawn profile after a restart");`.
   - `crates/daemon/tests/routes.rs` — new lib-level test using
     `test_terminal_registry_with_state_dir()` and
     `create_terminal_with_profile_for_test(..., "dummy-echo")`: assert
     `<state_dir>/agent-profiles/<terminal_id>/profile.json` exists with
     `{"profileId":"dummy-echo"}`, and that `mark_profile_pending` ran before
     that first byte (mirror the ordering-proof technique from
     `agent_profile_gc.rs:245-282`, adapted to the real spawn path — poll for
     the directory/file to appear while confirming the id is not yet present
     in `sessions`).
   - Do NOT add an adopt-side `boot_reconcile` lib test for the hookless case
     and do NOT add a new GC-ordering assertion for the hookless directory —
     both explicitly ruled out by the ticket's Verification boundary.
   - Confirm (inspection or a light assertion) that
     `crates/daemon/tests/agent_hook_missing_state_dir.rs` still passes
     unchanged — it already covers "profile resolved, no `state_dir`"
     end-to-end.

## Verification Plan

- `cargo test -p ws-dashboard-daemon --test terminal_notify_callback_restart`
  — the new `profileId: "claude"` assertion must go from failing (today's
  tree) to passing.
- `cargo test -p ws-dashboard-daemon --test routes` — new hookless
  spawn-side test passes; existing tests in this file (including
  `create_terminal_with_profile_for_test`-based ones) stay green.
- `cargo test -p ws-dashboard-daemon --test agent_hook_missing_state_dir` —
  must still pass unchanged (degrade-path regression guard).
- `cargo test -p ws-dashboard-daemon-lib agent_profile_gc` — existing GC
  ordering/pending tests stay green with no modification required.
- `cargo test -p ws-dashboard-daemon-lib agent_profile_store` — new module's
  own round-trip/tolerant-read/mode-0600 unit tests (mirror
  `agent_token_store.rs`'s existing test shapes).
- Repo-wide grep: `grep -rn "profile_id\|profileId" crates/daemon/src
  frontend/src` reviewed by hand to confirm no remaining comment describes
  the bug as live/current behavior (the ticket's own closing verification
  bullet).
- No browser/Playwright run — explicitly out of scope per the ticket's
  settled "Verification stays at daemon level" decision.

## Escalations

- None.
