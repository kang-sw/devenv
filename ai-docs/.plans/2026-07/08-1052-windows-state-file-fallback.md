# Plan: 260707-bug-dashboard-windows-daemon-state-persistence-silently-noop — Phase 1: Add a Windows-native fallback to default_state_file

## Relevant Ticket Contract
- Extend `default_state_file()` (`persistent_state.rs:478-491`) so a native
  Windows build resolves a real per-user state directory when none of
  `WS_DASHBOARD_STATE_FILE`/`WS_DASHBOARD_STATE_HOME`/`XDG_STATE_HOME`/`HOME`
  is set — default to `%LOCALAPPDATA%\ws-dashboard\opened-workroots.json` via
  `std::env::var_os("LOCALAPPDATA")`, no new dependency. Only reach for a
  platform-directories crate if the workspace already depends on one
  elsewhere for an equivalent purpose (it does not — confirmed, see Codebase
  Findings); otherwise the plain env-var approach is accepted and needs no
  further judgment.
- Preserve the existing fallback order and env var names for Linux/macOS
  byte-for-byte; only add the Windows-specific branch, guarded by
  `cfg(windows)` or as a final fallback after `HOME` fails.
- Reproduce locally first (isolated root-picker-pin check: native Windows
  daemon run, `HOME` unset, `POST /api/dashboard/root-picker/pins`, confirm
  no state file appears) before fixing, then re-verify the fix produces a
  persisted state file.
- Re-run the sibling ticket's reversed-topology forwarded-operation walk end
  to end **only if** a real native-Windows host is reachable in the
  implementing session; otherwise the isolated repro is sufficient — do not
  block on cross-machine access. Record which verification level was
  achieved in the Result.
- Consider (optional, not required) a one-time `tracing::warn!` in
  `default_local()` when `state_file` resolves to `None`, since
  `DashboardStateStore` is `Clone + Default` with no internal shared/mutable
  state (no per-call dedup state should be added).
- Add a new spec entry under the Daemon Foundation section
  (`ai-docs/spec/ws-web-dashboard/index.md`, anchor
  `#260515-ws-web-daemon-foundation`) documenting the full state-file
  resolution order including the Windows-native fallback, using a fresh stem
  from `ws/spec_stem.generate`. This is part of Phase 1 completion, not a
  follow-up.

## Out of Scope
- Phase 1 is the only phase in this ticket; no future phases to defer to.
- The already-settled `--bind-mode public` Host-check non-bug
  (`ai-docs/tickets/.dropped/260707-bug-dashboard-public-bind-host-check-rejects-own-address.md`).
- The sibling ticket `260707-chore-dashboard-linked-server-tunnel-dogfood-plan`
  itself — only its reversed-topology verification walk is referenced, and
  only conditionally (real Windows host reachable).
- Changing `DashboardStateStore`'s `Clone + Default`, no-shared-state
  contract — the optional warning must not add per-call dedup state.

## Codebase Findings
- `ws-dashboard/crates/daemon/src/persistent_state.rs#L478-491` — the target
  function. Current order: `WS_DASHBOARD_STATE_FILE` → `WS_DASHBOARD_STATE_HOME`
  (join `opened-workroots.json`) → `XDG_STATE_HOME` (join
  `ws-dashboard/opened-workroots.json`) → `HOME` (join
  `.local/state/ws-dashboard/opened-workroots.json`) → `None`.
- `ws-dashboard/crates/daemon/src/persistent_state.rs#L30-35` — `default_local()`
  is the only caller of `default_state_file()`; it maps `Some(path)` to
  `Self::at_path(path)` and `None` to `Self::disabled()`. This is where an
  optional one-time warn would go if pursued.
- `ws-dashboard/crates/daemon/src/persistent_state.rs#L46-124` — every
  persistence method (`load_opened_work_roots`, `load_work_root_registry`,
  `persist_opened_work_roots`, `load_root_picker_pins`,
  `persist_root_picker_pins`, `load_linked_servers`, `persist_linked_servers`)
  guards on `self.state_file.as_deref()` and silently no-ops/returns empty —
  this is the confirmed silent-noop mechanism the ticket describes; Phase 1
  does not need to touch these, only ensure `state_file` resolves on Windows.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L2193-2201` — existing
  precedent `home_dir()` helper: `#[cfg(windows)] let key = "USERPROFILE";
  #[cfg(not(windows))] let key = "HOME";` then a single
  `std::env::var_os(key).filter(|v| !v.is_empty()).map(PathBuf::from)`. Shows
  the established `cfg(windows)` idiom in this crate — reuse this style
  rather than inventing a new one. Note it also filters empty-string env vars,
  which `default_state_file()` currently does not do for any of its four
  vars (pre-existing behavior, not in scope to change per "byte-for-byte
  identical" constraint, but worth mirroring only for the new Windows
  branch if consistency is desired — optional, not required by the ticket).
