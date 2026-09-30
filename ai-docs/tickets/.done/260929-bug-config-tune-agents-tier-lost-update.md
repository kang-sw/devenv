---
title: config.tune agents.tier writes lose concurrent updates
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 32a3da0eb109ac80
sage-review-completeness-reviewed: 32a3da0eb109ac80
completed: 2026-09-30
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
  non-test callers of `save` / `saveGlobal` / `saveConfigFile`; the test
  `TestUnsetAgentsTierForHarnessPreservesLegacyTierFallback` also seeds a
  file through `save`, and `TestSaveStampsSchemaVersionOnFirstWrite`'s doc
  comment names `saveConfigFile`
  (`agents-plugin-tool/internal/wsconfig/config_test.go#L12-L18`,
  `agents-plugin-tool/internal/wsconfig/config_test.go#L1023-L1031`).
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
  persisted contract. `save` / `saveGlobal` go away with `saveConfigFile`;
  tests that seeded files through them seed through the helper or a direct
  file write instead. Existing no-write contracts are preserved: a mutation
  that changes nothing writes nothing, and `deleteOverrideInFile` on a missing
  file or key and a no-op `unsetAgentsTierForHarness` create no file, lock
  file, or directory and leave bytes and mtime untouched
  (`TestUnsetAgentsTierForHarnessNoOpDoesNotSave`). The helper therefore needs
  a no-change signal (for example a `changed bool` from `mutate` or a sentinel
  error) and a missing-file check before locking on those paths; the exact
  form is the implementer's choice. Rejected: a fourth copy of the lock sequence on the
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
  directory name (already tracked as
  `260924-bug-ws-config-project-scope-is-machine-wide`); the stale path comment in `wsconfig/scope.go`.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)

## Prior Decisions

- 260619-feat-ws-layered-config-scope-substrate (2026-06-19, Decisions): "File lock required, both file scopes. Read-modify-write on project and global config must be serialized (flock + temp-write + atomic-rename)." — bearing: supports
- 6b3ea800 (2026-06-19, commit): "flock primitive: github.com/gofrs/flock per Lead Decision 1 (cross- platform, auto-release, no stale-lock-file)." — bearing: supports
- 3f68d560 (2026-09-21, commit): "C2: stamped schema_version at the single saveConfigFile chokepoint (both save/saveGlobal route through it) ... so every future save call site gets the same guarantee for free." — bearing: constrains
- 260921-feat-config-tune-agents-tier-global-scope (2026-09-21, commit 726d1dfe): "Kept agents.tier outside wsconfig.Resolver: AgentTier remains a structured compound value ... The project writer now persists only its changed alias leaf" — bearing: constrains
- 260923-feat-config-tune-agents-tier-reset (2026-09-23, Result e649dc64): "Added project/global exact-leaf unsetters ... The agents.tier JSON response now includes a warnings array for absent-leaf resets and exact project shadowing" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsconfig/config.go, agents-plugin-tool/internal/wsconfig/resolver.go, agents-plugin-tool/internal/wsconfig/scope_test.go, agents-plugin-tool/internal/wsconfig/config_test.go |
| scope.surface | internal | exported SetAgentsTierForHarness, SetGlobalAgentsTierForHarness, UnsetAgentsTierForHarness, UnsetGlobalAgentsTierForHarness keep their signatures; config.tune response shape unchanged per Decisions; callers at internal/mcp/server.go#L2409-L2425 untouched |
| scope.new_public_symbol | no | updateConfigFile is unexported in package wsconfig |
| scope.new_type_contract | yes | unexported updateConfigFile path string, mutate func *Config error returning error; resolver-written files gain persisted schema_version 1 |
| scope.test_surface | existing | agents-plugin-tool/internal/wsconfig/scope_test.go TestConcurrentWritersNoLostWrites L495, TestConcurrentGlobalWritersNoLostWrites L596; config_test.go save caller L1026 must migrate when saveConfigFile is deleted |
| complexity.reuse_points | confirmed | flock plus CreateTemp plus Rename sequence in agents-plugin-tool/internal/wsconfig/resolver.go#L243-L300 and lockTimeout at resolver.go#L15 were read |
| complexity.side_effect_risk | moderate | every project and global config file writer is rerouted, resolver writes change persisted bytes, and a lock timeout becomes a new config.tune error path |
| risk.correctness | moderate | concurrent RMW across goroutines and processes, deleteOverrideInFile missing-file no-op, and response built from the committed value must all hold |
| risk.fit | low | follows the recorded 260619 file-lock decision and the existing resolver writer pattern |
| risk.test | moderate | lost-update tests must be shown failing pre-fix, and timing-dependent concurrency tests risk flakiness |
| risk.security_or_contract | moderate | on-disk config contract changes for resolver writes and cross-process lock-file semantics are shared with other ws-mcp processes |

