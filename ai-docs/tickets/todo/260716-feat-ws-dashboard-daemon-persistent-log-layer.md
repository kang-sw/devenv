---
title: "Persistent daemon log layer: durable rolling-file sink so detached daemons keep diagnosable logs"
sage-review-design: required
related:
  260714-bug-linked-terminal-ws-relay-502: "dependent — the 502 relay diagnosis needs this durable log sink to capture the real teardown cause"
---

# Persistent daemon log layer: durable rolling-file sink so detached daemons keep diagnosable logs

## Background

The daemon already wires `tracing`/`tracing-subscriber` (env-filter feature),
but only to **stderr**:

- `ws-dashboard/crates/daemon/src/main.rs:13` calls `logging::init(cli.log_filter())`.
- `ws-dashboard/crates/daemon/src/logging.rs:1-11` builds
  `tracing_subscriber::fmt().with_env_filter(...).try_init()` with no
  `.with_writer(...)` override, so events go to the `fmt` subscriber's default
  sink (stderr). No `tracing-appender`, no rolling-file writer, no file writer
  anywhere in the crate.
- Verbosity is controlled solely by the `--log-filter` CLI flag (default
  `"info"`, `cli.rs:11-12`). There is no `RUST_LOG` support
  (`EnvFilter::try_new`, not `from_default_env`), no `--log-file` flag, and no
  log-destination config field.

The operational reality is that this stderr stream is **discarded**: the
Windows-native dogfood daemon is launched with PowerShell
`Start-Process ... -WindowStyle Hidden` and no output redirection, and
`dev.sh run` foreground-execs `cargo run` with no redirection. Logs survive
only when a human happens to pipe the launch through `tee` for that one
session. So even when a bug is trivially reproducible (e.g. the linked-terminal
WebSocket relay drop, `260714-bug-linked-terminal-ws-relay-502`), there is no
durable record of what the daemon logged when it happened.

This ticket closes that gap once, as reusable diagnostic infrastructure, so
future investigations start from a durable log rather than a manual `tee`.

## Decisions

Approved design (owner-confirmed 2026-07-16):

- **Add a file sink alongside stderr, default-ON.** Keep the existing stderr
  `fmt` layer (so `dev.sh run` foreground output is unchanged) and add a second
  `fmt` layer writing to a file. Compose via `tracing_subscriber::registry()`
  with two layers rather than the single `fmt()` builder.
- **Rolling file via `tracing-appender`.** Daily rotation with bounded
  retention (`RollingFileAppender` / `max_log_files`), non-blocking writer
  (`tracing_appender::non_blocking`; hold the returned `WorkerGuard` for the
  process lifetime so buffered lines flush on shutdown).
- **Default location: the daemon state dir, `logs/daemon.log` (rotated to
  `daemon.log.YYYY-MM-DD`).** Reuse the existing persistent-state directory
  resolution (see `persistent_state.rs`) rather than inventing a new base path.
- **`--log-file <path>` override** on the serve subcommand to redirect the file
  sink; absent → the default state-dir location.
- **Human-readable text format** (timestamp + level + target + message), same
  family as the current stderr format, for greppability. JSON is a possible
  later option, out of scope here.
- **Keep `--log-filter` default `"info"`.** No change to level control in this
  ticket.
- **New dependency `tracing-appender`.** This is under `ws-dashboard/` (a
  downstream application tree), not `agents-plugin*`, so it does **not** trigger
  a plugin version bump.

## Constraints

- Do not regress the stderr sink — foreground `dev.sh run` must still print logs
  to the terminal.
- The file sink must not panic or abort daemon startup if the log directory
  cannot be created; degrade to stderr-only with a warning.
- The non-blocking `WorkerGuard` must outlive the server so shutdown flushes the
  tail of the log (a dropped guard silently discards buffered lines).

## Prior Art

- `logging::init` (`logging.rs:1-11`) — the single init point to extend.
- `persistent_state.rs` — existing daemon state-directory resolution to reuse
  for the default log path.
