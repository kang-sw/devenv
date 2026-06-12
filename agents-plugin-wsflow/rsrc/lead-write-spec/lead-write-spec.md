---
kind: print
delegates: true
---
# Write Spec

Target: user request

## Invariants

- Call `ws/convention.read(name: "spec-conventions")` before any write or update - conventions are canonical there.
- Location follows `judge: directory-vs-flat`.
- Call `ws/spec_index.verify()` after every write or update.
- Accuracy check: for every heading without `🚧`, confirm the feature exists. Spawn a native Explore-style subagent via the `explore` playbook (see lead-workflow-manual playbook) if uncertain.

## On: invoke

1. **judge: spec-impact** - does this work introduce or modify caller-observable behavior?
   - no  -> output "No public behavior affected."
       - While `ws:lead-proceed` -> continue with appropriate next step.
       - Otherwise -> suggest using the lead-write-ticket procedure. Exit.
   - yes -> proceed with steps below.
2. Identify the target from `user request` - area name, file path, or description.
3. If creating a new spec:
   a. Apply `judge: directory-vs-flat` to choose the file structure.
   b. Write the spec body following the `spec-format` template. Apply `judge: contract-first-spec` before inserting any `🚧` entries.
   c. Call `ws/spec_index.verify()` for duplicate-anchor verification.
   d. Add the spec to the listing in `ai-docs/_index.md`.
4. If updating an existing spec:
   a. Read the target file first.
   b. For each new anchor: call `ws/spec_stem.generate(slug: "<descriptive-slug>")` to get a collision-free `{#YYMMDD-slug}`.
   c. Insert the anchor - on a heading line or anywhere in body text (not heading-only).
   d. Apply `judge: contract-first-spec` before adding any `> [!note] Planned 🚧` callouts. Remove `🚧` from confirmed-implemented features as needed.
   e. Call `ws/spec_index.verify()` for duplicate-anchor verification.
5. Apply `judge: split-trigger` after writing - if any section warrants its own file, extract it to `<area>/<section>.md` and replace the original section with `See [section.md](section.md).`
6. Accuracy check - confirm every heading without `🚧` exists in the codebase. Spawn a native Explore-style subagent via the `explore` playbook (see lead-workflow-manual playbook) if uncertain. Never remove `🚧` without confirmation.
7. **Commit** - call `ws/git.commit(paths: ["<file>"], title: "<title>", ai_context: ["<bullet>"])`; include `ai-docs/_index.md` when the listing changed.
8. **Output Handoff** - report changed spec path, changed stem, whether any `🚧` marker was added, and whether the caller should add `spec:` or keep ticket-local `## Spec Impact`.

## Judgments

### judge: spec-impact

Evaluate whether work introduces or modifies behavior observable outside the implementation. Internal restructuring, behavior-preserving refactors, and tooling with no public-facing surface do not qualify. Callable interfaces, user-visible output, and documented conventions qualify.

### judge: contract-first-spec

Yes: planned behavior must be visible and stable before implementation begins.
Usually yes: externally consumed schemas, CLI/API contracts, file or wire formats, cross-skill routing contracts, or multi-ticket planned behavior.
No: planned text mainly restates a ticket phase, final behavior will be refined during implementation, or a ticket-local `## Spec Impact` is enough for post-implementation closeout.
When yes: write the marker, then emit "Session reminder: implementation `🚧` entries require a non-`epic`, non-`research`, non-`workset` `ready/` ticket; epic, research, or workset tickets may back only planned decomposition, investigation text, or operating context."
When no: do not write `🚧`; suggest ticket-local `## Spec Impact` instead.

### judge: directory-vs-flat

Use a directory (`<area>/index.md` + child files) when the area has or will have sub-sections split across multiple files. Use a flat file (`<area>.md`) for a single, self-contained feature surface. When uncertain, start flat - convert to directory when the split trigger fires.

### judge: split-trigger

Extract a section into its own file when it has:
- Its own `🚧` markers with a distinct ticket lifecycle, OR
- More than one `> [!note] Constraints` block, OR
- A distinct audience from the parent doc

Any one condition is sufficient.

## Templates

### ws/spec_index.verify

After writing or updating a spec file:

```text
ws/spec_index.verify()
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
- Call `ws/spec_stem.generate(slug: "<descriptive-slug>")` to obtain a `{#YYMMDD-slug}` before inserting any anchor.
- Anchors may appear on any line (heading or body text), not heading-only.
- Slugs are clean identifiers: lowercase, hyphens, no spaces.
- No ticket references (`[stem/pN]`) in headings or `🚧` markers - implementation traceability is via commits referencing spec-stems.
- Rename: when a slug changes, the commit message must include `renamed-spec: <old-stem> -> <new-stem>`.

Output handoff:

```text
Spec: <spec-stem> [new|updated|removed|renamed]
Path: ai-docs/spec/<path>.md
Planned marker: <added|none|removed>
Ticket handoff: <add spec: <spec-stem> | keep ## Spec Impact | no ticket action>
```

## Doctrine

Spec documents answer "what does this do from the outside" without source
exploration. Every choice optimizes for **behavioral drift resistance**:
describe caller observations, not implementation mechanics. When ambiguous,
choose what a reader can verify without source.
