---
title: Release tag push spawns duplicate concurrent workflow runs that race on asset upload
related:
  260814-bug-ship-preflight-go-test-cache-masks-drift: sibling ship-robustness finding
---

# Release tag push spawns duplicate concurrent runs that race on asset upload

## Background

Shipping v0.41.0 produced **two** `ws-mcp release` workflow runs for the same
tag, at the same commit, in the same second:

```
2026-08-14T09:18:30Z run=31787521909 event=push ref=v0.41.0 sha=7a5931f8 -> success
2026-08-14T09:18:30Z run=31787522121 event=push ref=v0.41.0 sha=7a5931f8 -> failure
```

Both ran the `build` job, which creates-or-updates the GitHub release and
uploads `dist/*`. Because they ran concurrently against one release, the loser
hit:

```
HTTP 422: Validation Failed .../assets?...name=ws-mcp-windows-amd64.exe
ReleaseAsset.name already exists
```

The release itself is fine — the winning run published all seven assets and the
published binaries verify against the published `SHA256SUMS`. But **every ship
leaves one spurious red run**, which forces the user to audit a failure that is
not real.

The workflow only triggers on `push.tags: v*` (plus `pull_request` on paths and
`workflow_dispatch`); branch pushes do not match. So the double run comes from
the tag push itself, not from the accompanying `main`/`develop` branch pushes.
The exact GitHub mechanism that emits two runs for one tag ref update (possibly
interaction with the immediately-preceding tag delete + `--follow-tags`
recreate) is not yet pinned down and is part of the investigation.

## Decisions

Not yet decided — captured for discussion. Candidate directions:

- **Serialize with a concurrency group.** Add `concurrency: { group: release-${{
  github.ref }}, cancel-in-progress: false }` (or `true`) so duplicate runs for
  the same tag do not upload concurrently. `cancel-in-progress: true` would kill
  the loser outright; `false` would queue it (still redundant work, but no race
  failure).
- **Make the publish step idempotent.** Tolerate "asset already exists" — e.g.
  delete-then-upload, or treat the 422 on an already-present identical asset as
  success — so a duplicate run cannot red-fail on upload.
- **Root-cause the double trigger.** Determine why one tag push emits two runs
  (reproduce with a throwaway `vX.Y.Z-test` tag; check whether the delete +
  `--follow-tags` recreate is the trigger) and eliminate it at the source if
  possible.

## Constraints

- The fix must not weaken release-asset integrity: the published binaries must
  keep matching the published `SHA256SUMS`.
- Ship-config change territory — `.github/workflows/ws-mcp-release.yml` is a
  release-pipeline artifact; coordinate with `ai-docs/ship/ws.md`.

## Notes

Raised during the v0.41.0 dogfood ship, alongside
`260814-bug-ship-preflight-go-test-cache-masks-drift`. The ship succeeded; this
is CI-noise hardening, not a release blocker.
