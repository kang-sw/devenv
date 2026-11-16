# Pi split map: `ask.ts`, `spawner.ts` and three test files

Planning asset for `261114-refactor-ws-split-oversized-source-files`. Phase 1's
binding rules, adapted to TypeScript ESM: compatibility T0 (internal module
layout only; no tool, schema, protocol, on-disk or caller-visible change);
move-only; no renames; no extracted helpers or wrappers; no logic changes; no
test redesign; declaration comments move with their declarations; only the
import/export scaffolding the move needs; one move-only commit per destination
file; no size-guard limit changes.

Snapshot: `616271b51`. The five targets are byte-identical at `c36321736` and at
`f66857935` (`git diff 616271b51 HEAD -- agents-plugin-pi/src agents-plugin-pi/test
agents-plugin-pi/scripts` is empty). Line ranges are 1-based and inclusive. A
range starts at the first line of a declaration's leading comment block (as
`ts.getLeadingCommentRanges` attaches it, so section banners travel with the
next declaration) and ends at the declaration's last line. Every top-level
non-blank line belongs to exactly one statement (checked).

**The `src/` sizes below come from a measured dry run** of this exact map in a
`git archive 616271b51` copy under `/tmp`, not estimates. The dry run passed the
module-load smoke test, an identical `tsc` error multiset, the
declaration-identity proof, an identical runtime export namespace for
`spawner.ts` (94 names) and `ask.ts` (64 names), bundle + native refresh, the Go
and Node freshness tests, and the full Pi suite with the same failure set as an
unsplit copy. Test-file sizes are estimates from the ranges (no dry run;
helper handling needs a decision first, see D3).

Destination names were checked: no `src/ask-*`, `src/spawner-*`,
`test/ask-*`, `test/spawner-*` or `test/goal-loop-*` file exists.

## Module facts that shape the split

- Importers. `src/spawner.ts` is imported by 19 src modules (including
  `index.ts`, `ask.ts`, `goal-loop.ts`, `agent-cost.ts` and `agent-view.ts`),
  by 44 test files, by dynamic `import()` in 4 tests, and by
  `test/fixtures/{nested-spawn-probe,usage-hop}.ts`. `src/ask.ts` is imported by
  `index.ts`, `agent-widget.ts`, `lead-bootstrap.ts` and 13 test files
  (`test/fork-lifecycle.integration.test.ts:36,38` imports both through
  `join(plugin, "src/...")`). **Recommendation: the original module re-exports
  every previously exported name it loses**, so no importer changes. There is
  precedent: `src/spawner.ts:121-122` (`import`+`export { ownerNotifyRef } from
  "./owner-notify.ts"`). Measured scaffolding: about 100 re-export lines in
  `spawner.ts`, about 70 in `ask.ts`.
- Type re-exports must be `export type { ... } from`. Node runs `.ts` by type
  stripping (package.json `"test": "... node --test"`; no tsconfig/tsc in the
  package). A value-form `export { SomeInterface } from` fails at ESM link time.
  Likewise every cross-module type import needs `type`.
- Module-private mutable state (`let`): all readers and writers stay together.
  `ask.ts:967-976` `activeQueueRepaint`/`activeQueueOverlayToken` are written
  only by `openLeadAskQueue` (2285) and read by `repaintActiveQueue` (978), so all
  three move to `ask-overlay.ts`. `spawner.ts:1181-1390` has ten `let`s
  (`leadInterrupt`, `boundaryTurnOwed`, `pushWakeReserved`, `wakeOptions`,
  `cancelWakeTimeout`, `publishOwnTurn`, `pushWakeAwaitingStart`,
  `failedPushWakes`, `settleWakeHandle`, `cancelSettleWake`). Their writers are
  1206, 1290, 1310, 1318, 1392, 1405, 1434, 1903 and 1966, and every reader is
  in 1206-2028. All of them move together into `spawner-push.ts`. No `let` is
  exported or written across modules. The splitter checks this
  (`LET-WRITE-ACROSS`: none).
- Cycles. The split adds value cycles: `ask.ts`<->`ask-overlay.ts`, and
  `spawner.ts`<->`spawner-push.ts`/`-launch.ts`/`-events.ts`/`-admission.ts`
  plus `spawner-push.ts`<->`spawner-launch.ts`. Cycles already exist
  (`spawner.ts`<->`agent-cost.ts`, `agent-view.ts`, `subtree-lifecycle.ts`,
  ...). They are safe because no top-level initializer, class heritage or
  top-level expression reads a binding across the new edges. The only
  cross-group initializer references are type arguments
  (`executionRegistries`, `persistenceBlockedRegistries`,
  `childLaunchPreflights`, `writerOperations` and `dispatchOrder` name
  `RpcAgentRegistry`/`RpcAgentRecord` as types). `ask-threads.ts`,
  `spawner-tools.ts` and `spawner-record.ts` are runtime leaves (type-only
  edges outward).
