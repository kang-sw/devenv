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
- Pipeline order is fixed: spec -> ticket -> implementation.
- Always route code-editing work through `wsflow:lead-implement`.

Execution
- Announce routing before execution; chain stages without pausing for confirmation.
- Prefix stages receive gate-suppression context in arguments.
- Warmth is current-session context, not target identity.

## Route Rules

Ticket Paths
- Existing non-epic `ready/` ticket path skips `wsflow:lead-write-ticket` unless the freshness gate fires, then selects an implementation slice.
- Existing `todo/` ticket path invokes `wsflow:lead-write-ticket` for autonomous ready promotion before slice selection.
- Epic ticket paths are board artifacts, never implementation targets; stop and route to child ticket creation, promotion, or proceed.

Inline Targets
- Actionable inline target invokes `wsflow:lead-write-ticket`, captures `Ticket:`, then re-checks status; `todo/` output must promote to `ready/` before implementation.
- Exploratory target stops and suggests `wsflow:lead-discuss`.

Escalation
- Escalate to `wsflow:lead-discuss` only for unresolved design choices that block ready promotion or implementation scope.

Slice
- Execution slice defaults to one unfinished phase; include multiple phases only by user request or inseparable verification.

## On: invoke

### 1. Assess

1. Parse target: ticket path or inline description.
2. If ticket path: read ticket; extract status, category, scope, phases, phase results, open questions, and `plans:`.
3. Check workflow artifacts: ticket frontmatter and `ai-docs/.plans/`; do not inspect source stubs or tests.
4. If inline: assess from description only.
5. Classify warmth from conversation state.
6. Classify exploratory vs actionable for `judge: needs-ticket`.
7. If a current or captured ticket exists and warmth is warm, apply `judge: ticket-freshness`.

### 2. Route

1. Invoke `wsflow:lead-write-spec` with:
   `Chained from wsflow:lead-proceed - write any planned entries without asking; the session reminder will still emit.`
2. Apply `judge: needs-ticket`.
3. If invoking `wsflow:lead-write-ticket`, append:
   `Chained from wsflow:lead-proceed - re-check spec coverage before invoking wsflow:lead-write-spec again; do not pause for approval when coverage can be created autonomously.`
4. If the current or captured ticket category is `epic`, stop implementation routing; suggest `wsflow:lead-write-ticket` for a child ticket, `wsflow:lead-discuss` to promote an existing child, or `wsflow:lead-proceed` on a ready child ticket.
5. If the current or captured ticket status is `todo/`, apply `judge: escalation-needed`.
6. If escalation is needed, stop and invoke `wsflow:lead-discuss` with the blocker.
7. If the current or captured ticket status is `todo/`, invoke `wsflow:lead-write-ticket` for `todo/` -> `ready/` promotion and append:
   `Chained from wsflow:lead-proceed - treat this as implementation intent; promote autonomously when only spec coverage, frontmatter, or queue updates are needed; escalate only unresolved design blockers.`
   Capture the moved ticket path.
8. If `judge: ticket-freshness` found missing settled decisions, invoke `wsflow:lead-write-ticket` edit and append:
   `Chained from wsflow:lead-proceed - refresh this ticket from active conversation context only; capture settled decisions, constraints, and rejected alternatives that are missing from the ticket; do not inspect source code, read broad documentation, rejudge decomposition, or plan implementation.`
   Capture the refreshed ticket path.
9. Use only non-epic `ready/` ticket paths downstream.
10. Apply `judge: implementation-slice`.
11. Build pipeline: `wsflow:lead-implement`.

### 3. Announce

```text
## Pipeline: <stage> -> <stage> [-> <stage>]

- **Target**: <ticket path or brief summary>
- **Warmth**: <warm | cold> - <evidence from conversation state>
- **Slice**: <Phase N[: title] | Phase N-M[: title summary] | whole target - no phases>
- **Execution**: wsflow:lead-implement - owns direct execution, documentation, and final reporting
- **Gate suppression**: prefix stages receive override context.

Proceeding.
```

Include prefix stages in the pipeline line when they fire.
Do not ask for confirmation; the user can interrupt.

### 4. Execute

1. Invoke stages sequentially with the current target.
2. After each stage, verify completion from committed artifacts or stage output.
3. Stop on failure or user interruption.
4. If `wsflow:lead-write-ticket` ran, capture its `Ticket:` path before any downstream stage.
5. If the captured path stem category is `epic`, stop; do not invoke implementation on the epic path. Route to child ticket creation, child ready promotion, or proceed on a ready child ticket.
6. If the captured path remains under `ai-docs/tickets/todo/`, stop and report the ready-promotion blocker; do not invoke implementation.
7. Re-read the ready ticket after promotion and select the implementation slice.
8. Invoke `wsflow:lead-implement` with the target and `Scope: implement <slice> only`.

## Judgments

### judge: needs-ticket

| Decision | When |
|----------|------|
| Stop, suggest `wsflow:lead-discuss` | Target is exploratory; user is weighing approaches |
| Proceed | Target is an existing ticket path |
| Invoke `wsflow:lead-write-ticket` | Target is an actionable inline description |

### judge: escalation-needed

| Decision | When |
|----------|------|
| Escalate to `wsflow:lead-discuss` | Ticket has unresolved design decisions, unclear completion criteria, unresolved user trade-offs, or cannot gain spec coverage |
| Continue autonomously | Promotion needs only spec coverage, frontmatter, queue entry, or routine ready-gate normalization |

### judge: implementation-slice

| Decision | When |
|----------|------|
| Whole target | Ready target has no phase sections |
| First unfinished phase | Ready target has unfinished phases and the user did not request a broader slice |
| User-requested phase range | User explicitly named phases to implement |
| Inseparable phase range | Adjacent phases cannot be verified separately from ticket artifacts |

### judge: ticket-freshness

| Decision | When |
|----------|------|
| Refresh ticket | Active conversation since ticket capture settled decisions, constraints, rejected alternatives, or scope boundaries that are absent from the ticket |
| Continue | The ticket already captures the active conversation's settled implementation intent, or the conversation only adds autonomous hygiene or implementation-detail work |

## Doctrine

Proceed optimizes for **full-pipeline routing accuracy**. Conversation state and
artifacts are the finite signal: use them to choose readiness stages, not to
perform code-editing stages. Warmth sharpens routing; slice selection bounds
execution without replacing ticket authoring. When a rule is ambiguous, apply
whichever interpretation better preserves the user's ability to intervene at any
pipeline stage.
