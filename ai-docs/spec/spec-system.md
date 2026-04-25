---
title: Spec System
summary: Tooling, conventions, and agents for authoring and maintaining caller-visible spec documents under ai-docs/spec/.
features:
  - Stem Tooling
    - `ws-generate-spec-stem`
    - `ws-list-spec-stems`
    - `ws-spec-build-index`
  - Authoring Conventions
    - Anchor Format
    - `🚧` Marker Protocol
    - Implementation Gap Callout
    - 🚧 Feature Removal Convention
    - Anchor Placement
    - File Location
  - `/write-spec` Integration
    - Spec-Impact Gate
    - Directory and Split Judgments
  - `spec-updater` Agent
    - Commit-Based `🚧` Stripping
    - 🚧 Pending-Removal Report
    - Missing-Anchor Report
    - Report Format
---

# Spec System

The spec system provides bin tools, authoring conventions (via `ws-print-infra spec-conventions.md`), and a maintenance agent for creating and keeping up-to-date the behavioral spec documents under `ai-docs/spec/`.

## Stem Tooling

### `ws-generate-spec-stem` {#260421-ws-generate-spec-stem-tool}

Generates collision-free `YYMMDD-slug` spec stems from descriptive slug arguments.

```
ws-generate-spec-stem <slug> [<slug>…]
```

One `YYMMDD-slug` is printed per line, where `YYMMDD` is today's date. Before emitting each stem, the tool scans all `*.md` files under `ai-docs/spec/` for existing `{#YYMMDD-*}` anchors. If the candidate stem is taken, it appends `-2`, `-3`, etc. until clear. Multiple slugs in one call are registered in sequence — stems generated earlier in the same call count as existing for later slugs.

- Zero-arg invocation exits with code 1 and prints usage to stderr.
- Unreadable spec files are silently skipped during collision detection.

### `ws-list-spec-stems` {#260421-ws-list-spec-stems-tool}

Lists `{#YYMMDD-slug}` anchors found in spec files.

**Flat-scan mode** (no file argument): recursively scans all `*.md` under `ai-docs/spec/` and prints one stem per line with no indentation. Exits with code 1 if `ai-docs/spec/` is absent.

**File mode** (file argument): parses the given spec file and prints stems indented to reflect heading depth. Body-text anchors (not on a heading line) appear one level deeper than their nearest preceding heading. H1 lines are excluded even when they carry an anchor.

**`-v` flag**: appends a tab-separated display label (heading text with `{#slug}` stripped) after each stem. In flat-scan mode, emits a stderr warning that labels are unavailable and continues.

When no stems are found, the tool prints a hint to stderr and exits with code 0.

### `ws-spec-build-index` {#260421-ws-spec-build-index-tool}

Rebuilds the `features:` frontmatter block of one or more spec files from their heading structure.

```
ws-spec-build-index <spec-file.md> [<spec-file.md>…]
```

For each file, the tool:
1. Parses all `##`–`######` headings, stripping `{#slug}` anchors from display text.
2. Builds an indented YAML list matching heading depth.
3. Upserts the `features:` key in frontmatter (replaces if present, appends if absent).
4. Removes any `stems:` block from frontmatter (stale migration artifact).
5. Prints `done: <path> (N features)` on success, `skip:` when no headings are found, or `error:` on failure.

`title` and `summary` frontmatter fields pass through unchanged.

- Files with no `##`+ headings are skipped without modification; skip notice goes to stdout.
- Unclosed frontmatter (`---` with no closing `---`) is an error; the file is left unmodified.
- Files without frontmatter are handled: the tool synthesizes an empty frontmatter block.
- Never edit the `features:` block manually — `ws-spec-build-index` owns it.

## Authoring Conventions

These rules apply to every file under `ai-docs/spec/`. They are published as `claude/infra/spec-conventions.md` and loaded at authoring time via `ws-print-infra spec-conventions.md`.

### Anchor Format {#260421-spec-anchor-format}

Every named feature carries a `{#YYMMDD-slug}` anchor — datestamped, globally unique, stable after creation.

- Obtain via `ws-generate-spec-stem <descriptive-slug>` before inserting.
- Slugs are lowercase with hyphens, no spaces.
- When a slug must change, the commit message includes `renamed-spec: <old-stem> → <new-stem>`.

Headings without an anchor are treated as organizational containers; they appear in the `features:` index but carry no spec identity.

### `🚧` Marker Protocol {#260421-spec-marker-protocol}

Two marker forms:

- **New unimplemented feature** — prefix the `##` or `###` heading: `## 🚧 Feature Name {#YYMMDD-slug}`
- **Planned change to an existing feature** — add a callout beneath the existing body:
  ```
  > [!note] Planned 🚧
  > Description of the planned change. Current behavior is unchanged until implemented.
  ```

