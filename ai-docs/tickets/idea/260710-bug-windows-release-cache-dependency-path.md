---
title: Windows release smoke cannot restore the Go dependency cache
---

# Windows release smoke cannot restore the Go dependency cache

## Background

The `v0.33.7` GitHub Actions release run completed successfully, including the
Windows `ws-mcp` smoke job. The job emitted this warning during Go setup:

`Restore cache failed: Dependencies file is not found in D:\\a\\devenv\\devenv. Supported file pattern: go.mod`

The repository's Go module is under `agents-plugin-tool/`, so the Windows
workflow cache configuration likely resolves its dependency path from the
repository root. This does not block release correctness but prevents cache
restoration and adds avoidable CI latency.

## Phases

### Phase 1: Correct the Windows Go cache dependency path

Inspect `.github/workflows/ws-mcp-release.yml` and configure the Windows Go
setup/cache path to resolve `agents-plugin-tool/go.mod`. Preserve the existing
Windows executable build and smoke behavior.

Verify with a GitHub Actions run that the cache-path warning is absent and the
Windows smoke job still succeeds.
