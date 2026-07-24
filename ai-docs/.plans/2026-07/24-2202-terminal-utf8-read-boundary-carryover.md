# Plan: 260724-bug-dashboard-terminal-utf8-multibyte-read-boundary-corruption — Phase 1

## Relevant Ticket Contract
- Fix `spawn_reader_thread` (`ws-dashboard/crates/daemon/src/terminal_helper_process.rs`) to hold a bounded (<=3 byte) carry-over `Vec<u8>` across loop iterations instead of decoding each raw `read()` independently.
- Decode `[carry, new_bytes].concat()` with `std::str::from_utf8`. Dispatch on the `Err`:
  - `error_len() == None` (incomplete tail at buffer end): emit `combined[..valid_up_to()]`, carry `combined[valid_up_to()..]` forward, stop processing this chunk.
  - `error_len() == Some(n)` (malformed span, not a split): emit `combined[..valid_up_to()]`, then emit exactly `combined[valid_up_to()..valid_up_to()+n]` via `String::from_utf8_lossy` (one U+FFFD), do NOT carry it, loop `str::from_utf8` again on `combined[valid_up_to()+n..]`.
- On `Ok(0)` (EOF), flush any remaining carry-over via `String::from_utf8_lossy` before breaking.
- Add a unit test alongside the existing test modules in this file, driving `spawn_reader_thread` with a fake `Read` returning chunks split mid-codepoint, then reading back output via `RingState::backfill_after(0)` and asserting exact reassembly + no U+FFFD.
- Companion cases required: (a) EOF-truncated bytes still degrade to lossy replacement without hanging/panicking, (b) a malformed interior byte mid-stream yields exactly one U+FFFD, does not wedge the carry-over, and text before/after the malformed span survives intact.
- Input path (`terminal.rs:945`) is explicitly out of scope for this phase.
- ANSI/CSI escape-sequence splitting is explicitly out of scope (was already correct; no test needed for it).

## Out of Scope
- `ws-dashboard/crates/daemon/src/terminal.rs:945` `write_input`'s `String::from_utf8_lossy` on the input path — ticket explicitly excludes it (documented deliberate simplification, JSON-text input dominates that path).
- ANSI/CSI escape-sequence splitting across reads — pre-existing correct behavior (pure ASCII, xterm resumes mid-sequence), not this bug's pattern; no test asserts on it.
- Any change to `RingState`, `TerminalHelperOutputChunk`, the NDJSON/IPC framing, or daemon-side (`terminal.rs`) output handling — the fix and its test are confined to the PTY-side read pump in `terminal_helper_process.rs`.
- Windows-specific reader-thread behavior beyond what already compiles cross-platform (the fix touches the reader loop body only, not the `#[cfg(windows)]` job/reaper wiring).

## Codebase Findings
- `ws-dashboard/crates/daemon/src/terminal_helper_process.rs#L499-514` — current `spawn_reader_thread`. **Line-number drift from the ticket**: ticket text cites `~line 457-463`; actual current lines are `499-514` (file grew from prior unrelated changes). Content matches the ticket's description exactly otherwise: `let mut buffer = [0_u8; 4096];` loop, `Ok(0) => break`, `Ok(n) => shared.append_output(String::from_utf8_lossy(&buffer[..n]).into_owned())`, `Err(_) => { shared.transition(Error); return; }`, then `shared.transition(Exited)` after the loop. Function currently returns `()`.
- `ws-dashboard/crates/daemon/src/terminal_helper_process.rs#L476` — sole call site: `spawn_reader_thread(shared, reader);` inside `spawn_shell`, as a bare statement whose return value is already discarded. Changing the return type to `thread::JoinHandle<()>` requires no edit here — a discarded non-`#[must_use]` return value compiles without warning (`JoinHandle` is not `#[must_use]`).
- `ws-dashboard/crates/daemon/src/terminal_helper_process.rs#L75-94` — `SharedState` struct definition, the exact field list a test must construct (see below).
- `ws-dashboard/crates/daemon/src/terminal_helper_process.rs#L665-679` (`mod kill_path_guard_tests`, function `shared_state_for_test()`) — **corrects a stale premise in the ticket's test-harness notes.** The ticket claims the new test "is also the first one in this module to construct a full `SharedState` literal." This is factually outdated: `kill_path_guard_tests` already has exactly this helper, constructing every field including the `#[cfg(windows)] job` field:
  ```rust
  fn shared_state_for_test() -> SharedState {
      SharedState {
          pid: 0,
          start_time: 0,
          ring: Mutex::new(RingState::new()),
          notify: Notify::new(),
          child: Mutex::new(None),
          master: Mutex::new(None),
          writer_tx: Mutex::new(None),
          shell_started: AtomicBool::new(false),
          exited_at: Mutex::new(None),
          #[cfg(windows)]
          job: Mutex::new(None),
      }
  }
  ```
  This lowers implementation risk: there is a proven, working construction pattern to copy rather than a genuinely novel one. It is a private `fn` inside a private (non-`pub`) `mod`, so it is not directly callable from a new sibling test module — duplicate a same-shaped helper in the new module rather than widening visibility on the existing one (keeps the two test modules independently self-contained, matching the existing style where `ring_state_tests` and `kill_path_guard_tests` do not share helpers either).
