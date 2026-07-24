---
title: Terminal PTY read pump corrupts UTF-8 multibyte sequences split across read() boundaries
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
the frontend sends input as JSON text already, but the same class of bug if
that path ever carries raw multi-byte-split bytes.

**Excludes:** ANSI/CSI escape-sequence splitting across reads is NOT a bug
(pure ASCII; xterm's parser resumes mid-sequence by design) — do not conflate
with this UTF-8 byte-corruption issue.

## Phases

### Phase 1: Carry incomplete trailing UTF-8 bytes across PTY reads

Maintain a small carry-over byte buffer across reads in the reader thread
(`spawn_reader_thread`): validate each read chunk with `str::from_utf8`,
detect an incomplete trailing multi-byte sequence, hold those tail bytes
back, and prepend them to the next `read()`'s buffer before decoding — so no
multi-byte codepoint is ever split across an emitted chunk. Only emit
complete, valid UTF-8 downstream.

Apply the same fix shape to the `write_input` lossy conversion at
`terminal.rs:945` if investigation during implementation confirms it can
receive raw byte-split input in practice.

Verification: add a daemon/helper-level test that feeds a multi-byte string
(e.g. Korean or emoji text) split across a read boundary and asserts the
reassembled output contains no U+FFFD replacement characters.
