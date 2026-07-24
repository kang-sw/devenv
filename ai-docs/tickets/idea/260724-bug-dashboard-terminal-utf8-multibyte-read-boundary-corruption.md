---
title: Terminal PTY read pump corrupts UTF-8 multibyte sequences split across read() boundaries
spec:
  - 260516-ws-web-dashboard-terminal-io-transport
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
  prefix (`e.valid_up_to()`) via `append_output`, and move the remaining
  trailing bytes (`combined[e.valid_up_to()..]`) into the carry-over buffer
  for the next iteration — those bytes are the start of a multi-byte
  sequence that the next `read()` will complete.
- Bound the carry-over: a UTF-8 sequence is at most 4 bytes, so the
  carry-over never exceeds 3 bytes in the well-formed case. If a `read()`
  returns bytes whose prefix cannot be valid UTF-8 continuation (a
  genuinely malformed/non-UTF-8 stream, not just a split boundary), fall
  back to `String::from_utf8_lossy` for just that undecodable leading
  span so the pump can never wedge or grow the carry-over unbounded.
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

Also add a companion assertion/test note that legitimate EOF-truncated bytes
(a read boundary that is also the true end of the byte stream — genuinely
malformed/incomplete UTF-8, not a split boundary) still degrade to lossy
replacement rather than hanging or panicking, and that ANSI/CSI
escape-sequence splitting across reads is out of scope per the Background
excludes note (no test asserts on ANSI byte-splitting behavior — it was
already correct before this change and unaffected by it).
