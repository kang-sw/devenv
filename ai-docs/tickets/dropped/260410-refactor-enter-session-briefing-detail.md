---
title: "Session briefing detail expansion — clerk synthesis, template relax, language rule drop"
related: []
---

# Session Briefing Detail Expansion

## Problem

The `/enter-session` briefing is rigidly templated and information-thin,
especially when active tickets are prose-only (no phases/skeletons/plans
in frontmatter). `collect-recent-context` only extracts metadata from
tickets and commit forward-notes, so even a richer `/enter-session`
template would have nothing to fill — the sparse briefing is
fundamentally a collect-side problem, not a template-side one.

Separately, the current `/enter-session` language invariant ("briefing
in user's conversation language, translate from clerk's English")
conflicts with the `/manual-think` English-thinking rule and introduces
a translation layer that adds no value for a workflow repo whose
AI-authored artifacts are already English-only per `CLAUDE.md`.

## Doctrine Reframe (collect-recent-context)

The existing doctrine optimizes for **"owner token economy"** — "only
the compact state report crosses the fork boundary." Reframe to:

> The finite resource is **owner work avoided**: the owner must never
> have to read raw ticket bodies or raw commit lists directly. Thousands
> of tokens of synthesized report crossing the fork boundary is
> acceptable; forcing the owner to re-scan source lists is not.
> Synthesis inside the fork always beats extraction that punts judgment
> to the owner.

All agenda items below derive from this reframe.

## Agenda

### 1. `collect-recent-context` — synthesis upgrade

- **Override clerk's model to sonnet at the invocation site.** Do
  **not** modify the base clerk agent definition — it must keep its
  default light model for other callers. The override lives in
  `collect-recent-context/SKILL.md` (or wherever this skill configures
  its fork). Sonnet is required because the expanded output below
  needs synthesis, not just extraction.
- **Expand clerk's task from extraction to synthesis.** Update the
  `## Your task` section in the skill so clerk produces:
  - `Purpose:` per ticket — a 1-line paraphrase of the ticket body's
    intent. Conservative: quote-adjacent language, do not editorialize.
  - `Open threads:` per ticket — unresolved design questions,
    un-acted forward notes, pending decisions carried from the ticket
    body or recent commits. Omit the bullet entirely if none.
  - Top-level `### Recent work` — a 2-4 bullet thematic rollup of the
    last ~10 commits. Themes, not per-commit, not raw log.
- **Update the report template** in the skill to include the new
  fields (`Recent work` top-level section; `Purpose` and `Open threads`
  per-ticket bullets). Keep the existing "omit empty bullets" rule.
- **Simplify line 58** from "Output in English regardless of any other
  context." to "Output in English." The "regardless" clause is
  vestigial once enter-session's language-variability source is removed
  (section 2).
- **Rewrite the Doctrine paragraph** to match the reframe above. The
  new doctrine should be re-derivable into all existing + new
  invariants.

### 2. `enter-session` — template relaxation + language rule removal

- **Remove language directives from 4 locations:**
  - `description` frontmatter (current lines 7-8): drop "in the user's
    conversation language"
  - Invariant (current line 20): delete the entire translation
    clause ("Briefing output is written in the user's conversation
    language; the subagent reports in English and you translate when
    presenting")
  - `On: invoke` step 3 (current line 63): simplify to just "Emit the
    briefing block per the template."
  - `Doctrine` paragraph (current line 105): drop "presents a briefing
    in the user's language while"
- **Preserve the skill-name-preservation rule as a standalone
  invariant.** Previously embedded inside the translation invariant.
  New standalone form (one line, falsifiable, actionable):
  > Skill names in the briefing are `/`-prefixed tokens — never
  > paraphrased, reformatted, or translated.
- **Restructure the briefing template** to two sections:

  ```
  ## Briefing

  ### Context
  - Branch: <name> (<status>)
  - Recent work:
    - <thematic bullet>
    - <thematic bullet>
  - Active ticket: <stem> — <Purpose from clerk>
  - Open threads:
    - <thread>
    - <thread>

  ### Recommended next
  `<skill>` — <one-line reason>
  ```

  - `### Context` is field-rich but each field may be multi-line,
    sub-bulleted, or omitted when empty. The agent exercises judgment
    on per-field shape per situation.
  - **No separate `### On the table` / `### Pending` section.** Open
    work surfaces inside Context via `Open threads`. One rich section
    is more skimmable than two.
  - `### Recommended next` stays **rigid**: backtick-quoted skill name
    + one-line reason, anchored at the bottom so routing signal is not
    diluted by the prose-friendly Context above.
- **Update the enter-session Doctrine paragraph** to drop the "user's
  language" clause. The optimizing resource ("routing clarity after
  context loss with minimal owner-context burn") stays the same.

### 3. Audit

After sections 1 and 2 land, spawn a fresh delegate (Explore or clerk)
to audit both skill files against `ai-docs/ref/skill-authoring.md`:

- Contradictions between invariants (within each skill and across the
  pair)
- Duplicated rules
- Orphan references — e.g., doctrine clauses that no invariant derives
  from, or invariants not derivable from doctrine
- Closure gaps — handlers referencing judgments/templates that no
  longer exist; template fields whose source isn't produced by collect
- Run the Invariant / Constraint checklist (falsifiable, actionable,
  one-line, context-free, non-redundant, universal, derivable) on every
  invariant line in both files

Report findings; apply corrections in-place before closing the ticket.

## Rejected Alternatives

- **Free-form prose briefing.** Rejected because
  `ai-docs/ref/skill-authoring.md` classifies briefing formats as
  structured Templates, not prose. Solution: keep templated shape,
  widen field content and allow per-field multi-line.
- **Leave `collect-recent-context` unchanged and only thicken
  enter-session's template.** Rejected because the template has
  nothing to fill — metadata-only extraction was the root cause of
  thin briefings, not the template shape.
- **Owner synthesizes after clerk returns raw material.** Rejected —
  violates the reframed doctrine (forces owner to read raw lists,
  which is exactly the cost we want eliminated).
- **Modify the base clerk agent definition to default to sonnet.**
  Rejected — clerk's other callers don't need sonnet, and changing the
  base model is out of scope for this ticket. The override must live
  at the `collect-recent-context` invocation site only.
- **Keep a separate `### On the table` / `### Pending` section.**
  Rejected in favor of Context-only layout. Open threads carry the
  same signal inside Context and keep the briefing skimmable.
- **Phase/skeleton/plan ticket machinery for this change.** Rejected —
  this is a workflow repo; markdown skill edits don't need
  integration-test phases. Agenda-only ticket follows the precedent of
  `260404-research-marathon-skill-improvements`.

## Ordering Constraints

- **(1) before (2).** Section 2's new briefing template references
  fields (`Purpose`, `Open threads`, `Recent work`) that section 1 must
  produce first. Reversing the order leaves enter-session referencing
  fields that don't exist.
- **(3) after both (1) and (2) are committed.** The audit needs the
  final state of both files.

## Open Unknowns

- **Clerk model override mechanism.** The exact syntax for overriding
  clerk's model to sonnet at the `collect-recent-context` invocation
  site is not yet confirmed. Candidates:
  - Frontmatter `model: sonnet` alongside `context: fork` / `agent:
    clerk` in the skill file
  - A dedicated skill-level override field
  - Wrapping the fork in an explicit `Agent()` call with `model`
    parameter
  Must **not** touch the base clerk agent definition. Resolve during
  section 1; update this ticket in-place if a new constraint is
  discovered.
