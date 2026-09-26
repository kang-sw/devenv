---
title: "Pi adapter: child telemetry refresh re-parses the whole child session.jsonl on the parent's main thread"
related:
  260924-feat-pi-agent-channel-usage-rollup: prior decision - kept the direct-child own-usage reduction and its cadence unchanged
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 0be785f8aec04b51
sage-review-completeness-reviewed: 0be785f8aec04b51
completed: 2026-09-26
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
  consumed offset. This covers every non-append write of Pi's session writer
  listed in Background.
  - The final line keeps today's classification exactly. Today a parse
    failure on the last line — terminated or not — is `transient`, and an
    unterminated final line that already parses is accepted as an entry. So
    the reader never advances its offset past the last line of the file: it
    parses that line without consuming it, and a malformed last line turns
    invalid only once a later line exists. Pi's load-time torn-line repair
    (appending `"\n"`) produces exactly a terminated malformed last line.
  - An invalid verdict is cached until a reset trigger fires; neither a
    later refresh nor the hash-mismatch fallback re-reads the whole file for
    a file already judged invalid, since an append-only file stays invalid.
  - The header comparison uses a retained hash of the raw header line, not
    only its `id` and `parentSession`: a migration rewrite keeps the id and
    changes the version.
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
  - The projection preserves absent versus `null` versus present for
    `message`, `message.usage`, and `usage`: `reduceTelemetry` reads
    `e.message?.usage ?? e.usage` and tests `rawUsage !== undefined`.
  - Reader state does not outlive the child's registry record: it is freed
    when the record is dropped, so a long-lived lead that spawns many
    children does not accumulate finished children's projections. Where the
    state lives (on the record, or in a map evicted with it) is the worker's
    choice, subject to avoiding an import cycle between `agent-telemetry.ts`
    and `spawner.ts`.

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

## Prior Decisions

- 260924-feat-pi-agent-channel-usage-rollup (2026-09-24, Decisions): "The parent keeps its existing reduction of the direct child's `session.jsonl` in `refreshAgentTelemetry` (`agents-plugin-pi/src/spawner.ts`), with its current cadence, prefix anchor, and context-token fields." — bearing: constrains
- 260913-bug-ws-pi-cost-footer-cpu-saturation (2026-09-13, Decisions): "Update cached cost only at explicit registry, telemetry, and session lifecycle boundaries. Do not poll, watch the filesystem, recursively discover ownership, reread session JSONL, or walk history for footer refresh." — bearing: constrains
- 2f833ec8 (2026-09-13, commit): "The synchronous 250 ms descendant scan and render-time history traversal were removed because both ran on Pi's main thread and reproduced sustained CPU saturation." — bearing: supports
- c103df38 (2026-09-13, commit): "RPC getSessionStats().contextUsage.tokens is the preferred source because it uses Pi's own context-accounting contract; JSONL assistant usage is retained as an offline and transient-failure fallback." — bearing: constrains
- 08ac495b (2026-09-21, commit): "The streamed-delta path now skips subtree observation, telemetry refresh, and subtree publication." — bearing: supports
- b45621ea (2026-09-10, commit): "TC-C3: distinguish transient absence of evidence from readable contradictions before reducing; preserve immutable origin and validated usage through transient reads without manufacturing a legacy lifetime baseline." — bearing: constrains
- fbe27f67 (2026-09-10, commit): "Explicit read classifications prevent absent or transient fork baselines from becoming empty-prefix attribution." — bearing: constrains
- 17dea8a3 (2026-09-10, commit): "Use durable SDK session-entry IDs and an immutable pre-first-prompt prefix anchor because responseId is optional and streaming usage is cumulative." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/agent-telemetry.ts, agents-plugin-pi/src/spawner.ts, plus new replay tests |
| scope.surface | internal | readSessionEntries is exported from agents-plugin-pi/src/agent-telemetry.ts#L99 but consumed only by spawner.ts#L762 and the telemetry tests; no package-level surface changes |
| scope.new_public_symbol | yes | a new exported incremental reader in agent-telemetry.ts for spawner.ts to call; name not fixed by the ticket; adapter-internal |
| scope.new_type_contract | yes | the retained incremental state: per-path byte offset, header id and parentSession, projected entries, per-entry line hashes |
| scope.test_surface | existing | agents-plugin-pi/test/agent-telemetry.test.ts, agents-plugin-pi/test/agent-telemetry-contract.test.ts, agents-plugin-pi/test/agent-telemetry-lifecycle.test.ts |
| complexity.reuse_points | confirmed | readSessionEntries as oracle and conflicting-duplicate fallback agent-telemetry.ts#L99-L116; reduceTelemetry reads only id, type, message.role, message.usage, usage agent-telemetry.ts#L118-L137; refreshAgentTelemetry reads headerId, parentSession, entries.at(-1).id spawner.ts#L762-L796 |
| complexity.side_effect_risk | moderate | introduces per-path reader state that outlives a refresh in the parent process; a missed reset would silently misreport child cost and context |
| risk.correctness | high | byte-offset tracking across torn lines, truncate-and-rewrite, header change, and hash-mismatch reparse must stay result-equivalent to readSessionEntries in every case |
| risk.fit | moderate | agent-telemetry.ts is otherwise pure functions with node-only imports; the stateful reader and its ownership per record or per path must avoid an import cycle with spawner.ts |
| risk.test | moderate | replay oracle tests need chunked, torn, rewrite, path-change, header-change, and conflicting-duplicate cases plus a retention non-reachability check |
| risk.security_or_contract | moderate | readSessionEntries validation outcomes transient versus invalid and the 260924 reduction semantics are a contract callers depend on |

