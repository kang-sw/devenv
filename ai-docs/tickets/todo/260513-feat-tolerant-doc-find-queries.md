---
title: Tolerant documentation lookup queries
related-mental-model:
  - documentation-system
  - mcp-runtime
---

# Tolerant documentation lookup queries

## Background

Documentation lookup tools currently invite broad use by human topic names, but
some inputs behave too much like strict filename or grep-style matching.
During dogfooding, a query such as "wsflow installer marketplace release
packaging" failed to surface nearby `plugin-runtime` or `claude-compatibility`
spec candidates even though those documents were clearly relevant to the task.
Similarly, `ws/convention.read(name: "mental-model")` fails even though users
and skills naturally treat that as the obvious alias for
`mental-model-conventions`.

This is a tool UX surprise: callers naturally expect documentation lookup
surfaces to discover candidate documents from tolerant keywords or accepted
short names, while exact lookup already has separate structured parameters such
as `spec_stem`, `path`, and `domain`.

## Phases

### Phase 1: Make doc lookup queries candidate-oriented

Update the documentation find surfaces so `specs.find(query=...)` and
`mental_models.find(query=...)` handle broad human query text as candidate
discovery, not strict exact matching.

Also update direct convention lookup so common short names resolve to canonical
bundled convention documents.

The implementation should prefer tolerant matching, accepted aliases, and useful
diagnostics:

- Match across titles, summaries, frontmatter, headings, anchor slugs, and
  relevant body text where practical.
- Normalize case, hyphens, underscores, and simple token boundaries.
- Prefer scored candidate results over all-or-nothing filtering for multi-word
  queries.
- Preserve exact lookup through existing structured parameters.
- Treat well-known convention aliases such as `spec`, `ticket`, and
  `mental-model` as aliases for `spec-conventions`, `ticket-conventions`, and
  `mental-model-conventions` in `ws/convention.read`.
- Include enough matched-field or snippet context for callers to understand why
  a candidate was returned.
- On zero results, provide a useful fallback signal such as nearby candidates or
  guidance to retry with shorter noun phrases.
- For convention lookup failures, report accepted convention names and common
  aliases instead of only returning the raw missing-file error.

Refresh workflow guidance if needed so skills treat zero-result find output as
non-final for broad queries and fall back to shorter queries or list/status
surfaces before concluding that no relevant document exists.
