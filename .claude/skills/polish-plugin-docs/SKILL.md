---
name: polish-plugin-docs
description: Iterative review + simplification cycle for ws plugin documentation
---

# polish-plugin-docs

Iterative review + simplification cycle for ws plugin documentation.

## Invariants

- Scope: `claude/skills/`, `claude/agents/`, `claude/infra/` (`.md` files only). Never touch `claude/CLAUDE.home.md` or `claude/bin/`.
- Call `ws-declare-agent` for all slots before any `ws-call-named-agent` call.
- Writer system prompt: `.claude/skills/polish-plugin-docs/polish-writer.md`.
- Commit after all per-file writer calls complete, before any reviewer call.
- Each file gets a separate `ws-call-named-agent` call with no session reuse across files.
- Exit after 3 rounds regardless of reviewer status; surface remaining findings to user.

## On: invoke

1. `ws-declare-agent reviewer-resume` — clears stale sessions.
2. `git checkout -b docs/polish-plugin-docs` — abort with message if branch already exists.
3. Collect targets using Glob: `claude/skills/**/*.md`, `claude/agents/**/*.md`, `claude/infra/**/*.md`. Exclude any path containing `CLAUDE.home.md`. Abort if list is empty.
4. Spawn initial reviewer:
   ```bash
   ws-call-named-agent opus --agent reviewer-resume \
     --system-prompt claude/agents/document-reviewer.md \
     "Review each file below for consistency, operational breakage, and authoring-rule compliance.
   <file: path>\n<contents>\n..."
   ```
   Capture Phase 1 findings (Critical / Important / Minor). Set `current-findings` to these findings.

5. Set `round=0`. Loop:

   a. Apply **judge: exit-condition**. Break if true.

   b. Capture current HEAD: `ROUND_BASE=$(git rev-parse HEAD)`.

   c. For each target file (separate call per file, no `--agent`):
      ```bash
      ws-call-named-agent sonnet \
        --system-prompt .claude/skills/polish-plugin-docs/polish-writer.md \
        "File: <path>
      Findings: <current-findings>
      Content:
      <file-content>"
      ```
      If the returned content is empty or the call fails, skip the write and log a warning — do not overwrite the file with empty or error output.

   d. Stage and commit only if files changed:
      ```bash
      git diff --quiet claude/skills claude/agents claude/infra \
        || { git add claude/skills claude/agents claude/infra && git commit; }
      ```
      (follow CLAUDE.md rules for commit message)

   e. Spawn both reviewers in parallel (two Bash calls in the same response):
      ```bash
      # fresh reviewer — new session, full re-read
      ws-call-named-agent opus \
        --system-prompt claude/agents/document-reviewer.md \
        "Re-review all files: <file: path>\n<contents>..."

      # resumed reviewer — diff only
      ws-call-named-agent opus --agent reviewer-resume \
        "Re-review. Diff since last round:
      $(git diff "$ROUND_BASE")"
      ```

   f. Set `current-findings` to merged findings from both reviewers. `round++`.

6. Output:
   ```
   Rounds: <N>
   Branch: docs/polish-plugin-docs
   Merge:  git merge --no-ff docs/polish-plugin-docs
   <reviewer Final reports or remaining findings>
   ```

## Judgments

**judge: exit-condition** — exit when both reviewers emitted a Phase 2 Final report (`## Review:` header present), OR `round == 3`. At round 3 without both Final reports, surface remaining findings before the merge instruction.

## Doctrine

This skill optimizes for **authoring-rule compliance per round** — each iteration closes the gap between current doc state and `skill-authoring.md` standards. When a rule is ambiguous, apply whichever interpretation a reviewer running the authoring checklist would flag first.
