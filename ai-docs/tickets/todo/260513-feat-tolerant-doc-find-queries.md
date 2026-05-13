---
title: Tolerant documentation find queries
related-mental-model:
  - documentation-system
  - mcp-runtime
---

# Tolerant documentation find queries

## Background

`ws/specs.find` and `ws/mental_models.find` currently invite broad lookup use
by name, but multi-word human queries can behave too much like strict grep-style
matching. During dogfooding, a query such as "wsflow installer marketplace
release packaging" failed to surface nearby `plugin-runtime` or
`claude-compatibility` spec candidates even though those documents were clearly
relevant to the task.

This is a tool UX surprise: callers naturally expect `find` tools to discover
candidate documents from tolerant keywords, while exact lookup already has
separate structured parameters such as `spec_stem`, `path`, and `domain`.

## Phases

### Phase 1: Make doc find queries candidate-oriented

Update the documentation find surfaces so `specs.find(query=...)` and
`mental_models.find(query=...)` handle broad human query text as candidate
discovery, not strict exact matching.

The implementation should prefer tolerant matching and useful diagnostics:

- Match across titles, summaries, frontmatter, headings, anchor slugs, and
  relevant body text where practical.
- Normalize case, hyphens, underscores, and simple token boundaries.
- Prefer scored candidate results over all-or-nothing filtering for multi-word
  queries.
- Preserve exact lookup through existing structured parameters.
- Include enough matched-field or snippet context for callers to understand why
  a candidate was returned.
- On zero results, provide a useful fallback signal such as nearby candidates or
  guidance to retry with shorter noun phrases.

Refresh workflow guidance if needed so skills treat zero-result find output as
non-final for broad queries and fall back to shorter queries or list/status
surfaces before concluding that no relevant document exists.