## Phases

### Phase 1: Serialize agents.tier config writes

Make concurrent `agents.tier` set and unset writes, at global and project
scope, all land, and stop them from discarding concurrent resolver override
writes to the same file. This includes extracting the shared locked update
helper and moving the three resolver writers onto it, per `## Decisions`.

Verification:

- A concurrent test runs N goroutines of `SetGlobalAgentsTierForHarness`
  (distinct tiers/harnesses) against one temp config home and asserts every
  leaf is present afterwards; the same for project scope and for `Unset*`.
  Mirror the existing `TestConcurrent*NoLostWrites` tests in `scope_test.go`.
- A mixed test runs concurrent agents.tier writes and resolver override
  writes against the same file and asserts both sets survive.
- The new tests fail against the pre-fix code and pass after it. Make the
  loss deterministic enough not to flake (for example a start barrier plus a
  fixed iteration count), and record the pre-fix failure evidence in the
  Result.
- A test asserts that the value an agents.tier set/unset returns equals the
  config as persisted on disk after the call.
- The existing no-op tests (`TestUnsetAgentsTierForHarnessNoOpDoesNotSave`)
  still pass unchanged.
- Resolver writes through the shared helper persist `schema_version: 1`, and
  `deleteOverrideInFile` on a missing file or key still creates no file.
- Existing `wsconfig` and `mcp` tests pass.

### Result (b66cb36cd) - 2026-09-30

Landed `updateConfigFile(path, mutate func(*Config) error) (Config, error)` in
`wsconfig/config.go`, the single writer for project and global config files. It
holds the sibling `<path>.lock` flock (`lockTimeout`, 10s) across read, mutate,
and write, stamps `schema_version`, and writes via `os.CreateTemp` + `os.Rename`.
Agents.tier set/unset (both scopes) and `setOverrideInFile`,
`setOverrideInFileRMW`, `deleteOverrideInFile` route through it; `save`,
`saveGlobal`, and `saveConfigFile` are deleted. Review fixes landed in
ff03cf651.

Decisions:
- The helper returns the committed `Config` (not only `error`) so the
  `config.tune` response is the value committed under the lock; the no-change
  signal is the `errConfigUnchanged` sentinel, which skips the write.
- Missing-file no-ops (`deleteOverrideInFile`, agents.tier unset) check
  `configFileExists` before the helper, so they create no directory or lock file.
- The temp file keeps an existing file's mode and creates a new file 0644
  (resolver-created files move from `CreateTemp`'s 0600 to 0644).
- A corrupt config on an agents.tier write now errors as `parse config for
  update` (the resolver wording); readers keep the scoped wording.

Verification:
- Pre-fix evidence: with the new tests applied to the pre-fix code,
  `go test ./internal/wsconfig/ -run 'ConcurrentAgentsTier' -count=10` failed
  all six set/unset/mixed subtests (project and global) in 10/10 runs. Failures
  were lost leaves plus 57 torn-read `unexpected end of JSON input` parse errors
  from the truncating write, which probably explains the reported response
  anomalies. `TestResolverWritesPersistSchemaVersion` also failed pre-fix
  (persisted 0).
- Post-fix: `go test -race ./internal/wsconfig/` ok; `go test ./...` in
  `agents-plugin-tool` ok (including `internal/mcp`, `cmd/ws-mcp`);
  `TestUnsetAgentsTierForHarnessNoOpDoesNotSave` unchanged and passing.
- New tests (`scope_test.go`): concurrent set/unset/mixed lost-update tests (3
  rounds, start barrier); a response-is-committed-value permutation check;
  `TestAgentsTierWriteReturnsPersistedConfig`;
  `TestAgentsTierWriteWaitsForFileLockHolder` (a lock held through another
  descriptor blocks the writer; catches an in-process-mutex swap);
  `TestResolverWritesPersistSchemaVersion`; `TestConfigNoOpWritesTouchNothing`;
  `TestConfigWritesKeepFileMode`.
- Review: correctness and test partitions, two rounds, clean. The test reviewer
  confirmed that each new check fails under its targeted mutation.

Known limits: the lock-timeout path has no test seam (unchanged from the
resolver path). With 50ms flock polling, the concurrency tests add about 15s to
the `wsconfig` package run.
