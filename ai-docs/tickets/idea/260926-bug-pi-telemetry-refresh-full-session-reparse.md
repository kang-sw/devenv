---
title: "Pi adapter: child telemetry refresh re-parses the whole child session.jsonl on the parent's main thread"
related:
  260924-feat-pi-agent-channel-usage-rollup: prior decision - kept the direct-child own-usage reduction and its cadence unchanged
---

# Pi adapter: child telemetry refresh re-parses the whole child session.jsonl on the parent's main thread

## Background

A Pi lead observed during dogfooding (2026-09-26) had its TUI become sluggish.
The parent process sat at ~100% CPU on its main thread while one ws subagent
child was running.

Evidence gathered on the live process:

- `ps -M -p <pid>`: the main thread at ~99% CPU (~35 min user time over a
  2.5 h session); V8 GC worker threads nearly idle.
- `sample <pid> 3`: nearly every main-thread sample sat under one path — a
  stream-read callback (the child's RPC stdout) resolving a promise into
  `node::fs::ReadFileUtf8`, then leaf frames `FindTwoByteStringIndices`
  (`String.split("\n")`), `Utf8Decoder`, and `JsonParser::ScanJsonString`.
- That shape matches `readSessionEntries` in `agents-plugin-pi/src/agent-telemetry.ts`:
  synchronous `readFileSync(path, "utf8")`, `split("\n")`, then `JSON.parse`
  per line, plus per-entry id de-duplication.
- The caller is `refreshAgentTelemetry` in `agents-plugin-pi/src/spawner.ts`,
  reached from the `refresh()` closure in `attachEventListener`. That closure
  fires on every non-delta child event of type `agent_start`, `agent_settled`,
  `message_end`, `message_update`, `thinking_level_changed`, or
  `compaction_end`. It coalesces with a dirty flag but loops `do … while (dirty)`,
  so while the child keeps emitting events the full re-read repeats back to
  back.
- The child's `session.jsonl` was 75 MB across only 669 lines; the largest
  lines were ~4.3 MB each — `read` tool results carrying base64 image content.
- One full read-and-parse of that file, measured standalone with Node 25.9,
  took 190–210 ms. Every refresh therefore blocks the parent's event loop, and
  with it TUI input and rendering, for ~200 ms, and the loop keeps the thread
  saturated.

The cost grows with the child's session size, not with the size of the change
since the last refresh, so any long-running or image-heavy child degrades its
parent over time. When the child exited, the parent's CPU dropped to 0%.

Pi's session writer (`core/session-manager.js` in
`@earendil-works/pi-coding-agent` 0.85.1) is append-only in steady state: each
entry is one `appendFileSync(JSON.stringify(entry) + "\n")`. The non-append
writes are: the first flush once an assistant message exists (the whole file
written with flag `"wx"`, so the file did not exist before); on load,
`_rewriteFile()` after a version migration or for an empty file (open `"w"`,
same inode, truncated and rewritten); on load, a torn final line repaired by
appending `"\n"`; and a fork or new session, which writes a different path.

A spike (lead scratchpad, not committed) replayed the 107 MB / 747-line child
session into a growing copy in 250 reads, half of them at a torn mid-line
boundary, and compared an offset-based incremental reader against the real
`readSessionEntries` after every read: 0 mismatches; 27,016 ms total and
276 ms max per read for the full reader versus 175 ms total and 16 ms max for
the incremental one. The incremental reader, kept naively, retains every parsed
entry, so the parent would hold the child's base64 image payloads in memory
(~100 MB for this session).

## Decisions

- **Status and linkage.** The ticket starts in `idea/`, per AGENTS.md "Dogfood
  surprises get captured", and links
  `260924-feat-pi-agent-channel-usage-rollup` as `related:`, whose Decisions
  fixed the reduction this bug lives in.
- **Offset-based incremental reader.** The parent keeps, per child session
  path, how many bytes of `session.jsonl` it has consumed, and on each refresh
  parses only the complete lines appended since. The user accepted this on the
  condition that it is safe; the next three decisions are that safety
  contract.
  - Rejected: dropping re-reads on `message_update` alone, and moving the read
    off the main thread or throttling it. Both keep a whole-file cost per read;
    the spike shows the incremental reader alone bounds the cost by appended
    bytes.
- **Reset rule.** The reader discards its state and reparses from byte 0
  whenever the session path changes, the header line on disk differs from the
  one it consumed, the file is missing, or the file's size is below the
  consumed offset. An unterminated final line is never consumed and yields
  `transient`, as today. This covers every non-append write of Pi's session
  writer listed in Background.
- **Bounded retention.** The incremental state retains only a projection of
  each entry — `id`, `type`, `message.role`, `message.usage`, `usage`, and
  entry order — plus the header's `id` and `parentSession`, and a hash of each
  entry's raw line. A later line with an already-seen id and the same hash is
  a duplicate as today; one whose hash differs triggers a one-off full reparse
  that applies today's `isDeepStrictEqual` rule, so the conflicting-duplicate
  outcome is unchanged.
  - Rejected: retaining full entries — simplest, but the parent would hold the
    child's image payloads (~100 MB for the observed session).
  - Rejected: comparing duplicates on the projection only — cheaper, but
    weaker than today's validation.
  - The projected fields are what `reduceTelemetry` and the
    `refreshAgentTelemetry` caller read today; a worker that finds another
    reader of the entries widens the projection rather than dropping it.

## Constraints

- Event-loop delay monitoring (for example `perf_hooks.monitorEventLoopDelay`)
  is out of scope; it is a diagnostics feature, not this fix.
- The reduction's semantics stay as `260924-feat-pi-agent-channel-usage-rollup`
  fixed them: the direct child's own usage and `contextTokens` come from its
  `session.jsonl`, starting after the telemetry origin's prefix anchor (a
  fork's inherited parent-history prefix stays excluded), and the widget,
  audit, sidecar, and fork-resume readers keep their current fields. That
  ticket also records that an earlier recursive design with 250 ms polling
  saturated the CPU — a fix must not reintroduce periodic whole-file reads.
- `readSessionEntries`' current validation outcomes stay observable: an
  unreadable file or a torn final line is `transient`; a malformed earlier
  line, a wrong header, or a conflicting duplicate id is invalid.

## Phases

### Phase 1: Incremental child-session reader for telemetry refresh

The parent's telemetry refresh reads a child's `session.jsonl` through the
offset-based incremental reader under the reset and retention rules in
`## Decisions`, so its per-refresh cost is bounded by the bytes appended since
the last refresh, and a large or image-heavy child session no longer saturates
the parent's main thread.

Verification:

- The existing full `readSessionEntries` is the oracle. Replay tests compare
  the incremental result with the full result after every read across chunked
  appends, torn mid-line writes, a truncate-and-rewrite, a path change, a
  header change, and a same-id line with different content.
- A test pins that the retained state does not hold entry content outside the
  projection (for example, a large tool-result payload is not reachable from
  it).
