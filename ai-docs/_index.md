<!-- Memory policy: prune aggressively as project advances. Completed
     work belongs in git history, not here. Keep only what an AI session
     needs to orient itself and pick up work. If it's derivable from
     code or git log, delete it from this file. -->

# devenv — Project Index

## What This Repo Is

Configuration and template repository for Claude Code workflows.
Meta-workflow project only — defines skills, agents, and workflow patterns for downstream projects. Sessions here work on the workflow system itself; domain specs, mental-models, and domain tickets belong to downstream projects.

**Plugin:** `ws@0.13.2` — see `claude-plugin/.claude-plugin/plugin.json`.

**Plugin topology:**
- Skills and agents are delivered via the `ws` Claude Code plugin, sourced from `claude-plugin/` via a `directory`-type marketplace entry in `~/.claude/settings.json`.
- After any change to `claude-plugin/`, run `claude plugin update ws@ws` to propagate to the plugin cache. `./install.sh update` handles first-time install and settings patching on a new machine.
- `claude-plugin/CLAUDE.home.md` is the canonical copy of `~/.claude/CLAUDE.md` — edits to the global thinking doctrine land in this repo; `git diff` surfaces them here.
- External install: `/plugin marketplace add kang-sw/devenv` → `/plugin install ws@ws`.

## Reference Documents

Read before authoring or modifying skills, agents, or infra:

| Document | Purpose |
|----------|---------|
| `ai-docs/ref/skill-authoring.md` | Skill & agent document layout, invariant/constraint checklist, doctrine format |
| `ai-docs/ref/codex-integration.md` | Probed codex CLI behavior: invocation, JSONL output format, session management, hook config |
| `ai-docs/ship/ws.md` | Ship config for the `ws` plugin: version strategy, changelog, tag, push |
| `claude-plugin/infra/impl-playbook.md` | Implementation discipline: test strategy, verify, deviation protocol. Access via `ws-print-infra impl-playbook.md`. |
| `claude-plugin/infra/subagent-rules.md` | Subagent dispatch rules: exploration, branches, general rules. Access via `ws-print-infra subagent-rules.md`. |
| `claude-plugin/infra/executor-wrapup.md` | Shared post-implementation wrapup: _index.md refresh, doc-commit gate, ticket update. Access via `ws-print-infra executor-wrapup.md`. |

## Native Agents

`claude-plugin/agents/` contains agent types used by Claude Code's native Agent tool.
Agent role documents (system prompts with frontmatter) live in `claude-plugin/infra/prompts/`.

```
claude-plugin/agents/
  clerk.md                — ticket management (used by forge-spec as ws:clerk subagent_type)

claude-plugin/infra/prompts/     — agent role docs (frontmatter: name, model, tools)
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

claude-plugin/bin/                   — PATH-accessible executables (added by plugin)
  ws-subquery                    — scoped sub-query via ws-oneshot-agent; delegates to -p subquery; haiku default, --deep-research for sonnet
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
```

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
| `ai-docs/spec/agent-system.md` | Agent System | Spawnable agent roles — output contracts, refusals, spawn contexts |
| `ai-docs/spec/personal-devenv.md` | Personal Dev Environment | install.sh, shell, dotfiles, Claude Code config |
| `ai-docs/spec/plugin-infra.md` | Plugin Infrastructure | ws plugin delivery, ws-call-named-agent primitives |
| `ai-docs/spec/plugin-management.md` | Plugin Management | Local .claude-plugin/skills/ tools for ws plugin maintenance |
| `ai-docs/spec/spec-system.md` | Spec System | Spec authoring, 🚧 markers, anchor protocol |
| `ai-docs/spec/tools.md` | Devenv Tools | Custom tools built in this repo (claude-watch TUI, claude-dash multiplexer) |
| `ai-docs/spec/workflow-skills.md` | Workflow Skills | /discuss, /write-*, /edit, /implement, /proceed, /ship |

## Tickets

Status directories: `idea/` → `todo/` → `done/` (or `dropped/`).
Reference by stem only (e.g., `260407-research-delegation-model-consolidation`).