- `tracing`/`tracing-subscriber` already declared as workspace deps
  (`ws-dashboard/Cargo.toml:30-31`, `env-filter` feature).

## Phases

### Phase 1: Durable rolling-file sink, default-on, alongside stderr

Extend `logging::init` (and its call site / CLI) to add the file sink:

- Add `tracing-appender` to the daemon crate deps (workspace or crate-level).
- Resolve the default log path under the daemon state dir (`logs/daemon.log`);
  honor a new `--log-file <path>` serve flag when provided.
- Build a `registry()` with two `fmt` layers: the existing stderr layer plus a
  non-blocking daily-rolling file layer with bounded retention; return/hold the
  `WorkerGuard` for the process lifetime.
- Preserve `--log-filter` semantics (default `"info"`, applied via `EnvFilter`)
  across both layers.
- Fail-soft: if the log dir/file can't be opened, log a warning to stderr and
  continue with stderr-only.

Acceptance check:

- Starting the daemon (detached, no `tee`) creates the log file at the default
  path and events accumulate there; the file rotates on date change and old
  files are pruned to the retention bound.
- `dev.sh run` still prints logs to the foreground terminal (stderr sink intact).
- `cargo build -p ws-dashboard-daemon` and `cargo test -p ws-dashboard-daemon`
  pass.

Scope note: this ticket ships ONLY the generic log-sink infrastructure. The
relay/connect-path `tracing` instrumentation (per-direction teardown reason,
the `connect_remote_terminal_websocket` blanket-`Unavailable` map_err logging)
belongs to the dependent bug ticket `260714-bug-linked-terminal-ws-relay-502`,
which consumes this durable sink to capture the real cause.

### Result (999be0dd) - 2026-07-16

- Landed: `logging::init` rebuilt as a `tracing_subscriber::registry()` with
  two fmt layers — the existing stderr layer (unchanged, foreground
  `dev.sh run` still prints) plus a default-ON non-blocking daily-rolling
  file layer via `tracing-appender` 0.2 (default `<state_dir>/logs/daemon.log`,
  ~14-file retention). New `--log-file <path>` flag on `serve`; new
  `persistent_state::default_state_dir()` helper; `main.rs` holds the returned
  `WorkerGuard` for process lifetime; fail-soft to stderr-only (with warning)
  when the log dir/file can't be opened. `--log-filter` default `"info"`
  unchanged. New dep `tracing-appender` under `ws-dashboard/` only (no plugin
  version bump).
- Commits: implementation `2dba0901`, review-fix cycles `3bacdfee` (test
  coverage + fail-soft warning) and `999be0dd` (ENV_LOCK test-isolation +
  de-tautologized test); range `c9e11d70..999be0dd`. Docs: spec `557f37ab`
  (anchor `{#260716-dashboard-daemon-persistent-log-file-sink}`), mental-model
  `31d0a6e8`.
- Verification: `cargo build -p ws-dashboard-daemon` +
  `cargo test -p ws-dashboard-daemon` green (stable across 3 runs). New unit
  tests: `resolve_log_target` both branches, `init` happy-path file creation,
  `--log-file` clap parsing, `default_state_dir`, plus a crate-local
  `ENV_LOCK` serializing env-var-mutating tests.
- Review: correctness (opus) clean; test (sonnet) clean after 2 fix cycles;
  1 accepted Minor (`try_init` global-subscriber forward-looking note).
- DEFERRED (not done — requires owner action): live runtime acceptance was
  NOT performed because it needs a daemon restart (owner-gated). Still to
  verify by an owner-run step: that a detached daemon (no `tee`) actually
  creates the file at the default path, accumulates events, rotates on date
  change, and prunes to the retention bound.
- Forward: the dependent bug ticket `260714-bug-linked-terminal-ws-relay-502`
  can now add its relay/connect-path instrumentation on top of this durable
  sink.
