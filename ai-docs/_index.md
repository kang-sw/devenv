<!-- Memory policy: prune aggressively as project advances. Completed
     work belongs in git history, not here. Keep only what an AI session
     needs to orient itself and pick up work. If it's derivable from
     code or git log, delete it from this file. -->

# devenv — Project Index

## What This Repo Is

Configuration and template repository for Claude Code workflows.
Meta-workflow project only — defines skills, agents, and workflow patterns for downstream projects. Sessions here work on the workflow system itself; domain specs, mental-models, and domain tickets belong to downstream projects.

**Plugin:** `ws@0.15.0` — see `claude-plugin/.claude-plugin/plugin.json`.

**Plugin topology:**
- Skills and agents are delivered via the `ws` Claude Code plugin, sourced from `claude-plugin/` via a `directory`-type marketplace entry in `~/.claude/settings.json`.
- `agents-plugin/` is the Codex-first host-neutral plugin candidate for `ws@0.1.0`; it currently contains Codex and Claude manifests plus the initial `skill-authoring` skill.
- Codex verification for `agents-plugin/`: registered through `.agents/plugins/marketplace.json`; user installed `ws` in Codex UI and verified `$ws:skill-authoring`, `$ws:write-ticket`, and `$ws:discuss`.
- Codex local plugin iteration: no supported CLI install/uninstall/updater command was found; use UI uninstall/install or a fresh Codex session after editing the registered local source.
- Codex plugin-managed MCP shape: `.codex-plugin/plugin.json` can reference plugin-local `.mcp.json` via `"mcpServers": "./.mcp.json"`; verifying changed MCP config requires a user-performed Codex UI cache refresh for this repo-local plugin.
- Claude verification for `agents-plugin/`: `claude plugin validate agents-plugin` passes; runtime invocation `/ws:skill-authoring` remains a manual closeout item.
- After any change to `claude-plugin/`, run `claude plugin update ws@ws` to propagate to the plugin cache. `./install.sh update` handles first-time install and settings patching on a new machine.
- `claude-plugin/CLAUDE.home.md` is the canonical copy of `~/.claude/CLAUDE.md` — edits to the global thinking doctrine land in this repo; `git diff` surfaces them here.
- External install: `/plugin marketplace add kang-sw/devenv` → `/plugin install ws@ws`.

## Reference Documents

Read before authoring or modifying skills, agents, or infra:

| Document | Purpose |
|----------|---------|
| `ai-docs/ref/skill-authoring.md` | Skill & agent document layout, invariant/constraint checklist, doctrine format |
| `ai-docs/ref/codex-integration.md` | Probed codex CLI behavior: invocation, JSONL output format, session management, hook config |
| `ai-docs/ref/ws-mcp.md` | Host-neutral `ws-mcp` process model, MCP tool contracts, CLI fallbacks, and distribution boundary |
| `ai-docs/ref/ws-agent-runtime.md` | Host-neutral durable agent session contract: state layout, registry schema, queues, tiers, and planned MCP tools |
| `ai-docs/ship/ws.md` | Ship config for the `ws` plugin: version strategy, changelog, tag, push |
| `claude-plugin/infra/impl-playbook.md` | Implementation discipline: test strategy, verify, deviation protocol. Access via `ws-print-infra impl-playbook.md`. |
| `claude-plugin/infra/subagent-rules.md` | Subagent dispatch rules: exploration, branches, general rules. Access via `ws-print-infra subagent-rules.md`. |
| `claude-plugin/infra/executor-wrapup.md` | Shared post-implementation wrapup: _index.md refresh, doc-commit gate, ticket update. Access via `ws-print-infra executor-wrapup.md`. |
| `agents-plugin-tool/` | Go MCP/tooling source for explicit host calls to ws helpers without relying on plugin PATH injection |

## Agents

Agent role documents (system prompts with frontmatter) live in `claude-plugin/infra/prompts/`.
All agents are invoked via `ws-oneshot-agent -p <stem>` or `ws-named-agent new -p <stem>`.

