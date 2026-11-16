---
title: "Split oversized source files flagged by the git.merge size guard"
---

# Split oversized source files flagged by the git.merge size guard

## Background

Owner answer (560), 2026-11-13, relayed by MAIN (translated): record that
these files' warnings are temporarily ignored rather than raising the baseline
and risking forgetting to restore it; keep a refactor backlog ticket and
populate the detailed plan at a suitable time.

The original advisories arrived on 2026-11-13/14. The owner requested a
docs-only preparation pass on 2026-11-16: refresh the inventory and specify a
purely mechanical `server.go` split. This ticket remains accepted backlog in
`todo/`; this preparation is not implementation or ready promotion. Warnings
remain temporarily accepted, without raising any size limits.

## Current Inventory

Snapshot: `d90d1bb61`, 2026-11-16. Counts are tracked working-tree source lines
(including comments and blanks, counting a final unterminated line), not SLOC.
Survey: `git ls-files -z` under all four `agents-plugin*/` packages, select the
extensions in `defaultMergeSizeExtensions`, count lines, then inspect
`git check-attr -z --stdin linguist-generated linguist-vendored ws-size-guard`.

The guard is repository-wide, not limited to Go or `agents-plugin-tool/`:
`agents-plugin-tool/internal/mcp/git_merge_size.go` defines a default hard
advisory at **>= 2000** lines and soft crossing at > 1000, skips generated or
vendored attributes and explicit `-ws-size-guard` paths, and checks files edited
by the merging branch. This is the full >= 2000 inventory of guard-covered
source paths in the plugin packages, whether or not a recent merge edited them:

| Source path | Lines |
|---|---:|
| `agents-plugin-tool/internal/mcp/server.go` | 6332 |
| `agents-plugin-tool/internal/mcp/playbook_tools_test.go` | 2238 |
| `agents-plugin-tool/internal/mcp/server_test.go` | 3459 |
| `agents-plugin-tool/internal/mcp/session_state_test.go` | 4084 |
| `agents-plugin-tool/internal/wsagent/lead_runner_test.go` | 2163 |
| `agents-plugin-tool/internal/wsagent/runner.go` | 3472 |
| `agents-plugin-tool/internal/wsdashboard/assets/tracker.js` | 7217 |
| `agents-plugin-tool/internal/wswhiteboard/assets/mermaid/mermaid.min.js` | 3636 |
| `agents-plugin-pi/src/ask.ts` | 2582 |
| `agents-plugin-pi/src/spawner.ts` | 4917 |
| `agents-plugin-pi/test/ask.test.ts` | 2849 |
| `agents-plugin-pi/test/goal-loop.test.ts` | 3520 |
| `agents-plugin-pi/test/spawner.test.ts` | 3591 |
| `agents-plugin/hooks/claude-mod/watcher.test.ts` | 2881 |
| `agents-plugin-wsflow/hooks/claude-mod/watcher.test.ts` | 2881 |

The following >= 2000 source-extension paths are present but already excluded
by `.gitattributes`; they are not split targets:

| Excluded path | Lines | Existing exemption |
|---|---:|---|
| `agents-plugin-pi/dist/index.js` | 39725 | `linguist-generated=true` |
| `agents-plugin/native-pi/first-party/dist/index.js` | 39725 | `linguist-generated=true` |
| `agents-plugin-wsflow/native-pi/first-party/dist/index.js` | 39725 | `linguist-generated=true` |
| `agents-plugin/bin/ws-mcp-launcher.py` | 2081 | `-ws-size-guard` |
| `agents-plugin-pi/bin/ws-mcp-launcher.py` | 2081 | `-ws-size-guard` |
| `agents-plugin-wsflow/bin/ws-mcp-launcher.py` | 2081 | `-ws-size-guard` |

`mermaid.min.js` is bundled third-party code but currently has no generated or
vendored exemption, so it is included in the covered table, not silently
omitted. Do not hand-split the upstream bundle in Phase 1; its packaging or
attribute policy needs a separate decision. Preserve the ws/wsflow watcher
mirror when planning their later split.

