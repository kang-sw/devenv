# Skill Document Authoring

## Invariants

- The audience is the model re-reading under attention pressure, not a human reading fresh.
- One-liners survive pressure; paragraphs dissolve. Every rule fits one line.
- Directives at top, rationale (if any) as a single Doctrine paragraph at bottom. Never interleave.
- Self-contained. No references to tickets, sessions, or sibling skills — only CLAUDE.md and explicitly declared cross-skill dependencies.
- Repeatedly violated rule → mechanize (structured output block at entry point), do not repeat louder.
- Mechanical rules and soft judgments do not mix. Soft decision points must be separated and stated explicitly.
- After restructuring, spawn a fresh delegate to audit: contradictions, duplication, orphan references, closure gaps.
- At every authoring turn's end, re-read additions and cut.

## Layout

Top-to-bottom order. Simpler skills use the subset they need. Adapt section types to the document's reading pattern (e.g., named procedures instead of event handlers for reference material) — the principles are universal, the specific sections are not.

1. **Invariants** — unambiguous imperatives, zero interpretation cost, skimmable.
2. **Event handlers** (`On: X`) — numbered step lists per entry point. Consistent sub-structure across siblings.
3. **Judgments** — soft decision points extracted from handlers. In routing-heavy skills, name them (`judge: <name>`) and centralize criteria here; handlers reference by name. A fixed lookup table with unambiguous triggers is a routing rule in the handler, not a judgment.
4. **Templates** — structured output formats: brief formats, spawn signatures, addenda. Procedures ("dispatch X, then do Y") belong in handlers, not here.
5. **Role table** — one row per role, orthogonal columns.
6. **Doctrine** — one paragraph, the generator.

### Invariant checklist

Run against each invariant line after drafting. Every item is yes/no.

- **Falsifiable?** — Can you describe a concrete violation? If not, it is a wish, not a rule.
- **Actionable?** — Does it say what to *do*, not just what to *avoid*?
- **One line?** — If it needs a paragraph to state, it is not yet distilled.
- **Context-free?** — Understandable without reading the surrounding file?
- **Non-redundant?** — Does it say something no other invariant already covers?
- **Universal?** — Is it a constraint that holds in all situations, not a step at a specific point? ("Never skip tests" is an invariant; "append Result after completing a phase" is a handler step.)
- **Derivable?** — Can it be regenerated from the Doctrine paragraph?

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