- Newly exported declarations (an `export ` keyword added, nothing else). They
  are NOT re-exported from the original module, so its public surface is
  unchanged (namespace check above).
  - ask (7): `persistThreads`, `refreshAgentWidget`, `notify`, `nowIso`,
    `repaintActiveQueue`, `bindThread`, type `LeadAskSessionManager`.
  - spawner (37): `executionRegistries`, `CHILD_BOOTSTRAP_OVERRIDE_ENVS`,
    `CHILD_MAILBOX_IDENTITY_ENVS`, `childLaunchPreflights`, `resolveSpawnModel`,
    `getAgentRpcSnapshot`, `triggerAgentWidgetRefresh`,
    `triggerAgentDashboardRefresh`, `triggerAgentCostRefresh`,
    `onDescendantUsageChanged`, `onAgentViewChanged`, `createTerminalDelivery`,
    `advanceForkFinish`, `finishResumeCtx`, `observeForkFinishEvent`,
    `issueDispatch`, `acceptDispatch`, `wasSupersededByAcceptedDispatch`,
    `stampWriter`, `acceptWriter`, `rollbackWriter`, `claimLeadOwnership`,
    `clearTerminalFacts`, `advanceWorkGeneration`, `clearLiveState`,
    `stoppedLaunches`, `claimLaunch`, `launchStoppedError`,
    `awaitExitedChildStdout`, `validateCwdOverride`, `awaitForkReadiness`,
    `awaitWebReadiness`, `captureForkSelection`, `applyModelEffort`,
    `verifyResearchSelection`, `resolveSpawnAdmission`, `harvestLastMessage`.
- `import.meta.url`: none in either file (`spawner.ts:2649` is a comment).
  `RPC_CLI_PATH` is `process.argv[1]`, so placement-neutral.
- Pruned imports. Each retained file drops import specifiers it no longer uses.
  Whole import declarations disappear from `ask.ts` (`node:fs`, `node:crypto`,
  `node:child_process`, pi-coding-agent, `bridge`, `model-catalog`, `fork`,
  `pi-tui`, `fork-context`, `atomic-write`, `lead-bootstrap`,
  `agent-usage-rollup`, `agent-storage`, `agent-cost`, `agent-telemetry`,
  `delegation-policy`). They also disappear from `spawner.ts` (`node:os`,
  `node:string_decoder`, `node:stream`, `mcp-stdio-client`, `background-tasks`,
  `fork-context`, `approval-protocol`, `owner-notify` (import only; the
  re-export at 122 stays), `agent-telemetry`, `write-scopes`, `push-protocol`,
  `adapter-label`, `summary-id`, `output-rate`, `model-observation`,
  `agent-batches`). The new siblings import those modules instead, so ESM
  evaluation order can shift. Harmless today (the suite is unchanged), but
  reviewers should know.

## Target 1: `agents-plugin-pi/src/ask.ts` (2582 lines)

`*` marks a module-private declaration.

### `src/ask-threads.ts`: lines 149-966 and 986-1001, 857 lines after the move

Thread model, registry IO and pure message builders. It is a runtime leaf.

- const: `ASK_TOOL_NAME`, `RESOLVE_TOOL_NAME`, `THREAD_SUMMARY_CUSTOM_TYPE`,
  `EXCERPT_WINDOW`, `EXCERPT_ENTRY_CHARS`, `MAX_CONTEXT_CHARS`,
  `EMPTY_SUMMARY_TEXT`.
- interface/type: `OverlayHandle`, `ThreadRecord`, `ThreadStatus`,
  `ThreadOrigin`, `PersistedForkResume`, `AskUiCtx`, `ThreadRegistryHandle`,
  `AskSessionCtx`.
- fn: `checkContextLength`, `formatSpawnTime`, `buildDoneSummaryPrompt`,
  `buildForkQuestionLeadNotice`, `normalizeThreadOrigin`,
  `buildThreadHeaderHint`, `nextThreadId`, `deriveThreadTitle`,
  `threadRegistryPath`, `serializeThreadRegistry`, `parseThreadRegistry`,
  `countPending`, `buildThreadListLines`, `mostRecentReopenable`, `isEntryLive`,
  `extractEntryText`, `buildVerbatimExcerpt`, `addAskToolsIfLead`,
  `buildDiscussionForkDirectiveText`, `buildDiscussionForkInitialMessage`,
  `buildInjectionMessage`, `buildQueuedAnswerInjectionMessage`,
  `captureForkResume`, `rehydrateForkRecord`, `resolveOwnerSendInterrupt`,
  `getLeafEntryId`, `captureAskCommitHash`, `buildAskAnchorLine`,
  `createThreadRegistryHandle`, `loadThreadRegistryFile`,
  `saveThreadRegistryFile`, `hydrateThreadRegistry`, `persistThreads*`
  (940-944), `refreshAgentWidget*` (946-965), `notify*` (986-988), `nowIso*`
  (990-992).

