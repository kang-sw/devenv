# Skill Document Authoring

## Invariants

- The audience is the model re-reading under attention pressure, not a human reading fresh.
- One-liners survive pressure; paragraphs dissolve. Every rule fits one line.
- Directives at top, rationale (if any) as a single Doctrine paragraph at bottom. Never interleave.
- Self-contained. No references to tickets, sessions, or sibling skills — only CLAUDE.md and explicitly declared cross-skill dependencies.
- Repeatedly violated rule → mechanize (structured output block at entry point), do not repeat louder.
- Irreducibly soft rule → extract as named judgment (`judge: <name>`) in a Judgments section; event handlers reference by name.
- After restructuring, spawn a fresh delegate to audit: contradictions, duplication, orphan references, closure gaps.
- At every authoring turn's end, re-read additions and cut.

## Layout

Top-to-bottom order. Simpler skills use the subset they need.

1. **Invariants** — unambiguous imperatives, zero interpretation cost, skimmable.
2. **Event handlers** (`On: X`) — numbered step lists per entry point. Consistent sub-structure across siblings.
3. **Judgments** — named soft signals (`judge: <name>`) with criteria. Invoked from handlers by name.
4. **Templates** — structured outputs: brief formats, spawn signatures, addenda.
5. **Role table** — one row per role, orthogonal columns.
6. **Doctrine** — one paragraph, the generator.

### Doctrine format

Two jobs: (1) name the single finite resource the skill optimizes for, (2) provide a generator clause: *"When a rule is ambiguous, apply whichever interpretation better preserves \<resource\>."* Anchor concretely — measurable nouns ("context window"), not fuzzy ones ("quality", "focus"). Test: can the invariants be re-derived from this paragraph alone?

## Doctrine

Skill files are consulted by the model under attention pressure
mid-session. Every authoring choice optimizes for **executability
under that pressure**: skimmable imperatives where attention lands
first, mechanical structure where judgment fails, preserved judgment
language where mechanism would lose signal, rationale collapsed into
a single generator at the end. When an authoring decision is
ambiguous, apply whichever choice the model under pressure would
execute more reliably.
