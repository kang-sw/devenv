---
title: git.commit cannot emit Mental Model Notes subsection
related-mental-model:
  - git-workflow-tools
  - documentation-system
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

## Desired Direction

Extend `ws/git.commit` with an explicit input for Mental Model Notes or another
structured way to add H3 subsections under `## AI Context` without falling back
to native Git message assembly.

The implementation should preserve the existing simple `ai_context` path and
keep generated commit bodies predictable for workflow parsing.
