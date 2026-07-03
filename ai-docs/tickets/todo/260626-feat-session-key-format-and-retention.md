---
title: Session key format and retention cleanup
related:
  260625-feat-ws-session-state-machine: session-state fields now live in the per-key record files this ticket will touch and prune
  260605-research-ws-native-subagent-pivot: original session-key auth direction and earlier no-eviction decision
related-mental-model:
  - mcp-runtime
---

# Session key format and retention cleanup

## Background

The current session key implementation stores one JSON file per key at
`<cache-root>/keys/<session-key>.json`. Source inspection on 2026-06-26 found:

- `internal/wskey/wskey.go` still generates four random words plus a two-digit
  numeric suffix.
- `internal/wskey/wskey_test.go` and `internal/mcp/session_auth_test.go` assert
  the four-word format.
- `internal/mcp/session_auth.go` creates, reads, and atomically rewrites key
  files, but its store comment still says there is no eviction or logout.
- Focused source search found no session-key auto-prune implementation and no
  `touch`/`Chtimes` path that refreshes key-file mtime on read-only keyed calls.

The desired behavior is now:

- New session keys use three random words plus a two-digit suffix.
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

### Phase 2: Touch active session records and prune stale records

Add session-key record activity refresh and bounded pruning.

Acceptance:

- Successful MCP calls that resolve or require a known `session_key` refresh the
  corresponding key-file mtime. Prefer a central session-store helper so
  root-aware calls, lead-only keyed tools, and session-state tools do not grow
  independent touch logic.
- `ws.ferrule` checks a prune marker or equivalent cache-local bookkeeping and
  scans `<cache-root>/keys/` at most about once per day.
- Prune deletes valid key record files whose mtime is older than about one
  month. Malformed files should not crash pruning; current or newly minted
  records must not be pruned.
- Tests cover no-scan-on-every-ferrule behavior, mtime refresh on successful
  keyed lookup, stale-key deletion, and retention of recently touched records.
- Update `mcp-tools` and `mcp-runtime` docs from "no automatic eviction" to the
  new touch/prune lifecycle.
