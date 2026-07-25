# Plan: 260725-feat-dashboard-pty-agent-attention-notification — Phase 3, steps 2 and 3

## Relevant Ticket Contract

- Step 2: "Daemon materializes the vendor hook config under
  `agent-profiles/<terminal_id>/` at `0600` and passes its PATH through
  helper argv."
- Step 3: "Add the hidden `ws-dashboard terminal-notify` subcommand and the
  bound-base-URL file the daemon rewrites on every bind."
- Step 1's spike is CLOSED and recorded inline in the ticket (POSITIVE:
  `UserPromptSubmit` and `Stop` both fire in a real interactive PTY via a
  `--settings <file>` path; hook shape proven:
  `{"hooks": {"<Event>": [{"matcher": "*", "hooks": [{"type": "command",
  "command": "<cmd>"}]}]}}`). NOT proven by the spike: `0600` permissions
  specifically (spike file was `0644`), and the daemon->helper->portable-pty
  delivery seam with the Phase-1 env scrub applied.
- "The token never touches the helper or the registry" (`## Decisions`):
  helper argv carries file PATHS only, never the callback token. The
  callback token itself is Phase 4's artifact; this phase must not invent a
  channel that later has to be un-invented.
- "Ephemeral port" (`## Decisions`): the callback URL cannot be baked at
  spawn time (`--port` defaults to `0`); the bound base URL must be
  resolvable at hook-FIRE time, not frozen at spawn time.
- On-disk layout (`## Decisions`, pinned): all under the daemon state dir,
  all `0600`: `agent-profiles/<terminal_id>/settings.json` (vendor config,
  no secret) and `agent-profiles/<terminal_id>/callback.json` (Phase 4's
  `{baseUrl, terminalId, token}`, kept separate from the vendor config on
  purpose). `agent-profiles/<terminal_id>/` is the Phase-4 GC sweep's scan
  root.
- Hook command shape (`## Decisions`, pinned): `ws-dashboard terminal-notify
  --callback <path> --state ready`, chosen over `curl` for portability
  (quoting, Windows availability).
