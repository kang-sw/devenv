---
title: "Retire the mercenary delegation surface; native harness delegation is the only path"
sage-review-design: completed
parent: 260909-epic-ws-worker-interpreter-refoundation
related:
  260909-refactor-lead-surface-collapse-worker-stop-protocol: prerequisite; it rewrites the lead playbooks that still carry the mercenary dispatch step, so removing that prose after the collapse avoids editing the same sentences twice
  260909-refactor-retire-spec-mental-model-layers: prerequisite; it retires `ai-docs/spec/`, where this surface's contract sections live, so this ticket carries no `spec:` field and edits no spec text
  260620-bug-mercenary-path-visible-when-prefer-off: drop candidate; the visibility defect disappears with the surface
  260517-bug-ws-agent-empty-result-after-tool-use: drop candidate; a mercenary runner defect with no runner left to fix
  260524-bug-ws-agent-register-stale-dir-result-hang: drop candidate; a mercenary runner defect with no runner left to fix
  260611-bug-agent-context-exhaustion-opaque-failure: drop candidate; a mercenary runner defect with no runner left to fix
  260611-research-ws-per-role-delegation-tuning-config: re-scope or drop; its per-role tuning surface is described over the mercenary tier pass-through
sage-review-completeness: completed
sage-review-design-reviewed: 4942c0194050de88
sage-review-completeness-reviewed: 4942c0194050de88
completed: 2026-09-10
---

# Retire the mercenary delegation surface; native harness delegation is the only path

## Background

`mercenary.*` is the ws-managed external-subprocess delegation family: a
registered named agent, an OS-process runner tree with per-platform launch,
cancel, and hook files, a debug/tail diagnostics family, a `ws-mcp mercenary`
CLI, a global `workflow.prefer_mercenary` config item, a product-mode marker
branch in the playbook render path, and mercenary sentences in the shipped lead
playbooks. It was built when native harness recursion could not be relied on.

Epic decision 4 makes the worker a native-harness subagent holding a
lead-capability child key, and epic decision 10 states flatly that mercenary is
deprecated and that no child may route new behavior through `mercenary.*`. The
evidence audit records the premise change behind that: depth-1 native recursion
is confirmed by the owner on Claude Code, pi, and Codex, and the alternative of
running the ws runtime as the workflow interpreter with mercenary as the main
path was rejected outright. The audit also notes that mercenary is nowhere
marked deprecated in specs or tickets today — this ticket is where the
deprecation stops being a decision and becomes mechanical.

That leaves the surface with no caller. It is already hidden by default
(`workflow.prefer_mercenary` builtin value `hide`), it is not part of the worker
topology, and it still costs tool-schema surface, a config knob, shipped
playbook prose, a branch threaded through every playbook render, a whole
process-runner package, and a CLI family. Epic decision 1's truth criterion
applied to a code layer rather than a document layer gives the same verdict:
maintenance without a consumer.

## Decisions

1. **Remove, do not leave permanently hidden.** `hide` is already the builtin
   default, so keeping the code and flipping nothing buys no behavior — it only
   preserves the maintenance cost the truth criterion (epic decision 1) targets.
   Rejected: keep the tools registered and hard-code the hidden state. That
   trades a config knob for dead code and leaves the render-path branch, the
   runner tree, and the CLI in place, which is most of the cost.
2. **Native harness delegation is the only supported path.** Taken from the
   research ticket's rejected alternatives: "ws runtime as the workflow
   interpreter (mercenary as main path)" was rejected by the owner, and epic
   decision 4 names the native-harness worker as the mechanism.
3. **No "fallback for harnesses without native recursion" carve-out.** The
   research ticket records that `ai-docs/ref/agent-harness-capability-tiers.md`
   has no Claude Code or pi column and that its recursion claims are playbook
   prose rather than fixtures. A carve-out would therefore rest on unverified
   prose, and epic decision 10 states the deprecation without a conditional.
4. **`exec.*` is out of scope, and this was checked rather than assumed.** The
   exec job family has its own package (`internal/execjob`), its own dispatch
   cases, and its own contract section; nothing in it imports or reaches the
   mercenary runner. It is not mercenary-only and is untouched here.
5. **Tier resolution survives the removal.** `wsconfig.ResolveAgentForHarnessConfig`
   and `config.resolve_agent` back native model selection through
   `playbook.render`'s recommended-tier, independently of the mercenary runner.
   Only the mercenary pass-through of that tier goes.
6. **Tests naming the removed surface are deleted with it, not skipped.** A
   green suite that still asserts mercenary behavior would keep the surface
   alive as a contract.
