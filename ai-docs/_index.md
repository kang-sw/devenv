# devenv — Project Index

## What This Repo Is

Configuration and template repository for Claude Code workflows.
Skills and agents are **authored** here, **consumed** in downstream
projects via symlink. Not a software project — no mental-model, no spec.

## Reference Documents

Read before authoring or modifying skills, agents, or infra:

| Document | Purpose |
|----------|---------|
| `ai-docs/ref/skill-authoring.md` | Skill document layout, invariant checklist, doctrine format |
| `claude/infra/impl-playbook.md` | Implementation discipline: test strategy, verify, deviation protocol |
| `claude/infra/impl-process.md` | Implementation lifecycle: task list, code review, doc pipeline, merge |
| `claude/infra/agents/_common.md` | Shared subagent rules: communication, branches, exploration |

## Infra Layout

```
claude/infra/
  impl-playbook.md   — subagent-safe implementation discipline
  impl-process.md    — top-level lifecycle (review, docs, merge)
  ask.sh             — interactive user query helper
  agents/            — shared subagent role files
    _common.md       — team communication + shared rules
    implementer.md   — code implementation
    reviewer.md      — code review (read-only)
    planner.md       — codebase research → plan
    worker.md        — non-code tasks
    clerk.md         — ticket management
```

## Skill Inventory

```
claude/skills/
  discuss/           — explore approach/direction, capture as tickets
  write-ticket/      — create/edit tickets in ai-docs/tickets/
  write-skeleton/    — public interface stubs + integration tests
  write-plan/        — deep codebase research → implementation plan
  implement/         — structured implementation (plan-driven or ad-hoc)
  delegated-implement/ — Tier 3 delegation: implementer + reviewer cycle
  write-spec/        — external-facing feature specs
  monologue/         — continuous operational narration
  chat-over-session/ — multi-agent chat across sessions
  manual-think/      — manual chain-of-thought when native thinking unavailable
  rebuild-mental-model/ — regenerate ai-docs/mental-model/
```

## Canonical Flows

```
Full ceremony:  /discuss → /write-ticket → /write-skeleton → (/write-plan) → /implement
Direct:         /implement <description>
Delegation:     /delegated-implement (implementer + reviewer cycle)
```

Agent suggests next step at each point; user decides. No auto-chaining.
Parallel work = dispatch multiple /delegated-implement. No session modes.

## Tickets

Status directories: `idea/` → `todo/` → `wip/` → `done/` (or `dropped/`).
Reference by stem only (e.g., `260407-research-delegation-model-consolidation`).

## Session Notes

<!-- Cross-session intent only, 2-5 lines max, delete when stale. -->
- Delegation model consolidation — sprint/marathon removed, delegated-implement
  landed. Phase 6 (write-ticket clerk delegation) remains
  (see ticket `260407-research-delegation-model-consolidation`).
