---
name: lead-proceed
description: First step for any implementation task. Reads existing tickets, plans, skeletons, and session context, then routes before code is touched.
---

# Proceed

Target: user request

## Invariants

- Route only; do not implement, plan, or write skeletons here.
- Invoke `ws:lead-workflow-manual` first when workflow primitives are not already in context.
- Assess from conversation state and artifacts only; do not read source code.
- Pipeline order is fixed: spec -> ticket -> skeleton -> implementation.
- Execution mode is single; split multi-scope work into separate tickets.
- Always route implementation through `ws:lead-implement`.
- Existing `ready/` ticket path skips `ws:lead-write-ticket`; existing `todo/` ticket path routes through ready promotion before implementation.
- Actionable inline target invokes `ws:lead-write-ticket`, captures `Ticket:`, then re-checks status; `todo/` output must promote to `ready/` before implementation.
- Exploratory target stops and suggests `ws:lead-discuss`.
- Announce routing before execution; chain stages without pausing for confirmation.
- Prefix stages receive gate-suppression context in arguments.
- Warmth is current-session context, not target identity.

## On: invoke

### 1. Assess

1. Parse target: ticket path or inline description.
2. If ticket path: read ticket; extract status, scope, phases, `plans:`, and `skeletons:`.
3. Check artifacts: ticket frontmatter, `ai-docs/.plans/`, skeleton stubs, or integration tests.
4. If inline: assess from description only.
5. Classify warmth from conversation state.
6. Classify exploratory vs actionable for `judge: needs-ticket`.

### 2. Route

1. Invoke `ws:lead-write-spec` with:
   `Chained from ws:lead-proceed - write any planned entries without asking; the session reminder will still emit.`
2. Apply `judge: needs-ticket`.
3. If invoking `ws:lead-write-ticket`, append:
   `Chained from ws:lead-proceed - treat spec coverage as satisfied whether ws:lead-write-spec wrote anything or exited early.`
4. If the current or captured ticket status is `todo/`, stop implementation routing and invoke `ws:lead-discuss` for `todo/` -> `ready/` promotion. Continue only after the target path is `ready/`.
5. Apply `judge: needs-skeleton`.
6. Build pipeline:
   - No skeleton: `ws:lead-implement`.
   - Skeleton: `ws:lead-write-skeleton` -> `ws:lead-implement`.

### 3. Announce

```text
## Pipeline: <stage> -> <stage> [-> <stage>]

- **Target**: <ticket path or brief summary>
- **Warmth**: <warm | cold> - <evidence from conversation state>
- **Skeleton**: <skip (reason) | ws:lead-write-skeleton (reason)>
- **Execution**: ws:lead-implement - <reason>
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
5. If the captured path is under `ai-docs/tickets/todo/`, invoke `ws:lead-discuss` for `todo/` -> `ready/` promotion and stop; do not invoke skeleton or implementation.
6. Use only `ready/` ticket paths downstream.

## Judgments

### judge: needs-ticket

| Decision | When |
|----------|------|
| Stop, suggest `ws:lead-discuss` | Target is exploratory; user is weighing approaches |
| Proceed | Target is an existing ticket path |
| Invoke `ws:lead-write-ticket` | Target is an actionable inline description |

### judge: needs-skeleton

| Decision | When |
|----------|------|
| Skip | Skeleton exists for this scope |
| Skip | Small isolated change: single file, no new public contracts |
| Skeleton | Public interface, cross-module boundary, or new type contract changes |

## Doctrine

Proceed optimizes for **full-pipeline routing accuracy**. Conversation state and
artifacts are the finite signal: use them to choose sub-skills, not to replicate
sub-skill gates. Warmth sharpens directives; it does not skip stages. When a
rule is ambiguous, apply whichever interpretation better preserves the user's
ability to intervene at any pipeline stage.
