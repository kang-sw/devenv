# MCP test split map: `server_test.go`, `session_state_test.go`, `playbook_tools_test.go`

Planning asset for `261114-refactor-ws-split-oversized-source-files`. Same
binding rules as Phase 1's Decisions: compatibility T0, move-only within
`package mcp` (all three targets and every destination are internal
`package mcp` test files; the package has no `package mcp_test` files), no new
packages, no renames, no extracted helpers or wrappers, no test redesign,
declaration comments move with their declarations, only package/import
scaffolding and `gofmt` changes, one move-only commit per destination file, no
size-guard limit changes.

Snapshot: `616271b51`. The three targets are byte-identical at `c36321736`
(`git diff 616271b51 c36321736 -- agents-plugin-tool/` is empty). Line ranges
are 1-based and inclusive in the snapshot file, from the first line of a
declaration's doc comment to its closing brace; a free-floating section banner
that heads a moved section is included in the range and moves with it. Each
range is bounded by blank lines; the separating blank line is removed with the
range. Sizes are measured, not estimated: this exact map was dry-run in a /tmp
export of the tree (`git archive 616271b51`), with per-file import pruning and
`gofmt`; `go vet ./internal/mcp` passed after every one of the nine steps in the
order listed, and every `Test*` function originally in the three files passed
afterward (the `-run` regex is described under Gates).

This group touches no non-test file. It never edits `server.go` and does not
depend on where Phase 1 puts non-test declarations: test files reference
package identifiers by name only.

## Destination names

No existing file in `internal/mcp` has any of these names, and none ends in a
GOOS/GOARCH suffix. Names deliberately avoid `server_config_test.go`,
`server_git_test.go` and `server_exec_test.go`: Phase 1 creates
`server_config.go`, `server_git.go` and `server_exec.go`, and these tests drive
the whole `ServeStdio` surface, not just the declarations in those files. The
`server_stdio_*` prefix keeps them listed next to `server_test.go` without
implying a 1:1 sibling.

## Target 1: `server_test.go` (3459 lines -> 1285 retained)

### `server_stdio_config_test.go`: 806 lines

Ranges: 134-188, 243-261, 1191-1818, 1993-2078.

- Tests: `TestServeStdioConfigShow`, `TestServeStdioConfigAgentsTier`,
  `TestServeStdioConfigAgentsTierUsesDetectedHarness`,
  `TestServeStdioConfigAgentsTierUsesDetectedPiHarness`,
  `TestServeStdioConfigResolveAgentUsesDetectedPiHarness`,
  `TestServeStdioConfigResolveAgentFallsBackToDefault`,
  `TestServeStdioConfigResolveAgentNoDetectedHarnessFallsBackToDefault`,
  `TestServeStdioConfigResolveAgentUsesDefaultCodexTierTable`,
  `TestServeStdioConfigResolveAgentExplicitHarnessOverridesDetected`,
  `TestServeStdioConfigResolveAgentRejectsUnknownTier`,
  `TestServeStdioConfigAgentsTierAcceptsHarnessCaseInsensitively`,
  `TestServeStdioConfigAgentsTierScopesWriteAndResolve`,
  `TestServeStdioConfigAgentsTierAcceptsTierSynonyms`,
  `TestFormatConfigViewQuotesControlCharactersInEffort`,
  `TestServeStdioConfigAgentsTierRoundTripsArbitraryEffortLabels`,
  `TestServeStdioConfigAgentsTierOmittedEffortClearsExistingEffort`,
  `TestServeStdioInitializeDetectsClaudeHarnessForAgentAlias`,
  `TestServeStdioCodexMetadataDetectsHarnessForAgentAlias`,
  `TestDetectHarnessFromInitializeParamsPrecedence`.
- Type: `resolveAgentTierResponse` (1261-1268; every user is in this file,
  including the two harness-detection tests at 2012 and 2039).
- Imports: bytes, context, encoding/json, fmt, os, path/filepath, strings,
  testing, wsconfig.

### `server_stdio_git_test.go`: 915 lines

Ranges: 673-752, 2179-2995.

- Tests: `TestActiveImplTicketAndGitStatusFormatting`,
  `TestActiveImplTicketFailsClosedOnUnreadableInventory`,
  `TestServeStdioGitStatusImplTicketContract`, `TestServeStdioGitToolCalls`,
  `TestServeStdioGitCommitUpdatedTicketsPresence`,
  `TestServeStdioGitCommitRefusesBranchMismatch`,
  `TestServeStdioGitCommitSurfacesTicketVerifyWarningsAsAdvisories`,
  `TestVerifyAdapterAppendsAmendRecipeToFixAdvisoriesOnly`,
  `TestFormatTicketVerifyRendersAdvisoriesWithoutAmendRecipe`,
  `TestVerifyAdapterDegradesToSilenceOnGraphLoadFailure`,
  `TestServeStdioGitCommitSurfacesTicketGraphParentBoard`,
  `TestServeStdioGitCommitAIContextConditionsAndDebugEvent`,
  `TestServeStdioGitCommitRejectsNonArrayAIContext`.
