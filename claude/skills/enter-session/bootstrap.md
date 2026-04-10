## Invariants

- Owner never runs `git log`, `git diff`, or reads ticket bodies under `ai-docs/tickets/` directly during bootstrap — all raw scanning happens inside the clerk fork.
- The clerk fork is scoped to context collection only — no ticket edits, no status transitions, no source or mental-model reads.
- The Briefing is emitted as a single structured block matching the template — never prose, never merged sections, never reordered.
- Skill names in the Briefing are `/`-prefixed tokens — never paraphrased, reformatted, or translated.
- Empty fields are omitted entirely rather than filled with placeholders.
- All output in English regardless of conversation language.

## On: invoke

1. Read `ai-docs/_index.md` directly — it is small, mandated by `CLAUDE.md`, and anchors the project-level truth the Briefing depends on.
2. Spawn the clerk subagent with the **Clerk spawn prompt** template below. Wait for its report.
3. Consult the **Workflow Map** section against the clerk report and `_index.md` to pick the next step. Apply `judge: scope-complexity` and `judge: parallelizable` when the mechanical lookup leaves room.
4. Emit the **Briefing** template, filling fields from the clerk report and omitting empty ones.
5. Stop. Do not proceed into the recommended next step — the user decides.

## Clerk spawn prompt

```
Collect recent-work context for session bootstrap. Context collection only — no ticket edits, no status transitions, no source or mental-model reads.

Tasks:
1. Capture branch state: `git rev-parse --abbrev-ref HEAD` and `git status --short`.
2. Run `git log --oneline -15`. Synthesize into 2-4 thematic bullets (themes, not per-commit). Do not include raw log lines in output.
3. List ticket stems + titles under `ai-docs/tickets/wip/` and `ai-docs/tickets/todo/`.
4. For each `wip/` ticket, read the body and extract:
   - Purpose: 1-line paraphrase of the intent; quote-adjacent language, no editorializing.
   - Open threads: unresolved design questions, un-acted forward notes, pending decisions carried from the ticket body or recent commits. Omit the bullet entirely if none.

Output format:

### Branch
- <name> (<status summary>)

### Recent work
- <thematic bullet>
- <thematic bullet>

### Active (wip)
#### <stem>
Purpose: <1-line>
Open threads:
- <thread>
- <thread>

### Queue (todo)
- <stem>: <title>

Output in English.
```
