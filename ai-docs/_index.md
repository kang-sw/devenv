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

Active plugin package: `agents-plugin/` (`ws@0.33.1`).
Agentless derivative package: `agents-plugin-wsflow/` (`wsflow@0.33.1`).
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
| `ai-docs/ref/windows-dogfood.md` | Native-Windows source-build dogfood / Phase C cold-load acceptance procedure |
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
| `260627-feat-enter-proceed-deterministic-verdict-engine` | ready | Move deterministic `lead-proceed` route/verdict resolution into `ws.enter.proceed` while keeping the public MCP surface to one mode-switch call |
| `260620-feat-ws-dashboard-agent-client-activity-sources` | todo | Normalize Codex app-server and OpenCode ACP activity through a dashboard agent-client provider contract |
| `260525-feat-ws-dashboard-document-polishing-backlog` | todo | Track non-critical document viewer/editor polish after the MVP document substrate |
| `260525-feat-ws-dashboard-workroot-polishing-backlog` | todo | Track non-critical WorkRoot lifecycle and Git toolbar polish after the MVP management substrate |
| `260525-feat-ws-dashboard-server-scoped-operation-forwarding` | todo | Make root picker, workRoot, file, Activity, Git, and terminal operations transparent across linked servers |
| `260524-epic-async-exec-job-surface` | todo | Coordinate async exec job tools, bounded output readers, and later model-backed output questions |
| `260524-feat-exec-output-ask` | todo | Add lead-facing model-backed questions over persisted exec job output |
| `260524-chore-exec-surface-runtime-contract` | todo | Close runtime capabilities, manifests, CLI mirror policy, and wsflow contract for exec tools |
| `260513-feat-runtime-binary-staging-copy` | todo | Stage runtime binaries under deterministic versioned paths |
| `260524-bug-wsstore-ci-sqlite-busy` | todo | Capture CI SQLite busy failures when concurrent wsstore handles write one state database |
| `260525-bug-implement-review-fix-owner` | todo | Clarify lead-implement review fixes so the implementation owner applies findings |
| `260626-bug-sage-review-config-setter-missing` | idea | Add a lead-facing setter/tuning catalog knob for `sage_review` so review posture can be changed without manual config JSON edits |
| `260627-bug-write-ticket-bypasses-tickets-create` | idea | Capture dogfood failure where ticket authoring manually created a file instead of invoking `ws.tickets.create` |
| `260627-bug-enter-implement-direct-edit-policy-gap` | idea | Investigate `ws.enter.implement` lacking a direct-edit/no-delegation policy override for narrow multi-file text changes |
| `260627-bug-playbook-render-uses-stale-plugin-cache-during-source-dogfood` | idea | Investigate branch-local playbook render or cache-refresh guidance for source rsrc dogfood |
| `260627-research-lead-proceed-route-matrix-authoring` | idea | Research whether Route Facts and Route Matrix tables would make `lead-proceed` routing clearer without semantic drift |
| `260626-feat-session-key-format-and-retention` | todo | Change new session keys to three words plus two digits, refresh key-file mtime on keyed use, and prune stale key records about monthly with daily-bounded scans |
| `260626-bug-workflow-manual-bootstrap-sentinel-surface` | idea | Investigate the fresh workflow-manual sentinel guidance not matching the visible session-state tool surface during dogfooding |
| `260626-bug-prefer-subagent-recursive-delegate-escape` | idea | Investigate forked workers that complete work through a second delegate despite an explicit direct-edit handoff boundary |
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
| `260620-bug-mercenary-path-visible-when-prefer-off` | idea | Reduce mercenary dispatch guidance salience when prefer_mercenary is off |
| `260524-bug-project-tree-stale-ticket-status-map` | idea | Clarify stale ticket status projection in project_tree output |
| `260524-bug-ws-agent-register-stale-dir-result-hang` | idea | Investigate ws agent stale registration reset failure, register/call ordering, and post-test missing result |
| `260620-feat-ws-ticket-status-transition-tools` | done | New `tickets.close` and `tickets.move` MCP tools for atomic ticket status transitions (promotion/demotion/close/drop) with convention guards |
| `260622-feat-sage-review-ticket-gate` | done | Sage review gate: `create-ticket` MCP tool, two-reviewer playbooks, `lead-write-ticket` judge-gate integration, and `sage_review*` config substrate (all 3 phases done) |
| `260616-bug-launcher-runtime-install-forced-test-drift` | done | Restore launcher runtime_install_forced test contract |
| `260616-bug-exec-mcp-running-large-abort-flaky-under-full-suite` | done | Fixed exec finalize/reconcile race + too-tight abort window flaking the full-suite abort test (260620 Phase 2) |
| `260512-research-claude-cli-stream-json` | idea | Capture Claude CLI stream-json contract before changing the Claude named-agent runner |
| `260513-research-dual-mcp-startup-order` | idea | Validate dual stdio doctor and HTTP MCP startup ordering |
| `260513-research-streamable-http-mcp-transport` | idea | Research Streamable HTTP transport and reconnect boundaries |
| `260514-research-ws-web-dashboard-direction` | idea | Research dashboard resource model, document UX, harness-library direction, and absorbed child backlog |
| `260523-bug-worktree-local-index-missing` | idea | Explore dashboard-managed propagation of ignored local workflow context across worktrees |
| `260523-bug-implement-merge-target-discovery` | idea | Investigate safer merge-target discovery for nested implement branches |
| `260523-bug-ws-mcp-launcher-runtime-repair-race` | idea | Investigate ws-mcp launcher runtime repair race behavior |
| `260523-chore-implement-branch-cleanup-guidance` | idea | Add post-merge branch cleanup guidance to implement workflows |
| `260625-bug-wsflow-rsrc-mirror-regen-missed-after-shipped-edit` | idea | Capture wsflow rsrc mirror drift when canonical shipped rsrc edits are not mirrored |
| `260626-bug-wsflow-lead-revive-skill-inventory-drift` | idea | Capture wsflow lead-revive shipped skill inventory drift blocking the wsflow package test suite |
| `260524-bug-subquery-non-head-history-evidence` | idea | Prevent subquery ticket surveys from citing non-HEAD branch commits as current evidence without labeling the boundary |
| `260524-bug-subquery-working-directory-stderr` | idea | Investigate delegated subquery shell stderr from inaccessible process working directories |
| `260525-bug-ws-setup-cwd-plugin-cache-root` | idea | Clarify or fix ws setup cwd placeholder resolution in installed-plugin sessions |
| `260525-bug-lead-implement-delegation-pre-edit-guard` | idea | Require an explicit direct-edit verdict or implementer spawn before lead-implement mutates source |
| `260524-research-ws-dashboard-react-aria-ui-primitives` | idea | Research broader React Aria primitive adoption for dashboard UI |
| `260524-research-ws-dashboard-visual-design-system-refresh` | idea | Research a coherent visual design system refresh for ws dashboard surfaces |
| `260525-bug-codex-local-marketplace-worktree-cache-regression` | idea | Investigate Codex local marketplace cache regression across sibling worktrees |

