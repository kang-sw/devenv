---
title: "write-ticket: fix stale /write-spec suggestion on discuss→write-spec→write-ticket chain"
---

# write-ticket: fix stale /write-spec suggestion on discuss→write-spec→write-ticket chain

## Background

The canonical workflow chain changed to `discuss → write-spec → write-ticket`, but
write-ticket/SKILL.md and the workflow-skills.md spec entry still describe the old
chain where write-ticket suggests /write-spec as its next step.

Drifted locations:

- `claude/skills/write-ticket/SKILL.md` step 7: "Spec write prompt — always suggest
  `/write-spec` as the next step after ticket authoring; write-spec's judge handles
  relevance." — stale; write-spec has already run before write-ticket on the new chain.
- `ai-docs/spec/workflow-skills.md` `/write-ticket` entry: "Always suggests `/write-spec`
  after authoring." — same drift.
- The canonical chain diagram in workflow-skills.md may also need updating.

## Decisions

- Fix write-ticket/SKILL.md step 7 to suggest the correct next step on the new chain.
  Likely `/proceed` or `/write-skeleton`, but confirm against the proceed/discuss skills
  before editing.
- Fix the workflow-skills.md spec entry to reflect the correct post-write-ticket behavior.
- Fix the canonical chain diagram if it still shows write-spec branching from discuss
  as optional rather than as the first mandatory step after discuss.

## Rejected alternatives

- Fixing as part of 260422-feat-write-ticket-review — deferred; the review feature
  depends on understanding the correct chain first.

## Phases

### Phase 1: Audit and fix write-ticket/SKILL.md + workflow-skills.md

1. Read `claude/skills/discuss/SKILL.md` to confirm the canonical chain
   (discuss → write-spec → write-ticket) and what write-ticket should suggest next.
2. Read `claude/skills/proceed/SKILL.md` to confirm whether proceed is the canonical
   next step after write-ticket.
3. Update write-ticket/SKILL.md: replace step 7 "Spec write prompt" with the correct
   next-step suggestion. Also assess step 6 (spec-stem check) — confirm whether it is
   still correct when write-spec has already run before write-ticket.
4. Update workflow-skills.md `/write-ticket` entry to remove the stale /write-spec
   suggestion and reflect the correct next step.
5. Update the canonical chain diagram in workflow-skills.md if it is stale.
6. Run `spec-build-index ai-docs/spec/workflow-skills.md`.
7. Commit.

Success: write-ticket/SKILL.md and workflow-skills.md accurately describe the
discuss→write-spec→write-ticket chain with no stale /write-spec suggestion.
