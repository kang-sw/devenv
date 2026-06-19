<!-- Memory policy: prune aggressively as project advances. Completed
     work belongs in git history, not here. Keep only what an AI session
     needs to orient itself and pick up work. If it's derivable from
     code or git log, delete it from this file. -->

# devenv - Project Index

## Repo

Meta-workflow repository for workflow documents, skills, agents, plugin
packaging, helper commands, MCP tooling, and dev-environment templates. Specs,
tickets, and mental models here describe the workflow system itself; downstream
application material belongs in downstream projects.

Active plugin package: `agents-plugin/` (`ws@0.30.0`).
Agentless derivative package: `agents-plugin-wsflow/` (`wsflow@0.30.0`).
Native MCP/tooling source: `agents-plugin-tool/`.
Dashboard scaffold: `ws-dashboard/` (Rust workspace with core, harness-core,
harness-cli, bind-guarded daemon shell, resource API fixtures, and a React/Vite
inspectable frontend shell).
Retired Claude source material: `ai-docs/ref/claude-home-legacy.md` and git
history.

## Current Branch Rules

- Verify the current branch with `git status`; do not rely on `_index.md` for a
  live branch name.
- No branch-specific spec or mental-model freeze is active.
- Keep `.codex` untracked unless the user explicitly asks to stage it.

## Plugin Topology

- `./install.sh update` handles first-time install and settings patching on a
  new machine.
- Root `CLAUDE.md` is the only live Claude compatibility shim and points at
  `AGENTS.md`.
- `install.sh` snapshots only `agents-plugin/` for Claude-compatible plugin
  installs when Claude Code is available; it intentionally does not install
  wsflow into Claude.
- `agents-plugin/` is registered through `.agents/plugins/marketplace.json`;
  Codex UI install has verified `ws:lead-skill-authoring`,
  `ws:lead-write-ticket`, and `ws:lead-discuss`.
- `agents-plugin-wsflow/` is an agentless derivative package with
  Codex/Claude manifests, package-local no-agent MCP env, shared launcher
  copies, a reduced `runtime.json`, thin wsflow skill shims over shared
  playbooks, and package tests for runtime-contract plus skill-shim drift.
- `.agents/plugins/marketplace.json` exposes both `ws` and `wsflow` as local
  Codex plugin entries; `.claude-plugin/marketplace.json` exposes both packages
  for manual Claude marketplace installation while `install.sh` still installs
  only `ws`.
- Codex local plugin iteration has no known CLI refresh path; use UI
  uninstall/install or a fresh Codex session after editing the registered source.
- `agents-plugin/.codex-plugin/plugin.json` references plugin-local `.mcp.json`
  through `"mcpServers": "./.mcp.json"`.
- Changed plugin-managed Codex MCP config requires user-performed plugin cache
  refresh before installed-cache verification.
- `claude plugin validate agents-plugin` passes; runtime Claude invocation of
  `agents-plugin` remains compatibility behavior, not a separate source tree.
- `ai-docs/.old/` is the Git-tracked project archive for inactive reference
  material that should not appear in default file listings.

## Read Before Editing

| File | Use |
|------|-----|
| `agents-plugin/skills/lead-skill-authoring/SKILL.md` | Skill/agent/prompt/convention authoring rules |
| `ai-docs/ref/wsflow-mirroring.md` | Required before editing full ws skills or plugin surfaces that may need wsflow mirrors |
| `ai-docs/ref/codex-integration.md` | Probed Codex CLI behavior |
| `ai-docs/ref/ws-mcp.md` | MCP operational runbook, launcher environment, release and verification steps |
| `ai-docs/ref/ws-agent-runtime.md` | Durable agent runtime contract |
| `ai-docs/ship/ws.md` | Release process for `ws` |
| `ws/infra.read("impl-playbook")` | Implementation discipline |
| `ws/infra.read("subagent-rules")` | Subagent dispatch rules |
| `ws/infra.read("executor-wrapup")` | Shared post-implementation wrapup |

