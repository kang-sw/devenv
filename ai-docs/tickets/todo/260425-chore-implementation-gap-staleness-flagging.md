---
title: Add doc-staleness reactive reporting to agent prompts
spec:
  - 260421-implementation-gap-callout
related-mental-model:
  - spec-system
  - workflow-routing
---

# Add doc-staleness reactive reporting to agent prompts

## Background

The spec-system defines two forms of Implementation Gap callout: missing behavior
(intended but not yet built) and unexposed capability (built but not caller-exposed).
A generic staleness-reporting guideline was added to spec-conventions.md and
spec-system.md during the initial design pass.

That guideline is wrong in two ways:

1. **Wrong audience**: spec-conventions.md is writer-facing, not agent-facing.
   Agents do not read it.
2. **Wrong framing**: a proactive "scan callouts and cross-reference code" instruction
   allocates too much overhead for a small gap. The two callout forms also have
   opposite staleness semantics — "missing behavior" has expected absence from code;
   "unexposed capability" has expected presence — so a single symmetric rule misfires
   on one form or the other.

The correct framing is reactive: if a spec or doc entry leads an agent to a wrong
assumption that is only discovered by checking code, the agent should report the
discrepancy. This fires exactly when drift causes real cost, adds one line per agent
doc, and covers all doc types, not just Implementation Gap callouts.

## Decisions

**Proactive scan rejected**: form-specific search logic (scan callout → parse item →
search code → compare) is disproportionate overhead for a gap that surfaces rarely
and only when an agent actually touches the relevant area.

**Reactive reporting chosen**: the instruction fires at the moment of actual
discrepancy during normal work — no additional search loop, no callout parsing,
no form-specific branching.

**Spec-conventions removal**: the misplaced generic guideline is removed from both
spec-conventions.md and spec-system.md. Agent-facing rules live in agent prompts.

**Correctness reviewer included**: the correctness partition checks spec-vs-diff
alignment and is the natural home for doc-staleness reporting on the review side.
Fit and test partitions are out of scope.

## Phases

### Phase 1: Remove misplaced guideline

Remove the staleness-reporting sentence from:

- `claude/infra/spec-conventions.md` — the bullet: "A survey agent or implementer
  encountering a callout whose referenced item is not found in code should flag it
  as potentially stale in their report."
- `ai-docs/spec/spec-system.md` — the corresponding sentence in the Implementation
  Gap Callout section.

Success: neither file contains any instruction directed at agents about staleness
reporting.

### Phase 2: Add reactive one-liner to agent prompts

Add one sentence to the report/output section of each of the following:

- `claude/infra/impl-playbook.md`
- `claude/skills/write-plan/survey-writer.md`
- `claude/skills/write-plan/plan-writer.md`
- `claude/infra/code-review-correctness.md`

Wording (adapt to fit surrounding prose):

> If a spec or doc entry led to a wrong assumption that you only discovered by
> checking code, include the discrepancy in your report.

The instruction is informational — it must not block or redirect the agent's
primary task.

Success: each of the four files contains a one-line reactive reporting instruction
in its output/report section.
