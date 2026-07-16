---
title: Anchor-scope placement rule — site-local traps to code comments, cross-cutting invariants to mental-model
sage-review-design: required
related:
  260716-feat-mental-model-openup-injection: complement — injection delivers the doc half; the comment half is delivered by code reading at the edit site
---

# Anchor-scope placement rule — site-local traps to code comments, cross-cutting invariants to mental-model

## Background

The 2026-07-16 three-repo mental-model audit (libhbs, InspectTGV_AIDriven,
PipelineDevProj; ~60 claims verified) found mental-model value concentrated in
non-derivable trap/invariant knowledge, with one systematic ceremony pattern:
content that duplicates knowledge already carried by code comments.
`hardware.md` in PipelineDevProj was ~50% near-verbatim restatement of
`if_camera.h` Doxygen blocks — where a good comment exists, the doc copy is
the ceremony. The fix is a placement rule keyed to **anchor scope**, not
wholesale relocation in either direction.

## Decisions

Placement rule (to encode in the mental-model convention and the
implementer/updater/reviewer playbooks):

- **Site-local trap → code comment at the site.** A trap with a single
  natural anchor site (an argument-order hazard, a surprising edge case of
  one function) is captured as a constraint comment where delivery
  probability to the future editor is ~1. This matches the existing comment
  doctrine ("state a constraint the code itself can't show").
- **Future-site, absence, and cross-module knowledge → mental-model.** Three
  classes structurally cannot live in comments: invariants whose violation
  site does not exist yet (extension recipes, "any new X path must Y"),
  absence knowledge ("there is no rebuild edge A→B", "this subsystem is dead
  on the active path"), and multi-file coupling contracts.
- **Default when unsure → mental-model.** Misclassification cost is
  asymmetric: a site-local trap written into the doc is benign bloat, while a
  cross-module invariant written as a comment at one site falsely reads as
  "captured" and never reaches the future site's author.
- **Reviewer rule:** flag constraint-comment deletions visible in diffs.
  Comments are refactor-fragile and have no reconciliation loop of their own;
  deletion is diff-visible, so review is the natural enforcement point at
  zero new tooling cost.
- **Updater dedup rule:** the mental-model updater treats a new constraint
  comment in the reviewed diff as already-captured knowledge — do not restate
  it in the doc; promote to the doc only when it has cross-module
  implications. This prevents the double-entry regression where the same
  finding lands in both places. (No extra reading load: the updater already
  alternates between diff and doc.)
- **Prune direction on existing duplication:** where doc content duplicates
  existing comments/Doxygen, the doc side is removed, not the comment.
- Related section-level diet (same audit): inventory-style "Entry Points"
  sections and source-comment restatement are the ceremony concentration in
  otherwise high-value docs; the convention text should discourage them for
  new writing. Bulk cleanup of existing docs is out of scope here.

## Phases

### Phase 1: Encode the rule across convention and playbooks

Add the anchor-scope placement rule, the unsure-default, the reviewer
deletion-flag rule, and the updater dedup rule to: the mental-model
convention (`ws/convention.read` source), the implementer playbook's capture
guidance, the code-reviewer playbook, and the mental-model-updater playbook.
Follow `lead-skill-authoring` invariant checks for every changed line; keep
each addition to the one-line/one-row scale that the skill-diet epic's layer
model prescribes (judgment rules belong in playbook bodies; no schema or
rationale prose). Verification: fresh-reader pass over each changed playbook
confirms the rule is stated once per owning document without cross-document
contradiction; wsflow mirror tests stay green.
