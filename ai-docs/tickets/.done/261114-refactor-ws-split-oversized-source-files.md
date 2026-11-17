---
title: "Split oversized source files flagged by the git.merge size guard"
completed: 2026-11-17
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

Owner request, 2026-11-17, relayed by the lead (translated): "Count all the
line counts and do the whole split this time, not just server.go. It's a big
job, so use several agents. I expect it to be mechanical splitting, but there
will be a few things to watch, such as cross-references, so plan it well." This
authorizes every covered group below, not only Phase 1. Phases 2-8 were
planned against `616271b51`; each map was dry-run in a `/tmp` copy by its
planning agent (results in the linked maps).

## Current Inventory

Snapshot: `616271b51`, 2026-11-17 (previous: `d90d1bb61`, 2026-11-16). Counts
are tracked working-tree source lines (including comments and blanks, counting a
final unterminated line), not SLOC. Survey: `git ls-files -z` under all four
`agents-plugin*/` packages and `scripts/`, select the extensions in `defaultMergeSizeExtensions`, count lines, then inspect
`git check-attr -z --stdin linguist-generated linguist-vendored ws-size-guard`.

The guard is repository-wide, not limited to Go or `agents-plugin-tool/`:
`agents-plugin-tool/internal/mcp/git_merge_size.go` defines a default hard
advisory at **>= 2000** lines and soft crossing at > 1000, skips generated or
vendored attributes and explicit `-ws-size-guard` paths, and checks files edited
by the merging branch. This is the full >= 2000 inventory of guard-covered
source paths in the plugin packages and `scripts/`, whether or not a recent merge
edited them:

| Source path | Lines | Phase |
|---|---:|---|
| `agents-plugin-tool/internal/mcp/server.go` | 6374 | 1 |
| `agents-plugin-tool/internal/mcp/session_state_test.go` | 4084 | 2 |
| `agents-plugin-tool/internal/mcp/server_test.go` | 3459 | 2 |
| `agents-plugin-tool/internal/mcp/playbook_tools_test.go` | 2234 | 2 |
| `agents-plugin-tool/internal/wsagent/runner.go` | 3555 | 3 |
| `agents-plugin-tool/internal/wsagent/lead_runner_test.go` | 2206 | 3 |
| `agents-plugin-tool/internal/wsdashboard/assets/tracker.js` | 7217 | 4 |
| `agents-plugin-pi/src/spawner.ts` | 4618 | 5 |
| `agents-plugin-pi/src/ask.ts` | 2582 | 5 |
| `agents-plugin-pi/test/goal-loop.test.ts` | 3520 | 6 |
| `agents-plugin-pi/test/spawner.test.ts` | 3476 | 6 |
| `agents-plugin-pi/test/ask.test.ts` | 2847 | 6 |
| `agents-plugin/hooks/claude-mod/watcher.test.ts` | 2881 | 7 |
| `agents-plugin-wsflow/hooks/claude-mod/watcher.test.ts` | 2881 | 7 (mirror) |
| `scripts/board-headless.mjs` | 2475 | 8 |
| `agents-plugin-tool/internal/wswhiteboard/assets/mermaid/mermaid.min.js` | 3636 | none (third-party; Open Decisions) |

The following >= 2000 source-extension paths are present but already excluded
by `.gitattributes`; they are not split targets:

| Excluded path | Lines | Existing exemption |
|---|---:|---|
| `agents-plugin-pi/dist/index.js` | 38210 | `linguist-generated=true` |
| `agents-plugin/native-pi/first-party/dist/index.js` | 38210 | `linguist-generated=true` |
| `agents-plugin-wsflow/native-pi/first-party/dist/index.js` | 38210 | `linguist-generated=true` |
| `agents-plugin/bin/ws-mcp-launcher.py` | 2258 | `-ws-size-guard` |
| `agents-plugin-pi/bin/ws-mcp-launcher.py` | 2258 | `-ws-size-guard` |
| `agents-plugin-wsflow/bin/ws-mcp-launcher.py` | 2258 | `-ws-size-guard` |

`mermaid.min.js` is bundled third-party code with no generated or vendored
exemption, so it stays listed rather than silently omitted; it is not hand-split
(see Open Decisions). Outside the surveyed trees,
`ai-docs/ref/tracker-mockup/engine.js` (2442 lines) is reference material, not a
plan target. `dashboard.js` (536 lines) is no longer oversized.

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
- **Gates run locally on macOS, then CI (lead, 2026-11-17):** for every move
  commit, format the affected files and run the phase's build/vet/compile gates
  and narrow tests locally with `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`
  (never the whole `./internal/mcp` or `./internal/wsdashboard` package; the
  phase lists its narrow runs); the lead runs the full CI on the merged tree.
  Supersedes the earlier remote-only (w5090) gate, written when this machine had
  no Go toolchain.
