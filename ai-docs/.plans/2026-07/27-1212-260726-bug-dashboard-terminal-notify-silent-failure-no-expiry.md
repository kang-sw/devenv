# Plan: 260726-bug-dashboard-terminal-notify-silent-failure-no-expiry — Phase 1: Bound the silence with a per-terminal failure record and give it a daemon-side reader

## Relevant Ticket Contract

- **Shape/paths (binding, do not re-derive):** `notify_failure_path(profile_dir) -> <profile_dir>/notify-failures.json`; `NotifyFailureRecord { count: u32, last_failure_at_ms: u64, last_error: String }`, serialized `camelCase` (`count`/`lastFailureAtMs`/`lastError`). New module, not `agent_callback.rs`.
- **Escalation rule (three conditions, ANDed), evaluated per live terminal on every `sweep_agent_profiles` tick:**
  1. record exists with `count >= 1`;
  2. `now - last_failure_at_ms >= AGENT_PROFILE_GC_SWEEP_PERIOD` (300 s, `server.rs:36`);
  3. `callback.json` mtime is **not** newer than `last_failure_at_ms` (missing/unreadable mtime counts as "not superseded").
  Warn once per terminal id per daemon lifetime; drop the warned flag when the id leaves the live set **or** is next observed with no record / `count == 0`.
- **Binding constraints (see `## Constraints` in the ticket, all apply):** do not extend `CallbackTarget`; writer must never `create_dir_all` the profile directory (skip silently if absent); `last_error` is `log_failure`'s own error string verbatim, truncated to 512 bytes on a `char` boundary; atomic temp-then-rename at `0600` created directly (mirror `write_callback_target`'s create-at-mode sequence, not `write_bound_base_url`'s write-then-chmod); temp name must be **unique** (reuse `agent_callback::unique_temp_path`, promote to `pub(crate)`), never the fixed `callback.json.tmp` shape; keyed by path sibling of `args.callback`, never a parsed terminal id; the already-warned set is an explicit `&mut NotifyFailureWatch` parameter (never a module static), owned by the sweep loop in `server.rs:159-172`; the blocking sweep body returns **owned** observations rather than borrowing across `spawn_blocking`; GC deletion ordering/contract is untouched — the new read only applies to directories the sweep already skips (live ids).
- **Verification boundary (ticket, `### Phase 1`):** writer unit coverage (increment across failures, clear on success, verbatim truncated `lastError`, `lastFailureAtMs` stamp, no directory creation, mode `0600`); escalation rule as a **pure function** over `(record, callback_mtime, now)` plus the warn-once set, no filesystem/no live sweep, covering each condition failing independently plus warn-once suppression plus the drop-then-rewarn case; one dedicated idle-owner test (count exactly 1, aged, unchanged callback → warns); one CLI-level silence-regression test extending `crates/daemon/tests/terminal_notify_end_to_end.rs`; `cargo test -p ws-dashboard-daemon` with the two pre-existing `routes.rs` failures tolerated by SITE, not exit code; a manual end-to-end confirmation this session cannot execute (see `## Verification Plan`).
- **Incidental cleanup on contact (ticket says do it, not a separate concern):** reword the stale "Phase 4 has not populated this callback target yet" text at `terminal_notify.rs:66-72` (Phase 4 shipped).
- **Spec Impact is a merge gate for this phase, not a follow-up:** add a NEW entry to `ai-docs/spec/ws-web-dashboard/index.md` immediately after `#260726-dashboard-terminal-attention-event-stream` describing the hook-to-daemon failure-visibility contract, and AMEND the "deliberately the only defense" sentence in `#260726-dashboard-terminal-tab-attention-indicator` to tier it without overclaiming (stranded badge stays stranded; only `daemon.log` gains a signal).

## Out of Scope

