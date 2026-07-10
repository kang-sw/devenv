# Plan: 260710-bug-dashboard-windows-verbatim-path-not-normalized — Phase 1: Shared path-normalization helper and call-site fixes

## Relevant Ticket Contract
- Extract a shared `normalize_display_path(&Path) -> String`-style helper,
  reusing/generalizing the verbatim-prefix-stripping logic already in
  `canonical_path_bytes` (`work_root_activity.rs`) rather than duplicating it;
  keep it a no-op on non-Windows.
- Apply it in `root_picker_view`/`entry_for_directory` and sibling
  `push_place`/`push_pin_place` helpers so the browser never receives a
  `\\?\`-prefixed path.
- Apply it defensively in `open_work_root` before persisting
  `RegisteredWorkRoot.path`.
- Apply it in `resolve_online_available_work_root` as a self-healing pass so
  paths persisted by a pre-fix daemon build recover without manual migration.
- Confirm `terminal.rs`'s `resolve_terminal_cwd`/`TerminalSession::spawn`
  receive an already-normalized path through the above, or fix independently
  if some other route still reaches `command.cwd()` unnormalized.
- Constraint: fix on the daemon side only; browser must never see a
  `\\?\`-prefixed path. Non-Windows must be unaffected (no-op).
- Add unit test coverage for the helper (verbatim-prefixed input -> plain;
  already-plain input -> unchanged; non-Windows no-op) alongside the existing
  `canonical_path_bytes` tests.
- Live-verify on the native Windows dogfood daemon is a manual step outside
  this survey's reach; note it as a follow-up rather than an automated check.

## Out of Scope
- Any other phase of this ticket beyond Phase 1.
- The `260710-epic-ws-dashboard-terminal-ux-polishing` parent epic's other
  tickets.
- Reworking `wsstate` short-hash key derivation itself (only the shared
  string-normalization logic is being factored out of it).

## Codebase Findings
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L2325-2343` —
  `canonical_path_bytes(path: &Path) -> Vec<u8>`: on `#[cfg(windows)]` strips
  `\\?\UNC\` -> `\\` and `\\?\` -> nothing via `to_string_lossy()` +
  `strip_prefix`; on non-Windows returns raw `OsStr` bytes unchanged. This is
  the logic to generalize; existing tests at `#L2580-2594` cover both the
  `\\?\C:\repo` and `\\?\UNC\server\share` cases and the Unix no-op case.
- `ws-dashboard/crates/daemon/src/root_picker.rs#L359-388` —
  `root_picker_view`: calls `path.canonicalize()` (L361), builds
  `current_path` (L383) and `parent_path` (L384) via `path.display().to_string()`.
- `ws-dashboard/crates/daemon/src/root_picker.rs#L390-409` —
  `entry_for_directory`: builds `entries[].path` via `path.display().to_string()`
  (L398).
- `ws-dashboard/crates/daemon/src/root_picker.rs#L467-493` — `push_place`:
  canonicalizes (L475) and sets `display_path` via `.display().to_string()`
  (L481), used both as the `seen` dedup key and the place's `path` field.
- `ws-dashboard/crates/daemon/src/root_picker.rs#L495-512` — `push_pin_place`:
  same pattern (L497-498), also builds `seen_key`/`id`/`label` from
  `display_path`.