Before editing tickets/specs/mental models, read the matching convention through
`ws/convention.read`. Before editing skill, agent, prompt, or convention text,
read `agents-plugin/skills/lead-skill-authoring/SKILL.md`. Before editing full
`agents-plugin/skills/lead-*` skills, plugin packaging, runtime contracts,
launcher behavior, prompt guidance, or release validation that may affect
wsflow, read `ai-docs/ref/wsflow-mirroring.md` and run
`python3 -m unittest discover agents-plugin-wsflow/tests` when the derivative
surface may drift.

## Runtime Surfaces

MCP behavior contracts live in `ai-docs/spec/mcp-tools.md`,
`ai-docs/spec/plugin-runtime.md`, and related mental models. Current tool
schemas and inventory are runtime-discoverable through `tools/list` and
`runtime capabilities`; do not copy them into project memory or reference docs.
Shared `agents-plugin` skill text uses MCP names, not repo-local paths. Infra
and convention text are bundled into the runtime and read through
`ws/infra.read` and `ws/convention.read`.

## MCP Runtime Notes

Runtime and launcher contracts are maintained in `ai-docs/spec/plugin-runtime.md`,
`ai-docs/spec/mcp-tools.md`, and the source under `agents-plugin-tool/` and
`agents-plugin/bin/`. `ai-docs/ref/ws-mcp.md` is the operational runbook for
launcher environment, release, and verification steps.

Windows plugin-managed startup uses the same Python launcher. Native Windows
needs a working `python3` command; if the Windows Store alias is present without
Python installed, install Python 3 and refresh/reinstall the plugin before
rechecking `codex mcp list`.

## Prompt And Agent Inventory

Active workflows use the embedded runtime prompt bundle. For prompt inventory
and hash behavior, read `ai-docs/mental-model/prompt-bundle.md`,
`agents-plugin/runtime.json`, and the embedded prompt sources.

## Skill Inventory

Codex-first skills live under `agents-plugin/skills/` and use `lead-*` names.
Use the source tree and plugin manifest/tests for the current inventory.

## Canonical Flows

```text
Full ceremony:  discuss -> proceed -> implement -> review/docs/final gate
Direct:         implement <description>
Auto-route:     proceed <ticket-path>
Sprint:         sprint -> discuss/explore -> sprint-edit episode? -> episode closure or normal handoff
Review:         review [branch] -> verdict -> (discuss -> fix | comment | merge)
Recovery:       salvage -> research report -> recovery epic? -> child tickets
```

User decides next step at each handoff. `proceed` is the explicit opt-in for
auto-chaining through the pipeline.

## Specs

| File | Title | Summary |
|------|-------|---------|
| `ai-docs/spec/plugin-runtime.md` | Plugin Runtime | Codex plugin packaging, runtime metadata, launcher repair, release assets, and runtime CLI |
| `ai-docs/spec/mcp-tools.md` | MCP Tools | Host-neutral ws MCP tool contracts for context, workflow state, Git, docs, and agents |
| `ai-docs/spec/named-agent-runtime.md` | Named Agent Runtime | Durable named-agent sessions, async lifecycle, subquery fan-out, diagnostics, and Codex backend behavior |
| `ai-docs/spec/workflow-skills.md` | Workflow Skills | Codex-facing lead skills, routing, sprint work, reconstruction, utilities, and workflow primitives |
| `ai-docs/spec/documentation-system.md` | Documentation System | Project memory, conventions, specs, tickets, mental models, reference tracing, and doc workflows |
| `ai-docs/spec/api-documentation-cache.md` | API Documentation Cache | Host-neutral API documentation lookup through cached domain docs and manager sessions |
| `ai-docs/spec/claude-compatibility.md` | Claude Compatibility | Root Claude shim, agents-plugin compatibility metadata, installer behavior, and retired legacy boundaries |
| `ai-docs/spec/developer-environment-tools.md` | Developer Environment Tools | Personal bootstrap, shell/terminal/editor config, tmux helpers, statusline, and Claude TUIs |
| `ai-docs/spec/ws-web-dashboard/index.md` | ws Web Dashboard | Personal ws-aware web dashboard daemon, browser UI, and host-control behavior |