```
claude-plugin/infra/prompts/     — agent role docs (frontmatter: name, model, tools)
  clerk.md                — ticket management (one-shot; invoked via ws-oneshot-agent -p clerk)
  code-reviewer.md        — code diff review: correctness, security, contracts (read-only)
  mental-model-updater.md — mental-model doc updates after code changes
  spec-updater.md         — strip 🚧 markers; flag entries for removal on `removed: <stem>`
  project-survey.md       — pre-invocation context survey; [Must|Maybe]-tiered reference list
  sprint-survey.md        — sprint-context survey for wrap-up doc assessment
  implementer.md          — code implementer role
  searcher.md             — codebase search assistant
  skeleton-writer.md      — skeleton stub and integration test authoring
  plan-populator-research.md — step-by-step plan drafter
  plan-populator-survey.md   — codebase survey for brief support
  subquery.md             — scoped sub-query worker prompt; haiku default; Constraints/Process/Output/Doctrine layout
  api-doc-manager.md      — persistent per-domain API doc executor; check-stale, fetch, answer
  pre-router.md           — oneshot Haiku pre-router; resolves domain list from free-text prompt
```

## Infra Layout

```
claude-plugin/infra/                 — docs only; accessed via ws-print-infra or -p flag
  impl-playbook.md            — subagent-safe implementation discipline
  mental-model-conventions.md — mental-model doc format and invariants
  ticket-conventions.md       — ticket format, status directories, stem convention; optional spec: and spec-remove: fields
  spec-conventions.md         — spec doc format, 🚧 marker rules, {#slug} anchor protocol
  subagent-rules.md           — exploration, branches, general rules
  code-review-correctness.md  — Correctness review partition: logic, error paths, contracts, security
  code-review-fit.md          — Fit review partition: conventions, naming, reuse, patterns
  code-review-test.md         — Test review partition: assertion validity, coverage, mock integrity
  executor-wrapup.md          — Shared executor wrapup: _index.md refresh, doc-commit gate, ticket update
  agent-compression.md        — compression handoff prompt injected into agents approaching the 120K token threshold
  workflow-for-agent.md       — doc-layer orientation for sub-agents + safe primitive subset; auto-injected by ws-named-agent new
  cargo-brief.md              — Rust API hint; injected via --prompt-cond cargo-brief when cargo-brief binary is in PATH

claude-plugin/bin/                   — PATH-accessible executables (added by plugin)
  ws-subquery                    — scoped sub-query via ws-oneshot-agent; delegates to -p subquery; haiku default, --deep-research for sonnet; Explore-level tool access; accepts inline string, `-` sentinel, or piped stdin
  ws-spec-build-index            — rebuild features: frontmatter in spec docs; removes stale stems: blocks
  ws-generate-spec-stem          — emit a new {#YYMMDD-slug} anchor for a given descriptive slug
  ws-list-spec-stems             — list {#YYMMDD-slug} anchors from spec files; file-arg adds heading context
  ws-merge-branch                — branch merge with strategy selection (squash or --no-ff)
  ws-list-mental-model           — enumerate mental-model docs relevant to target paths
  ws-print-infra                  — cat any infra doc by name or bare stem (agent Bash tool context)
  ws-infra-path               — return absolute path to an infra doc (for external path contexts; use -p <stem> with ws-new-named-agent)
  ws-proj-tree                — render ai-docs/ tree + spec/ticket summary for /discuss project map
  ws-review-path                 — allocate temp file paths for review outputs (multi-stem, non-deterministic)
  ws-oneshot-agent                  — register + call + erase a named agent in one invocation; -p required; full tool access; doc-system injected by default
  ws-named-agent                    — unified Python entry point for named agent management (subcommands: new, call, erase, interrupt, print, check-mailbox, tail, override)
  ws-new-named-agent                — shim → ws-named-agent new; create named agent registry entry (.git/ws@<repo>/agents/<name>.json); supports -p flag for multi-prompt composition
  ws-call-named-agent               — shim → ws-named-agent call; auto-routes session, compresses at 120K tokens (claude backend only), persists output, drains outbox
  ws-interrupt-named-agent          — shim → ws-named-agent interrupt; queue a message to a named agent's outbox
  ws-agent-check-mailbox            — shim → ws-named-agent check-mailbox; PostToolBatch hook: exits 2 when WS_AGENT_OUTBOX is non-empty
  ws-print-named-agent-output       — shim → ws-named-agent print; print the persisted output file of a named agent
  ws-ask-api                        — 2-layer API doc query: pre-router resolves domains, per-domain executor answers
  ws-ask-api-internal               — per-domain executor; acquires flock, dispatches to api-doc-<domain> named agent
```

