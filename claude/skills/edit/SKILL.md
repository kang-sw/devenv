---
name: edit
description: >
  Direct-edit primitive. The lead reads, edits, verifies, and commits on the
  current branch. One named-agent reviewer covers correctness and fit.
  No brief, no delegation, no doc pipeline — callers own those.
argument-hint: "[ticket-path or inline brief]"
---

# Edit

Target: $ARGUMENTS

## Invariants

- The lead edits directly — no subagent delegation for the edit itself.
- Follow impl-playbook: run `ws-print-infra impl-playbook.md` for test strategy, verify, failure diagnosis, and mechanical-edit criteria.
- Load relevant mental-model docs before editing: run `ws-list-mental-model <target-paths>` and read every listed file.
- Ancestor loading: when a read touches `mental-model/<domain>/<sub>.md`, load `mental-model/<domain>/index.md` first.
- When skeleton exists for the target scope, its stubs and integration tests are the acceptance criteria.
- Commit per logical unit following CLAUDE.md commit rules; include `## AI Context`.
- Escalate to `/implement` if scope grows to multi-file with new public API or cross-module without established pattern.
- Self-cleanup: review path file is deleted before returning.
- On completion, output commit range and test status in the format defined in Templates.

## On: invoke

### 1. Prepare

1. Parse arguments: ticket path or inline brief.
2. If ticket-driven: read the ticket; collect skeleton references from frontmatter.
3. Apply `judge: skeleton-check`. If skeleton required but absent, stop and suggest `/write-skeleton`.
4. Load mental-model docs: `ws-list-mental-model <target-paths>`; read every listed file, ancestors first.
5. Run `ws-print-infra impl-playbook.md`.
6. Identify integration test file paths and the run command.

### 2. Edit

Edit files directly per the brief or ticket, following impl-playbook.md.
Commit at logical checkpoints per CLAUDE.md rules. Include `## AI Context`.

### 3. Verify

1. Run the test suite and build step. Read full output — never claim pass from a skimmed tail.
2. Resolve warnings per impl-playbook.md §Verify.
3. On failure: diagnose per impl-playbook.md §Test Failure Diagnosis. Do not patch tests to match broken impl.
4. Re-run until verify passes.

### 4. Review

Register reviewer and allocate review path (two separate Bash calls):

```bash
ws-new-named-agent reviewer --agent ws:code-reviewer \
  --system-prompt "$(ws-infra-path code-review-correctness.md)
$(ws-infra-path code-review-fit.md)"
```

```bash
ws-review-path direct
```

Store the returned path as `<review-path>`.

Spawn reviewer (`run_in_background: true`):

```bash
ws-call-named-agent reviewer - <<'PROMPT'
Diff range: <first-commit>..HEAD
Scope: direct-edit — <brief scope description>

Review for correctness and fit.
Write full findings to: <review-path>
Return only: [clean|non-clean]: <one-line summary>
PROMPT
```

After notification, read summary: `ws-print-named-agent-output reviewer`.

**If `[clean]`:** proceed to cleanup.

**If `[non-clean]`:** read the review file directly. Apply fixes. Re-verify tests. Re-call reviewer:

```bash
ws-call-named-agent reviewer - <<'PROMPT'
Re-review. Updated diff: <first-commit>..HEAD
PROMPT
```

Repeat until `[clean]` or after 2 relay cycles — then proceed to cleanup regardless (surface remaining issues in output).

### 5. Cleanup

```bash
rm -f <review-path>
```

Output the **completion report** (see Templates).

## Judgments

### judge: skeleton-check

| Decision | When |
|----------|------|
| Proceed without skeleton | Change is a small isolated edit (single file, no new public contracts) |
| Require skeleton | Change touches public interfaces or cross-module boundaries |

## Templates

### Completion report format

```
Edit complete.
Commit range: <first>..HEAD
Test status: pass | fail | skipped
<if issues remain:> Open issues: <list>
```

## Doctrine

Edit optimizes for **session-context preservation during code changes** —
the lead retains accumulated understanding by editing directly rather than
forking to a subagent. The reviewer fires in a fresh named-agent context so
its judgment is uncommitted, but the relay loop is kept short (2 cycles max)
because the lead can self-apply fixes without won't-fix negotiation. When a
rule is ambiguous, apply whichever interpretation keeps the lead's context
continuous over the change's full lifecycle.