## Tickets

Status directories: `ready/`, `todo/`, `idea/`, `.done/`, `.dropped/`.
Reference tickets by stem. This index lists active tickets only; completed or
dropped tickets live in hidden archive dirs and git history.

| Stem | Status | Summary |
|------|--------|---------|
| `260514-epic-ws-web-dashboard-mvp` | todo | Coordinate the personal ws-aware web dashboard MVP |
| `260525-feat-ws-dashboard-document-polishing-backlog` | todo | Track non-critical document viewer/editor polish after the MVP document substrate |
| `260525-feat-ws-dashboard-workroot-polishing-backlog` | todo | Track non-critical WorkRoot lifecycle and Git toolbar polish after the MVP management substrate |
| `260525-feat-ws-dashboard-server-scoped-operation-forwarding` | todo | Make root picker, workRoot, file, Activity, Git, and terminal operations transparent across linked servers |
| `260524-epic-async-exec-job-surface` | todo | Coordinate async exec job tools, bounded output readers, and later model-backed output questions |
| `260524-feat-exec-output-ask` | todo | Add lead-facing model-backed questions over persisted exec job output |
| `260524-chore-exec-surface-runtime-contract` | todo | Close runtime capabilities, manifests, CLI mirror policy, and wsflow contract for exec tools |
| `260513-feat-runtime-binary-staging-copy` | todo | Stage runtime binaries under deterministic versioned paths |
| `260524-bug-wsstore-ci-sqlite-busy` | todo | Capture CI SQLite busy failures when concurrent wsstore handles write one state database |
| `260525-bug-implement-review-fix-owner` | todo | Clarify lead-implement review fixes so the implementation owner applies findings |
| `260616-refactor-remove-agent-backed-api-tools` | done | Remove the agent-backed api.ask MCP tool family from the playbook pivot |
| `260616-epic-api-namespace-documentation-memory-tooling` | todo | Rebuild api.* later as pure documentation, corpus, hierarchical memory, and playbook-manual tooling |
| `260616-refactor-wsflow-product-mode-convergence` | done | Collapsed wsflow onto product-mode playbook rendering and removed curated skill bodies |
| `260616-refactor-explicit-namespace-render-vars` | done | Replaced implicit ws->wsflow playbook string substitution with explicit reserved namespace render vars |
| `260617-feat-fresh-reader-audit-playbook` | idea | Add a reusable fresh-reader audit playbook for skill and prompt authoring |
| `260617-refactor-mcp-stateless-subagent-context` | done | Make MCP subagent context stateless and filesystem-backed |
| `260517-bug-ws-dashboard-windows-terminal-control-keys` | todo | Investigate native-Windows cmd.exe terminal Ctrl-C/control-key behavior after fixed-endpoint dogfood reached the live PTY |
| `260517-bug-ws-agent-empty-result-after-tool-use` | todo | Investigate ws named-agent empty final result after long Claude backend tool-use runs |
| `260523-feat-ws-dashboard-main-session-activity-source` | idea | Represent direct main-session Codex work in WorkRoot Activity freshness |
| `260525-bug-ws-dashboard-agent-tab-close-confirmation-sticky` | idea | Investigate sticky agent tab close confirmation in browser acceptance |
| `260523-research-ws-dashboard-persistable-ui-state-map` | idea | Map persistable ws dashboard UI state |
| `260524-bug-project-tree-stale-ticket-status-map` | idea | Clarify stale ticket status projection in project_tree output |
| `260524-bug-ws-agent-register-stale-dir-result-hang` | idea | Investigate ws agent stale registration reset failure, register/call ordering, and post-test missing result |
| `260616-bug-launcher-runtime-install-forced-test-drift` | done | Restore launcher runtime_install_forced test contract |
| `260616-bug-exec-mcp-running-large-abort-flaky-under-full-suite` | idea | Investigate exec MCP running/abort timing flakiness under full agents-plugin-tool test suite load |
| `260512-research-claude-cli-stream-json` | idea | Capture Claude CLI stream-json contract before changing the Claude named-agent runner |
| `260513-research-dual-mcp-startup-order` | idea | Validate dual stdio doctor and HTTP MCP startup ordering |
| `260513-research-streamable-http-mcp-transport` | idea | Research Streamable HTTP transport and reconnect boundaries |
| `260514-research-ws-web-dashboard-direction` | idea | Research dashboard resource model, document UX, harness-library direction, and absorbed child backlog |
| `260523-bug-worktree-local-index-missing` | idea | Explore dashboard-managed propagation of ignored local workflow context across worktrees |
| `260523-bug-implement-merge-target-discovery` | idea | Investigate safer merge-target discovery for nested implement branches |
| `260523-bug-ws-mcp-launcher-runtime-repair-race` | idea | Investigate ws-mcp launcher runtime repair race behavior |
| `260523-chore-implement-branch-cleanup-guidance` | idea | Add post-merge branch cleanup guidance to implement workflows |
| `260524-bug-subquery-non-head-history-evidence` | idea | Prevent subquery ticket surveys from citing non-HEAD branch commits as current evidence without labeling the boundary |
| `260524-bug-subquery-working-directory-stderr` | idea | Investigate delegated subquery shell stderr from inaccessible process working directories |
| `260525-bug-ws-setup-cwd-plugin-cache-root` | idea | Clarify or fix ws setup cwd placeholder resolution in installed-plugin sessions |
| `260525-bug-lead-implement-delegation-pre-edit-guard` | idea | Require an explicit direct-edit verdict or implementer spawn before lead-implement mutates source |
| `260524-research-ws-dashboard-react-aria-ui-primitives` | idea | Research broader React Aria primitive adoption for dashboard UI |
| `260524-research-ws-dashboard-visual-design-system-refresh` | idea | Research a coherent visual design system refresh for ws dashboard surfaces |
| `260525-bug-codex-local-marketplace-worktree-cache-regression` | idea | Investigate Codex local marketplace cache regression across sibling worktrees |