Go MCP/tooling baseline:

```
agents-plugin-tool/
  cmd/ws-mcp/       — `ws-mcp` command; stdio MCP server plus version/doctor commands
  internal/mcp/     — minimal JSON-RPC/MCP stdio loop
  internal/wsagent/ — host-neutral agent registry/session prototype used by `ws-mcp agents`
  internal/wsdoc/   — project document helper logic used by MCP tools
  internal/wsprompt/ — embedded prompt bundle and resolver for agent prompt chains
  internal/wsstate/ — cache-root project/worktree path manager for workflow state
```

Current MCP contract: `ai-docs/ref/ws-mcp.md`. Implemented tools are
`ws/project_tree`, `ws/infra.read`, `ws/convention.read`,
`ws/spec_stem.generate`, `ws/spec_index.verify`, and
`ws/mental_models.list`, plus runtime helper `ws/runtime.info`, scoped delegate
helper `ws/subquery`, generated path helper `ws/path.generate`, and local
read-only Git helpers `ws/git.status`, `ws/git.diff`, `ws/git.log`, and
`ws/git.merge_base`. Shared
`agents-plugin` skills assume MCP availability and should not reference
repo-local `claude-plugin/infra/*` paths; convention text is bundled into the
runtime and read through `ws/convention.read`.

Agent runtime prototype: `ai-docs/ref/ws-agent-runtime.md`. `ws-mcp` exposes
minimum MCP tools `ws/agents.register`, `ws/agents.call`,
`ws/agents.wait`, `ws/agents.status`, `ws/agents.tail`, `ws/agents.cancel`,
`ws/agents.print`, and `ws/agents.erase`, plus matching CLI fallback subcommands under
`ws-mcp agents`. Both surfaces are backed by `internal/wsagent`. The Codex
backend stores thread ids in `agent.json`, resumes with
`codex exec resume --json <thread-id>`, and persists plain-text output in
`output.md`. Agent registration now accepts runtime-resolved prompt chains
through `prompts` with `prompt_refs` retained as a migration alias. The first
embedded prompt bundle contains `code-reviewer`, `code-review-correctness`,
`code-review-fit`, and `skeleton-writer`; `runtime.json` records the expected
bundle hash for launcher drift detection. `ws/agents.call` writes a
current prompt snapshot, starts a cache-local `agents run-current` worker, and
captures backend streams under
`current/` while the lead regains control. The wait/status/tail/cancel tools
inspect or mark that current-call state without invoking another backend turn.
Shared skill text should use `ws/agents.*` notation rather than the CLI command
names. `runtime.json` records both MCP tool and CLI command surfaces so the
plugin launcher can repair stale local cache binaries.

Codex launcher POC status: `agents-plugin` now contains plugin-local `.mcp.json`,
`runtime.json`, and `bin/ws-mcp-launcher`. Host-free launcher smoke, temporary
global Codex MCP registration, and installed plugin-managed Codex MCP startup
work on macOS when `.mcp.json` sets `cwd: "."`; without cwd, Codex registers the
server but relative command startup fails. The Unix launcher direction is POSIX
`sh` with internal OS/arch selection; Windows remains blocked on verifying
whether `./bin/ws-mcp-launcher` resolves to `./bin/ws-mcp-launcher.exe`.
Because that cwd is the plugin cache, the launcher derives `WS_MCP_PROJECT_ROOT`
from the parent Codex process `PWD`; `ws-mcp` uses it as the default root when
tools omit `root` or pass `"."`.

