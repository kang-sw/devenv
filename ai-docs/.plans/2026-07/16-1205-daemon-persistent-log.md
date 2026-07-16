# Plan: 260716-feat-ws-dashboard-daemon-persistent-log-layer — Phase 1: Durable rolling-file sink, default-on, alongside stderr

## Relevant Ticket Contract

- Add a file sink alongside stderr, default-ON; keep the existing stderr `fmt`
  layer unchanged (`dev.sh run` foreground output must not regress) and compose
  both under `tracing_subscriber::registry()` instead of the single `fmt()`
  builder.
- Rolling file via `tracing-appender`: daily rotation, bounded retention
  (`max_log_files`), non-blocking writer; hold the returned `WorkerGuard` for
  the process lifetime so buffered lines flush on shutdown.
- Default location: daemon state dir, `logs/daemon.log` (rotated to
  `daemon.log.YYYY-MM-DD`); reuse the existing persistent-state directory
  resolution rather than inventing a new base path.
- New `--log-file <path>` flag on the `serve` subcommand overrides the file
  sink path; absent → default state-dir location.
- Human-readable text format (timestamp + level + target + message), same
  family as current stderr format.
- Keep `--log-filter` default `"info"` — no level-control changes.
- Fail-soft: if the log dir/file cannot be opened, warn on stderr and continue
  stderr-only; must not panic or abort startup.
- New dependency `tracing-appender`, added under `ws-dashboard/` only — no
  plugin version bump required.
- Acceptance: default-path file created/rotated/pruned on detached start;
  `dev.sh run` still prints to foreground stderr; `cargo build -p
  ws-dashboard-daemon` and `cargo test -p ws-dashboard-daemon` pass.

## Out of Scope

- Relay/connect-path `tracing` instrumentation (per-direction teardown reason,
  `connect_remote_terminal_websocket` map_err logging) — belongs to dependent
  bug ticket `260714-bug-linked-terminal-ws-relay-502`.
- JSON log format (explicitly deferred, "possible later option").
- `RUST_LOG` / `EnvFilter::from_default_env` support, or any `--log-filter`
  default change.
- Any change to `WS_DASHBOARD_STATE_FILE`/`WS_DASHBOARD_STATE_HOME` semantics
  themselves — only reuse of their resolution logic for a sibling `logs/` dir.

## Codebase Findings

- `ws-dashboard/crates/daemon/src/logging.rs#L1-L11` — current single-sink
  init: `tracing_subscriber::fmt().with_env_filter(env_filter).try_init()`,
  returns `anyhow::Result<()>`. This is the sole extension point; no
  `tracing-appender`, no file writer anywhere in the crate today.
- `ws-dashboard/crates/daemon/src/main.rs#L1-L15` — call site: `logging::init(cli.log_filter())?;`
  runs before `cli.into_serve_config()?` (which consumes `cli`). A new
  `--log-file` accessor on `Cli` must be read before that consuming call, and
  `main` must bind the new return value to a named local (not `_`) so the
  `WorkerGuard` lives through `server::run(...).await`.
