# Skill Document Authoring

Principles distilled from a marathon skill reorganization session.
They apply to any skill file where orchestration complexity has
grown beyond what prose can carry — the authoritative file the
model consults under context pressure to route actions correctly.

## Invariants

- The audience is primarily the model re-reading the file mid-session
  under context pressure, not a human reading it fresh.
- Directives and rationale never interleave. Directives up top,
  rationale (if any) at the bottom as a single coda.
- Every rule is one line or as few lines as possible. Long paragraphs
  dissolve under attention pressure; one-liners survive.
- The file is self-contained. No cross-references to tickets, session
  docs, or sibling skill files except explicitly documented universal
  dependencies.
- Mechanism first, judgment second. If a rule fails under attention
  pressure, mechanize it — do not repeat it louder.

## Layout (tested shape)

Skill files for orchestration-heavy workflows follow this top-to-
bottom order:

1. **Invariants** — unambiguous imperatives, zero interpretation
   cost, skimmable under pressure. *"Never read source."*
2. **Event handlers** (`On: X`) — numbered step lists for each entry
   point the skill reacts to. Consistent structure across siblings:
   if one uses sub-bullets, all do. Mixing breaks skim.
3. **Judgments** — soft signals extracted as named handles
   (`judge: <name>`). Criteria live here; event handlers invoke by
   name or reference implicitly.
4. **Templates** — structured outputs the executor produces: brief
   formats, spawn signatures, addenda. Single location so consistency
   is enforceable.
5. **Role table** — one row per role, orthogonal columns (purpose,
   lifespan, etc.).
6. **Doctrine** — one paragraph. The generator.

The marathon skill was restructured from ~360 lines of interleaved
prose to ~250 lines in this layout with no rule changes — the shape
itself carried the compression.

## Doctrine is a generator, not a rationale dump

The Doctrine section is one paragraph. Two jobs:

1. Name a single finite resource the skill optimizes for.
2. Provide a generator clause: *"When a rule is ambiguous, apply
   whichever interpretation better preserves <resource>."*

Test: can the invariants and event handlers be re-derived from this
paragraph? If not, either the paragraph is wrong or the rules are
incoherent with each other.

Anchor concretely. Marathon uses **context window** — measurable,
matches existing vocabulary (token-aware refresh, usage file).
"Attention" was considered and rejected as too fuzzy and too easy
to rationalize violations against. Avoid vague anchors like
"quality" or "focus"; they do not regenerate rules.

## Invariants (top) and Doctrine (bottom) are complementary, not redundant

Two distinct roles:

- **Invariants** — pattern-matchable imperatives for skimming under
  pressure. Read without interpretation.
- **Doctrine** — the generator the invariants were derived from.
  Consulted when a novel case makes a rule ambiguous.

A reader under time pressure hits the top; a reader facing a novel
case hits the bottom. Collapsing them into one section loses both
affordances. State the distinction explicitly in Doctrine ("this is
the generator; return here when in doubt") so the roles are clear.

## Mechanism over prose for frequently-violated rules

When a rule is repeatedly violated despite being written down, the
fix is to mechanize it, not to repeat it.

Example from this session: marathon's delegation discipline was
failing under imperative user phrasing. The fix was not more
invariants but a mandatory structured block emitted at the turn's
entry point (`## Delegation plan` with Intent / Decomposition /
Routing fields). Once the block existed, "classify the turn" became
a mechanism, not a judgment call.

Generalization: if you find yourself repeating a rule across
sections, ask whether it can be replaced by structured output at a
specific point. If yes, that is always better.

## Preserve judgment language as named soft signals

Not every rule can be mechanical. Some are irreducibly soft:

- "Reuse the existing member unless domain contamination would
  mislead"
- "Use haiku for simple lookups, sonnet for cross-module tracing"

Reducing these to boolean branches loses signal. Extract them as
named judgments (`judge: <name>`) referenced from event handlers;
criteria live in a single Judgments section. This preserves the
judgment language where it matters without scattering it through
the file.

## Self-containedness

Skill files are durable; tickets and sessions are not. Cross-references
from skill files to session artifacts will rot.

A skill file must be readable and usable with no external context
except:

- Universal project files (CLAUDE.md)
- Explicitly documented cross-skill dependencies (e.g., "clerk loads
  /write-ticket conventions")

Any reference that exists only in a specific session context — ticket
number, plan path, in-flight decision — must be rewritten as self-
contained content or removed.

## Verify via fresh delegate

After restructuring a skill file, spawn a fresh agent with strict
scope to audit it. The delegate brings no context bias and catches
things the author missed from fatigue.

The audit brief should specify:

- Exact files to read (and explicitly NOT read — closure test)
- Issue categories: contradiction, duplication, orphan reference,
  closure gap
- Iterate until self-satisfied
- Deliverable: structured report
- Do NOT commit

This pattern caught two classes of bugs in the session that produced
these principles: orphan references to undefined roles, and a
lifespan regression that contradicted the file's own judgments.

## Wrapper scripts close permanent traps

When a raw command has a gotcha (argument ordering, fragile flags),
a documentation warning survives only as long as no one rearranges
the args. A wrapper script makes the fix structural — future editors
cannot re-break it by moving text.

Apply when: call sites are identical and the raw command has
non-obvious constraints. Do not apply when call sites vary
meaningfully — premature abstraction.

## Resist verbosity under context fatigue

Long authoring sessions bias toward "richer explanation." This is
the opposite of what skill files need. Symptoms:

- A section terse in the morning has three clarifying sentences by
  evening.
- Meta-commentary about internal mechanism leaks into user-facing
  docs.
- Two code blocks where one sufficed.

Counter: at the end of each authoring turn, re-read your own
additions and cut. Anything explaining internal mechanism to an
external caller is deletable — they did not need to know, and the
script's own header comment already has it.

## Doctrine

Skill files are consulted by the model under attention pressure
mid-session. Every authoring choice optimizes for that moment: put
skimmable imperatives where attention lands first, mechanize rules
that judgment cannot reliably enforce, preserve judgment language
where mechanism would lose signal, and collapse the "why" into a
single generator paragraph at the end. When any authoring decision
looks ambiguous, apply whichever choice the model under pressure
would find easier to execute correctly.
