---
title: "Delegation model consolidation — sprint redesign, shared spawn patterns, agent decolonization"
related:
  - 260405-research-marathon-delegation-hardening  # marathon-specific subset
---

# Delegation Model Consolidation

After extracting shared infrastructure (`infra/impl-playbook.md`,
`infra/impl-process.md`, `infra/agents/`), the delegation model across
skills has clear structural problems. This ticket captures the remaining
design decisions.

## Problem

Three entry points (implement, sprint, marathon) compose four concerns
(implementation discipline, lifecycle, team communication, session model)
in inconsistent ways. Agent role files now live in `infra/agents/` but
their content still assumes marathon context. Sprint carries full
lifecycle ceremony despite being "lightweight."

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

2. **Sprint absorbed.** Sprint's roles are covered by the skill toolkit
   (skeleton, implement, delegated-implement). Session modes are
   unnecessary when the agent suggests skills per situation.

3. **`/implement` inline outline — mandatory.** Every implementation,
   no matter how trivial, starts with: (a) search for reusable
   components, (b) sketch what goes where. "Zero-thought coding" is
   abandoned.

4. **`/write-plan` = deep outline.** Same activity as implement's inline
   outline but at greater depth, possibly delegated. Invoked when
   implementation involves complex multi-module interaction.

5. **Phase 2 (delegation tool) — deferred.** Skeleton as delegation
   protocol may obviate a separate `/delegate` skill. Revisit after
   skeleton is in use.

6. **Marathon → coordination mode (name TBD).** Marathon shrinks to
   parallel dispatch + round checkpoints. No longer a heavyweight
   execution framework. Rename pending.

## Remaining phases

### Phase 3: Agent content decolonization

Agent files in `infra/agents/` still reference marathon-flavored
conventions (e.g., `_common.md` mentions marathon branching). Review
each file and generalize language so they work cleanly from any
calling context (implement, bare session, future skills).

Check if `marathon/ask.sh` should also move to `infra/` or remain
marathon-specific.

### Phase 5: Sprint removal

Delete `claude/skills/sprint/` once `/write-skeleton` and updated
`/implement` are validated. Migrate any useful patterns (delegation
templates, reviewer spawn) to infra or implement.

### Phase 6: Marathon rename + simplification

Rename marathon to coordination-focused mode. Strip orchestration
overhead (bootstrap.sh, TeamCreate, sub-branches, merge ceremony)
down to: parallel delegated-implement dispatch + round checkpoints +
integration coordination.

Related: `260405-research-marathon-delegation-hardening` covers
marathon-specific lead discipline issues.