### `src/ask-overlay.ts`: lines 967-984 and 1454-2582, 1208 lines after the move

Queue-repaint state, respondent binding, `/done` actions, overlay handle, the
lead-ask queue modal, `openThread` and the thread commands.

- let: `activeQueueRepaint*`, `activeQueueOverlayToken*`. fn:
  `repaintActiveQueue*` (978-984).
- fn: `ensureRespondent` (1454-1578, 125 lines), `bindThread*`,
  `resolveDoneAction`, `summarizeThenClose`, `runDoneAction`,
  `buildOverlayHandle`, `resolveLeadAskEscapeAction`,
  `resolveLeadAskQueueEntryAction`, `runLeadAskEscapeAction`,
  `buildLeadAskQueueOnClose`, `collectLeadAskQueue`, `countQueueAnswered`,
  `buildQueueCoverageLine`, `buildQueueSubmitConfirmMessage`,
  `openLeadAskQueue*`, `openThread*` (2380-2518), `registerThreadCommands`.
- interface/type: `AskCustomUiCtx*`, `DoneAction`, `LeadAskEscapeAction`,
  `FocusableEditorLike`, `LeadAskQueueConfirmState*`, `LeadAskQueueOptions`.
- const: `QUEUE_SHIFT_TAB*`, `QUEUE_BORDER_OVERHEAD*`,
  `QUEUE_IDENTITY_EDITOR_THEME*`.
- class: `LeadAskQueueComponent` (1968-2283, 316 lines; kept intact).

### Retained `src/ask.ts`: 656 lines

The file header and imports (1-147) plus 1003-1452: `handleForkRaisedQuestion`,
`WithdrawOutcome`, `withdrawQueuedQuestion`, `registerAsk`,
`closeThreadOnDone`, `finishForkRaisedThread*`, `detachForkRaisedThread`,
`injectDiscussionSummary`, `LeadAskSessionManager*` (type),
`queuedAnswerContent*`, `deliverQueuedAnswers`, `deliverQueuedAnswer`,
`resolveChildLiveness`, `createForkChannel`. It also holds the re-exports of
every moved exported name.

Commit order: `ask-threads.ts`, then `ask-overlay.ts`. Each step passed load,
proof and `tsc` checks. Neither step edits the other new file.

## Target 2: `agents-plugin-pi/src/spawner.ts` (4618 lines)

### `src/spawner-tools.ts`: lines 140-432, 309 lines

Execution-persistence gate, tool groups, child launch preflights, and model/tier
resolution. Runtime leaf.

- const: `executionRegistries*`, `persistenceBlockedRegistries*`,
  `REPORT_TO_LEAD_TOOL_NAME`, `GATED_EXEC_TOOL_NAME`,
  `CHILD_BOOTSTRAP_OVERRIDE_ENVS*`, `CHILD_MAILBOX_IDENTITY_ENVS*`,
  `READ_ONLY_BUILTINS*`, `childLaunchPreflights*`, `TOOL_GROUPS`,
  `EXECUTE_WORKER_WS_TOOLS*`, `BACKEND_TO_PROVIDER`.
- type/interface: `ToolGroup`, `ResolveAgentCallToolClient`, `TierResolution`,
  `ResolvedModelInfo`.
- fn: `blockExecutionPersistence`, `assertExecutionPersistenceAvailable`,
  `registerChildLaunchPreflight`, `captureChildBootstrapFailure`,
  `resolveTools`, `tierResolution*`, `resolveModelForAliasViaWsMcp`,
  `resolveSpawnModel*`, `effectiveModelEffort`, `inheritModelFromToolCtx`,
  `storageContextFromToolCtx`, `resolveSpawnToolGroup`.

### `src/spawner-record.ts`: lines 453-1015, 580 lines

The agent record, telemetry snapshot and registry caps. It has type-only edges
outward.

- interface/type: `PendingApprovalState`, `RpcAgentRecord` (477-810, 334
  lines), `AgentContextStats*`, `ContextReading*`, `AgentReportLogEntry`,
  `RpcAgentRegistry`, `AgentStatus`, `SpawnAgentRole`.
- fn: `contextReading*`, `nextContextTokens*`, `getAgentRpcSnapshot*`,
  `refreshAgentTelemetry`, `resolveAgentId`, `resolveAgentRegistryCap`,
  `truncatePromptForStorage`.
- const: `REPORT_LOG_CAP`, `WS_PI_AGENT_REGISTRY_CAP_ENV`,
  `DEFAULT_AGENT_REGISTRY_CAP`, `PROMPT_STORAGE_CAP_BYTES`,
  `PROMPT_TRUNCATION_MARKER*`.

### `src/spawner-admission.ts`: lines 3464-3767, 318 lines

- fn: `applyModelEffort*`, `verifyResearchSelection*`, `reserveAgentAlias`,
  `lastActivityAt`, `evictForCapacity`, `runSpawnGuards`, `nextGeneratedAlias`,
  `callerDelegationPolicy`, `leadParentPolicy`, `requestedWriteCapability*`,
  `resolveSpawnAdmission*`, `spawnAdmission`.