`ws-mcp` release distribution: `agents-plugin-tool/scripts/build-release-assets.sh`
cross-compiles `ws-mcp-<os>-<arch>[.exe]` assets plus `SHA256SUMS`. GitHub
Actions workflow `.github/workflows/ws-mcp-release.yml` runs tests, builds
assets, and uploads workflow artifacts on branch/PR validation; it publishes
release assets only for pushed `v*` tags. Runtime binaries remain plugin
cache-local under `.runtime/<os>-<arch>/ws-mcp[.exe]`. The POSIX launcher can
derive the GitHub release URL from `runtime.json`, download the selected asset
and `SHA256SUMS`, verify the matching checksum, and repair missing or
incompatible cache-local binaries. It also checks `tools/list` against
`runtime.json.tools` before exec so dev cache binaries with matching versions but
missing tools are repaired. For this machine's local Codex marketplace install
only (`~/.codex/plugins/cache/kang-sw-devenv/ws/`), the POSIX launcher can copy a
dev binary from `~/devenv/agents-plugin-tool/dist/` or
`~/devenv/agents-plugin/.runtime/`, or build `~/devenv/agents-plugin-tool`
directly when local candidates are stale, but only when the installed cache
contains the gitignored `.local-devenv-runtime` marker. Windows plugin-managed
startup still needs a native launcher or adapter-specific manifest verification.

`ws-mcp` development verification levels are documented in
`ai-docs/ref/ws-mcp.md`: Level 1 Go/MCP tests, Level 2 local release asset and
checksum build, Level 3 Codex plugin-managed MCP smoke after user-refreshed plugin
cache.

## Skill Inventory

```
claude-plugin/skills/
  discuss/             — explore approach/direction, capture as tickets
  write-ticket/        — create/edit tickets in ai-docs/tickets/
  update-spec/         — lead-driven spec audit: scan commits for spec-impact, add entries, strip 🚧, handle removals
  write-spec/          — create/update external-perspective spec docs in ai-docs/spec/
  write-skeleton/      — public interface stubs + integration tests
  add-rule/            — classify and route a new rule to CLAUDE.md (cross-cutting) or mental-model Domain Rules (domain-scoped)
  write-code/          — brief → plan depth → implementer → reviewer loop; core delegated implementation primitive
  edit/                — direct-edit primitive: lead edits, one named-agent reviewer (correctness+fit), no doc pipeline
  implement/           — harness: routes to write-code or edit; runs doc pipeline + approval + merge
  proceed/             — auto-route through the canonical pipeline; all implementation paths call /implement
  sprint/              — multi-task session container; branch-as-state persistence, deferred doc pipeline; calls write-code and edit
  ship/                — release: version bump, tag, build, publish per project config
  manual-think/        — manual chain-of-thought when native thinking unavailable
  bootstrap/           — scaffold new project or upgrade existing to canonical template
  forge-spec/          — from-scratch spec reconstruction; archive-first, domain-by-domain, cross-compact via TaskCreate (disable-model-invocation)
  forge-mental-model/  — from-scratch mental-model construction; survey → user confirm → per-domain verify cycle (disable-model-invocation)
  workflow/            — loads orchestration primitives reference; session-resident across compaction; invoked at discuss/sprint entry
  exit-session/        — session handoff: commit staged work, write context note to _index.md ## Session Notes, commit after user approval
```

`agents-plugin/` Codex-first candidate skills:

```
agents-plugin/skills/
  skill-authoring/ — author or audit ws skills and agent prompts; porting pivot
  write-ticket/    — draft host-neutral ticket creation/update workflow; helper/MCP execution deferred
  discuss/         — draft host-neutral design discussion workflow; survey/tooling execution deferred
  write-spec/      — draft host-neutral spec creation/update workflow; convention/stem/index helpers called through MCP
  update-spec/     — draft host-neutral commit-range spec audit workflow; convention/stem/index helpers called through MCP
  add-rule/        — draft host-neutral persistent rule capture workflow; convention and mental-model catalog via MCP
  ship/            — draft host-neutral release workflow driven by ai-docs/ship config
  exit-session/    — draft host-neutral session handoff workflow for ai-docs/_index.md
  workflow/        — host-neutral session-resident reference for MCP notation and orchestration primitive boundaries
  write-skeleton/  — draft host-neutral skeleton workflow using ws/agents.* delegate sessions
  edit/            — draft host-neutral direct-edit workflow using generated review paths and one async reviewer
  write-code/      — draft host-neutral delegated implementation workflow using brief/plan, async implementer, partitioned reviewers, and bounded relay
  forge-spec/      — draft host-neutral from-scratch spec reconstruction using ws/subquery surveys and user-confirmed domain gates
  forge-mental-model/ — draft host-neutral mental-model reconstruction using ws/subquery survey/verify loops and visible domain tasks
```

