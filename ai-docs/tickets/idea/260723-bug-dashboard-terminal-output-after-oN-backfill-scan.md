---
title: "dashboard terminal daemon: output_after does an O(N) ring scan + clone on every PTY output chunk"
related-mental-model:
  - ws-web-dashboard
---

## Symptom

CPU amplifier under continuous terminal streaming (progress spinners, `tail
-f`, a ConPTY prompt repaint). With N actively-streaming terminals this scales
to N hot reader-thread/socket-task pairs; on a 4-core box this can approach
all-cores-busy purely from output fan-out, not a spinlock. Surfaced during a
"Windows dashboard pegs 4 cores at 100%, invisible in Task Manager"
investigation.

## Finding

In `ws-dashboard/crates/daemon/src/terminal.rs`, each PTY read chunk flows
`spawn_reader` (~line 929) -> `append_output` (~line 700) ->
`output_signal.send` (watch) -> the socket task (`terminal_socket_task`,
~line 778) / `send_output_backfill` (~line 874) wakes and calls
`output_after()` (~line 607). `output_after` linearly scans and clones the
entire retained `VecDeque` (up to `MAX_OUTPUT_CHUNKS = 1024`, line 27) on
every wake:

```rust
fn output_after(&self, after: u64) -> TerminalOutputView {
    let inner = self.inner.lock().expect("terminal session lock poisoned");
    TerminalOutputView {
        terminal_id: self.id.clone(),
        status: inner.status,
        next_sequence: inner.next_sequence,
        chunks: inner
            .output
            .iter()
            .filter(|chunk| chunk.sequence > after)
            .cloned()
            .collect(),
    }
}
```

The result is then re-serialized and WS-sent. A shell/TUI that repaints
continuously makes each session's reader thread + socket task hot, and the
per-chunk cost is O(ring size) regardless of how few chunks are actually new.

Confirmed there is NO spinlock/busy-loop in the daemon's own Rust (reader
breaks on EOF, writer blocks on `recv()`); the "4" observed pegged is just
tokio's default `worker_threads = available_parallelism` on a 4-core machine
(`main.rs:4`, nothing hard-coded to 4). This O(N)-per-chunk backfill is the
prime application-layer amplifier IF the peg reproduces with terminals
actively streaming. If the peg reproduces with ZERO open terminals, the cause
is below this codebase (portable-pty 0.8 ConPTY pump or the runtime) - out of
scope for this ticket. Confidence: medium, pending the 0-vs-N-terminal
measurement.

## Fix direction (not decided)

- Avoid re-scanning/cloning the whole ring per wake: track a per-subscriber
  cursor index and slice only new chunks since last-sent sequence, instead of
  filtering the full `VecDeque` by sequence number every time.
- Coalesce output bursts (e.g. debounce/batch rapid successive chunks) before
  triggering a backfill send.

None of the above is decided; this ticket captures the finding for triage
pending measurement.

## Relation

Sibling to the frontend O(N) rerender idea tickets filed 260723
(`260723-bug-dashboard-terminal-frontend-output-oN-rerender`,
`260723-bug-dashboard-terminal-http-poll-oN-fallback`); this one is the
backend/daemon counterpart. The just-landed writer-thread fix
(`260723-bug-dashboard-terminal-blocking-pty-write-thread-starvation`) is
unrelated (write path, not output path).
