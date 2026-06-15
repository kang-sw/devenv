---
title: project_tree should suppress vendored/generated directory noise
related:
  260524-bug-project-tree-stale-ticket-status-map: adjacent project_tree projection issue; different failure mode
spec:
  - 260505-project-context-convention-tools
completed: 2026-06-15
---

# project_tree should suppress vendored/generated directory noise

## Background

During ws dogfood on 2026-06-15, `ws/project_tree()` worked after a lead session
key was supplied but expanded `ai-docs/presentation/node_modules/` in detail.
The resulting output was dominated by dependency files and made the project map
hard to use for its intended purpose: loading specs, tickets, mental models, and
high-signal project structure.

Expected behavior: `project_tree` should omit or compact obvious vendored and
generated directories such as `node_modules/`, while preserving the spec,
ticket, and mental-model inventory that workflow sessions need.

## Phases

### Phase 1: Compact noisy generated directories in project_tree

Update the project tree renderer so generated or vendored directories under
document-adjacent paths are omitted. Use Git ignore awareness for the hotfix:
before printing an entry or recursing into a directory, check whether Git would
ignore that path. If the root is not a Git worktree or the ignore check fails,
fall back to the current include behavior rather than failing `project_tree`.

Preserve the current spec, ticket, and mental-model inventory sections. Add
coverage that would fail if a fixture tree with `.gitignore` expands
`node_modules/` or similar dependency directories into the returned project map.

### Result (eb4a2806) - 2026-06-15

`project_tree` now builds a Git-ignore matcher for the target root and skips
ignored `ai-docs/` entries before printing or recursing. Non-Git roots fall back
to the prior include behavior. Coverage now initializes a Git fixture with
`.gitignore` rules and fails if ignored files or `node_modules/` entries appear
in the rendered project tree. The `260505-project-context-convention-tools`
spec entry was updated in `f78bcc73` to record the caller-visible ignore
filtering behavior.
