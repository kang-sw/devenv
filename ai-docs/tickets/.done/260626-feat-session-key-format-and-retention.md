---
title: Session key format and retention cleanup
related:
  260625-feat-ws-session-state-machine: session-state fields now live in the per-key record files this ticket will touch and prune
  260605-research-ws-native-subagent-pivot: original session-key auth direction and earlier no-eviction decision
related-mental-model:
  - mcp-runtime
sage-review-design: completed
sage-review-completeness: completed
completed: 2026-08-30
---

# Session key format and retention cleanup

## Background

The session key implementation stores one JSON file per key at
`<cache-root>/keys/<session-key>.json`.

Phase 1 (key-mint format) has landed; the retention/eviction lifecycle is the
remaining scope of this ticket. Current source state:

- `internal/wskey/wskey.go` mints three random words joined by hyphens, with no
  numeric suffix. `internal/wskey/wskey_test.go` and
  `internal/mcp/session_auth_test.go` assert the three-word format.
- `internal/mcp/session_auth.go` creates, reads, and atomically rewrites key
  files. Its store comment still says there is no eviction or logout, and that
  is still true: no session-key auto-prune exists, and no `touch`/`Chtimes`
  path refreshes key-file mtime on read-only keyed calls.

The remaining desired behavior (Phase 2):

- `ws.ferrule` runs cache pruning at most about once per day, not on every
  bootstrap call.
- MCP functions that consume a `session_key` refresh the corresponding key-file
  mtime in a cross-platform-aware way.
- Prune removes session records whose key-file mtime is older than about one
  month.

## Decisions

- Session keys remain opaque to callers. The generator and tests may assert the
  mint shape, but root-aware tool code must continue to treat keys as opaque
  filenames guarded by the broad path-safe regex.
- Key-store pruning uses key-file mtime as the activity clock. Record writes
  already refresh mtime through replacement; successful read-only key resolution
  must also refresh it.
- Do not shell out to platform-specific `touch`. Use Go filesystem APIs and add
  Windows-relevant test coverage for the cross-platform contract where feasible.
- The prior no-eviction docs are obsolete once this lands; implementation must
  update `ai-docs/spec/mcp-tools.md` and `ai-docs/mental-model/mcp-runtime.md`.

## Spec Impact

Phase 2 changes caller-visible session-key lifecycle, so it addresses the
session-auth model in `ai-docs/spec/mcp-tools.md` (and the mirror in
`ai-docs/mental-model/mcp-runtime.md`). Expected change: session keys are no
longer retained indefinitely — a key whose record file goes untouched for about
one month is pruned, and any successful keyed call refreshes that record's
activity clock. The current spec/mental-model text stating "no automatic
eviction" is replaced by this touch/prune lifecycle. Phase 1 (mint format) is
an internal string-shape change with no spec-level contract impact.

## Phases

### Phase 1: Mint three-word session keys

Change `wskey.Generate()` from four random words plus `00`-`99` to three random
words plus `00`-`99`.

Acceptance:

- Update `internal/wskey` tests and MCP session-auth tests that assert the key
  regex.
- Keep `sessionKeyFilenamePattern` broad and path-safety-oriented rather than
  making it an exact three-word parser.
- Verify the focused Go tests for `internal/wskey` and `internal/mcp`.

### Result (fd84f0a4) - 2026-08-25

Landed out-of-band from a direct ferrule-key tweak, not through this ticket's
proceed flow. Deviation from plan: the two-digit numeric suffix was dropped
entirely — keys are now three bare words (e.g. `amber-tide-fox`), not three
words plus `00`-`99`. `wskey.Generate()`, the `internal/wskey` format test, and
`internal/mcp/session_auth_test.go`'s `sessionKeyPattern` were updated to
`^[a-z]+(-[a-z]+){2}$`; `sessionKeyFilenamePattern` was left broad and
path-safety-oriented. Collision headroom stays ample (7772-word pool →
7772^3 ≈ 4.7e11 combinations) and `GenerateUnique` re-rolls on collision, so
dropping the suffix does not meaningfully raise duplicate risk. `go build ./...`
and full `go test ./...` green.

### Phase 2: Touch active session records and prune stale records

Add session-key record activity refresh and bounded pruning.

Acceptance:

