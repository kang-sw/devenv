---
name: lead-exit-session
description: Write and commit a next-session handoff note in ai-docs/_index.md. Use when the user asks to end, exit, hand off, compact, or preserve current session context.
---

# Exit Session

## Invariants

- Phase 1 commits staged files only through `ws/git.status` and `ws/git.commit`; exclude `ai-docs/_index.md`.
- Phase 2 issues no information-gathering tool calls - write context from conversation memory only.
- Phase 3 may issue a read call only when the user explicitly requests one to inform a correction.
- Every reference in the context note must include a file path; add `:line-range` when known.
- Mark every uncertain item with `(uncertain)` inline.
- Do not commit `ai-docs/_index.md` until the user explicitly approves in Phase 3.
- All written content must be in English regardless of conversation language.

## On: invoke

### Phase 1 - Commit pass

1. Call `ws/git.status()` to identify staged files.
2. If staged files exist (excluding `ai-docs/_index.md`): commit in logical units per CLAUDE.md commit rules with `## AI Context`.
3. If nothing staged (or only `_index.md`): skip to Phase 2.

### Phase 2 - Context write

Without information-gathering calls, replace the entire `## Session Notes` section in `ai-docs/_index.md` (including HTML comments) with the **Context Note** template.

### Phase 3 - User approval

Present the written `## Session Notes` section. Wait for explicit approval before committing.

On change requests, edit inline. A read call is permitted only when the user explicitly requests it for a correction. Re-present and wait again.

### Phase 4 - Commit

Call `ws/git.commit(paths: ["ai-docs/_index.md"], title: "chore(session): exit context note", ai_context: ["Session context note; no design decisions."])`.

Report: "Session context committed. Prune ## Session Notes from _index.md once the next session has absorbed it."

## Templates

### Context Note

Structure for the `## Session Notes` replacement:

```
## Session Notes

**Branch:** <branch-name> - <top-level purpose>

**Accomplished:** <short hash> <what was done> (one line per logical commit unit)

**In-flight:** <uncommitted or partially-complete items, or "none">

**Next actions:** <what the user intended to do next>

**Key artifacts:** <file-path:line-range> - <why the next session should read it>

**Open questions:** <unresolved items or decisions pending the user, or "none">
```

Append `(uncertain)` after any item the next session should verify before acting on. Omit empty sections.

## Doctrine

Exit-session optimizes for **next-session orientation cost**. File-path
citations and `(uncertain)` markers make the note actionable without
re-derivation. When ambiguous, produce the more actionable context note.
