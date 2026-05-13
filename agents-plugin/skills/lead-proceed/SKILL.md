---
name: lead-proceed
description: Required first step for implementation tasks. Use when starting from a ticket path or task description so existing specs, tickets, plans, and session context are routed before code is touched.
---

# Proceed

Target: user request

## Invariants

- Route only; do not implement, plan, or write skeletons here.
- Invoke `ws:lead-workflow-manual` first when workflow primitives are not already in context.
- Assess from conversation state and artifacts only; do not read source code.
- Pipeline order is fixed: spec -> ticket -> implementation.
- Execution mode is single; split multi-scope work into separate tickets.
- Always route code-editing work through `ws:lead-implement`, including skeleton work.
- Existing non-epic `ready/` ticket path skips `ws:lead-write-ticket`; existing `todo/` ticket path routes through ready promotion before implementation.
- Epic ticket paths are board artifacts, never implementation targets; stop and route to child ticket creation, promotion, or proceed.
- Actionable inline target invokes `ws:lead-write-ticket`, captures `Ticket:`, then re-checks status; `todo/` output must promote to `ready/` before implementation.
- Exploratory target stops and suggests `ws:lead-discuss`.
- Announce routing before execution; chain stages without pausing for confirmation.
- Prefix stages receive gate-suppression context in arguments.
- Warmth is current-session context, not target identity.

## On: invoke

### 1. Assess

1. Parse target: ticket path or inline description.
2. If ticket path: read ticket; extract status, category, scope, phases, and `plans:`.
3. Check workflow artifacts: ticket frontmatter and `ai-docs/.plans/`; do not inspect source stubs, skeletons, or tests.
4. If inline: assess from description only.
5. Classify warmth from conversation state.
6. Classify exploratory vs actionable for `judge: needs-ticket`.

### 2. Route

1. Invoke `ws:lead-write-spec` with:
   `Chained from ws:lead-proceed - write any planned entries without asking; the session reminder will still emit.`
2. Apply `judge: needs-ticket`.
3. If invoking `ws:lead-write-ticket`, append:
   `Chained from ws:lead-proceed - re-check spec coverage before invoking ws:lead-write-spec again; do not pause for approval when coverage can be created autonomously.`
4. If the current or captured ticket category is `epic`, stop implementation routing; suggest `ws:lead-write-ticket` for a child ticket, `ws:lead-discuss` to promote an existing child, or `ws:lead-proceed` on a ready child ticket.
5. If the current or captured ticket status is `todo/`, stop implementation routing and invoke `ws:lead-discuss` for `todo/` -> `ready/` promotion. Continue only after the target path is `ready/`.
6. Build pipeline: `ws:lead-implement`.

### 3. Announce

```text
## Pipeline: <stage> -> <stage> [-> <stage>]

- **Target**: <ticket path or brief summary>
- **Warmth**: <warm | cold> - <evidence from conversation state>
- **Execution**: ws:lead-implement - owns skeleton decisions, code-editing stages, and branch lifecycle
- **Gate suppression**: prefix stages receive override context.

Proceeding.
```

Include prefix stages in the pipeline line when they fire.
Do not ask for confirmation; the user can interrupt.

### 4. Execute

1. Invoke stages sequentially with the current target.
2. After each stage, verify completion from committed artifacts or stage output.
3. Stop on failure or user interruption.
4. If `ws:lead-write-ticket` ran, capture its `Ticket:` path before any downstream stage.
5. If the captured path stem category is `epic`, stop; do not invoke skeleton or implementation on the epic path. Route to child ticket creation, child ready promotion, or proceed on a ready child ticket.
6. If the captured path is under `ai-docs/tickets/todo/`, invoke `ws:lead-discuss` for `todo/` -> `ready/` promotion and stop; do not invoke skeleton or implementation.
7. Use only non-epic `ready/` ticket paths downstream.
8. Invoke `ws:lead-implement` with the target.

## Judgments

### judge: needs-ticket

| Decision | When |
|----------|------|
| Stop, suggest `ws:lead-discuss` | Target is exploratory; user is weighing approaches |
| Proceed | Target is an existing ticket path |
| Invoke `ws:lead-write-ticket` | Target is an actionable inline description |

## Doctrine

Proceed optimizes for **full-pipeline routing accuracy**. Conversation state and
artifacts are the finite signal: use them to choose readiness stages, not to
perform or pre-decide code-editing stages. Warmth sharpens routing; it does not
skip stages. When a rule is ambiguous, apply whichever interpretation better
preserves the user's ability to intervene at any pipeline stage.
