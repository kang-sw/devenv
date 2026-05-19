---
title: Tolerant documentation lookup queries
parent: 260513-epic-workflow-question-loop-hygiene
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

Default human-readable find output should behave like document-grouped
grep-style evidence rather than a prose candidate catalog:

- Start with a summary such as `2 candidate specs for query="..."`.
- Render each document as `<path>\tscore=<score>\thits=<count>` so metadata is
  visually separated without depending on fragile spacing.
- Under each document, render line evidence as `  <line>: <snippet>` with a
  snippet containing roughly N surrounding tokens around the matched term or
  terms.
- Omit separate `matched:` lines from default text output; the snippet itself is
  the human evidence surface.
- Sort document groups by aggregate score descending, then path. Within each
  document, display selected hits in line-number order.
- If results need truncation, choose the top scored documents or hits first,
  then display the selected evidence in document and line order. The summary
  should indicate when only a subset is shown.
- Keep exact structured lookups free to use their existing status/list style;
  the grep-style output applies to broad `query` discovery.

JSON output should preserve the existing document-centered metadata and add
line-level match evidence, for example a `matches` array with `line`,
`matched_terms`, and `snippet` fields. A relative `match_score` field may be
included for ordering, but callers should treat line evidence and matched terms
as the stable explanation surface.

Refresh workflow guidance if needed so skills treat zero-result find output as
non-final for broad queries and fall back to shorter queries or list/status
surfaces before concluding that no relevant document exists.
