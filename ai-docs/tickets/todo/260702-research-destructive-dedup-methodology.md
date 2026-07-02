---
title: "Aggressive playbook dedup: audit methodology for destructive-first section merges"
parent: 260630-epic-skill-playbook-diet
sage-review: required
---

# Aggressive playbook dedup: audit methodology for destructive-first section merges

## Background

During the `lead-write-ticket.md` diet (371 → 305 → 277 lines, this epic), the
user separately probed an extreme hypothetical: could the same skill be cut to
under 50 lines, even allowing the *definition* of "capture user intent" itself
to be rephrased (more rational, or "concise but potent")? That extreme was
never applied — the committed result stayed at the conservative 277-line
dedup. But pursuing the hypothetical surfaced a repeatable methodology worth
naming and testing against other skills before this epic's remaining Phase 3/4
targets.

The core move used throughout: apply `lead-skill-authoring`'s destructive-first
test ("would a model following only Layer 3 + MCP schemas reach the same
execution outcome? uncertain → delete") not sentence-by-sentence, but at
section/checklist scope — asking whether an entire parallel checklist or
sub-block can collapse into a shared restatement.

## Self-corrections observed this session

Two real self-corrections happened while applying this aggressive scope, both
worth treating as evidence about the failure mode, not just anecdotes:

1. **Hidden guardrail loss.** The first Content/Intent/Decision-Queue merge
   draft (20 → 9 lines) silently dropped two independent checks that the
   "generative fresh-implementer test" (could a fresh implementer produce a
   materially different result from the settled discussion?) does not itself
   cover: sketch-literalness (an under-capture failure mode not implied by the
   test) and over-capture / unconfirmed-content exclusion (the *inverse*
   failure direction — the test only catches missing content, never wrongly
   included speculative content). Caught only because the user asked directly
   "is this lossy, or a clarity gain?" — not caught by the destructive-first
   test itself, since the test was framed at the wrong grain (per-item, not
   per-guardrail-role).
2. **Placement error, not information loss.** The generative fresh-implementer
   test was drafted into `On: Apply Ticket Content` (a write-time step), but
   the test is inherently a re-read/self-review action ("can this be restored
   from what was written") and belongs in `On: Intent Review`. No information
   was lost, but the draft would have executed the check at the wrong point in
   the flow. Caught by the user re-reading the merged draft, not by any
   built-in check.

A third, lower-risk variant of the same family was used successfully in the
`lead-implement.md` diet (281 → 225 lines): instead of inferring from the MCP
schema what a playbook section restates, the actual generated `enter.implement`
todo `Instruction` text was read directly and compared against the playbook
prose, confirming near-verbatim duplication before deleting. This is
destructive-first applied against real generated output rather than
inferred/assumed output, and it did not trigger either self-correction above.

## Working hypothesis

Section-scope destructive-first dedup is a valid and higher-yield lever than
sentence-level dedup, but it is unsafe to apply without an explicit per-merge
audit step that separately asks:

- Is each item in the pre-merge lists a **pure restatement** of another item
  (safe to fold), or does it encode a **distinct guardrail** with its own
  failure mode (must be preserved even if it looks similar on the surface)?
- Does the destructive-first test I'm using to justify a merge cover *both*
  failure directions for that content (under-capture and over-capture), or
  only one? A single generative test tends to only catch one direction.
- Where the pre-merge content encoded a step-ordering or flow-position
  decision (e.g., write-time vs. review-time), does the merged version
  preserve that position, or did the merge implicitly relocate the check?
- Where a schema-inferred restatement is being deleted, has the actual MCP
  tool's generated output been read and diffed against the prose being
  deleted, rather than assuming the schema alone predicts it?

None of this was encoded anywhere before this session; it was reconstructed
ad hoc, twice, only because the user caught both misses after the fact.

## Open questions

- Should this per-merge audit become an explicit step in
  `lead-skill-authoring.md`'s destructive-first stance (a checklist run before
  any section-scope merge is applied), rather than relying on the user to
  catch misses in review? If so, what is the minimal wording that avoids
  turning it into its own restatement-heavy ceremony?
- Is the "distinct guardrail vs. pure restatement" distinction cleanly
  checkable in general, or specific to checklist-shaped content (as in
  `lead-write-ticket`)? Worth testing against a differently-shaped skill before
  generalizing.
- Should the remaining Phase 3/4 diet targets (`lead-write-spec`,
  `lead-add-rule`, `lead-workflow-manual`, `lead-sprint`) be diet-ed under this
  audit discipline from the start, or should one of them first be used as a
  deliberate test case to see whether the audit catches misses in a fresh
  skill (rather than only retrospectively explaining misses already found in
  `lead-write-ticket`)?
- Is there a cheaper substitute for "read the actual generated MCP output and
  diff against prose" for skills that are Layer-3-heavy with no `enter.*`
  backing (e.g. `lead-add-rule`), where there is no generated instruction text
  to compare against?

## Non-goals

- This ticket does not diet any additional skill. No skill file changes are
  in scope here.
- This ticket does not resolve the extreme "sub-50-line, redefine intent
  capture" hypothesis from `lead-write-ticket` — that hypothesis is closed
  (not applied, not carried forward) and is recorded here only as the
  originating probe, not as a target to revisit.
