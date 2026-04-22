---
name: proceed
description: >
  First step for any implementation task. Reads what already exists —
  tickets, plans, skeletons, session context — and determines the right
  execution path before any code is touched. Use when starting
  implementation work on a ticket or task description.
argument-hint: "<ticket-path or inline description>"
---

# Proceed

Target: $ARGUMENTS

## Invariants

- This skill routes — it does not implement, plan, or write skeletons itself.
- Every routing decision is announced with rationale before execution begins.
- Each pipeline sub-skill is invoked via the Skill tool with the appropriate arguments.
- Pipeline order is fixed: skeleton → plan → implementation. Skeleton establishes locked contracts that the plan consumes.
- `/parallel-implement` is never preceded by `/write-plan` — if a plan is needed, execution-mode is locked to single.
- Routing assessment uses conversation state (what has already been discussed or read this session) and artifacts only — do not read source code during assessment.
- Warmth is a property of the current session (has the main agent already engaged relevant code), not of the target itself.
- When direct-edit verdict fires, announce and invoke `ws:edit` via the Skill tool.
- If the target is too vague to route (no ticket, no actionable description), stop and suggest `/write-ticket` or `/discuss`.
- Never skip announce — the user must see the routing decision before anything proceeds.
- Chain pipeline stages without pausing for user confirmation between stages. The only stopping points are explicit gates defined in sub-skills — report-and-approval in `/delegate-implement` and `/parallel-implement`, and merge.

## On: invoke

### 1. Assess

Gather the facts needed for routing. Do not read source code — read only artifacts and metadata.

1. Parse the target: ticket path or inline description.
2. If ticket path: read the ticket. Extract scope, phases, and existing artifact references (`plans:`, `skeletons:` frontmatter).
3. Check for existing artifacts:
   - **Plan exists?** — check ticket frontmatter `plans:` field, or scan `ai-docs/plans/` for matching files.
   - **Skeleton exists?** — check ticket frontmatter `skeletons:` field, or grep for `todo!()`/`unimplemented`/`NotImplementedError` stubs in relevant paths.
4. If inline description (no ticket): assess from the description alone.
5. Assess context warmth: has this session already engaged with relevant code (prior turns read files in the target scope, or user explicitly signaled direct authorship like "let me draft it" or "I'll do it directly")? Signal is observable from conversation state alone — do not read source to decide.

### 2. Route

First check whether the pipeline should be skipped entirely:

**judge: direct-edit** — if the verdict is direct-edit, skip the pipeline judges and announce direct-edit in step 3.

Otherwise, apply the pipeline judgments in order. Each produces a yes/no that builds the pipeline.

1. **judge: needs-ticket** — Is the target actionable as-is?
2. **judge: needs-plan** — Does this need codebase research before implementation?
3. **judge: needs-skeleton** — Does this need contract stubs before implementation?
4. **judge: execution-mode** — Single-scope or parallel?

Build the pipeline from the results. Skeleton always precedes plan — the plan consumes skeleton contracts as locked inputs. When warmth is warm, `/write-plan` internally selects warm mode; proceed does not pass a mode flag — it only decides whether `/write-plan` is invoked at all.

| needs-skeleton | needs-plan | execution-mode | Pipeline |
|----------------|------------|----------------|----------|
| no | no | single | `ws:delegate-implement` |
| no | no | parallel | `ws:parallel-implement` |
| no | yes | single | `ws:write-plan` then `ws:delegate-implement` |
| yes | no | single | `ws:write-skeleton` then `ws:delegate-implement` |
| yes | no | parallel | `ws:write-skeleton` then `ws:parallel-implement` |
| yes | yes | single | `ws:write-skeleton` then `ws:write-plan` then `ws:delegate-implement` |

### 3. Announce

For a direct-edit verdict, announce:

```
## Direct edit → /edit

- **Target**: <ticket path or brief summary>
- **Warmth**: warm — <what the main agent already knows>
- **Reason**: <why pipeline is overkill for this change>

Invoking `/edit`.
```

For a pipeline verdict, announce:

```
## Pipeline: <stage> → <stage> [→ <stage>]

- **Target**: <ticket path or brief summary>
- **Warmth**: <warm | cold> — <evidence from conversation state>
- **Plan**: <skip (reason) | /write-plan (reason)>
- **Skeleton**: <skip (reason) | /write-skeleton (reason)>
- **Execution**: </delegate-implement | /parallel-implement> — <reason>

Proceeding.
```

Do not ask for confirmation — announce and proceed. The user can interrupt if the routing is wrong.

### 4. Execute

For a direct-edit verdict, invoke `ws:edit` via the Skill tool with the target as arguments.

For a pipeline verdict, invoke each stage sequentially via the Skill tool, passing the target as arguments.

- After each stage, verify it completed (check for committed artifacts).
- Pass downstream context: if `/write-plan` produces a plan path, pass it to `/delegate-implement`.
- If a stage fails or the user interrupts, stop — do not continue the pipeline.

## Judgments

### judge: direct-edit

| Decision | When |
|----------|------|
| Direct edit (skip pipeline) | Change is confined to a single file AND purely internal (no callers affected, no new public symbols, no new test files needed) AND user has not explicitly requested delegation |
| Engage pipeline | Any condition above is unmet — including any cross-file touch, new public contract, or new test file |

Direct edit invokes `/edit`. This is the exception, not the fast path. Warmth improves briefing quality for delegation — it does not exempt a change from delegation. When the main agent is warm, produce a richer brief for `/delegate-implement` rather than editing directly.

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
| Skip | Implementation path is derivable from existing code — main agent is warm on the area, or the pattern is established and a cold implementer can orient during brief planning |
| Plan | Multiple viable architectural approaches with non-obvious trade-offs that must be resolved before coding starts |
| Plan | Changes requiring coordination across 3+ modules with no existing pattern to follow |
| Plan | User explicitly requests deep research, or main agent wants to lock decisions as a committed artifact before implementation |

When plan fires with a warm main agent, `/write-plan` internally selects warm mode (main-agent-authored draft + `plan-populator` enrichment).

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
| Single (`/delegate-implement`) | Default — one cohesive scope |
| Parallel (`/parallel-implement`) | 2+ scopes with no shared files and no shared interfaces, each independently testable. Structural isolation (separate directory trees) is sufficient signal without a skeleton; file-level isolation within a shared directory requires a skeleton to confirm disjointness mechanically. |

## Doctrine

Proceed optimizes for **routing accuracy under session-warmth awareness** —
gather just enough signal from conversation state and artifacts to pick the
right path, announce the decision for user visibility, then delegate to the
chosen sub-skills. Delegation is the default; direct edit is the exception
for trivially local single-file changes with no external impact. Warmth is
the axis that improves briefing precision for delegation — a warm agent
writes sharper directives and a richer brief, not fewer delegation steps.
When a rule is ambiguous, apply whichever interpretation better preserves
the user's ability to intervene while keeping delegation as the baseline.
