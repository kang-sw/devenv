---
title: "write-ticket: add document-reviewer invocation after intent review"
---

# write-ticket: add document-reviewer invocation after intent review

## Background

write-ticket's intent review (step 5) is an author self-check: did the ticket
capture the conversation's decisions and alternatives? It cannot catch
architectural drift, spec conflicts, or reuse gaps because it holds the same
context as the ticket author.

A fresh-eye document-reviewer invocation after drafting closes this gap.
document-reviewer already accepts tickets as valid input and its existing
process (mental-model load, spec read, drift/reuse/realism checks, fast-exit)
is exactly right — no changes to document-reviewer are needed.

## Decisions

- **Mandatory review, fast-exit** — not conditional on ticket complexity. The
  reviewer's "No issues found." path is cheap; skipping review on "simple"
  tickets is the failure mode this change prevents.
- **Embedded in write-ticket, not a separate /review-ticket skill** — review
  is always part of ticket authoring, not an optional follow-up step.
- **document-reviewer unchanged** — it already handles tickets correctly as-is.
  Subquery is not suppressed; conceptual realism checks may legitimately need it.

## Rejected alternatives

- Hard judge on `spec:` field or `feat`/`arch` stem — misses refactors, which
  often carry significant architectural implications without touching specs.
- Separate `/review-ticket` skill — unnecessary layer; adds user friction.
- New mode in document-reviewer — the existing behavior is already correct for
  ticket review.
- New `claude/infra/ticket-reviewer.md` script — no new file needed.

## Phases

### Phase 1: Edit write-ticket/SKILL.md

Insert a new step 6 in `On: invoke` between the current steps 5 (intent review)
and 6 (spec-stem check):

```
6. Review — `judge: review-model`. Spawn document-reviewer on the ticket.
   Present findings. If Critical or Important: fix in-place and re-review.
   Proceed when clean.
```

Renumber existing steps 6 (spec-stem check) → 7 and 7 (spec write prompt) → 8.

Add to `## Judgments`:

```
### judge: review-model

Opus by default.
Sonnet only when the ticket is single-phase, no design decisions,
and purely mechanical (typo, config-only, doc-only update).
```

Success: write-ticket/SKILL.md has the new step and judgment; all step
references within the file are consistent.
