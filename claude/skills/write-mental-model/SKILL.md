---
name: write-mental-model
description: >
  Rebuild or update ai-docs/mental-model/ with operational knowledge for
  modifying the codebase. Delegates source exploration to subagents to keep
  the main context window small. Format and doctrine: .claude/infra/mental-model-conventions.md.
argument-hint: "[target domain or special instruction] (omit for full rebuild)"
---

# Write Mental Model

Target: $ARGUMENTS

## Invariants

- Mental-model conventions: `.claude/infra/mental-model-conventions.md` — inclusion test, document format, sizing, doctrine.
- No direct source reading — all source exploration is subagent-delegated.
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
> Focus on what would cause wrong outcomes if unknown:
> 1. Implicit contracts between modules (ordering, data flow, sync)
> 2. Coupling (changes here → must also change there)
> 3. Extension points (registries, enums, plugin interfaces, config)
> 4. Fragile areas (invariants that break silently or cause wrong results, known debt)
> 5. Common mistakes (forgetting required steps, wrong outcomes)
> 6. Distinguish existing patterns from scaffolded/planned features.
> Be concrete: cite file paths, function names, specific types.
> Do NOT produce type/field listings or paraphrase what functions do.

Run subagents in parallel.

### 3. Write / update documents

Using subagent analyses, create or update `ai-docs/mental-model/` documents:
- Follow the document format in `.claude/infra/mental-model-conventions.md`.
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

## Doctrine

Write-mental-model optimizes for **minimal, accurate diffs to `ai-docs/mental-model/`** — subagent delegation keeps the main context free for authoring judgment; incremental scope prevents churn; the inclusion test in `.claude/infra/mental-model-conventions.md` filters every claim. When a rule is ambiguous, apply whichever interpretation produces the smallest correct change that passes the inclusion test.
