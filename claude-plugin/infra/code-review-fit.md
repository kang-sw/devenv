# Code Review — Fit Partition

Review whether the implementation belongs in this codebase.
Restrict findings to this partition's scope. Do not report issues
that belong to the Correctness or Test partitions.

## Checklist

1. **Conventions** — naming (variables, functions, types, files),
   structure, and formatting as defined in CLAUDE.md.
2. **Code reuse** — duplicate logic that reimplements existing utilities,
   abstractions that already exist in the codebase, bypassed helpers or
   extension points documented in mental-model docs.
3. **Patterns** — does the implementation follow established patterns?
   If a new pattern is introduced, is it justified and consistent with
   the rest of the codebase?
4. **Test style** — test file naming, fixture organization, mock patterns
   (style only — test validity belongs to the Test partition).

## Out of scope

Logic correctness, error paths, security, contract compliance → Correctness partition.
Assertion validity, coverage gaps, mock integrity → Test partition.