## Ticket Focus

- `260702-bug-config-unset-asymmetry` (ready, bug) - redefine config `unset`
  as reset-to-builtin (not clear-to-empty) and add `session` scope to
  `config_prompt_unset`; spec addressing via `## Spec Impact`
  (`mcp-tools.md`, Contract-first: no). Sage review completed.
- `260702-bug-lead-manual-sections-thin` (ready, bug) - fill the empty
  `workflow_manual` `Session setup`/`User preferences` sections with the
  ferrule reuse-discipline; spec addressing via `## Spec Impact`
  (`mcp-tools.md`, Contract-first: no). Sage review completed.
- `260702-feat-agenda-enumerate-and-clear-all` (ready, feat) - add
  `agenda_list` and/or `agenda_clear(all: true)` to enumerate/clear agenda
  blob keys; spec addressing via `## Spec Impact` (`mcp-tools.md`,
  Contract-first: no). Sage review completed.
- `260702-feat-enter-implement-policy-feedback` (ready, feat) - `enter_implement`
  verdict notes when a caller policy field was outside its applicability
  window and ignored; spec addressing via `## Spec Impact` (`mcp-tools.md`,
  Contract-first: no). Sage review completed.
- `260702-feat-tickets-move-ready-gate-warning` (ready, feat) - `tickets_move`
  to `ready` emits a soft non-blocking warning when no spec addressing is
  detected; spec addressing via `## Spec Impact` (`mcp-tools.md`,
  Contract-first: no). Sage review completed.
- `260702-feat-lead-revive-session-key-candidates` (idea, feat) - **sage
  review blocked**: design premise assumed transient in-memory session-key
  storage, but storage is actually persistent per-key disk files with no
  eviction; needs re-authoring before promotion.
- `260702-feat-workflow-manual-state-only-view` (ready, feat) - add a
  lead-only session-state-only MCP tool (name TBD, e.g. `session_state`)
  returning only the Session State (todos/agenda) for the caller's key,
  reusing `workflow_manual`'s key-validation behavior; spec addressing via
  `## Spec Impact` (`mcp-tools.md`, Contract-first: no). Sage review
  completed (re-authored after initial completeness block: added Phase 1 +
  verification criteria, resolved lead-only-gating design concern).
