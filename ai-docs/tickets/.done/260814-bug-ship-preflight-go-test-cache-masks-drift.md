---
title: Ship pre-flight go test cache masks generated-artifact drift; add autonomous CI-failure recovery policy
completed: 2026-08-14
---

# Ship pre-flight `go test` cache masks generated-artifact drift

## Background

During the v0.41.0 ship, the ship-config pre-flight ran
`cd agents-plugin-tool && go test ./...` and reported all packages green — but
`internal/wsrsrc` was served from the Go test cache (`ok ... (cached)`). The
cached result predated a real drift: `agents-plugin/skills/manifest.json` was
stale relative to `lead-bootstrap/AGENTS.template.md` and
`lead-bootstrap/WORKFLOW.md`, which were re-edited in `2ef51ad2` without a
manifest regen. CI runs fresh, so `TestSkillsManifestDriftIsVisible` failed on
the tag-push release build **after** the tag was already pushed, blocking the
GitHub release (no release artifact was created).

Two gaps compounded:

1. **Detection gap.** A cached local `go test ./...` can pass while CI fails on
   the same tree, so the pre-flight gate is not a faithful predictor of CI.
2. **Recovery gap.** The ship procedure had no stated policy for a
   post-tag-push CI failure, so each such failure required an interactive
   confirmation gate even when the fix was mechanical and carried no
   behavior/semantics change — forcing the user to audit routine recoveries.

## Resolution (7a5931f8 / follow-up) - 2026-08-14

Fixed directly against `ai-docs/ship/ws.md`; captured here as the durable record.

- **Detection.** Pre-flight `go test` now mandates `-count=1`
  (`cd agents-plugin-tool && go test ./... -count=1`) so a cached pass can no
  longer mask a stale generated artifact.
- **Recovery.** Added a `## Recovery` section encoding the autonomous
  CI-failure policy the user requested: when a post-tag-push workflow failure
  is an "auto-proceed"-tier fix with no policy/behavior-semantics change (stale
  generated artifact, formatting/lint, mechanical test locator,
  changelog/version-metadata mismatch), the lead fixes it, fast-forwards `main`,
  and — only while no release artifact exists under the tag — moves the tag to
  the fix commit and re-pushes, looping until green, without opening a
  confirmation gate. Failures that would change workflow semantics,
  protocol/API surface, the shipped feature set, or the release version (or with
  unclear cause) still stop and surface. Tag moves are permitted only until a
  release is published under the tag; afterward, ship a new patch instead.

The stale manifest itself was regenerated and committed as `7a5931f8`.