- `ws-dashboard/crates/daemon/src/root_picker.rs#L514-527` — a second,
  different `home_directory()` helper (HOME → USERPROFILE → HOMEDRIVE+HOMEPATH
  chain). Confirms this crate has more than one home/profile resolution
  helper already; this is a **different** concern (home directory vs. state
  dir) and the ticket explicitly specifies `LOCALAPPDATA`, not a HOME/
  USERPROFILE fallback, so this helper is not reusable for Phase 1 — noted
  only to avoid confusing it with the target function.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L2878-2899` — existing
  env-var-mocking test pattern: `resolve_cache_root_prefers_explicit_override_then_env`
  saves `std::env::var_os("WS_CACHE_HOME")`, calls `std::env::set_var`,
  asserts, then restores the previous value (or `remove_var` if it was unset)
  in the same test, with a comment noting "no other test reads this variable
  ... stays self-contained." This is the established save/mutate/restore
  idiom to follow for any new `default_state_file()` unit test that touches
  `LOCALAPPDATA`/`HOME`/etc.
- No workspace crate depends on `dirs` or `directories`
  (`grep` over root and crate `Cargo.toml` files found no match) — confirms
  the ticket's plain-`LOCALAPPDATA`-env-var instruction applies with no
  reuse-judgment override needed.
- `ai-docs/spec/ws-web-dashboard/index.md#L12` — `## Daemon Foundation
  {#260515-ws-web-daemon-foundation}` is the target section for the new spec
  entry; existing prose there is OS-neutral (serving/auth/bind-mode
  behavior), so the new state-file-resolution paragraph will be a new
  standalone entry with its own fresh anchor, not an edit to existing prose.

## Implementation Plan
1. In `ws-dashboard/crates/daemon/src/persistent_state.rs`, extend
   `default_state_file()` (currently `L478-491`): after the existing `HOME`
   branch fails (returns `None`), add a `cfg(windows)`-guarded fallback that
   reads `std::env::var_os("LOCALAPPDATA")` and, if present, returns
   `Some(PathBuf::from(path).join("ws-dashboard/opened-workroots.json"))`
   (mirroring the join-style already used for `WS_DASHBOARD_STATE_HOME` and
   `XDG_STATE_HOME`). Follow the `cfg(windows)` idiom from
   `work_root_activity.rs#L2193-2201` for style consistency. Do not alter the
   existing four branches or their order — Linux/macOS behavior must stay
   byte-for-byte identical (non-Windows builds should not even compile the
   new branch, or it should be a strict no-op fallback since `HOME` is
   expected to be set there).
2. (Optional, per ticket's "consider") in `default_local()`
   (`persistent_state.rs#L30-35`), add a single `tracing::warn!` when
   `default_state_file()` returns `None`, logged once at construction time —
   no new struct fields or shared/mutable state, since `DashboardStateStore`
   must remain `Clone + Default`.
3. Add a unit test for the new Windows branch in the existing `#[cfg(test)]
   mod tests` block (`persistent_state.rs`, starts `L494`), following the
   save/set/restore env-var idiom from
   `work_root_activity.rs#L2878-2899`: save prior `LOCALAPPDATA`/`HOME`/
   `XDG_STATE_HOME`/`WS_DASHBOARD_STATE_HOME`/`WS_DASHBOARD_STATE_FILE`
   values, clear the higher-priority vars, set `LOCALAPPDATA`, assert
   `default_state_file()` resolves to
   `<LOCALAPPDATA>/ws-dashboard/opened-workroots.json`, then restore all
   saved values. Gate this test with `#[cfg(windows)]` if the fallback
   branch itself is `cfg(windows)`-gated (test should not run/assert
   Windows-only behavior on Linux CI).
4. Generate a fresh spec stem via `ws/spec_stem.generate` and add a new entry
   under `## Daemon Foundation {#260515-ws-web-daemon-foundation}` in
   `ai-docs/spec/ws-web-dashboard/index.md` (after the existing foundation
   prose, `L12` onward) documenting the full state-file resolution order:
   `WS_DASHBOARD_STATE_FILE` → `WS_DASHBOARD_STATE_HOME` → `XDG_STATE_HOME` →
   `HOME` (Linux/macOS) → `LOCALAPPDATA` (Windows fallback) → disabled
   (persistence silently no-ops). Tag the new paragraph with the generated
   anchor.
5. Reproduce and re-verify per the ticket's stated verification boundary
   (see Verification Plan) — record which level (isolated repro only vs.
   full cross-machine walk) was actually achieved in the ticket's `###
   Result` section as part of this phase's completion, per repo convention
   (`AGENTS.md` commit-message conventions / ticket phase-result rules).

## Verification Plan
- `cargo test -p ws-dashboard-daemon persistent_state::` (or the crate's
  actual test-invocation convention — confirm crate/package name from
  `ws-dashboard/crates/daemon/Cargo.toml` if `-p` name differs) to run the
  existing plus new unit tests.
- Manual/isolated repro required by the ticket: run the native Windows
  daemon with `HOME` unset and no other override vars set, `POST
  /api/dashboard/root-picker/pins`, confirm a state file now appears under
  `%LOCALAPPDATA%\ws-dashboard\opened-workroots.json` (previously: `200`
  with no file created). This is a manual check, not automatable in this
  survey/plan.
- Full cross-machine reversed-topology walk (resources, root-picker,
  work-roots/open, files read/write, Git status/branches, terminal
  create/close, terminal WebSocket relay) only if a real native-Windows host
  is reachable in the implementing session; otherwise the isolated repro
  above is the accepted verification boundary per the ticket text.
- Confirm non-Windows behavior is unaffected: existing
  `persistent_state.rs` tests (`state_store_persists_deduplicated_opened_work_roots`,
  etc.) must continue to pass unmodified.

## Escalations
- None.
