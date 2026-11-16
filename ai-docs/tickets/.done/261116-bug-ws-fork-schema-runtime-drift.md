---
title: "ws-fork advertised schema disagrees with runtime behavior"
completed: 2026-11-16
sage-review:
  design: skipped
  completeness: skipped
related:
  261112-feat-ws-mixed-version-compat-guards: "distinguish schema/runtime drift from a host caching an older tool inventory"
---

# ws-fork advertised schema disagrees with runtime behavior

## Background

Dogfood #16, 2026-11-16: the lead reports that the `ws-fork` tool's schema and
runtime disagree. This worker did not observe a fork invocation or its schema
failure directly. The exact advertised schema, request, runtime response,
argument names, host/profile and version pins were not supplied. Do not infer
which field or producer is wrong, or conflate this with the separate fork
identity/authority incidents.

## Decisions

- Compatibility tier: **T0**, description-only drift tolerance. The lead captures
  tool metadata in `agents-plugin-pi/src/fork.ts:350` through
  `captureRegisteredTools` (`src/fork-context.ts:84`; remaining `src/` paths
  below are relative to `agents-plugin-pi/`); the child and parent read
  it through `compareForkRegistrations` (`src/index.ts:1324`,
  `src/spawner.ts:2878`). The new comparator ignores prose, not validation
  constraints, and never rewrites the captured or runtime definitions
  (`src/fork-context.ts`, `forkValidationSchema` and `classifyForkRegistrations`).
  Launch/context version and durable data are unchanged; old readers still
  read new snapshots, and reopening needs no migration or recovery. Old
  adapters retain their existing strict refusal; this fix adds no boundary.
- Choose the smallest evidenced fix: ignore tool descriptions and JSON Schema
  descriptions at schema nodes, and compare objects independent of key order.
  Preserve literal `const`/`enum`/`default` values and description-named arguments.
  Optional-property additions, other schema changes, missing/extra callables,
  and callable reordering remain fail-closed. General schema subtyping is out
  of scope; current runtime metadata stays authoritative, never replayed.
- Owner's implementation instruction supersedes the backlog's open-ended live
  repro requirement: use a deterministic regression and one passing scoped
  check; no local native ws processes. Design/completeness ceremony skipped
  under that explicit fast-path instruction.

## Constraints

- No Go, ws/ws-mcp executable, launcher, or native test binary runs locally.
- Go verification runs remotely on w5090 via `D:/sys_lock.sh`; local Node tests
  are permitted only when they spawn no native ws processes or launcher.
- Local temporary files stay under `D:/Temp/exec4/`; no other worktrees, push,
  or merge. No Implementation Conventions table row matches the changed Pi
  paths; release-compatibility manual was additionally read for the T0 verdict.
- Ticket tools are absent from this worker's inventory: use file discovery
  and the documented `git mv` lifecycle fallback.

## Phases

### Phase 1: Reproduce and reconcile the fork schema/runtime mismatch

Trace parent capture and child/parent readiness validation. Reproduce harmless
metadata drift in the shared registration comparator, tolerate description-only
drift without replaying stale schemas, and retain real schema and tool-surface
refusals. Add a focused regression that fails before the fix. Preserve fork
identity, authority, lineage, and callable order. Verify the pure Node tests
locally (no subprocesses) and the MCP schema producer's Go package remotely.
Live host probing and general additive-schema compatibility are not in scope.

### Result (ac20891d1) - 2026-11-16

- Found the defect in the shared `classifyForkRegistrations`: full-definition
  `JSON.stringify` equality rejected description-only drift in child bootstrap,
  input admission, and parent readiness. Source evidence: `a2918bc7e` changed
  only worktree.release/force guidance wording, an independently documented
  T0 change. No live failed invocation was recovered or claimed.
- Compare validation schemas structurally after stripping descriptions only at
  schema nodes. Tool-level prose no longer blocks startup; schema constraints,
  literal data, tool presence and ordering remain checked. Runtime registrations
  are not modified. All existing readiness callers use this shared fix.
- Regression before fix: 10 passed, 1 failed with
  `changed callable tools: ws__worktree_release`. After fix: **11/11 passed**:
  `cd agents-plugin-pi && TMPDIR=D:/Temp/exec4 TEMP=D:/Temp/exec4 TMP=D:/Temp/exec4 node --test test/fork-context.test.ts`.
  Inspected imports: only Node built-ins and `atomic-write.ts`; no subprocess
  or launcher is spawned.
- Required remote check **passed** (209.110s):
  `bash D:/sys_lock.sh remote w5090 run exec4 25 --sync 'D:/Workspace/Repos/.ws-worktrees/devenv/groove-animosity-playroom' -- 'export PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && go test ./internal/mcp/...'`.
- Dogfood surprise: ticket query/move/close tools were unavailable in this
  worker profile; used documented filesystem/Git fallbacks. No other surprises
  or Forward items; no live fork probe or local native execution.