- Const: `unresolvedPhaseTicketBody` (2411-2422).
- Functions: `ticketGraphAdvisoryFixture`, `aiContextDebugEvents`,
  `newDebugEvents` (file-local; all users move).
- Imports: bytes, encoding/json, fmt, os, path/filepath, runtime, strings,
  testing, wsdoc, wsgit, wskey.

### `server_stdio_exec_test.go`: 493 lines

Ranges: 2080-2156, 3063-3459.

- Tests: `TestServeStdioDoesNotBlockToolsListBehindLongCall` (it uses the exec
  helpers below, so it moves with them), `TestExecToolsListNoAgentAndMCPFlow`,
  `TestExecMCPRunningLargeAndAbort`,
  `TestExecMCPResultReadableJSONStdoutAndTimeout`, `TestMCPExecHelperProcess`.
- Type: `execToolResponse`.
- Functions: `reapExecKeys`, `execKeyFromText`, `toolCallLine`,
  `mcpShellShapeArgs`, `mcpLongShellArgs`, `mcpAbortShellArgs`,
  `mcpLargeShellArgs`, `containsResolvedPath` (file-local; all users move).
- Imports: bufio, bytes, context, encoding/json, fmt, io, os, path/filepath,
  regexp, runtime, strings, testing, time.

### Retained `server_test.go`: 1285 lines

Snapshot lines 1-133, 190-242, 263-672, 754-1190, 1820-1992, 2158-2178,
2997-3062.

