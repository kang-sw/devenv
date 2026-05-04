---
name: lead-edit
description: Directly implement a narrow code change on the current branch, then verify it with one correctness-and-fit reviewer before updating specs and reporting completion.
---

# Edit

Target: user request

## Invariants

- The lead edits directly - no subagent delegation for the edit itself.
- Follow impl-playbook: call `ws/infra.read(name: "impl-playbook")` for test strategy, verify, failure diagnosis, and mechanical-edit criteria.
- Load relevant mental-model docs before editing: call `ws/mental_models.list(paths: <target-paths>)` and read every listed file.
- Ancestor loading: when a read touches `mental-model/<domain>/<sub>.md`, load `mental-model/<domain>/index.md` first.
- When skeleton exists for the target scope, its stubs and integration tests are the acceptance criteria.
- Commit per logical unit following CLAUDE.md commit rules; include `## AI Context`.
- Review relay cap: 2 cycles maximum; proceed to cleanup regardless of status after the cap.
- Lead adjudicates review findings: fix correctness, security, and contracts; reject style or scope expansion when appropriate.
- Escalate to `ws:lead-write-code` if scope grows to multi-file with new public API or cross-module without established pattern.
- Self-cleanup: review path file is deleted before returning.
- Run `ws:lead-update-spec` with the edit's commit range before outputting the completion report.
- On completion, output the completion report in the format defined in Templates.

## On: invoke

### 1. Prepare

1. Parse arguments: ticket path or inline brief.
2. Record current HEAD as `<start-commit>`: `git rev-parse HEAD`.
3. If ticket-driven: read the ticket; collect skeleton references from frontmatter.
4. Apply `judge: skeleton-check`. If skeleton required but absent, stop and suggest `ws:lead-write-skeleton`.
5. Call `ws/mental_models.list(paths: <target-paths>)`; read every listed file, ancestors first.
6. Call `ws/infra.read(name: "impl-playbook")`.
7. Identify integration test file paths and the run command.

### 2. Edit

Edit files directly per the brief or ticket, following impl-playbook.md.
Commit at logical checkpoints per CLAUDE.md rules. Include `## AI Context`.

### 3. Verify

1. Run the test suite and build step. Read full output - never claim pass from a skimmed tail.
2. Resolve warnings per impl-playbook.md section Verify.
3. On failure: diagnose per impl-playbook.md section Test Failure Diagnosis. Do not patch tests to match broken impl.
4. Re-run until verify passes.

### 4. Review

Call `ws/agents.register(name: "reviewer", prompts: ["code-reviewer", "code-review-correctness", "code-review-fit"])`.
Call `ws/path.generate(kind: "review", stems: ["direct"])`; store the returned path as `<review-path>`.

Call `ws/agents.call(name: "reviewer", prompt: <block below>)`:

```text
Diff range: <start-commit>..HEAD
Scope: direct-edit - <brief scope description>

Review for correctness and fit.
Write full findings to: <review-path>
Return only: [clean|non-clean]: <one-line summary>
```

After notification, read the reviewer summary from `ws/agents.print(name: "reviewer")` only if the async result did not include a usable summary.

**If `[clean]`:** proceed to cleanup.

**If `[non-clean]`:** read the review file directly. Classify each finding:

- Fix: correctness, security, contract, and clear regression findings.
- Reject: style-only suggestions that conflict with established codebase patterns.
- Reject: suggestions that expand scope beyond the brief.

Apply accepted fixes. Keep a rejected-finding list with reasons. Re-verify
tests. Re-call `ws/agents.call(name: "reviewer", prompt: <block below>)`:

```text
Re-review. Updated diff: <start-commit>..HEAD
Rejected findings: <list with reasons>
For each rejected finding: respond [accepted] or [maintained: <brief reason>].
```

Repeat until `[clean]` or after 2 relay cycles - then proceed to cleanup regardless.

### 5. Cleanup

Delete `<review-path>`.

### 6. Spec update

Invoke `ws:lead-update-spec` via Skill tool with args `<start-commit>..HEAD`.

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
Commit range: <start-commit>..HEAD
Test status: pass | fail | skipped
Review: clean | non-clean (<one-line summary>)
Spec: <N entries added, M [planned] stripped> | no changes
<if issues remain after cap:> Open issues: <list>
```

## Doctrine

Edit optimizes for **session-context preservation during code changes** -
the lead retains accumulated understanding by editing directly rather than
forking to a subagent. The reviewer fires in a fresh named-agent context so
its judgment is uncommitted. The relay cap (2 cycles) keeps the loop bounded
so the lead's context is not consumed by negotiation. When a rule is ambiguous,
apply whichever interpretation keeps the lead's context continuous over the
change's full lifecycle.
