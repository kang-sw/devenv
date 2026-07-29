---
title: "git.commit accepts updated_mental_models but never writes the (mental-model-updated) checkpoint"
related:
  260728-feat-lead-backfill-docs-entry-skill: consumes the checkpoint this surprise drops
---

# git.commit accepts updated_mental_models but never writes the (mental-model-updated) checkpoint

## Observation

Caught while dogfooding on `718773c3`. The commit staged
`ai-docs/mental-model/developer-environment-tools.md` and passed
`updated_mental_models: [...]`. `git.commit` rendered an
`## Updated Mental Models` section and committed cleanly. The body carries no
`(mental-model-updated)` marker.

`mental-model-conventions` line 159 states the marker is mandatory on every
commit that updates mental-model documents. Nothing in
`agents-plugin-tool/internal/` emits or checks it — the only in-tree producers
are playbook prose instructing the model to type it by hand
(`mental-model-updater` step 13, `lead-forge-mental-model`).

## Why it is a surprise

The tool has every input it needs: it stages the paths, so it knows a
mental-model document changed, and it takes a dedicated
`updated_mental_models` field. A caller supplying that field reasonably reads it
as "record this mental-model update per convention" — and gets a section heading
that no consumer reads instead of the marker that every consumer does.

The failure is also silent in the direction that hides it: the commit succeeds,
and the rendered section looks like the update was recorded.

## Downstream cost

The marker is a checkpoint, not decoration. Two consumers resolve ranges from it:

- `mental-model-updater` step 1 scopes from the last checkpoint.
- `lead-backfill-docs` step 1 derives its audit floor from the newest
  `(mental-model-updated)` commit.

A dropped marker moves the floor backwards to an older checkpoint, so the next
backfill or updater run re-audits work already documented. That direction is
safe — it over-covers rather than under-covers — but it is wasted window, and it
compounds silently the longer the marker is missed.

## Candidate directions

Not yet adjudicated; listed so the research does not restart from zero.

- Emit the marker in `git.commit` whenever a staged path is under
  `ai-docs/mental-model/` or is `ai-docs/mental-model.md`. Removes the hand-typed
  step entirely, but writes commit-body text the caller did not ask for.
- Emit it only when `updated_mental_models` is supplied. Narrower, and makes the
  field mean what callers already read it as meaning; still leaves hand-typed
  commits unstamped.
- Verify rather than write: fail the commit when mental-model paths are staged
  and the body carries no marker. Consistent with how ticket guardrails already
  behave at this gate, and keeps authorship with the caller.

The third fits the existing guardrail posture at `git.commit` most closely, but
the first is the only one that makes the marker unmissable.

## Open Questions

- Does any downstream project rely on committing mental-model edits without the
  marker, such that a hard guardrail would break them?
- Should the marker be a body line at all, or has it outgrown prose into a
  trailer that tooling can parse without substring matching? Substring matching
  is how both consumers find it today.