- `TestMain`, `runTestMain` (stay; the package's only `TestMain`).
- Shared helpers (all stay; most are used by many other test files):
  `toolPropertiesByName`, `toolEntryTextByName`, `toolNameListed`,
  `canonicalTestPath`, `serveStdioWithSession`, `withSessionKeyInToolCalls`,
  `useLeadProfile`, `mustWrite`, `assertStringAbsentFromTree`,
  `snapshotTreeForTest`, `initGit`, `headBranch`, `initTicketRepo`,
  `callToolsList`, `responseLinesByID`, `rawIDForTest`, `toolIsError`,
  `toolText`, `runGit`, `runGitOutput`.
- Tests: `TestServeStdioDefaultsToLeadToolsWithoutRootAuthorityDetection`,
  `TestPromptRenderToolRemoved`, `TestNamespaceTermsSubstitution`,
  `TestWsflowPlaybookRenderAllLegacyStemsFromRsrc`,
  `TestServeStdioLogsCancellationNotificationsWhenEnabled`,
  `TestServeStdioExposesCancellationNotificationsInDebugEvents`,
  `TestKeyedScopeGatesRestrictedTools`,
  `TestExplicitAllowedToolsCannotBypassEffectiveRole`,
  `TestServeStdioTicketToolsRejectSpecStemArgument`,
  `TestServeStdioTicketsQueryPointResolveMatchesRemovedStatusShape`,
  `TestTicketsQueryPointResolveIncludesTerminalStatuses`,
  `TestPathGenerateAdvertisesAndAllocatesPlanPaths`,
  `TestServeStdioPingPreservesIDsAndIsNotAdvertised`,
  `TestServeStdioToolsListAndCall`,
  `TestServeStdioNoAgentModeHidesAgentBackedTools`,
  `TestWsflowModePlaybookRenderAbsorbsPromptRenderContext`,
  `TestPlaybookRenderReturnsResolvedNativeBindings`,
  `TestWsflowConfigAgentsTierWriteRoundTripsIntoPlaybookRenderRecommendedModel`,
  `TestSubqueryToolRemovedFromListAndCallRejected`, `TestAPIListDomains`,
  `TestAgentBackedAPIToolsRemovedFromMCP`.
- Imports after all three moves: drops bufio, io, runtime, time, wsdoc, wsgit,
  wskey; keeps bytes, context, encoding/json, fmt, os, os/exec, path/filepath,
  reflect, regexp, strconv, strings, testing, wsconfig.

## Target 2: `session_state_test.go` (4084 lines -> 757 retained)

### `implement_todos_test.go`: 440 lines

Ranges: 43-406, 1482-1549.

- Tests: `TestDeriveImplementTodos`, `TestDeriveImplementTodosFromVerdictTitles`,
  `TestDeriveImplementTodoInstructionsPrepGuardrails`,
  `TestImplementReviewStructuredWrappers`,
  `TestDeriveImplementTodoInstructionsPartitionedReview`,
  `TestDeriveImplementTodoInstructionsBarePartitionedReviewFallback`,
  `TestDeriveImplementTodoInstructionsFinalGate`,
  `TestDeriveImplementTodoInstructionsMergeConfirmSkip`,
  `TestDeriveImplementTodoInstructionsRecreate`,
  `TestDeriveImplementTodoInstructionsBranchStop`,
  `TestDeriveImplementTodoInstructionsCriticalStop`,
  `TestDeriveImplementTodoInstructionsLiteReview`.
- Functions: `todoByKey`, `requireInstruction`, `implementReviewRoundWants`,
  `implementReviewRoundForbidden` (only users are this file and
  `route_resolve_implement_test.go`; same package, so placement is free).
- Imports: strings, testing.

### `route_resolve_implement_test.go`: 749 lines

Ranges: 909-1096, 1265-1276, 1316-1480, 1551-1832, 3986-4073 (3986-3992 is
the free-floating binding-anchor section banner; it moves with its section).

- Tests: `TestRouteResolveImplementSchemaIsOpaque`,
  `TestRouteResolveWrappedCallsMatchTypedVerdictAndState`,
  `TestRouteResolveWrappedMalformedEnvelopeDoesNotMutateState`,
  `TestRouteResolveWrappedUsesOuterSessionKey`,
  `TestEnterImplementNewSchemaReturnsVerdictAndStoresAgenda`,
  `TestEnterImplementTicketTargetTodosCarryNoPlanningStage`,
  `TestEnterImplementAllocatesSingleReviewForBoundedPublicExistingTestChange`,
  `TestEnterImplementSkippedDocsOmitsDocTodos`,
  `TestEnterImplementUnbornRepositoryUsesStandardCreatePath`,
  `TestEnterImplementNearMissesPreserveStandardBranchAndMergeTodos`,
  `TestEnterImplementStopsOnImplementBranchWithoutMergeTarget`,
  `TestEnterImplementNewImplPrefixBranchTargetExists`,
  `TestEnterImplementCreatePathMergeRootRefConflictDetectedByRealGit`,
  `TestEnterImplementTypedPathRendersDeclaredBindingAnchor`.
- Consts: `bindingAnchorTestPath`, `bindingAnchorTestTopics`,
  `bindingAnchorTestClause` (3994-3999, ungrouped; keep as written).
- Functions: `assertRouteResolveSchemaIsOpaque`, `routeStateSnapshot`,
  `readTodoInstruction`, `writeBindingAnchorAGENTS`, `writeSectionlessAGENTS`,
  `readPrepInstruction`, `assertPrepAnchorClause` (file-local; all users move).
- Imports: encoding/json, os, path/filepath, strings, testing.

### `tickets_mutate_stdio_test.go`: 600 lines

Range: 1834-2422.

- Tests: `TestServeStdioTicketsCreateUsesResolvedSageReviewConfig`,
  `TestServeStdioTicketsCreateEpicIgnoresDesignKnob`,
  `TestServeStdioGlobalSageReviewDesignAutoRequiresTicketBoundaryReview`,
  `TestServeStdioTicketsCreateEmptyStatesSkeletonCaveat`,
  `TestServeStdioTicketsMoveToReadyStatesSageGateNextInstruction`,
  `TestServeStdioTicketsCloseUnresolvedPhaseStatesSoftWarnNextInstruction`,
  `TestServeStdioTicketsCloseMergeReviewNudgeOnUnmergedImplBranch`,
  `TestServeStdioTicketsCloseWarnsWhenImplMergeStateUnverifiable`,
  `TestServeStdioTicketsCloseNoMergeReviewNudgeWhenImplBranchClean`,
  `TestServeStdioTicketsCloseNoMergeReviewNudgeOnPlainBranch`,
  `TestServeStdioTicketsMoveToReadyUnresolvedPostureWarnsInResponse`,
  `TestServeStdioTicketsMoveNonReadyBlockedRejectsInResponse`,
  `TestServeStdioTicketsMoveNonReadyBlockedSurfacesPartialMutationNotice`,
  `TestServeStdioTicketsCreateEmptyReadyUnresolvedPostureWarnsInResponse`,
  `TestServeStdioTicketsMoveDefaultsToSkippedDesignReview`,
  `TestServeStdioTicketsMoveExplicitOverrideWinsOverBuiltinDefault`.
- Types/vars/consts/helpers: none.
- Imports: os, path/filepath, strings, testing, wsconfig.

### `todo_agenda_tools_test.go`: 878 lines

Range: 2424-3289.

- Tests: `TestServeStdioTodoKeyNormalization`, `TestServeStdioTodoAddErrorBranches`,
  `TestServeStdioTodoAddReproducesOldPlacements`,
  `TestServeStdioTodoInstructionReadSurface`,
  `TestServeStdioTodoListInstructionRendering`,
  `TestServeStdioTodoCheckCheckpointRendering`,
  `TestTodoCheckToolSchemaHasNoFormat`, `TestServeStdioTodoReorderHandler`,
  `TestServeStdioAgendaHandler`, `TestServeStdioAgendaListHandler`,
  `TestServeStdioAgendaSetMerge`,
  `TestRenderSessionStateAgendaSizeNoteThreshold`,
  `TestRenderSessionStateAgendaSizeNoteCountsResolverBlobs`,
  `TestServeStdioAgendaSetSizeNote`, `TestServeStdioTodoListRequiresSessionKey`.
- Function: `sizedBlob`. Const: `agendaNotePrefix` (file-local).
- Imports: bytes, context, encoding/json, fmt, path/filepath, strings, testing.

### `workflow_manual_state_test.go`: 707 lines

Range: 3291-3984 (includes the three free-floating section banners at 3291,
3387 and 3752).

- Tests: `TestStripModeGatedRegion_NoMarker`, `TestStripModeGatedRegion_KeepContent`,
  `TestStripModeGatedRegion_StripContent`,
  `TestStripModeGatedRegion_MultipleLines`,
  `TestStripModeGatedRegion_MultipleRegions`,
  `TestStripModeGatedRegion_UnclosedMarker`, `TestWorkflowManualFreshMode`,
  `TestWorkflowManualFreshModeWithRoot`, `TestWorkflowManualKeylessRejected`,
  `TestWorkflowManualDelegateKeyBlocked`, `TestWorkflowManualContinueMode`,
  `TestWorkflowManualTodoInstructionPreview`, `TestWorkflowManualUnknownKey`,
  `TestWorkflowManualGitCommitReinjection`, `TestAppendSessionKeyTip`,
  `TestWorkflowStateReturnsSessionStateOnly`,
  `TestWorkflowStateByteIdenticalToWorkflowManualWhenNotesPresent`,
  `TestWorkflowStateEmptySessionReturnsEmptyPayloadNotError`,
  `TestWorkflowStateKeylessRejected`,
  `TestWorkflowStateDelegateKeyBlockedSameAsWorkflowManual`,
  `TestWorkflowStateUnknownKeySameFailLoudAsWorkflowManual`,
  `TestWorkflowStateFreshSentinelIsFailLoudNotFresh`,
  `TestWorkflowStateToolSchema`.
- Function: `callToolNoKey` (file-local).
- Imports: bytes, context, encoding/json, fmt, os, path/filepath, strings,
  testing.

### Retained `session_state_test.go`: 757 lines

Snapshot lines 1-42, 408-908, 1098-1264, 1278-1315, 4075-4084.

- Shared helpers (stay): `keysOf`, `eqKeys` (used by
  `implement_resolver_test.go`, `implement_route_facts_test.go`),
  `stringPtr`, `implementReadyArgs`, `implementSkipDocsArgs`,
  `implementReadyFacts`, `writeImplementReadyTicket`, `callToolLineWithKey`,
  `containsString`, `containsAnyString`, `nonEmptyLines`, `callToolWithKey`
  (used by ~40 test files), `enableReviewPhase`, `newSandboxStore`.
- Tests: todo model/render tests 408-778 (`TestTodoKeyUniquenessAndReuse`
  through `TestParseTodoStatus`), store tests 780-907
  (`TestStoreConcurrentTodoWrites`, `TestStoreAgendaRoundTrip`,
  `TestStoreMergeAgendaKeepsUntouchedNumbers`, `TestStoreClearAllAgenda`,
  `TestEnterModeReplacesTodos`), `TestServeStdioSessionStateFlow`.
- Banners `// --- store-level tests ...` (780) and `// --- MCP integration ...`
  (1236) stay.
- Imports after all five moves: drops os.

## Target 3: `playbook_tools_test.go` (2234 lines -> 1292 retained)

### `playbook_golden_test.go`: 953 lines

Range: 1285-2223 (from the `Golden render: real agents-plugin/rsrc tree`
banner through `TestNonClaudeRendersOmitEffortAgents`; includes the banners at
1285, 1381, 1497, 1550, 1648 and 2090).

- Tests: `TestPlaybookPrintGoldenDelegateSampleClaudeHarness`,
  `TestPlaybookPrintGoldenDelegateSampleCodexHarness`,
  `TestPlaybookPrintGoldenDelegateSampleUnknownHarness`,
  `TestPlaybookPrintGoldenSamplePlaybookNoDelegation`,
  `TestPlaybookPrintGoldenExploreClaudeHarness`,
  `TestPlaybookPrintGoldenExploreCodexHarness`,
  `TestPlaybookPrintGoldenExploreUnknownHarness`,
  `TestPlaybookPrintGoldenExploreJunkHarness`, `TestTerminologyTableCoverage`,
  `TestTermsDifferThreeWay`, `TestReservedToolVarNamesContainsRequiredNames`,
  `TestPlaybookPrintGoldenLeadCheckBlockers`,
  `TestPlaybookPrintRetiredSpecStemsGone`,
  `TestPlaybookPrintGoldenLeadWorkflowManual`,
  `TestPlaybookPrintGoldenLeadTicket`, `TestRetiredLeadPlaybooksNoLongerResolve`,
  `TestPlaybookPrintGoldenLeadShip`, `TestPlaybookDocumentAuditProducts`,
  `TestPlaybookPrintGoldenLeadDiscuss`, `TestPlaybookPrintGoldenLeadReview`,
  `TestSkillAuthoringRelocatedOutOfRsrc`, `TestSkillsCallEnterTools`,
  `TestPhase3bSkillRepoint`, `TestPlaybookPrintGoldenLeadBootstrap`,
  `TestRenderMintsLeadScopedChildKeyForWorkerPlaybook`,
  `TestPlaybookRenderGoldenTicketWorker`,
  `TestPlaybookPrintLeadRunWorkerTierPolicy`,
  `TestLeadRunTicketQueryRiskProjection`,
  `TestClaudeSpawnIdiomRendersAtEverySite`,
  `TestSpawnIdiomSitesListIsComplete`,
  `TestClaudeSpawnIdiomLevelsMatchGeneratedAgents`,
  `TestLeadWorkflowManualClaudeSpawnBindingOverlay`,
  `TestNonClaudeRendersOmitEffortAgents`.
- Const: `workerPlaybookContent` (1926-1937). Var: `spawnIdiomSites`
  (2094-2095; a constant string-slice literal with no initialization
  dependency). Function: `wantClaudeSpawnIdiom`. All file-local.
- Imports: encoding/json, os, path/filepath, reflect, strings, testing,
  claudeagents, wsconfig.

### Retained `playbook_tools_test.go`: 1292 lines

Snapshot lines 1-1284 and 2225-2234: the test-helper block and fixture consts
(`buildTestRsrcTree`, `newTestServerWithHarness`,
`isolatedPlaybookConfigOptions`, `shippedImplementerContext`, `initGitRepo`,
`asPlaybookError`, `writeTestFile`, `plainPlaybookContent`,
`delegatePlaybookContent`, `modelAliasPlaybookContent`,
`tierModelPlaybookContent`, `noVarsPlaybookContent`), fixture-based
`playbook.read`/`render` tests, the MCP-dispatch tests, and
`mustOverrideLookup`. Imports drop reflect and claudeagents.

Optional, not in the primary plan: cutting the MCP-dispatch section
(804-1283, ~480 lines) out as a third file would bring the retained file under
~820. It is not needed for the < 2000 bound.

## Sequencing

Nine commits, one per destination; each removes its ranges from the source and
adds that one file. The dry run used the order listed above (config, git,
exec; implement_todos, route_resolve_implement, tickets_mutate_stdio,
todo_agenda_tools, workflow_manual_state; playbook_golden) and every
intermediate state passed `go vet`. The order is free: destinations are disjoint
and helpers left in the original file are visible package-wide. Move by
declaration name and use the snapshot ranges as the guide; after an earlier
commit, later ranges shift in the working file. Recompute each file's import
block (the lists above are exact for this order and final state) and run
`gofmt -w` on the two touched files. The last `server_test.go` move otherwise
leaves one trailing blank line at EOF.

## Hazards (read from code at `616271b51`)

1. **`TestMain` lives in `server_test.go:27-65`** (`TestMain` + `runTestMain`).
   It sets `WS_RSRC_ROOT`, `WS_CACHE_HOME` and `WS_CONFIG_HOME` for the whole
   package. It stays put. Nowhere else in the package defines `TestMain` or a
   test `init()`, and moving tests between files does not change its effect.
2. **The helper-process test is found by name, not file.**
   `server_test.go:3073` spawns `os.Args[0] -test.run=TestMCPExecHelperProcess`
   with `GO_WANT_MCP_EXEC_HELPER=1`, and `TestMCPExecHelperProcess` (3440-3459)
   answers that env var. Both move together into `server_stdio_exec_test.go`.
   Location does not matter, but the name must not change (no renames anyway).
3. **Shared helpers stay in their original files.** Cross-file users counted by
   `grep -lw` over `internal/mcp/*_test.go`. `server_test.go`: `mustWrite`,
   `initGit`, `useLeadProfile`, `toolText`, `toolIsError`, `runGit`,
   `runGitOutput`, `callToolsList`, `responseLinesByID`,
   `serveStdioWithSession`, `toolPropertiesByName`, `toolEntryTextByName`,
   `toolNameListed`, `canonicalTestPath`, `headBranch`, `initTicketRepo`.
   `session_state_test.go`: `callToolWithKey`, `keysOf`, `eqKeys`,
   `implementReadyArgs`, `implementSkipDocsArgs`, `implementReadyFacts`,
   `writeImplementReadyTicket`, `callToolLineWithKey`, `containsString`,
   `enableReviewPhase`. `playbook_tools_test.go`: `buildTestRsrcTree`,
   `newTestServerWithHarness`, `isolatedPlaybookConfigOptions`,
   `shippedImplementerContext`, `initGitRepo`, `writeTestFile`,
   `delegatePlaybookContent`, `modelAliasPlaybookContent`,
   `mustOverrideLookup`. Only file-local helpers whose every user moves are
   moved (listed per destination). One exception is deliberate: `todoByKey`
   and `requireInstruction` go to `implement_todos_test.go` but are also used
   at `session_state_test.go:1686-1687`, a line that ends up in
   `route_resolve_implement_test.go`.
4. **Stale file-name comments after the move.** These are in other files and
   stay unedited under move-only:
   `review_track_alarm_test.go:16` says `TestWorkflowManualFreshModeWithRoot`
   is in `(session_state_test.go)`; it moves to `workflow_manual_state_test.go`.
   `internal/execjob/execjob_test.go:142` points to the poll-until-terminal
   pattern "in internal/mcp/server_test.go"; it moves to
   `server_stdio_exec_test.go`. Two comments stay valid because their helpers
   stay: `session_auth_test.go:83` (`runGitOutput` in `server_test.go`) and
   `playbook_render_surface_test.go:132` (`delegatePlaybookContent` in
   `playbook_tools_test.go`).
5. **Repo-relative fixture paths are relative to the package directory,** not
   to the file: `filepath.Join("..", "..", "..", "agents-plugin", "rsrc")`
   (e.g. `playbook_tools_test.go:2109`, `:2139`, and about 30 other sites),
   `agents-plugin-wsflow/rsrc` (`:1874`), `ai-docs/manuals/skill-authoring.md`
   (`:1810`), and `shippedRsrcRootForTest` (`playbook_render_surface_test.go:446`).
   They are unaffected because every destination is in the same directory. The
   narrow test gate needs the real checkout (a bare `agents-plugin-tool`
   export fails `TestPlaybookPrintLeadRunWorkerTierPolicy` and
   `TestSkillAuthoringRelocatedOutOfRsrc`). There is no `internal/mcp/testdata/`,
   and none of the targets uses golden files on disk.
6. **Package-level test vars/consts and init order.** The only package-level
   var is `spawnIdiomSites` (`playbook_tools_test.go:2094-2095`, literal,
   no dependencies). The consts are `unresolvedPhaseTicketBody` (2411),
   `agendaNotePrefix` (3157), `bindingAnchorTest*` (3994-3999), and the
   playbook fixture group (118-188) plus `workerPlaybookContent` (1926). None
   has an initialization dependency, so file order cannot change init order.
7. **No `t.Parallel`, no build tags, no `init()`** in any target (grep for
   `t.Parallel`, `//go:build`, `// +build`, `func init(`).
8. **No test scans the package's own source or file list.** Repo-wide grep for
   the three file names, `"server.go"`, `runtime.Caller`, `go/parser` and
   `*.go`/`*_test.go` globs found only comments (hazard 4) and unrelated
   packages (`wsstore/store_test.go:382` and `wsagent/permission_store_test.go:221`
   parse their own files with `go/parser`; neither reads `internal/mcp`).
   `TestSpawnIdiomSitesListIsComplete` (`playbook_tools_test.go:2134`) walks
   `agents-plugin/rsrc`, not Go sources.
9. **Pre-existing gofmt drift:** `gofmt -l internal/mcp` already lists
   `impl_identity_test.go` at the snapshot. Run the gofmt gate on touched files
   only, and leave that file alone.
10. **Size guard (`git_merge_size.go:226-232`).** A newly added file counts from
    `before = 0`, so any new file over 1000 lines would raise a
    `size_soft_limit` advisory on merge. Every destination here is at most 953
    lines, which is why `session_state_test.go`'s implement tests are split in
    two (one merged file would be 1183 lines). The retained originals shrink
    from above 1000, so they raise neither rule. No `.gitattributes`
    `ws-size-guard` change.
11. **Docs citing these files (note only, not updated).** Active:
    `ai-docs/tickets/ready/261115-feat-ws-session-hire-approval.md:339` lists
    `playbook_tools_test.go` as an extension point, at file level, and the file
    remains. This ticket's inventory table lists `playbook_tools_test.go` at
    2238 lines; the snapshot has 2234. Historical `.done/`/`.dropped/` tickets
    cite old `file:line` values (e.g. `260630-bug-enter-proceed-status-report-dead-code.md:38`,
    `261028-chore-core-review-followup-backlog.md:53`,
    `260924-chore-review-sweep-test-and-naming-minors.md:183-184`). Those
    citations were already stale and are not updated.

## Verification gates (macOS, local)

From `agents-plugin-tool/` with `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`.
Never run `./internal/mcp` without `-run`. At the snapshot,
`go vet ./internal/mcp` and `go test -count=1 -run '^$' ./internal/mcp` pass.

Per commit:

1. `gofmt -l internal/mcp/<source>_test.go internal/mcp/<dest>_test.go` prints
   nothing.
2. `go build ./...`
3. `go vet ./internal/mcp`
4. `go test -count=1 -run '^$' ./internal/mcp` (compiles every test file).
5. Narrow sample for the destination (all passed in the dry run):
   - config: `-run '^(TestServeStdioConfigShow|TestServeStdioConfigResolveAgentFallsBackToDefault|TestDetectHarnessFromInitializeParamsPrecedence)$'`
   - git: `-run '^(TestActiveImplTicketAndGitStatusFormatting|TestServeStdioGitToolCalls|TestFormatTicketVerifyRendersAdvisoriesWithoutAmendRecipe)$'`
   - exec: `-run '^(TestExecToolsListNoAgentAndMCPFlow|TestServeStdioDoesNotBlockToolsListBehindLongCall)$'`
   - implement_todos: `-run '^(TestDeriveImplementTodos|TestDeriveImplementTodoInstructionsLiteReview)$'`
   - route_resolve_implement: `-run '^(TestRouteResolveImplementSchemaIsOpaque|TestEnterImplementNearMissesPreserveStandardBranchAndMergeTodos|TestEnterImplementTypedPathRendersDeclaredBindingAnchor)$'`
   - tickets_mutate_stdio: `-run '^TestServeStdioTicketsCreateEmptyStatesSkeletonCaveat$'`
   - todo_agenda_tools: `-run '^(TestServeStdioAgendaSetMerge|TestRenderSessionStateAgendaSizeNoteThreshold)$'`
   - workflow_manual_state: `-run '^(TestStripModeGatedRegion_NoMarker|TestWorkflowManualFreshModeWithRoot|TestWorkflowStateToolSchema)$'`
   - playbook_golden: `-run '^(TestPlaybookPrintGoldenLeadTicket|TestSpawnIdiomSitesListIsComplete|TestClaudeSpawnIdiomRendersAtEverySite)$'`
   - retained originals, run once at the end:
     `-run '^(TestServeStdioToolsListAndCall|TestStoreAgendaRoundTrip|TestServeStdioSessionStateFlow|TestPlaybookPrintClaudeHarness)$'`
6. `git show --stat HEAD` touches exactly two files. Review with
   `git show --color-moved=dimmed-zebra --color-moved-ws=no`: only the import
   blocks may show as plain additions or deletions.

End of series (optional, about 45 s in the dry run): run every `Test*` that was
in the three snapshot files as one `-run` regex built from the snapshot, for
example
`git show 616271b51:agents-plugin-tool/internal/mcp/server_test.go | grep -oE '^func Test[A-Za-z0-9_]+' | grep -v TestMain`
repeated for all three files, joined with `|` and wrapped in `^(...)$`. That
covers about 250 tests, every one of which passed after the dry-run split.

### Declaration-identity proof

The proof is a multiset of `(kind, name, sha256(exact source bytes from doc
comment start to declaration end))` for every top-level non-import declaration,
plus `sha256` of every comment group that lies outside all declarations
(section banners). It is computed over the three snapshot files before and over
the three originals plus all destinations after; the sorted lists must be
identical. In the dry run both sides had 334 entries, 24 of them banners, and
they were identical. The proof also confirms that every after-file declares
`package mcp`. Keep the tool outside the repo (e.g. `/tmp/ids/main.go` with its
own `go.mod`, stdlib only):

```go
package main

import (
	"crypto/sha256"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
)

func main() {
	for _, path := range os.Args[1:] {
		src, err := os.ReadFile(path)
		if err != nil {
			panic(err)
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, src, parser.ParseComments)
		if err != nil {
			panic(err)
		}
		fmt.Printf("package %s\n", f.Name.Name)
		type span struct{ a, b token.Pos }
		var spans []span
		for _, d := range f.Decls {
			start := d.Pos()
			var name, kind string
			switch x := d.(type) {
			case *ast.FuncDecl:
				if x.Doc != nil {
					start = x.Doc.Pos()
				}
				kind, name = "func", x.Name.Name
				if x.Recv != nil {
					kind = "method"
					t := x.Recv.List[0].Type
					if s, ok := t.(*ast.StarExpr); ok {
						t = s.X
					}
					if id, ok := t.(*ast.Ident); ok {
						name = id.Name + "." + name
					}
				}
			case *ast.GenDecl:
				if x.Tok == token.IMPORT {
					spans = append(spans, span{x.Pos(), x.End()})
					continue
				}
				if x.Doc != nil {
					start = x.Doc.Pos()
				}
				kind = x.Tok.String()
				for _, s := range x.Specs {
					switch sp := s.(type) {
					case *ast.TypeSpec:
						name += sp.Name.Name + ","
					case *ast.ValueSpec:
						for _, n := range sp.Names {
							name += n.Name + ","
						}
					}
				}
			}
			spans = append(spans, span{start, d.End()})
			text := src[fset.Position(start).Offset:fset.Position(d.End()).Offset]
			fmt.Printf("%s %s %x\n", kind, name, sha256.Sum256(text))
		}
	outer:
		for _, cg := range f.Comments {
			for _, s := range spans {
				if cg.Pos() >= s.a && cg.End() <= s.b {
					continue outer
				}
			}
			text := src[fset.Position(cg.Pos()).Offset:fset.Position(cg.End()).Offset]
			fmt.Printf("comment - %x\n", sha256.Sum256(text))
		}
	}
}
```

Usage: extract the snapshot files with `git show
616271b51:agents-plugin-tool/internal/mcp/<f>`. Then run `ids <3 snapshot
files> | grep -v '^package' | sort > before`, run `ids <3 originals + dests> |
sort | uniq -c | grep package` (it must show only `package mcp`), and finally
`ids ... | grep -v '^package' | sort > after; diff before after`. For a
per-commit check, compare the source file at `HEAD~1` against the source plus
destination at `HEAD`. Also confirm that no other package file changed:
`git diff --stat 616271b51..HEAD -- agents-plugin-tool/internal/mcp` lists
only the three originals and the nine new files.

## Parallelism with Phase 1

This group can run in parallel with Phase 1. The file sets are disjoint
(Phase 1: `server.go` plus new non-test `server_*.go`; this group: three test
files plus nine new test files), no identifier is added or renamed, and no
destination name collides with a Phase 1 file. Ordering is free. Whichever
group lands second must re-run gates 2-4 and the end-of-series test run on the
combined tree before merging, because package-level compile errors only show
up when both are present. If both groups share one branch, their commits can
interleave freely.

## Needs a lead/owner decision

1. **Gate location.** This ticket's Decisions require gates on w5090 only, with
   no local `go`/`go test`, while this map's brief asked for local macOS gates.
   Confirm which applies to this group. The dry run here was local and
   read-only, in /tmp.
2. **Stale comments (hazard 4).** Either leave them, which is strict
   move-only, or fix them in a separate comment-only commit after the series.
   That commit would touch `review_track_alarm_test.go` and
   `internal/execjob/execjob_test.go`, which are outside the moved set.
3. **Ticket scope.** The Backlog boundary in Decisions says only `server.go`
   has a concrete map. Executing this map needs a phase (or a ticket edit) that
   adopts it. Refresh the inventory row for `playbook_tools_test.go`
   (2238 -> 2234).
4. **File count against size.** The primary plan keeps every new file at or
   below 1000 lines (hazard 10). Merging `implement_todos_test.go` into
   `route_resolve_implement_test.go` (1183 lines) saves one commit but adds a
   soft advisory. Lead's call.
