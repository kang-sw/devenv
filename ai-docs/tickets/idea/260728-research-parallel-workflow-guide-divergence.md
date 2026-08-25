---
title: "three hand-maintained WORKFLOW.md copies share content with no sync mechanism"
related:
  260726-refactor-retire-spec-planned-marker-mechanism: the range that first gave the three copies behavioral rather than boilerplate divergence risk
---

# three hand-maintained WORKFLOW.md copies share content with no sync mechanism

## Background

The repository carries three near-identical workflow guides:

- `ai-docs/WORKFLOW.md` — this repo's own guide.
- `agents-plugin/skills/lead-bootstrap/WORKFLOW.md` — the full-ws template
  written into downstream projects.
- `agents-plugin-wsflow/skills/lead-bootstrap/WORKFLOW.md` — the wsflow lineage
  of the same template.

Nothing relates them mechanically. `lead-bootstrap` is deliberately absent from
`substitutionMirroredSkills`, so neither the rsrc mirror regen nor the skills
mirror regen touches any of the three. `agents-plugin/skills/manifest.json`
SHA-256s two of them, but a manifest hash only detects that a file changed after
its manifest entry was written; it cannot detect that one copy changed and the
other two did not. The two shipped copies differ today only in one tooling word
(`ws` vs `wsflow`), which is why the drift risk has been invisible: the content
has always been boilerplate that no reader acts on destructively.

## Why this surfaced now

`260726-refactor-retire-spec-planned-marker-mechanism` Phase 2 added the same
carve-out sentence to all three: the implemented-behavior-only rule now names
the Implementation Gap Callout as its exception, so a spec verification pass
keeps a known-but-unscheduled gap rather than deleting it.

That made the shared content **behavioral**. Measured during that phase's cycle-2
review: reverting the carve-out in `ai-docs/WORKFLOW.md` alone, leaving both
shipped copies carved out, passed the full Go suite and the wsflow Python bundle.
A `lead-forge-spec` verification pass run against this repo would then read the
uncarved rule and delete the very callouts the carve-out protects.

The same measurement was then run on the repository's *other* hand-maintained
parallel pair, with the same result. In that phase's cycle-3 review, reverting
the marker-form correction in
`agents-plugin-wsflow/skills/lead-bootstrap/AGENTS.template.md` alone — leaving
the full-ws copy corrected — also passed the full Go suite and the wsflow Python
bundle. `c5ceb879` and `f6162411` each edited that pair by hand in that same
range. So the hole is measured on two independent pairs, not one, and the
`WORKFLOW.md` trio is the instance that happened to be looked at first.

## The targeted guard is not a solution

`TestWorkflowGuidesKeepImplementationGapException`
(`agents-plugin-tool/internal/wsrsrc/workflow_guide_test.go`) now requires each
of the three files to carry that one exception. It was proven by mutating each
file in turn.

**That guard covers exactly one sentence and gives no confidence about anything
else in the three files.** That limitation is this ticket's whole point. The
guard exists because one sentence was known to be safety-critical; the next
divergence will be in a sentence nobody thought to pin, and the same measurement
that justified this guard would come out the same way for every other shared
paragraph.

## Open questions

- Is the right shape a generator (one canonical source, two generated copies with
  a substitution pass, like `substitutionMirroredSkills`), a structural
  equivalence test (the three files agree modulo a declared substitution table),
  or an accepted-divergence manifest that lists what may legitimately differ?
- Is `ai-docs/WORKFLOW.md` genuinely the same document as the two templates, or
  is it a distinct artifact that merely quotes them? A generator answers the
  first reading; a divergence test answers the second. The `ws`/`wsflow` tooling
  word is the only known intentional difference, which argues for the first.
- The two `AGENTS.template.md` copies have the same hole, measured (above), so
  any answer here should cover both pairs or say why it does not. That pair is
  the harder case: `test_wsflow_skill_bundle.py:229-230` *requires* the two
  lineages to differ, so a naive equality guard cannot be added at all without a
  declared substitution table — the constraint is part of the problem, not an
  obstacle to routing around.
- Would a generator break the version-lineage design? The two templates carry
  independent version histories (`v0045` vs `v0006`) precisely so downstream
  projects on each lineage apply their own upgrade chain.

## Non-Scope

- The one-sentence guard already landed; this ticket does not re-litigate it.
- Converging the two `AGENTS.template.md` lineages, which an existing test
  forbids.

## Resolution Note

Ticket `260825` overrides this ticket's Non-Scope exclusion above: it
converges the two `AGENTS.template.md` lineages onto one package-neutral
artifact and one shared migration-ordinal counter (the "existing test
forbids" guard that Non-Scope cites was itself rewritten to assert the
opposite — convergence, not divergence), and separately converges the two
shipped `WORKFLOW.md` template copies to byte-identical content. This
answers this ticket's "structural equivalence, declared-substitution" open
question (L66-67) for those two pairs: they are now the generator/equality
reading, not the divergence-test reading.

This resolves 2 of the 3 near-identical copies this ticket names. The THIRD
copy — this repo's own `ai-docs/WORKFLOW.md` — is downstream generated
output of the now-unified template and is not itself converged by `260825`;
that residual stays open if still relevant to a future triage pass. This
note does not move this ticket's status.
