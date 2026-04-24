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

- This skill routes. It does not implement, plan, or write skeletons itself.
- Every routing decision is announced with rationale before execution begins.
- Each pipeline sub-skill is invoked via the Skill tool with the appropriate arguments.
- Pipeline order is fixed: skeleton → plan → implementation.
- Execution mode is always single. Split multi-scope work into separate tickets; parallel execution is not available.
- Routing assessment uses conversation state (what has already been discussed or read this session) and artifacts only. Do not read source code during assessment.
- Warmth is a property of the current session (has the main agent already engaged relevant code), not of the target itself.
- When direct-edit verdict fires, announce and invoke `ws:edit` via the Skill tool.
- If the target is an actionable inline description, auto-invoke `/write-ticket` and continue.
- If the target is an existing ticket path, skip `/write-ticket`.
- If the target is exploratory (user weighing approaches, not requesting implementation), stop and suggest `/discuss`.
- Never skip announce.
- Announce reflects routing decisions, not post-hoc outcomes. Include prefix stages in the pipeline line even when their gates exit without writing.
- Chain pipeline stages without pausing for user confirmation between stages. The only stopping points are explicit gates defined in sub-skills — report-and-approval in `/implement`, and merge.
- When invoking prefix stages (`/write-spec`, `/write-ticket`) via the Skill tool, include natural-language gate-suppression context in the args. For `/write-spec`: append "Chained from /proceed — write any 🚧 entries without asking to defer (judge: idea-level is suppressed)." For `/write-ticket`: append "Chained from /proceed — if /write-spec already ran (even as a no-op), treat spec coverage as satisfied and do not stop on judge: spec-gate."

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
6. Assess whether the target is exploratory vs. actionable: does the conversation have clear scope and direction, or is the user still weighing approaches? This signal feeds `judge: needs-ticket`.

### 2. Route

Before the pipeline judges, two prefix judges fire in order:

**judge: needs-spec (always fires first)** — always invoke `/write-spec`. Continue to `judge: needs-ticket` regardless of outcome.

**judge: needs-ticket (fires second)** — see judgment table below.

Then apply the existing pipeline judges: direct-edit → needs-plan → needs-skeleton → execution-mode.

After prefix judges complete, check whether the implementation pipeline should be skipped entirely:

**judge: direct-edit** — if the verdict is direct-edit, skip the pipeline judges and announce direct-edit in step 3.

Otherwise, apply the pipeline judgments in order. Each produces a yes/no that builds the pipeline.

1. **judge: needs-plan** — Does this need codebase research before implementation?
2. **judge: needs-skeleton** — Does this need contract stubs before implementation?
3. **judge: execution-mode** — Single-scope.

Build the pipeline from the results. Skeleton always precedes plan — the plan consumes skeleton contracts as locked inputs. When warmth is warm, `/write-plan` internally selects warm mode; proceed does not pass a mode flag — it only decides whether `/write-plan` is invoked at all.

| needs-skeleton | needs-plan | Pipeline |
|----------------|------------|----------|
| no | no | `ws:implement` |
| no | yes | `ws:write-plan` then `ws:implement` |
| yes | no | `ws:write-skeleton` then `ws:implement` |
| yes | yes | `ws:write-skeleton` then `ws:write-plan` then `ws:implement` |

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
- **Execution**: /implement — <reason>
- **Gate suppression**: prefix stages receive override context — interactive confirmation gates are suppressed.

Proceeding.
```

When prefix stages fire, prefix them in the pipeline line:
- Spec fires + ticket fires: `## Pipeline: /write-spec → /write-ticket → <implementation stages>`
- Spec fires only: `## Pipeline: /write-spec → <implementation stages>`
- Direct-edit: use existing direct-edit format unchanged

Do not ask for confirmation — announce and proceed. The user can interrupt if the routing is wrong.

### 4. Execute

For a direct-edit verdict, invoke `ws:edit` via the Skill tool with the target as arguments.

For a pipeline verdict, invoke each stage sequentially via the Skill tool, passing the target as arguments.

- After each stage, verify it completed (check for committed artifacts).
- Pass downstream context: if `/write-plan` produces a plan path, pass it to `/implement`.
- If a stage fails or the user interrupts, stop — do not continue the pipeline.
- After `judge: needs-ticket` auto-invoke: capture the ticket path from `/write-ticket`'s output. Use it as the target for all downstream stages (skeleton, plan, implementation).

## Judgments

### judge: direct-edit

| Decision | When |
|----------|------|
| Direct edit (skip pipeline) | Change is confined to a single file AND purely internal (no callers affected, no new public symbols, no new test files needed) AND user has not explicitly requested delegation |
| Engage pipeline | Any condition above is unmet — including any cross-file touch, new public contract, or new test file |

Direct edit invokes `/edit`. This is the exception, not the fast path. Warmth improves briefing quality for delegation — it does not exempt a change from delegation. When the main agent is warm, produce a richer brief for `/implement` rather than editing directly.

### judge: needs-ticket

| Decision | When |
|----------|------|
| Stop, suggest `/discuss` | Target is exploratory — user is weighing approaches, not requesting implementation |
| Proceed | Target is an existing ticket path |
| Invoke `/write-ticket`, capture `Ticket:` output, continue | Target is an inline description — any scope |

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

Execution mode is always single. Split multi-scope work into separate tickets.

| Decision | When |
|----------|------|
| Single (`/implement`) | Always |

## Doctrine

Proceed optimizes for **full-pipeline routing accuracy** — spanning spec,
ticket, and implementation stages. The signal available from conversation
state and artifacts is the finite resource: use it to select the right
sub-skill at each stage, not to replicate logic already owned by that
sub-skill's gate. Warmth improves briefing precision — a warm session
writes sharper directives, not fewer delegation steps. When a rule is
ambiguous, apply whichever interpretation better preserves the user's
ability to intervene at any pipeline stage.