## Canonical Flows

```
Full ceremony:  /discuss → /proceed
                                         ↓
                     /write-skeleton? → /implement
                                             ↓
                                    (write-code | edit — routed internally)
Direct:         /implement <description>   — judge: execution-mode routes internally
Auto-route:     /proceed <ticket-path>     — all implementation paths route to /implement
Sprint:         /sprint → write-code | edit per task → wrap-up
```

Agent suggests next step at each point; user decides. `/proceed` is the explicit opt-in for auto-chaining through the pipeline.

## Specs

| File | Title | Summary |
|------|-------|---------|
| `ai-docs/spec/api-deps.md` | API Dependency Docs | Filesystem-based external API doc cache with 2-layer agent routing, consumed via ws-ask-api |
| `ai-docs/spec/agent-system.md` | Agent System | Spawnable agent roles — output contracts, refusals, spawn contexts |
| `ai-docs/spec/personal-devenv.md` | Personal Dev Environment | install.sh, shell, dotfiles, Claude Code config |
| `ai-docs/spec/plugin-infra.md` | Plugin Infrastructure | ws plugin delivery, ws-call-named-agent primitives |
| `ai-docs/spec/plugin-management.md` | Plugin Management | Local .claude-plugin/skills/ tools for ws plugin maintenance |
| `ai-docs/spec/spec-system.md` | Spec System | Spec authoring, 🚧 markers, anchor protocol |
| `ai-docs/spec/tools.md` | Devenv Tools | Custom tools built in this repo (claude-watch TUI, claude-dash multiplexer) |
| `ai-docs/spec/workflow-skills.md` | Workflow Skills | /discuss, /write-*, /edit, /implement, /proceed, /ship, /exit-session |

## Tickets

Status directories: `idea/` → `todo` → `wip` → `.done/` (or `.dropped/`).
Reference by stem only (e.g., `260407-research-delegation-model-consolidation`).
This index lists active tickets only; completed tickets live under
`ai-docs/tickets/.done/` and are discoverable by filename or git history.

| Stem | Status | Summary |
|------|--------|---------|
| `260503-epic-agents-plugin-skill-porting` | todo | Roadmap for porting `claude-plugin/skills/` into `agents-plugin/`: front-of-pipeline first, runtime/MCP boundary before core orchestration, bootstrap last |
| `260503-epic-ws-agent-workflow-stability` | todo | Live stabilization epic for named-agent workflow; completed phases split into child tickets, active child is worktree orchestrator lock |
| `260503-feat-ws-mcp-worktree-orchestrator-lock` | todo | Worktree-local MCP orchestrator lock; delegate containment, prompt orientation, current-call locking, and initial interrupt surface implemented; leaf/profile and hook-driven interrupt follow-ups remain |
| `260504-feat-ws-mcp-hook-driven-interrupt` | todo | Rework Codex interrupt delivery to use hook-injected inbox messages instead of signal-based subprocess interruption |
| `260503-epic-ws-mcp-vcs-reference-tools` | todo | Roadmap for portable `ws/git.*` MCP tooling plus ticket/spec/stem reference lookup |
| `260503-feat-agents-plugin-runtime-boundary` | wip | Go-based stdio MCP baseline and runtime boundary for replacing implicit ws helper PATH dependency; Phases 1-2 complete |
| `260429-feat-api-deps` | todo | ws-ask-api 2-layer API doc cache; phases: api-doc-manager prompt, pre-router prompt, bin tools, workflow integration |
| `260427-chore-claude-dash-windows` | todo | Verify native Windows build/runtime behavior for claude-dash |
| `260429-research-host-neutral-ws-plugin` | idea | Host-neutral ws plugin architecture research anchor |
| `260501-research-agents-bootstrap-root-context` | idea | Agents bootstrap root context research |