- `claude_cli.rs:473-497` is cited ONLY for the hook-block JSON shape
  (confirmed identical to the spike's proven shape), explicitly NOT for its
  delivery mechanism (that module injects the JSON inline as argv; this
  ticket's `0600`-file delivery is a deliberate divergence, recorded in
  `## Decisions`).
- No `TerminalRegistryEntry` field change (`## Constraints`, cross-referenced
  bug ticket `260725-bug-dashboard-terminal-registry-schema-evolution-orphans-helpers`).
- `AgentProfile.hook_config: Option<HookConfigShape>` (`agent_profile_registry.rs:14-22,35`)
  is an explicit placeholder Phase 2 left for this phase: "Phase 3 extends
  `hook_config` rather than replacing this shape."

## Out of Scope

- Phase 4 and all later phases: the daemon-owned token store, the
  `POST /api/dashboard/terminals/{terminal_id}/turn-state` callback route,
  the `agent-profiles/` GC sweep, and the `#260515-ws-web-daemon-foundation`
  spec amendment for the unauthenticated route class.
- The callback token itself. Nothing in this phase generates, stores, or
  transmits a token. `callback.json` (which will hold the token) is not
  written by this phase.
- Any `TerminalRegistryEntry` schema change.
- Any real vendor-CLI dependency in tests (no `claude` binary invoked; the
  vendor hook *shape* was already proven by the closed spike, not
  re-verified here).
- Removal/cleanup of `agent-profiles/<terminal_id>/` on terminal close or at
  daemon restart with stale directories present — that GC sweep is
  explicitly Phase 4's, ordered strictly after `boot_reconcile`. This phase
  only creates the directory/file at spawn time.
- `boot_reconcile`/`reconcile_entry` changes: adopting a still-live helper
  reconnects over IPC to an already-running process; it does not re-spawn or
  re-materialize hook config, so this phase does not touch that path.
- Any browser-visible change. Nothing in this phase touches
  `frontend/`; Phase 3 steps 2-3 are daemon-only Rust.

## Codebase Findings

- `ws-dashboard/crates/daemon/src/agent_profile_registry.rs:14-22,29-36,68-74` —
  `HookConfigShape` is a placeholder unit struct; `AgentProfile.hook_config`
  is `Option<HookConfigShape>` (owned, `Copy`), currently `None` for both
  registered profiles (`claude`, `dummy-echo`). This phase gives the type
  real fields for the `claude` profile only (`dummy-echo` stays `None` — no
  hooks for the test-only profile). Keep the field name/slot; only widen the
  type's contents so this stays an extension, not a struct-shape rewrite.
- `ws-dashboard/crates/daemon/src/terminal.rs:984-1007` (`resolve_create_command`) —
  pure, I/O-free resolver returning
  `(Option<(String, Vec<String>)>, Vec<(String,String)>, Option<&'static EnvScrubProfile>)`,
  paired Some/Some per resolved profile, `None` triple for no/absent
  profile. Extend to a 4-tuple by appending `Option<HookConfigShape>`
  (`profile.hook_config`, plain copy — no lifetime widening needed since the
  field is owned/`Copy`). Existing unit tests at `:2375,2384,2399` destructure
  the 3-tuple and must be updated for the new arity.
- `ws-dashboard/crates/daemon/src/terminal.rs:669-720` (`create_terminal`) —
  single call site of both `resolve_create_command` and `TerminalSession::spawn`;
  thread the new `hook_config` value straight through, no new HTTP-facing
  field (browser API is untouched).
- `ws-dashboard/crates/daemon/src/terminal.rs:1011-1051` (`TerminalSession::spawn`) —
  `let id = opaque_terminal_id();` (L1028) generates the terminal id BEFORE
  `build_helper_command` is called (L1031). This is the ONLY point in the
  spawn path where a per-terminal id is available before the helper argv is
  built, so hook-config materialization (which needs the id for
  `agent-profiles/<terminal_id>/`) must happen here, between those two
  lines — not inside `resolve_create_command` (called before any id exists)
  and not inside `build_helper_command` (a pure argv builder with no
  filesystem access today, deliberately kept that way per its own doc
  comment about `scrub` being "inert on the default path"). Add a new
  `hook_config: Option<HookConfigShape>` parameter to `spawn`
  (already `#[allow(clippy::too_many_arguments)]`, so one more parameter is
  consistent with precedent, not a new violation).
- `ws-dashboard/crates/daemon/src/terminal.rs:871-949` (`build_helper_command`) —
  `command`'s args are forwarded one-by-one as repeated `--command-arg`
  flags (L910-913). A materialized settings-file path can therefore reach
  the helper as an ORDINARY extra `--command-arg` pair (`--settings
  <path>`) appended to the profile's `args` Vec before this function is
  called — no new CLI flag is needed at the `TerminalHelperArgs` layer.
  This keeps the "helper argv carries file PATHS only" invariant intact:
  the settings path is a path, not a secret.
- `ws-dashboard/crates/daemon/src/terminal_registry_file.rs:37-55`
  (`write_registry_entry`) — the exact atomic-write-with-`0600`-on-Unix
  pattern to reuse: `fs::write` to a `.json.tmp` sibling,
  `fs::set_permissions(&temp_path, fs::Permissions::from_mode(0o600))`
  under `#[cfg(unix)]` (no Windows permission call at all — Windows has no
  POSIX mode bits, and this is the codebase's own existing precedent for
  leaving Windows permission-unhardened rather than inventing an ACL story),
  then `fs::rename` for atomicity. This is a synchronous, `std::fs`-based
  helper called directly from sync contexts elsewhere; reuse the SAME shape
  for both the per-terminal `settings.json` and the new global
  bound-base-url file rather than inventing a second atomic-write idiom.
- `ws-dashboard/crates/daemon/src/persistent_state.rs:485-510`
  (`default_state_file`/`default_state_dir`) — the SINGLE state-home
  resolution order (`WS_DASHBOARD_STATE_FILE` >
  `WS_DASHBOARD_STATE_HOME` > `XDG_STATE_HOME` > `HOME` > (Windows)
  `LOCALAPPDATA`), already reused by `terminal.rs::default_registry_dir()`
  (`state_dir.join("terminals")`, `terminal.rs:173-177`). `agent-profiles/`
  and the new bound-base-url file must resolve through this SAME
  `default_state_dir()` call, as a sibling of `terminals/` — not a new env
  var, not a new resolution order, and not nested inside `agent-profiles/`
  itself (that directory is reserved as the Phase-4 GC sweep's scan root
  over per-terminal-id subdirectories; a stray non-terminal-id file at its
  top level would need special-casing in that later sweep).
- `ws-dashboard/crates/daemon/src/server.rs:61,69-70,99-104` — `bound_addr =
  listener.local_addr()?` is known immediately after `TcpListener::bind`
  (L69), strictly BEFORE `TerminalRegistry::boot_reconcile` runs (L99-104).
  This is the correct, and only reasonable, place to write the bound-base-url
  file — it needs no terminal/registry state, only the bound address, and
  writing it before `boot_reconcile` means any adopted terminal's eventual
  (Phase 4) callback rewrite can already find a fresh file.
- `ws-dashboard/crates/daemon/src/cli.rs:18-28` (`Command` enum) —
  `TerminalHelper` is the existing hidden-subcommand precedent
  (`#[command(hide = true)]`, dispatched in `main.rs:17-20` before
  `into_serve_config`). Add `TerminalNotify(TerminalNotifyArgs)` the same
  way: a new hidden variant, a new accessor mirroring
  `terminal_helper_args()` (`cli.rs:176-181`), and a new branch in
  `main.rs` before the `log_file`/`into_serve_config` path (mirroring
  `main.rs:17-20`).
- `ws-dashboard/crates/daemon/src/claude_cli.rs:473-497`
  (`default_deny_hook_settings`) — hook JSON shape reference only (matches
  the spike's proven shape exactly): `{"hooks": {"<Event>": [{"matcher":
  "*", "hooks": [{"type": "command", "command": "<cmd>"}]}]}}`. Do NOT
  reuse this function or its inline-argv delivery; only the shape is
  precedent. Note `shell_single_quote` (`claude_cli.rs:500-502`) as a
  reusable pattern for quoting the command string if the notify binary path
  or callback path could contain spaces (both are under a user's home
  directory / state dir, which is not guaranteed space-free, especially on
  Windows/macOS) — a risk signal worth guarding against explicitly rather
  than assuming POSIX-clean paths.
- `ws-dashboard/crates/daemon/src/terminal.rs:169-171`
  (`default_helper_binary`, `std::env::current_exe().unwrap_or_else(...)`) —
  the existing precedent for resolving "my own binary's absolute path"
  without relying on `$PATH`. The hook command materialized in
  `settings.json` must embed this SAME resolved absolute path rather than a
  bare `ws-dashboard` program name, because Phase 1 already scrubs the
  spawned agent's environment of Claude-specific markers (not `PATH`
  itself, but relying on `PATH` resolution inside a vendor CLI's hook-runner
  shell is fragile and untested) — resolving the binary path once, the same
  way the helper spawn already does, is the safe, precedented answer to
  "how does the hook command find the `ws-dashboard` binary."
- `ws-dashboard/crates/daemon/Cargo.toml:28` — `reqwest.workspace = true` is
  already a daemon-crate dependency; `terminal-notify`'s HTTP POST attempt
  needs no new dependency.
- `ws-dashboard/crates/daemon/tests/terminal_lifetime.rs:110`,
  `terminal_windows_reaper_acceptance.rs:264` — the
  `env!("CARGO_BIN_EXE_ws-dashboard")` pattern for spawning the REAL
  compiled binary as a subprocess in an integration test. Use this for the
  `terminal-notify` end-to-end test (real subprocess, real argv parsing,
  real file I/O) rather than calling internal functions directly, since the
  CLI boundary itself (argv -> file resolution -> HTTP attempt) is what
  needs proving.
- `ws-dashboard/crates/daemon/tests/server.rs:1-40` — `run_with_shutdown`
  is directly callable from a test (no subprocess needed) with a real
  `TcpListener` bind to port `0`, giving a genuinely ephemeral,
  unpredictable port each run. Reuse this for the "rewrites on every bind"
  test: two sequential `run_with_shutdown` calls under a temp
  `WS_DASHBOARD_STATE_HOME` must produce two different `bound-base-url.json`
  contents.
- `ws-dashboard/crates/daemon/src/persistent_state.rs:512-521` (`ENV_LOCK`) —
  a `cfg(test)` mutex serializing access to the state-home env vars; any new
  test that sets/reads `WS_DASHBOARD_STATE_HOME` (or relies on
  `default_state_dir()`) MUST hold this lock for its whole body, per the
  existing doc comment, to avoid racing sibling tests that mutate the same
  process-global vars.
- No existing daemon-side code sets `0600` (or any explicit permission) on a
  DIRECTORY — only on individual files
  (`terminal_registry_file.rs:48-52`). Follow that precedent: create
  `agent-profiles/<terminal_id>/` with ordinary `fs::create_dir_all`
  (default/umask permissions, matching how `registry_dir` itself is created
  today), and apply `0600` only to the `settings.json` FILE inside it. The
  ticket's "all `0600`" bullet lists files (`settings.json`,
  `callback.json`, `terminal-tokens/<id>.json`), not directories; a
  directory needs its execute bit to be traversable at all, so treating it
  identically to a file would be a mistake, not extra safety.

## Implementation Plan

1. **`agent_profile_registry.rs`** — replace the placeholder
   `pub struct HookConfigShape;` with a real, `Copy`-able struct describing
   vendor hook-event -> turn-state pairs, e.g.
   `pub struct HookConfigShape { pub events: &'static [(&'static str, &'static str)] }`
   (event name, turn state — `[("UserPromptSubmit", "working"), ("Stop",
   "ready")]` for Claude, per the ticket's pinned three-state vocabulary and
   the closed spike's two verified events). Add a `CLAUDE_HOOK_CONFIG`
   const and set `CLAUDE_PROFILE.hook_config = Some(CLAUDE_HOOK_CONFIG)`.
   Leave `DUMMY_ECHO_PROFILE.hook_config` as `None` — no hooks for the
   test-only profile.

2. **New module `agent_hook_config.rs`** (daemon crate) — owns
   materialization I/O, separate from the pure vendor-neutral data in step 1
   (mirrors the existing `agent_env_profile.rs` data /
   `terminal_registry_file.rs` I/O split):
   - `pub fn materialize_hook_config(profile_dir: &Path, shape: &HookConfigShape, notify_binary: &Path, callback_path: &Path) -> io::Result<PathBuf>`.
   - Builds one JSON object merging every `shape.events` entry into a single
     `"hooks"` map (matching the spike's PROVEN shape exactly — same file
     registers multiple event keys), each hook's `"command"` string built as
     `<notify_binary> terminal-notify --callback <callback_path> --state <state>`,
     with `notify_binary`/`callback_path` quoted via a reused (or
     locally-duplicated, if the executor judges `claude_cli.rs`'s function
     private/non-importable) `shell_single_quote`-style helper so a
     space-containing state-dir path does not break the vendor CLI's shell
     parsing of the hook command.
   - `fs::create_dir_all(profile_dir)` (default permissions, no directory
     lockdown, per Codebase Findings).
   - Writes `profile_dir.join("settings.json")` using the SAME
     temp-write + `fs::set_permissions(0o600)` (`#[cfg(unix)]`) +
     `fs::rename` sequence as `terminal_registry_file::write_registry_entry`.
   - Returns the settings.json path for the caller to append as an argv
     value.

3. **New module `agent_callback.rs`** (daemon crate) — the shared piece Phase
   4 will extend, so both the daemon-write side (this phase) and the
   `terminal-notify` read side (this phase) share one JSON shape now instead
   of inventing two:
   - `#[derive(Deserialize)] #[serde(rename_all = "camelCase")] pub struct CallbackTarget { pub base_url: String, #[serde(default)] pub terminal_id: Option<String>, #[serde(default)] pub token: Option<String> }`
     — `base_url` required (its absence is the "resolution failed" case the
     ticket's verification cares about); `terminal_id`/`token` optional
     because Phase 4 has not landed yet (see design answer 1 below for what
     happens when they're absent).
   - `pub fn bound_base_url_path(state_dir: &Path) -> PathBuf` ->
     `state_dir.join("bound-base-url.json")` (sibling of `terminals/` and
     `agent-profiles/`, NOT inside `agent-profiles/` — see Codebase
     Findings on the GC-sweep scan-root conflict).
   - `pub fn write_bound_base_url(state_dir: &Path, base_url: &str) -> io::Result<()>`
     — same atomic-temp-rename + `0600`-on-Unix pattern, writing
     `{"baseUrl": "<base_url>"}`.
   - `pub fn resolve_callback_target(path: &Path) -> Result<CallbackTarget, ResolveError>`
     (a small local `enum ResolveError { Io(io::Error), Parse(serde_json::Error) }`
     with a `Display` impl bounded enough for a CLI stderr message) — reads
     the file fresh on every call, no caching of any kind. This function
     is the one both the "written after the config file" test and the real
     `terminal-notify` CLI use, so a later regression that adds caching
     anywhere in this path is caught by both.

4. **`cli.rs`** — add:
   ```rust
   #[command(hide = true)]
   TerminalNotify(TerminalNotifyArgs),
   ```
   and
   ```rust
   #[derive(Debug, Clone, Parser)]
   pub struct TerminalNotifyArgs {
       #[arg(long)]
       pub callback: std::path::PathBuf,
       #[arg(long, value_enum)]
       pub state: TurnStateArg,
   }
   #[derive(Clone, Copy, Debug, Eq, PartialEq, ValueEnum)]
   pub enum TurnStateArg { Working, Ready, Idle }
   ```
   plus a `terminal_notify_args(&self) -> Option<&TerminalNotifyArgs>`
   accessor mirroring `terminal_helper_args()` (`cli.rs:176-181`) exactly —
   non-consuming, checked before `into_serve_config`.

5. **`main.rs`** — add a dispatch branch mirroring the existing
   `terminal_helper_args()` branch (`main.rs:17-20`), calling a new
   `run_terminal_notify(args).await` (implemented in `agent_callback.rs` or
   a new `terminal_notify.rs`, executor's call — keep it small either way):
   - `resolve_callback_target(&args.callback)`; on `Err`, print a clear
     stderr message identifying WHICH failure (missing file vs. unparseable
     JSON vs. missing `baseUrl`) and return a non-zero exit via
     `anyhow::bail!` — fail loudly, per design answer 1.
   - If `terminal_id`/`token` are `None`, `anyhow::bail!` with a message
     naming the gap ("callback file has no terminalId/token — Phase 4 has
     not populated this callback target yet") rather than silently
     succeeding with nothing delivered.
   - Otherwise, POST to
     `format!("{base_url}/api/dashboard/terminals/{terminal_id}/turn-state")`
     with JSON body `{"token": token, "state": <lowercased state>}`
     (matching the pinned Decisions body shape exactly, so Phase 4 needs no
     client-side changes when it adds the receiving route). A
     connection/HTTP failure (expected in THIS phase, since the route does
     not exist yet) is reported to stderr and returns non-zero — this phase
     does not special-case "route not found yet" as success.

6. **`terminal.rs`** — thread the new value through:
   - `resolve_create_command` (`:984-1007`): widen the return type to a
     4-tuple, appending `profile.hook_config` in the `Some(id)` branch and
     `None` in the absent-`profile_id` branch (mirrors the existing
     Some/Some, None/None/None pairing exactly).
   - `create_terminal` (`:669-720`): destructure 4 values, pass the fourth
     into `TerminalSession::spawn`.
   - `TerminalSession::spawn` (`:1011-1051`): add a
     `hook_config: Option<HookConfigShape>` parameter. Immediately after
     `let id = opaque_terminal_id();` (L1028) and before
     `build_helper_command` (L1031):
     ```rust
     let mut command = command;
     if let (Some(hook_config), Some((_, args))) = (hook_config, command.as_mut()) {
         let state_dir = crate::persistent_state::default_state_dir()
             .unwrap_or_else(std::env::temp_dir);
         let profile_dir = state_dir.join("agent-profiles").join(&id);
         let callback_path = crate::agent_callback::callback_path(&profile_dir); // profile_dir.join("callback.json")
         match crate::agent_hook_config::materialize_hook_config(
             &profile_dir, &hook_config, &default_helper_binary(), &callback_path,
         ) {
             Ok(settings_path) => {
                 args.push("--settings".to_owned());
                 args.push(settings_path.display().to_string());
             }
             Err(error) => tracing::error!(
                 terminal_id = %id, %error,
                 "failed to materialize agent hook config; spawning without hooks"
             ),
         }
     }
     ```
     Materialization failure degrades to a hookless spawn (logged, not
     fatal) rather than failing terminal creation outright — turn-attention
     signaling is best-effort UX, not correctness-critical, so a local disk
     hiccup should not block the user from getting a terminal at all. Flag
     this choice explicitly for review; a reviewer preferring fail-closed
     here is a legitimate, cheap-to-apply disagreement.
   - `settings.json`'s hook command therefore references
     `agent-profiles/<terminal_id>/callback.json` — the exact Phase-4 file
     path — even though nothing writes that file until Phase 4 lands. A
     real hook fire against a Phase-3-only build will hit
     `resolve_callback_target`'s `Io` error branch (file not found) and
     fail loudly, which is correct and expected for an intentionally
     partial vertical slice (the ticket's own `## Phases` preamble: "Phases
     1-6 form one vertical slice"). Do not fabricate a stand-in
     callback.json writer in this phase to avoid that failure — that would
     be inventing Phase 4 scope under a different name.

7. **`server.rs`** — right after `let bound_addr = listener.local_addr()?;`
   (`:69`) and before `boot_reconcile` (`:99`), add:
   ```rust
   if let Some(state_dir) = crate::persistent_state::default_state_dir() {
       let base_url = format!("http://{}", display_addr(bound_addr));
       if let Err(error) = crate::agent_callback::write_bound_base_url(&state_dir, &base_url) {
           tracing::warn!(%error, "failed to write bound-base-url file");
       }
   }
   ```
   (reusing the existing `display_addr` helper already used by
   `startup_info` at `:157,161` for pairing-URL formatting, so the base URL
   string is byte-consistent with what the daemon already advertises).
   Non-fatal on write failure — mirrors how startup already tolerates a
   missing state dir elsewhere (`opened_work_roots` seeding is best-effort
   too).

## Verification Plan

- Unit tests, `agent_profile_registry.rs`: `claude` profile's `hook_config`
  is `Some` and contains both `UserPromptSubmit` and `Stop`; `dummy-echo`'s
  stays `None`.
- Unit tests, `agent_hook_config.rs`: `materialize_hook_config` writes a
  file whose parsed JSON has a `"hooks"` key with both event names present,
  each resolving to the SAME `terminal-notify --callback <path> --state
  <state>` structure the spike proved fires; on Unix, assert the written
  file's mode is exactly `0o600` (mirroring
  `terminal_registry_file.rs`'s own `:156-159` mode-assertion test) — this
  is the concrete test the ticket flags as NOT yet proven anywhere
  (`0600` was `0644` in the spike).
- Unit tests, `agent_callback.rs`:
  - `resolve_callback_target` on a well-formed fixture round-trips
    `baseUrl`/`terminalId`/`token`.
  - `resolve_callback_target` on a missing file and on malformed JSON both
    return distinct `Err` variants (not a panic, not a silent default).
  - **The load-bearing ordering test**: write a fixture with `baseUrl` A,
    call `resolve_callback_target`, assert A; REWRITE the same path with
    `baseUrl` B (simulating a daemon rebind on a different ephemeral port,
    happening strictly after the file's first write — the "config file"
    in the ticket's verification line is `settings.json`, written once at
    spawn and never touched again; this file is what changes AFTER that),
    call `resolve_callback_target` again, assert B. The concrete mutation
    this test is designed to catch: `resolve_callback_target` (or any
    future caller) memoizing the parsed value in a `OnceCell`/`static`
    instead of reading the file fresh on every invocation — that mutation
    would make the second assertion observe A instead of B and fail.
- Unit tests, `terminal.rs`: update the three existing
  `resolve_create_command` tests (`:2375,2384,2399`) for the new 4-tuple
  arity; add one asserting the `claude` branch's fourth element is
  `Some(...)` and the no-profile/`dummy-echo` branches are `None`.
- Integration test, new `crates/daemon/tests/terminal_notify.rs` (mirrors
  the `env!("CARGO_BIN_EXE_ws-dashboard")` subprocess pattern from
  `terminal_lifetime.rs:110`): bind a throwaway local `TcpListener` on an
  OS-assigned port as a MOCK callback receiver (no real daemon route
  needed — Phase 4 owns that), write a callback fixture JSON pointing
  `baseUrl` at that mock listener's address plus a dummy `terminalId`/
  `token`, run the real compiled `ws-dashboard terminal-notify --callback
  <fixture> --state ready` as a subprocess, and assert the mock listener
  received exactly one HTTP POST to
  `/api/dashboard/terminals/<terminalId>/turn-state` with the expected
  JSON body. This proves resolution AND delivery through the real CLI
  boundary without depending on any Phase-4 code.
- Integration test, `crates/daemon/tests/server.rs` (new test function,
  holding `persistent_state::ENV_LOCK` for its whole body per the existing
  contract on that mutex): under a temp `WS_DASHBOARD_STATE_HOME`, call
  `run_with_shutdown` twice in sequence (each on port `0`, so the OS
  assigns two DIFFERENT ephemeral ports), and after each run read back
  `bound-base-url.json` and assert its `baseUrl` matches that run's OWN
  returned `StartupInfo.bound_addr` — and that the second run's content is
  NOT the first run's content. This is the "rewrites on every bind" half
  of the ticket's verification line, proven against the real daemon
  startup path rather than the write function called directly.
- Full-crate regression, exact commands (capture exit status inline):

  ```
  cd ws-dashboard && cargo test -p ws-dashboard-daemon --lib > /tmp/hookcfg_lib.log 2>&1
  echo $?
  ```
  ```
  cd ws-dashboard && cargo test -p ws-dashboard-daemon --test terminal_lifetime > /tmp/hookcfg_lifetime.log 2>&1
  echo $?
  ```
  ```
  cd ws-dashboard && cargo test -p ws-dashboard-daemon --test terminal_notify > /tmp/hookcfg_notify.log 2>&1
  echo $?
  ```
  ```
  cd ws-dashboard && cargo test -p ws-dashboard-daemon --test server > /tmp/hookcfg_server.log 2>&1
  echo $?
  ```
  ```
  cd ws-dashboard && cargo check -p ws-dashboard-daemon --tests > /tmp/hookcfg_check.log 2>&1
  echo $?
  ```
- No browser/Playwright verification — this phase does not touch
  `frontend/` (see design answer 5).

## Escalations

- None.

---

## Design questions from the render brief, answered explicitly

1. **Bound-base-URL file: location, format, writer, and missing/stale/
   unreadable behavior.** Lives at `<state_dir>/bound-base-url.json`
   (sibling of `terminals/`, deliberately NOT inside `agent-profiles/` so
   Phase 4's GC sweep over per-terminal-id subdirectories never has to
   special-case a stray file). Format: `{"baseUrl": "http://<host>:<port>"}`,
   written by `server.rs` right after `listener.local_addr()` resolves, on
   every daemon start (unconditional overwrite — "every bind" means every
   process start, since the default port is ephemeral). This SAME
   resolution function/shape (`agent_callback::resolve_callback_target`) is
   reused for BOTH this global file (Phase 3, `baseUrl` only) and Phase 4's
   richer per-terminal `callback.json` (`baseUrl` + `terminalId` + `token`)
   — the struct's `terminal_id`/`token` fields are `#[serde(default)]`
   `Option`s so a Phase-3-era global file (missing those keys) parses
   successfully with `base_url` populated. A missing/unreadable/unparseable
   file makes `terminal-notify` **fail loudly** (non-zero exit, a stderr
   message naming which of "file not found" / "invalid JSON" / "missing
   baseUrl" occurred) rather than silently no-op — a silent no-op would let
   a broken notification pipeline look identical to "the agent simply
   hasn't finished a turn yet" from the user's perspective, with zero
   diagnostic trail. The daemon's OWN write of this file is best-effort
   (a warning log on failure, not a startup abort) since losing attention
   notifications is not as severe as failing to start the daemon at all.

2. **Lifecycle of `agent-profiles/<terminal_id>/`.** Creation only, in this
   phase: `fs::create_dir_all` at spawn time (inside `TerminalSession::spawn`,
   right after the terminal id is generated), default/umask permissions on
   the directory itself, `0600` on the `settings.json` file written inside
   it. Removal — both "on terminal close" and "on daemon restart with stale
   directories present" — is explicitly NOT this phase's job: the ticket
   assigns the GC sweep to Phase 4 and pins its ordering constraint ("must
   run strictly AFTER `boot_reconcile` completes"). This phase leaves
   orphaned `agent-profiles/<terminal_id>/` directories on disk whenever a
   terminal closes; that is a known, ticket-acknowledged, deliberately
   deferred gap, not an oversight of this plan.

3. **What the hook `command` invokes, and how it finds the `ws-dashboard`
   binary given Phase 1's env scrub.** The command string embeds the
   daemon's OWN resolved absolute binary path via `std::env::current_exe()`
   — the exact same resolution `default_helper_binary()` already uses for
   spawning the terminal helper (`terminal.rs:169-171`) — rather than a
   bare `ws-dashboard` name relying on `$PATH` lookup inside the spawned
   agent's (env-scrubbed) shell. Phase 1's scrub is a deny-list of specific
   `CLAUDE*`/`AI_AGENT` markers, not `PATH` itself, so a `$PATH`-relative
   invocation might often work by accident — but relying on that is
   fragile and untested, whereas embedding the absolute path removes the
   question entirely, matching existing precedent exactly.

4. **How the "written after" ordering is tested, and what mutation it
   catches.** Two tests, both described in `## Verification Plan`: a
   focused unit test that rewrites the SAME fixture path between two
   `resolve_callback_target` calls and asserts the second read wins (catches
   any memoization/caching regression in the resolver itself), and an
   integration test that runs the real `terminal-notify` binary against a
   fixture pointing at a mock HTTP receiver bound to a genuinely OS-assigned
   ephemeral port (catches a regression anywhere in the CLI's read-then-POST
   path, including one that bakes a URL at argument-parsing time instead of
   at request time). The concrete mutation both are designed to catch: an
   implementer "optimizing" by resolving `baseUrl` once and stashing it
   (a `OnceCell`, a value captured before a retry loop, or a URL baked into
   `settings.json`'s hook command STRING at materialization time instead of
   left in a file read at fire time) — any of these would make the
   first-observed value stick, and the rewrite-then-reassert test would
   catch it.

5. **Visible browser UI.** None. Phase 3 steps 2-3 are entirely daemon-side
   Rust (new CLI subcommand, new file-materialization modules, a threaded
   parameter through the existing spawn path). No file under `frontend/` is
   touched, so the mental model's mandatory browser-verification rule for
   visible-UI changes does not apply to this phase; the reviewer should
   confirm no `frontend/` diff exists rather than expect a Playwright step.
