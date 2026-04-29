---
title: "Delegation model consolidation — flat skill architecture, session mode removal"
related:
  - 260405-research-marathon-delegation-hardening  # superseded — marathon removed entirely
completed: 2026-04-08
---

# Delegation Model Consolidation

After extracting shared infrastructure (`infra/impl-playbook.md`,
`infra/impl-process.md`, `infra/agents/`), the delegation model across
skills had clear structural problems. This ticket captures design
decisions and remaining implementation work.

## Problem

Three entry points (implement, sprint, marathon) composed four concerns
(implementation discipline, lifecycle, team communication, session model)
in inconsistent ways. Session modes (sprint, marathon) added complexity
without irreducible value once individual skills gained clear delegation
patterns.

## Completed (prior session)

- Extracted `infra/impl-playbook.md` (subagent-safe) and `infra/impl-process.md` (top-level only).
- Merged execute-plan into implement; deleted execute-plan.
- Moved `marathon/agents/` → `infra/agents/`; updated all references.
- Updated install.sh for per-entry infra symlinks.

## Decisions (260408 session)

### Quality Pyramid

All delegation flows enforce a three-tier quality model:

| Tier | What | Who |
|------|------|-----|
| 1 | Public contracts (API shape, data types) | Decided at ticket time |
| 2 | Contract joints (integration tests, cross-module seams) | Main agent writes directly |
| 3 | Internal implementation | Delegated — "make the tests pass" |

### Workflow sequence

```
/write-ticket → (agent suggests skeleton)
  → /write-skeleton → stubs + integration tests (code change starts here)
    → complex: /write-plan → /implement
    → simple: /implement (inline outline, always)
```

Workflow is a **suggest loop**, not an auto-pipeline. Agent proposes next
step; user decides. Skill descriptions encode when to suggest, not just
what they do.

### Specific decisions

1. **`/write-skeleton` — new skill.** Standalone, between ticket and
   plan/implement. Main agent writes public interface stubs and
   integration tests at contract joints. Delegating adjacent-contract
   research to subagents is fine; writing the skeleton itself is not
   delegated.

2. **Session modes removed entirely.** Both sprint and marathon are
   deleted. Their value is fully covered by the flat skill set.
   Marathon's original justification (opus token cost) is resolved by
   delegated-implement sending Tier 3 work to sonnet subagents.
   Sprint was redundant once implement gained inline outline.

3. **`/implement` inline outline — mandatory.** Every implementation,
   no matter how trivial, starts with: (a) search for reusable
   components, (b) sketch what goes where. "Zero-thought coding" is
   abandoned.

4. **`/write-plan` = deep outline.** Same activity as implement's inline
   outline but at greater depth, possibly delegated. Invoked when
   implementation involves complex multi-module interaction.

5. **Flat skill architecture.** No session modes, no team orchestration
   layer. Each skill owns its own delegation pattern:
   - `/delegated-implement` — implementer + reviewer cycle (new skill)
   - `/write-plan` — can delegate research to planner subagent
   - `/write-ticket` — gains clerk delegation mode for subagent edits
   - `/write-skeleton` — can delegate contract gathering to Explore agent
   Parallel work = dispatch multiple delegated-implements. No special
   coordination mode needed.

6. **infra/agents/ role files retained.** Role definitions survive as
   references for subagent spawning. Marathon-specific language removed
   (decolonization).

### Target architecture

```
Workflow skills:
  /discuss → /write-ticket → /write-skeleton → (/write-plan) → /implement
  /delegated-implement  (Tier 3 delegation: implementer + reviewer)
  /write-spec

Utility skills:
  /monologue, /manual-think, /rebuild-mental-model, /chat-over-session

infra/agents/:
  Role definitions consumed by skills when spawning subagents.
  No team orchestration — each skill manages its own spawns.
```

## Remaining phases

### Phase 3: Agent content decolonization

Agent files in `infra/agents/` still reference marathon-flavored
conventions (e.g., `_common.md` mentions marathon branching). Review
each file and generalize language so they work cleanly from any
calling context (implement, delegated-implement, bare session).

Move `marathon/ask.sh` to `infra/` if generally useful, or delete.

### Phase 4: Create `/delegated-implement`

New skill: one implementation cycle with implementer + reviewer +
lead communication. Consumes `infra/agents/implementer.md` and
`infra/agents/reviewer.md`. Skeleton stubs + integration tests are
the acceptance criteria.

### Phase 5: Sprint + Marathon removal

Delete `claude/skills/sprint/` and `claude/skills/marathon/`.
Migrate useful patterns to infra or individual skills:
- Sprint delegation templates → delegated-implement or infra
- Marathon ask.sh → infra/ (if retained)
- Sprint/marathon reviewer spawn → delegated-implement
Update `_index.md`, `CLAUDE.md`, `install.sh`.

### Phase 6: write-ticket clerk delegation

Add delegation mode to `/write-ticket`: when invoked by a subagent
or when the main agent delegates ticket edits, spawn a one-shot
clerk subagent that reads write-ticket conventions and applies
directed edits. Replaces the marathon clerk team-member pattern.