- `260622-chore-windows-shipping-hardening` (ready, chore, child of 260605) -
  successor to the done 260620; makes the Windows surface shipping-correct with
  mercenary-on-Windows in v1 scope. Phase A static code hardening (`go test`-
  verifiable on Windows), B launcher cold-load robustness, C branch-pinned
  real-Windows acceptance. 260620 verified `go test` only, never the launcher
  cold-install path. **Epic merge to `main` is deferred until this passes.**
  Implementation-ready; spec addressing via `## Spec Impact` (Contract-first: no —
  Windows conformance to existing `named-agent-runtime` + `plugin-runtime`
  contracts). **Phases A+B done** (branch `implement/260622-windows-shipping-hardening`,
  unmerged). Phase A (`8461b4cf`): 7 Windows Go fixes, partitioned-review clean,
  cross-compile green. Phase B (`da1047fb`): canonical Python launcher cold-load
  hardening — best-effort rsrc-tree wait, OS-aware contract-read timeout +
  `(OSError, ValueError)` retry, bounded `os.replace` retry; 39 launcher tests
  green, review clean (1 correctness critical fixed). Canonical-launcher-only;
  wsflow divergence captured as `260622-bug-wsflow-launcher-coldload-divergence`.
  Empirical cold-install/cmd.exe/backslash/tree-kill assertions deferred to
  Phase C (real Windows host). Next target: **Phase C** (branch-pinned acceptance)
  — gates the epic merge to `main`.
- `260627-feat-enter-proceed-deterministic-verdict-engine` (ready, feat, child
  of 260605) - make `ws.enter.proceed` the deterministic route/verdict resolver
  at the routing-facts-complete boundary. The playbook keeps fact gathering and
  ambiguous judgments, while MCP owns normalized precedence, warnings, JSON
  result shape, canonical raw verdict text, `next_instruction`, agenda storage,
  and proceed todo replacement. Spec addressing via `## Spec Impact`
  (Contract-first: no — ticket pins the implementation slice; closeout updates
  `workflow-skills` and `mcp-tools`). Sage review completed. **Phases 1-3 done**
  on branch `implement/260627-enter-proceed-verdict-engine` (latest
  `cc930648`), unmerged: deterministic `ws.enter.proceed` resolver, canonical
  raw/JSON verdict output, concrete raw/JSON next-action directives with common
  follow rails, two-item proceed todo replacement, lead-proceed MCP handoff,
  docs, manifests, wsflow mirror, and partitioned review clean for Phase 1.
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
- `260619-epic-ws-layered-config-prompt-tuning` (**.done**, epic) - user-tunable
  playbook prompt config, all four children landed on the epic branch:
  layered `session>project>global>builtin` config substrate
  (`260619-feat-ws-layered-config-scope-substrate`: `acf1be70` resolver/scopes/
  file-lock + `c65326bd` `prefer_mercenary` migration, closed `260618`),
  block-marker override engine (`260619-feat-ws-prompt-override-marker-engine`,
  `DelegationSection` seed), data plane (`260619-feat-ws-config-prompt-tool-self-doc`:
  `24e7e0d1` `config.prompt.set` + `4e4460a1` `config.prompt()` listing, merged
  `461fef11`), and the `ws:lead-tune` umbrella tuning skill
  (`260619-feat-ws-lead-tune-skill`: `670e37dd`, merged `d3ca7a90`). Future
  `config.model_alias`/`config.role_tier` rename (260611 axis) must adopt — not
  fork — the shared scope primitive, and can slot into the `ws:lead-tune` umbrella.
- `260620-chore-pre-shipping-windows-surface-verification` (done, chore, in
  `.done/`; parent `260605` epic, gates its closure) - **All phases DONE: 1
  (`d89e6539`), 2 (`f6c4e7d1`), 3 (`326fa74f`), 4 (`994974af`); branch
  unmerged pending merge to the `260605` epic.** Pre-ship
  hardening of the Windows surface (six `*_windows.go` files were 0%-covered on
  Linux). All phases live on branch `implement/260620-win-surface` (unmerged; was
  `implement/win-cancel-process-tree`, renamed when Phase 2 stacked on). **Phase
  1:** both Windows cancel paths (`cancelAsyncProcessTree` + execjob
  `cancelProcess`) now reap the whole spawned subtree via Toolhelp32 PID-tree
  enumeration (was root-pid-only → orphans); deterministic cross-platform reap
  tests added. **Phase 2:** fixed the flaky exec abort (`260616`, now `.done/`) —
  a real `finalize()`/`reconcile()` race (active-map delete moved inside the `mu`
  section, after the terminal status write) plus a too-tight abort window;
  green under `-race`. **Phase 3:** first real Windows full-suite run
  (go1.26.3) — `go test ./...` now green 12/12. It found a real defect Phase 1
  could not (the subtree kill worked but Windows `processAlive` mis-reported an
  exited-but-unreaped process as alive); fixed across wsagent/execjob/wsstate
  with a zero-timeout `WaitForSingleObject` signaled-state check (also closes a
  Windows recovery-path defect). Three anticipated test-side divergences
  (abort timing, JSON-path matching, separator) also fixed; review clean. Single
  host / single toolchain. Contract unchanged (`260505-agent-cancel-recovery`
  best-effort + `cleanup_needed`; no new contract). **Phase 4:** dropped the
  Windows skip on the linked-worktree layout test (`internal/wsstate/paths_test.go`,
  `994974af`) so it runs on a Windows drive-letter root; test-only, no defect
  found (key derivation was already correct on Windows), review clean. **Hard
  constraint:** tree-kills scoped
  to the spawned subtree by PID/job — never image-name (`taskkill /IM`) —
  because the dogfooding WSL2 host runs a live `claude.exe`.
