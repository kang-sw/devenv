---
kind: print
---

# Review Config

Write `ai-docs/_review.local.md` before the first branch review of a
project that has none.

1. Ask the user, in one response: remote, branch naming, review phases,
   checklist, blocked paths, comment, merge-approval, and notification
   methods, contributor workflow, and deep review.
2. Write the file from the template below and confirm it with the user
   before the review continues.

## Template

```markdown
# Review: <project>

## Remote
<how to list and fetch MR/PR branches>

## Branch Naming                       ← optional
<prefix or regex pattern, e.g. feature/, fix/, TICKET-[0-9]+>

## Review Phases
### intent
Commit messages and ## AI Context match the stated ticket or MR purpose.
### alignment
Diff is consistent with the ticket's stated contract and the project's declared conventions.
### risk
No breaking changes, security issues, or missing tests without justification.

## Landing Lens                        ← optional to customize; range scenario always runs it (built-in default below if omitted); branch scenario never runs it
Diff follows the repo's own conventions (`AGENTS.md` and any authoring manual it
names). A document the change contradicts is updated in the same range — the
document whose own function the change invalidates, not just "any doc touched."

## Checklist                           ← optional
- [ ] <gate item>

## Blocked Paths                       ← optional
- <path pattern>

## Comment Method                      ← optional
<glab mr note / GitLab Web UI / none>

## Merge Approval Method               ← optional
<local merge → push / push → web approve → merge>

## Notification Method                 ← optional
<post-merge contributor notification method>

## Contributor Workflow                ← optional; default: mixed
mixed

## Deep Review                         ← optional
threshold: 20 files / 500 lines
```
