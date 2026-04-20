---
name: write-spec
description: >
  When the user mentions creating, writing, or updating a spec, or
  when chained from /write-ticket after a phase that changes public
  behavior, invoke this.
argument-hint: "[area name, or spec file path to update]"
---

# Write Spec

Target: $ARGUMENTS

## Invariants

- Run `load-infra spec-conventions.md` (Bash) before any write or update — conventions are canonical there.
- Location follows `judge: directory-vs-flat`.
- Run `spec-build-index` on the target file after every write or update.
- Accuracy check: for every heading without 🚧, confirm the feature exists. Use an Explore agent if uncertain.

## On: invoke

1. Identify the target from `$ARGUMENTS` — area name, file path, or description.
2. If creating a new spec:
   a. Apply `judge: directory-vs-flat` to choose the file structure.
   b. Write the spec body following the `spec-format` template.
   c. Run `spec-build-index` on the new file.
   d. Add the spec to the listing in `ai-docs/_index.md`.
3. If updating an existing spec:
   a. Read the target file first.
   b. Apply changes — add 🚧 headings, add Planned callouts, remove 🚧 from confirmed-implemented features.
   c. Run `spec-build-index` to regenerate frontmatter.
4. Apply `judge: split-trigger` after writing — if any section warrants its own file, extract it to `<area>/<section>.md` and replace the original section with `See [section.md](section.md).`
5. Accuracy check — confirm every heading without 🚧 exists in the codebase. Use an Explore agent if uncertain. Never remove 🚧 without confirmation.

## Judgments

### judge: directory-vs-flat

Use a directory (`<area>/index.md` + child files) when the area has or will have sub-sections split across multiple files. Use a flat file (`<area>.md`) for a single, self-contained feature surface. When uncertain, start flat — convert to directory when the split trigger fires.

### judge: split-trigger

Extract a section into its own file when it has:
- Its own 🚧 markers with a distinct ticket lifecycle, OR
- More than one `> [!note] Constraints` block, OR
- A distinct audience from the parent doc

Any one condition is sufficient.

## Templates

### spec-build-index

After writing or updating a spec file:

```bash
spec-build-index <spec-file.md>
```

Parses heading structure from `##` and deeper, rebuilds the `features:` frontmatter field. Preserves `title` and `summary`.

## Doctrine

Spec documents are the pivot for discussion sessions — when the topic is "what does this currently do from the outside," the spec must answer without requiring source exploration. Every authoring choice optimizes for **drift resistance at the behavioral level**: describe what callers observe, not what the implementation does, so that internal refactors preserving behavior do not invalidate the spec. When a rule is ambiguous, apply whichever interpretation a reader could verify without reading source code.