Previously listed paths outside the requested plugin-package survey remain
tracked here: `ai-docs/ref/tracker-mockup/engine.js` is 2442 lines and
`scripts/board-headless.mjs` is now 2475 lines. The former oversized
`agents-plugin-tool/internal/wsdashboard/assets/dashboard.js` is now 536 lines
and no longer belongs in the >= 2000 list.

## Decisions

- **Compatibility T0 (internal only):** relocate existing declarations within
  `package mcp`. No behavior, API, tool/schema, protocol, serialization,
  identifier, or on-disk format changes; old and new binaries read and write
  exactly the same surfaces. File placement is the only intended difference.
- **Same package, move only:** no new packages, wrappers, extracted functions,
  rewritten switch arms, deduplication, logic cleanup, or test redesign. No
  renames of functions, types, variables, constants, fields, or parameters.
  Move declaration comments with their declarations. Only package/import
  scaffolding and `gofmt` adjustments accompany the moves; preserve package
  initialization dependencies and ordering.
- **One move-only commit per target file:** each commit removes its listed
  declarations from `server.go` and adds that one destination, with the minimum
  necessary import adjustments. Do not mix moves with fixes or other files'
  refactors; this keeps blame, review, cherry-pick, and rebase straightforward.
- **Gate on w5090 only:** for every move commit, run `gofmt` on the affected Go
  files, then from `agents-plugin-tool/` run `go build ./...`, `go vet ./...`,
  and the existing package tests (`go test ./internal/mcp`). Require all to
  pass and record remote output; compare against a remote pre-split baseline.
  These are future implementation gates, not claimed results of this docs pass.
  No `go`/`go test`, ws/ws-mcp executable, launcher, or test binary runs locally
  on this PC.
- **Timing:** execute only at a quiet point with **no open branch touching
  `server.go`**. Coordinate with the lead, land or park parallel edits first,
  refresh this map against the execution base, and keep the move series short:
  the split conflicts with every parallel change to this file. Do not start
  code moves during this docs-only preparation pass.
- **Backlog boundary:** keep the ticket in `todo/`. Only `server.go` has a
  concrete split map below; the other inventory entries remain later planning
  work, not permission to refactor them in Phase 1. No guard baseline changes.

## Phases

### Phase 1: Mechanically split the MCP server declarations

Intended behavior: unchanged. Move the following complete top-level
declarations from `agents-plugin-tool/internal/mcp/server.go` into new sibling
files in `agents-plugin-tool/internal/mcp/`, all `package mcp`. Names below are
exact; `Server.<name>` denotes a method on `*Server`, while a bare name denotes
a package declaration. Approximate sizes include declaration comments/blank
lines, with small variations for per-file imports and `gofmt`.

**Important move-only boundary:** `Server.callTool` is a single ~1645-line
function and `tools` is a single ~1147-line function. Their ticket/worktree/
whiteboard cases and schema entries are not top-level declarations. Move each
function intact; extracting those cases into `server_worktree.go` or
`server_whiteboard.go` would require new identifiers and dispatcher changes,
which this ticket explicitly forbids. The existing `worktree_tools.go`,
`whiteboard_tools.go`, and other family files stay untouched.

#### `server_dispatch.go` — approximately 1650 lines

- Function: `Server.callTool` (entire dispatch, including all prechecks,
  deferred piggyback/refresh/nudge actions, and all tool-family cases).
- Types/vars/constants: none.

#### `server_schemas.go` — approximately 1400 lines

- Functions: `tools`, `withSessionKeyToolSchemas`,
  `toolSchemaRequiresSessionKey`, `appendRequiredString`, `execLaunchSchema`,
  `execKeySchema`, `execResultSchema`, `execRawTailSchema`, `execRawReadSchema`,
  `execRawGrepSchema`, `stringProperty`, `nullableStringProperty`,
  `stringArrayProperty`, `enumStringProperty`, `enumStringOrArrayProperty`,
  `nullableEnumStringProperty`, `scopeSchemaProperty`, `numberProperty`,
  `integerProperty`, `boolProperty`, `stringOrObjectProperty`, `objectProperty`,
  `objectArrayProperty`.