## Phases

### Phase 1: Incremental child-session reader for telemetry refresh

The parent's telemetry refresh reads a child's `session.jsonl` through the
offset-based incremental reader under the reset and retention rules in
`## Decisions`, so its per-refresh cost is bounded by the bytes appended since
the last refresh, and a large or image-heavy child session no longer saturates
the parent's main thread.

Verification:

- The existing full `readSessionEntries` is the oracle. After every read,
  replay tests compare the read classification (entries, `transient`, or
  invalid) and, for an entries result, the header `id`/`parentSession` and the
  projection of the oracle's entries against the incremental result. Cases:
  chunked appends; torn mid-line writes; a tear exactly before a line's
  `"\n"` (parseable unterminated tail); a torn line repaired by appending
  `"\n"`; a missing file that reads `transient` and then recovers; a
  truncate-and-rewrite to a smaller size; a same-size-or-larger rewrite that
  changes only the header line (the migration shape); a path change; a
  same-id line with different content; and entries whose `message`,
  `message.usage`, or `usage` is absent or `null`.
- A test pins that per-refresh cost is bounded by appended bytes: after the
  initial read, a refresh reads only from the consumed offset (for example by
  asserting the byte ranges read through an injected reader), so an
  implementation that still reads the whole file and parses only new lines
  fails.
- A test pins that an invalid verdict is cached: further refreshes of an
  unchanged invalid file do not read it from byte 0.
- A test pins that the retained state does not hold entry content outside the
  projection (for example, a large tool-result payload is not reachable from
  it).

### Result (e02126f6b) - 2026-09-26

`IncrementalSessionReader` in `agents-plugin-pi/src/agent-telemetry.ts` now
feeds `refreshAgentTelemetry`. The reader state lives on
`RpcAgentRecord.telemetryReader`. It is created lazily, never serialized, and
deleted at both registry removal sites: `evictForCapacity` and
`pruneRemovedAgents`.

**Reader behavior (commits f64b88dd9 and e02126f6b):**
- Each refresh reads the header line (to compare its hash), a one-byte check
  at the consumed boundary, and the bytes from the consumed offset onward.
- It never consumes the file's last line. That line is parsed on every
  refresh.
- Retained state is limited to `projectEntry` projections, one raw-line
  SHA-256 per id, and a cache of `(id, hash)` duplicate verdicts.
- `SessionFileIo` is an injectable byte-range seam, used by the tests to
  observe which byte ranges a refresh reads.
- `readSessionEntries` stays as the oracle. It now shares only
  `sessionHeaderOf`/`validEntry` with the reader.

**Decisions:**
- **Extra reset trigger.** Beyond the ticket's reset list, the reader also
  resets when the byte before the consumed offset is not a newline. This
  catches rewrites that keep the header but move line boundaries, which
  existing lifecycle fixtures perform.
- **Size equal to the offset resets.** In append-only state the unconsumed
  last line always holds at least one byte, so a file that ends exactly at
  the offset was truncated.
- **Order of verdicts.** A consumed line that fails to parse stays invalid
  even when the last line is torn, which matches the oracle's order.
- **Duplicate verdicts cached both ways.** A conflicting duplicate as the last
  line does not repeat the full-file lookup.
- **Failed lookup.** If the full-file lookup cannot find the first occurrence,
  the read resets and returns `transient`.
- **Lazy hashing.** A new id's line is hashed only when it is consumed.
- **Fixture spy.** The spy in `test/fixtures/usage-hop.ts` now also observes
  `openSync`, so its "only its direct child's session" positive control still
  holds.

**Verification:**
- `npm test -- test/agent-telemetry*.test.ts test/agent-usage-rollup*.test.ts test/eviction-records.test.ts`:
  132/132 pass.
- Full `npm test` in `agents-plugin-pi` at 70c587dc0: 1891 tests, 1889 pass,
  0 fail.
- `test/agent-telemetry-incremental.test.ts` compares the reader against the
  oracle after every read. It covers:
  - chunked appends, including seeded random tears and tears inside UTF-8
    characters;
  - a tear just before `"\n"`, and a torn line repaired with `"\n"`;
  - a missing file, truncation, an in-place header rewrite, and a path change
    (these last two also with identical header bytes and line boundaries);
  - same-id lines that are identical, deep-equal, or conflicting;
  - `message`, `message.usage`, and `usage` absent, null, or present.
- It also pins:
  - the byte ranges each refresh reads;
  - that an invalid verdict is cached;
  - that a ~1 MB payload is not reachable from the reader's state;
  - the reader's lifetime on the record, including eviction and prune.
- Reviews ran in partitions (correctness, fit, test), round 1 and round 2.
  They left no open findings. The correctness reviewer's independent fuzz ran
  about 25.5k reads against the oracle with 0 mismatches.
