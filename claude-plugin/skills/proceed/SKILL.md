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
- Pipeline order is fixed: skeleton → implementation.
- Execution mode is always single. Split multi-scope work into separate tickets; parallel execution is not available.
- Routing assessment uses conversation state (what has already been discussed or read this session) and artifacts only. Do not read source code during assessment.
- Warmth is a property of the current session (has the main agent already engaged relevant code), not of the target itself.
- Always invoke `ws:implement` for implementation — implement applies its own judge: execution-mode and routes to edit or write-code internally.
- If the target is an actionable inline description, auto-invoke `/write-ticket` and continue.
- If the target is an existing ticket path, skip `/write-ticket`.
- If the target is exploratory (user weighing approaches, not requesting implementation), stop and suggest `/discuss`.
- Never skip announce.
- Announce reflects routing decisions, not post-hoc outcomes. Include prefix stages in the pipeline line even when their gates exit without writing.
- Chain pipeline stages without pausing for user confirmation between stages. The only stopping points are explicit gates defined in sub-skills — report-and-approval in `/implement`, and merge.
- When invoking prefix stages (`/write-spec`, `/write-ticket`) via the Skill tool, include gate-suppression context in the args.

## On: invoke

### 1. Assess

Gather the facts needed for routing. Do not read source code — read only artifacts and metadata.

1. Parse the target: ticket path or inline description.
2. If ticket path: read the ticket. Extract scope, phases, and existing artifact references (`plans:`, `skeletons:` frontmatter).
3. Check for existing artifacts:
   - **Plan exists?** — check ticket frontmatter `plans:` field, or scan `ai-docs/.plans/` for matching files.
   - **Skeleton exists?** — check ticket frontmatter `skeletons:` field, or grep for `todo!()`/`unimplemented`/`NotImplementedError` stubs in relevant paths.
4. If inline description (no ticket): assess from the description alone.
5. Assess context warmth: has this session already engaged with relevant code (prior turns read files in the target scope, or user explicitly signaled direct authorship like "let me draft it" or "I'll do it directly")? Signal is observable from conversation state alone — do not read source to decide.
6. Assess whether the target is exploratory vs. actionable: does the conversation have clear scope and direction, or is the user still weighing approaches? This signal feeds `judge: needs-ticket`.

### 2. Route

Before the pipeline judges, two prefix judges fire in order:

**Prefix (always):** Invoke `/write-spec`. Continue to `judge: needs-ticket` regardless of outcome.

**judge: needs-ticket (fires second)** — see judgment table below.

Prefix-stage gate-suppression context applies in all routing paths (direct-edit and pipeline):
- For `/write-spec`: append to args — "Chained from /proceed — write any 🚧 entries without asking; the session reminder will still emit (this is not a standalone invocation)."
- For `/write-ticket`: append to args — "Chained from /proceed — treat spec coverage as satisfied regardless of whether /write-spec wrote anything or exited early at judge: spec-impact."

Then apply the pipeline judgments in order. Each produces a yes/no that builds the pipeline.

1. **judge: needs-skeleton** — Does this need contract stubs before implementation?

Build the pipeline:

| needs-skeleton | Pipeline |
|----------------|----------|
| no | `ws:implement` |
| yes | `ws:write-skeleton` then `ws:implement` |

### 3. Announce

```
## Pipeline: <stage> → <stage> [→ <stage>]

- **Target**: <ticket path or brief summary>
- **Warmth**: <warm | cold> — <evidence from conversation state>
- **Skeleton**: <skip (reason) | /write-skeleton (reason)>
- **Execution**: /implement — <reason>
- **Gate suppression**: prefix stages receive override context — interactive confirmation gates are suppressed.

Proceeding.
```

When prefix stages fire, prefix them in the pipeline line:
- Spec fires + ticket fires: `## Pipeline: /write-spec → /write-ticket → <implementation stages>`
- Spec fires only: `## Pipeline: /write-spec → <implementation stages>`

Do not ask for confirmation — announce and proceed. The user can interrupt if the routing is wrong.

### 4. Execute

Invoke each pipeline stage sequentially via the Skill tool, passing the target as arguments.

- After each stage, verify it completed (check for committed artifacts).
- If a stage fails or the user interrupts, stop — do not continue the pipeline.
- After `judge: needs-ticket` auto-invoke: capture the ticket path from `/write-ticket`'s output. Use it as the target for all downstream stages (skeleton, plan, implementation).

## Judgments

### judge: needs-ticket

| Decision | When |
|----------|------|
| Stop, suggest `/discuss` | Target is exploratory — user is weighing approaches, not requesting implementation |
| Proceed | Target is an existing ticket path |
| Invoke `/write-ticket`, capture `Ticket:` output, continue | Target is an inline description — any scope |

### judge: needs-skeleton

| Decision | When |
|----------|------|
| Skip | Skeleton already exists for this scope (stubs or integration tests found) |
| Skip | Change is small and isolated — single file, no new public contracts |
| Skeleton | Change introduces or modifies public interfaces, cross-module boundaries, or new type contracts |

## Doctrine

Proceed optimizes for **full-pipeline routing accuracy** — spanning spec,
ticket, and implementation stages. The signal available from conversation
state and artifacts is the finite resource: use it to select the right
sub-skill at each stage, not to replicate logic already owned by that
sub-skill's gate. Warmth improves briefing precision — a warm session
writes sharper directives, not fewer delegation steps. When a rule is
ambiguous, apply whichever interpretation better preserves the user's
ability to intervene at any pipeline stage.