- Successful MCP calls that resolve or require a known `session_key` refresh the
  corresponding key-file mtime. Prefer a central session-store helper so
  root-aware calls, lead-only keyed tools, and session-state tools do not grow
  independent touch logic. The helper must cover the read-only session-state
  seams that call `readRecord` directly and bypass `lookup`
  (`getOverride`/`listOverrideKeys`/`children`), not just the root-aware
  `lookup` path — a session doing only read-only session-state calls must still
  refresh its clock.
- Throttle the touch to avoid a filesystem write on every keyed call: skip the
  `Chtimes` when the record's mtime is already within a recent window (suggested
  default: skip if refreshed inside the last hour). Prune's daily cap keeps the
  scan cheap; the per-call touch needs its own cheap-refresh guard.
- `ws.ferrule` checks a prune marker or equivalent cache-local bookkeeping and
  scans `<cache-root>/keys/` at most about once per day.
- Prune deletes valid key record files whose mtime is older than the retention
  window. Malformed files should not crash pruning; current or newly minted
  records must not be pruned.
- Concrete defaults (implementer may adjust with rationale): prune-scan cadence
  ~24h, retention age ~30 days, touch-refresh guard ~1h. The mtime-based tests
  must assert against whatever exact constants land, so pick them before writing
  the tests rather than leaving "about" in code.
- Tests cover no-scan-on-every-ferrule behavior, mtime refresh on successful
  keyed lookup (including the read-only session-state seams), stale-key
  deletion, and retention of recently touched records.
- Update `mcp-tools` and `mcp-runtime` docs from "no automatic eviction" to the
  new touch/prune lifecycle.

### Result (42f4d4f5) - 2026-08-30

Landed on `impl/goal/develop/copper-lantern-drizzle/perch-dust-purse` across
`fddc207a` (implementation + tests), `2465fbc9` (docs), and `42f4d4f5` (review
fix).

- **Touch.** A central `sessionStore.touch(dir, key)` helper stats the record
  and, when `time.Since(mtime) >= touchGuardWindow`, calls `os.Chtimes` (no
  shell-out, cross-platform). It is invoked from every read-only seam that
  bypasses the write path: `lookup`, `readState`, `getOverride`,
  `listOverrideKeys`, and `children` (parent key only, never the scanned
  entries). Write paths keep relying on `writeRecordAtomic`'s temp+rename mtime
  refresh, so no redundant touch was added there.
- **Prune.** New `internal/mcp/session_prune.go` `maybePrune()` is marker-gated
  (`.prune-marker`, scans at most once per `pruneScanCadence`), deletes
  `keys/*.json` records whose mtime is older than `keyRetentionAge`, never parses
  record JSON (malformed files cannot crash it and are judged on age only), and
  excludes the marker itself. Wired into `handleLeadLogin` (`ws.ferrule`) as a
  best-effort call whose result is discarded, so it cannot fail bootstrap.
- **Constants (concrete, tests assert by symbol):** `touchGuardWindow = 1h`,
  `pruneScanCadence = 24h`, `keyRetentionAge = 30 * 24h`.
- **Docs.** `ai-docs/spec/mcp-tools.md` and `ai-docs/mental-model/mcp-runtime.md`
  rewritten from "no automatic eviction" to the touch/prune lifecycle; the
  `sessionStore` type doc comment updated to match.

Deviation: `getOverride`/`listOverrideKeys` are exercised via direct
`sessionStore` method calls rather than the MCP `session_config` surface, because
those adapter methods are trivial pass-throughs and the only session-writable MCP
entry point would have required test-only wiring outside scope. `lookup` and
`readState` are covered via the real MCP surface (`ferrule`, `todo.list`).

Review (partitioned: correctness / fit / test). One Important test finding —
`TestReadStateTouchesRecordViaMCPTodoList` was false-confidence (the pre-dispatch
keyed-capability gate's `lookup()` touches the record before `readState`'s own
touch runs, so the MCP-surface test passed even with `readState`'s touch removed)
— resolved by dropping it; the sibling direct-call `TestReadStateTouchesRecord`
isolates the seam and was regression-verified to fail when the touch is removed.
Accepted minors (no change): prune/`lookup` TOCTOU window (negligible, benign
re-login), `deleteOverride` no-op path not refreshing mtime (out-of-scope edge),
and the touch-guard exact-boundary test (optional hardening).

Verification: `go build ./...` clean; `go test ./internal/mcp/...` green (44.8s);
full `go test ./...` green across all 12 packages at `fddc207a`, `internal/mcp`
re-verified green after the review fix (`42f4d4f5`).

This was the last unfinished phase; the ticket is complete.