- **Timing:** execute only at a quiet point with **no open branch touching
  `server.go`**. Coordinate with the lead, land or park parallel edits first,
  refresh this map against the execution base, and keep the move series short:
  the split conflicts with every parallel change to this file. Do not start
  code moves during this docs-only preparation pass.
- **Scope (owner, 2026-11-17):** every covered inventory group is authorized
  (Background). Phase 1 still covers only `server.go`; Phases 2-8 carry the
  other groups, each with its own map. No guard baseline or limit changes.
- **Phases 2-8 adapt the rules above per language:** same Go package, same TS
  package, or same assembled asset; no renames, no extracted helpers or
  wrappers, no logic or test redesign; comments move with their declarations;
  one move-only commit per destination file. Only import/export scaffolding
  accompanies a move (for TS: `import` lines, an `export` keyword on a moved
  private declaration, and re-export lines in the original module; see each
  phase for any further exception it asks for). Compatibility stays T0: no tool,
  schema, protocol, store, or on-disk change. Regenerated committed outputs
  (Pi bundle, native first-party payload, workflow bundles) change bytes but not
  behavior.
- **Phases 2-8 gate locally:** per the lead's 2026-11-17 brief, their gates run on
  the macOS dev machine (Go with `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`
  from `agents-plugin-tool/`; never the full `./internal/mcp` or
  `./internal/wsdashboard` package without `-run`). Full CI runs after merge.
  The same local-then-CI gate applies to Phase 1 (Decisions).
