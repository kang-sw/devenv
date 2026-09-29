---
title: config.tune agents.tier writes lose concurrent updates
---

# config.tune agents.tier writes lose concurrent updates

## Background

A downstream report (ws 0.46.23, pi harness 0.87.1, Windows) issued four
`config.tune { key: "agents.tier", harness: "pi", scope: "global" }` calls in
one parallel tool block, one per tier. Every call returned success with
`"warnings":[]`, yet `config.resolve_agent` afterwards showed only two of the
four `pi` leaves; the other tiers fell back to `default`. Re-issuing the lost
writes one per block landed them reliably.

Code confirms the reporter's lost-update hypothesis:

- The MCP stdio loop dispatches each request in its own goroutine
  (`ServeStdio` in `internal/mcp/server.go`), so same-block tool calls run
  concurrently. Separate processes (each pi/Claude/Codex session's ws-mcp, the
  `ws-mcp` CLI) also write the same config files.
- `setAgentsTierForHarness` and `unsetAgentsTierForHarness`
  (`internal/wsconfig/config.go`) load the whole `Config` document, change one
  `agents.model_aliases.<tier>.<harness>` leaf, and write the whole document
  back through `saveConfigFile`, which is an in-place truncating
  `os.WriteFile` with no lock and no temp-file rename. They are the only
  callers of `save` / `saveGlobal` / `saveConfigFile`.
- The resolver override writers in the same package (`setOverrideInFile`,
  `setOverrideInFileRMW`, `deleteOverrideInFile` in `resolver.go`) already
  hold a `gofrs/flock` lock on the sibling `<config path>.lock` across
  read-modify-write and write via `os.CreateTemp` + `os.Rename`, with
  concurrency tests in `scope_test.go`. The agents.tier writers bypass that
  lock, so an agents.tier write also rewrites `overrides` and can drop a
  concurrent locked resolver write (`workflow.lang`, `prefer_subagent`, ...)
  to the same file.
- The response is built from the goroutine's own in-memory merged value, so
  each caller sees its own leaf and cannot detect that a later writer
  discarded it.

Some reported response anomalies (a writer's response omitting the leaf it
just wrote) are not explained by the code; a torn read of a half-written file
from the non-atomic truncate-write is one possibility, report misreading
another. They were not reproduced.

## Decisions

- **One lock for every writer of a config file.** The agents.tier set and
  unset paths (global and project scope) hold the same sibling
  `<config path>.lock` `gofrs/flock` lock the resolver override writers use,
  across load/modify/write, and write via `os.CreateTemp` + `os.Rename`.
  Rejected: a separate lock or an in-process mutex, since neither serializes
  against resolver writes or against other ws-mcp processes.
- **Shared locked update helper.** Extract one helper in `wsconfig` (shape:
  `updateConfigFile(path string, mutate func(*Config) error) error`) that owns
  lock, read, mutate, `schema_version` stamp, temp write, and rename. Route the
  agents.tier set/unset writers and the three resolver writers
  (`setOverrideInFile`, `setOverrideInFileRMW`, `deleteOverrideInFile`)
  through it, and delete the unlocked `saveConfigFile` write. Resolver writes
  therefore start persisting `schema_version: 1`, matching the established
  persisted contract. `deleteOverrideInFile`'s no-op on a missing file or key
  is preserved. Rejected: a fourth copy of the lock sequence on the
  agents.tier path only, or a helper used by agents.tier only; both leave the
  per-writer drift that caused this bug.
- **Response.** The `config.tune` response keeps its current shape and is
  built from the `Config` value committed under the lock.
- **Lock timeout.** Reuse `lockTimeout` (10s); a timeout surfaces as a
  `config.tune` error, not a warning.
- **Readers stay unlocked.** `LoadAgentTierConfig`, `config.list`, and
  `config.resolve_agent` do not take the lock; atomic rename means they see
  the old or the new file, never a torn one. The Windows rename-over-open-file
  risk is accepted at the same level the resolver path already accepts it.

## Constraints

- Out of scope (separate follow-ups, user-confirmed): `config.list` not
  reporting the global config path (`~/.ws/config.json`); the possibly
  machine-wide project-scope config path from `wsstate.CacheRoot`'s fixed
  directory name; the stale path comment in `wsconfig/scope.go`.
- Read `ai-docs/manuals/shipped-surface-boundary.md` and
  `ai-docs/manuals/ws-mcp.md` before editing (Implementation Conventions rows
  for `agents-plugin-tool/` and `agents-plugin-tool/internal/mcp/`).

## Phases

### Phase 1: Serialize agents.tier config writes

Make concurrent `agents.tier` set and unset writes, at global and project
scope, all land, and stop them from discarding concurrent resolver override
writes to the same file.

Verification:

- A concurrent test runs N goroutines of `SetGlobalAgentsTierForHarness`
  (distinct tiers/harnesses) against one temp config home and asserts every
  leaf is present afterwards; the same for project scope and for `Unset*`.
  Mirror the existing `TestConcurrent*NoLostWrites` tests in `scope_test.go`.
- A mixed test runs concurrent agents.tier writes and resolver override
  writes against the same file and asserts both sets survive.
- The new tests fail against the pre-fix code (run them before the fix, or
  under `-race` / enough iterations to make the loss observable) and pass
  after it.
- Resolver writes through the shared helper persist `schema_version: 1`, and
  `deleteOverrideInFile` on a missing file or key still creates no file.
- Existing `wsconfig` and `mcp` tests pass.
