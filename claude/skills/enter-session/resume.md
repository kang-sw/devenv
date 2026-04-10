## Invariants

- The Continuation payload appended below by the dispatcher is authoritative — the dispatcher pre-gated this path on HEAD SHA match, so the payload reflects the current commit state.
- Do not spawn clerk. Do not rescan `git log`, `git diff`, or ticket bodies. The payload is the sole recent-work source.
- Read `ai-docs/_index.md` directly; it is small and mandated.
- Never delete or modify `ai-docs/_continue.local.md` — it is consumed non-destructively so re-invocation is idempotent.
- The Briefing is emitted as a single structured block matching the template — never prose, never merged sections, never reordered.
- Skill names in the Briefing are `/`-prefixed tokens — never paraphrased, reformatted, or translated.
- Empty fields are omitted entirely rather than filled with placeholders.
- All output in English regardless of conversation language.

## On: invoke

1. Read `ai-docs/_index.md` directly.
2. Parse the **Continuation payload** block appended to this skill by the dispatcher. Extract `HEAD`, `Branch`, and `Active` from the header; read `Mental state`, `Next concrete step`, `Open threads`, and `User directives pending` sections.
3. Consult the **Workflow Map** below against the payload's `Next concrete step` to pick the next skill. Apply `judge: scope-complexity` and `judge: parallelizable` if the mapping is ambiguous.
4. Emit the **Briefing** template, mapping payload fields:
   - `Branch` — from payload header
   - `Recent work` — 1-3 bullets paraphrased from `Mental state`
   - `Active` — payload header `Active` stem, purpose drawn from `Mental state`
   - `Open threads` — merge `Open threads` and `User directives pending`
   - `Queue` — omit in resume path
   - `Recommended next` — skill chosen from Workflow Map, reason copied from `Next concrete step`
5. Stop. Do not proceed into the recommended next step — the user decides.
