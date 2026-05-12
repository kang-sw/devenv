---
name: lead-write-skeleton
description: Skeleton primitive for ready tickets whose public contracts must be locked before implementation.
---

# Write Skeleton

Target: user request

## Invariants

- Skeleton = the first code change for a ticket. No implementation code - only interface stubs and integration tests.
- Lead owns public contract draft: research insertion points, write low-resolution source, commit draft, review, commit final skeleton.
- The populator owns source discovery and compile-clean normalization, not public contract design.
- Draft markers are language-neutral comment text: `CONTRACT:`, `HINT:`, and `HOLE:`.
- `CONTRACT:` markers bind public names, visibility, module placement, call shape, and behavior targets.
- `HINT:` markers name approximate references the populator should research and normalize.
- `HOLE:` markers name intentionally unknown concrete types, imports, fixtures, helpers, or harnesses.
- Low-resolution source may be non-compiling only before populator handoff.
- Do not modify existing public interfaces unless the ticket explicitly mandates it.
- The delegate does not commit - lead reviews and commits.
- Operate on the current branch; caller owns branch creation and merge.
- Standalone invocation is allowed only when the caller already owns branch and merge lifecycle.
- Leave two source commits: one draft checkpoint and one final populated skeleton checkpoint.
- Register the skeleton-populator agent once per invocation via `ws/agents.register`; resume via `ws/agents.call` for amendment rounds.
- Register the skeleton-reviewer agent once per invocation via `ws/agents.register`; it is read-only.
- Skeleton review loop is lightweight: one reviewer, one amendment round, then stop and report if still non-clean.

## On: invoke

### 1. Identify contract

1. Read the ticket. Note ambiguities requiring lead judgment: scope boundaries, design choices, and non-obvious integration constraints.
2. Skim relevant mental-model docs only if needed to resolve those ambiguities.
3. Call `ws/subquery(deep_research: true, question: "<candidate insertion points, adjacent conventions, public contracts, and integration test targets for this ticket>")`.
4. Read the result with `ws/agents.result(name: <subquery-key>, timeout_seconds: 600)`.
5. Choose insertion points, public shapes, and test target locations.

### 2. Draft

Write the first source edit directly as low-resolution source:

- Public API stubs, public fields, trait/function/method signatures, and module placement.
- Comment-only integration test targets that name the behavior or boundary to cover.
- `CONTRACT:` comments for binding public shape and behavior targets.
- `HINT:` comments for approximate type references, adjacent APIs, dependency direction, or "something like X" cues.
- `HOLE:` comments for unknown concrete types, imports, fixtures, helpers, or test harness locations.

Do not add implementation logic. Placeholder identifiers may be invalid when they
reduce lead effort and leave clear `HINT:` or `HOLE:` markers for the populator.

Commit the lead draft before population:

1. Commit only draft files as one logical checkpoint.
2. Commit message: `feat(<scope>): skeleton draft - <what contracts are sketched>`
3. Include `## AI Context` with key contract decisions and note that the commit is a draft checkpoint for population.
4. Store `<draft-commit>`.

### 3. Populate

Call `ws/agents.register(name: "skeleton-populator", prompts: ["skeleton-populator"])`.

Call `ws/agents.call(name: "skeleton-populator", prompt: <block below>)`:

```text
Ticket: <ticket-path>
Draft commit: <draft-commit>

## Lead-authored draft
- Files and locations changed by the lead draft.
- CONTRACT markers are binding; report conflicts with existing public interfaces.
- HINT markers are research cues to normalize.
- HOLE markers are open source-discovery tasks to fill when one clear project-local choice exists.
```

Read the result with `ws/agents.result(name: "skeleton-populator", timeout_seconds: 600)`.

### 4. Review

Call `ws/agents.register(name: "skeleton-reviewer", prompts: ["skeleton-reviewer"])`.

Call `ws/agents.call(name: "skeleton-reviewer", prompt: <block below>)`:

```text
Ticket: <ticket-path>
Lead draft files: <paths>
Populator report: <summary or result path>
Diff scope: <draft-commit>..working tree

Review only contract preservation, marker resolution, stub-only scope, and build/syntax evidence.
Return [clean|non-clean] plus findings.
```

Read the result with `ws/agents.result(name: "skeleton-reviewer", timeout_seconds: 600)`.

If review is `[clean]`, run build to confirm compilation. Do not run tests -
tests will fail by design because stubs are unimplemented. Passing tests is the
implementor's responsibility, not the skeleton's.

If review is `[non-clean]`, run one amendment round:
- **Contract issue** - amend the lead draft markers or ask the user for judgment.
- **Population issue** - relay amended markers or findings with `ws/agents.call(name: "skeleton-populator", prompt: <block below>)`:
  ```text
  Amend: <issues and revised CONTRACT/HINT/HOLE markers>
  ```
- **Implementation leakage** - remove directly or send back for populator cleanup.

After the amendment, re-call `skeleton-reviewer` once. If still non-clean, stop
and report instead of continuing the relay.

### 5. Commit

1. Commit populated stubs and tests together as one logical unit.
2. Commit message: `feat(<scope>): skeleton - <what contracts are established>`
3. Include `## AI Context` with key contract decisions.
4. Include `## Ticket Updates` with the ticket stem and what future phases must know.
5. Store `<final-skeleton-commit>`.
6. Update the ticket's `skeletons:` frontmatter in a separate ticket commit with the phase and final skeleton commit hash (e.g., `phase-1: abc1234`). Only add entries for phases that have a skeleton - no null placeholders. Do not record the draft commit as a skeleton artifact.
7. Commit message: `docs(ticket): record skeleton hash`.
8. Store `<ticket-skeleton-commit>`.

### 6. Return

Return:

```text
Skeleton draft commit: <draft-commit>
Skeleton final commit: <final-skeleton-commit>
Ticket skeletons updated: <yes|no - reason>
Ticket skeleton commit: <ticket-skeleton-commit | none>
Verification: <build or syntax command and result>
Next: <caller-owned when invoked by ws:lead-implement | ws:lead-implement when invoked standalone>
```

When invoked standalone, recommend `ws:lead-implement` with the same target and
"skeleton exists" context. Do not recommend `ws:lead-edit` or
`ws:lead-write-code` directly from this skill.

## Judgments

### judge: test-scope

| Layer | Default | Condition |
|---|---|---|
| Structural seam tests | Always | Every cross-module boundary |
| Behavioral tests | Include when ticket specifies behavior | Any behavior the ticket describes - drop the "complex" qualifier |
| Error / edge case tests | Opt-in | Only when the ticket explicitly specifies error contracts or edge conditions |

### judge: stub-granularity

| Level | When |
|-------|------|
| Module-level (types + top-level functions) | Most cases |
| Method-level (all public methods stubbed) | When the ticket specifies detailed API surface |

## Doctrine

The skeleton optimizes for **contract visibility before implementation**. The
lead spends context on low-resolution public shape; the populator spends context
on source discovery and build cleanup. When ambiguous, preserve contract
stability while minimizing lead serialization.
