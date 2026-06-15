---
title: project_tree should suppress vendored/generated directory noise
related:
  260524-bug-project-tree-stale-ticket-status-map: adjacent project_tree projection issue; different failure mode
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
document-adjacent paths are omitted or summarized. Preserve the current spec,
ticket, and mental-model inventory sections. Add coverage that would fail if a
fixture tree expands `node_modules/` or similar dependency directories into the
returned project map.