7. **Board fallout is a user-and-lead action.** Epic decision 8 keeps inventory
   stage moves with the user and lead. This ticket surfaces the drop candidates;
   it does not move them.

## Constraints

- **Shipped-surface rule (`AGENTS.md` Architecture Rule 4, epic decision 11).**
  The removal edits text that ships: `lead-workflow-manual`, `lead-tune`, the
  `lead-tune` skill description, and the delegation footer the Go render path
  appends to every `delegates: true` playbook. `lead-implement` no longer
  exists under that name: `260909-refactor-lead-surface-collapse-worker-stop-protocol`
  landed and renamed it to `agents-plugin/rsrc/implementer/implementer.md`,
  which already carries no mercenary sentence (`grep -in mercenary
  agents-plugin/rsrc/implementer/implementer.md` returns nothing), so it needs
  no rewrite here. Each rewritten sentence must read correctly in a project
  that has never heard of this repository — no refoundation vocabulary, no
  ticket stems, no package paths. The replacement text should describe native
  delegation on its own terms rather than describing what was removed.
- **wsflow mirroring (`ai-docs/manuals/wsflow-mirroring.md`).**
  `lead-tune` and `lead-workflow-manual` are in the shipped wsflow skill set
  (`lead-implement` is not: it was renamed to `implementer`, which ships only
  under `rsrc/`, carries no skill wrapper in either package, and already has
  no mercenary text); `agents-plugin-wsflow/rsrc/` is generated
  byte-identical from the shared playbooks. wsflow already omits
  `workflow.prefer_mercenary` as a full-ws-only knob and already strips
  `ws:full-only` and `ws:mercenary-on` blocks, so this removal *collapses* a
  product-mode difference rather than creating one. Run the wsflow package tests
  in the same change.
- **No new behavior may route through `mercenary.*` while the removal is in
  flight** (epic decision 10).
- **Removal is whole-package, not per-platform.** The runner tree carries
  paired `_unix.go` / `_windows.go` files for async command start, cancel,
  process snapshot, hook quoting, and atomic replace; a partial removal leaves
  one platform building and the other not.
- **On-disk agent state is not migrated.** Prior art: the retired typed
  `PreferMercenary` session field is silently ignored on read rather than
  migrated away. Follow that precedent unless the sage review says otherwise.
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/rsrc/, agents-plugin-wsflow/skills/, agents-plugin-tool/internal/wsdoc/conventions/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)

## Prior Art

Found by grepping `mercenary` across `agents-plugin-tool/`, `agents-plugin/`,
`agents-plugin-wsflow/`, `ai-docs/spec/`, and the ticket directories.

**Tool dispatch, schema, and gating** — `agents-plugin-tool/internal/mcp/server.go`:
the `mercenary.register/call/wait/result/status/interrupt/tail/cancel/recall/print/erase`
dispatch cases and the collapsed `mercenary.debug.*` case; the matching
tool-schema entries; the tool-profile name list; `mercenaryHiddenFromConfig`
and `mercenaryHiddenFromGlobalConfig` feeding the `tools/list` filter and the
explicit-call gate; `canonicalPreferMercenaryValue`; the
`workflow.prefer_mercenary` tuning-catalog knob; and the `config.tune`
description strings that name the key.

**Render-path coupling** — `internal/mcp/playbook_tools.go`: the always-on
"Mercenary path (always available)" unit inside the delegation tip;
`mercenaryGuidanceBlock` (the `prefer_mercenary active` block for
implementer/reviewer roles); the `mercenaryEnabled` branch and the
`mercenaryOnlyStart/End` constants inside `selectProductModeBlocks` (the
function itself stays: it also strips `ws:full-only` / `ws:wsflow-only`
blocks, which is the product-mode mechanism); `renderProductModePlaybookBody`;
and the `preferMercenary` parameter threaded through `renderPlaybookBody`,
`renderPlaybook`, and the `playbook.render` dispatch case that resolves it.

**Config item** — `internal/wsconfig/scope.go`
(`ItemWorkflowPreferMercenary`, the retired unprefixed `ItemPreferMercenary`,
and its `RegisterGlobalOnly` registration) and `internal/mcp/config_registry.go`
(`preferMercenaryEnum` and the registry entry with `NoAgentVisible: false`).

**Session-auth precedent** — `internal/mcp/session_auth.go` documents that the
former typed `PreferMercenary` bool was retired and old records carrying the
JSON field are silently dropped on read. That is the model for retiring the
remaining knob without a data migration.