- `ws-dashboard/crates/daemon/src/terminal_helper_process.rs#L166-169` — `status_and_next_sequence()` returns `(TerminalHelperStatus, u64)` reading `ring.status`; usable for a poll-based sync if the JoinHandle-return refactor is rejected, but see recommendation below.
- `ws-dashboard/crates/daemon/src/terminal_helper_protocol.rs#L19-24` — `TerminalHelperStatus` enum: `Running`, `Exited`, `Terminated`, `Error`.
- `ws-dashboard/crates/daemon/src/terminal_helper_protocol.rs#L28-31` — `TerminalHelperOutputChunk { pub sequence: u64, pub data: String }` — what `backfill_after` returns; the test reassembles output by concatenating `.data` across the returned `Vec` in order.
- `ws-dashboard/crates/daemon/src/terminal_helper_process.rs#L62-68` — `RingState::backfill_after(after: u64) -> Vec<TerminalHelperOutputChunk>`, the exact call the ticket's verification section names (`backfill_after(0)`).
- `ai-docs/mental-model/ws-web-dashboard/terminal.md#L34` — confirms the input-path lossy conversion is a documented deliberate simplification (matches ticket's out-of-scope framing for `terminal.rs:945`); no drift here.
- `ws-dashboard/crates/daemon/Cargo.toml#L2` — crate name `ws-dashboard-daemon`, confirming the verification command's package flag.

## Decision: JoinHandle-return vs. status-poll for test synchronization
**Recommend refactoring `spawn_reader_thread` to return `thread::JoinHandle<()>`** and have the new tests `.join()` it directly, rather than polling `status_and_next_sequence()` in a sleep loop.

Rationale:
- Zero-risk signature change: the single call site (`terminal_helper_process.rs#L476`) already discards the return value as a bare statement, and `JoinHandle` is not `#[must_use]`, so production code needs no edit at all (or at most a stylistic `let _handle = ...`).
- A direct `.join()` is deterministic and immediate (no timeout/backoff tuning, no flake surface from a poll interval racing thread scheduling) — strictly simpler test code than a `while status != Exited { sleep(..) }` loop, and avoids inventing a poll timeout constant.
- The existing test modules in this file (`ring_state_tests`, `kill_path_guard_tests`) exercise pure synchronous logic with no threads; this is the first test needing real thread synchronization, so there is no existing poll-based pattern to match — free to pick the simpler primitive.

## Implementation Plan
1. `ws-dashboard/crates/daemon/src/terminal_helper_process.rs#L499-514` — rewrite `spawn_reader_thread` body: change the fn signature to return `thread::JoinHandle<()>` (`thread::spawn` already returns this; just return it instead of discarding). Inside the closure, replace the fixed `[0u8; 4096]` + per-read `from_utf8_lossy` with: a `let mut carry: Vec<u8> = Vec::new();` outside the loop; each iteration reads into a fixed `[0u8; 4096]` scratch buffer, builds `combined = [carry.as_slice(), &scratch[..n]].concat()` (or equivalent), then runs the `std::str::from_utf8` dispatch loop exactly as specified in the ticket's Phase 1 body (`Ok` → emit all + clear carry; `Err` with `error_len() == None` → emit prefix, carry suffix (<=3 bytes), stop; `Err` with `error_len() == Some(n)` → emit prefix, emit one lossy `U+FFFD` for the `n`-byte malformed span, loop on the remainder). On `Ok(0)`, flush `carry` via `String::from_utf8_lossy` (if non-empty) before breaking. Keep the `Err(_) => transition(Error); return` arm unchanged (still needs to return the handle's join value implicitly by falling out of the closure — no interface change needed since the closure return type stays `()`, only the outer fn's return type changes).
2. `ws-dashboard/crates/daemon/src/terminal_helper_process.rs#L476` — no code change required (return value discard is already valid); optionally note in a comment that the handle is intentionally detached in production (matches the existing "detached, never joined" style used for `spawn_process_exit_reaper` at L537-544).
3. `ws-dashboard/crates/daemon/src/terminal_helper_process.rs` (new `#[cfg(test)] mod reader_thread_utf8_tests` appended after `kill_path_guard_tests`, ~after L743) — add:
   - A local `shared_state_for_test() -> SharedState` helper duplicating the one at L665-679 (same field list, same values).
   - A `ScriptedReader` fake implementing `std::io::Read`: wraps a `VecDeque<Vec<u8>>` of chunks; each `read()` call pops the front chunk, copies it into `buf`, returns `Ok(len)`; once the queue is empty, returns `Ok(0)` (models "N scripted chunks then EOF" generically enough to cover all three required cases).
   - Test 1 (split-mid-codepoint): pick a multi-byte string (e.g. Korean or emoji), split its UTF-8 bytes so the first scripted chunk ends mid-codepoint and the second chunk carries the remainder; call `spawn_reader_thread(Arc::new(shared_state_for_test()), Box::new(ScriptedReader::new(chunks)))`, `.join()` the returned handle, then assert `ring.backfill_after(0)` concatenated `.data` equals the original string exactly and contains no `'\u{FFFD}'`.
   - Test 2 (EOF truncation): scripted chunks end with a genuinely incomplete multi-byte sequence and no further chunk follows (natural `Ok(0)` after); assert the flushed output is the lossy-replaced string (contains `\u{FFFD}` for the truncated tail) and the thread does not hang (bounded by the `.join()` itself — a hang here fails the test via harness timeout, no extra polling needed).
   - Test 3 (malformed interior byte): a single chunk containing valid ASCII/multi-byte text, then one invalid byte (e.g. a lone continuation byte `0x80` or invalid leading byte `0xFF`) sandwiched mid-chunk, then more valid text, then EOF; assert output contains exactly one `\u{FFFD}`, and the valid text before and after the malformed byte is present unmodified in the reassembled string (i.e., no cascading corruption / no wedge).
4. Run `cargo build -p ws-dashboard-daemon` first to catch signature/borrow issues before running tests (the `str::from_utf8` loop involves slicing `combined` repeatedly — verify borrow-checker-friendly structuring, e.g. re-`concat`/reslice each loop pass rather than holding overlapping borrows).

## Verification Plan
- `cargo test -p ws-dashboard-daemon terminal_helper_process` (or the crate-wide `cargo test -p ws-dashboard-daemon` if scoping by module path proves awkward) — runs the new `reader_thread_utf8_tests` module alongside existing `ring_state_tests`/`kill_path_guard_tests` in the same file. This is the real daemon crate's test binary, not a mock/dry-run — confirm all three new tests plus all pre-existing tests in this file still pass (no regression in `ring_state_tests`/`kill_path_guard_tests`, which are untouched by this change but share the file).
- Manual/incidental: no dashboard/browser-level manual verification is prescribed by the ticket for Phase 1; the ticket's verification section is fully covered by the unit tests above.

## Escalations
- None.
