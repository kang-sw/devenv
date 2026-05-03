---
name: exit-session
description: Write and commit a next-session handoff note in ai-docs/_index.md. Use when the user asks to end, exit, hand off, compact, or preserve current session context.
---

# Exit Session

## Invariants

- Commit staged files only in the commit pass; never stage unrelated work.
- Do not commit `ai-docs/_index.md` until the user explicitly approves the handoff note.
- Keep every handoff reference tied to a concrete file path.
- Mark uncertain statements with `(uncertain)`.
- Keep all AI-authored handoff content in English.
- Use `git commit -F` for multi-paragraph commit messages.

## On: Exit Session

1. Run `git status` to identify staged files.
2. Commit staged non-`ai-docs/_index.md` files in logical units when present.
3. Draft a replacement `## Session Notes` section in `ai-docs/_index.md` using `Templates / Context Note`.
4. Present the written `## Session Notes` section to the user.
5. Wait for explicit approval before committing.
6. Apply requested corrections and re-present when the user asks for changes.
7. Commit only `ai-docs/_index.md` after approval.
8. Report that the session context note was committed.

## Templates

### Context Note

```markdown
## Session Notes

**Branch:** <branch-name> — <top-level purpose>

**Accomplished:** <short hash> <what was done>

**In-flight:** <uncommitted or partially-complete items, or "none">

**Next actions:** <what the user intended to do next>

**Key artifacts:** <file-path> — <why the next session should read it>

**Open questions:** <unresolved items or decisions pending the user, or "none">
```

### Commit Message

```text
chore(session): exit context note

## AI Context
- Session context note; no design decisions.
```

## Doctrine

Exit-session optimizes for next-session orientation cost: the handoff should let a fresh session resume without reconstructing the conversation. When a rule is ambiguous, apply whichever interpretation produces a more directly actionable next-session note.
