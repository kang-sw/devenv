---
name: rebuild-mental-model
description: Rebuild or update the ai-docs/mental-model/ directory with operational knowledge for modifying the codebase. Delegates source exploration to subagents to keep the main context window small.
argument-hint: "[target domain or special instruction] (omit for full rebuild)"
---

# Rebuild Mental Model

Target: $ARGUMENTS

## What Mental Model Documents Are

Mental-model documents capture **operational knowledge for modifying the codebase**.

They are **not** API references, type listings, or source paraphrases.

They are:
- **Module contracts** — implicit invariants not enforced by the type system
- **Coupling maps** — which changes propagate where
- **Extension points** — where the code is designed to grow, and what to touch
- **Common mistakes** — silent-failure footguns
- **Technical debt** — known limitations, fragile areas

## Inclusion Test

Before recording any fact, apply this filter:

> "Would a developer cause a **silent failure** by not knowing this,
> AND is this NOT derivable from reading the entry point files in <30 seconds?"

- Both yes → **record** (contract, invariant, non-obvious coupling)
- Either no → **omit**

Never record:
- Type/struct field listings
- Function signatures or argument counts
- API route/endpoint enumerations
- "This module does X" descriptions that paraphrase source code
- Information already in `_index.md`

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
- **Right-sized.** Target 60–120 lines per domain. Split past 150 lines.
  Merge thin documents that are always read together.
- **Incremental by default.** Only rebuild affected domains unless a full
  rebuild is requested.

## Document Format

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

## Step 2: Write / update documents

Using subagent analyses, create or update `ai-docs/mental-model/` documents:
- Follow the document format above.
- Apply the inclusion test to every claim before writing it.
- Remove documents for domains that no longer exist.
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
- **[BLOAT]**: Remove — content fails inclusion test.

## Step 4: Update overview.md and _index.md

**overview.md**: Package graph, shared patterns, cross-domain contracts.

**_index.md**: Update crate-level descriptions and operational state only.
Do not duplicate module-level detail into `_index.md` — that belongs in
`lib.rs`/`mod.rs` entry files.

## Step 5: Summary

Report:
- Dirty scope: which domains were rebuilt and why
- Documents created / updated / removed
- Verifier results: corrections applied, items for manual review
