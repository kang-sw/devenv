---
name: lead-write-skeleton
description: Skeleton primitive for ready tickets whose public contracts must be locked before implementation.
---

# Write Skeleton

Target: user request

## Invariants

Scope
- Skeleton is the first code change for a ticket: interface stubs and integration tests only, no implementation code.
- Low-resolution source may be non-compiling only before populator handoff.
- Do not modify existing public interfaces unless the ticket explicitly mandates it.
- Operate on the current branch; caller owns branch creation and merge.
- Standalone invocation is allowed only when the caller already owns branch and merge lifecycle.

Ownership
- Lead owns public contract draft: research insertion points, write low-resolution source, commit draft, review, commit final skeleton.
- The populator owns source discovery and compile-clean normalization, not public contract design.
- The delegate does not commit - lead reviews and commits.

Contract Markers
- Draft markers are language-neutral comment text: `CONTRACT:`, `HINT:`, and `HOLE:`.
- `CONTRACT:` markers bind public names, visibility, module placement, call shape, and behavior targets.
- `HINT:` markers name approximate references the populator should research and normalize.
- `HOLE:` markers name intentionally unknown concrete types, imports, fixtures, helpers, or harnesses.

Commits
- Leave two source commits: one draft checkpoint and one final populated skeleton checkpoint.
- Record only the final skeleton commit hash in ticket `skeletons:` frontmatter.

Agents
- Register the skeleton-populator agent once per invocation via `ws/agents.register`; resume via `ws/agents.call` for amendment rounds.
- Register the skeleton-reviewer agent once per invocation via `ws/agents.register`; it is read-only.
- Skeleton review loop is lightweight: one reviewer, one amendment round, then stop and report if still non-clean.

## On: invoke

### 1. Identify Contract

1. Read the ticket. Note ambiguities requiring lead judgment: scope boundaries, design choices, and non-obvious integration constraints.
2. Skim relevant mental-model docs only if needed to resolve those ambiguities.
3. Call `ws/subquery(deep_research: true, question: "<candidate insertion points, adjacent conventions, public contracts, and integration test targets for this ticket>")`.
4. Read the result with `ws/agents.result(name: <subquery-key>, timeout_seconds: 600)`.
5. Choose insertion points, public shapes, and test target locations.

### 2. Draft Source

1. Write the first source edit directly as low-resolution source.
2. Add public API stubs, public fields, trait/function/method signatures, and module placement.
3. Add comment-only integration test targets that name the behavior or boundary to cover.
4. Add `CONTRACT:` comments for binding public shape and behavior targets.
5. Add `HINT:` comments for approximate type references, adjacent APIs, dependency direction, or "something like X" cues.
6. Add `HOLE:` comments for unknown concrete types, imports, fixtures, helpers, or test harness locations.
7. Do not add implementation logic.
8. Use invalid placeholder identifiers only when they reduce lead effort and leave clear `HINT:` or `HOLE:` markers for the populator.

### 3. Commit Draft

1. Commit only draft files as one logical checkpoint before population.
2. Commit message: `feat(<scope>): skeleton draft - <what contracts are sketched>`
3. Include `## AI Context` with key contract decisions and note that the commit is a draft checkpoint for population.
4. Store `<draft-commit>`.

### 4. Populate

1. Call `ws/agents.register(name: "skeleton-populator", prompts: ["skeleton-populator"])`.
2. Call `ws/agents.call(name: "skeleton-populator", prompt: <block below>)`:

```text
Ticket: <ticket-path>
Draft commit: <draft-commit>

## Lead-authored draft
- Files and locations changed by the lead draft.
- CONTRACT markers are binding; report conflicts with existing public interfaces.
- HINT markers are research cues to normalize.
- HOLE markers are open source-discovery tasks to fill when one clear project-local choice exists.
```

3. Read the result with `ws/agents.result(name: "skeleton-populator", timeout_seconds: 600)`.

### 5. Review

1. Call `ws/agents.register(name: "skeleton-reviewer", prompts: ["skeleton-reviewer"])`.
2. Call `ws/agents.call(name: "skeleton-reviewer", prompt: <block below>)`:

```text
Ticket: <ticket-path>
Lead draft files: <paths>
Populator report: <summary or result path>
Diff scope: <draft-commit>..working tree

Review only contract preservation, marker resolution, stub-only scope, and build/syntax evidence.
Return [clean|non-clean] plus findings.
```

3. Read the result with `ws/agents.result(name: "skeleton-reviewer", timeout_seconds: 600)`.
4. If review is `[clean]`, set `<final-review>` to `[clean]` and skip to step 11.
5. If review is `[non-clean]`, run one amendment round:
   - Contract issue: amend the lead draft markers or ask the user for judgment.
   - Population issue: relay amended markers or findings to `skeleton-populator`.
   - Implementation leakage: remove directly or send back for populator cleanup.
6. If amendment requires population cleanup, call `ws/agents.call(name: "skeleton-populator", prompt: <block below>)`:

```text
Ticket: <ticket-path>
Draft commit: <draft-commit>
Lead draft files: <paths>

Amend: <issues and revised CONTRACT/HINT/HOLE markers>
```

7. If `skeleton-populator` was called, read the amendment result with `ws/agents.result(name: "skeleton-populator", timeout_seconds: 600)`.
8. Re-call `skeleton-reviewer` once after amendment:

```text
Ticket: <ticket-path>
Lead draft files: <paths>
Populator amendment report: <summary, result path, or none>
Diff scope: <draft-commit>..working tree

Review only contract preservation, marker resolution, stub-only scope, and build/syntax evidence.
Return [clean|non-clean] plus findings.
```

9. Read the re-review result with `ws/agents.result(name: "skeleton-reviewer", timeout_seconds: 600)`.
10. Set `<final-review>` to the re-review result.
11. If `<final-review>` is `[non-clean]`, stop and report instead of continuing the relay.
12. If `<final-review>` is `[clean]`, run build to confirm compilation.
13. Do not run tests; tests fail by design because stubs are unimplemented.

### 6. Commit Skeleton

1. Commit populated stubs and tests together as one logical unit.
2. Commit message: `feat(<scope>): skeleton - <what contracts are established>`
3. Include `## AI Context` with key contract decisions.
4. Include `## Ticket Updates` with the ticket stem and what future phases must know.
5. Store `<final-skeleton-commit>`.

### 7. Record Ticket Skeleton

1. Update the ticket's `skeletons:` frontmatter in a separate ticket commit.
2. Use the phase and final skeleton commit hash, e.g. `phase-1: abc1234`.
3. Add entries only for phases that have a skeleton; no null placeholders.
4. Do not record the draft commit as a skeleton artifact.
5. Commit message: `docs(ticket): record skeleton hash`.
6. Store `<ticket-skeleton-commit>`.

### 8. Return

Output:

```text
Skeleton draft commit: <draft-commit>
Skeleton final commit: <final-skeleton-commit>
Ticket skeletons updated: <yes|no - reason>
Ticket skeleton commit: <ticket-skeleton-commit | none>
Verification: <build or syntax command and result>
Next: <caller-owned when invoked by ws:lead-implement | ws:lead-implement when invoked standalone>
```

When invoked standalone, report next route as `ws:lead-implement`. Do not
recommend direct-edit or delegated implementation directly from this skill.

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
