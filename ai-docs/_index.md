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

Active plugin package: `agents-plugin/` (`ws@0.37.5`).
Agentless derivative package: `agents-plugin-wsflow/` (`wsflow@0.37.5`).
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
  Codex UI install has verified `ws:lead-write-ticket` and `ws:lead-discuss`.
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
| `ai-docs/ref/skill-authoring.md` | On auditing skill/agent/prompt/convention content — authoring rules and invariant checklist |
| `ai-docs/ref/wsflow-mirroring.md` | Required before editing full ws skills, shared `agents-plugin/rsrc/` playbooks, or plugin surfaces that may need wsflow mirrors |
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
read `ai-docs/ref/skill-authoring.md`. Before editing full
`agents-plugin/skills/lead-*` skills, shared `agents-plugin/rsrc/` playbooks,
plugin packaging, runtime contracts, launcher behavior, prompt guidance, or
release validation that may affect wsflow, read
`ai-docs/ref/wsflow-mirroring.md` and run
`python3 -m unittest discover agents-plugin-wsflow/tests` when the derivative
surface may drift.

**Any canonical `agents-plugin/rsrc/` edit is incomplete until both regen
commands run**, in order, from `agents-plugin-tool/`:

```bash
WSRSRC_REGEN=1 go test ./internal/wsrsrc/... -count=1 -run TestGenerateRealManifest
WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowRsrcMirror
```

Both `-count=1` flags are mandatory (the regen entrypoints are env-gated test
bodies with no changing input, so the test cache returns a green `ok` without
writing). Never hand-edit `agents-plugin-wsflow/rsrc/` — it is a generated
byte-identical mirror. The python package test above verifies the skill bundle
and runtime contract; it does **not** catch rsrc mirror drift, so running only
that command leaves the mirror stale. Omitting the second regen is the process
gap recorded as `260625-bug-wsflow-rsrc-mirror-regen-missed-after-shipped-edit`.

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
| `ai-docs/spec/workflow-skills.md` | Workflow Skills | Codex-facing lead skills, routing, reconstruction, utilities, and workflow primitives |
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
| `260729-bug-dashboard-submodule-workroot-empty-projection` | idea | Dashboard `git_identity()` still rejects submodules after wsstate gained support, so submodule workRoots render as an empty projection |
| `260729-feat-workflow-manual-submodule-detection` | idea | Announce ws-aware submodules in `workflow_manual` output as separate roots and separate boards (detection, not federation) |

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