**Runner tree** — `internal/wsagent/`: `agent.go` (registry, call lifecycle,
runtime log, `backendInvocationError`, the `SelfWorkerStarter` re-entry that
shells back into `ws-mcp mercenary run-current`, and the
`ws-mcp mercenary check-inbox` hook command), `claude.go`, `codex.go`
(`runnerForBackend`, backend version capture), and the paired platform files
for async command, cancel process, cancel tree, process snapshot, hook quoting,
and file replace. Only `cmd/ws-mcp/main.go`, `internal/mcp/server.go`, and their
tests import this package; `internal/wsrsrc/loader.go` references it in a
comment only. One coupling is by path, not import:
`internal/wsstore/store_test.go` (`TestRuntimeMetadataInventoryCoversCurrentJSONFields`)
parses `../wsagent/agent.go` from disk and fails if the file is gone. The
store also holds the runner's persistence API — `AgentDefinition`,
`UpsertAgentDefinition` / `DeleteAgentDefinition`,
`migrateAgentDefinitionsToInstances`, `PruneAgentInstances` — and the
`agent.json` / `current/state.json` rows of `metadata_inventory.go`, whose
only producer is the runner.

**CLI** — `cmd/ws-mcp/main.go`: the `mercenary` subcommand and its
`register|call|run-current|wait|result|status|interrupt|check-inbox|tail|debug|cancel|recall|print|erase`
dispatch, the mercenary entries in the tool-name list, and the top-level usage
string.

**Marker and mirror plumbing** — `internal/wsrsrc/skills_mirror.go` (the
`mercenary` entry in `disqualifyingTokens`, the substitution-eligibility guard
that hard-fails a wsflow mirror source containing the word), `internal/wsrsrc/wsrsrc.go`
(parse-only tier comment referencing mercenary model routing), and
`internal/mcp/workflow_manual.go` (marker-list comment).

**Smoke** — `agents-plugin-tool/scripts/smoke-ws-mcp.sh` registers a
`smoke-reviewer` mercenary as part of the smoke run.

**Shipped text** — `agents-plugin/rsrc/lead-implement/lead-implement.md` no
longer exists: `260909-refactor-lead-surface-collapse-worker-stop-protocol`
landed and renamed it to `agents-plugin/rsrc/implementer/implementer.md`,
which already carries no mercenary or `ws:full-only` text (`grep -in
"mercenary\|full-only" agents-plugin/rsrc/implementer/implementer.md` returns
nothing) — nothing to rewrite there.
`agents-plugin/rsrc/lead-workflow-manual/lead-workflow-manual.md` (English-prompt
rule, the register/call/result delegation walkthrough, and the cancellation
note), `agents-plugin/rsrc/lead-tune/lead-tune.md` (the
`workflow.prefer_mercenary` handler and its request-mapping row),
`agents-plugin/skills/lead-tune/SKILL.md` (description names
"mercenary-vs-native delegation"), plus the `agents-plugin-wsflow/rsrc/` mirrors
of each.

**Tests to remove or rewrite** — `internal/mcp/mercenary_surface_test.go`,
`internal/mcp/prefer_mercenary_phase2_test.go`, mercenary cases in
`internal/mcp/playbook_tools_test.go`, `prompt_override_test.go`,
`server_test.go`, `session_auth_test.go`; `cmd/ws-mcp/main_test.go`;
`internal/wsagent/*_test.go`; `internal/wsconfig/scope_test.go`;
`internal/wsrsrc/skills_mirror_test.go`; and
`agents-plugin-wsflow/tests/test_wsflow_runtime_contract.py`.

