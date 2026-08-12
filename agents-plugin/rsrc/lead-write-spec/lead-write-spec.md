---
kind: print
delegates: true
---
# Write Spec

Target: user request

## Invariants

- Call `{{.McpNamespace}}/convention.read(name: "spec-conventions")` before any write or update - conventions are canonical there.
- Location follows `judge: directory-vs-flat`.
- Call `{{.McpNamespace}}/spec_index.verify()` after every write or update.
- Accuracy check: for every heading, confirm the feature exists. Spawn a host-native exploration worker directly with an accuracy-check prompt if uncertain.

## On: invoke

1. **judge: spec-impact** - does this work introduce or modify caller-observable behavior?
   - no  -> output "No public behavior affected."
       - While `{{.SkillNamespace}}:lead-proceed` -> continue with appropriate next step.
       - Otherwise -> suggest using the lead-write-ticket procedure. Exit.
   - yes -> proceed with steps below.
2. Identify the target from `user request` - area name, file path, or description.
3. If creating a new spec:
   a. Apply `judge: directory-vs-flat` to choose the file structure.
   b. Write the spec body following the `spec-format` template.
   c. Call `{{.McpNamespace}}/spec_index.verify()` for duplicate-anchor verification.
   d. If `ai-docs/_index.md` exists, add the spec to its listing (pre-dissolution coexistence only; spec inventory is otherwise derived from the source tree and needs no manual listing).
4. If updating an existing spec:
   a. Read the target file first.
   b. For each new anchor: call `{{.McpNamespace}}/spec_stem.generate(slug: "<descriptive-slug>")` to get a collision-free `{#YYMMDD-slug}`.
   c. Insert the anchor - on a heading line or anywhere in body text (not heading-only).
   d. Call `{{.McpNamespace}}/spec_index.verify()` for duplicate-anchor verification.
5. Apply `judge: split-trigger` after writing - if any section warrants its own file, extract it to `<area>/<section>.md` and replace the original section with `See [section.md](section.md).`
6. **Commit** - call `{{.McpNamespace}}/git.commit(paths: ["<file>"], title: "<title>", ai_context: ["<bullet>"])`; include `ai-docs/_index.md` when its listing changed (pre-dissolution coexistence only).
7. **Output Handoff** - report changed spec path, changed stem, and whether the caller should add `spec:` or keep ticket-local `## Spec Impact`.

## Judgments

### judge: spec-impact

Evaluate whether work introduces or modifies behavior observable outside the implementation. Internal restructuring, behavior-preserving refactors, and tooling with no public-facing surface do not qualify. Callable interfaces, user-visible output, and documented conventions qualify.

### judge: directory-vs-flat

Use a directory (`<area>/index.md` + child files) when the area has or will have sub-sections split across multiple files. Use a flat file (`<area>.md`) for a single, self-contained feature surface. When uncertain, start flat - convert to directory when the split trigger fires.

### judge: split-trigger

Extract a section into its own file when it has:
- More than one `> [!note] Constraints` block, OR
- A distinct audience from the parent doc

Any one condition is sufficient.

## Templates

### {{.McpNamespace}}/spec_index.verify

After writing or updating a spec file:

```text
{{.McpNamespace}}/spec_index.verify()
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
```

Anchoring rules:
- Call `{{.McpNamespace}}/spec_stem.generate(slug: "<descriptive-slug>")` to obtain a `{#YYMMDD-slug}` before inserting any anchor.
- Anchors may appear on any line (heading or body text), not heading-only.
- Slugs are clean identifiers: lowercase, hyphens, no spaces.
- No ticket references (`[stem/pN]`) in headings - implementation traceability is via commits referencing spec-stems.
- Rename: when a slug changes, the commit message must include `renamed-spec: <old-stem> -> <new-stem>`.

Output handoff:

```text
Spec: <spec-stem> [new|updated|removed|renamed]
Path: ai-docs/spec/<path>.md
Ticket handoff: <add spec: <spec-stem> | keep ## Spec Impact | no ticket action>
```

## Doctrine

Spec documents answer "what does this do from the outside" without source
exploration. Every choice optimizes for **behavioral drift resistance**:
describe caller observations, not implementation mechanics. When ambiguous,
choose what a reader can verify without source.