- Any frontend change, any change to `pendingAttentionStateFor` or the attention wire shapes (ticket: "Deferred scope").
- Any wall-clock expiry of attention state — rejected in `## Decisions`; do not reintroduce it even as a "small" addition.
- A "never-posted" liveness probe distinct from the failure counter — rejected in `## Decisions`.
- Retiring or reading `bound-base-url.json` — its do-not-read invariant is restated in the ticket and must not be weakened.
- A user-visible "hook delivery broken" affordance — rejected for now, reversible and additive later.
- Any second/duplicate end-to-end test harness for `terminal-notify` — extend `terminal_notify_end_to_end.rs`, do not stand up a new one.

## Codebase Findings

- `ws-dashboard/crates/daemon/src/terminal_notify.rs#L55-92` — `run_terminal_notify` calls `deliver`, and on `Err` calls `log_failure(&args, &error)` then unconditionally returns `Ok(())`. This is the exact hook point for `record_failure`/`clear_record`: both must receive the *same* `error` string `log_failure` gets (constraint: verbatim reuse), and both branches need `args.callback.parent()` as the profile dir.
- `ws-dashboard/crates/daemon/src/terminal_notify.rs#L66-72` — the stale-text `else` block (`target.terminal_id`/`target.token` both `None`). Ticket's own "Corrections" section already re-verified this range once; it still holds exactly at `:66-72` in the current tree.
- `ws-dashboard/crates/daemon/src/terminal_notify.rs#L147-152` — existing private `now_ms() -> u64` helper. Reuse it for the record's `now_ms` argument (no new duplicate needed in this file); this crate's convention (confirmed via `claude_cli.rs:818`, `terminal_helper_process.rs:712`, `terminal.rs:3338`, `codex_app_server.rs:817`) is a small private per-module `now_ms()` rather than one shared helper, so `agent_profile_gc.rs` should get its own copy for the async side rather than importing this one.
- `ws-dashboard/crates/daemon/src/agent_callback.rs#L160-185` — `write_callback_target`: the create-at-`0600` sequence to mirror (`agent_token_store::create_new_file_at_mode_0600` at `:182`, then `fs::rename` at `:183`). **Drift found:** the ticket's "Corrections" section cites the mode-`0600` call at `:179` and the test at `agent_callback.rs:392-409`; both have drifted a further ~3-5 lines since that correction was recorded — the call is now at `:182`, and `write_callback_target_writes_at_mode_0600` now spans `:395-412` (was cited `:392-409`). Non-blocking (the referenced code itself is unchanged, only line numbers moved), but confirms this ticket's own corrections layer needs a second correction pass eventually.
- `ws-dashboard/crates/daemon/src/agent_callback.rs#L187-200` — `unique_temp_path` (module-private `fn`, not `pub(crate)` yet, currently at `:192` not the ticket's cited `:189` — same minor drift as above). Must be promoted to `pub(crate)`; it is the only helper in the tree that produces a collision-free temp name for a non-terminal-id-keyed writer, and this record's writer is exactly that case per the ticket's constraint on why `write_callback_target`'s fixed temp name does not transfer.
- `ws-dashboard/crates/daemon/src/agent_token_store.rs#L110-127` — `create_new_file_at_mode_0600` (Unix: `OpenOptions::create_new(true).mode(0o600)`; non-Unix: plain `fs::write`), already `pub(crate)`. `create_new(true)` fails with `NotFound` if the parent directory is absent — this is exactly the mechanism the writer's "must not create the profile directory" constraint should lean on (check `profile_dir.is_dir()` before attempting the write, rather than relying on the error path, since `create_new_file_at_mode_0600` itself never creates directories).
- `ws-dashboard/crates/daemon/src/agent_profile_gc.rs#L60-110` — `sweep_agent_profiles`/`sweep_agent_profiles_blocking`. The blocking function currently only computes `orphaned_profile_ids(names, live_ids)` (directories NOT in `live_ids`) and reclaims them; it never touches or observes directories that ARE in `live_ids` — those are exactly "the live directories it skips" that the new read must additionally observe. `names` is presently a single-consume iterator; must be materialized into a `Vec<String>` first so both the (unchanged) orphan-reclaim pass and the new live-directory observation pass can each iterate it.
- `ws-dashboard/crates/daemon/src/agent_profile_gc.rs#L186`, `#L216`, `#L269`, `#L295` — the four `sweep_agent_profiles(&state_dir, &registry).await;` call sites inside `#[cfg(test)] mod tests`. **Confirmed exact** (no drift) against the ticket's cited line numbers. Each needs `&mut notify_failure::NotifyFailureWatch::default()` appended.
- `ws-dashboard/crates/daemon/src/server.rs#L36` — `const AGENT_PROFILE_GC_SWEEP_PERIOD: Duration = Duration::from_secs(300);`, currently module-private. Confirmed exact line. Needs `pub(crate)` so the escalation rule's grace-window comparison can reference the single named constant instead of duplicating the literal `300_000`.
- `ws-dashboard/crates/daemon/src/server.rs#L159-173` — the `gc_sweep_task` closure: calls `sweep_agent_profiles` once immediately (`:162`) then once per `interval.tick()` (`:170`) inside an infinite `loop`. Per the ticket's constraint, `NotifyFailureWatch::default()` must be constructed **inside** this `async move` block (so it is owned by the task and dies with it on the paired `.abort()` shutdown path — see the surrounding CONTRACT comment at `:139-158` about this task being untracked-and-abortable) and threaded as `&mut` into both call sites.
- `ws-dashboard/crates/daemon/src/terminal.rs#L506-514` — `live_terminal_ids(&self) -> HashSet<String>`, `pub(crate)`. Confirms the type the sweep already threads through, and what the pure `NotifyFailureWatch::retain_live` method should accept.
- `ws-dashboard/crates/daemon/src/agent_profile_registry.rs#L96-97` — `CLAUDE_HOOK_CONFIG`'s `events: &[("UserPromptSubmit", "working"), ("Stop", "ready")]`. Confirmed exact, no drift — this is the arithmetic basis for why the escalation rule cannot be a bare count threshold (one turn = at most 2 invocations).
- `ws-dashboard/crates/daemon/src/cli.rs#L34-35` and `ws-dashboard/crates/daemon/src/main.rs#L21-28` — hidden-subcommand registration and pre-`logging::init` dispatch. Both confirmed exact, unaffected by this phase (no CLI surface change).
- `ws-dashboard/crates/daemon/tests/terminal_notify.rs#L124-127,#L168-180` — the existing `terminal_notify_exits_zero_and_stays_silent_on_stdio_when_the_callback_file_is_missing` test uses a `fixture_dir` that is **never created** (`temp_fixture_dir` only builds a `PathBuf`, no `create_dir_all`). This is a live proof point that `args.callback.parent()` can point at a directory that does not exist on disk — the new writer's "skip, do not create" branch must handle this exact case, and this existing test implicitly continues to guard against a regression (it asserts silence, not presence/absence of `notify-failures.json`, so no test edit needed there — but the new writer logic must not panic or error out into stdout/stderr on this input).
- `ws-dashboard/crates/daemon/tests/terminal_notify_end_to_end.rs#L280-286,#L332-355` — establishes the `temp_fixture_path` naming helper and the `Command::new(env!("CARGO_BIN_EXE_ws-dashboard")).arg("terminal-notify")...stdin(Stdio::null())...output().await` pattern already used for the real-CLI-subprocess assertion in this file. The new silence-regression test should reuse this exact pattern rather than the mock-listener pattern in `terminal_notify.rs`'s own test file (ticket: "Extend the existing... harness rather than standing up a second one").
- `ws-dashboard/crates/daemon/src/lib.rs#L23-24` — `pub mod mock;` then `pub mod persistent_state;`. New module declaration `pub mod notify_failure;` sorts alphabetically between them.
- `ai-docs/spec/ws-web-dashboard/index.md#L2323-2401` — flat `## Heading {#anchor}` sections, no separate TOC to update. The NEW entry goes between `#260726-dashboard-terminal-attention-event-stream` (ends `:2360`) and `#260726-dashboard-terminal-tab-attention-indicator` (starts `:2362`). The AMEND target sentence is at `:2375-2377` ("This is a presentation gate, not a daemon guarantee, and it is deliberately the only defense").
- `ai-docs/mental-model/ws-web-dashboard/terminal.md` (~`:78`, the "Testing a deliberately-silent CLI's delivery path..." trap) — reinforces reusing `terminal_notify_end_to_end.rs`'s transparent-relay/real-subprocess pattern rather than a hand-built mock, consistent with the ticket's own instruction. No mental-model edit needed for this phase (no new operational trap introduced beyond what the ticket already documents).

**Risk signals surfaced by the survey (none blocking, all resolved into the plan below):**

- The blocking-sweep return type change (`()` → `Vec<(String, Option<NotifyFailureRecord>, Option<u64>)>`) touches a function whose ordering CONTRACT (`agent_profile_gc.rs:1-25`) is explicitly load-bearing for a *different* regression (`terminal_notify_callback_restart.rs`'s adopt-before-sweep ordering test). The plan below keeps the orphan-reclaim loop's logic and iteration order byte-for-byte identical, only adding a second, independent pass over the same already-collected `Vec<String>` of directory names — it does not touch which directories get deleted or when.
- `should_warn`'s grace-window constant referencing `crate::server::AGENT_PROFILE_GC_SWEEP_PERIOD` creates a dependency from a low-level module back to the top-level wiring module. This is unusual layering but not a cycle (Rust has no file-level import-cycle restriction within one crate), and it is the only way to honor the ticket's explicit tie between the grace window and that specific named constant rather than duplicating the `300_000` literal.

## Implementation Plan

1. **`ws-dashboard/crates/daemon/src/agent_callback.rs`** — change `fn unique_temp_path` (currently `:192`) to `pub(crate) fn unique_temp_path`. No other change in this file.
2. **`ws-dashboard/crates/daemon/src/server.rs`** — change `const AGENT_PROFILE_GC_SWEEP_PERIOD` (`:36`) to `pub(crate) const AGENT_PROFILE_GC_SWEEP_PERIOD`.
3. **New file `ws-dashboard/crates/daemon/src/notify_failure.rs`.** Module header comment states the CONTRACT this ticket settles (writer-owned sibling state, policy lives in the reader, warn-once semantics). Contents:
   - `#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)] #[serde(rename_all = "camelCase")] pub struct NotifyFailureRecord { pub count: u32, pub last_failure_at_ms: u64, pub last_error: String }`.
   - `pub fn notify_failure_path(profile_dir: &Path) -> PathBuf { profile_dir.join("notify-failures.json") }`.
   - `const MAX_LAST_ERROR_BYTES: usize = 512;` plus a private `fn truncate_last_error(error: &str) -> String` that walks back from byte 512 to the nearest `char` boundary via `str::is_char_boundary`.
   - `pub fn read_record(profile_dir: &Path) -> Option<NotifyFailureRecord>` — best-effort `fs::read_to_string` + `serde_json::from_str`, `None` on any error (missing file, unreadable, malformed).
   - `pub fn record_failure(profile_dir: &Path, error: &str, now_ms: u64)`:
     - `if !profile_dir.is_dir() { return; }` (constraint: never create the profile directory).
     - `let previous_count = read_record(profile_dir).map(|r| r.count).unwrap_or(0);`
     - Build `NotifyFailureRecord { count: previous_count.saturating_add(1), last_failure_at_ms: now_ms, last_error: truncate_last_error(error) }`, serialize with `serde_json::to_string_pretty`.
     - `let temp_path = crate::agent_callback::unique_temp_path(&notify_failure_path(profile_dir));` then `crate::agent_token_store::create_new_file_at_mode_0600(&temp_path, raw.as_bytes())` then `fs::rename(&temp_path, &path)`. Swallow every `Result::Err` silently (`let _ =` / early `return`), matching `log_failure`'s silence CONTRACT — no stdout/stderr, no changed exit code, on the write path or the rename path.
   - `pub fn clear_record(profile_dir: &Path) { let _ = fs::remove_file(notify_failure_path(profile_dir)); }`.
   - `#[derive(Debug, Default)] pub struct NotifyFailureWatch { warned: HashSet<String> }` with:
     - `pub fn should_warn(&mut self, terminal_id: &str, record: Option<&NotifyFailureRecord>, callback_mtime_ms: Option<u64>, now_ms: u64) -> bool` — pure, no I/O:
       1. `let has_failure = record.map(|r| r.count > 0).unwrap_or(false);`
       2. `if !has_failure { self.warned.remove(terminal_id); return false; }` (drop-rule trigger #2: no record or count 0).
       3. `let record = record.expect(...);`
       4. `let grace_ms = crate::server::AGENT_PROFILE_GC_SWEEP_PERIOD.as_millis() as u64;`
       5. `let aged_enough = now_ms.saturating_sub(record.last_failure_at_ms) >= grace_ms;`
       6. `let not_superseded = callback_mtime_ms.map_or(true, |mtime| mtime <= record.last_failure_at_ms);`
       7. `if !aged_enough || !not_superseded { return false; }`
       8. `if self.warned.contains(terminal_id) { return false; }` (warn-once suppression).
       9. `self.warned.insert(terminal_id.to_owned()); true`
     - `pub fn retain_live(&mut self, live_ids: &HashSet<String>) { self.warned.retain(|id| live_ids.contains(id)); }` (drop-rule trigger #1: id left the live set).
   - `#[cfg(test)] mod tests` — see `## Verification Plan` for the exact case list.
4. **`ws-dashboard/crates/daemon/src/lib.rs`** — insert `pub mod notify_failure;` between `pub mod mock;` and `pub mod persistent_state;`.
5. **`ws-dashboard/crates/daemon/src/terminal_notify.rs`**:
   - Reword the stale `else` block at `:66-72` to drop "Phase 4 has not populated this callback target yet" (e.g. "callback file at {} has no terminalId/token — expected a per-terminal callback target, not a bare base-url file"). No test currently asserts this exact substring (confirmed via grep — only the source line itself matches).
   - In `run_terminal_notify` (`:55-60`), replace the `if let Err(error) = ... { log_failure(...) }` shape with a `match` (or equivalent) so both arms run:
     ```
     let now = now_ms();
     match deliver(&args).await {
         Ok(()) => {
             if let Some(profile_dir) = args.callback.parent() {
                 crate::notify_failure::clear_record(profile_dir);
             }
         }
         Err(error) => {
             if let Some(profile_dir) = args.callback.parent() {
                 crate::notify_failure::record_failure(profile_dir, &error, now);
             }
             log_failure(&args, &error);
         }
     }
     Ok(())
     ```
     `error` is passed to both `record_failure` and `log_failure` — same binding, so verbatim reuse is structural, not just documented.
6. **`ws-dashboard/crates/daemon/src/agent_profile_gc.rs`**:
   - Add a private `fn now_ms() -> u64` (mirrors this crate's existing per-module convention — see Codebase Findings).
   - Change `sweep_agent_profiles_blocking(state_dir: &Path, live_ids: &HashSet<String>)` to return `Vec<(String, Option<crate::notify_failure::NotifyFailureRecord>, Option<u64>)>`:
     - Collect `names` into `let names: Vec<String> = read_dir.filter_map(...).collect();` (was a lazy iterator consumed once; now materialized so it can be walked twice).
     - Keep the existing `for orphaned in orphaned_profile_ids(names.iter().cloned(), live_ids) { ... }` reclaim loop byte-for-byte as today.
     - Add: `names.into_iter().filter(|n| is_sane_directory_name(n) && live_ids.contains(n)).map(|terminal_id| { let dir = profile_root.join(&terminal_id); let record = crate::notify_failure::read_record(&dir); let callback_mtime_ms = std::fs::metadata(crate::agent_callback::callback_path(&dir)).ok().and_then(|m| m.modified().ok()).and_then(|t| t.duration_since(std::time::UNIX_EPOCH).ok()).map(|d| d.as_millis() as u64); (terminal_id, record, callback_mtime_ms) }).collect()` — return this `Vec` as the function's value. This never influences which directories were just deleted above (constraint: ordering contract untouched).
     - The two early-return arms (`NotFound` → return, unreadable → warn + return) now return `Vec::new()` instead of `()`.
   - Change `pub async fn sweep_agent_profiles(state_dir: &Path, registry: &TerminalRegistry, watch: &mut crate::notify_failure::NotifyFailureWatch)`:
     - `let live_ids = registry.live_terminal_ids();`
     - Clone `live_ids` for the `spawn_blocking` move (`let live_ids_for_blocking = live_ids.clone();`), keep the original for `retain_live` after the await.
     - `let observations = tokio::task::spawn_blocking(move || sweep_agent_profiles_blocking(&state_dir, &live_ids_for_blocking)).await.unwrap_or_else(|error| { tracing::warn!(%error, "agent-profiles GC sweep task panicked"); Vec::new() });`
     - `let now = now_ms();`
     - `for (terminal_id, record, callback_mtime_ms) in &observations { if watch.should_warn(terminal_id, record.as_ref(), *callback_mtime_ms, now) { let record = record.as_ref().expect("should_warn only returns true with a recorded failure"); tracing::warn!(terminal_id = %terminal_id, count = record.count, last_error = %record.last_error, "terminal-notify delivery has been failing and has not self-healed"); } }`
     - `watch.retain_live(&live_ids);`
   - Update the four test call sites (`:186`, `:216`, `:269`, `:295`) to `sweep_agent_profiles(&state_dir, &registry, &mut crate::notify_failure::NotifyFailureWatch::default()).await;`.
7. **`ws-dashboard/crates/daemon/src/server.rs`** — inside the `gc_sweep_task` closure (`:159-173`), add `let mut notify_failure_watch = crate::notify_failure::NotifyFailureWatch::default();` as the first statement inside `async move { ... }` (owned by the spawned task, so its lifetime matches the task's — it must die on the paired `.abort()` shutdown path, per the ticket's constraint), then pass `&mut notify_failure_watch` to both `sweep_agent_profiles` calls (`:162`, `:170`).
8. **`ws-dashboard/crates/daemon/tests/terminal_notify_end_to_end.rs`** — add one new `#[tokio::test]`, e.g. `terminal_notify_cli_stays_silent_against_a_deliberately_broken_callback_target`, following the file's existing `temp_fixture_path` + `Command::new(env!("CARGO_BIN_EXE_ws-dashboard"))` pattern:
   - Write a `callback.json` fixture: `{"baseUrl":"http://127.0.0.1:1","terminalId":"t1","token":"wrong"}` (port 1 refuses immediately — bounded by `CONNECT_TIMEOUT`, no real daemon needed, deterministic and fast).
   - Run the compiled CLI directly against it with `--state ready`.
   - Assert `status.success()`, `stdout` empty, `stderr` empty — the regression guard the ticket calls "most at risk of breaking" by this phase.
9. **Spec** — edit `ai-docs/spec/ws-web-dashboard/index.md`:
   - Insert a NEW `## <title> {#260726-dashboard-terminal-notify-failure-visibility}` section between the end of `#260726-dashboard-terminal-attention-event-stream` (`:2360`) and the start of `#260726-dashboard-terminal-tab-attention-indicator` (`:2362`), covering exactly the five points the ticket's `## Spec Impact` enumerates: the hook process's permanent stdio silence (source-comment-only today); every failure appended to a rotated daemon-side log; the per-terminal record (count/timestamp/error text, cleared on success); the daemon surfacing an unrepaired-past-grace-window, not-superseded failure once per terminal; and the explicit non-goals (no attention-state expiry, no user-facing affordance). Follow spec authoring conventions (`ws/convention.read` spec, or the bundled fallback) for heading/anchor style — sibling anchors in this neighborhood use the `260726-dashboard-*` prefix.
   - Amend the sentence at `:2375-2377` ("This is a presentation gate, not a daemon guarantee, and it is deliberately the only defense") to state both halves explicitly per the ticket: the badge itself gains no new defense (still the only thing that ever clears it, still only on a dead session), and the new record/reader gives the *operator* — not the badge — a `daemon.log` signal. Do not let the tiering imply the stranded-badge case is now handled.

## Verification Plan

- `cd ws-dashboard && cargo test -p ws-dashboard-daemon > /path/in/scratchpad/notify-failure-phase1.log 2>&1` **then** `echo $?` on the very next line of the same invocation (never `| tee`, `| tail`, or `; echo $?` — the ticket's own verification history recorded a false `0` from the semicolon form). Read the log's `failures:` block, not the exit status.
  - **Two known-baseline failures tolerated by SITE, not message drift:** `dashboard_resources_refresh_prunes_workspace_without_available_work_roots` (`crates/daemon/tests/routes.rs:1066`) and `online_missing_work_root_returns_bounded_unavailable_without_path_leak` (`crates/daemon/tests/routes.rs:1383`), baseline `174 passed; 2 failed` in the `routes` target at `8bbb1f6d`. A third failing site anywhere, or a changed message at either of these two sites, is this phase's regression — stop and investigate rather than rerunning.
  - All new tests below must show as individually passed in the log.
- New pure-function unit tests in `notify_failure.rs`'s `#[cfg(test)] mod tests` (no filesystem, no live sweep, no `tokio`):
  - `should_warn` returns `false` when `record` is `Some` with `count: 0`.
  - `should_warn` returns `false` when the failure is younger than the grace window (`now_ms - last_failure_at_ms < 300_000`).
  - `should_warn` returns `false` when `callback_mtime_ms` is newer than `last_failure_at_ms` (self-heal suppression), with an otherwise-aged, count-1 record.
  - `should_warn` returns `true` when `callback_mtime_ms` is `None` (missing/unreadable callback file must NOT suppress), with an otherwise-aged, count-1 record.
  - Warn-once: two consecutive `should_warn` calls with the same aged, unrepaired, unchanged record — first call `true`, second call `false`.
  - Drop-then-rewarn: `should_warn` → `true` (warns); then `should_warn` with `record` absent or `count: 0` → `false` (and removes the id from the warned set); then `should_warn` with a fresh aged, unrepaired record for the same id → `true` again. This is the ticket's explicit drop-rule test.
  - Idle-owner case (the mechanism's whole reason to exist): `count: 1` exactly, aged past the grace window, `callback_mtime_ms` older than/equal to the failure (unchanged target) → `true`. Must not require `count >= 2` or `>= 3`.
  - Recommended (beyond the ticket's explicit list, cheap and directly required by the "leaves the live set" drop-rule constraint): `retain_live` drops a warned id once it is absent from the passed live-id set, and a subsequent aged/unrepaired observation for that same id after re-entering the live set warns again.
- New writer unit tests in `notify_failure.rs`'s `#[cfg(test)] mod tests` (real filesystem, temp dirs, no `tokio` needed — these are sync fs calls):
  - `record_failure` called twice against the same existing profile dir increments `count` from 1 to 2 and updates `last_error`/`last_failure_at_ms` to the second call's values.
  - `record_failure` stores `last_error` as exactly the passed string when under 512 bytes (byte-for-byte, proving no reformatting).
  - `record_failure` with an error string over 512 bytes truncates to `<= 512` bytes and lands on a `char` boundary (use a multi-byte UTF-8 filler that straddles the 512-byte cut point, and assert `String::from_utf8` succeeds / no panic and no U+FFFD).
  - `clear_record` after one or more `record_failure` calls leaves `read_record` returning `None` (file removed).
  - `record_failure` against a profile dir that was never created leaves the directory absent afterward (`!profile_dir.exists()`) — proves the "never create the profile directory" constraint.
  - `#[cfg(unix)]` `record_failure` writes `notify-failures.json` at mode `0600`.
- New CLI-level regression test in `crates/daemon/tests/terminal_notify_end_to_end.rs` (implementation step 8 above): exit status `0`, empty stdout, empty stderr against a deliberately-broken callback target, driving the real compiled binary.
- **Manual CLI-level reproduction (executable now, deterministic, no wall-clock wait beyond process spawn time — run this in the session):**
  1. `cd ws-dashboard && cargo build -p ws-dashboard-daemon --bin ws-dashboard > /path/in/scratchpad/build.log 2>&1` then `echo $?` next line.
  2. Pick scratch paths, e.g. `WS_DASHBOARD_STATE_HOME=/private/tmp/.../notify-repro-state` and a callback file at `/private/tmp/.../notify-repro/callback.json` containing `{"baseUrl":"http://127.0.0.1:1","terminalId":"t1","token":"wrong"}`.
  3. Run `./target/debug/ws-dashboard terminal-notify --callback <path> --state ready > run1.log 2>&1` then `echo $?` next line, five times total. Confirm: exit `0` every time, `run*.log` empty every time.
  4. Confirm exactly one rotated file under `<state dir>/logs/terminal-notify.log.<date>` with 5 lines.
  5. Confirm `notify-failures.json` beside `callback.json` shows `count: 5`, `lastFailureAtMs` matching the fifth run's approximate wall clock, and `lastError` matching the fifth log line's error text.
  6. Do not attempt a live-daemon re-point-and-clear step by hand here — the ticket's own step 6 requires a live daemon with a valid token, which is out of scope for a fast deterministic repro. `clear_record`'s behavior is already covered deterministically by the writer unit test above (`clear_record` after `record_failure`), and real delivery-then-clear is covered end-to-end by the existing `terminal_notify_cli_delivers_a_real_turn_state_post_through_the_real_route` test once this phase wires `clear_record` into the `Ok(())` arm of `run_terminal_notify`.
- **NOT executed in this session, and must not be faked or approximated:** the ticket's `## Reproduction` "End-to-end level" step 6 — "Wait out two sweep periods (~10 minutes) with the daemon still running... confirm `daemon.log` stays silent for the whole window," and the Phase 1 verification boundary's final "Manual confirmation" bullet (same wait, post-fix, confirming exactly one warning appears and does not repeat on a third sweep). This requires a real Claude agent, a browser, and ~10 minutes of wall-clock time across two 300 s sweep periods, which this session cannot provide. The pure-function unit tests above exercise the identical logic with `now` as a parameter (zero wall-clock cost) and the CLI-level tests prove the writer side; together they are the closest deterministic substitute, but they are not a substitute for the real end-to-end confirmation. Do not sleep 600 seconds to fake this.

## Escalations

- None. Confidence is high: every file-path, function-name, and line-range claim in the ticket that bears on this phase's implementation was re-verified against the current tree (see Codebase Findings for the two small further-drifted line-number citations, both non-blocking), the design decisions are fully settled in `## Decisions` with rejected alternatives recorded, and the pure-function decomposition the verification boundary requires has a concrete, testable signature (`NotifyFailureWatch::should_warn(&mut self, terminal_id: &str, record: Option<&NotifyFailureRecord>, callback_mtime_ms: Option<u64>, now_ms: u64) -> bool`, in `notify_failure.rs`).
