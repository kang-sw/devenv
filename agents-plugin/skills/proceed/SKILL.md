---
name: proceed
description: First step for any implementation task. Reads existing tickets, plans, skeletons, and session context, then routes before code is touched.
---

# Proceed

Target: user request

## Invariants

- This skill routes. It does not implement, plan, or write skeletons itself.
- Every routing decision is announced with rationale before execution begins.
- Each pipeline sub-skill is invoked with the appropriate arguments.
- Pipeline order is fixed: spec -> ticket -> skeleton -> implementation.
- Execution mode is always single. Split multi-scope work into separate tickets.
- Routing assessment uses conversation state and artifacts only. Do not read source code during assessment.
- Warmth is a property of the current session, not of the target itself.
- Always invoke `ws:implement` for implementation; implement applies its own execution-mode judgment.
- If the target is an actionable inline description, auto-invoke `ws:write-ticket` and continue.
- If the target is an existing ticket path, skip `ws:write-ticket`.
- If the target is exploratory, stop and suggest `ws:discuss`.
- Never skip announce.
- Announce reflects routing decisions, not post-hoc outcomes.
- Chain pipeline stages without pausing for user confirmation between stages.
- Prefix stages receive gate-suppression context in their arguments.

## On: invoke

### 1. Assess

Gather routing facts. Do not read source code; read only artifacts and metadata.

1. Parse the target: ticket path or inline description.
2. If ticket path: read the ticket. Extract scope, phases, and artifact references (`plans:`, `skeletons:` frontmatter).
3. Check for existing artifacts:
   - Plan exists? Check ticket frontmatter `plans:` or scan `ai-docs/.plans/` for matching files.
   - Skeleton exists? Check ticket frontmatter `skeletons:` or grep for `todo!()`/`unimplemented`/`NotImplementedError` stubs in relevant paths.
4. If inline description: assess from the description alone.
5. Assess context warmth from conversation state only.
6. Assess whether the target is exploratory vs. actionable; this feeds `judge: needs-ticket`.

### 2. Route

Prefix judges fire in order:

1. Invoke `ws:write-spec`. Continue to `judge: needs-ticket` regardless of outcome.
2. Apply `judge: needs-ticket`.

Prefix-stage gate-suppression context applies in all routing paths:

- For `ws:write-spec`: append `Chained from ws:proceed - write any planned entries without asking; the session reminder will still emit.`
- For `ws:write-ticket`: append `Chained from ws:proceed - treat spec coverage as satisfied whether ws:write-spec wrote anything or exited early.`

Then apply `judge: needs-skeleton`.

| needs-skeleton | Pipeline |
|----------------|----------|
| no | `ws:implement` |
| yes | `ws:write-skeleton` then `ws:implement` |

### 3. Announce

```text
## Pipeline: <stage> -> <stage> [-> <stage>]

- **Target**: <ticket path or brief summary>
- **Warmth**: <warm | cold> - <evidence from conversation state>
- **Skeleton**: <skip (reason) | ws:write-skeleton (reason)>
- **Execution**: ws:implement - <reason>
- **Gate suppression**: prefix stages receive override context.

Proceeding.
```

When prefix stages fire, include them in the pipeline line:

- Spec and ticket: `## Pipeline: ws:write-spec -> ws:write-ticket -> <implementation stages>`
- Spec only: `## Pipeline: ws:write-spec -> <implementation stages>`

Do not ask for confirmation - announce and proceed. The user can interrupt if the routing is wrong.

### 4. Execute

Invoke each pipeline stage sequentially, passing the target as arguments.

- After each stage, verify it completed by checking committed artifacts or stage output.
- If a stage fails or the user interrupts, stop.
- After `judge: needs-ticket` auto-invokes, capture the ticket path from `ws:write-ticket` output and use it downstream.

## Judgments

### judge: needs-ticket

| Decision | When |
|----------|------|
| Stop, suggest `ws:discuss` | Target is exploratory - user is weighing approaches, not requesting implementation |
| Proceed | Target is an existing ticket path |
| Invoke `ws:write-ticket`, capture `Ticket:` output, continue | Target is an inline description - any scope |

### judge: needs-skeleton

| Decision | When |
|----------|------|
| Skip | Skeleton already exists for this scope |
| Skip | Change is small and isolated - single file, no new public contracts |
| Skeleton | Change introduces or modifies public interfaces, cross-module boundaries, or new type contracts |

## Doctrine

Proceed optimizes for **full-pipeline routing accuracy** - spanning spec,
ticket, and implementation stages. The signal available from conversation
state and artifacts is the finite resource: use it to select the right
sub-skill at each stage, not to replicate logic already owned by that
sub-skill's gate. Warmth improves briefing precision - a warm session
writes sharper directives, not fewer delegation steps. When a rule is
ambiguous, apply whichever interpretation better preserves the user's
ability to intervene at any pipeline stage.
