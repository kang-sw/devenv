---
name: lead-proceed
description: Required first step for implementation tasks. Use when starting from a ticket path or task description so existing specs, tickets, plans, and session context are routed before code is touched.
---

# Proceed

Target: user request

## Invariants

Scope
- Route only; do not implement or plan here.
- Invoke `ws:lead-workflow-manual` first when workflow primitives are not already in context.
- Assess from conversation state and artifacts only; do not read source code.
- Do not rejudge ticket quality, demand ticket splitting, or mutate ticket structure.

Pipeline
- Handoff stage order is fixed when stages fire: spec -> ticket -> implementation.
- Always route code-editing work through `ws:lead-implement`.
- Proceed assumes implementation intent; stop only when the target is not actionable or user-blocking discussion remains.

Execution
- Announce routing before execution; chain stages without pausing for confirmation.
- Warmth is current-session context, not target identity.

## Route Rules

Route Context
- `has-ticket` is artifact state; do not treat it as a judgment.
- `discussion-needed` blocks every implementation route.
- `needs-ticket` applies only to actionable inline targets without a ticket.
- `ticket-freshness` applies only when `has-ticket=yes` and warmth is warm.

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
13. For implementation routes, read `ws:lead-implement` skill text.
14. For implementation routes, apply `lead-implement` `judge: execution-mode` and `judge: branch-mode` from route context only; unknown direct-edit predicates produce a delegated verdict.

### 2. Select Route

| When | Route |
|------|-------|
| `target-kind=inline` and `actionable=no` | Continue through `ws:lead-discuss`; stop. |
| `has-ticket=yes` and category is `epic` | Stop; suggest child ticket creation, child promotion, or proceed on a ready child. |
| `discussion-needed=yes` | Continue through `ws:lead-discuss`; stop. |
| `has-ticket=yes` and status is `todo/` | Continue through `ws:lead-write-spec`, then `ws:lead-write-ticket`; capture `Ticket:` and re-route. |
| `has-ticket=yes` and freshness is missing settled decisions | Continue through `ws:lead-write-ticket`; capture `Ticket:` and re-route. |
| `has-ticket=yes` and status is `ready/` | Continue through `ws:lead-implement`. |
| `has-ticket=no` and `needs-ticket=yes` | Continue through `ws:lead-write-spec`, then `ws:lead-write-ticket`; capture `Ticket:` and re-route. |
| `has-ticket=no` and `needs-ticket=no` | Continue through `ws:lead-implement`. |

### 3. Announce

```text
## Pipeline: <stage> -> <stage> [-> <stage>]

- **Target**: <ticket path or brief summary>
- **Warmth**: <warm | cold> - <evidence from conversation state>
- **Ticket**: <present | absent> - <status/category or reason no ticket is needed>
- **Discussion**: <not needed | needed - blocker>
- **Slice**: <Phase N[: title] | whole target - no phases>
- **Implementation Verdict**: <ws:lead-edit | ws:lead-write-code> via ws:lead-implement
- **Verdict Basis**: lead-implement route contract; source-free; unknown direct-edit predicate -> delegated
- **Execution**: ws:lead-implement - owns code-editing stages and branch lifecycle

Proceeding.
```

Include chained stages in the pipeline line when they fire.
Do not ask for confirmation; the user can interrupt.

### 4. Execute

1. Invoke the selected route.
2. After each stage, verify completion from committed artifacts or stage output.
3. Stop on failure or user interruption.
4. If `ws:lead-write-ticket` ran, capture its `Ticket:` path before downstream routing.
5. If the captured path remains under `ai-docs/tickets/todo/`, stop and report the ready-promotion blocker.
6. If a ticket path was captured, rebuild route context from that path and re-enter `Select Route`.

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

## Doctrine

Proceed optimizes for **full-pipeline routing accuracy**. Conversation state and
artifacts are the finite signal: use them to choose readiness stages, not to
perform code-editing stages. Warmth sharpens routing; scope resolution bounds
execution without replacing ticket authoring. When a rule is ambiguous, apply
whichever interpretation better preserves the user's ability to intervene at any
pipeline stage.
