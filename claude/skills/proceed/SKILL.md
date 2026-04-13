---
name: proceed
description: >
  Auto-route and execute the right workflow pipeline. Assesses existing
  artifacts and scope, announces the chosen path, then chains through
  /write-plan, /write-skeleton, /implement, or /parallel-implement as needed.
argument-hint: "<ticket-path or inline description>"
---

# Proceed

Target: $ARGUMENTS

## Invariants

- This skill routes — it does not implement, plan, or write skeletons itself.
- Every routing decision is announced with rationale before execution begins.
- Each sub-skill is invoked via the Skill tool with the appropriate arguments.
- Pipeline order is fixed: skeleton → plan → implement. Skeleton establishes locked contracts that the plan consumes.
- `/parallel-implement` is never preceded by `/write-plan` — if a plan is needed, execution-mode is locked to single.
- If the target is too vague to route (no ticket, no actionable description), stop and suggest `/write-ticket` or `/discuss`.
- Never skip announce — the user must see the pipeline before it runs.

## On: invoke

### 1. Assess

Gather the facts needed for routing. Do not read source code — read only artifacts and metadata.

1. Parse the target: ticket path or inline description.
2. If ticket path: read the ticket. Extract scope, phases, and existing artifact references (`plans:`, `skeletons:` frontmatter).
3. Check for existing artifacts:
   - **Plan exists?** — check ticket frontmatter `plans:` field, or scan `ai-docs/plans/` for matching files.
   - **Skeleton exists?** — check ticket frontmatter `skeletons:` field, or grep for `todo!()`/`unimplemented`/`NotImplementedError` stubs in relevant paths.
4. If inline description (no ticket): assess from the description alone.

### 2. Route

Apply routing judgments in order. Each produces a yes/no that builds the pipeline.

1. **judge: needs-ticket** — Is the target actionable as-is?
2. **judge: needs-plan** — Does this need deep research before implementation?
3. **judge: needs-skeleton** — Does this need contract stubs before implementation?
4. **judge: execution-mode** — Single-scope or parallel?

Build the pipeline from the results. Skeleton always precedes plan — the plan consumes skeleton contracts as locked inputs.

| needs-skeleton | needs-plan | execution-mode | Pipeline |
|----------------|------------|----------------|----------|
| no | no | single | `/implement` |
| no | no | parallel | `/parallel-implement` |
| no | yes | single | `/write-plan` then `/implement` |
| yes | no | single | `/write-skeleton` then `/implement` |
| yes | no | parallel | `/write-skeleton` then `/parallel-implement` |
| yes | yes | single | `/write-skeleton` then `/write-plan` then `/implement` |

### 3. Announce

Output the routing decision as a structured block:

```
## Pipeline: <stage> → <stage> [→ <stage>]

- **Target**: <ticket path or brief summary>
- **Plan**: <skip (reason) | /write-plan (reason)>
- **Skeleton**: <skip (reason) | /write-skeleton (reason)>
- **Execution**: </implement | /parallel-implement> — <reason>

Proceeding.
```

Do not ask for confirmation — announce and proceed. The user can interrupt if the routing is wrong.

### 4. Execute

Invoke each pipeline stage sequentially via the Skill tool, passing the target as arguments.

- After each stage, verify it completed (check for committed artifacts).
- Pass downstream context: if `/write-plan` produces a plan path, pass it to `/implement`.
- If a stage fails or the user interrupts, stop — do not continue the pipeline.

## Judgments

### judge: needs-ticket

| Decision | When |
|----------|------|
| Stop, suggest `/write-ticket` | Target is a vague idea with no clear scope or acceptance criteria |
| Stop, suggest `/discuss` | Target is exploratory — user is weighing approaches, not requesting implementation |
| Proceed | Target is a ticket path, or an inline description with clear scope and deliverables |

### judge: needs-plan

| Decision | When |
|----------|------|
| Skip | Plan already exists for this scope (found in ticket frontmatter or plans directory) |
| Skip | Implementation path is derivable from existing code — even if cross-module, the pattern is established or the implementer can orient during brief planning |
| Plan | Multiple viable architectural approaches with non-obvious trade-offs that must be resolved before coding starts |
| Plan | Changes requiring coordination across 3+ modules with no existing pattern to follow |
| Plan | User explicitly requests deep research |

### judge: needs-skeleton

| Decision | When |
|----------|------|
| Skip | Skeleton already exists for this scope (stubs or integration tests found) |
| Skip | Change is small and isolated — single file, no new public contracts |
| Skeleton | Change introduces or modifies public interfaces, cross-module boundaries, or new type contracts |

### judge: execution-mode

If `needs-plan = yes`, execution-mode is locked to single — do not evaluate parallel.

| Decision | When |
|----------|------|
| Single (`/implement`) | Default — one cohesive scope |
| Parallel (`/parallel-implement`) | 2+ scopes with no shared files and no shared interfaces, each independently testable. Structural isolation (separate directory trees) is sufficient signal without a skeleton; file-level isolation within a shared directory requires a skeleton to confirm disjointness mechanically. |

## Doctrine

Proceed optimizes for **routing accuracy with minimal assessment overhead** —
gather just enough signal to pick the right pipeline, announce the decision
for user visibility, then delegate fully to the chosen sub-skills. When a
rule is ambiguous, apply whichever interpretation gets to the first sub-skill
invocation faster while preserving the user's ability to intervene.
