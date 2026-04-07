---
name: implement
description: >
  When the user provides a ticket, plan, or description for structured
  implementation, invoke this. Covers both plan-driven execution and
  ad-hoc implementation.
argument-hint: [ticket-path or description]
---

# Implementation Workflow

Target: $ARGUMENTS

## Invariants

- Read `~/.claude/skills/infra/impl-playbook.md` and `~/.claude/skills/infra/impl-process.md` before starting.
- Follow CLAUDE.md code standards in all implementation work.
- Before touching a module, verify its contracts and invariants in mental-model docs; prefer documented extension points over new abstractions.
- When a plan is loaded in context, it is the spec — follow its contracts, do not re-research or second-guess. Deviate only per playbook §Deviation Protocol.
- Claim "pass" only after reading full test/build output — never "should pass" or "looks correct."
- Do not include design rationale in code-review prompts — the reviewer evaluates independently.
- Commit freely on the feature branch; the merge commit carries the final summary.
- User approves the report before doc-update tasks proceed.
- Dismiss false-positive review issues with a brief rationale — do not apply unnecessary fixes.

## On: invoke

### 1. Understand

1. Read the ticket/description/plan.
2. Read `ai-docs/mental-model/overview.md`; read every mental-model doc touching the change area and adjacent domains. If none exist, note for docs task.
3. **Plan-driven**: if a plan has been loaded, its contracts and step ordering are authoritative. Derive tasks from plan steps, preserving testing classifications (TDD/post-impl/manual).
4. **Ad-hoc**: research the change area. Before designing new components, search for reusable existing utilities or patterns.
5. Record current branch as `<original-branch>`. If already on an `implement/` branch, treat as resumed session — infer `<original-branch>` from merge-base with `main`, skip branch creation, continue from existing task list. Otherwise create `implement/<scope>` from current branch.

### 2. Create task list

Create tasks per process §Task List. Bookend tasks (marked `[fixed]`) are mandatory. Fill implementation tasks between them. State assumptions and success criteria before the first implementation task.

### 3. Execute tasks

Work through tasks sequentially per playbook (§Test Strategy, §Verify, §Deviation Protocol, §Mechanical-Edit Criteria). When tests fail, follow playbook §Test Failure Diagnosis.

For orchestration tasks (code review, doc pipeline, report, merge), follow process.

## Judgments

### judge: approval-gate

- **Auto-proceed:** bug fixes, pattern-following additions, tests, refactoring.
- **Ask first:** new components/protocols, architectural changes, cross-module interfaces.
- **Always ask:** deleting functionality, changing API semantics, schema changes.

## Doctrine

Implementation correctness depends on **verified task closure** — every
task runs through build, test, and review before the branch merges. When
a rule is ambiguous, apply whichever interpretation better preserves
verified closure of each task in the sequence.
