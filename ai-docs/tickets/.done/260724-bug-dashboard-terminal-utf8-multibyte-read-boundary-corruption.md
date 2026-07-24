---
title: Terminal PTY read pump corrupts UTF-8 multibyte sequences split across read() boundaries
spec:
  - 260516-ws-web-dashboard-terminal-io-transport
sage-review-design: completed
sage-review-completeness: completed
completed: 2026-07-24
---

# Terminal PTY read pump corrupts UTF-8 multibyte sequences split across read() boundaries

## Background

Dogfood observation (owner, 2026-07-24): intermittent garbled characters
("글씨 깨짐") appear in dashboard terminals, especially with non-ASCII
(Korean/CJK/emoji) output.

Root cause (this-session investigation, high confidence): the terminal
helper's PTY read pump decodes each raw `read()` independently with
`String::from_utf8_lossy`, using a fixed `[0u8; 4096]` buffer:

```rust
// ws-dashboard/crates/daemon/src/terminal_helper_process.rs:459-463
let mut buffer = [0_u8; 4096];
loop {
    match reader.read(&mut buffer) {
        Ok(0) => break,
        Ok(n) => shared.append_output(String::from_utf8_lossy(&buffer[..n]).into_owned()),
```

When a multi-byte UTF-8 codepoint straddles the 4096-byte boundary between
two separate `read()` calls, the trailing partial bytes of chunk 1 and the
leading partial bytes of chunk 2 are each individually invalid UTF-8 and get
replaced with U+FFFD — the original bytes are lost irrecoverably. There is no
byte-level carry-over buffer holding an incomplete trailing sequence until
the next read. This is a byte-exactness bug at the source; every downstream
hop (NDJSON IPC, JSON WS frame, xterm write) faithfully propagates the
already-corrupted replacement characters.

Secondary, same-pattern lossy step on the input path (lower likelihood,
worth noting): `ws-dashboard/crates/daemon/src/terminal.rs:945`,
`String::from_utf8_lossy(input)` in `write_input` — lower likelihood because
the frontend sends input as JSON text already, and the surrounding code
already documents this as a deliberate simplification for the WS
binary-frame path rather than an oversight (see `terminal.rs:938-944`).

