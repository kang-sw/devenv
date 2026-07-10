---
title: "ws dashboard exposes unnormalized Windows verbatim (\\\\?\\) paths to the browser and PTY cwd"
sage-review: required
parent: 260710-epic-ws-dashboard-terminal-ux-polishing
related-mental-model:
  - ws-web-dashboard
completed: 2026-07-10
---

# ws dashboard exposes unnormalized Windows verbatim (\\?\) paths to the browser and PTY cwd

## Background

Found live during Windows-native dogfood (2026-07-10, native `ws-dashboard.exe`
built from `D:\dbg-ws-dashboard-dev`, `--no-auth`, `127.0.0.1:4100`): opening a
directory through the root-picker on native Windows surfaced a path that,
once used, made a spawned PowerShell terminal show its prompt as
`Microsoft.PowerShell.Core\FileSystem::\\?\D:\Workspace\Repos\...` instead of
a plain `D:\Workspace\Repos\...` path.

Root cause chain, confirmed by direct source read (no reproduction script
needed beyond the live dogfood observation):

1. `crates/daemon/src/root_picker.rs:359-388` (`root_picker_view`) calls
   `path.canonicalize()` (line 361) and builds `current_path`/`parent_path`/
   every `entries[].path` straight from `path.display().to_string()`
   (lines 383-384, and `entry_for_directory` at 390-409). On Windows,
   `std::path::Path::canonicalize()` returns an extended-length **verbatim**
   path prefixed with `\\?\` — this raw prefixed string is sent to the
   browser unstripped.
2. `crates/daemon/src/root_picker.rs:208-245` (`open_work_root`) takes the
   client-supplied path (which round-tripped from step 1's unstripped
   `current_path`) and stores it verbatim into `RegisteredWorkRoot.path`
   (line 241), then persists it — the corruption becomes durable.
3. `crates/daemon/src/work_root_files.rs:776-790`
   (`resolve_online_available_work_root`) returns `root.path` unchanged
   (line 789).
4. `crates/daemon/src/terminal.rs:480-499` (`TerminalSession::spawn`) and
   `resolve_terminal_cwd` (889-924) pass that still-`\\?\`-prefixed path
   straight into `command.cwd(spawn_cwd)` for the PTY child process — this is
   why a shell started (or `cd`'d) into that directory shows the
   provider-qualified prompt.

A private strip-prefix helper already exists —
`canonical_path_bytes` in `crates/daemon/src/work_root_activity.rs:2325-2337`
(tests at 2580-2594) — but it is narrowly scoped to wsstate SHA-256
short-hash key derivation and is not called from any of the four sites
above. No `dunce` crate dependency exists in this workspace.

## Constraints

- Fix on the daemon side (path ingestion/normalization), not by asking the
  frontend to guess or strip prefixes — the browser should never see a
  `\\?\`-prefixed path in the first place.
- Any already-persisted `RegisteredWorkRoot.path` values captured before this
  fix should self-heal on next open/resolve rather than requiring a manual
  migration step or state-file edit.
- Non-Windows platforms must be unaffected (`canonicalize()` does not add a
  verbatim prefix on Linux/macOS; the normalization helper should be a no-op
  there).

## Phases

### Phase 1: Shared path-normalization helper and call-site fixes

1. Extract a shared `normalize_display_path(&Path) -> String` (or similar)
   helper — reuse/generalize the verbatim-prefix-stripping logic already in
   `canonical_path_bytes` (`work_root_activity.rs`) rather than duplicating
   it, or introduce the `dunce` crate if that proves cleaner. Keep it a
   no-op on non-Windows.
2. Apply it in `root_picker_view`/`entry_for_directory` (and any sibling
   `push_place`/`push_pin_place`-style helpers that turn a `canonicalize()`
   result into a display/API string) so the browser never receives a
   `\\?\`-prefixed path.
3. Apply it defensively in `open_work_root` before persisting
   `RegisteredWorkRoot.path`, so a client-supplied path is normalized before
   it becomes durable state.
4. Apply it in `resolve_online_available_work_root` (or wherever persisted
   `root.path` values are read back) as a self-healing normalization pass,
   so paths persisted by a pre-fix daemon build recover without a manual
   migration.
5. Confirm `terminal.rs`'s `resolve_terminal_cwd`/`TerminalSession::spawn`
   receive an already-normalized path through the above and need no
   independent fix, or add one if a path can still reach `command.cwd()`
   unnormalized through some other route.
6. Add unit test coverage for the normalization helper (verbatim-prefixed
   input -> plain path; already-plain input -> unchanged; non-Windows
   no-op) alongside the existing `canonical_path_bytes` tests.
7. Live-verify on the native Windows dogfood daemon (`D:\dbg-ws-dashboard-dev`,
   already running, `--no-auth`): re-open the previously-corrupted workRoot,
   confirm the displayed path and the spawned terminal's cwd are both plain
   (no `\\?\` prefix), without needing to remove/re-add the workRoot by hand.

### Result

Implemented items 1-6 in commit `20cabdb9` on
`implement/windows-verbatim-path-normalize`: added
`pub(crate) fn normalize_display_path` in `work_root_activity.rs` (reusing
the existing `canonical_path_bytes` strip-prefix logic, no new crate
dependency), refactored `canonical_path_bytes` to delegate to it on the
Windows branch, and applied it at every path-display/persist boundary —
`root_picker_view`, `entry_for_directory`, `push_place`, `push_pin_place`,
`open_work_root`'s `requested_path`, and
`resolve_online_available_work_root`'s returned path (self-healing pass).
Confirmed `resolve_online_available_work_root` is the single funnel for
`terminal.rs`'s `resolve_terminal_cwd`/`TerminalSession::spawn`, so item 5
needed no independent fix. Added unit tests
(`normalize_display_path_strips_windows_verbatim_prefix`,
`normalize_display_path_is_noop_on_non_windows`) alongside the existing
`canonical_path_bytes` tests. `cargo test -p ws-dashboard-daemon` (159
tests) and `cargo build`/`cargo clippy -p ws-dashboard-daemon` pass clean;
no new clippy warnings introduced. Item 7 (native Windows dogfood
live-verification) remains an outstanding manual step, as scoped by the
plan — could not be exercised from this Linux environment.

#### Edition (89283667) - 2026-07-10

Review-cycle-1 fix: the correctness partition flagged that
`OpenedWorkRoots::resolve()`/`get()`/`candidate_paths()`/
`candidate_roots()`/`owner_candidate_roots()` (`work_root_files.rs`)
returned `RegisteredWorkRoot.path` verbatim, unlike
`resolve_online_available_work_root`. `git_worktree.rs`'s
`resolve_workspace_git`/`git_worktree_add_submit` and `git_toolbar.rs`'s
`git_context` read the registry through `OpenedWorkRoots::resolve` and fed
the unnormalized path into `git` subprocess `-C` args and
`valid_branch_name`. Fixed by normalizing inside the shared accessor layer
(`OpenedWorkRoots::get`, via a new `normalize_registered_root` helper
reused by `candidate_paths`/`candidate_roots`/`owner_candidate_roots`) so
every reader is normalized uniformly; `resolve_online_available_work_root`
now relies on the already-normalized `get()` result instead of its own
`normalize_display_path` call. Added unit tests for
`resolve`/`get`/`candidate_paths`/`candidate_roots`/
`owner_candidate_roots` normalization. `cargo test -p ws-dashboard-daemon`
(full suite, 45 unit + 144 route + 15 server tests) passes clean. The
test partition's 2 Minor findings were accepted as pre-scoped-out per the
ticket/plan; no action taken.

#### Edition (fdb4a57d) - 2026-07-10

Item 7 (native Windows dogfood live-verification) completed after merge to
`ws-dashboard-dev`, on the real native-Windows daemon
(`D:\dbg-ws-dashboard-dev`, rebuilt at the merge tip, `--no-auth`,
`127.0.0.1:4100`), against a workRoot (`InspectTGV_AIDriven`) that was
originally registered by the pre-fix binary and had shown the
`\\?\`-prefixed corruption live during the initial dogfood that surfaced
this bug:

- `GET /api/dashboard/root-picker?path=D:\Workspace\Repos` returned every
  entry (including `InspectTGV_AIDriven`) with a plain path, no `\\?\`
  prefix.
- `POST /api/dashboard/work-roots/open` with that path resolved to the
  same already-registered `workRootId`, confirming the persisted registry
  entry self-healed rather than creating a duplicate.
- `GET .../git/status` for that workRoot succeeded (previously errored
  `"unknown workRoot"` before the workRoot was opened this session, then
  returned real branch/sync data once opened), confirming
  `git_toolbar.rs`'s accessor-driven path resolution works end-to-end.
- A live terminal spawned for that workRoot showed prompt
  `PS D:\Workspace\Repos\InspectTGV_AIDriven>` — plain path, no
  `Microsoft.PowerShell.Core\FileSystem::\\?\` provider-qualified prefix,
  confirming the exact original symptom is gone. Terminal closed after
  verification.

All Phase 1 items are now verified complete.


## Resolution (2026-07-10)

Fixed and merged (implement/windows-verbatim-path-normalize -> ws-dashboard-dev, merge commit at HEAD). Item 7 native-Windows live verification completed post-merge against the actual originally-corrupted workRoot: root-picker listing, self-healing re-open, git status, and a live terminal prompt all confirmed plain paths with no \\?\ prefix.
