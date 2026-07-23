---
title: "dashboard terminal daemon: output_after does an O(N) ring scan + clone on every PTY output chunk"
related-mental-model:
  - ws-web-dashboard
sage-review-design: completed
sage-review-completeness: completed
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

The below `## Phases` section decides and scopes the concrete fix for Phase 1;
this section is retained as the original triage-time brainstorm.

## Phases

### Phase 1: Replace O(N) output_after scan with index arithmetic and drop the redundant status rescan

**Change 1 — `output_after` (`terminal.rs` ~607-620):** Chunks retained in
`inner.output` have strictly contiguous, gapless, increasing `sequence`
(`append_output` increments `next_sequence` exactly once per push; eviction
only ever does `pop_front`). Replace the
`.iter().filter(|c| c.sequence > after).cloned().collect()` scan with index
arithmetic: compute `front_seq = inner.output.front().map(|c| c.sequence)`
(no front → nothing retained); then
`skip = after.saturating_add(1).saturating_sub(front_seq).min(len) as usize`
(0 when there is no front); then
`inner.output.iter().skip(skip).cloned().collect()`. This drops the cost from
O(N) to O(1) + O(K) (K = chunks actually returned) and is byte-identical to
the old filter for every `after`, by the gapless-contiguous-sequence
invariant. Add a CONTRACT comment on `output_after` noting the optimization
depends on that invariant (gapless contiguous sequence + front-only
eviction) and must be revisited if either changes.

**Change 2 — `send_terminal_socket_status` (`terminal.rs` ~893-916):** Add a
small private accessor `status_and_next_sequence(&self) -> (TerminalStatus,
u64)` that locks `inner` once and returns only those two scalars — it must
never touch `inner.output`. Call it at ~899 instead of the current
`output_after(u64::MAX)`, which eliminates a second unconditional full-ring
scan + clone + allocation on every wake purely to read `status` and
`next_sequence`.

**Constraints on the completion boundary:**

- No public signature or struct-field changes.
- Other `output_after`/`is_range_truncated` call sites are unaffected and
  stay unchanged: HTTP `terminal_output` (~line 432), `plan_output_backfill`
  (~line 863), `is_range_truncated` (~line 628).
- Use saturating arithmetic throughout (`saturating_add`, `saturating_sub`);
  clamp `skip` to `len` so it can never index past the deque.
- Fully behavior-preserving: identical bytes, order, JSON wire shape,
  resume-by-cursor semantics, and truncation semantics as today.
- Spec Impact: None — internal perf refactor, no caller-visible behavior
  change.

**Verification (Phase 1 completion boundary):**

- Add a unit test asserting the new skip-based `output_after` is equal to the
  old `filter(seq > after)` semantics across representative `after` values,
  on a deque pushed past `MAX_OUTPUT_CHUNKS` (eviction forced): before-window,
  at-boundary, mid-window, at-next_sequence, and near-`u64::MAX` — identical
  in-order `Vec` for each case.
- Existing `is_range_truncated_*` and `plan_output_backfill_*` tests must keep
  passing unmodified.
- `cargo build --workspace` and `cargo test -p ws-dashboard-daemon` both
  green.

## Relation

Sibling to the frontend O(N) rerender idea tickets filed 260723
(`260723-bug-dashboard-terminal-frontend-output-oN-rerender`,
`260723-bug-dashboard-terminal-http-poll-oN-fallback`); this one is the
backend/daemon counterpart. The just-landed writer-thread fix
(`260723-bug-dashboard-terminal-blocking-pty-write-thread-starvation`) is
unrelated (write path, not output path).