- Types/vars/constants: none. Keep `tools`' literal and append order unchanged.

#### `server_inventory.go` — approximately 240 lines

- Functions: `isLeadOnlyTool`, `workflowPreferenceWriterTool`, `LeadToolNames`,
  `LeadToolNamesFromEnv`, `leadToolNames`, `Server.filteredTools`,
  `publicToolDefinition`, `Server.toolAllowed`, `isNoteMutationTool`,
  `noteReadOnlyDenial`, `roleAllowsTool`, `advertisedToolName`, `namespaceText`,
  `namespaceTerms`, `namespaceValue`, `permanentlyHiddenTool`,
  `noAgentHiddenTool`, `parseAllowedTools`.
- Variable: `wsNamespaceRef` (namespace rewriting's compiled regexp).
- Types/constants: none.

#### `server_config.go` — approximately 830 lines

- Types: `configListView`, `resolveAgentTierResult`, `promptOverrideValue`,
  `promptOverridePoint`, `tuningCatalog`, `tuningKnob`, `tuningWriter`,
  `tuningField`, `tuningScopedValue`, `tuningAgentTierCurrent`,
  `agentsTierTuneResult`.
- Functions: `builtinConfigDefaults`, `builtinConfigAndPromptDefaults`,
  `Server.sessionConfigOptions`, `Server.sessionResolver`,
  `Server.resolveBucketedOverrideRows`, `formatConfigView`,
  `buildPromptOverrideListing`, `formatPromptOverrideListing`,
  `buildTuningCatalog`, `tuneAgentsTier`, `Server.resolveReviewPhase`,
  `Server.resolveSageReviewConfig`, `currentWorkflowPreference`,
  `currentHarnessBucketedValue`, `currentAgentTierMappings`,
  `formatTuningCatalog`, `formatManifestErrors`, `Server.effectiveConfigValue`,
  `resetEchoScope`, `tuningFieldLabels`, `formatTuningCurrent`,
  `sortedAgentModelAliasKeys`, `sortedAgentTierKeys`, `tuneStringValueRejection`.
- Vars/constants: none.

#### `server_tickets.go` — approximately 570 lines

- Functions: `Server.assigneeFeature`, `applyAssigneeGate`, `resolveSetAssignee`,
  `formatTicketCreate`, `formatTicketMutate`, `ticketMutateNextInstruction`,
  `verifyAdapter`, `Server.verifyAdapter`, `verifyAdapterEnv`,
  `formatTicketVerify`, `formatSageGate`, `sageGateNextInstruction`,
  `sageStageForReviewers`, `formatSageRecord`, `sageRecordIssueRouting`,
  `sageBlockedReviewers`, `parseSageVerdicts`, `ticketScopeAnnotation`,
  `effectiveTicketStatuses`, `formatTickets`, `ticketDiscoveryPagination`,
  `hasTicketOnlyArgument`.
- Constants: `sageGatePostureUncommittedNote`, `sageRestampAfterFixes`,
  `sageNoSameVerdictRestamp`.
- Types/vars: none.

#### `server_git.go` — approximately 270 lines

- Types: `gitStatusResult`, `implTicketStatus`, `reviewMarkerJSON`.
- Functions: `formatGitStatus`, `formatGitStatusWithImplTicket`,
  `activeImplTicket`, `verifyActiveTicketInventory`, `formatGitLog`,
  `formatMergeBase`, `formatGitCommit`, `reviewRefSuffix`, `reviewMarkerJSONValue`.
- Vars/constants: none. Review-ledger projections stay with the Git adapters.

#### `server_sessions.go` — approximately 340 lines

- Type: `sessionChildOutput`.
- Functions: `canonicalSetupRoot`, `Server.handleLeadLogin`,
  `Server.handleLeadRelogin`, `Server.leadKeyRefusal`, `hostedKeyRefusal`,
  `Server.leadLoginResponse`, `Server.handleSessionChildren`,
  `sessionChildScopeLabel`, `formatSessionChildren`, `parseCapabilityScope`,
  `Server.resolveToolRoot`, `canonicalGitRoot`, `Server.requireLeadSessionKey`.
- Vars/constants: none.

#### `server_harness.go` — approximately 110 lines

- Functions: `codexWorkspaceRoots`, `Server.observeHarness`,
  `Server.currentHarness`, `detectHarnessFromInitializeParams`,
  `detectHarnessFromRaw`, `detectHarnessFromMeta`, `normalizedHarness`.
- Types/vars/constants: none.

#### `server_exec.go` — approximately 120 lines

- Functions: `execResponse`, `formatExecResponse`, `execRawTailResponse`,
  `execRawReadResponse`, `execRawGrepResponse`.
- Types/vars/constants: none. Schema helpers move to `server_schemas.go` above.

#### `server_diagnostics.go` — approximately 140 lines

- Functions: `appendDebugEvent`, `crashLogPath`, `crashDir`, `recordPanic`,
  `Server.recordPanic`, `recordPanicToCrashLog`, `RecordLifecycleEvent`,
  `recentDebugEvents`.
- Types/vars/constants: none. Keep crash sink and debug-ring behavior unchanged.

#### `server_playbook.go` — approximately 35 lines

- Function: `appendRenderContext`.
- Variable: `wsflowRenderEligibleStems`.
- Types/constants: none. This is only the existing legacy render-context bridge.

#### `server_util.go` — approximately 340 lines

- Functions: `rawMessageString`, `wantsJSON`, `formatStringLines`, `yesNo`,
  `capitalizeFirst`, `writeIndentedLines`, `displayString`, `displayOrDash`,
  `boolValue`, `shortHash`, `isHexString`, `toolJSONResponse`, `toolTextResponse`,
  `toolErrorTextResponse`, `errorResponse`, `stringMapArgument`,
  `int64FromArgument`, `optString`, `typedArrayRejection`, `jsonValueTypeName`,
  `stringList`, `stringListKeepBlank`, `boolArgument`, `exactIntegerArgument`,
  `hasSpecStemArgument`, `durationFromSeconds`, `intFromArgument`.
- Types/vars/constants: none. Shared argument/response helpers are moved once,
  not copied into tool-family files.

#### Retained `server.go` — approximately 420 lines

- Types: `Server`, `toolRole`, `request`, `cancelledNotificationParams`,
  `response`, `rpcError`.
- Constants: `roleLead`, `roleDelegate`, `roleLeaf`, `bootstrapToolName`,
  `ProtocolVersion`, `maxDebugEvents`, `leadWorkflowBootstrapMethod`,
  `envNoAgent`, `envNamespace`, `envNoDashboard`, `envHostedSession`,
  `envHostedSessionKey`, `envHostedSessionRow`, `envHarnessPID`, `envDebugLog`,
  `envAllowedTools` (retain the existing grouped declarations).
- Variable: `testPanicHook`.
- Functions: `NewServer`, `Server.ServeStdio`, `Server.handleNotification`,
  `Server.handle`, `NoDashboardMode`, `NoAgentMode`, `RuntimeNamespace`,
  `runtimeInfo`, `formatRuntimeInfo`.

Verification boundary: first confirm the declaration inventory still matches
this snapshot, then execute the per-target move-only commit and remote gate
from Decisions. Review using moved-line-aware diffs; require each existing
non-import declaration to occur exactly once in the same package with the same
body, comments, and name. All destinations (and the retained core) should remain
below 2000 lines. A gate failure requiring a behavior fix is not permission to
mix that fix into a move commit: stop and report it separately.

Deferred scope: no splitting of the intact dispatch/schema functions, no
changes to existing family implementations or tests, and no implementation
plan for other oversized files until their own quiet-point planning pass.
