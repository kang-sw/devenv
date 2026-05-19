---
name: lead-proceed
description: Required first step for implementation tasks. Use when starting from a ticket path or task description so existing specs, tickets, plans, and session context are routed before code is touched.
---

# Proceed

Target: user request

## Invariants

Scope
- Route only; do not implement or plan here.
- Invoke `wsflow:lead-workflow-manual` first when workflow primitives are not already in context.
- Assess from conversation state and artifacts only; do not read source code.
- Do not rejudge ticket quality, demand ticket splitting, or mutate ticket structure.

Pipeline
- Handoff stage order is fixed when stages fire: spec -> ticket -> implementation.
- Always route code-editing work through `wsflow:lead-implement`.
- Proceed assumes implementation intent; stop only when the target is not actionable or user-blocking discussion remains.

Execution
- Announce routing before execution; chain stages without pausing for confirmation.
- Handoff stages receive carried gate-suppression context.
- Implementation handoff carries execution path, complexity flag, and branch mode.
- Warmth is current-session context, not target identity.

## Route Rules

Route Context
- `has-ticket` is artifact state; do not treat it as a judgment.
- `discussion-needed` blocks every implementation route.
- `needs-ticket` applies only to actionable inline targets without a ticket.
- `ticket-freshness` applies only when `has-ticket=yes` and warmth is warm.
- `complexity-flag` applies only from conversation and workflow artifacts.

Routing
- Use the first matching route row.
- Captured `Ticket:` paths re-enter route context before implementation.
- Honor one explicit phase name exactly.
- Stop when one proceed request names multiple phases.
- When the user does not name a phase, select the first unfinished phase.
- Stop when the next phase is too broad for one complete implementation unit.
- Accept `--auto-slice`, `auto-slice`, and equivalent phrasing as the same default phase-selection policy.

## On: invoke

### 1. Build Route Context

1. Parse target: ticket path or inline description.
2. Set `target-kind`: `ticket-path` or `inline`.
3. Set `has-ticket=yes` for an existing ticket path or captured `Ticket:` path.
4. If `has-ticket=yes`: read ticket; extract status, category, scope, phases, phase results, open questions, and `plans:`.
5. Check workflow artifacts: ticket frontmatter and `ai-docs/.plans/`; do not inspect source stubs or tests.
6. If `target-kind=ticket-path`: set `actionable=yes`.
7. If `target-kind=inline`: apply `judge: actionable`.
8. Apply `judge: discussion-needed`.
9. If `target-kind=inline` and `actionable=yes`: apply `judge: needs-ticket`.
10. Classify warmth from conversation state.
11. If `has-ticket=yes` and warmth is warm: apply `judge: ticket-freshness`.
12. Resolve implementation scope for ready tickets:
   - No phase sections -> whole target.
   - One explicit phase -> that phase.
   - Multiple explicit phases -> stop for phase or ticket slicing.
   - No explicit phase -> first unfinished phase.
   - Selected phase is plainly too broad from ticket text -> stop for phase or ticket slicing.
13. For implementation routes, set `execution-path=wsflow:lead-implement -> wsflow:lead-edit`.
14. For implementation routes, apply `judge: complexity-flag`.
15. For implementation routes, apply `judge: branch-mode`.

### 2. Select Route

| When | Route |
|------|-------|
| `target-kind=inline` and `actionable=no` | Continue through `wsflow:lead-discuss`; carry the blocker; stop. |
| `has-ticket=yes` and category is `epic` | Stop; suggest child ticket creation, child promotion, or proceed on a ready child. |
| `discussion-needed=yes` | Continue through `wsflow:lead-discuss`; carry the blocker; stop. |
| `has-ticket=yes` and status is `todo/` | Continue through `wsflow:lead-write-spec`, then `wsflow:lead-write-ticket`; carry `promote-context`; capture `Ticket:` and re-route. |
| `has-ticket=yes` and freshness is missing settled decisions | Continue through `wsflow:lead-write-ticket`; carry `freshness-context`; capture `Ticket:` and re-route. |
| `has-ticket=yes` and status is `ready/` | Continue through `wsflow:lead-implement`; carry resolved scope, execution path, complexity flag, and branch mode. |
| `has-ticket=no` and `needs-ticket=yes` | Continue through `wsflow:lead-write-spec`, then `wsflow:lead-write-ticket`; carry `create-context`; capture `Ticket:` and re-route. |
| `has-ticket=no` and `needs-ticket=no` | Continue through `wsflow:lead-implement`; carry inline target, no-ticket scope, execution path, complexity flag, and branch mode. |

### 3. Announce

```text
## Pipeline: <stage> -> <stage> [-> <stage>]

- **Target**: <ticket path or brief summary>
- **Warmth**: <warm | cold> - <evidence from conversation state>
- **Ticket**: <present | absent> - <status/category or reason no ticket is needed>
- **Discussion**: <not needed | needed - blocker>
- **Slice**: <Phase N[: title] | whole target - no phases>
- **Execution Path**: wsflow:lead-implement -> wsflow:lead-edit
- **Complexity Flag**: <narrow | broad | caller-visible | cross-module>
- **Branch Mode**: <continue current branch | create branch | sprint blocked>
- **Execution**: wsflow:lead-implement - owns direct execution, documentation, and final reporting
- **Carried context**: downstream stages receive route constraints.

Proceeding.
```

