---
title: implementer-relay and implementer-elevated duplicate most of their prompt
  instead of sharing an includes base
related:
  260726-bug-lead-implement-lost-review-relay-cycle-cap: its Phase 3 shipped the
    second delegate and accepted the duplication with a cross-file guard rather
    than a refactor
  260630-epic-skill-playbook-diet: the near-duplicate-prose debt this belongs to
---

# implementer-relay and implementer-elevated duplicate most of their prompt

## Topic

Phase 3 of `260726-bug-lead-implement-lost-review-relay-cycle-cap` shipped
`agents-plugin/rsrc/implementer-elevated/implementer-elevated.md` modelled on
`implementer-relay.md`. A fit reviewer measured the result: **51 of 81 non-empty
lines are byte-identical between the two files.**

The differences are real and deliberate — the ticket required the elevated
delegate to differ in inputs, posture, and output contract, not only in tier, and
a reviewer confirmed it does on all three axes. The duplication is in everything
*around* those differences: the shared scaffolding both delegates need in order to
be a fix-cycle implementer at all.

## Why it matters

The repo already has the extension point for this. `reviewer` and the three
`code-review-{correctness,fit,test}` playbooks share a base through
`includes: [code-reviewer]`, a root-level flat dep with no frontmatter and no
model-var placeholder, precisely so that editing the shared half changes all
modes at once. The implementer pair bypassed it.

The concrete cost is drift, not line count. The disposition token vocabulary
(`[fixed]` / `[won't fix: <reason>]` / `[deferred: <reason>]` /
`[escalate: <reason>]`) is now enumerated at **four** sites across the two files.
Phase 2 had added a `strings.Count(...) == 2` guard for the two sites that existed
then; it cannot see a token reworded in one file and missed in the other.

## Current mitigation

Phase 3 did not refactor. It added
`TestRenderedImplementerDelegatesShareOneDispositionVocabulary`, a cross-file
guard that extracts both delegates' enumeration sites and asserts all four agree.
A reviewer verified independently that it catches 7 of 7 enumeration-layer drift
classes, including the two no per-file assertion can see.

So the hazard is currently contained. That is also the risk this ticket exists to
name: **the guard makes the duplication safe, and therefore invisible.** Nothing
will surface it again on its own.

## Open questions

- What actually belongs in the shared base? The token vocabulary and the fix-cycle
  process shape clearly; the posture sections clearly not. The boundary is the
  design question, and getting it wrong would either leave the drift hazard or
  force the elevated delegate's distinguishing posture back into shared text.
- `code-reviewer.md` is var-free by necessity, because wrappers with different
  tiers would otherwise collide on an undeclared model var. Both implementer
  delegates declare `RoleModel`, so check whether the same constraint applies
  before assuming the `code-reviewer` shape transfers.
- Does the cross-file guard survive the refactor, become redundant, or need to
  become an intra-base check? A refactor that silently drops it would trade a
  contained hazard for an uncontained one.

## Prior art

- `260630-epic-skill-playbook-diet` — the near-duplicate-prose debt this is an
  instance of.
- `ai-docs/mental-model/prompt-bundle.md` records the `includes: [code-reviewer]`
  arrangement and warns that playbook-local includes are for fragments owned by
  one playbook and should not replace shared root-level deps used by several.