## Ticket Queue

<!-- Implementation order for todo/ tickets. One line per ticket: `stem` — purpose and dependency notes. -->
`260503-epic-ws-agent-workflow-stability` — active runtime quality gate for `write-code`; completed slices split into child tickets, current blocker is plugin-managed containment smoke
`260503-feat-ws-mcp-worktree-orchestrator-lock` — Phase 6 child: delegate containment, host-neutral orientation, current-call locking, and initial interrupt surface implemented; remaining follow-up is durable leaf-profile assignment
`260504-feat-ws-mcp-hook-driven-interrupt` — follow-up runtime slice: make Codex `agents.interrupt` hook-driven and keep signal/kill behavior under cancel semantics
`260503-epic-agents-plugin-skill-porting` — active roadmap for staged `agents-plugin` skill porting; next child sequence is core implementation orchestration after resolving remaining runtime gaps
`260503-epic-ws-mcp-vcs-reference-tools` — portable MCP roadmap for `ws/git.*` and ticket/spec/stem reference graph tooling; supports later replacement of direct shell wording in shared skills
`260503-feat-agents-plugin-runtime-boundary` — wip; macOS/Codex runtime launcher and release download path are verified; Windows plugin-managed launcher verification is deferred
`260429-feat-api-deps` — ws-ask-api 2-layer API doc cache; phases: api-doc-manager prompt → pre-router prompt → bin tools → workflow integration
`260427-chore-claude-dash-windows` — verify native Windows build/runtime behavior for claude-dash

## Session Notes

**Branch:** `topic/open-conventions-mcp-skills` — Codex-first `ws` plugin and
MCP runtime migration. Defer all spec and mental-model updates until this branch
merges; do not add `spec:` frontmatter, run `ws:update-spec`, or edit
`ai-docs/spec/` / `ai-docs/mental-model/`.

**Accomplished:** `6203e71` added worktree-local MCP lead/delegate authority,
`5596eec` fixed launcher tool-surface probing, and `6f022ed` added
`delegate-orientation` to public `agents.register`. Plugin-managed smoke showed
delegated Codex agents cannot see `agents.*`/`config.*`; `subquery` remains
available at delegate level and intentionally keeps its scoped prompt. The
current WSL2/Linux session added same-agent `agents.call` setup serialization
and durable `agents.interrupt` delivery with inbox-backed resume.

**In-flight:** `260504-feat-ws-mcp-hook-driven-interrupt` captures the next
runtime correction. `agents.call` now uses `current/setup.lock`;
`agents.interrupt` queues `inbox/<id>.json`. Follow-up testing on Codex CLI
0.128.0 / WSL2 showed inline `PostToolUse` hooks do fire, but `exit 2` injects
hook feedback into the next model step instead of stopping the subprocess. The
next slice should make hook-injected inbox delivery the primary interrupt path
and keep signal/kill behavior under `agents.cancel`.

**Next actions:** Implement `260504-feat-ws-mcp-hook-driven-interrupt`, then
refresh the local Codex plugin cache before plugin-managed verification of the
new `agents.interrupt` surface. Remaining runtime gap: durable leaf-level role
assignment beyond environment propagation.

**Key artifacts:** `agents-plugin-tool/internal/wsagent/agent.go` — current-call
and one-shot/subquery flow; `agents-plugin-tool/internal/mcp/server.go` — role
filtering; `agents-plugin-tool/internal/wsprompt/infra/delegate-orientation.md`
— prompt-level delegate boundary; `ai-docs/ref/ws-agent-runtime.md` — runtime
contract.

**Open questions:** Whether leaf-level tool restriction needs durable role
assignment beyond env propagation; whether Codex hook feedback semantics differ
on macOS or a later CLI.
