<!-- Memory policy: prune aggressively as project advances. Completed
     work belongs in git history, not here. Keep only what an AI session
     needs to orient itself and pick up work. If it's derivable from
     code or git log, delete it from this file. -->

# devenv — Project Index

## What This Repo Is

Configuration and template repository for Claude Code workflows.
Meta-workflow project only — defines skills, agents, and workflow patterns for downstream projects. Sessions here work on the workflow system itself; domain specs, mental-models, and domain tickets belong to downstream projects. The skill system itself has a spec at `ai-docs/spec/skills.md`.

**Plugin topology:**
- Skills and agents are delivered via the `ws` Claude Code plugin, sourced from `claude/` via a `directory`-type marketplace entry in `~/.claude/settings.json`.
- After any change to `claude/`, run `claude plugin update ws@ws` to propagate to the plugin cache. `./install.sh update` handles first-time install and settings patching on a new machine.
- `claude/CLAUDE.md` is symlinked to `~/.claude/CLAUDE.md` — edits to the global thinking doctrine land in this repo; `git diff` surfaces them here.
- External install: `/plugin marketplace add kang-sw/devenv` → `/plugin install ws@ws`.

## Reference Documents

Read before authoring or modifying skills, agents, or infra:

| Document | Purpose |
|----------|---------|
| `ai-docs/ref/skill-authoring.md` | Skill & agent document layout, invariant/constraint checklist, doctrine format |
| `claude/infra/impl-playbook.md` | Implementation discipline: test strategy, verify, deviation protocol. Access via `load-infra impl-playbook.md` (agent context) or `${CLAUDE_PLUGIN_ROOT}/infra/impl-playbook.md` (skill context). |
| `claude/infra/subagent-rules.md` | Subagent dispatch rules: exploration, branches, general rules. Access via `load-infra subagent-rules.md`. |

## Native Agents

Agent definitions live in `claude/agents/`. Each file defines what an
agent IS and HOW it works (identity, constraints, process, output).
Team communication rules are injected by the calling skill at spawn time.

```
claude/agents/
  implementer.md          — code implementation from plan or brief
  parallel-implementer.md — scope-bounded implementer for parallel runs; never commits
  reviewer.md             — code review (read-only, produces findings)
  worker.md               — general-purpose non-code tasks
  clerk.md                — ticket management
  mental-model-updater.md — mental-model doc updates after code changes
  plan-populator.md       — enrich main-agent draft plan with codebase-grounded detail (warm mode)
  spec-updater.md         — strip completed 🚧 markers; flag spec/implementation drift
```

## Infra Layout

```
claude/infra/                 — docs only; accessed via load-infra or ${CLAUDE_PLUGIN_ROOT}/infra/
  impl-playbook.md            — subagent-safe implementation discipline
  mental-model-conventions.md — mental-model doc format and invariants
  ticket-conventions.md       — ticket format, status directories, stem convention
  subagent-rules.md           — exploration, branches, general rules

claude/bin/                   — PATH-accessible executables (added by plugin)
  ask                         — interactive user query helper
  merge-branch                — branch merge with strategy selection (squash or --no-ff)
  list-mental-model           — enumerate mental-model docs relevant to target paths
  load-infra             — cat any infra doc by name (agent Bash tool context)
```

## Skill Inventory

```
claude/skills/
  enter-session/       — session bootstrap; clerk-forked context synthesis + workflow map injection
  exit-session/        — end-of-session handoff; seal continuation payload for next /enter-session
  discuss/             — explore approach/direction, capture as tickets
  write-ticket/        — create/edit tickets in ai-docs/tickets/
  write-spec/          — create/update external-perspective spec docs in ai-docs/spec/
  write-skeleton/      — public interface stubs + integration tests
  write-plan/          — deep codebase research → implementation plan
  implement/           — main-agent-direct single-scope cycle (warm sessions, owner edits)
  delegate-implement/  — delegated implementer + reviewer cycle (cold sessions or wide scope)
  parallel-implement/  — multiple implementer pairs, disjoint file sets on shared branch
  proceed/             — auto-route through the canonical pipeline
  team-lead/           — team orchestration mode (TeamCreate, coordination, shutdown)
  manual-think/        — manual chain-of-thought when native thinking unavailable
  write-mental-model/  — mental-model document format, inclusion test, rebuild
  bootstrap/           — scaffold new project or upgrade existing to canonical template
```

## Canonical Flows

```
Full ceremony:  /discuss → /write-ticket → /write-skeleton → /implement (warm) or /delegate-implement (cold)
Direct:         /implement <description>
Parallel:       /parallel-implement (disjoint file sets on shared branch)
Auto-route:     /proceed <ticket-path>    — pipeline selection via warmth + scope judges
```

Agent suggests next step at each point; user decides. `/proceed` is the explicit opt-in for auto-chaining through the pipeline.

## Tickets

Status directories: `idea/` → `todo/` → `wip/` → `done/` (or `dropped/`).
Reference by stem only (e.g., `260407-research-delegation-model-consolidation`).

| Stem | Status | Summary |
|------|--------|---------|
| `260419-chore-blueprint-plugin-extraction` | done | Package claude/ as a Claude Code plugin (now named "ws"); all phases complete and validated |

## Session Notes

<!-- Cross-session intent only, 2-5 lines max, delete when stale. -->