**Contract text retired under the sibling ticket, listed here for traceability
only** — `260909-refactor-retire-spec-mental-model-layers` has landed and
archived `ai-docs/spec/` and `ai-docs/mental-model/` in full (`ls ai-docs/spec/
ai-docs/mental-model/`: no such file or directory), so
`ai-docs/spec/named-agent-runtime.md`, the `{#260505-named-agent-mcp-tools}`
and `{#260610-mercenary-delegation-surface}` sections of
`ai-docs/spec/mcp-tools.md`, and the mercenary mentions in
`ai-docs/spec/workflow-skills.md` and `ai-docs/spec/plugin-runtime.md` are
already gone with the rest of the corpus; no spec text remains for this
ticket to touch.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | internal/mcp/server.go, internal/mcp/playbook_tools.go, internal/mcp/config_registry.go, internal/mcp/workflow_manual.go, internal/mcp/session_auth.go, internal/wsconfig/scope.go, internal/wsagent/ (whole package), internal/wsstore/store.go, internal/wsstore/metadata_inventory.go, internal/wsrsrc/skills_mirror.go, internal/wsrsrc/wsrsrc.go, cmd/ws-mcp/main.go, agents-plugin/rsrc/lead-workflow-manual/lead-workflow-manual.md, agents-plugin/rsrc/lead-tune/lead-tune.md, agents-plugin/skills/lead-tune/SKILL.md, plus the agents-plugin-wsflow/ mirrors and the listed test files |
| scope.surface | public-interface | removes the mercenary.* MCP tool family, the workflow.prefer_mercenary config key, and the ws-mcp mercenary CLI subcommand, all caller-facing surface even though currently hidden by default |
| scope.new_public_symbol | no | none; both phases are pure removal, confirmed against internal/mcp/server.go, internal/wsconfig/scope.go, internal/mcp/config_registry.go |
| scope.new_type_contract | no | none; no new type or function signature is introduced |
| scope.test_surface | existing | internal/mcp/mercenary_surface_test.go, prefer_mercenary_phase2_test.go, playbook_tools_test.go, prompt_override_test.go, server_test.go, session_auth_test.go, cmd/ws-mcp/main_test.go, internal/wsagent/*_test.go, internal/wsconfig/scope_test.go, internal/wsrsrc/skills_mirror_test.go, agents-plugin-wsflow/tests/test_wsflow_runtime_contract.py all exist today and carry mercenary references (verified by grep) |
| complexity.reuse_points | not-applicable | this is a deletion ticket; it removes an existing component rather than reusing one |
| complexity.side_effect_risk | moderate | removes a config key, CLI subcommand, and MCP tool family that a downstream installation could still be calling even though workflow.prefer_mercenary already defaults to hide; also edits Architecture-Rule-4 shipped text read by projects that never installed this repository |
| risk.correctness | moderate | large mechanical removal across paired _unix.go/_windows.go platform files and several packages (internal/mcp, internal/wsagent, internal/wsstore, internal/wsrsrc, cmd/ws-mcp) where a missed reference leaves one platform building and the other not, per the ticket's own whole-package-not-per-platform constraint |
| risk.fit | low | matches the landed epic decisions (4, 10) and the already-landed sibling removals (260909-refactor-lead-surface-collapse-worker-stop-protocol, 260909-refactor-retire-spec-mental-model-layers) in shape and rationale |
| risk.test | moderate | many existing mercenary-asserting tests must be deleted or rewritten rather than skipped (epic constraint); the delegate-orientation.md deletion was moved from Phase 1 to Phase 2 at fact population so `ws-mcp mercenary register` (still run by scripts/smoke-ws-mcp.sh until Phase 2) keeps loading it through Phase 1 |
| risk.security_or_contract | moderate | removing a documented MCP tool family, config key, and CLI subcommand is a breaking contract change for any already-installed downstream caller of mercenary.*, even though the surface is hidden by default today |

## Phases

### Phase 1: Remove the caller-visible mercenary surface

Goal: after this phase no MCP caller — lead or worker — can reach or hear
about a mercenary; the CLI family and the runner stay until Phase 2. Delete the `mercenary.*` MCP tool family including the
`mercenary.debug.*` diagnostics; delete the `workflow.prefer_mercenary` config
item, its registry entry, its tuning-catalog knob, and its mentions in the
`config.tune` and `config.list` descriptions; delete the
`<!-- ws:mercenary-on -->` marker branch and the `preferMercenary` parameter
threaded through the render path; delete the always-on "Mercenary path (always
available)" delegation-tip unit and `mercenaryGuidanceBlock`; and rewrite the
mercenary sentences in `lead-workflow-manual`, `lead-tune`, and the `lead-tune`
skill description so each reads as native-delegation guidance on its own terms
(`lead-implement` was renamed to `agents-plugin/rsrc/implementer/implementer.md`
by `260909-refactor-lead-surface-collapse-worker-stop-protocol` and already
carries no mercenary text; nothing to rewrite there). Mirror every shared-playbook edit into
`agents-plugin-wsflow/`. Leave `internal/wsagent/` in the tree so this phase is
one reviewable removal of the surface rather than a mixed surface-and-runtime
change.

Verification expectations:

- `tools/list` and the runtime capability listing contain no `mercenary.*`
  entry, and an explicit call to any of them is rejected as unknown, with no
  config value able to bring them back.
- `config.list` no longer offers `workflow.prefer_mercenary`, and
  `config.tune` on that key errors rather than writing orphaned state.
- `playbook.render` output for the implementer and reviewer roles contains no
  mercenary sentence in either product mode; diff a before/after render and
  confirm the only deltas are the removed units.
- `grep -ri mercenary agents-plugin agents-plugin-wsflow` returns nothing.
- `go build ./...` and `go test ./...` are green with the mercenary-only tests
  deleted rather than skipped; the wsflow package tests pass.
- The full-ws versus wsflow rendering difference for the touched playbooks
  narrows to the namespace substitution.

Touchpoints: `internal/mcp/server.go`, `internal/mcp/playbook_tools.go`,
`internal/mcp/config_registry.go`, `internal/wsconfig/scope.go`,
`internal/mcp/workflow_manual.go` (not `agents-plugin/rsrc/lead-implement/`:
that path is gone, renamed to `agents-plugin/rsrc/implementer/implementer.md`,
which already has no mercenary text to remove),
`agents-plugin/rsrc/lead-workflow-manual/`, `agents-plugin/rsrc/lead-tune/`,
`agents-plugin/skills/lead-tune/SKILL.md`, the `agents-plugin-wsflow/` mirrors,
`agents-plugin-wsflow/skills/lead-tune/SKILL.md` (hand-curated, not a
substitution mirror), `agents-plugin/runtime.json` (the `mercenary.*`
tool-window entries; `cmd/ws-mcp/main_test.go` asserts the advertised tool
set equals that file), `agents-plugin/tests/test_skill_dispatch_contracts.py`
(pins a `mercenary.call` sentence),
`agents-plugin/tests/test_ws_mcp_launcher_capabilities.py` (sample payload),
the test files listed under Prior Art. `agents-plugin/rsrc/delegate-orientation.md`
and its manifest entry stay until Phase 2: `internal/wsagent/agent.go`
(`loadDelegateOrientation`) still loads it while the runner and the
`ws-mcp mercenary` CLI remain live through this phase, so it is deleted with
its reader there (its reporting rule already lives in the worker stop protocol
placed by `260909-refactor-lead-surface-collapse-worker-stop-protocol` Phase 1).

### Result (86aca803) - 2026-09-10

Landed as `232752e2` (surface removal), `09c055b7` (shipped text and mirrors),
`86aca803` (review fixes) on `impl/epic/refound/bagel-grape-sway`.

The `mercenary.*` MCP family is gone: dispatch cases, tool schemas, the
tool-profile name list, `mercenaryHiddenFromConfig` /
`mercenaryHiddenFromGlobalConfig` / `canonicalPreferMercenaryValue` /
`agentCallHandleText` / `agentDebugSchema`, and the mercenary arms of
`toolAllowed`, `roleAllowsTool`, `noAgentHiddenTool`, `filteredTools`,
`publicToolDefinition`, `toolSchemaRequiresSessionKey`, and `LeadToolNames`.
`workflow.prefer_mercenary` is gone from `wsconfig` scope, the config registry,
the tuning catalog, and the builtin defaults. The `ws:mercenary-on` marker
branch, the `preferMercenary` parameter through
`renderProductModePlaybookBody` / `renderPlaybookBody` / `renderPlaybook`, the
"Mercenary path (always available)" delegation-tip unit, and
`mercenaryGuidanceBlock` are gone. `internal/mcp` no longer imports
`internal/wsagent`; the runner and the `ws-mcp mercenary` CLI stay live for
Phase 2.

Shipped text: `lead-workflow-manual` lost all three mercenary passages — the
English-prompt rule now covers delegated subagents generally, the
register/call/result walkthrough became a product-neutral "Delegate prompts"
section (render the delegate prompt, hand the returned path to a native
subagent at the recommended tier), and the cancellation note now says
interruption is the harness's affordance. `lead-tune` lost the "tune delegation
mode" handler and its request-mapping row; the `lead-tune` skill description
lost "mercenary-vs-native delegation". Those were the only `ws:full-only`
blocks in either file, so the full-ws/wsflow difference for both playbooks is
now the namespace substitution alone. wsflow mirrors regenerated with the
documented entrypoints in the documented order; nothing hand-edited.

Verification: `go build ./...` clean; `go test ./... -count=1` green across all
14 packages; `gofmt -l` clean for every file this phase touched;
`python3 -m unittest discover agents-plugin/tests` 55 OK and
`agents-plugin-wsflow/tests` 10 OK. Against a binary built from the branch:
`tools/list` advertises 50 tools and no `mercenary.*`; `mercenary.call` returns
`-32602 unknown tool`; `config.list` offers no `workflow.prefer_mercenary` and
`config.tune` on that key errors as an unknown config key rather than writing
orphaned state. A before/after `playbook.render` diff at `c0a23a2f` vs the
branch head, in both product modes: `code-review-correctness` differs by
exactly the removed "Mercenary path" paragraph and nothing else; `implementer`
is byte-identical (it declares `delegates: false`, so it never carried the
tip). Reviewed in two rounds by fresh correctness and test reviewers
(partitioned); round 1 raised three Important findings, all fixed in
`86aca803`; round 2 verified all three and raised nothing new.

Decisions taken:

- `agents-plugin/runtime.json` needed no edit. The phase's touchpoint list
  calls its mercenary rows "tool-window entries", but `runtime.json["tools"]`
  holds none — all 17 mercenary rows are under `["commands"]`, which describes
  the `ws-mcp mercenary` CLI that this phase deliberately keeps.
  `cmd/ws-mcp/main_test.go` asserts the advertised command set equals that
  file, so removing the rows here would break the contract against a CLI that
  still exists. The same reasoning keeps the `mercenary.*` rows in
  `agents-plugin-wsflow/tests/test_wsflow_runtime_contract.py`'s
  `HIDDEN_COMMANDS`; its `HIDDEN_TOOLS` mercenary rows were removed. Therefore
  the phase's `grep -ri mercenary agents-plugin agents-plugin-wsflow` clean
  expectation is not met yet and lands with the CLI in Phase 2.
- Two mercenary-named test files were pruned and renamed rather than deleted:
  they also carried child-key mint, golden shipped-delegate render,
  recommended-tier, and `workflow.prefer_subagent` coverage that outlives the
  surface. `mercenary_surface_test.go` -> `playbook_render_surface_test.go`;
  `prefer_mercenary_phase2_test.go` -> `workflow_prefer_subagent_test.go`.
- Three tests observed harness detection and tier/effort clearing *through*
  `mercenary.call`; they were retargeted onto `config.resolve_agent` rather
  than deleted, so decision 5's surviving tier resolution keeps its coverage.
  The ServeStdio concurrency test was rebuilt on `exec.shell` + `exec.result`.
- The surviving `NoAgentVisible` mechanism lost its only `false` entry with the
  mercenary registry row, which made the agentless tuning-catalog test vacuous.
  A new test flips one live entry for its duration to exercise
  `buildTuningCatalog`'s agentless cut; it was mutation-checked against both a
  neutered and an inverted filter.
- The "Delegate prompts" section is product-neutral rather than `ws:full-only`:
  its subject is native subagents, which both product modes have, and leaving
  it gated would have kept a product-mode difference this phase exists to
  collapse. It is the manual's only remaining description of rendering a
  delegate prompt.

Not done here (Phase 2): `internal/wsagent/`, the `ws-mcp mercenary` CLI and
its usage string, `runtime.json` command rows, `scripts/smoke-ws-mcp.sh`,
`delegate-orientation.md`, the `wsstore` runner persistence API, and the
`disqualifyingTokens` / `wsrsrc` / `workflow_manual.go` marker-comment cleanup.

### Phase 2: Remove the mercenary runtime, CLI, and marker plumbing

Sequentially dependent on Phase 1: the runner tree is reachable only through
the MCP tools removed there and through the `ws-mcp mercenary` CLI, so deleting
it first would break live dispatch mid-change.

Goal: delete `internal/wsagent/` including every paired platform file; delete
the `ws-mcp mercenary` subcommand family, its entries in the CLI tool-name list,
and the mercenary token in the usage string; delete the `SelfWorkerStarter`
re-entry and the `check-inbox` hook command it installs; drop the mercenary
registration step from `scripts/smoke-ws-mcp.sh`; delete
`agents-plugin/rsrc/delegate-orientation.md` and its manifest entry in both
packages together with the runner that was its only reader; drop the `mercenary` entry
from `disqualifyingTokens` in `internal/wsrsrc/skills_mirror.go`; clean the
stale references in `internal/wsrsrc/wsrsrc.go` and
`internal/mcp/workflow_manual.go`; and remove the runner's persistence API
from `internal/wsstore` (`AgentDefinition`, `UpsertAgentDefinition`,
`DeleteAgentDefinition`, `migrateAgentDefinitionsToInstances`,
`PruneAgentInstances`) together with the `agent.json` / `current/state.json`
rows of `metadata_inventory.go`, re-anchoring
`TestRuntimeMetadataInventoryCoversCurrentJSONFields` so it no longer parses
`../wsagent/agent.go` from disk. Leave existing on-disk agent registries and
runtime logs under the ws cache home orphaned rather than writing a deletion
migration, following the retired-session-field precedent; the rows are
orphaned data, the Go API is dead code and goes.

Verification expectations:

- `go build ./...` and `go test ./...` green with no `internal/wsagent`
  package in the tree and no dangling import.
- `ws-mcp` usage output no longer lists `mercenary`; the subcommand errors as
  unknown.
- `scripts/smoke-ws-mcp.sh` completes without the mercenary step.
- `grep -ri mercenary agents-plugin-tool` returns nothing outside deleted-file
  history.
- The runtime metadata inventory gate passes against the re-anchored
  inventory, and the `exec.*` family is unaffected — exercise an
  `exec.spawn`/`exec.result` round trip to confirm the shared runtime path
  still works after the runner package is gone.

Touchpoints: `internal/wsagent/` (delete), `agents-plugin/rsrc/delegate-orientation.md`
and its manifest entries (delete), `cmd/ws-mcp/main.go`,
`internal/wsstore/{store,metadata_inventory}.go` and `store_test.go`,
`agents-plugin-tool/scripts/smoke-ws-mcp.sh`,
`internal/wsrsrc/skills_mirror.go`, `internal/wsrsrc/wsrsrc.go`,
`internal/mcp/workflow_manual.go`, `cmd/ws-mcp/main_test.go`.

Board fallout surfaced by this removal — a user-and-lead inventory action per
epic decision 8, not part of this ticket's implementation:
`260620-bug-mercenary-path-visible-when-prefer-off` (the visibility defect
disappears with the surface); `260517-bug-ws-agent-empty-result-after-tool-use`,
`260524-bug-ws-agent-register-stale-dir-result-hang`, and
`260611-bug-agent-context-exhaustion-opaque-failure` (runner defects with no
runner left to fix); `260611-research-ws-per-role-delegation-tuning-config`
(its tuning surface is described over the mercenary tier pass-through and needs
re-scoping to native tiers or dropping). Tickets that merely mention mercenary
in passing — the harness-pivot research, the opencode drop-in research, the
pre-release cleanup and API-namespace epics, the dogfood workset — need a
mention sweep rather than a drop.

### Result (99573fea) - 2026-09-10

Landed as `3d36decd` (runtime, CLI, and marker plumbing), `65b1a812`
(delegate-orientation and drifted manuals), `99573fea` (review fixes) on
`impl/epic/refound/bagel-grape-sway`.

`internal/wsagent/` is gone whole — 26 files, every paired `_unix.go` /
`_windows.go` among them. The `ws-mcp mercenary` subcommand family, its
`runtime capabilities` command rows, its `runtime.json` command entries, the
usage-string token, and the smoke script's registration step went with it, as
did the matching `HIDDEN_COMMANDS` rows in the wsflow runtime-contract test.
`usage()` collapsed from a two-branch product-mode string to one, since the
retired verb was the only difference. From `internal/wsstore`: `AgentDefinition`
and its upsert/read/delete, `AgentInstanceRetentionTTL`,
`AgentInstanceCleanupResult`, `PruneAgentInstances`,
`migrateAgentDefinitionsToInstances`, `AgentInternalKey`, and the private
helpers that served only them; the `agent.json` and `current/state.json` rows of
`metadata_inventory.go` went too, and
`TestRuntimeMetadataInventoryCoversCurrentJSONFields` is re-anchored onto
`internal/execjob` so it no longer parses a deleted file from disk. The
`mercenary` entry left `disqualifyingTokens`, and the stale comments in
`wsrsrc.go`, `loader.go`, and the two `TestMain` docstrings were rewritten.
`agents-plugin/rsrc/delegate-orientation.md` and its wsflow mirror are deleted
with the runner that was their only reader; both manifests were regenerated with
the documented entrypoints in the documented order and came out byte-identical.

Verification: `go build ./...` clean; `go vet ./...` clean; `go test ./...
-count=1` green across all 13 remaining packages; `gofmt -l` clean for every
file this phase touched (`internal/wsconfig/config.go` and `global.go` are
pre-existing offenders this range does not touch); `python3 -m unittest discover
agents-plugin/tests` 55 OK and `agents-plugin-wsflow/tests` 10 OK. Cross-platform
builds confirm the whole-package-not-per-platform constraint held: `GOOS=windows
GOARCH=amd64` and `GOOS=darwin GOARCH=arm64` both build clean, and `go vet` under
`GOOS=windows` compiles the Windows-side test tree too. Against a binary built
from the branch: `ws-mcp` usage prints
`<version|doctor|runtime|serve|smoke|config|path|git|tickets>`;
`ws-mcp mercenary status --name impl` exits 2 with that usage;
`ws-mcp runtime capabilities` reports 18 commands and no `mercenary.*` in either
list; `scripts/smoke-ws-mcp.sh` completes without the registration step. A live
stdio round trip on that binary confirms the `exec.*` family is unaffected —
`ferrule` -> `exec.shell` returned `status: succeeded`, `exit_code: 0`, stdout
captured — and `playbook.render` still succeeds, which forces a full rsrc
manifest load and proves the manifest-entry deletion left the tree loadable.
`grep -ri mercenary` over `agents-plugin-tool`, `agents-plugin`, and
`agents-plugin-wsflow` returns nothing, so Phase 1's deferred clean expectation
is now met. Reviewed in two partitions by fresh correctness and test reviewers;
correctness returned clean with three Minor, test returned one Important.

Decisions taken:

- `wsstore` keeps the `agent_defs` / `agent_instances` DDL, their column
  migrations, and the `Count` allowlist. The phase's instruction is that on-disk
  state stays orphaned rather than getting a deletion migration, and the schema
  is what lets an existing store still open with those rows readable. Only the Go
  API on top of them is dead code. The correctness reviewer flagged the residue
  as Minor and in-scope-as-written: the remaining schema work is now pure cost on
  every store open, which is a follow-up for whoever decides to prune the cache
  home.
- Three SQLite busy-retry tests observed the retry wrap *through* the
  agent-definition write and point-read paths; they were retargeted onto
  `UpsertExecJob` / `ExecJob` rather than deleted, since `withSQLiteRetry` is
  shared store machinery that outlives the runner and `exec_jobs` is the
  surviving table with the same single-row write-then-read shape.
  `TestAgentRolePointerHistoryAndCollision` was deleted instead: per-root pointer
  collision is a property of the removed API alone.
- The negative guards naming `mercenary.` in the `tools/list` and
  playbook-render tests were removed rather than kept, because this phase's own
  verification expectation is a clean `grep -ri mercenary agents-plugin-tool` and
  after it no code path can emit the token. Where the token was one entry in a
  larger forbidden-token list, only that entry was dropped and the test kept.
- The test reviewer's one Important — add a CLI test invoking the retired
  subcommand by name and asserting it errors as unknown — is
  **[won't fix: it reintroduces the exact token the phase's grep-clean
  expectation exists to remove, and the unknown-subcommand path is already
  covered three times over by the retired-documentation-verb loop in the same
  file]**. The concern underneath it is addressed token-free by a new
  `TestTopLevelUsageListsTheWholeSubcommandSet`, which pins the exact advertised
  verb set in both product modes: a retired verb's dispatch case and its usage
  token have to disappear together, and the assertion catches either half going
  missing. Mutation-checked by inserting a fake verb into the usage string.
- `ai-docs/manuals/ws-agent-runtime.md` is archived to `ai-docs/.old/manuals/`
  rather than deleted, following the precedent the sibling layer retirement set.
  Its entire subject was the deleted runner; leaving it live would have kept the
  ambient manual index advertising a contract for absent code. The mercenary
  round-trip item in the Windows dogfood checklist was rewritten onto `exec.*`,
  which is where the per-platform process start/snapshot/cancel path still lives,
  with a clause on how to drive tools that dispatch but are never advertised in
  `tools/list`.

Follow-up left open, both Minor from the correctness review and outside this
phase's named touchpoints: `wsstate.Layout.AgentsDir` is now a dead field whose
`Manager.Ensure` still creates an empty `agents/` directory per worktree; and
the orphaned `agent_defs` / `agent_instances` schema noted above.

Board fallout listed in the phase plan (the four mercenary/runner bug tickets and
the per-role tuning research) is a user-and-lead inventory action per epic
decision 8 and was not acted on here.

## Open Questions

- Should a release between now and removal emit a deprecation notice (a
  `mercenary.*` call returning a removal message) for already-installed
  projects, or does removal land in one release? The epic settles that mercenary
  is deprecated and that no new behavior routes through it, but not release
  sequencing for existing installs.
- Should stored mercenary state under the ws cache home be pruned by a one-time
  cleanup instead of left orphaned? Phase 2 proposes "left orphaned" on the
  session-field precedent; the epic does not settle it.
- `exec.*` was checked and is out of scope here, but the survey also found no
  shipped playbook text referencing it. Whether that family has a caller is the
  same no-consumer question in a different place, and belongs to
  `260903-epic-mcp-tool-surface-affordance-reduction` rather than this ticket.