## Ticket Focus

- `260605-epic-ws-playbook-factory-pivot` (todo, epic) - playbook-factory board;
  not implementation-ready (board artifact). **M0/M1/M2/M3 done.** M1
  `260609-feat-ws-playbook-surface-mvp` (`.done/`, merged `4bc4efd9`):
  `internal/wsrsrc` call-time loader + `playbook.print`/`playbook.render` MCP tools
  with harness-aware rendering; spec stems `260609-playbook-tools` /
  `-harness-rendering` / `-rsrc-playbook-distribution` implemented. M2
  `260609-refactor-ws-skill-text-playbook-conversion` (`.done/`, 2026-06-10, merged
  `b6850dc3`): all lead-* procedure text moved to `agents-plugin/rsrc/` playbooks —
  11 thin entry-skill shims + 8 internal procedures; subquery→Explore absorption;
  spec `260610-entry-skill-surface-reduction` and
  `260610-subquery-explore-delegation-shift` implemented. `ws/subquery`/`agents.*`
  runtime stays callable but unreferenced by shipped skill text (reshape/deletion
  is M3). **M3 done** (`260609-refactor-ws-spawn-runtime-deletion-session-auth`,
  `.done/`, merged `be8c39e6`) — spawn engine → scoped mercenary reshape + ephemeral per-call
  session-key auth (drops actor/wsstore/authority); promoted to ready with
  contract-first spec authored (`77a9322a`: `260610-ephemeral-session-auth-model`,
  `260610-mercenary-delegation-surface`). **M3 Phase 1 (additive session-auth)
  merged to the epic branch** (`c917c9f0` no-ff; Phase 1 result `447946f4`).
  **Phase 2a complete** (`9649a4bf`): actor model + `ws.setup` + setup-fence
  deleted, mandatory `session_key`, `root` stripped from all schemas, registry
  re-keyed off actorID. **Phase 2b complete** (`60015691`, branch
  `implement/ws-session-auth-phase2b`, stacked on unmerged 2a — both pending a
  combined merge): gemini runner impl + subquery runtime + retired-path
  diagnostics deleted (harness-neutral Runner interface kept as deferred plug), 3
  resolved-by-deletion bug tickets dropped. **Phase 2c complete** (`0c7c0f50`,
  branch `implement/ws-session-auth-phase2c`, stacked on unmerged 2a+2b — all
  three pending a combined merge): `agents.*` reshaped to the mercenary surface —
  render-minted child keys via keyed `playbook.render`, `ws.lead.prefer_mercenary`
  guidance flip + always-on tip, register schema narrowed (`prompts`/`tier`/`model`
  dropped), native-shaped `agentId=` handle; diagnostic minimization a deliberate
  no-op (spec retains debug.*); bugs `260517`+`260524` re-triaged as live (not
  dropped). **Phase 3 complete** (`ec2ad888`, stacked on 2a+2b+2c): exec fully
  stateless (`exec_jobs.owner_actor_id` dropped via generalized recreate-table
  migration); capability-scope enforcement folded into the keyed session-key gate
  (`WS_MCP_TOOL_PROFILE` retired — `Server.role`/`requestedToolRole`/env
  propagation removed; keyed `callTool` gate is sole authority); dashboard
  build-fix (test-fixture only, no feature change). **All phases merged to the
  epic** (`be8c39e6`, combined --no-ff); 260609 closed `.done/`. Open fill
  (delegate `role:`/`tier:` asset + per-spawn/per-role tier routing +
  reviewer-tier default) re-homed to
  `260611-refactor-ws-tier-taxonomy-delegate-tier-routing` (done, was promoted
  2026-06-11). **M4**
  (`260616-refactor-remove-agent-backed-api-tools`, done) — removed the
  agent-backed `api.ask` family and stale guidance from the playbook pivot while
  retaining `api.list` only as deterministic cache-domain discovery. The dropped
  corpus-routing predecessor is replaced by a
  future outside-epic board:
  `260616-epic-api-namespace-documentation-memory-tooling`, where `api.*` becomes
  pure documentation, corpus, hierarchical memory, and playbook-manual tooling
  with no MCP-owned agent delegation. **Post-M4 complete**
  (`260616-refactor-wsflow-product-mode-convergence`, `.done/`): wsflow is
  converged onto product-mode playbook rendering. Phase 1
  landed product-marker rendering, Phase 1.5
  `260616-refactor-explicit-namespace-render-vars` (`.done/`, `ae0c6959`) replaced
  broad playbook namespace rewriting with explicit reserved render vars. Phase 2
  (`6ca530ab`) absorbed legacy `prompt.render` context materialization into
  wsflow-mode `playbook.render`.
  Phase 3 (`87aec145`) collapsed shipped wsflow skill bodies to thin
  `wsflow/playbook.print` shims over shared rsrc playbooks. Phase 4
  (`6fec9107`) removed the wsflow-only `prompt.render` MCP/runtime surface and
  stale migration doctrine; wsflow delegate prompts now render through
  `playbook.render`. Dogfood follow-up
  `260617-refactor-mcp-stateless-subagent-context` (**done** `f757f70f`,
  `.done/`) replaced the in-memory session registry with a flat per-key
  filesystem store (`keys/<key>.json`), so native subagents that start fresh
  MCP instances resolve session keys from disk instead of shared process state.
  Open: Codex non-skill `rsrc/` cache materialization
  (prereqs `260523`, `260524-codex-cache`).
