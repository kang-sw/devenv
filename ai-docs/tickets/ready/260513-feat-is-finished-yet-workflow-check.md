---
title: Is finished yet workflow check
parent: 260513-epic-workflow-question-loop-hygiene
spec:
  - 260513-is-finished-yet-skill
  - 260513-proceed-ticket-freshness-gate
related-mental-model:
  - workflow-skills
  - documentation-system
---

# Is finished yet workflow check

## Background

The user frequently asks a question shaped like: "from a code hygiene
perspective, excluding things the agent can decide autonomously, what design
open questions remain?" This is different from `lead-verify-discussion`.
`lead-verify-discussion` validates assumptions and direction; this workflow
needs a fast finish check that classifies what still needs user or design
resolution before proceeding.

The same conversation pattern exposes a `lead-proceed` gap. A user may discuss a
ticket, write or update it, continue discussing details, and then invoke
`proceed`. The current routing can start from the ticket artifact without an
explicit freshness gate for discussion decisions that happened after the ticket
write.

## Decisions

- Add a short, frequently callable skill with an oral, verb-like invocation name.
  `lead-is-finished-yet` is the working candidate.
- Keep the skill body extremely short, similar to `lead-verify-discussion`.
- The skill must not edit files. It reports remaining user-blocking design
  questions, ticket/spec capture gaps, autonomous hygiene items, and proceed
  readiness.
- Add a `lead-proceed` freshness gate for warm discussions with an existing
  related ticket.
- The freshness gate gathers missing ticket context only from the active
  conversation and the ticket artifact. It should not read source code or broad
  extra documentation.

## Phases

### Phase 1: Add the spoken finish-check skill

Create a short workflow skill for the recurring "is this finished yet?" design
question.

Acceptance criteria:

- The skill name is spoken and verb-like enough for frequent direct use.
- The skill body is compact and checkpoint-shaped, not a full workflow manual.
- The report distinguishes user-blocking design decisions from autonomous code
  hygiene or implementation-detail work.
- The skill is documented in workflow skill inventory, specs, mental models,
  and wsflow mirroring guidance if applicable.

### Phase 2: Refresh related tickets before proceed

Update `lead-proceed` so warm discussion state can refresh an existing related
ticket before implementation routing.

Acceptance criteria:

- When the active discussion has a related ticket and settled decisions may be
  missing from that ticket, `lead-proceed` routes through `lead-write-ticket`
  edit before selecting the implementation slice.
- The gate uses conversation state and ticket artifacts only.
- The gate does not rejudge ticket decomposition, inspect source code, or
  perform implementation planning.
- After ticket refresh, `lead-proceed` re-reads the ticket and continues through
  the existing spec -> ticket -> implementation pipeline.
