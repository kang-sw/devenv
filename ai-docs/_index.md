<!-- Memory policy: prune aggressively as project advances. Completed
     work belongs in git history, not here. Keep only what an AI session
     needs to orient itself and pick up work. If it's derivable from
     code or git log, delete it from this file. -->

# devenv — Project Index

## What This Repo Is

Configuration and template repository for Claude Code workflows.
Skills and agents are **authored** here, **consumed** in downstream
projects via symlink. Not a software project — no mental-model, no spec.

## Reference Documents

Read before authoring or modifying skills, agents, or infra:

| Document | Purpose |
|----------|---------|
| `ai-docs/ref/skill-authoring.md` | Skill & agent document layout, invariant/constraint checklist, doctrine format |
| `claude/infra/impl-playbook.md` | Implementation discipline: test strategy, verify, deviation protocol |
| `claude/infra/impl-process.md` | Implementation lifecycle: task list, code review, doc pipeline, merge |
| `claude/infra/agents/_subagent-rules.md` | Subagent dispatch rules: exploration, branches, general rules |

## Native Agents

Agent definitions live in `claude/agents/`. Each file defines what an
agent IS and HOW it works (identity, constraints, process, output).
Team communication rules are injected by the calling skill at spawn time.

```
claude/agents/
  implementer.md          — code implementation from plan or brief
  reviewer.md             — code review (read-only, produces findings)
  worker.md               — general-purpose non-code tasks
  clerk.md                — ticket management
  mental-model-updater.md — mental-model doc updates after code changes
```

## Infra Layout

```
claude/infra/
  impl-playbook.md   — subagent-safe implementation discipline
  impl-process.md    — top-level lifecycle (review, docs, merge)
  ask.sh             — interactive user query helper
  agents/            — subagent dispatch rules (caller-injected)
    _subagent-rules.md — exploration, branches, general rules
```

## Skill Inventory

```
claude/skills/
  discuss/           — explore approach/direction, capture as tickets
  write-ticket/      — create/edit tickets in ai-docs/tickets/
  write-skeleton/    — public interface stubs + integration tests
  write-plan/        — deep codebase research → implementation plan
  implement/         — delegation: implementer + reviewer cycle
  parallel-implement/ — multiple pairs, worktree isolation, coordinated merge
  team-lead/         — team orchestration mode (TeamCreate, coordination, shutdown)
  monologue/         — continuous operational narration
  manual-think/      — manual chain-of-thought when native thinking unavailable
  write-mental-model/  — mental-model document format, inclusion test, rebuild
  bootstrap/         — scaffold new project or upgrade existing to canonical template
```

## Canonical Flows

```
Full ceremony:  /discuss → /write-ticket → /write-skeleton → /implement
Direct:         /implement <description>
Parallel:       /parallel-implement (multiple implementer pairs, coordinated merge)
```

Agent suggests next step at each point; user decides. No auto-chaining.

## Tickets

Status directories: `idea/` → `todo/` → `wip/` → `done/` (or `dropped/`).
Reference by stem only (e.g., `260407-research-delegation-model-consolidation`).

## Session Notes

<!-- Cross-session intent only, 2-5 lines max, delete when stale. -->
<!-- no active session notes -->
