---
title: Skills-manifest regen is a hidden third regen surface, missing from the authoring procedure and the dev-merge gate
related:
  260810-feat-idea-ticket-attention-policy: origin — its Phase 1 edit to lead-scope-worktree/SKILL.md drifted the skills manifest that this ticket wants covered
---

# Skills-manifest regen is a hidden third regen surface, missing from the authoring procedure and the dev-merge gate

## Background

Editing a skill (`SKILL.md`, or its `rsrc/**` source that mirrors into
`SKILL.md`) requires regenerating **three** artifacts, but the documented
authoring/regen procedure names only two:

- `WSRSRC_REGEN=1 ... TestGenerateRealManifest` (rsrc manifest)
- `WS_REGEN_WSFLOW_RSRC=1 ... TestRegenerateWsflowRsrcMirror` (wsflow mirror)
- **`WSRSRC_REGEN_SKILLS=1 ... TestGenerateRealSkillsManifest`
  (`agents-plugin/skills/manifest.json`) — undocumented in the regen checklist.**

Because the third surface is not in the procedure, an edit can regenerate the
first two and silently leave `agents-plugin/skills/manifest.json` stale. The
only guard is `TestSkillsManifestDriftIsVisible`.

**Concrete incident (2026-08-10/11).** `260810-feat-idea-ticket-attention-policy`
Phase 1 edited `lead-scope-worktree/SKILL.md` and regenerated the rsrc and wsflow
mirrors but not the skills manifest. The stale manifest rode the goal branch
through six per-impl dev-merges undetected and only surfaced red on `main`
(`TestSkillsManifestDriftIsVisible`) when the full suite ran after the goal
merge. Fixed forward on `main` by regenerating the manifest (commit
`2fddee1f`), but the process gap that let it slip remains.

Two distinct gaps, either of which alone would have prevented the incident:

1. **Procedure gap.** The skills-manifest regen is not in the authoring manual's
   regen steps, so an author following the documented steps misses it.
2. **Gate gap.** The per-impl dev-merges did not fail on the drift — either the
   full `go test ./...` (which includes `TestSkillsManifestDriftIsVisible`) was
   not run at each dev-merge, or the drift was masked until the combined state.
   A red drift test should block a dev-merge, not surface only at the roll-up.

## Phases

### Phase 1: Close the regen-surface gap

Candidate directions (pick during planning; not yet decided):

- **Document it.** Add `WSRSRC_REGEN_SKILLS=1 ... TestGenerateRealSkillsManifest`
  to `ai-docs/manuals/skill-authoring.md`'s regen checklist alongside the other
  two, so all three regen surfaces are named in one place.
- **Unify the surface (preferred if cheap).** Collapse the three regen
  invocations behind one command/env (`REGEN_ALL` or a single
  `go generate`-style target) so an author cannot regenerate a subset. Fewer
  surfaces beats better documentation of many.
- **Tighten the gate.** Ensure the dev-merge / version-bump step runs the full
  drift-inclusive suite so a stale manifest fails the merge that introduced it
  rather than the later roll-up.

Verification: after a skill edit that regenerates only a subset, the guard fails
loudly at the authoring/merge step; the documented procedure (or a single
unified command) leaves no manifest un-regenerated.
