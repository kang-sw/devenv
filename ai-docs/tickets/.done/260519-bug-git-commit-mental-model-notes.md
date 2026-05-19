---
title: git.commit cannot emit Mental Model Notes subsection
spec:
  - 260519-git-commit-mental-model-notes
plans:
  phase-1: 2026-05/19-1642.git-commit-mental-model-notes
related-mental-model:
  - git-workflow-tools
  - documentation-system
completed: 2026-05-19
---

# git.commit cannot emit Mental Model Notes subsection

## Background

Dogfooding `260518-feat-contextual-mental-model-update` showed that the new
`### Mental Model Notes` convention needs a commit body subsection under
`## AI Context`, but `ws/git.commit` currently accepts only `ai_context` bullets
plus top-level workflow sections such as updated tickets, specs, and mental
models.

This means a caller that follows the preferred `ws/git.commit` path cannot
produce the exact commit-body shape consumed by `mental-model-updater`.

## Phases

### Phase 1: Add structured Mental Model Notes commit input

Extend `ws/git.commit` with an explicit input for Mental Model Notes or another
structured way to add H3 subsections under `## AI Context` without falling back
to native Git message assembly.

The implementation should preserve the existing simple `ai_context` path and
keep generated commit bodies predictable for workflow parsing.

### Result (2464c6f5) - 2026-05-19

Implemented structured `mental_model_notes` input for `ws/git.commit`, wired it
through the MCP schema/dispatch and the `ws-mcp git commit --mental-model-note`
CLI mirror, and rendered populated notes as `### Mental Model Notes` under
`## AI Context`.

The implementation preserves required `ai_context` behavior, omits the subsection
for empty notes, and keeps later workflow sections as top-level sections. Tests
cover core commit-message rendering, MCP schema/dispatch, and CLI commit body
behavior.