| Stem | Status | Summary |
|------|--------|---------|
| `260419-chore-blueprint-plugin-extraction` | done | Package claude-plugin/ as a Claude Code plugin (now named "ws"); all phases complete and validated |
| `260420-feat-spec-driven-workflow` | done | Spec-driven workflow infrastructure; all phases done (phase 6 migration cancelled) |
| `260421-feat-global-spec-stems` | done | Global unique YYMMDD-slug stems; all phases done (phase 5 migration cancelled) |
| `260421-feat-forge-spec` | done | /forge-spec skill — from-scratch spec reconstruction; all 3 phases complete |
| `260421-feat-delegate-implement-feature-branch` | done | /implement feature-branch auto-merge mode; all phases complete — **reverted in f4b11f7** (approval gate now unconditional) |
| `260422-chore-write-ticket-workflow-drift` | done | Fix stale /write-spec suggestion in write-ticket + workflow-skills.md chain drift |
| `260422-chore-workflow-chain-drift` | done | Fix remaining chain drift in discuss/SKILL.md, write-spec/SKILL.md, write-skeleton/SKILL.md |
| `260422-feat-write-ticket-review` | done | Add mandatory document-reviewer step to write-ticket after intent review |
| `260422-feat-proceed-full-pipeline` | done | Extend /proceed to full-pipeline routing — add judge: needs-spec and auto-invoke judge: needs-ticket |
| `260422-chore-rename-implement-to-edit` | done | Rename /implement skill to /edit; phase 1 of two-phase skill rename |
| `260422-chore-rename-delegate-implement-to-implement` | done | Rename /delegate-implement skill to /implement; phase 2 of skill rename |
| `260423-feat-proceed-mandatory-ticket` | done | Tighten /proceed judge: needs-ticket — always invoke /write-ticket for inline descriptions |
| `260423-feat-doc-system-gap-fixes` | done | Documentation system gap fixes — feature removal protocol, spec diff signal, discuss staleness warning, cross-reference convention |
| `260423-feat-doc-tooling-restructure` | done | Doc tooling restructure — forge-mental-model new skill, write-mental-model removal, forge-spec palette flag, bootstrap legacy detection |
| `260424-feat-project-survey-agent` | done | project-survey Haiku agent + auto-invoke integration into edit/implement/parallel-implement/discuss |
| `260424-feat-domain-rules-layering` | done | Architecture Rules split + /add-rule skill; domain rules in mental-model docs |
| `260424-feat-polish-plugin-docs` | done | /polish-plugin-docs local skill + polish-writer agent + ws-call-named-agent context-fill hotfix |
| `260424-refactor-proceed-gate-suppression` | done | judge:idea-level demoted to reminder + /proceed gate-suppression context in prefix-stage invocations |
| `260424-feat-infra-path-portability` | done | ws-infra-path portability script; all bare claude-plugin/infra/ paths replaced with $(ws-infra-path) |
| `260424-feat-discuss-on-demand-survey` | done | /discuss on-demand survey via judge:needs-survey; project-survey enriched output (titles + summaries) |
| `260424-refactor-implement-file-based-review` | done | File-based review loop in /implement; reviewers write to ws-review-path files, implementer reads directly |
| `260425-feat-sprint-skill` | done | /sprint session-container skill — branch-as-state persistence, deferred doc pipeline, sprint-aware survey, 2-reviewer delegation |
| `260425-feat-ws-agent-registry-compression` | done | ws-call-named-agent redesign: named agent registry (ws-new-named-agent) + auto-compression at 100K tokens |
| `260425-chore-implementation-gap-staleness-flagging` | done | Reactive doc-staleness reporting — removed misplaced guideline from spec-conventions + spec-system; reactive one-liner added to impl-playbook, survey-writer, plan-writer, code-review-correctness |
| `260426-feat-claude-watch` | done | claude-watch Rust TUI — session history browser and live subprocess monitor for Claude CLI; all 4 phases complete |
| `260426-feat-claude-watch-mouse` | done | claude-watch mouse support — scroll wheel + left-click session selection + event-drain loop perf fix |
| `260426-perf-claude-watch-scroll-cache` | done | claude-watch scroll perf — cache total visual rows; Phase 2 (Arc clone) dropped (ratatui ownership constraint) |
| `260426-feat-claude-watch-features` | done | claude-watch sprint — token count display, headless/-p color distinction, worktree session discovery, vertical scrollbar, on-demand background parsing; ws-orchestration output persistence + background mode |
| `260426-feat-claude-dash` | done | claude-dash Rust TUI multiplexer — worktree tabs, interactive PTY terminal, named agent read-only panel, process lifecycle modal; all 4 phases complete |

## Ticket Queue

<!-- Implementation order for todo/ tickets. One line per ticket: `stem` — purpose and dependency notes. -->

## Session Notes

<!-- Cross-session intent only, 2-5 lines max, delete when stale. -->
