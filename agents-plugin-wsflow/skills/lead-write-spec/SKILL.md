---
name: lead-write-spec
description: Spec-authoring primitive for explicit spec requests or workflow chains; creates or updates behavioral specs for caller-visible workflow behavior.
---

# Write Spec

Target: user request

## Invariants

- Call `wsflow/convention.read(name: "spec-conventions")` before any write or update - conventions are canonical there.
- Location follows `judge: directory-vs-flat`.
- Call `wsflow/spec_index.verify()` after every write or update.
- Accuracy check: for every heading without `🚧`, confirm the feature exists through direct source inspection or subagent verification when uncertain.

## On: invoke

1. **judge: spec-impact** - does this work introduce or modify caller-observable behavior?
   - no  -> output "No public behavior affected."
       - While `wsflow:lead-proceed` -> continue with appropriate next step.
       - Otherwise -> suggest `wsflow:lead-write-ticket`. Exit.
   - yes -> proceed with steps below.
2. Identify the target from `user request` - area name, file path, or description.
3. If creating a new spec:
   a. Apply `judge: directory-vs-flat` to choose the file structure.
   b. Write the spec body following the `spec-format` template. Apply `judge: idea-level` before inserting any `🚧` entries.
   c. Call `wsflow/spec_index.verify()` for duplicate-anchor verification.
   d. Add the spec to the listing in `ai-docs/_index.md`.
4. If updating an existing spec:
   a. Read the target file first.
   b. For each new anchor: call `wsflow/spec_stem.generate(slug: "<descriptive-slug>")` to get a collision-free `{#YYMMDD-slug}`.
   c. Insert the anchor - on a heading line or anywhere in body text (not heading-only).
   d. Apply `judge: idea-level` before adding any `> [!note] Planned 🚧` callouts. Remove `🚧` from confirmed-implemented features as needed.
   e. Call `wsflow/spec_index.verify()` for duplicate-anchor verification.
5. Apply `judge: split-trigger` after writing - if any section warrants its own file, extract it to `<area>/<section>.md` and replace the original section with `See [section.md](section.md).`
6. Accuracy check - confirm every heading without `🚧` exists in the codebase through direct source inspection or subagent verification when uncertain. Never remove `🚧` without confirmation.
7. **Commit** - call `wsflow/git.commit(paths: ["<file>"], title: "<title>", ai_context: ["<bullet>"])`; include `ai-docs/_index.md` when the listing changed.

## Judgments

### judge: spec-impact

Evaluate whether work introduces or modifies behavior observable outside the implementation. Internal restructuring, behavior-preserving refactors, and tooling with no public-facing surface do not qualify. Callable interfaces, user-visible output, and documented conventions qualify.

### judge: idea-level

When about to write a `🚧` entry or `> [!note] Planned 🚧` callout: write it, then emit: "Session reminder: a non-`epic`, non-`research` `ready/` ticket must exist before this session ends for this `🚧` entry to be valid per spec-conventions." Do not ask whether to defer.

### judge: directory-vs-flat

Use a directory (`<area>/index.md` + child files) when the area has or will have sub-sections split across multiple files. Use a flat file (`<area>.md`) for a single, self-contained feature surface. When uncertain, start flat - convert to directory when the split trigger fires.

### judge: split-trigger

Extract a section into its own file when it has:
- Its own `🚧` markers with a distinct ticket lifecycle, OR
- More than one `> [!note] Constraints` block, OR
- A distinct audience from the parent doc

Any one condition is sufficient.

## Templates

### wsflow/spec_index.verify

After writing or updating a spec file:

```text
wsflow/spec_index.verify()
```

Scans all `*.md` files under `ai-docs/spec/` automatically and reports duplicate anchors. No file arguments accepted. Output is `Spec index: ok` when the corpus is healthy; duplicate anchors or read failures are reported as errors.

### spec-format

```markdown
---
title: <Area / Feature Name>
summary: <One-line external-perspective summary>
---

# <Area / Feature Name>

<One-two sentence summary.>

## Implemented Feature {#YYMMDD-implemented-feature}

Behavioral description. Pseudo-code where it aids clarity.

A specific sub-concept within a section can also carry an anchor. {#YYMMDD-sub-concept}

> [!note] Constraints
> - Intentional limitation or out-of-scope boundary.

> [!note] Planned 🚧
> Will gain X capability. Current behavior unchanged until implemented.

## 🚧 New Feature {#YYMMDD-new-feature}

Planned behavior description - what the caller will observe once implemented.
```

Anchoring rules:
- Call `wsflow/spec_stem.generate(slug: "<descriptive-slug>")` to obtain a `{#YYMMDD-slug}` before inserting any anchor.
- Anchors may appear on any line (heading or body text), not heading-only.
- Slugs are clean identifiers: lowercase, hyphens, no spaces.
- No ticket references (`[stem/pN]`) in headings or `🚧` markers - implementation traceability is via commits referencing spec-stems.
- Rename: when a slug changes, the commit message must include `renamed-spec: <old-stem> -> <new-stem>`.

## Doctrine

Spec documents answer "what does this do from the outside" without source
exploration. Every choice optimizes for **behavioral drift resistance**:
describe caller observations, not implementation mechanics. When ambiguous,
choose what a reader can verify without source.
