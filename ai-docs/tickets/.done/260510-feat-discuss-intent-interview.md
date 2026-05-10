---
title: discuss intent interview judgment
related:
  260429-research-host-neutral-ws-plugin: host-neutral skill semantics must stay portable across Codex and Claude surfaces
spec:
  - 260510-discuss-intent-frame-interview
related-mental-model:
  - workflow-skills
  - claude-compatibility
completed: 2026-05-10
---

# discuss intent interview judgment

## Background

`lead-discuss` should make design discussions more objective without importing the
full visible-thinking protocol from `claude-plugin/CLAUDE.home.md`.

The desired behavior is explicit user intent decomposition and objectification:
the assistant should show how it understood the user's proposal before evaluating
it. A useful shape is: "I understood X as Y and Z; objectified as YY and ZZ, this
means ...". The exact format should remain natural and skill-sized rather than a
rigid `CLAUDE.home.md` block protocol.

The current handler name `On: discussion loop` is also too vague. The skill should
be framed around each user message, with a judgment such as
`judge: needs-interview`, so the model treats the interview behavior as a per-turn
decision rather than an abstract loop.

## Decisions

- Preserve visible intent interpretation as user-facing output, not hidden
  reasoning.
- Use objective cost/benefit and failure-condition review instead of performative
  agreement with user proposals.
- Decompose intent from macro to micro when the discussion is exploratory,
  ambiguous, architectural, or design-heavy.
- Treat user signals like "the rest is autonomous" as a return to the parent
  decision level: decide remaining child details and resume exploration from the
  next unresolved higher-level branch.
- Do not copy `CLAUDE.home.md`'s verdict-block protocol into `lead-discuss`; port
  only the intent decomposition and objective review behavior.

## Phases

### Phase 1: Reframe discuss user-message handling

Rename or rewrite the `lead-discuss` handler shape from `On: discussion loop` to a
clearer user-message oriented event, such as `On: user message`.

Add `judge: needs-interview` with concrete triggers and non-triggers. It should
fire for design discussions, architecture decisions, ticket scoping, unclear user
intent, and user proposals that need trade-off review. It should not force
interview output for simple status requests, direct implementation commands, or
mechanical ticket/status operations.

Success criteria:
- The skill explicitly tells the assistant to surface how it interpreted user
  intent before evaluating design proposals.
- The judgment limits unnecessary interview overhead on straightforward turns.
- The handler terminology no longer depends on the ambiguous phrase
  `discussion loop`.

### Result (4b0070e) - 2026-05-10

`lead-discuss` and the Claude compatibility `discuss` skill now use
`On: user message` instead of `On: discussion loop`. Both skills apply
`judge: needs-intent-frame` and `judge: needs-interview` after survey and
domain/spec loading, so premise audits and stance output do not run from stale
or unloaded context.

### Phase 2: Define interview output and traversal behavior

Add concise output guidance for interview turns. The output should show:

- the user's proposal decomposed into intent branches;
- an objective interpretation of those branches;
- trade-offs, failure conditions, or missing information;
- the next macro-to-micro question when a decision branch remains ambiguous.

Define the autonomy signal behavior: when the user delegates remaining lower-level
judgment, the assistant records that branch as delegated, makes the local call,
and returns to the parent unresolved decision instead of continuing to ask
unbounded child-level questions.

Success criteria:
- A future maintainer can distinguish "ask another clarifying question" from
  "make the delegated lower-level decision and move back up".
- The guidance is short enough to survive skill-authoring pressure.

### Result (4b0070e) - 2026-05-10

Both skills now define an Intent Frame template with parsed claims/goals,
premise checks, objectified decision problem, considered and dropped options,
and stance. Interview Workflow tracks an implicit decision tree, asks one
highest-unresolved question at a time, treats delegated lower-level detail as an
autonomous branch closure, and returns to the nearest unresolved parent branch.

### Phase 3: Align docs and compatibility surfaces

Update the workflow-skills spec and mental model if the behavior changes the
caller-visible `lead-discuss` contract. Check whether the Claude compatibility
`discuss` skill should receive equivalent wording in Claude-native notation.

Success criteria:
- Codex-facing and Claude compatibility guidance intentionally agree or document
  the reason for divergence.
- Any updated skill or prompt text passes the skill-authoring invariant checklist.

### Result (4b0070e) - 2026-05-10

The workflow-skills spec now records the caller-visible intent-frame and
interview behavior under `260510-discuss-intent-frame-interview`. The
workflow-skills mental model records the trigger surface and the Interview
Workflow entry condition. A correctness/fit reviewer flagged ordering and
trigger-surface gaps; those findings were fixed and the re-review returned clean.
