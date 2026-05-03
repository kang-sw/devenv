---
name: forge-mental-model
description: Reconstruct mental-model documents from scratch by surveying operational domains, confirming the domain list, and writing verified modification-focused domain files under ai-docs/mental-model/.
---

# Forge Mental Model

## Invariants

- Call `ws/convention.read` for `mental-model-conventions` before any mental-model write.
- Use `ws/subquery` with `deep_research: true` for every survey and verifier delegate call.
- Warn and proceed when specs are absent; spec cross-references are opportunistic, not blocking.
- Confirm the final domain list with the user before any domain file is written.
- Maintain a visible task list with one `forge-mental-model-<domain>` item per confirmed domain.
- Complete the domain survey before drafting any domain file content.
- Complete the verifier pass before writing the final domain file.
- Commit every mental-model document change with `(mental-model-updated)` in the commit message body.
- Dispatch all cold-start survey delegates in one response turn when the host supports parallel calls.
- Keep all AI-authored mental-model and commit text in English.
- Write operational contracts, coupling, extension hazards, mistakes, and debt; do not write type listings or source paraphrases.
- Do not read convention files from host-local plugin source paths.

## On: Invoke

1. Inspect the visible task list for items whose names begin with `forge-mental-model-`.
2. If an incomplete matching item exists, resume **On: Per Domain** with the first incomplete item.
3. If no matching item exists, start **On: Cold Start**.

## On: Cold Start

### 1. Spec gate

1. Check whether `ai-docs/spec/` exists and contains at least one spec file.
2. If absent or empty, tell the user: `No spec found — mental-model will be built without spec stem cross-references. Run the forge-spec workflow first for full cross-reference support.`
3. Record whether specs are available for later stem inspection.
4. Do not block mental-model authoring when specs are absent.

### 2. Existing catalog lookup

1. Call MCP tool `ws/mental_models.list` to inspect existing mental-model domains and source mappings.
2. Use the catalog only as a coverage signal; do not extend stale content without re-surveying the code.

### 3. Parallel codebase survey

Call `ws/subquery` three times in the same response turn, each with `deep_research: true`:

1. Survey directory and module structure; return module or area names, responsibilities, outward-facing interfaces, and rough size signals.
2. Survey entry points and cross-module contracts; return orchestration paths, dependency directions, outputs, protocols, registries, and configuration schemas.
3. Survey coupling hotspots and implicit contracts; return involved modules, required ordering or data flow, and failure modes if violated.

Wait for all three survey results before synthesizing.

### 4. Synthesize domain candidates

1. Cross-reference module boundaries, entry points, coupling hotspots, and existing catalog coverage.
2. Produce one candidate domain per coherent operational area.
3. For each candidate, note source paths, coupling owned by the domain, and any existing mental-model file that may be replaced.

### 5. User domain confirmation

1. Present candidate domains as a numbered list.
2. Tell the user they may reorder, merge, split, rename, or drop entries.
3. Wait for the user's adjustments or confirmation.
4. Do not proceed until the user explicitly confirms the final domain list.

### 6. Register domain work

1. Create or update a visible task list with one item per confirmed domain in confirmed order.
2. Name each item `forge-mental-model-<domain>`.
3. Include the domain name, inferred source paths, spec availability, and existing file coverage in each item description.
4. Mark the first domain `in_progress` and proceed to **On: Per Domain**.

## On: Per Domain

For each visible task-list item named `forge-mental-model-<domain>`, in confirmed order, skip completed items.

### 1. Mark in progress

1. Mark the current domain task `in_progress` in the visible task list.
2. Read the domain task description for source paths, spec availability, and existing file coverage.

### 2. Domain survey

Call MCP tool `ws/subquery` with `deep_research: true` and a self-contained prompt using `Templates / Domain Survey Prompt`.

Wait for the survey result before drafting any domain file content.

### 3. Draft domain file

1. Call MCP tool `ws/convention.read` with `{"name":"mental-model-conventions"}` and read the result.
2. Apply the convention inclusion test to every candidate claim.
3. Draft the domain content in memory using `Templates / Domain File`.
4. Set frontmatter `domain` to the filename stem.
5. Set frontmatter `description` to a one-line operational scope summary.
6. Set frontmatter `sources` to directory-level patterns from the task description.
7. Set frontmatter `related` only for domains with real coupling notes.
8. Omit empty sections.

### 4. Embed spec stems when available

1. If specs are unavailable, skip this step and record `none (no spec found)` for the summary.
2. If specs are available, inspect `ai-docs/spec/` with available file or search capabilities for existing `{#YYMMDD-slug}` anchors.
3. Embed only stems that correspond to a concrete behavior discussed in the domain draft.
4. Do not call `ws/spec_stem.generate` or `ws/spec_index.verify` for stem listing.

### 5. Verify draft

