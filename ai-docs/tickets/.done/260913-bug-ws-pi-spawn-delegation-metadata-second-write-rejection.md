---
title: Pi spawn performs a redundant delegation-metadata write that may fail after allocation
related:
  260913-feat-ws-pi-delegated-write-scopes: introduced durable delegated write authority
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 1e0e04773285b042
sage-review-completeness-reviewed: 1e0e04773285b042
completed: 2026-11-15
---

# Pi spawn performs a redundant delegation-metadata write that may fail after allocation

## Background

A historical dogfood report observed the session-bounded warning `ws: could not persist owned-agent metadata; the affected operation was rejected.` after an Explore allocation without stdout corruption. Its underlying failure and causal connection to the second write have not been reproduced. MAIN assigned this structural fix on 2026-11-15: include delegation in the initial durable allocation and remove the redundant immediate write.

Current `spawnAgent()` flow allocates an owned agent home through `allocateAgentHome()` and writes `ownership.json` without delegation, then immediately assigns in-memory delegation and calls `updateOwnership()` to persist it; that second call reports failure and returns `undefined` rather than throwing, and spawn ignores its result, so a rejected delegation write does not itself reject allocation or launch (`agents-plugin-pi/src/agent-storage.ts#L273-L285`, `agents-plugin-pi/src/agent-storage.ts#L440-L459`, `agents-plugin-pi/src/spawner.ts#L3835-L3837`). The warning fingerprint intentionally discards the underlying error, so the historical event cannot distinguish transient lock contention, malformed metadata, path-safety rejection, or write/rename failure. Historical inspection reportedly found valid delegation metadata and no surviving locks; that past machine state is not newly verified. Persistent corruption is not established.

## Decisions

- Pass the already-normalized `DelegationPolicy` into `allocateAgentHome()` and include `AgentOwnership.delegation` in the first `ownership.json` write. Remove only the immediate delegation-only `updateOwnership()` in `spawnAgent()`. Rejected: keeping two durability boundaries or suppressing the warning.
- Complete initial persistence before registry registration or child start. If that write fails, allocation rejects and existing channel cleanup/diagnostics remain effective. A first-write failure must not leave a launched child whose durable authority is incomplete.
- Compatibility is **T0**: retain the existing version-1 optional `delegation` shape and reader/update paths. Existing readers parse this field and updates preserve it (`agent-storage.ts:400–438,448–454`); reopening uses the same descriptor reader. No new tool argument, protocol, store version or migration is needed. Internal signature changes preserve unrelated callers, including unscoped allocation.

## Constraints

- Keep ownership locks, atomic replacement, path checks, retention protection, normalized immutable scope binding and fail-closed mutations unchanged.
- Preserve the synchronous admission-to-registration boundary and concise session-bounded notification for genuine authoritative failures. Do not broaden lifecycle recovery or change later metadata updates.
- Read the relevant Pi harness documentation/examples before adapter edits, plus repository skill-authoring guidance if any prompt or convention text changes. No repository Implementation Conventions row currently covers these Pi source paths.
- Use offline focused fixtures. No live child/Pi/provider launch, network/install/download, shared daemon or broad test sweep is authorized. No approved visual reference applies.

## Prior Decisions

