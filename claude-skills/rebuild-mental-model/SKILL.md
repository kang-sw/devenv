---
name: rebuild-mental-model
description: Rebuild or update the ai-docs/mental-model/ directory with operational knowledge for modifying the codebase. Delegates source exploration to subagents to keep the main context window small.
argument-hint: "[target domain or special instruction] (omit for full rebuild)"
---

# Rebuild Mental Model

Target: $ARGUMENTS

## What Mental Model Documents Are

Mental-model documents capture **operational knowledge for modifying the codebase**.

They are NOT API references or type listings.

They ARE:
- **Modification patterns**: "to add X, modify files A → B → C in this order"
- **Module contracts**: implicit invariants not enforced by the type system
- **Coupling maps**: which changes propagate where
- **Extension points**: where the code is designed to grow
- **Common mistakes**: silent-failure footguns
- **Technical debt**: known limitations, fragile areas

## Constraints

- **No direct source reading.** Delegate to subagents. Read source yourself only
  when a subagent summary is clearly insufficient.
- **Domain-oriented documents.** Each covers a cross-cutting concern, not a source
  module. The directory is flat:
  ```
  ai-docs/mental-model/
    overview.md          ← project-wide: package graph, shared patterns
    <domain-a>.md
    <domain-b>.md
  ```
- **Right-sized.** Split documents past ~200 lines or mixing unrelated concerns.
  Merge thin documents always read together.
- **Incremental by default.** Only rebuild affected domains unless full rebuild
  is requested.

## Document Format

```markdown
# [Domain Name]

## Overview
One paragraph: what this domain covers, where it sits in the system.

## Relevant Source Files
Table of key files with one-line role description.

## Modification Patterns
- **Add a new [X]**: file A (do Y) → file B (do Z) → ...
- **Change [behavior]**: primary logic at [location], ripple effects to [locations]
Mark recipes for unimplemented features as **(planned)**.

## Module Contracts
- "[A] guarantees [X] to [B]" — enforced by [mechanism] / convention only

## Coupling
- A ↔ B: bidirectional through [mechanism]
Focus on non-obvious coupling.

## Extension Points
- [Registry/enum/interface]: protocol for adding new entries

## Common Mistakes
- "When adding [X], forgetting [Y] → [silent failure]"
Focus on mistakes that fail silently.

## Technical Debt
- [Issue]: current state, impact, possible improvement
```

Omit empty sections.

## Step 0: Determine dirty scope

1. Check whether `ai-docs/mental-model/` exists.
   - **Exists →** Find the last commit that touched each domain document via
     `git log -1 --format="%H" -- ai-docs/mental-model/<file>`. Collect changed
     source files via `git diff --name-only <base> HEAD`. Map changed files to
     affected domains.
   - **Does not exist →** Full rebuild.
2. If `$ARGUMENTS` names a specific domain, only rebuild that domain (check
   cross-domain coupling too).
3. Carry forward any special instructions from `$ARGUMENTS`.

## Step 1: Explore source (subagent-delegated)

Dispatch one subagent per dirty domain with:
- The list of relevant source files
- This analysis directive:

> Analyze this domain for a developer who needs to modify it.
> For each relevant source file:
> 1. Modification patterns (common change scenarios, file-by-file path)
> 2. Implicit contracts between modules (ordering, data flow, sync)
> 3. Coupling (changes here → must also change there)
> 4. Extension points (registries, enums, plugin interfaces, config)
> 5. Fragile areas (invariants that break silently, known debt)
> 6. Common mistakes (forgetting required steps, silent failures)
> 7. Distinguish existing patterns from scaffolded/planned features.
> Be concrete: cite file paths, function names, specific types.

Run subagents in parallel.

## Step 2: Write / update documents

Using subagent analyses, create or update `ai-docs/mental-model/` documents:
- Follow the document format above.
- Every claim should be traceable to a specific file/function.
- Remove documents for domains that no longer exist.
- Prefer concrete examples over abstract descriptions.
- Cross-reference other domain docs when relevant.
- Tag unimplemented features as **(planned)**.

## Step 3: Verify (subagent-delegated)

Dispatch one **mental-model-verifier** agent per updated domain with:
- The full content of the document to verify
- `git log --oneline --stat` for relevant files (last 30 commits or since last update)

Process verifier output:
- **[HIGH]**: Apply corrections directly.
- **[LOW]**: Add if clearly relevant; otherwise collect for summary.
- **[STALE]**: Rewrite or remove the recipe.

## Step 4: Update overview.md and _index.md

**overview.md**: Package graph, shared patterns, cross-domain recipes.

**_index.md**: Update documentation references and operational state.

## Step 5: Summary

Report:
- Dirty scope: which domains were rebuilt and why
- Documents created / updated / removed
- Verifier results: corrections applied, items for manual review
