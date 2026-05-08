---
name: edit
description: >
  Direct-edit primitive for narrow changes routed by /implement or explicit
  user request. The lead edits current branch, verifies, reviews once, updates
  specs, and reports.
argument-hint: "[ticket-path or inline brief]"
---

# Edit

Target: $ARGUMENTS

## Invariants

- The lead edits directly — no subagent delegation for the edit itself.
- Follow impl-playbook: run `ws-print-infra impl-playbook.md` for test strategy, verify, failure diagnosis, and mechanical-edit criteria.
- Load relevant mental-model docs before editing: run `ws-list-mental-model <target-paths>` and read every listed file.
- Ancestor loading: when a read touches `mental-model/<domain>/<sub>.md`, load `mental-model/<domain>/index.md` first.
- When skeleton exists for the target scope, its contracts and integration tests are the acceptance criteria.
- Commit per logical unit following CLAUDE.md commit rules; include `## AI Context`.
- Review relay cap: 2 cycles maximum; proceed to cleanup regardless of status after the cap.
- Escalate to `/implement` if scope grows to multi-file with new public API or cross-module without established pattern.
- Self-cleanup: review path file is deleted before returning.
- Run `ws:update-spec` with the edit's commit range before outputting the completion report.
- On completion, output the completion report in the format defined in Templates.

## On: invoke

### 1. Prepare

1. Parse arguments: ticket path or inline brief.
2. Record current HEAD as `<start-commit>`: `git rev-parse HEAD`.
3. If ticket-driven: read the ticket; collect existing skeleton references from frontmatter.
4. Treat collected skeleton references as acceptance criteria.
5. Load mental-model docs: `ws-list-mental-model <target-paths>`; read every listed file, ancestors first.
6. Run `ws-print-infra impl-playbook.md`.
7. Identify integration test file paths and the run command.

### 2. Edit

Edit files directly per the brief or ticket, following impl-playbook.md.
Commit at logical checkpoints per CLAUDE.md rules. Include `## AI Context`.

### 3. Verify

1. Run the test suite and build step. Read full output — never claim pass from a skimmed tail.
2. Resolve warnings per impl-playbook.md §Verify.
3. On failure: diagnose per impl-playbook.md §Test Failure Diagnosis. Do not patch tests to match broken impl.
4. Re-run until verify passes.

### 4. Review

Apply `judge: review-scope`. If lead-only review is selected, record the rationale and proceed to cleanup.

Register reviewer and allocate review path (two separate Bash calls).

Reviewer registration:

```bash
ws-new-named-agent reviewer -p code-reviewer -p code-review-correctness -p code-review-fit
```

```bash
ws-review-path direct
```

Store the returned path as `<review-path>`.

Spawn reviewer (`run_in_background: true`):

```bash
ws-call-named-agent reviewer - <<'PROMPT'
Diff range: <start-commit>..HEAD
Scope: direct-edit — <brief scope description>

Review for correctness and fit.
Review focus:
- <2-4 changed invariants, bug paths, or local fit risks>
Ignore broad style or unrelated architecture unless directly broken by the diff.
Write full findings to: <review-path>
Return only: [clean|non-clean]: <one-line summary>
PROMPT
```

After notification, read summary: `ws-print-named-agent-output reviewer`.

**If `[clean]`:** proceed to cleanup.

**If `[non-clean]`:** read the review file directly. Apply fixes. Re-verify tests. Re-call reviewer:

```bash
ws-call-named-agent reviewer - <<'PROMPT'
Re-review. Updated diff: <start-commit>..HEAD
Focus only on prior findings and touched follow-up changes.
PROMPT
```

Repeat until `[clean]` or after 2 relay cycles — then proceed to cleanup regardless.

### 5. Cleanup

```bash
rm -f <review-path>
```

### 6. Spec update

Invoke `ws:update-spec` via Skill tool with args `<start-commit>..HEAD`.

Output the **completion report** (see Templates).

## Judgments

### judge: review-scope

Soft judgment. Direct edit uses one reviewer by default.
Lead-only review is allowed for mechanical, low-risk edits with rationale.
When calling a reviewer, provide 2-4 focus bullets.

## Templates

### Completion report format

```
Edit complete.
Commit range: <start-commit>..HEAD
Test status: pass | fail | skipped
Review: clean | non-clean (<one-line summary>)
Spec: <N entries added, M 🚧 stripped> | no changes
<if issues remain after cap:> Open issues: <list>
```

## Doctrine

Edit optimizes for **session-context preservation during code changes** —
the lead retains accumulated understanding by editing directly rather than
forking to a subagent. The reviewer fires in a fresh named-agent context so
its judgment is uncommitted. The relay cap (2 cycles) keeps the loop bounded
so the lead's context is not consumed by negotiation. When a rule is ambiguous,
apply whichever interpretation keeps the lead's context continuous over the
change's full lifecycle.
