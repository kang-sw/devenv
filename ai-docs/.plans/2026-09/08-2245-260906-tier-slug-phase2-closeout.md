# Plan — 260906 tier-slug Phase 2 closeout

Survey-depth plan authored inline by the lead (holds the authoritative
inspection context). Phase 2 of
`260906-bug-ws-pi-tier-slug-rejected-children-inherit-parent-model`:
"Pass the resolved effort to the explore child."

## Relevant Ticket Contract

Phase 2 (independent of Phase 1). Extend `BuildSpawnArgsOptions` with an
optional `thinking` and append `--thinking <level>` when it is a non-empty
string, before the task argument; `resolveExploreModel` returns `{model,
effort}` and `exploreLeaf` forwards the effort; correct the stale
`applyModelEffort` comment; amend the spec sentence under Spec Impact.
`buildSpawnArgs` stays pure; existing callers unchanged. Tests: `buildSpawnArgs`
emits `--thinking` only when the option is a non-empty string, before the task
argument; `exploreLeaf` passes the tier's effort through (mock the ws-mcp
resolve to return one) and passes nothing on an inherit.

## Out of Scope

- Phase 3 (advisory dedupe; lands after the static-body ticket).
- Any Phase 1 behavior (already landed).
- Validating against Pi's exact `ThinkingLevel` enum (the string is forwarded
  as-is; unsupported degrades to a no-op).
- No `--thinking`/effort parameter added to the `explore` tool surface.

## Codebase Findings

The forwarding **substance already shipped** in later unrelated work
(`abee6d7e`), so this phase is largely a test/comment/spec closeout:

- `agents-plugin-pi/src/spawner.ts:254` — `BuildSpawnArgsOptions.thinking?:
  string` present. **Done.**
- `agents-plugin-pi/src/spawner.ts:296-297` — `buildSpawnArgs` appends
  `--thinking <opts.thinking>` under `if (opts.thinking)` (empty/undefined
  guarded by truthiness). **Done.**
- `agents-plugin-pi/src/spawner.ts:758` — `exploreLeaf` passes
  `thinking: options.effort` into `buildSpawnArgs`. **Done.**
- `agents-plugin-pi/src/spawner.ts:2995` — `resolveRequiredExploreModel`
  (renamed from `resolveExploreModel` by Phase 1 work) returns `{model,
  effort?}`; already unit-tested at `test/spawner.test.ts:654-655`. **Done.**

Genuine remaining delta:

1. **GAP (test):** the `describe("buildSpawnArgs")` block
   (`test/spawner.test.ts:256-370`) has no `--thinking` case. Phase 2
   enumerates it.
2. **GAP (test/judgment):** no `exploreLeaf` effort-forwarding test. Note
   `exploreLeaf` (`src/spawner.ts:717`) spawns a real `pi` process and is
   "exercised only by a lead-scoped Pi session" (`test/spawner.test.ts:44`),
   so a pure unit test of it may not be feasible. The observable seam is
   `buildSpawnArgs` (`--thinking`) plus `resolveRequiredExploreModel`
   (`effort`), both unit-testable; there is a `runExploreLeaf` injection point
   in the tool factory (`src/spawner.ts:2966`). Implementer judgment: add a
   focused `exploreLeaf`/seam test if the DI point permits mocking the spawn
   without a live process; otherwise cover the behavior through `buildSpawnArgs`
   + `resolveRequiredExploreModel` and record why a direct `exploreLeaf` unit
   test is not added.
3. **Comment:** `applyModelEffort`'s doc comment (`src/spawner.ts:2242-2251`)
   currently frames the RPC path as "Pi has no separate `--reasoning-effort`
   CLI launch flag." Phase 2 wants this corrected: a launch-time `--thinking`
   flag *does* exist (the ephemeral collection leaf uses it via
   `buildSpawnArgs`); the persistent path uses `setThinkingLevel` because the
   effort can change across restarts/resumes, not because no flag exists.
   Reword to that effect.
4. **Spec:** `{#260903-pi-spawner-model-tier-inherit}` already says "collection
   forwards effort as `--thinking`" (Phase 1 pass). Verify the process-spawned
   (`--thinking`) vs RPC-backed (`setThinkingLevel`) distinction and the
   "inherit or empty effort passes no level" clause are present; amend the
   anchor minimally if the RPC-backed half is missing. Keep additive.

## Implementation Plan

1. `test/spawner.test.ts`, `describe("buildSpawnArgs")`: add cases — `--thinking
   <level>` is emitted when `thinking` is a non-empty string and sits before the
   task positional; `--thinking` is absent when `thinking` is `""` or omitted.
2. Per finding 2: add an `exploreLeaf`/seam effort test if feasible via the
   `runExploreLeaf`/spawn DI point without a live process; else document the
   coverage decision in the commit/Result.
3. `src/spawner.ts`: reword the `applyModelEffort` comment per finding 3
   (comment-only; no behavior change).
4. `ai-docs/spec/pi-adapter-runtime.md`: verify/minimally amend the Phase 2
   sentence in `{#260903-pi-spawner-model-tier-inherit}` (the doc pre-pass owns
   the final spec commit — the implementer may leave the spec to the lead's doc
   pass; do not commit spec from the implementer).

## Verification Plan

`cd agents-plugin-pi && node --test test/spawner.test.ts test/bridge.test.ts
test/agent-sidecar.test.ts` with `WS_PI_SPAWN_ROLE` unset → all green. The full
suite's 130 pre-existing environment-bound failures (linuxbrew-hardcoded fork
tests + one lead-bootstrap) are unrelated.

## Escalations

None. Scope is a bounded test/comment closeout over already-landed behavior;
confidence high.
