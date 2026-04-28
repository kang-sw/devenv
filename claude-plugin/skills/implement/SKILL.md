---
name: implement
description: >
  Implementation harness. Routes to direct-edit or delegated write-code,
  then runs the shared doc pipeline, report/approval, and merge.
argument-hint: "<ticket-path or inline description> [--ticket <stem>]"
---

# Implement

Target: $ARGUMENTS

## Invariants

- This skill harnesses — it routes and merges; it does not implement or review code itself.
- User approves the report before merge — no code reaches the target branch without user confirmation.
- Follow CLAUDE.md commit rules for the merge commit (including `## Ticket Updates` when ticket-driven).
- Task list is created at prepare and tracked to completion — no task may be skipped or reordered.

## On: invoke

### 1. Assess

Parse `$ARGUMENTS`: extract ticket path or inline description.
If ticket-driven: read the ticket. Extract scope, stem, and existing artifact references.

Apply `judge: execution-mode`.

### 2. Prepare

1. Record current branch as `<original-branch>`.
2. Create task list. All tasks are mandatory — do not skip or reorder.
   ```
   [ ] Execute — invoke ws:edit or ws:write-code; capture commit range
   [ ] Doc pre-pass — update-spec then mental-model-updater; commit each
   [ ] Report to user — wait for approval; loop on tweaks
   [ ] Merge to original branch (delegated path only)
   [ ] Doc commit gate — executor-wrapup §Doc Commit Gate + §Ticket Update
   [ ] Update project docs — refresh ai-docs/_index.md, ticket status
   ```

### 3. Execute

**Direct-edit:** Invoke `ws:edit` via the Skill tool with the target as arguments.

**Delegated:** Create `implement/<scope>` branch. Invoke `ws:write-code` via the Skill tool with the target as arguments.

Capture the commit range from the skill's completion report.

### 4. Doc pre-pass

1. Invoke **ws:update-spec** via Skill tool with args `<commit-range>`. (Lead-driven; runs inline.)
2. Register and call `mental-model-updater` (run_in_background: true; timeout: 600000):
   ```bash
   ws-new-named-agent mental-model-updater -p mental-model-updater
   ws-call-named-agent mental-model-updater "Commit range: <commit-range>"
   ```
   Wait for completion. Commit any file changes. (Runs after update-spec so it sees any 🚧 strips.)

### 5. Report and approval

Report to the user:
- What was implemented (from edit/write-code completion report)
- Review result (from edit's `Review:` line or write-code's reviewer summaries)
- Test status
- Deviations or open items
- If write-code escalated at cycle 3: list each unresolved dispute — user decides fix or accept

Wait for user approval. If tweaks requested:
- For direct-edit: apply fixes directly and re-verify.
- For delegated: relay to implementer via `ws-call-named-agent implementer`; re-review via
  `ws-call-named-agent reviewer-correctness` / `reviewer-fit` / `reviewer-test` per the write-code
  reviewer prompt pattern. Note: this is a post-approval tweak cycle separate from write-code's
  3-cycle cap; cycle counter resets.
- Re-invoke ws:update-spec with the new commit range; re-dispatch mental-model-updater; commit each.
- Re-report. Loop until approved.

### 6. Merge (delegated path only)

Run `ws-merge-branch <original-branch> <branch> "<commit-message>"`.
The script selects strategy by commit count: squash (1 commit) or --no-ff (2+).
Compose the commit message per CLAUDE.md commit rules.

### 7. Doc commit gate

Run `ws-print-infra executor-wrapup.md`. Follow §Doc Commit Gate and (if ticket-driven) §Ticket Update.
Do not re-run §Doc Pipeline — update-spec and mental-model-updater already ran in step 4.

### 8. Update project docs

Refresh `ai-docs/_index.md` if new skills, agents, or major patterns were introduced.
Update ticket status if ticket-driven.

## Judgments

### judge: execution-mode

| Decision | When |
|----------|------|
| Direct edit → `ws:edit` | Change is confined to a single file AND purely internal (no callers affected, no new public symbols, no new test files) AND user has not explicitly requested delegation |
| Delegated → `ws:write-code` | Any condition above is unmet — cross-file touch, new public contract, new test file, or explicit delegation requested |

## Doctrine

Implement optimizes for **verified code reaching the target branch** — routing,
doc pipeline, and merge are the harness concerns; code quality is owned by
write-code and edit. When a rule is ambiguous, apply whichever interpretation
keeps harness logic out of the primitives and primitive logic out of the harness.