- 260913-feat-ws-pi-delegated-write-scopes (2026-09-13, Decisions): "Persist the normalized immutable scope binding with the child registry/sidecar lifecycle." — bearing: constrains
- 9310cda9 (2026-09-24, commit): "Only the retry loop and its hooks were extracted; each caller keeps its own temp naming, mode," — bearing: constrains
- 185452a2 (2026-09-24, commit): "binding first and keeping everything from the guards to registration synchronous restores the invariant without changing what any guard does." — bearing: constrains
- 07486950 (2026-09-09, commit): "[fixed] C6 later metadata writes no longer propagate through updateOwnership" — bearing: constrains
- 260908-feat-ws-pi-agent-session-disk-retention (2026-09-09, commit): "Used Pi's current session identity and configured root; no fallback identity or default test root is permitted." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/agent-storage.ts, agents-plugin-pi/src/spawner.ts, agents-plugin-pi/test/agent-storage.test.ts, agents-plugin-pi/test/spawner.test.ts |
| scope.surface | cross-module | Exported allocateAgentHome is called by spawnAgent; no new tool argument or delegation store shape is required. |
| scope.new_public_symbol | no | None; reuse allocateAgentHome, AgentOwnership.delegation and DelegationPolicy. |
| scope.new_type_contract | yes | allocateAgentHome currently accepts context, agentId, role and optional exploreMode; passing delegation extends its internal exported signature (agents-plugin-pi/src/agent-storage.ts#L273-L285). |
| scope.test_surface | existing | agents-plugin-pi/test/agent-storage.test.ts#L11-L47 covers allocation and bounded diagnostics; agents-plugin-pi/test/spawner.test.ts#L853-L876 covers scoped admission. |
| complexity.reuse_points | confirmed | AgentOwnership already has optional delegation; writeOwnership locks and atomically replaces metadata; validDescriptor parses existing delegation (agents-plugin-pi/src/agent-storage.ts#L40-L43, agents-plugin-pi/src/agent-storage.ts#L391-L438). |
| complexity.side_effect_risk | moderate | The first persisted descriptor controls durable authority; write failure must precede registry.set and RpcClient.start, although channel binding and capacity guards already precede allocation (agents-plugin-pi/src/spawner.ts#L3823-L3837, agents-plugin-pi/src/spawner.ts#L3879-L3925). |
| risk.correctness | high | Ignoring the existing second-write result permits in-memory and durable delegation to differ; moving delegation must preserve initial-write failure propagation (agents-plugin-pi/src/agent-storage.ts#L283-L285, agents-plugin-pi/src/agent-storage.ts#L411-L414, agents-plugin-pi/src/spawner.ts#L3835-L3837). |
| risk.fit | moderate | Keep the synchronous guards-to-registration boundary and existing storage writer; broader checkpoint recovery is separate work (agents-plugin-pi/src/spawner.ts#L3826-L3884). |
| risk.test | moderate | Existing installRpcHarness stubs start and exposes onStart; new regression assertions must distinguish the first write from later lifecycle writes, not merely inspect final metadata (agents-plugin-pi/test/spawner.test.ts#L601-L618). |
| risk.security_or_contract | high | Delegation is durable write authority. The prescribed unchanged version-1 optional delegation shape supports T0 for this write-order fix: current readers accept absent or parsed delegation, updates preserve it, and reopening uses the same reader; no migration or recovery reconstruction is needed (agents-plugin-pi/src/agent-storage.ts#L13, agents-plugin-pi/src/agent-storage.ts#L400-L438, agents-plugin-pi/src/agent-storage.ts#L448-L454). |

## Phases

### Phase 1: Make initial spawn metadata complete and remove the redundant durability boundary

Implement the first-write contract in Decisions through the existing allocation/storage owner. Establish the confirmed two-write sequence with deterministic fixture instrumentation; do not block the structural fix on reproducing the historical transient warning or claim that its cause was proved. Pass delegation into `allocateAgentHome()`, persist it in the initial descriptor, and remove the immediate delegation-only update. Preserve unrelated allocation and lifecycle behavior.

Verification uses the existing focused harness for `test/agent-storage.test.ts` and `test/spawner.test.ts`:

- Assert normalized delegation in the returned descriptor and first persisted JSON before any later update, for scoped children; cover unchanged unscoped allocation.
- Assert spawn passes delegation into allocation and performs no immediate delegation-only follow-up write. Distinguish this removed write from legitimate later lifecycle writes.
- Inject initial persistence failure and assert rejection, no registry registration and zero child-start calls. Preserve channel cleanup and existing bounded-notification assertions.
- Run the touched allocation/spawn regressions and existing bounded diagnostic tests; read full output and reject empty selections. Run applicable narrow TypeScript/package checks without starting live sessions.

Record the actual fix and verification under Result, explicitly preserving the unverified status of the historical warning's cause. No follow-up investigation or warning-suppression scope is implied.

### Result (eaa7469fc) - 2026-11-15

- Added an optional trailing `DelegationPolicy` argument to `allocateAgentHome()` and included it in the initial descriptor. `spawnAgent()` passes its normalized policy into allocation and no longer mutates the returned descriptor or issues the immediate delegation-only update. The synchronous registration boundary and existing writer, locks, cleanup, diagnostics, readers and later updates remain unchanged. Regenerated `agents-plugin-pi/dist/index.js`.
- Before editing, offline rename instrumentation on the existing `spawn admission preserves omitted legacy authority` fixture confirmed the original sequence for both scoped and unrestricted children: write 1 omitted delegation and write 2 added it. The historical warning's underlying cause and its causal connection to that second write remain unverified; no live reproduction or warning suppression was attempted.
- Added allocation and spawn regressions that inspect initial JSON and returned authority before later writes, preserve allocation without delegation, require exactly one write before registration/start, retain legitimate lifecycle updates, and inject initial rename failure to prove rejection with zero registrations/starts and channel, lock and temporary-file cleanup.
- Verification: the bounded `node --test --test-reporter spec --test-name-pattern` selection in `test/agent-storage.test.ts` and `test/spawner.test.ts` passed 14/14, with zero failures or skips. It covered the new regressions, existing scoped/legacy admission and narrowing, bounded diagnostics, session path safety, Explore mode compatibility and ownership rename retry fixtures. The initial seven-test subset also passed 7/7. All runs unset `WS_PI_SPAWN_ROLE`, `WS_PI_EXPLORE_MODE`, `WS_PI_DELEGATION_POLICY` and `WS_PI_LEAD_MEMORY_LOG`.
- Checks: `node --check` passed for all four changed TypeScript files and the generated bundle; `node scripts/bundle.mjs` and `git diff --check` passed. No package TypeScript type-check script is configured. Existing cached dependencies were linked into the ignored local dependency directory; cached esbuild 0.28.1 produced only the intended source changes and bundle input-hash update. No install, download, network, shared-runtime operation, broad test sweep or live Pi/provider launch ran.
- Independent one-pass Lite review of `320a12c51..eaa7469fc` was clean, with no findings. The review inspected the source, tests and corresponding generated bundle code; it did not independently rerun tests/build or establish full binary bundle equivalence. Findings: `/Users/kang-sw/.cache/ws@kang-sw-devenv/proj/17da6bdc@2b250898/review-paths/e87c23f2-01-metadata-first-write-lite.md`.
- Decisions: retain the existing fourth `exploreMode` argument and add delegation fifth, preserving unrelated callers and T0 storage compatibility. The small storage-test leaf ran directly after the implementer playbook render was refused by an existing capability-ceiling mismatch; no ceiling changes, delegation retry, extra investigation or new idea ticket followed. Independent implementation review remained available and completed through the permitted scoped reviewer path.