**Excludes:** ANSI/CSI escape-sequence splitting across reads is NOT a bug
(pure ASCII; xterm's parser resumes mid-sequence by design) — do not conflate
with this UTF-8 byte-corruption issue.

## Spec Impact

`260516-ws-web-dashboard-terminal-io-transport` (and its
`260516-ws-web-dashboard-terminal-websocket-transport` sub-anchor) already
specifies that the terminal transport "carries ordered PTY output ... to the
browser" for daemon-owned PTY sessions. That contract is implicitly
byte-exact: an "ordered PTY output" stream that silently substitutes
replacement characters for bytes the PTY actually produced is not delivering
the output, it is delivering a corrupted derivative of it. This ticket is a
conformance bug fix restoring that existing contract rather than introducing
new caller-visible behavior, so no spec text changes — the corrected
behavior is: **terminal output bytes are delivered UTF-8-exact; a multi-byte
codepoint is never split or replaced with U+FFFD merely because it straddled
a PTY `read()` boundary.**

## Phases

### Phase 1: Carry incomplete trailing UTF-8 bytes across PTY reads

Fix `spawn_reader_thread` in
`ws-dashboard/crates/daemon/src/terminal_helper_process.rs` (~line 457) to
hold a small carry-over `Vec<u8>` across loop iterations instead of decoding
each raw `read()` in isolation:

- Before each `read()`, size the read so the carry-over bytes (from the
  previous iteration) are prepended to the newly read bytes before decoding
  — e.g. read into a scratch buffer, then build `[carry_bytes, new_bytes].concat()`
  (or reuse a single growable buffer that already contains the carry-over
  prefix and read into its remaining capacity).
- Decode the combined bytes with `std::str::from_utf8`. On `Ok`, emit the
  whole decoded string and clear the carry-over. On `Err(e)`, emit the valid
  prefix `combined[..e.valid_up_to()]` via `append_output`, then use
  `e.error_len()` as the precise discriminator for what follows that prefix
  — this is Rust's canonical signal for "incomplete tail" vs. "malformed
  span", and reading it literally is what prevents an unbounded carry-over
  wedge:
  - `error_len() == None`: the bytes from `valid_up_to()` to the end of
    `combined` are an INCOMPLETE multi-byte sequence at end-of-buffer (a
    genuine read-boundary split, not corruption) → carry
    `combined[e.valid_up_to()..]` over to prepend to the next `read()`, and
    stop processing this chunk (nothing left to loop over).
  - `error_len() == Some(n)`: the `n` bytes starting at `valid_up_to()` are
    a GENUINELY MALFORMED span, not a split boundary → emit exactly those
    `n` bytes via `String::from_utf8_lossy` (yields exactly one U+FFFD),
    do NOT carry them over, then continue decoding the remainder
    `combined[e.valid_up_to() + n..]` by looping `str::from_utf8` again on
    that remainder (repeating the same `Ok`/`Err`/`error_len()` dispatch).
    This loop-and-advance-past-the-malformed-span step is what stops the
    same byte offset from failing every iteration and wedging the pump.
- Because `error_len() == None` is the only branch that carries bytes
  forward, and a UTF-8 sequence is at most 4 bytes, the carry-over is
  bounded to at most 3 bytes at all times — it can never grow unbounded,
  since the `Some(n)` branch always consumes and advances past its
  malformed span within the same iteration instead of carrying it.
- On `Ok(0)` (EOF/shell exit), flush any remaining carry-over bytes with
  `String::from_utf8_lossy` before breaking the loop — those bytes are
  genuinely truncated at EOF, not merely split across two reads, so lossy
  replacement is the correct terminal behavior there (matches existing
  `Ok(0) => break` shutdown path).

**Input path (`terminal.rs:945`) is out of scope for this Phase.** The
existing code comment there already documents the lossy conversion as a
deliberate simplification for the WS binary-frame path, since the primary
input paths carry JSON-text input already (not raw split bytes). Revisit
only if a future ticket demonstrates raw byte-split input actually reaches
that path in practice.

Verification: add a unit test in `terminal_helper_process.rs` (alongside the
existing `ring_state_tests` module) that drives `spawn_reader_thread`
directly with a fake `Read` implementation returning two chunks split
mid-codepoint — e.g. a Korean or emoji string (multi-byte UTF-8) where the
first `read()` call returns bytes ending partway through one codepoint and
the second `read()` call returns the remaining bytes followed by `Ok(0)`.
After the reader thread finishes, read back the accumulated output via the
shared `RingState`'s `backfill_after(0)` and assert the reassembled text:

- equals the original multi-byte string exactly, and
- contains no `\u{FFFD}` (U+FFFD replacement character).

Test-harness notes for the implementer (not behavioral, but avoids surprise):
`spawn_reader_thread` discards its `JoinHandle` and signals completion only
by transitioning the shared `RingState.status` to `Exited` — the test has no
handle to `.join()`, so it must poll `status_and_next_sequence()` (or an
equivalent status read) until it observes `Exited` before reading back
output, or `spawn_reader_thread` may be refactored to return the
`JoinHandle` for a direct join. This test is also the first one in this
module to construct a full `SharedState` literal (every field, including the
`ring`/`notify`/`child`/`master`/`writer_tx` mutexes and the `#[cfg(windows)]
job` field) rather than a bare `RingState` — existing `ring_state_tests`
only ever construct `RingState` directly.

Also add these companion cases:

- Legitimate EOF-truncated bytes (a read boundary that is also the true end
  of the byte stream — genuinely malformed/incomplete UTF-8, not a split
  boundary) still degrade to lossy replacement rather than hanging or
  panicking.
- Malformed-interior-byte fallback (the `error_len() == Some(n)` branch
  above): feed a stream containing a genuinely invalid byte
  mid-stream — not a clean split-at-buffer-end and not EOF truncation, e.g.
  a lone continuation byte or invalid leading byte sandwiched between two
  valid ASCII/multi-byte spans within a single `read()` chunk — and assert
  that (a) the output contains exactly one U+FFFD for that malformed span,
  (b) the carry-over does not grow unbounded (the pump keeps advancing
  rather than re-failing at the same offset), and (c) the valid text before
  and after the malformed span still decodes correctly and reaches the
  ring unmodified.
- ANSI/CSI escape-sequence splitting across reads is out of scope per the
  Background excludes note (no test asserts on ANSI byte-splitting
  behavior — it was already correct before this change and unaffected by
  it).

### Result (0741e94f) - 2026-07-24

- `spawn_reader_thread` now holds a bounded carry-over `Vec<u8>` (≤3 bytes,
  since a UTF-8 sequence is at most 4 bytes) across `read()` boundaries
  instead of decoding each raw chunk in isolation. Carry-over bytes are
  prepended to each new read before decoding with `std::str::from_utf8`.
- `Ok`: the whole combined buffer decodes cleanly, emit it and clear the
  carry-over.
- `Err(e)` with `error_len() == None`: the tail from `valid_up_to()` onward
  is an incomplete trailing sequence (genuine read-boundary split) — the
  valid prefix is emitted via `append_output` and the incomplete tail is
  carried over to prefix the next `read()`.
- `Err(e)` with `error_len() == Some(n)`: the `n`-byte span at
  `valid_up_to()` is genuinely malformed — exactly one U+FFFD is emitted for
  that span via `String::from_utf8_lossy`, and decoding resumes past the
  span within the same iteration (loop-and-advance), so the carry-over stays
  bounded and the pump never wedges re-failing at the same offset.
- `Ok(0)` (EOF): any remaining carry-over is flushed lossily via
  `String::from_utf8_lossy` before the loop breaks, matching the existing
  shutdown path.
- `spawn_reader_thread` now returns its `JoinHandle` (previously discarded)
  so tests can join it directly instead of polling `RingState.status` for
  `Exited`.
- Added 4 regression-guard unit tests alongside `ring_state_tests`: split
  mid-codepoint across two reads, a cross-read case followed by clean EOF,
  EOF-truncation degrading to lossy replacement without hanging, and a
  malformed-interior-byte span emitting exactly one U+FFFD while surrounding
  valid text still decodes correctly. 3 of the 4 were proven to fail against
  the pre-fix code (reverting the fix reproduces double-U+FFFD corruption or
  a hang/wedge), confirming the tests are regression guards and not
  vacuously passing.
- `cargo test -p ws-dashboard-daemon` green. Correctness and test-quality
  reviews both clean (correctness: clean + 1 minor, addressed; test-quality
  findings fixed and regression-guard-verified).
- Input path (`terminal.rs:945`, `write_input`'s `String::from_utf8_lossy`)
  remains explicitly out of scope per the Phase 1 plan; unchanged by this
  fix.