Include chained stages in the pipeline line when they fire.
Do not ask for confirmation; the user can interrupt.

### 4. Execute

1. Invoke the selected route.
2. After each stage, verify completion from committed artifacts or stage output.
3. Stop on failure or user interruption.
4. If `wsflow:lead-write-ticket` ran, capture its `Ticket:` path before downstream routing.
5. If the captured path remains under `ai-docs/tickets/todo/`, stop and report the ready-promotion blocker.
6. If a ticket path was captured, rebuild route context from that path and re-enter `Select Route`.

## Carried Context

`spec-context`:
`Chained from wsflow:lead-proceed - write any planned entries without asking; the session reminder will still emit.`

`gate-suppression-context`:
Carry: this is an autonomous proceed chain; downstream stages do not pause for approvals that this route already grants.

`execution-context`:
Carry: `execution-path`, `complexity-flag`, and `branch-mode`; `wsflow:lead-implement` preserves the flag as routing context for `wsflow:lead-edit`.

`create-context`:
Carry `spec-context` and `gate-suppression-context`, then continue through `wsflow:lead-write-ticket`.
Carry: re-check spec coverage before returning to `wsflow:lead-write-spec`; create autonomous coverage when possible.

`promote-context`:
Carry `spec-context` and `gate-suppression-context`, then continue through `wsflow:lead-write-ticket`.
Carry: implementation intent; autonomous promotion for spec coverage, frontmatter, or queue updates; escalate unresolved design blockers.

`freshness-context`:
Continue through `wsflow:lead-write-ticket`.
Carry: refresh from active conversation only; capture missing settled decisions, constraints, and rejected alternatives; do not inspect source, broad docs, decomposition, or implementation plan.

## Judgments

### judge: actionable

| Decision | When |
|----------|------|
| No | Target does not name a concrete change, observable outcome, or accepted implementation direction |
| Yes | Target gives enough implementation intent to route without another design turn |

Proceed assumes implementation intent, but this judge catches malformed or still-open targets.

### judge: discussion-needed

| Decision | When |
|----------|------|
| Yes | User-blocking design choice, scope boundary, acceptance criterion, trade-off, or delegation decision remains open |
| Yes | Ticket promotion or implementation scope cannot be completed autonomously |
| No | Missing spec coverage, frontmatter, queue hygiene, tests, or local implementation details can be resolved autonomously |

### judge: needs-ticket

| Decision | When |
|----------|------|
| Yes | Inline target changes workflow semantics, public contracts, cross-skill routing, queue behavior, branch behavior, or documentation pipeline behavior |
| Yes | Inline target needs phases, acceptance criteria, explicit traceability, or durable discussion capture |
| Yes | Caller-visible behavior may need spec coverage before implementation |
| No | Inline target is narrow, routine, fully scoped, and commit `AI Context` is enough traceability |
| No | Work is internal hygiene with no useful phase tracking and no unresolved user decision |

### judge: ticket-freshness

| Decision | When |
|----------|------|
| Refresh ticket | Active conversation since ticket capture settled decisions, constraints, rejected alternatives, or scope boundaries that are absent from the ticket |
| Continue | The ticket already captures the active conversation's settled implementation intent, or the conversation only adds autonomous hygiene or implementation-detail work |

### judge: complexity-flag

Use only conversation state, ticket text, frontmatter, phase text, status,
category, spec links, and captured plan metadata. Do not inspect source, source
stubs, tests, broad docs, or implementation plans.

Pick the strongest matching flag.

| Decision | When |
|----------|------|
| `cross-module` | Artifacts show cross-skill routing, cross-module boundary, public contract, or integration boundary changes |
| `caller-visible` | Artifacts show user-visible output, CLI, MCP, documented behavior, ticket queue, branch behavior, or spec-linked behavior changes |
| `broad` | Artifacts show multi-file work, expected tests, ready-ticket work with unknown narrowness, or any unknown narrow predicate |
| `narrow` | Artifacts explicitly show single-file scope, internal-only behavior, no caller-visible behavior change, no public contract change, no new public symbols, no new tests expected, and no explicit delegation request |

### judge: branch-mode

Pick the first matching decision.

| Decision | When |
|----------|------|
| `sprint blocked` | Current branch starts with `sprint/` |
| `create branch` | User explicitly requests branch isolation or repository rules require it |
| `continue current branch` | Current branch is suitable or already matches the requested implementation scope |

## Doctrine

Proceed optimizes for **full-pipeline routing accuracy**. Conversation state and
artifacts are the finite signal: use them to choose readiness stages, not to
perform code-editing stages. Warmth sharpens routing; scope resolution bounds
execution without replacing ticket authoring. When a rule is ambiguous, apply
whichever interpretation better preserves the user's ability to intervene at any
pipeline stage.