- `260611-refactor-ws-tier-taxonomy-delegate-tier-routing` (**done** `.done/`,
  refactor; ready→.done 2026-06-12) - first-class `small/medium/large/xlarge` tier
  vocab + `light/core/deep` alias demotion, mercenary per-spawn tier routing, full
  delegate-prompt convergence onto rsrc, `wsprompt` loader retirement, and the
  `agents.*`→`ws.mercenary.*` delegation-surface rename. All 7 phases complete
  (P1 `3019ade9`, P2 `54e53d70`, P3 `fc1cdc5f`+`45f32b80`, P4 `5a26b1d6`,
  P5 `5023562c`, P6 `6be3bb64`, P6b `6873b480`, P7 `d18883a0` — agents.*→
  ws.mercenary.* MCP/CLI/runtime/rsrc/spec rename, hard rename no alias). P4-7
  merged to the epic at `04452233`; P1-3 had already merged at `016c1425`.
  **Live follow-ups:** deferred config-surface slice
  (`config.agents_tier`→`config.model_alias`, `config.role_tier`), the
  `(skill,role)→tier` override surface, the `lead-verify-design` inline-reviewer
  model/tier path, and `ref/ws-agent-runtime.md` pre-M3 staleness cleanup.
- `260611-research-ws-per-role-delegation-tuning-config` (idea, research) - owns
  the tier-taxonomy model (two planes: first-class abstraction vs alias/concrete
  layer; native vs opt-in mercenary). First-class axis resolved 2026-06-11 =
  **capability level** (subscription/plan rejected as leaky); alias mapping
  `light↦small`/`core↦medium`/`deep↦large` locked; the actionable above is now
  unblocked. The session-key word-chain generator generalization to other id
  surfaces remains reserved as `260610-refactor-ws-wordchain-id-generalization`
  (todo, follow-up).