- `ws-dashboard/crates/daemon/src/cli.rs#L1-L86` — `log_filter: String` lives on
  top-level `Cli` (shared across subcommands incl. `--remote-guide`), default
  `"info"`, parsed via `EnvFilter::try_new` (not `from_default_env`, matches
  ticket's "no `RUST_LOG` support" constraint — preserve). `ServeArgs`
  (`#L23-L49`) is the `serve`-subcommand-scoped flag struct where `--log-file`
  belongs (pattern: `#[arg(long)] pub static_dir: Option<std::path::PathBuf>`
  at `#L47-L48` is the direct analog for an `Option<PathBuf>` flag). `Cli` needs
  a new accessor mirroring `pub fn log_filter(&self) -> &str` (`#L63-L65`) that
  peeks `self.command.as_ref()` for `Some(Command::Serve(args)) => args.log_file.as_deref()`
  without consuming `self` (since `into_serve_config` at `#L75-L85` consumes
  `self.command` afterward).
- `ws-dashboard/crates/daemon/src/config.rs#L1-L46` — `ServeConfig::from_args`
  is the existing pattern for threading a new `ServeArgs` field into
  `ServeConfig` if the log path needs to survive into `server.rs`; not
  required for Phase 1 since `logging::init` runs in `main.rs` before
  `ServeConfig` exists, and the log path has no runtime dependency on bind
  config.
- `ws-dashboard/crates/daemon/src/persistent_state.rs#L485-L503` — `fn
  default_state_file() -> Option<PathBuf>` (module-private) resolves
  `WS_DASHBOARD_STATE_FILE` → `WS_DASHBOARD_STATE_HOME`/`opened-workroots.json`
  → `XDG_STATE_HOME`/`ws-dashboard/opened-workroots.json` →
  `HOME/.local/state/ws-dashboard/opened-workroots.json` → (Windows)
  `LOCALAPPDATA/ws-dashboard/opened-workroots.json` → `None`. It returns a
  **file** path, not a directory — the "state dir" is this file's parent. The
  function is private and returns no dedicated directory accessor today, so
  Phase 1 needs a small new pub helper here (e.g. `pub fn default_state_dir()
  -> Option<PathBuf>` returning `default_state_file()?.parent().map(Path::to_path_buf)`)
  for `logging.rs` to build `<state_dir>/logs/daemon.log` from. This is a
  narrow visibility/helper addition, not a design change — resolution order is
  reused verbatim.
- `ws-dashboard/crates/daemon/src/lib.rs#L1-L24` — both `pub mod logging;`
  (`#L13`) and `pub mod persistent_state;` (`#L15`) are already public crate
  modules; no visibility changes needed at the module level, only the new pub
  fn inside `persistent_state.rs`.
- `ws-dashboard/Cargo.toml#L15-L34` (`[workspace.dependencies]`) —
  `tracing-subscriber = { version = "0.3", features = ["env-filter"] }`
  (`#L31`); `fmt` and `registry` are tracing-subscriber 0.3 **default**
  features (not currently disabled anywhere), so no feature-flag change is
  needed to use `tracing_subscriber::registry()` + `tracing_subscriber::fmt::layer()`.
  Add `tracing-appender = "0.2"` (current stable per `cargo search
  tracing-appender` → `tracing-appender = "0.2.5"`) as a new workspace dep.
- `ws-dashboard/crates/daemon/Cargo.toml#L11-L29` — all daemon deps are
  declared via `<name>.workspace = true`; add `tracing-appender.workspace =
  true` here to match the existing pattern (`tracing.workspace = true`,
  `tracing-subscriber.workspace = true` at `#L25-L26`).
- `tracing-appender` 0.2.x API (targeted, matches `tracing-subscriber = "0.3"`):
  `tracing_appender::rolling::RollingFileAppender::builder()` →
  `.rotation(Rotation::DAILY)` → `.max_log_files(n)` → `.filename_prefix(...)` /
  `.filename_suffix(...)` → `.build(directory)` (builder path is required to
  get `max_log_files`; the plain `rolling::daily(dir, prefix)` constructor has
  no retention bound). `tracing_appender::non_blocking(writer) -> (NonBlocking,
  WorkerGuard)`. `WorkerGuard` has no public constructor and is not `Clone`; it
  must be returned out of `logging::init` and held by the caller — dropping it
  early silently stops the flush and can drop buffered lines.
- Risk signal — **return-contract change**: `logging::init` currently returns
  `anyhow::Result<()>` (`logging.rs#L3`). Phase 1 must change this to return
  something that lets `main.rs` keep the file-layer `WorkerGuard` alive for
  the whole process (e.g. `anyhow::Result<Option<WorkerGuard>>`, `None` in the
  fail-soft stderr-only path). `main.rs#L13` must change from
  `logging::init(cli.log_filter())?;` to a binding that keeps the guard alive,
  e.g. `let _guard = logging::init(...)?;` — using a bare `_guard` name (not
  literal `_`) is required, since `_` drops immediately and would silently
  disable the file sink's non-blocking flush.
- Risk signal — **two independent `EnvFilter` instances needed**: composing
  two `fmt` layers under one `registry()` with per-layer filtering
  (`layer.with_filter(env_filter)`, via `EnvFilter`'s `Filter<S>` impl) needs
  two separate `EnvFilter::try_new(filter)` calls (one per layer) built from
  the same filter string, since a single `EnvFilter` is consumed per
  `.with_filter()` call site. Confirm this compiles with `tracing-subscriber`
  0.3's registry/layer/filter feature set already enabled by the crate's
  current `env-filter` feature flag — no new feature needed.

## Implementation Plan

1. `ws-dashboard/Cargo.toml` — add `tracing-appender = "0.2"` to
   `[workspace.dependencies]`, alongside the existing `tracing`/
   `tracing-subscriber` lines (`#L30-L31`).
2. `ws-dashboard/crates/daemon/Cargo.toml` — add `tracing-appender.workspace =
   true` under `[dependencies]` (`#L25-L26` area).
3. `ws-dashboard/crates/daemon/src/persistent_state.rs` — add `pub fn
   default_state_dir() -> Option<PathBuf>` next to `default_state_file()`
   (`#L485-L503`), returning `default_state_file()?.parent().map(Path::to_path_buf)`.
   No change to existing env-var resolution order or `default_state_file`
   itself.
4. `ws-dashboard/crates/daemon/src/logging.rs` — rewrite `init` to:
   - Accept a `filter: &str` and `log_file_override: Option<PathBuf>`.
   - Resolve the effective log path: override if `Some`, else
     `persistent_state::default_state_dir()` joined with `logs/daemon.log`
     (split into a directory for `RollingFileAppender::builder().build(dir)`
     plus a `daemon` filename prefix/`.log` suffix so rotated files land at
     `daemon.log.YYYY-MM-DD` per the ticket's stated rotated-name shape).
   - Build the stderr layer (`tracing_subscriber::fmt::layer()` with default
     writer) with its own `EnvFilter::try_new(filter)?`.
   - Try to build the file layer (`RollingFileAppender::builder().rotation(Rotation::DAILY).max_log_files(N).build(dir)`,
     then `tracing_appender::non_blocking(appender)`); on any error (dir
     create failure, etc.), `eprintln!`/`tracing::warn!`-equivalent (must use
     plain `eprintln!` before the subscriber is installed) and continue
     stderr-only.
   - Compose via `tracing_subscriber::registry().with(stderr_layer.with_filter(stderr_env_filter))`
     and, when the file layer built successfully,
     `.with(file_layer.with_ansi(false).with_filter(file_env_filter)).try_init()`;
     otherwise `.try_init()` with only the stderr layer.
   - Return `anyhow::Result<Option<tracing_appender::non_blocking::WorkerGuard>>`
     — `Some(guard)` when the file sink is active, `None` in the fail-soft
     path.
   - Pick a bounded retention constant (e.g. `max_log_files(14)`) since the
     ticket only specifies "bounded retention", not an exact number.
5. `ws-dashboard/crates/daemon/src/cli.rs` — add `#[arg(long, help = "Override
   the daemon log file path (default: state dir logs/daemon.log)")] pub
   log_file: Option<std::path::PathBuf>` to `ServeArgs` (`#L23-L49`, alongside
   `static_dir`), and add a non-consuming accessor on `Cli`, e.g. `pub fn
   log_file(&self) -> Option<&std::path::Path>` that matches
   `self.command.as_ref()` for `Some(Command::Serve(args)) =>
   args.log_file.as_deref()`, `_ => None` (mirrors `log_filter()` at
   `#L63-L65`, added before `into_serve_config` at `#L75-L85` since that
   consumes `self.command`).
6. `ws-dashboard/crates/daemon/src/main.rs#L13` — change to `let _guard =
   logging::init(cli.log_filter(), cli.log_file().map(Path::to_path_buf))?;`
   (named binding, not literal `_`) before `cli.into_serve_config()?` is
   called, so the guard is read out while `cli.command` is still intact.
7. Add a focused unit test in `logging.rs` (or alongside) for the fail-soft
   path: pointing the log dir at a location that cannot be created (e.g. a
   path under a file, not a directory) and asserting `init` still returns
   `Ok(None)` / succeeds with stderr-only rather than erroring. Add a
   `cli.rs` help-discoverability test for `--log-file` following the existing
   `serve_no_auth_flag_is_discoverable_from_help` pattern (`#L157-L166`).

## Verification Plan

- `cargo build -p ws-dashboard-daemon` (ticket acceptance check).
- `cargo test -p ws-dashboard-daemon` (ticket acceptance check; covers the new
  fail-soft unit test and the `--log-file` help-discoverability test).
- Manual-only (not required for Phase 1 automated gate, but matches ticket
  acceptance intent): start the daemon detached with no `tee`, confirm
  `logs/daemon.log` appears under the resolved state dir and accumulates
  events; `dev.sh run` foreground output still shows log lines on stderr.

## Escalations

- None.
