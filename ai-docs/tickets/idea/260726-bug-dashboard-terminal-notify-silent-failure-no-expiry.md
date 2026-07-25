---
title: terminal-notify's deliberate silence has no expiry, no failure counter, and no reader anywhere
related:
  260725-feat-dashboard-pty-agent-attention-notification: found-during Phase 3 steps 2-3 review; all three findings here land on that ticket's Phase 4 boundary, which is why one ticket is the right container
---

# terminal-notify's deliberate silence has no expiry, no failure counter, and no reader anywhere

## Background

Phase 3 of `260725-feat-dashboard-pty-agent-attention-notification` added a
hidden `ws-dashboard terminal-notify` subcommand
(`ws-dashboard/crates/daemon/src/terminal_notify.rs`), invoked as a vendor hook
on every agent turn boundary. `run_terminal_notify` (`terminal_notify.rs:37-42`)
unconditionally returns `Ok(())` regardless of whether delivery succeeded, and
on failure appends one line to `logs/terminal-notify.log` under the daemon
state dir (`log_path`/`log_failure`, `terminal_notify.rs:72-98`) instead of
writing to stdout/stderr. The subcommand is hidden from `--help`
(`#[command(hide = true)]` at `cli.rs:34`, immediately above the
`TerminalNotify(TerminalNotifyArgs)` variant at `cli.rs:35`), and it is
dispatched in `main.rs` at lines 21-28 BEFORE `logging::init` runs (the
comment at `main.rs:21-24` states this is deliberate: "a short-lived,
hook-fired invocation with no server lifetime to instrument"), so nothing in
this path ever calls `tracing::`.

That silence was NOT arbitrary. The module's own header comment
(`terminal_notify.rs:6-30`) documents a real-PTY measurement: a non-zero exit
with stderr makes Claude Code surface a visible `<Event> hook error` line plus
a persistent "Stop hook error occurred" status-line indicator on every
`UserPromptSubmit` and every `Stop`, for as long as the callback file is
absent. The silent design is the correct call for the Phase 3-to-Phase 4
window and this ticket is not a request to revert it.

**The problem: nothing bounds or surfaces the silence.** Checked and confirmed
absent from the tree:

- No failure counter or consecutive-failure threshold anywhere in
  `terminal_notify.rs`, `agent_callback.rs`, or `agent_hook_config.rs` (grepped
  for `consecutive`/`failure_count` across the daemon crate — no hits).
- No daemon-side "this terminal has never posted" probe. `bound_base_url_path`
  / `write_bound_base_url` (`agent_callback.rs:61-63`, `74-92`) is written on
  every bind at `server.rs:81`, but grepping the whole daemon crate for
  `bound_base_url_path` turns up only its own definition, its writer, and its
  own module's tests (`agent_callback.rs:209-237`) — no production reader
  exists anywhere.
- No `tracing::` call in the module (confirmed by grep — zero hits in
  `terminal_notify.rs`), consistent with the dispatch-before-`logging::init`
  ordering above.
- Exit code is always `0` (`terminal_notify.rs:41`), so a process-exit-status
  monitor would see nothing either.
- No reader of the log file. `logs/terminal-notify.log` is referenced only in
  `terminal_notify.rs` (the writer) and its own test file
  (`crates/daemon/tests/terminal_notify.rs`); nothing else in the tree opens,
  tails, or surfaces it.
- Not documented: the hidden subcommand has no mention in `--help` (by design,
  `hide = true`) or in any doc.

**Consequence.** Once Phase 4 makes notifications real, a broken callback path
(daemon down, token mismatch, network failure, disk full, anything) becomes
indistinguishable from "the agent simply hasn't finished its turn" — from the
browser, from the terminal, and from the daemon log alike. The feature would
be silently dead with no signal anywhere a human would encounter it.

## Candidate Directions

Not decided here — listed for whoever picks this up:

- A daemon-side never-posted signal, using the now-unread `bound-base-url.json`
  write path as a precedent for what a small daemon-side liveness probe could
  look like.
- A bounded escalation: stay silent for N consecutive failures, then emit one
  visible signal (log line via `tracing`, or a one-time surfaced hook error)
  instead of silence forever.
- Phase 4 asserting end-to-end delivery in a test, so a regression fails a test
  run instead of failing a user's expectations silently.

## Related Finding 1: the 0600 write sequence is write-then-chmod, harmless today, load-bearing once a real credential travels it

Both `materialize_hook_config` (`agent_hook_config.rs:60-69`) and
`write_bound_base_url` (`agent_callback.rs:74-91`) share the same sequence:
`fs::write(&temp_path, raw)?` at umask-default permissions (typically
`~0644`), THEN `fs::set_permissions(&temp_path, ..0o600)` under `#[cfg(unix)]`,
THEN `fs::rename`. This mirrors the existing
`terminal_registry_file.rs:48-51` precedent (`#[cfg(unix)]`-gated
`set_permissions`, no Windows ACL equivalent, no runtime warning) and is
harmless for Phase 3's two secret-free files (`settings.json` has no secret;
`bound-base-url.json` carries only a base URL). It is a brief world-readable
window on Unix between the two calls, and on Windows the mode is never applied
at all.

If Phase 4's token-bearing `callback.json` (per the parent ticket's
`## Decisions`, "The token never touches the helper or the registry" —
`260725-feat-dashboard-pty-agent-attention-notification.md:87-101`) copies this
same write sequence, a genuine credential would be briefly world-readable on
Unix, and permanently unprotected-by-mode on Windows. This is acceptable for
secret-free files by the existing precedent, but Phase 4 should decide the
credential-bearing case explicitly (e.g., write directly at restrictive
permissions instead of write-then-chmod, or accept the window and record why)
rather than silently inherit the sequence from Phase 3.

## Related Finding 2: `bound-base-url.json` has no reader — and must not gain one that re-derives `baseUrl` from it

`bound-base-url.json` is written on every daemon bind (`server.rs:81`) but,
per Finding above, has zero production readers today. If Phase 4 derives
`callback.json`'s `baseUrl` field by reading this shared well-known file
instead of from the `bound_addr` the daemon already holds in memory, it
reintroduces exactly the multi-daemon steal the parent ticket's `## Decisions`
already rejected: "the state dir is per-user with no override flag
(`persistent_state.rs:495-510`), so a shared file would let a second
concurrent daemon silently steal every agent's callback target — and the
acceptance harness runs its own daemon"
(`260725-feat-dashboard-pty-agent-attention-notification.md:152-157`). Phase 4
must source `baseUrl` from the daemon's own in-memory bound address when
writing each terminal's `callback.json`, not by reading the shared file back.
