---
name: write-spec
description: >
  When the user mentions creating, writing, or updating a spec, or
  when chained from /discuss before /write-ticket, invoke this.
argument-hint: "[area name, or spec file path to update]"
---

# Write Spec

Target: $ARGUMENTS

## Invariants

- Run `ws-print-infra spec-conventions.md` (Bash) before any write or update — conventions are canonical there.
- Location follows `judge: directory-vs-flat`.
- Run `ws-spec-build-index` (no args) after every write or update.
- Accuracy check: for every heading without 🚧, confirm the feature exists. Use an Explore agent if uncertain.

## On: invoke

0. **judge: spec-impact** — does this work introduce or modify behavior a caller can observe?
   - no  → output "No public behavior affected." Suggest `/write-ticket`. Exit.
   - yes → proceed with steps below.
1. Identify the target from `$ARGUMENTS` — area name, file path, or description.
2. If creating a new spec:
   a. Apply `judge: directory-vs-flat` to choose the file structure.
   b. Write the spec body following the `spec-format` template. Apply `judge: idea-level` before inserting any `🚧` entries.
   c. Run `ws-spec-build-index` (no args) for cleanup and verification.
   d. Add the spec to the listing in `ai-docs/_index.md`.
3. If updating an existing spec:
   a. Read the target file first.
   b. For each new anchor: run `ws-generate-spec-stem <descriptive-slug>` to get a collision-free `{#YYMMDD-slug}`.
   c. Insert the anchor — on a heading line or anywhere in body text (not heading-only).
   d. Apply `judge: idea-level` before adding any `🚧` Planned callouts. Remove 🚧 from confirmed-implemented features as needed.
   e. Run `ws-spec-build-index` (no args) for cleanup and verification.
4. Apply `judge: split-trigger` after writing — if any section warrants its own file, extract it to `<area>/<section>.md` and replace the original section with `See [section.md](section.md).`
5. Accuracy check — confirm every heading without 🚧 exists in the codebase. Use an Explore agent if uncertain. Never remove 🚧 without confirmation.
6. **Commit** — in a single Bash command, stage the spec file(s) updated in this run (and `ai-docs/_index.md` if the listing changed) then commit:
   `git add <file(s)> && git commit -m "$(cat <<'EOF'\n...\nEOF\n)"`. Do not use `git add -A`. Chaining in one invocation minimizes interleave risk from concurrent sessions.

## Judgments

### judge: spec-impact

Evaluate whether the work introduces or modifies behavior a caller can observe from outside the implementation. Internal restructuring, refactors that preserve external behavior, or tooling changes with no public-facing surface are not spec-relevant. Any addition to or change of a callable interface, user-visible output, or documented convention qualifies.

### judge: idea-level

When about to write a `🚧` entry: write it. Then emit this session reminder: "Session reminder: a `todo/`-or-higher ticket must be created before this session ends for this `🚧` entry to be valid per spec-conventions." Do not ask the user whether to defer — write the entry and remind.

### judge: directory-vs-flat

Use a directory (`<area>/index.md` + child files) when the area has or will have sub-sections split across multiple files. Use a flat file (`<area>.md`) for a single, self-contained feature surface. When uncertain, start flat — convert to directory when the split trigger fires.

### judge: split-trigger

Extract a section into its own file when it has:
- Its own 🚧 markers with a distinct ticket lifecycle, OR
- More than one `> [!note] Constraints` block, OR
- A distinct audience from the parent doc

Any one condition is sufficient.

## Templates

### ws-spec-build-index

After writing or updating a spec file:

```bash
ws-spec-build-index
```

Scans all `*.md` files under `ai-docs/spec/` automatically. Removes any `features:` and `stems:` frontmatter blocks. No file arguments accepted. Output: `[fix] features: removed — <path>` when a `features:` block was stripped; `[error]` on failure; silent when the file was already clean.

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

Planned behavior description — what the caller will observe once implemented.
```

Anchoring rules:
- Run `ws-generate-spec-stem <descriptive-slug>` to obtain a `{#YYMMDD-slug}` before inserting any anchor.
- Anchors may appear on any line (heading or body text), not heading-only.
- Slugs are clean identifiers: lowercase, hyphens, no spaces.
- No ticket references (`[stem/pN]`) in headings or `🚧` markers — implementation traceability is via commits referencing spec-stems.
- Rename: when a slug changes, the commit message must include `renamed-spec: <old-stem> → <new-stem>`.

## Doctrine

Spec documents are the pivot for discussion sessions — when the topic is "what does this currently do from the outside," the spec must answer without requiring source exploration. Every authoring choice optimizes for **drift resistance at the behavioral level**: describe what callers observe, not what the implementation does, so that internal refactors preserving behavior do not invalidate the spec. When a rule is ambiguous, apply whichever interpretation a reader could verify without reading source code.