- `260703-chore-implement-branch-rename-default-allow` (ready, chore) -
  default `policy.branch.allow_rename` to `yes` in `enter.implement`'s
  branch plan resolver so the lead no longer needs explicit per-invocation
  user consent before a rename verdict is reachable; existing
  `TargetExists`/`Upstream`/`Ahead`/`Behind` guardrails are unchanged and
  remain the safety net. Spec addressing via Phase 1 (spec update bullet
  added per completeness-reviewer finding, Contract-first: no). Sage
  review completed.
- `260707-feat-forge-autonomy-bootstrap-chaining` (ready, feat) - narrow
  `lead-forge-spec`'s per-ambiguous-item classification loop to auto-proceed
  (inline `<!-- AMBIGUOUS: ... -->` markers, summarized in the final report),
  leaving the destructive archive gate and one-time domain-list confirmation
  untouched; add a `lead-forge-spec` wrap-up chaining prompt into
  `lead-forge-mental-model` (covers all entry paths, since bootstrap has no
  call/return path back from an indirectly-triggered forge-spec run).
  Complements `260707-feat-doc-coverage-live-bootstrap-alarm`'s cross-session
  safety net. Spec addressing via `## Spec Impact` (`workflow-skills.md`,
  Contract-first: no). Sage review completed.
- `260707-feat-doc-coverage-live-bootstrap-alarm` (ready, feat) - add a live
  (non-persisted, no set/clear flag) session-bootstrap check for whether
  `ai-docs/spec/`/`ai-docs/mental-model/` each contain at least one
  frontmatter-bearing `.md` file, surfaced via `ferrule`/`workflow_manual`
  and muted by a single new combined `wsconfig.Item*` entry; reuses
  `260703-chore-bootstrap-staleness-alarm`'s warning-delivery-channel
  pattern and rejects a generic `config.set_flag`-shaped setter for the same
  reason that ticket already rejected one. Spec addressing via
  `## Spec Impact` (likely shared with `260703`'s spec area, Contract-first:
  no). Sage review completed.
- `260707-research-drain-queue-default-branch-policy` (todo, research) -
  agenda to default the implementation branch policy to reuse+rename
  (instead of creating a fresh `implement/<slug>` branch and separately
  prompting for merge confirmation) when a ticket is driven end-to-end via
  `ws:lead-drain-ready-queue` with an explicit up-front goal. Not yet
  discussed or designed; sage-review left at `recommended` pending
  next-session discussion.
## Session Notes

Open: verify Codex hook feedback semantics on macOS/later CLI; durable leaf role
assignment remains deferred.

### Closeout: 260625 Phase 2 forge migration (260626)

Dogfooding the ws session-state machine. Lead session key
`thong-surfboard-container-easiness-26`. The unresolved forge audit from WIP
`14244ca6` was closed by `72503fd1`; commit-message heading normalization
followed in `41b2163e`, and `2a4aaba7` recorded Phase 2 completion. Dev-merge
`47aebbf9` integrated `implement/260625-forge-migration-audit-fix` into
`feature/ferrule`, and the merge path bumped `ws`/`wsflow` to `0.30.11`.

Phase 2 is fully complete on `feature/ferrule`: enter-call integration,
forge/delegate migration, audit fixes, wsrsrc manifest regeneration, wsflow
mirror regeneration, and the additive `lead-sprint` closeout are all recorded.
The ticket remains open until the larger branch is integrated to its final
target. `python3 -m unittest discover agents-plugin-wsflow/tests` still fails
only on pre-existing `lead-revive` inventory drift, captured as
`260626-bug-wsflow-lead-revive-skill-inventory-drift`.

Dogfood findings from this session remain under epic
`260605-epic-ws-playbook-factory-pivot`; the sage-review posture surface ticket
is closed, while follow-up tuning and session-key retention work remain active
in their respective idea/todo tickets.