- `260619-epic-ws-layered-config-prompt-tuning` (todo, epic) - board artifact,
  not implementation-ready. New epic under `260605` pushing the playbook prompt
  engine toward user-tunable config: a 4-layer `session>project>global>builtin`
  config substrate (per-item default scope, file lock, `prefer_mercenary`
  migrated in → closes `260618`) + a block-marker prompt-override engine
  (`DelegationSection` seed) + a self-doc `config.prompt()` listing. **Substrate
  child shipped** (`260619-feat-ws-layered-config-scope-substrate`, done: Phase 1
  `acf1be70` resolver + scopes + file-lock RMW + `config.show` scope reporting;
  Phase 2 `c65326bd` `prefer_mercenary` migrated to a session-scope desired-state
  item, closes `260618`). Future `config.model_alias`/`config.role_tier` rename
  slice must adopt (not fork) the shared scope primitive.
  Dependent children: marker-engine **shipped** (done, both phases — `705795b8`
  engine + `b2929411` `DelegationSection` seed). `260619-feat-ws-config-prompt-tool-self-doc`
  (todo, dep both substrate + engine) is now **unblocked** and is the remaining
  child.
- `260619-feat-ws-config-prompt-tool-self-doc` (todo) - the remaining epic child:
  a `config.prompt.set(pointId, harness, prompt, scope?)` setter plus a no-arg
  `config.prompt()` self-doc that tree-scans override markers and lists the
  tunable points with their stored/seed values. Both deps shipped (layered-config
  substrate + marker engine with the live `DelegationSection` point keyed
  `prompt.DelegationSection.<harness>`), so it is ready to promote.
- `260619-refactor-stateless-implement-review-continuity` (ready) - makes the
  `lead-implement` review fix-loop correct under the default native-subagent path
  by treating delegates as stateless and moving continuity to the lead, anchored
  on commit `## AI Context`. Single interlocking Phase 1 across
  `implementer.md`/`reviewer.md`/`lead-implement.md` (D1-D6). Spec addressing:
  `## Spec Impact` on `workflow-skills.md` (`#260612-reviewer-allocation-tier-default`),
  contract-first: no (spec + playbooks land together in the slice). Implementation
  is Ask-first + `lead-skill-authoring` review (observable workflow behavior). Epic
  child of `260605`.
## Session Notes

Open: verify Codex hook feedback semantics on macOS/later CLI; durable leaf role
assignment remains deferred.
