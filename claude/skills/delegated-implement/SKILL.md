---
name: delegated-implement
description: >
  Delegate a single implementation cycle to an implementer + reviewer pair.
  Use for Tier 3 work (internal implementation behind locked contracts).
  Dispatch multiple instances in parallel for concurrent work.
argument-hint: "<plan-path or inline brief> [--ticket <ticket-stem>]"
---

# Delegated Implementation

Target: $ARGUMENTS

## Invariants

- This skill delegates — the lead does not read source code or write implementation.
- Skeleton stubs and integration tests are the acceptance criteria. If they don't exist, stop and suggest `/write-skeleton` first.
- The implementer and reviewer communicate directly; the lead receives only final reports.
- One delegation cycle per invocation. For parallel work, dispatch multiple instances.
- Follow CLAUDE.md commit rules for the merge commit (including `## Ticket Updates` when ticket-driven).

## On: invoke

### 1. Prepare

1. Parse arguments: extract plan path or inline brief, and optional ticket stem.
2. If plan-driven: verify the plan file exists. Read it to extract scope and branch name hint.
3. If brief-driven: the brief is the full specification.
4. Verify skeleton exists: grep for `todo!()`/`unimplemented`/`NotImplementedError` stubs or check for integration tests that reference the target contracts. If absent, stop and suggest `/write-skeleton`.
5. Record current branch as `<original-branch>`. Create `implement/<scope>` branch.

### 2. Spawn implementer

```
Agent(
  name = "implementer",
  model = "sonnet",
  prompt = """
    Before starting, read `~/.claude/infra/agents/_common.md` then
    `~/.claude/infra/agents/implementer.md`.

    Lead name: <lead-name>
    Mode: <A: plan-driven | B: inline brief>
    <Plan path | Brief text>

    Acceptance criteria: skeleton integration tests must pass.
  """
)
```

Wait for the implementer's completion report. Note the commit range.

### 3. Spawn reviewer

```
Agent(
  name = "reviewer",
  model = "sonnet",
  prompt = """
    Before starting, read `~/.claude/infra/agents/_common.md` then
    `~/.claude/infra/agents/reviewer.md`.

    Lead name: <lead-name>
    Implementer name: implementer
    Diff range: <first-commit>..<last-commit>
  """
)
```

The reviewer and implementer iterate directly until the reviewer
reports clean. Wait for the reviewer's final report to the lead.

### 4. Merge and report

1. Verify all integration tests pass on the implementation branch.
2. Merge back to `<original-branch>` with a summary commit per CLAUDE.md commit rules.
3. Report to the user:
   - What was implemented (from implementer report)
   - Review result (from reviewer report)
   - Test status
   - Any deviations or open items

## Judgments

### judge: skeleton-check

| Decision | When |
|----------|------|
| Proceed without skeleton | Brief is a small, isolated change (single file, no public contracts) |
| Require skeleton | Change touches public interfaces or cross-module boundaries |

## Doctrine

Delegated implementation optimizes for **contract-bounded autonomy** —
the implementer has full freedom within skeleton-locked contracts, and
the reviewer validates without lead involvement. When a rule is
ambiguous, apply whichever interpretation better preserves the
implementer's autonomy within contract boundaries.