- `ws-dashboard/crates/daemon/src/root_picker.rs#L208-245` — `open_work_root`:
  `requested_path = PathBuf::from(request.path)` (L212, client-supplied, may
  already carry a `\\?\` prefix round-tripped from an unfixed picker view) is
  stored verbatim into `RegisteredWorkRoot { path: requested_path, .. }`
  (L241) before persisting.
- `ws-dashboard/crates/daemon/src/work_root_files.rs#L31-35` —
  `RegisteredWorkRoot { pub path: PathBuf, .. }`.
- `ws-dashboard/crates/daemon/src/work_root_files.rs#L776-790` —
  `resolve_online_available_work_root` returns `root.path` (a `PathBuf`)
  unchanged (L789). This is the single funnel: every terminal and file
  operation (both in `terminal.rs` and `work_root_files.rs`) resolves the
  work-root path through this one function — confirmed via grep, all
  `TerminalSession::spawn` call sites (`terminal.rs#L344-357`) and every
  file-read/write path in `work_root_files.rs` go through it. Normalizing
  here is sufficient to satisfy Phase 1 item 5 (`resolve_terminal_cwd`/
  `TerminalSession::spawn` need no independent fix once this function
  normalizes).
- `ws-dashboard/crates/daemon/src/terminal.rs#L889-924` —
  `resolve_terminal_cwd(root_path: &Path, cwd_hint) -> (PathBuf, Option<String>)`:
  joins `root_path` (from `resolve_online_available_work_root`) with a
  sanitized hint; no separate canonicalize/verbatim-prefix path here, so no
  independent fix is needed once the funnel above is normalized.
- `ws-dashboard/crates/daemon/src/lib.rs#L1-20` — module list; no dedicated
  path-utility module exists. `work_root_activity` and `work_root_files`
  already have a mutual `use crate::...` relationship (`work_root_activity.rs#L26`
  imports `resolve_online_available_work_root` from `work_root_files`), so a
  reverse import (`work_root_files.rs` importing a helper from
  `work_root_activity.rs`) is not a new-module-boundary risk — Rust allows
  this within one crate.
- `ws-dashboard/Cargo.toml#L1-30` (workspace) — no `dunce` dependency exists;
  `sha2` is already a workspace dependency yet `work_root_activity.rs` hand-rolls
  SHA-256 locally, suggesting this codebase's convention favors small
  hand-rolled logic over adding a crate for a narrow need. Reusing/generalizing
  the existing strip-prefix logic (no new dependency) fits this convention
  better than introducing `dunce`.
- Risk signal: `RegisteredWorkRoot.path` and the path returned by
  `resolve_online_available_work_root` are also used for real filesystem I/O
  (`work_root_files.rs`, `is_dir()`/`read_dir()` checks). Stripping the
  `\\?\` verbatim prefix from the *stored/returned* `PathBuf` (not just a
  display string) means long paths (>260 chars without Windows long-path
  opt-in) could theoretically stop working through the verbatim form. This is
  the ticket's explicit, settled instruction (Phase 1 items 3-4 target
  `RegisteredWorkRoot.path` and `resolve_online_available_work_root`'s return
  value directly, not just a separate display string), so this is not an
  open contract question — flagging only so the executor doesn't second-guess
  the instruction mid-implementation.

## Implementation Plan
1. In `ws-dashboard/crates/daemon/src/work_root_activity.rs`, add a
   `pub(crate) fn normalize_display_path(path: &Path) -> String` near
   `canonical_path_bytes` (`#L2325`). On `#[cfg(windows)]`, reuse the exact
   `strip_prefix(r"\\?\UNC\")` / `strip_prefix(r"\\?\")` logic currently
   inlined in `canonical_path_bytes` (operating on `path.to_string_lossy()`);
   on `#[cfg(not(windows))]`, return `path.display().to_string()` unchanged
   (no-op, matches existing call-site pattern elsewhere in the codebase).
2. Refactor `canonical_path_bytes` (`#L2325-2343`) to call
   `normalize_display_path` for the `#[cfg(windows)]` branch
   (`normalize_display_path(path).into_bytes()`) instead of duplicating the
   strip logic; leave the non-Windows raw-`OsStr`-bytes branch untouched
   (behavior must stay byte-identical there — no lossy UTF-8 substitution for
   the hash input).
3. In `root_picker.rs`, import `normalize_display_path` from
   `crate::work_root_activity`. Apply it:
   - `root_picker_view` (`#L383-384`): wrap `current_path`/`parent_path`
     construction, e.g. `normalize_display_path(&path)` /
     `path.parent().map(|p| normalize_display_path(p))`.
   - `entry_for_directory` (`#L398`): `path: normalize_display_path(path)`.
   - `push_place` (`#L481`): compute `display_path` via
     `normalize_display_path(&canonical)` instead of
     `canonical.display().to_string()`.
   - `push_pin_place` (`#L497-498`): same substitution for both the
     canonicalize-success and fallback (non-canonicalizable pin) branches —
     the fallback already uses `path.display().to_string()`, which should
     also route through `normalize_display_path` for consistency (harmless
     no-op if the raw pin path never had a verbatim prefix).
4. In `root_picker.rs`'s `open_work_root` (`#L212`), after building
   `requested_path = PathBuf::from(request.path)`, defensively normalize it
   before constructing `RegisteredWorkRoot`:
   `let requested_path = PathBuf::from(normalize_display_path(&requested_path));`.
   Apply this immediately after line 212, before it is cloned into the
   `LocalWorkRootCandidate` at L213-215 and stored at L241, so both the
   discovery probe and the persisted value see the normalized path.
5. In `work_root_files.rs`'s `resolve_online_available_work_root`
   (`#L776-790`), import `normalize_display_path` from
   `crate::work_root_activity` and normalize the return value as a
   self-healing pass: replace `Ok(root.path)` (L789) with
   `Ok(PathBuf::from(normalize_display_path(&root.path)))`. This is
   sufficient for `terminal.rs`'s `TerminalSession::spawn`/
   `resolve_terminal_cwd` (no independent change needed there — verified
   single funnel, see Codebase Findings) and for every file-op call site in
   `work_root_files.rs` that also goes through this function.
6. Add unit tests in `work_root_activity.rs`'s existing `#[cfg(test)] mod
   tests` block, alongside `canonical_path_bytes_strips_windows_verbatim_prefix`
   (`#L2580-2594`):
   - `#[cfg(windows)]`: `normalize_display_path(Path::new(r"\\?\C:\repo"))`
     -> `"C:\\repo"`; `normalize_display_path(Path::new(r"\\?\UNC\server\share"))`
     -> `r"\\server\share"`; and an already-plain input (`r"C:\repo"`) ->
     unchanged.
   - `#[cfg(not(windows))]`: `normalize_display_path(Path::new("/tmp/ws-root"))`
     -> `"/tmp/ws-root"` (no-op).

## Verification Plan
- `cargo test -p ws-dashboard-daemon work_root_activity::` (or the workspace's
  usual per-crate test invocation) to run the new normalization tests plus
  existing `canonical_path_bytes`/`sha256` tests untouched.
- `cargo build -p ws-dashboard-daemon` (or workspace build) to confirm the
  cross-module import (`work_root_files.rs` / `root_picker.rs` importing from
  `work_root_activity.rs`) compiles without a cycle issue.
- Manual/live verification (ticket Phase 1 item 7) is out of this survey's
  reach: re-open the previously-corrupted workRoot on the native Windows
  dogfood daemon and confirm both the displayed path and the spawned
  terminal's cwd are plain (no `\\?\` prefix), without removing/re-adding the
  workRoot. Flag this as a required manual step for whoever executes/reviews
  this phase; it cannot be scripted from this environment.

## Escalations
- None.
