---
name: rebuild-mental-model
description: Rebuild or update the ai-docs/mental-model/ directory with operational knowledge for modifying the codebase. Delegates source exploration to subagents to keep the main context window small.
argument-hint: "[target domain or special instruction] (omit for full rebuild)"
---

# Rebuild Mental Model

Target: $ARGUMENTS

## Invariants

- Mental-model documents capture operational knowledge for modifying the codebase: module contracts, coupling maps, extension points, common mistakes, technical debt.
- Not API references, type listings, or source paraphrases — those are derivable from code.
- Inclusion test: record only facts where (a) ignorance causes silent failure AND (b) not derivable from reading entry-point files in <30 seconds. Both must hold.
- Never record: type/struct field listings, function signatures, API route enumerations, source-paraphrasing descriptions, information already in `_index.md`.
- No direct source reading — delegate to subagents. Read source yourself only when a subagent summary is clearly insufficient.
- Each document covers a cross-cutting concern, not a source module. Directory is flat: `ai-docs/mental-model/overview.md` + `<domain>.md` files.
- Target 60–120 lines per domain. Split past 150; merge thin documents read together.
- Incremental by default — only rebuild affected domains unless full rebuild is requested.

## On: invoke

### 1. Determine dirty scope

1. Check whether `ai-docs/mental-model/` exists.
   - **Exists →** Find last commit per domain doc via
     `git log -1 --format="%H" -- ai-docs/mental-model/<file>`. Collect changed
     source files via `git diff --name-only <base> HEAD`. Map changed files to
     affected domains.
   - **Does not exist →** Full rebuild.
2. If `$ARGUMENTS` names a specific domain, only rebuild that domain (check
   cross-domain coupling too).
3. Carry forward any special instructions from `$ARGUMENTS`.

### 2. Explore source (subagent-delegated)

Dispatch one subagent per dirty domain with:
- The entry point files AND files changed in the dirty scope
- This analysis directive:

> Analyze this domain for a developer who needs to modify it.
> Focus on what would cause silent failures if unknown:
> 1. Implicit contracts between modules (ordering, data flow, sync)
> 2. Coupling (changes here → must also change there)
> 3. Extension points (registries, enums, plugin interfaces, config)
> 4. Fragile areas (invariants that break silently, known debt)
> 5. Common mistakes (forgetting required steps, silent failures)
> 6. Distinguish existing patterns from scaffolded/planned features.
> Be concrete: cite file paths, function names, specific types.
> Do NOT produce type/field listings or paraphrase what functions do.

Run subagents in parallel.

### 3. Write / update documents

Using subagent analyses, create or update `ai-docs/mental-model/` documents:
- Follow the `document-format` template.
- Apply the inclusion test to every claim before writing it.
- Remove documents for domains that no longer exist.
- Cross-reference other domain docs when relevant.
- Tag unimplemented features as **(planned)**.

### 4. Verify (subagent-delegated)

Dispatch one **mental-model-verifier** agent per updated domain with:
- The full content of the document to verify
- `git log --oneline --stat` for relevant files (last 30 commits or since last update)

Process verifier output:
- **[HIGH]**: Apply corrections directly.
- **[LOW]**: Add if clearly relevant; otherwise collect for summary.
- **[STALE]**: Rewrite or remove the recipe.
- **[BLOAT]**: Remove — content fails inclusion test.

### 5. Update overview.md and _index.md

**overview.md**: Package graph, shared patterns, cross-domain contracts.

**_index.md**: Update project-level descriptions and operational state only.
Do not duplicate module-level detail into `_index.md` — that belongs in
entry-point files (e.g. `mod.rs`, `index.ts`, `__init__.py`).

### 6. Summary

Report:
- Dirty scope: which domains were rebuilt and why
- Documents created / updated / removed
- Verifier results: corrections applied, items for manual review

## Templates

### document-format

```markdown
# [Domain Name]

## Entry Points
2-3 key files that serve as starting points for understanding this domain.
NOT an exhaustive file listing — just "where to start reading."

## Module Contracts
- "[A] guarantees [X] to [B]" — enforced by [mechanism] / convention only

## Coupling
- A ↔ B: bidirectional through [mechanism]
Focus on non-obvious coupling.

## Extension Points & Change Recipes
- [Registry/enum/interface]: protocol for adding new entries
- **Add a new [X]**: key files to touch + critical pitfalls
- **Change [behavior]**: key files + ripple effects + pitfalls
Only non-obvious multi-file changes. Mark unimplemented features **(planned)**.

## Common Mistakes
- "When adding [X], forgetting [Y] → [silent failure]"
Focus on mistakes that fail silently.

## Technical Debt
- [Issue]: current state, impact, possible improvement
```

Omit empty sections.

## Doctrine

Mental-model documents exist so that a developer modifying the codebase
does not cause silent failures from ignorance of implicit contracts.
Every authoring choice optimizes for **modification-relevant knowledge
density**: only facts that pass the inclusion test (silent-failure risk
AND not quickly derivable from source) earn space. When a rule is
ambiguous, apply whichever interpretation better preserves the density
of modification-relevant knowledge while excluding derivable content.