### `src/spawner-events.ts`: lines 3013-3462 and 4307-4351, 521 lines

- interface: `RpcEventOutcome`, `ApprovalChannelHost`.
- fn: `markAgentOutput*`, `isStreamedAssistantOutput*`, `applyRpcEvent`,
  `questionReportOutcome`, `observeChildSubtree`, `attachApprovalChannel`,
  `attachEventListener` (3245-3462, 218 lines; kept intact), `messageText*`,
  `observeQueuedWorkBoundary*`, `assistantMessageText*`, `harvestLastMessage*`.
  The last four are used only by events and push code, never by the retained
  file.

### `src/spawner-launch.ts`: lines 434-451 and 2234-3011, 827 lines

The RPC engine banner and `RPC_CLI_PATH`, writer-operation ordering,
`promptAgent`, launch and liveness, spawn and resume contexts, fork and channel
readiness, `buildRpcClientOptions`, and reports.

- const: `RPC_CLI_PATH*` (434-451, with the "RPC-backed persistent-child
  engine" banner; its only user is `buildRpcClientOptions`, 2962; keeping it in
  `spawner.ts` would have added it to that module's namespace), `writerOperations*`,
  `dispatchOrder*`, `OWNER_TERMINAL_RETRY_DELAY_MS`, `stoppedLaunches*`,
  `EXITED_CHILD_STDOUT_WAIT_MS*`, `LIVENESS_PROBE_INTERVAL_MS`,
  `CHANNEL_HELLO_TIMEOUT_MS`, `CHANNEL_READINESS_TIMEOUT_MS`,
  `CHANNEL_LIVENESS_PROBE_MS*`.
- interface/type: `WriterOperation*`, `WriterOperationState*`,
  `SpawnAgentParams`, `WriteScopeDiagnostic`, `RpcSpawnCtx`,
  `ChannelLaunchOptions`, `RpcResumeCtx`.
- fn: `issueDispatch*`, `acceptDispatch*`, `wasSupersededByAcceptedDispatch*`,
  `projectWriter*`, `compactWriterOperations*`, `stampWriter*`, `acceptWriter*`,
  `rollbackWriter*`, `claimLeadOwnership*`, `promptAgent`,
  `clearTerminalFacts*`, `advanceWorkGeneration*`, `clearLiveState*`,
  `claimLaunch*`, `launchStoppedError*`, `recordedExitError*`, `childStdout*`,
  `awaitExitedChildStdout*`, `probeAgentLiveness`, `markAgentExited`,
  `startLivenessProbe`, `pushSpawnFailed`, `validateCwdOverride*`,
  `prepareForkLaunch`, `awaitChannelStage`, `awaitForkReadiness*`,
  `awaitWebReadiness*`, `validateForkReadiness`, `containedOwnedPath*`,
  `captureForkSelection*`, `buildRpcClientOptions`, `recordReport`,
  `syncOwnershipProtection`, `startOwnedSessionObserver`.

### `src/spawner-push.ts`: lines 1017-2232, 1243 lines

Lead push gate, wake/settle state machine, held push queue, terminal delivery
and fork finish. It holds all ten mutable `let`s.

- const: `PUSH_FAMILIES`, `leadIdleRef`, `leadCompactingRef`,
  `leadWakeStartPendingRef`, `untakenPushes*`, `ownTurnRef`,
  `PUSH_WAKE_BACKOFF_BASE_MS`, `PUSH_WAKE_BACKOFF_CAP_MS`,
  `agentWidgetRefreshRef`, `agentDashboardRefreshRef`, `agentCostRefreshRef`,
  `heldPushQueue`.
- let: `leadInterrupt*`, `boundaryTurnOwed*`, `pushWakeReserved*`,
  `wakeOptions*`, `cancelWakeTimeout*`, `publishOwnTurn*`,
  `pushWakeAwaitingStart*`, `failedPushWakes*`, `settleWakeHandle*`,
  `cancelSettleWake*`.
- type/interface: `PushFamily`, `PushDeliverAs`, `LeadCustomMessage*`,
  `WakeStartOptions`, `RawPushBatchMode`, `TerminalDelivery`,
  `ForkFinishOperation`, `HeldPush*`, `HeldRawSend*`,
  `HeldGoalReplacementResult`, `HeldGoalReplacement`, `HeldInput`.
- fn: `shouldPushToLead`, `isOwnerHeld`, `computeFanIn*`,
  `computeRunningStatusLine`, `hasRunningAgents`, `buildPushContent`,
  `markLeadInterrupted`, `isLeadInterrupted`, `interruptHolding*`,
  `sendIntoRun*`, `forgetTakenPush*`, `releaseAfterInterrupt*`,
  `syncOwnTurnOwed*`, `resetOwnTurn`, `clearWakeStart`, `reserveWakeStart`,
  `reserveAdapterPromptStart`, `buildPushWakeLine`, `isPushWakeLine`,
  `resetPushWakeBreaker*`, `pushWakeBackingOff*`, `scheduleSettleWake*`,
  `requestPushWake*`, `labelRawSend*`, `sendToLead`, `sendBatchToLead`,
  `admitPush*`, `triggerAgentWidgetRefresh*`, `triggerAgentDashboardRefresh*`,
  `triggerAgentCostRefresh*`, `onDescendantUsageChanged*`,
  `onAgentViewChanged*`, `isOwningAgentIdle`, `prependRecoveredPushes`,
  `enqueueHeldGoalReplacement`, `xmlEscape*`, `customContentText*`,
  `batchItemIdentity*`, `buildPushBatchContent*`, `heldActionState*`,
  `resolveHeldGoalReplacement*`, `materializeHeldInput*`, `sendPush*`,
  `submitHeldPushBatch*`, `flushHeldPushes`, `registerPushFlush`,
  `hasPendingTerminalDelivery`, `createTerminalDelivery*`, `startForkFinish`,
  `advanceForkFinish*`, `finishResumeCtx*`, `parkForkFinish*`,
  `observeForkFinishEvent*`, `pushToLead`.

### Retained `src/spawner.ts`: 1095 lines

The file header and imports (1-138, including the `ownerNotifyRef` re-export at
122), the re-export block, `WORKER_LIFECYCLE_GUIDE`, `spawnAgent` (3773-4008, 236
lines), `relaunchDelegation`, `sendToAgent` (4025-4305, 281 lines), `listAgents`,
`stopAgent`, `getAgentTranscriptPath`, `AgentToolsHandle`, and
`registerAgentTools` (4557-4618; it contains the
`    async stopAll(): Promise<void> {` line, so it must stay, see H2).

Commit order (each step measured: loads, proof holds, `tsc` multiset
unchanged): `spawner-tools` -> `spawner-record` -> `spawner-admission` ->
`spawner-events` -> `spawner-launch` -> `spawner-push`. Because of the cycles,
some later commits rewrite import lines of earlier new siblings (only import
lines change; non-import diff = 0): step 2 touches `spawner-tools.ts`; step 5
touches `spawner-admission.ts` and `spawner-events.ts`; step 6 touches
`spawner-record.ts`, `spawner-admission.ts`, `spawner-events.ts` and
`spawner-launch.ts`. `spawner.ts` stays above 2000 lines until step 5.

## Tests (estimates; blocked on D3)

The three test files import only through `../src/ask.ts`, `../src/spawner.ts`
and `../src/goal-loop.ts`. With the re-exports kept, they are independent of the
src split. Test files are not bundle inputs (see
`TestPiBundleInputHashCoversInputsOnly`, `pi_bundle_test.go:46-53`, which
asserts `test/util.test.ts` is excluded). So test commits need no
bundle/mirror regeneration. `node --test` runs every file under `test/`
(including `test/fixtures/*.ts`, which already hold shared helpers such as
`channel-child.ts` and `subtree-channels.ts`). Each file runs in its own
process, so module state cannot leak across files. Within a file, top-level
hooks reset shared `spawner.ts` state.

Shared, non-movable pieces (one declaration cannot serve two files without an
import, and hooks are statements, not declarations):

- `test/ask.test.ts:127-144` is a top-level `afterEach` that resets
  `agentWidgetRefreshRef`, `leadCompactingRef`, `heldPushQueue` and
  `leadWakeStartPendingRef`. Helpers: `capturePush` 146-154 (used by
  1127/1347/1502/2444/2559/2710) and `thread` 156-166 (used in every group).
- `test/spawner.test.ts:216-219` is an `afterEach` (storage-root cleanup) used
  only by the engine blocks. Shared helpers: `freshRpcRecord` 1344-1356 (used
  in every group), `liveRpcRecord` 1358-1367, `fakePi` 1369-1404,
  `fakeRpcClient` 1735-1756. Single-group helpers move with their users:
  `macrotask` 167-168 -> push; `TEST_EXTENSION_ENTRY` 176-178 + the local
  `buildRpcClientOptions` wrapper 179-183 -> registry; `tierCatalog` 308 ->
  tools; `REAL_EXTENSION_ENTRY`, `startRpcWithFakeChild`,
  `createEngineTestHarness`, `storageRoots`/`storageRoot` stay. Mid-file
  imports (for example 298-306 and through 500) move with the blocks they
  serve.
- `test/goal-loop.test.ts:663-3520` is ONE top-level `describe` (2858 lines).
  It cannot be kept whole (>2000). Its closure preamble 681-846 (`beforeEach`,
  `afterEach`, `configPath`, `fakeClock`, `fakePi`, `fakeCtx`, `carryHeading`,
  `exactCarry`, `assertCarry`, `reminderMarker`, `acknowledgeLatestReminder`)
  is used by every child. Splitting therefore requires repeating the
  `describe("registerGoalLoop IO glue (fake pi): compaction release (260906
  Phase 1)", () => {` wrapper with its doc comment (663-680) and the preamble
  verbatim in each new file. That keeps every test's full name identical. The
  top-level helpers 78-134 (`LABEL`, `labeled`, `prose`, `compactionEvent`,
  `tmpDir` + `after` hook 108-110, `writeConfig`, `fileReader`) are used only
  inside that describe, by every chunk.

Proposed files (sizes approximate):

| File | Blocks (snapshot lines) | ~Lines |
|---|---|---:|
| `test/ask-threads.test.ts` | 168-756 (tool names .. getLeafEntryId, 17 describes), 2191-2213, 2837-2847 | 670 |
| `test/ask.test.ts` (retained) | header/imports/hook/helpers + 758-1500 (handleForkRaisedQuestion, registerAsk, withdraw, closeThreadOnDone, deliverQueuedAnswer) | 910 |
| `test/ask-queue.test.ts` | 1502-2189 (escape/queue-entry actions, registerThreadCommands, collect/coverage, LeadAskQueueComponent) | 740 |
| `test/ask-overlay.test.ts` | 2215-2835 (ensureRespondent, summarizeThenClose, buildOverlayHandle, resolve/runDoneAction, integration) | 680 |
| `test/spawner.test.ts` (retained) | header/imports/engine helpers + 590-1296 (low-level engine, onModelResolved) | 950 |
| `test/spawner-tools.test.ts` | 226-588 (TOOL_GROUPS .. catalog validation), 1298-1342 | 440 |
| `test/spawner-events.test.ts` | 1406-1733 (applyRpcEvent, attachEventListener x3, WORKER_LIFECYCLE_GUIDE, report/exec), 3218-3476 (attachApprovalChannel) | 630 |
| `test/spawner-push.test.ts` | 1758-2456 (push gate .. pushSpawnFailed) | 735 |
| `test/spawner-registry.test.ts` | 2458-3216 (promptAgent .. runSpawnGuards) | 800 |
| `test/goal-loop-pure.test.ts` | 136-661 (all pure top-level describes; imports only) | 550 |
| `test/goal-loop.test.ts` (retained) | 1-135 + describe 663-1805 (preamble, explicit /goal stop 848-1109, release tests 1111-1805) | 1280 |
| `test/goal-loop-compaction.test.ts` | wrapper+preamble copy + 1807-2697 (settle timer, compaction ownership, lever pushes, no raw tail, triggers/backstops) | 1170 |
| `test/goal-loop-resume.test.ts` | wrapper+preamble copy + 2699-3519 (advisory/milestones, goal-less resume, adapter settings) | 1080 |

Dangling prose pointers (comments only, left as-is by a move-only rule; fix
afterwards if wanted): `test/lead-skills.test.ts:6,256` and
`test/audit.test.ts:12` point at blocks in `test/ask.test.ts` (`registerAsk
(fake pi)` stays there). `test/goal-loop.test.ts` 663-680 cites
`test/ask.test.ts`'s `closeThreadOnDone` block (stays). The ready ticket
`261115-feat-ws-worker-profile-session-spawn` cites `spawner.ts#L4612-L4665`.

## Bundle and native mirror: build, sync, guards

- `agents-plugin-pi/scripts/bundle.mjs` (esbuild, `npm run bundle`) writes
  `dist/index.js`, `dist/THIRD_PARTY_NOTICES` and `dist/eff_large_wordlist.txt`.
  Line 1 is `// ws-pi-bundle-inputs: sha256-<hash>` over every `src/**/*.ts|mjs`,
  the wordlist, `package-lock.json` and `bundle.mjs`. **Adding src files changes
  the hash**, so every src move commit must rebuild. Rebuilding at
  `616271b51` in a `/tmp` copy reproduced the committed bundle byte for byte
  (about 0.25 s).
- `python3 agents-plugin-pi/scripts/prepare-native-payload.py --first-party-only`
  (`refresh_first_party`, lines 225-252; about 0.5 s) validates the bundle
  hash, then rewrites both packages' `native-pi/first-party/`. Dry-run changes:
  `dist/index.js`, `bundle-inputs.json` and `first-party-manifest.json` (its
  `inputs` map lists every `agents-plugin-pi/src/*` hash, so new files appear
  there). `native-pi/closure.zip` and `closure-manifest.json` stay unchanged
  because they are copied (dependency closure unchanged). The full form
  without the flag runs `npm ci` and is unnecessary here. README.md:67-74
  documents the full form.
- Per src commit: `cd agents-plugin-pi && npm run bundle && cd .. && python3
  agents-plugin-pi/scripts/prepare-native-payload.py --first-party-only`, then
  stage `agents-plugin-pi/dist/index.js`,
  `agents-plugin{,-wsflow}/native-pi/first-party/{dist/index.js,bundle-inputs.json}`
  and `agents-plugin{,-wsflow}/native-pi/first-party-manifest.json` with the
  move.
- Guards: `agents-plugin-pi/test/bundle-freshness.test.mjs` (Node: header hash,
  both manifests, both native headers, bundle-inputs, notices). Go
  `TestPiBundleCommittedIsFresh` (`internal/wsagent/pi_bundle_test.go:13`) plus
  `TestPiPrepared*` (`pi_payload_review_test.go`, `pi_payload_test.go`).
  Python `agents-plugin/tests/test_native_pi_payload.py`
  (`test_both_archives_and_materializers_agree`, `check_source` drift) and
  `test_daemon_workflow_bundle.py`.
- Behavior equivalence of the bundle. A byte diff is not reorder-only: esbuild
  re-numbers collision suffixes (`messageText2`, `readFileSync12`) when module
  order changes (raw statement-multiset diff: 160/162 statements). With
  collision suffixes normalized (`/\b([A-Za-z_$]+)[0-9]+\b/` -> `$1`), the
  multiset of top-level bundle statements differs only in merged `import ...
  from "node:*"` lines (5 vs 7). That is an advisory check. The authority is
  the full Pi suite (tests load `src/` directly) plus the declaration proof.

## Hazards

- H0 (precondition, found in this pass). HEAD `f66857935` regenerated
  `agents-plugin-pi/workflow-bundles/*` without refreshing the native copies.
  `--first-party-only` on clean HEAD rewrites 8 native files
  (`first-party/workflow-bundles/{manifest.json,ws.zip,wsflow.zip}` and both
  manifests). In-tree `test_native_pi_payload.py` fails 6 + 1 at HEAD with
  `FIRST-PARTY stale: agents-plugin-pi/workflow-bundles/manifest.json`. Land
  that refresh as its own commit first, or the first move commit absorbs
  unrelated payload changes.
- H1. Unmerged branches edit `spawner.ts`: `exec5/hire-child-ceiling`,
  `impl/develop/shove-stem-disk` (both 2026-11-15). Run the split at a quiet
  point; refresh this map's line ranges against the execution base.
- H2. `test/mailbox-bootstrap.integration.test.ts:68-71` patches
  `src/spawner.ts` text and asserts exactly one `    async stopAll():
  Promise<void> {` (`spawner.ts:4606`). `registerAgentTools` must stay in
  `spawner.ts`. (That test already fails later at baseline on macOS: "mailbox
  waiter never started".)
- H3. Type re-exports/imports need `type` (Node type stripping). There is no
  in-package typechecker, so use the cached-tsc gate below.
- H4. Mutable `let` bindings: `spawner.ts:1181-1390` and `ask.ts:967-976` must
  move with all their writers (done in this map). Never add an `export` to a
  `let`.
- H5. New value cycles (see above). Do not add a top-level initializer that
  reads across them.
- H6. Pruned imports shift ESM evaluation order (see above).
- H7. The size guard's soft threshold (>1000) will flag `spawner-push.ts`
  (1243), `ask-overlay.ts` (1208) and `spawner.ts` (1095) at merge. That is
  informational. No hard (>=2000) result remains.

## Gates (macOS, measured on this PC)

1. Pi suite: `cd agents-plugin-pi && npm test`. 55 s in-tree; 2685 tests, 2664
   pass, 17 fail at baseline (environmental: workflow-forwarder `/private/var`
   vs `/var` x5, fork-lifecycle x3, agent-channel.integration x2,
   fork-review-regressions C2/C2a, mailbox-bootstrap, hosted-session,
   child-tool-inheritance, lead-skills typed `/skill:`,
   workflow-bundle-consumer). Gate: sorted, timing-stripped spec lines
   (`^\s*(✔|✖|﹣|▶)`) identical before and after. Verified identical (3547
   lines, 0 diff) on the src dry run. For test splits this also proves the
   test-name multiset is unchanged.
2. Typecheck (no package script): `node
   ~/.npm/_npx/5abcb55351612887/node_modules/typescript/bin/tsc --noEmit
   --allowImportingTsExtensions --module nodenext --moduleResolution nodenext
   --target esnext --skipLibCheck --verbatimModuleSyntax --erasableSyntaxOnly
   src/index.ts` (about 2.5 s; cached tsc 5.9.3). Baseline has 58 errors. Gate:
   the error multiset with `file(line,col)` stripped is identical.
3. Bundle and mirror: the per-commit commands above, then `node --test
   test/bundle-freshness.test.mjs` (<1 s).
4. Go: `cd agents-plugin-tool && GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go
   test ./internal/wsagent -run 'TestPiBundle|TestPiPrepared|TestPiLead'
   -count=1` (about 34 s; the freshness test alone takes 1.4 s). Passes at base
   and after the split. Note: Phase 1's Decisions say Go gates run on w5090
   only. Confirm whether that applies to this machine (D5).
5. Python: `python3 -m unittest agents-plugin/tests/test_native_pi_payload.py
   agents-plugin/tests/test_daemon_workflow_bundle.py` (about 50-60 s). Run
   `test_native_pi_pruning.py` separately: it imports `test_native_pi_payload`
   as a top-level module. In-tree this currently fails (H0). In a clean
   export, `test_producer_inventory_and_common_first_party_are_complete` fails
   with "workflow bundle stale" (environmental there). Gate: same result as
   baseline.
6. Declaration identity (below), per commit.

## Declaration-identity proof

For each move commit, compare the parent's file against the child's set
{original, all new siblings}. The multiset of top-level non-import statements,
each taken as leading-comment text plus body with one leading `export `
stripped, must be byte-identical. Exported names may only grow, by exactly the
listed newly exported names. Import/export-from statements are reported as
scaffolding. Dry-run result: ask 95=95, spawner 220=220, 0 mismatches, no
exports lost. Also compare `Object.keys(await import(<module>))` before and
after (must be equal).

```js
// node declproof.cjs <before.ts> <after.ts>...  (TypeScript compiler API, parse only)
const ts = require(process.env.HOME + "/.npm/_npx/5abcb55351612887/node_modules/typescript");
const fs = require("fs");
function collect(files) { const m = new Map(), exported = new Set();
  for (const f of files) { const text = fs.readFileSync(f, "utf8"); const sf = ts.createSourceFile(f, text, ts.ScriptTarget.Latest, true, ts.ScriptKind.TS);
    for (const s of sf.statements) { if (ts.isImportDeclaration(s) || ts.isExportDeclaration(s)) continue;
      const r = ts.getLeadingCommentRanges(text, s.getFullStart()) || []; const from = r.length ? r[0].pos : s.getStart(sf);
      let body = text.slice(s.getStart(sf), s.getEnd());
      if ((ts.getModifiers?.(s) || []).some(x => x.kind === ts.SyntaxKind.ExportKeyword)) { body = body.replace(/^export /, "");
        (ts.isVariableStatement(s) ? s.declarationList.declarations.map(d => d.name.getText(sf)) : [s.name?.getText(sf)]).forEach(n => exported.add(n)); }
      const key = text.slice(from, s.getStart(sf)).replace(/^\s+/, "") + body; m.set(key, (m.get(key) || 0) + 1); } }
  return { m, exported }; }
const [before, ...after] = process.argv.slice(2); const A = collect([before]), B = collect(after); let bad = 0;
for (const [k, n] of A.m) if ((B.m.get(k) || 0) !== n) { bad++; console.log("MISSING/CHANGED:", k.slice(0, 120)); }
for (const [k, n] of B.m) if ((A.m.get(k) || 0) !== n) { bad++; console.log("EXTRA/CHANGED:", k.slice(0, 120)); }
const lost = [...A.exported].filter(n => !B.exported.has(n)), added = [...B.exported].filter(n => !A.exported.has(n));
console.log(`mismatches=${bad} exports lost=${lost.join(" ") || "none"} added=${added.join(" ")}`); process.exit(bad || lost.length ? 1 : 0);
```

For test files, use the same script plus gate 1's spec-line multiset. Duplicated
hooks and preambles (D3) will appear as EXTRA entries; list them explicitly in
the commit's AI Context.

## Recommendation and decisions needed

- Two phases, either order, which may run in parallel on separate branches:
  (A) the src split, 8 move commits (ask 2, spawner 6), each with
  bundle + native refresh; (B) the test splits, 10 move commits, no
  regeneration. The phases touch disjoint files.
- D1. Keep re-exports in `ask.ts`/`spawner.ts` (recommended; otherwise 19 src
  and 44+ test importers change, which is not move-only).
- D2. Allow adding `export` to the 44 previously private declarations and
  import-only edits to earlier sibling files in later commits (the TS analogue
  of Go's same-package visibility).
- D3. Test helpers and hooks. Option (a): move each shared helper declaration
  once into `test/fixtures/{ask-threads,spawner-records,goal-loop}.ts` (move
  plus `export`, following the fixtures precedent), and duplicate verbatim only
  the statements that cannot be shared (the `afterEach` at `ask.test.ts:127-144`
  into each ask file; `goal-loop.test.ts:108-110` `after`; the goal-loop
  describe wrapper and preamble 663-846). Option (b): duplicate every needed
  helper verbatim. Either way, goal-loop needs about 184 lines of verbatim
  duplication twice. The only way to avoid it is an owner-approved
  `-ws-size-guard` exemption for `goal-loop.test.ts`, which the ticket's
  no-limit-change rule seems to exclude.
- D4. Include regenerated `dist/` and native mirrors in each src move commit
  (recommended, so every commit stays green), instead of one trailing
  regeneration commit.
- D5. Confirm local Go test runs are allowed on this macOS PC (Phase 1 says
  w5090 only).
- H0's native refresh must land first.