1. Call MCP tool `ws/subquery` with `deep_research: true` and a self-contained prompt using `Templates / Verifier Prompt`.
2. Process verifier findings with `judge: verifier-finding`.
3. Apply required corrections to the in-memory draft.
4. Collect unresolved low-severity additions for the user summary.
5. Do not write the final file until the verifier pass is complete.

### 6. Write and commit domain file

1. Write the verified draft to `ai-docs/mental-model/<domain>.md` or the directory shape required by loaded conventions.
2. Commit the domain file change.
3. Include `(mental-model-updated)` in the commit message body.

### 7. Complete domain

1. Mark the current domain task `completed` in the visible task list.
2. If more incomplete domain tasks remain, continue with the next one.
3. When all domain tasks are complete, proceed to **On: Wrap Up**.

## On: Wrap Up

### 1. Update mental-model index

1. Update `ai-docs/mental-model.md` with rows for newly created or replaced domain files.
2. Update shared cross-domain conventions only when the verified domain work exposed a real repeated pattern.
3. Commit the index change.
4. Include `(mental-model-updated)` in the commit message body.

### 2. Summary report

Report using `Templates / Completion Report`.

### 3. Suggested next steps

1. Suggest running the forge-spec workflow if specs were absent.
2. Suggest using the configured mental-model update workflow after future code changes.

## Judgments

### judge: verifier-finding

| Severity | Action |
|----------|--------|
| `[HIGH]` | Correct the draft before writing; factual inversions and wrong names cannot remain. |
| `[LOW]` | Add the missing contract if it passes the inclusion test; otherwise report it for user review. |
| `[STALE]` | Rewrite or remove the stale claim before writing. |
| `[BLOAT]` | Remove the content; it fails the mental-model inclusion test. |

### judge: inclusion-test

Include a claim only when ignorance causes a wrong outcome and the fact is not derivable from entry-point files in under 30 seconds.

## Templates

### Domain Survey Prompt

```markdown
Analyze domain: <domain>
Source paths: <paths from task description>

Analyze this domain for a developer who needs to modify it.
Focus on facts whose absence would cause wrong outcomes:
1. Implicit contracts between modules, including ordering, data flow, and synchronization.
2. Coupling where changes in one place require changes elsewhere.
3. Extension points such as registries, enums, plugin interfaces, and configuration schemas.
4. Fragile areas, invariants that break silently, known debt, and planned scaffolds.
5. Common mistakes where forgetting a step causes a wrong outcome.
6. Distinctions between implemented patterns and scaffolded or planned features.

Be concrete: cite file paths, function names, specific types, and failure modes.
Do not produce type or field listings, function paraphrases, or exhaustive API inventories.
```

### Verifier Prompt

```markdown
Verify this mental-model domain draft against the codebase.

Domain file draft:
<full draft content>

Source paths to check:
<paths from task description>

For each claim in the draft, assign one severity:
- [HIGH] Factually wrong: misnames code, inverts dependency direction, or states an unenforced constraint.
- [LOW] Incomplete: a relevant operational contract or coupling is missing.
- [STALE] References removed code or an old API.
- [BLOAT] Fails the inclusion test: type listing, source paraphrase, or cheaply derivable content.

Return findings as bullets with severity, draft location, correction or removal, and evidence path.
```

### Domain File

```markdown
---
domain: <name>
description: "<one-line operational scope summary>"
sources:
  - <directory-pattern>/
related:
  <domain>: "<coupling or contract>"
---

# <Domain Name>

## Entry Points
- <2-3 files that are useful starting points, not an exhaustive listing>

## Module Contracts
- <component> guarantees <contract> to <consumer>; enforced by <mechanism or convention>.

## Coupling
- <A> ↔ <B>: coupled through <mechanism>; changing <A> requires <B> because <failure mode>.

## Extension Points & Change Recipes
- **Add or change <thing>**: touch <files>; preserve <contract>; avoid <pitfall>.

## Common Mistakes
- When changing <area>, forgetting <step> causes <wrong outcome>.

## Technical Debt
- <issue>: current state, impact, and possible improvement.
```

Omit empty sections. Omit `related` when no cross-domain coupling exists.

### Visible Task Item

```text
forge-mental-model-<domain> — <status>
  Domain: <domain>
  Source paths: <comma-separated module paths>
  Spec available: <yes | no>
  Existing mental-model file: <path, or none>
```

### Completion Report

```text
## Forge Mental Model — Complete

Domains covered: <N>
Domain files created: <list of paths>
Spec stems embedded: <count, or 'none (no spec found)'>
Verifier corrections applied: <count>
Items for user review: <LOW findings list, or 'none'>
```

## Doctrine

Forge-mental-model optimizes for modification-relevant operational knowledge per domain: every domain file records contracts, coupling, hazards, and change recipes that prevent wrong outcomes during future edits. When a rule is ambiguous, apply whichever interpretation better preserves dense operational knowledge while excluding type listings, source paraphrases, and cheaply derivable facts.