No `🚧` means the feature is implemented and verified. Never include ticket references inside a `🚧` marker — implementation traceability flows through commit `## Spec` sections that reference the spec-stem.

### Implementation Gap Callout {#260421-implementation-gap-callout}

`> [!note] Implementation Gap · YYYY-MM-DD` marks a known-but-unscheduled gap with no ticket. Two forms:

- **Missing behavior**: the intended behavior is understood but not yet built.
- **Unexposed capability**: a capability exists in code but is not yet caller-exposed, with no decision to keep it private.

The callout body text identifies which form applies. Both forms share the same syntax, the same 90-day staleness mechanism, and the same resolution path: create a ticket and convert to `🚧`, or accept the current state and absorb into body prose.

The date records when the gap was first noted.

Permanent behavioral invariants belong in body prose, not in any callout.

### 🚧 Feature Removal Convention {#260423-spec-removal-commit-convention}

When a commit removes a feature from the codebase, the commit's `## Spec` section includes a `removed: <spec-stem>` line for each deleted feature. The spec-updater detects this line and adds the corresponding spec entry to the `### Pending removal` report section rather than auto-deleting it. The caller then removes the spec entry manually after confirming the removal report.

### Anchor Placement {#260421-spec-anchor-placement}

Anchors may appear on any line — heading or body text — not heading-only. A sub-concept within a section can carry its own anchor inline in a body paragraph. `{#YYMMDD-slug}` text is stripped from display output by all tooling.

### File Location {#260421-spec-file-location}

| Form | When to use |
|------|-------------|
| `ai-docs/spec/<area>.md` | Single, self-contained feature surface |
| `ai-docs/spec/<area>/index.md` | Any one split condition is met (see `/write-spec` split judges) |

Start flat. Re-evaluate after writing the file.

## `/write-spec` Integration

### Spec-Impact Gate {#260421-spec-impact-judge}

`/write-spec` evaluates spec impact on every invocation before writing anything. If the topic introduces or modifies no caller-observable behavior, the skill exits with "No public behavior affected." and suggests `/write-ticket` instead.

Caller-observable means: a change a downstream user or consumer can detect without reading source code.

### Directory and Split Judgments {#260421-spec-structure-judges}

Two structural judgments govern file layout:

**`judge: directory-vs-flat`** — applied before first write. Start flat. Convert to `<area>/index.md` only when a split condition fires.

**`judge: split-trigger`** — applied after writing. Fires when any one condition is true:
- A section has its own `🚧` markers with a distinct ticket lifecycle from the parent.
- More than one `[!note] Implementation Gap` block is present.
- A section has a distinct caller audience from the parent document.

When the trigger fires, the section is extracted to a child file (`<area>/<section>.md`) and the original location is replaced with a `See <area>/<section>.md` link.

## `spec-updater` Agent

### Commit-Based `🚧` Stripping {#260421-spec-updater-strip}

The `spec-updater` agent removes `🚧` markers from spec entries whose implementation has landed in commit history.

For each `🚧` occurrence, the agent:
1. Extracts the `{#YYMMDD-slug}` anchor from the heading or nearby body text.
2. Runs `git log --all --grep="<slug>" --oneline` to check for matching commits.
3. If matching commits exist and context is unambiguous: strips `🚧 ` from the heading prefix **and** removes the entire `> [!note] Planned 🚧` callout block (all continuation `> ` lines).
4. Runs `ws-spec-build-index` on each modified file.
5. Defers to caller on ambiguous matches — does not strip.

The agent can target a single stem or scan all files under `ai-docs/spec/`.

### 🚧 Pending-Removal Report {#260423-spec-updater-pending-removal}

When a commit's `## Spec` section contains a `removed: <spec-stem>` line, the spec-updater detects the removal intent and routes the corresponding spec entry to one of two report sections — never deleting automatically:

- **`### Pending removal`** — the entry exists and has no `🚧` prefix (implemented feature being removed). The caller deletes the spec entry manually after confirmation.
- **`### Planned entry dropped`** — the entry exists and carries a `🚧` prefix (planned feature dropped before implementation). The caller deletes the `🚧` entry manually after confirmation.

If the stem is not found in any spec file, the agent skips silently (already cleaned up).

### Missing-Anchor Report {#260421-spec-updater-missing-anchor}

`🚧` headings with no `{#slug}` anchor cannot be confirmed via commit history. The agent reports them in a `### Missing anchors` section and does not attempt to strip them.

### Report Format {#260421-spec-updater-report}

After each run, the agent emits a structured report with sections — empty sections are omitted:

```
### Stripped
<list of stripped stems>

### Needs confirmation
<list of stems with ambiguous commit matches>

### Missing anchors
<list of 🚧 headings with no {#slug} anchor>

### Pending removal
<list of spec entries flagged by `removed: <stem>` in commit history — requires human confirmation before deletion>
```
