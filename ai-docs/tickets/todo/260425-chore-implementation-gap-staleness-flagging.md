---
title: Add Implementation Gap staleness flagging to survey and implement skills
spec:
  - 260421-implementation-gap-callout
related-mental-model:
  - spec-system
  - workflow-routing
---

# Add Implementation Gap staleness flagging to survey and implement skills

## Background

The spec-system now defines two forms of Implementation Gap callout: missing behavior
(intended but not yet built) and unexposed capability (built but not caller-exposed).
Both are known-but-unscheduled gaps with no ticket.

The spec-conventions guideline now reads: "A survey agent or implementer encountering
a callout whose referenced item is not found in code should flag it as potentially
stale in their report."

Without explicit instruction in skill docs, agents silently ignore stale callouts —
leaving false positives that could mislead future survey passes into treating
non-existent capabilities as planned features or known gaps.

## Decisions

**Automation rejected**: staleness detection via automation (stems on callouts,
spec-updater stripping, cron review) was considered and rejected. Intent-based
classification — "is this item planned for exposure, or intentionally private?" —
cannot be inferred from code signals or commit history. Any automated mechanism
requires developer discipline to reference stems in commits; projects with that
discipline already create tickets at registration time, making the gap rare.

**Piggyback on existing workflow**: flagging staleness at the moment agents already
read spec adds no new tooling or markers. The error-mode asymmetry justifies this:
a stale callout that triggers a search is better than no callout, which produces
silent duplication.

**Scope**: guideline-text additions to write-plan and implement/edit skill docs only.
No spec format changes, no new markers, no stem additions to callouts.

## Phases

### Phase 1: write-plan survey step

In the survey step of the write-plan skill: when scanning spec files for context,
if an Implementation Gap callout references an item that cannot be found in the
codebase, include a note in the survey report:

> Implementation Gap callout for `<item>` — referenced item not found in code;
> potentially stale.

The flag is informational — it must not block or redirect the survey.

Success: a write-plan survey run against a spec containing a stale Implementation
Gap callout produces a report that explicitly names it as potentially stale.

### Phase 2: implement and edit pre-read step

In the implement and edit skills, whichever has a "read spec before implementation"
step: same instruction — flag stale Implementation Gap callouts encountered during
spec read, before implementation begins.

Success: an implement or edit run against a spec with a stale callout surfaces the
flag in the pre-read output before any code change is made.