- **Shared phase rules:** first re-confirm the target is byte-identical to the
  `616271b51` snapshot (or refresh the map); cut bottom-up so remaining snapshot
  ranges stay valid; run `gofmt -l` only on touched files
  (`impl_identity_test.go`, `snapshot_idle_test.go`, `wake_test.go` are already
  not gofmt-clean); prove declaration identity per phase (multiset of top-level
  declaration names plus byte-identical declaration text including doc comments,
  before vs after, using the script described in the phase's map); stale
  `file:line` citations in docs, tickets, and comments are left as-is and listed
  in the maps. New files start at 0 lines for the merge guard
  (`git_merge_size.go:177-181,212-228`), so destinations are kept at or below 1000 lines
  where practical to avoid soft advisories.


- **Lead decisions for Phases 2-8 (2026-11-17, under the owner's request to plan
  and run the full split autonomously):**
  - Phase 4 may open with one setup commit (fragment registration and the switch
    of raw `tracker.js` test readers to the assembled reader) that moves no code,
    following the earlier dashboard split. Rejected: forcing it into the first
    move commit, which mixes reader changes with a move.
  - Phase 4 keeps `ai-docs/ref/tracker-mockup/index.html` working with a small
    inline loader that fetches the fragments in assembly order, joins them, and
    executes the result as one script; the mockup notes that it must be served
    over http. Rejected: one `<script src>` per fragment (the single IIFE spans
    every fragment, so separate scripts fail to parse), a generated assembled
    copy (new generated artifact and staleness test), and retiring the mockup
    (deletion).
  - Phase 5 keeps re-exports in `ask.ts`/`spawner.ts`, adds `export` to moved
    private declarations, allows import-only edits to earlier new files, and
    commits the regenerated bundle and native payload in every src commit.
    Rejected: rewriting importers, which is not move-only.
  - Phase 6 moves shared helpers once into `test/fixtures/*.ts` and duplicates
    the hooks and the goal-loop wrapper/preamble verbatim into each resulting
    file. Rejected: duplicating every helper (more copies), and a size-guard
    exemption for `goal-loop.test.ts` (needs owner approval and leaves the file
    oversized).
  - Phase 7 accepts `watcher-harness.ts` as a non-test module under
    `hooks/claude-mod/`, with workflow bundle and native payload regeneration in
    the series. Rejected: a location outside `hooks/` (loses the wsflow drift
    guard) or a packaging exclusion (not move-only).
  - Phase 8 moved modules import back from the entry (prototyped import cycle,
    one `export {...}` list and a one-line warning comment). Rejected: factory
    wrappers, which break the no-wrapper rule.
  - Stale comment citations are fixed in one comment-only commit after the
    series. Rejected: leaving pointers that no longer resolve.
  - Files above the 1000-line soft limit after a split are accepted; the guard
    baseline and limits do not change.
- Owner (1249)=a: `internal/wswhiteboard/assets/mermaid/mermaid.min.js`
  (3636 lines, third-party) is not split and is marked `linguist-vendored` in
  `.gitattributes` - a classification, not a size-limit change.

## Execution Plan

Phases touch disjoint source files and can run in parallel worktrees, one worker
each: Phase 1 (`internal/mcp/server.go`, in progress), 2 (`internal/mcp`
tests), 3 (`internal/wsagent`), 4 (`internal/wsdashboard` tracker), 5 (Pi src),
6 (Pi tests), 7 (claude-mod watcher tests, both packages), 8
(`scripts/board-headless.mjs`). Ordering constraints:

- Phases 1 and 2 share a package but not files; the second to merge re-runs the
  compile gates on the combined tree.
- Phases 5 and 7 both regenerate `native-pi/first-party-manifest.json` (and
  Phase 7 the workflow bundles); merge them serially and regenerate on the
  second merge rather than hand-resolving generated files.
- Phase 6 depends on Phase 5 only through re-exports, which keep `../src/*.ts`
  imports valid in either order; Phase 6's helper shape is settled in Decisions.
- Phase 4 should land before `261116-feat-ws-dashboard-session-page-payload-and-render-cost`
  starts on `pages.go`.
- Quiet points: Phase 3 and Phase 5 vs `impl/develop/shove-stem-disk`
  (`runner.go`, `spawner.ts`) and `exec5/hire-child-ceiling` (`spawner.ts`);
  Phase 8 vs `impl/develop/perm-unlit-icon` and
  `261115-feat-ws-dashboard-board-reorder-hold-rewrite`.
  Lead check 2026-11-17: those three branches are parked (last commits
  2026-11-14/15, unmerged, no worker on them), so the split proceeds now and a
  parked branch rebases onto the split when it resumes.

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
plan for other oversized files in this phase (Phases 2-8 carry them).

### Result (4faad059e) - 2026-11-17

Merged to develop at 4faad059e (12 move-only commits, one per declaration family). `mcp/server.go` went from 6374 to 403 lines; declarations moved into `server_dispatch.go` (1664), `server_schemas.go` (1402), `server_config.go` (839), `server_tickets.go` (569), `server_sessions.go` (336), `server_util.go` (328), `server_git.go` (264), `server_inventory.go` (257), `server_diagnostics.go` (140), `server_harness.go` (132), `server_exec.go` (120), `server_playbook.go` (35).
- Verification: 202 declarations proven byte-identical by AST comparison, gofmt clean, offline build and MCP vet passed.
- The intact dispatch and schema functions were not split (deferred scope); `server_dispatch.go` and `server_schemas.go` are over 1000 lines but within the 2000-line goal.

### Phase 2: Split the oversized MCP test files

Intended behavior: unchanged; same `package mcp` (internal tests only). Map:
[mcp-tests-map.md](../assets/261114-refactor-ws-split-oversized-source-files/mcp-tests-map.md)
(exact declarations, snapshot ranges, import lists, per-commit `-run` samples,
and the `go/ast` identity script). Nine move-only commits:

- `server_test.go` 3459 -> 1285 retained, plus `server_stdio_config_test.go`
  (~806), `server_stdio_git_test.go` (~915), `server_stdio_exec_test.go` (~493).
  Names avoid `server_{config,git,exec}_test.go`, which would shadow Phase 1's
  `server_{config,git,exec}.go`.
- `session_state_test.go` 4084 -> 757 retained, plus `implement_todos_test.go`
  (~440), `route_resolve_implement_test.go` (~749),
  `tickets_mutate_stdio_test.go` (~600), `todo_agenda_tools_test.go` (~878),
  `workflow_manual_state_test.go` (~707).
- `playbook_tools_test.go` 2234 -> 1292 retained, plus
  `playbook_golden_test.go` (~953).

Hazards: `TestMain` (`server_test.go:27-65`) is the package's only one and
stays; no `init()`, `t.Parallel`, build tags, or external test package. The exec
helper process is located by name (`server_test.go:3073` runs
`-test.run=TestMCPExecHelperProcess`, defined at `:3440`); both move together to
the exec file. Helpers used by several files (`mustWrite`, `initGit`,
`toolText`, `runGit*`, `callToolWithKey`, `keysOf`, `buildTestRsrcTree`,
`newTestServerWithHarness`, ...) stay put; a helper moves only with all its
callers. Repo-relative fixture paths (`playbook_tools_test.go:1810,1874,2109`)
resolve from the package directory and are unaffected. Stale comments left
as-is: `review_track_alarm_test.go:16`, `internal/execjob/execjob_test.go:142`.

Parallelism: disjoint from Phase 1 (no test depends on which non-test file
holds a declaration); either may land first, and the second re-runs `go build`,
`go vet`, and the compile check on the combined tree.

Verification boundary (per commit): `gofmt -l` on the two touched files;
`go build ./...`; `go vet ./internal/mcp`;
`go test -count=1 -run '^$' ./internal/mcp`; the destination's narrow `-run`
sample from the map; `git show --stat HEAD` lists exactly two files and
`--color-moved` shows only import lines as real changes. End of series: one
`-run` regex over every `Test*` from the three snapshot files (~250 tests,
~45 s), plus the identity proof (dry run: 334 entries identical).

Deferred scope: no edits to non-test files or other test files.

### Result (7f9403a06) - 2026-11-17

Merged to develop at 7f9403a06 (9 move-only commits). New test files: `playbook_golden_test.go` (953), `server_stdio_git_test.go` (915), `todo_agenda_tools_test.go` (878), `server_stdio_config_test.go` (806), `route_resolve_implement_test.go` (749), `workflow_manual_state_test.go` (707), `tickets_mutate_stdio_test.go` (600), `server_stdio_exec_test.go` (493), `implement_todos_test.go` (440). Retained: `server_test.go` 1285, `playbook_tools_test.go` 1292, `session_state_test.go` 757 (from 3459, 2234, and 4084 lines respectively).
- Verification: an AST source-slice proof preserved all 334 declaration and comment entries against the base; `TestMain` and shared helpers were kept in place, with only package and import scaffolding changed.
- After merge the lead's narrow `mcp` tests passed. Retained files are over 1000 lines but within the 2000-line goal.

### Phase 3: Split wsagent runner and lead-runner tests

Intended behavior: unchanged; same `package wsagent`. Map:
[wsagent-map.md](../assets/261114-refactor-ws-split-oversized-source-files/wsagent-map.md).
Five move-only commits, bottom-up, each cut on a blank line:

- `runner.go` 3555 -> 1287 retained (interfaces, `Options`, `Runner` and its
  request methods, request errors, `State`, `conv.startBackend`), plus
  `runner_events.go` (snapshot 2800-3555, ~770: `conv.handle` .. `conv.close`,
  approvals, exit, idle/lever/settings, `actionGrace`, `actionNeededText`,
  `actionNeededTool`, `leverHoldCap`), `runner_process.go` (2178-2798, ~636:
  `conv.runCommand` .. `conv.persistSeq`, `composeInstructions`),
  `runner_conv.go` (1291-2176, ~901: the `conv` type, loop, row/registration,
  `conv.apply`, `conv.kick`, `rowForeign`, `nextTurn`).
- `lead_runner_test.go` 2206 -> 778 retained (shared `specEnv`,
  `leadHarness.controls`, `leadHarness.steerHeld`, `shortActionGrace`), plus
  `lead_rejoin_test.go` (781-1499, ~731) and `lead_lifecycle_test.go`
  (1501-2206, ~722).

Hazards: `actionGrace` (`runner.go:3066`) moves and is reassigned by tests
(`lead_runner_test.go:40-44`); same package, safe. No `init()`; package vars
have literal initializers. Neither target has `//go:embed` or build tags (embeds
live in `builtin.go:116,119`, `pi_lead.go:13,16`). No test reads these files by
path. Stale citations left as-is: `ai-docs/manuals/ws-mcp.md:967,992`,
`pi_daemon_workflow_test.go:34`, several todo/ready tickets.

Ordering: independent of Phases 1-2 (`mcp` imports `wsagent`, but no identifier
changes). The unmerged `impl/develop/shove-stem-disk` branch edits
`runner.go` (`startConv`, `State`, `closedState`, the `conv` struct,
`conv.state`, `conv.apply`); land or park it before this phase.

Verification boundary (per commit): `gofmt -l` on touched files;
`go build ./...`; `go vet ./internal/wsagent`;
`go test -run '^$' ./internal/wsagent` (~4 s total); `go test -list '.*'
./internal/wsagent` still lists 463 tests; the map's 11-test `-run` sample
(~4 s). End of series: full `go test -count=1 ./internal/wsagent` (~89 s; check
`git status` afterwards, it reads sibling packages) and the identity proof (dry
run: 1638 declarations identical).

Deferred scope: optional fourth move (`Runner.Create` .. `Runner.act` plus
request errors into `runner_requests.go`) only if the lead wants `runner.go`
under ~1000 lines.

### Result (21e4698a0) - 2026-11-17

Merged to develop at 21e4698a0 (5 move-only commits). `wsagent/runner.go` went from 3555 to 1287 lines, with `runner_conv.go` (901), `runner_events.go` (770), and `runner_process.go` (636) split out; `lead_runner_test.go` went from 2206 to 778 lines, with `lead_rejoin_test.go` (731) and `lead_lifecycle_test.go` (722) split out.
- Verification: all 1638 declaration hashes and the package-wide comment/body multisets are identical to the pre-series base.
- The optional fourth move (`runner_requests.go`) was not taken; `runner.go` stays at 1287 lines, within the 2000-line goal.

### Phase 4: Split the dashboard tracker script into assembled fragments

Intended behavior: unchanged; the assembled `tracker.js` stays byte-identical
(SHA-256 `0bba48f6…2aecf`), so its content-hash asset ID (`pages.go:341`) does
not change. Map:
[tracker-js-map.md](../assets/261114-refactor-ws-split-oversized-source-files/tracker-js-map.md).

Mechanism (reused from `261114-refactor-ws-dashboard-whiteboard-asset-split`):
`readDashboardSource` (`pages.go:71-90`) maps a logical name to an ordered
fragment list and concatenates without separators; embedded and dev-assets
paths both go through `get()` (`pages.go:256-265`). `tracker.js` is only inlined
(`pages.go:281` into `dashboard.tmpl:37,371`); no standalone URL, CSP hash, or
PWA precache entry.

Destinations, contiguous snapshot ranges cut on existing top-level `// ----`
section headings inside the IIFE: `tracker.js` retained 1-1150 (header, IIFE
open at `:222`, `'use strict'` at `:223`, shared state, small views, item rows);
`tracker-panel.js` 1151-1777 (~627); `tracker-wiki.js` 1778-2444 (~667);
`tracker-window.js` 2445-3216 (~772); `tracker-forms.js` 3217-3952 (~736);
`tracker-menu.js` 3953-4852 (~900); `tracker-writes.js` 4853-5671 (~819);
`tracker-render.js` 5672-6616 (~945); `tracker-mount.js` 6617-7217 (~601, ends
with `})();`). A 10-file variant with every file <= 1000 is in the map.

Commits: commit 0 (registration scaffolding, no move; needs lead approval, see
Decisions) adds `trackerScriptFiles = []string{"tracker.js"}`, the
`case "tracker.js"`, the `testdata/asset-source.cjs` declaration, and the
`TestDashboardSourceAssembly` row, and switches raw test readers to the
assembled reader. Commits 1-8 peel from the tail (`tracker-mount.js` first),
each extracting its range with `sed -n` from the snapshot, inserting its name
directly after `"tracker.js"`, and updating the assembly-test row. Commit 1
also appends `assets/tracker-*.js` to the `//go:embed` line (`pages.go:34`); an
unmatched embed glob does not compile, so it cannot go in earlier.

Hazards: (1) 18 `dashboardFS.ReadFile("assets/tracker.js")` sites in 12 test
files (listed in the map) would silently see only the first fragment and turn
"must not contain" checks vacuous (`tracker_wiki_test.go:490-500`,
`search_palette_test.go:51,62`); all switch to `dashboardTestAsset` in commit 0,
and node tests `testdata/search-palette.test.cjs:7` and `tracker_draft.test.js:7`
switch to `readAssetSource`. (2) Closure/strict/hoisting: no top-level
`let`/`const` outside the IIFE, contiguous concatenation preserves order, no test
parses fragments individually. (3) `ai-docs/ref/tracker-mockup/index.html:84`
loads `tracker.js` by `<script src>` and breaks after the split (Open
Decisions). Nothing under `scripts/` reads `tracker.js`.

Ordering: `261116-feat-ws-dashboard-session-page-payload-and-render-cost` (now
`ready/`) touches `pages.go` (embed line, `buildDashboardAssets`) and the
asset-ID/asset-source tests; textual conflicts only, since no bytes change.
Recommend landing this phase first and not running both on `pages.go` at once;
whichever lands second, 261116 must hash and serve the assembled
`get("tracker.js")`, never the raw file.

Verification boundary (per commit): assemble via the cjs list and `cmp`
against `git show 616271b51:agents-plugin-tool/internal/wsdashboard/assets/tracker.js`,
plus `cat` of fragments in list order piped to `cmp`; `node --check` on the
assembled file; `gofmt -l`, `go build ./...`, `go vet ./internal/wsdashboard`,
`go test -run '^$' ./internal/wsdashboard`, and the map's narrow `-run` over
tracker/search-palette/asset-source tests (~3.5 s); `node
testdata/search-palette.test.cjs` and `node --test tracker_draft.test.js`.

Deferred scope: no asset-ID test coverage additions (`assets_id_test.go:65-67`
does not list `tracker.js` today) and no mockup fix inside move commits.

### Result (9b1466d3d) - 2026-11-17

Merged to develop at 9b1466d3d (9 commits: one setup commit registering assembled tracker source readers, then 8 fragment moves). `assets/tracker.js` went from 7217 to 1150 lines, with fragments `tracker-render.js` (945), `tracker-menu.js` (900), `tracker-writes.js` (819), `tracker-window.js` (772), `tracker-forms.js` (736), `tracker-wiki.js` (667), `tracker-panel.js` (627), `tracker-mount.js` (601).
- Embedded and development readers serve the assembled single-IIFE source, preserving the original bytes and asset identity; raw tracker test readers were switched to the assembled source before splitting so negative assertions inspect the complete source.
- After merge the lead's narrow `wsdashboard` tests passed. Each fragment is under 1000 lines.

### Phase 5: Split Pi `ask.ts` and `spawner.ts`

Intended behavior: unchanged; the bundle is rebuilt, so `dist/index.js` bytes
change but runtime behavior must not. Map:
[pi-map.md](../assets/261114-refactor-ws-split-oversized-source-files/pi-map.md)
(per-file declarations, commit order, bundle procedure, compiler-API identity
check).

Destinations (measured in the dry run):
- `src/ask.ts` 2582 -> 656 retained (header plus 1003-1452), plus
  `src/ask-threads.ts` (149-966 and 986-1001, ~857) and `src/ask-overlay.ts`
  (967-984 and 1454-2582, ~1208).
- `src/spawner.ts` 4618 -> 1095 retained, plus `spawner-tools.ts` (140-432,
  ~309), `spawner-record.ts` (453-1015, ~580), `spawner-admission.ts`
  (3464-3767, ~318), `spawner-events.ts` (3013-3462 and 4307-4351, ~521),
  `spawner-launch.ts` (434-451 and 2234-3011, ~827), `spawner-push.ts`
  (1017-2232, ~1243; it holds all ten mutable `let`s with their writers, so it
  stays one module). `ask-overlay.ts` and `spawner-push.ts` exceed 1000 lines
  and will raise soft advisories on merge.

Scaffolding: `ask.ts` and `spawner.ts` re-export every moved name (types via
`export type`) so their runtime export sets stay identical (64 and 94 names)
and no importer changes; 44 previously private declarations gain `export`;
later commits may adjust import lines of earlier new files (lead decision, see
Decisions).

Hazards: `test/mailbox-bootstrap.integration.test.ts:68-71` text-patches the
`async stopAll()` line in `spawner.ts:4606`, so `registerAgentTools` stays in
`spawner.ts`. Module-private mutable `let`s (`spawner.ts:1181-1390`,
`ask.ts:967-976`) move together with all their writers; no cross-file writes.
New import cycles carry only function and type references, never top-level
evaluation. esbuild renumbers collision suffixes, so the raw bundle diff is not
reorder-only; normalized, only `node:*` import lines differ. Open branches
`exec5/hire-child-ceiling` and `impl/develop/shove-stem-disk` edit
`spawner.ts`: quiet point required. The stale native payload found at
`f66857935` was already refreshed by `237473d31`; confirm
`test_native_pi_payload.py` passes on the execution base before the first move.

Regeneration in every src commit: `cd agents-plugin-pi && npm run bundle`, then
from the repo root `python3 agents-plugin-pi/scripts/prepare-native-payload.py
--first-party-only`; commit `dist/index.js`, both packages'
`native-pi/first-party/{dist/index.js,bundle-inputs.json}` and
`native-pi/first-party-manifest.json`. `closure.zip`/`closure-manifest.json` do
not change.

Verification boundary (per commit): `npm test` in `agents-plugin-pi` (~55 s;
17 environment failures at baseline; the sorted, timing-stripped pass/fail list
must be identical); typecheck with tsc 5.9.3 (~2.5 s; the 58 baseline errors
unchanged); `node --test test/bundle-freshness.test.mjs`; Go
`-run 'TestPiBundleCommittedIsFresh|TestPiPrepared'` (~34 s); Python
`test_native_pi_payload.py` and `test_daemon_workflow_bundle.py` (~60 s); the
compiler-API declaration identity check.

Parallelism: disjoint source files from Phase 6. Shares generated
`native-pi/first-party-manifest.json` with Phase 7: whichever merges second
regenerates instead of hand-merging.

### Result (24bf3f3e6) - 2026-11-17

Merged to develop at 24bf3f3e6 (8 move-only commits). `src/ask.ts` went from 2582 to 555 lines, with `ask-overlay.ts` (1169) and `ask-threads.ts` (849) split out; `src/spawner.ts` went from 4618 to 935 lines, with `spawner-push.ts` (1244), `spawner-launch.ts` (827), `spawner-record.ts` (580), `spawner-events.ts` (512), `spawner-admission.ts` (318), `spawner-tools.ts` (302) split out. The Pi bundle and both native first-party payloads were regenerated in the same change.
- Verification: a compiler-API proof showed all 95 ask and 220 spawner declarations and attached comments byte-identical after ignoring `export` keywords; declaration multisets are unchanged.
- The Pi suite's 17 failures and 58 tsc diagnostics are unchanged from the pre-split baseline (pre-existing; to be investigated separately).
- `ask-overlay.ts` and `spawner-push.ts` are over 1000 lines but within the 2000-line goal.

### Phase 6: Split Pi test files

Intended behavior: unchanged; test names stay identical. Map: the Tests section
of [pi-map.md](../assets/261114-refactor-ws-split-oversized-source-files/pi-map.md)
(estimates; not dry-run). Tests import only `../src/*.ts`, which Phase 5's
re-exports preserve, and test files are not bundle inputs
(`pi_bundle_test.go:46-53`), so no regeneration.

Proposed files: `ask.test.ts` -> retained (~910), `ask-threads.test.ts`
(~670), `ask-queue.test.ts` (~740), `ask-overlay.test.ts` (~680);
`spawner.test.ts` -> retained (~950), `spawner-tools.test.ts` (~440),
`spawner-events.test.ts` (~630), `spawner-push.test.ts` (~735),
`spawner-registry.test.ts` (~800); `goal-loop.test.ts` -> retained (~1280),
`goal-loop-pure.test.ts` (~550), `goal-loop-compaction.test.ts` (~1170),
`goal-loop-resume.test.ts` (~1080); the last two raise soft advisories.

Blocking hazard: shared top-level hooks are statements, not declarations
(`ask.test.ts:127-144` `afterEach`; `goal-loop.test.ts:108-110` `after`), and
`goal-loop.test.ts:663-3520` is one 2858-line `describe` whose closure preamble
(681-846) every child uses. A split therefore needs either verbatim duplication
(the describe wrapper with its doc comment and the ~184-line preamble, in two
new files) or moving shared helpers once into `test/fixtures/*.ts` with
`export`. Both exceed pure move-only; this phase waits on the lead/owner choice
(Decisions). Single-group helpers move with their users (map lists them).
Comment pointers left as-is: `test/lead-skills.test.ts:6,256`,
`test/audit.test.ts:12`.

Verification boundary: `npm test` with the identical sorted pass/fail list and
identical set of full test names (`describe` path + title) before and after;
declaration identity for top-level helpers.

Parallelism: disjoint from Phase 5; may run in parallel.

### Result (ea1998923) - 2026-11-17

Merged to develop at ea1998923 (13 test-layout commits; no src or runtime changes). `test/ask.test.ts` went from 2847 to 834 lines (new `ask-overlay` 674, `ask-queue` 741, `ask-threads` 699), `spawner.test.ts` (3476) to 875 (new `spawner-registry` 814, `spawner-push` 730, `spawner-events` 621, `spawner-tools` 422), `goal-loop.test.ts` (3520) to 1187 (new `goal-loop-compaction` 1102, `goal-loop-resume` 1033, `goal-loop-pure` 559), plus fixtures `ask-threads.ts`, `goal-loop.ts`, `spawner-records.ts`.
- Verification: gates compared the full named-test outcome multiset and the existing typecheck diagnostics against the execution baseline; the Pi suite's 17 failures and 58 tsc diagnostics are unchanged (pre-existing).
- Caveats, accepted: `node --test` discovers `test/fixtures/*.ts` (existing precedent), adding 3 load passes; loading `fixtures/goal-loop.ts` standalone leaves one empty tmp dir (minor).
- `goal-loop-compaction.test.ts` and `goal-loop-resume.test.ts` are over 1000 lines but within the 2000-line goal.

### Phase 7: Split the Claude mod watcher tests (ws and wsflow mirror)

Intended behavior: unchanged. Map: section A of
[watcher-board-map.md](../assets/261114-refactor-ws-split-oversized-source-files/watcher-board-map.md).
Identical split in `agents-plugin/hooks/claude-mod/` and
`agents-plugin-wsflow/hooks/claude-mod/` in the same commit: `watcher.test.ts`
2881 -> 1326 retained (72 tests), plus `watcher-harness.ts` (~258, new non-test
module holding the shared `harness()` from lines 57-278; 8 declarations gain
`export`), `watcher-board.test.ts` (~857, 46 tests),
`watcher-dashboard.test.ts` (~451, 22 tests).

Hazards: the wsflow tree is a hand-synced copy guarded by
`TestWsflowHooksMirrorUpToDate` (`internal/wsrsrc/wsflow_hooks_mirror_test.go:21`;
missing, extra, or differing files fail), so every commit changes both packages.
`claude plugin test` discovers `*.test.ts` by glob; `hooks.json` lists only
`register.ts`. A `*.test.ts` file with no tests fails the run and test files
cannot import each other, hence the non-test harness module. Any non-test file
under `hooks/` enters the Pi workflow bundles
(`prepare-workflow-bundles.py:25` skips only `*.test.ts`), so the harness
commit must also run `python3 agents-plugin-pi/scripts/prepare-workflow-bundles.py`
and then `python3 agents-plugin-pi/scripts/prepare-native-payload.py` (lesson
from `237473d31`) or CI's freshness step (`ws-mcp-ci.yml:83-85`) and
`TestPiPrepared*` fail. Pre-existing failure, not part of this phase:
`hooks/claude-mod/dashboard.test.ts` fails to load (`node:fs` import rejected by
the test kit) in both packages.

Verification boundary: scoped `claude plugin test` on the watcher files (~1 s;
140 pass with identical names before and after); full suite once at the end
(~2m23s per package; 763 pass, 1 pre-existing fail);
`go test ./internal/wsrsrc -run TestWsflowHooksMirrorUpToDate`;
`prepare-workflow-bundles.py --check`; Go `-run TestPiPrepared`; the
TypeScript unresolved-import check and declaration identity proof from the map.

Parallelism: disjoint source files from all other phases; shares generated Pi
bundle/payload files with Phase 5 (regenerate on the second merge).

### Result (be25443b0) - 2026-11-17

Merged to develop at be25443b0 (3 commits). `hooks/claude-mod/watcher.test.ts` went from 2881 to 1326 lines in both `agents-plugin/` and the byte-identical `agents-plugin-wsflow/` mirror, with `watcher-board.test.ts` (857), `watcher-dashboard.test.ts` (451), and the shared `watcher-harness.ts` (258) added.
- Verification: 46 board tests and 22 dashboard tests moved verbatim (72 tests remain in `watcher.test.ts`); names, helpers, comments, and bodies preserved with only import scaffolding changed.
- Merge conflicts were confined to the generated Pi workflow bundles and the native first-party payload; resolved by rerunning `prepare-workflow-bundles.py` and `prepare-native-payload.py --first-party-only`. Payload staleness tests (`TestPiPrepared*`, `TestPiDaemonWorkflowOldReaderSeam`) passed afterwards.

### Phase 8: Split `scripts/board-headless.mjs`

Intended behavior: unchanged. Map: section B of
[watcher-board-map.md](../assets/261114-refactor-ws-split-oversized-source-files/watcher-board-map.md).
A local developer CLI, not shipped; guard-covered. The entry path must stay
(opt-in Go test `history_unified_browser_test.go:146` runs it by path).
`scripts/board-headless.mjs` 2475 -> 1242 retained, plus
`scripts/board-headless/in-page.mjs` (~245), `motion.mjs` (~490),
`history.mjs` (~514).

Hazards: `main()` runs at load (`:2475`); `REPO` (`:183`) and
`scenarioHistory`'s self re-run (`:1450`) depend on `import.meta.url`, so they and
`openBoard` stay in the entry, and moved scenarios need `openBoard` back from
it. Strict move-only therefore means moved modules import from the entry (a
cycle) and the entry gains one `export {…}` list, safe only while the entry is
the sole directly-run module; the alternative is the factory-wrapper pattern of
`toolbar-overview.mjs`, which adds wrappers (Decisions). The opt-in
baseline in `scripts/board-headless/split.test.mjs` stops matching (skipped by
default, documented as not a freeze). Quiet point: ready ticket
`261115-feat-ws-dashboard-board-reorder-hold-rewrite` adds scenarios here and
unmerged `impl/develop/perm-unlit-icon` has 7 commits touching it.

Verification boundary: `node --check` on every file; no-argument usage output
identical; smoke run with the map's fake Playwright through all 21 scenarios,
output identical apart from stack line numbers (~2 s);
`node --test scripts/board-headless/split.test.mjs`; declaration identity proof.

Parallelism: disjoint from all other phases.

### Result (3137b3d85) - 2026-11-17

Merged to develop at 3137b3d85 (3 commits). `scripts/board-headless.mjs` went from 2475 to 1242 lines, with `board-headless/history.mjs` (514), `motion.mjs` (490), and `in-page.mjs` (245) split out; the entry path is unchanged.
- Verification: only import/export scaffolding changed; declaration identity, no-argument usage output, all 21 stub scenarios, static names, syntax checks, and the split tests passed, and the split matches the map prototype exactly.
- After all eight merges the lead ran `go build ./...` and `go vet ./...` (clean) and narrow tests for wswhiteboard, wsi18n, wsdashboard, mcp, wsagent payload, and wsrsrc mirror, all ok. Full CI run 43967536145 on 3137b3d85 is in progress (full CI pending).
- The vendored `mermaid.min.js` was not split (owner decision recorded above).


## Resolution (2026-11-17)

All eight split phases merged to develop with per-phase Results. The first full CI after the merges (on 3137b3d85) went red on stale test assertions unrelated to the splits, from render-cost Phase 1 and an earlier folder-revision change; the test-only fix f9250dbc1 made the next full CI on develop green.
