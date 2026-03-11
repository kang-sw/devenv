---
name: rebuild-mental-model
description: Rebuild or update the ai-docs/mental-model/ directory with operational knowledge for modifying the codebase. Delegates source exploration to subagents to keep the main context window small.
argument-hint: "[target domain or special instruction] (omit for full rebuild)"
---

# Rebuild Mental Model

Target: $ARGUMENTS

## What Mental Model Documents Are

Mental-model documents capture **operational knowledge for modifying the codebase** —
the kind of understanding a developer builds after working with the code for weeks.

They are NOT:
- API references (auto-generated docs, IDE navigation, or source reading handle this)
- Type/function/class listings
- Module-tree mirrors

They ARE:
- **Modification patterns**: "to add X, modify files A → B → C in this order"
- **Module contracts**: implicit invariants not enforced by the type system
- **Coupling maps**: which changes propagate where, and through what mechanism
- **Extension points**: where the code is designed to grow, and the protocol for extending
- **Technical debt**: known limitations, fragile areas, silent failure modes

The goal: a new session can load a domain document and immediately know how to
make changes without re-exploring the entire codebase.

## Principles

- **Do not read source directly.** Delegate all source exploration to subagents.
  Read source yourself only when a subagent summary is clearly insufficient.
  This keeps the main context window small.
- **Domain-oriented documents.** Each document covers a cross-cutting system/concern,
  not a source module or package. A single document may reference files across multiple
  packages/crates/modules. The directory is flat (no nesting):
  ```
  ai-docs/mental-model/
    overview.md          ← project-wide: package graph, shared patterns, cross-domain concerns
    <domain-a>.md        ← e.g., auth, networking, data-pipeline, rendering, ...
    <domain-b>.md
    ...
  ```
  Domain boundaries are project-specific. Determine them from the source — don't
  force a fixed list. A game project might have `combat.md`, `inventory.md`; a web
  app might have `auth.md`, `billing.md`, `api-gateway.md`.
- **Incremental by default.** Only rebuild domains affected by changed files, unless
  a full rebuild is explicitly requested.

## Document Format

Each domain document follows this structure:

```markdown
# [Domain Name]

## Overview
One paragraph: what this domain covers, where it sits in the system.

## Relevant Source Files
Table of key files with one-line role description. Helps the reader
know where to look for details beyond what's captured here.

## Modification Patterns
Concrete recipes for common change scenarios:
- **Add a new [X]**: file A (do Y) → file B (do Z) → ...
- **Change [behavior]**: primary logic at [location], ripple effects to [locations]
Each recipe cites an existing pattern to follow (e.g., "follow how FooHandler is registered").

## Module Contracts
Implicit invariants and assumptions between modules:
- "[A] guarantees [X] to [B]" — enforced by [mechanism] / NOT enforced (convention only)
- Data flow ordering assumptions
- Serialization compatibility rules

## Coupling
Which changes propagate where:
- A ↔ B: bidirectional through [mechanism]
- C → D: one-way, D is safe to modify independently
Focus on non-obvious coupling — skip obvious import dependencies.

## Extension Points
Where the code is designed to accept new things:
- [Registry/enum/interface/plugin system]: protocol for adding new entries
- Constraints: fixed-size arrays, hardcoded limits, sealed types

## Technical Debt
Known limitations and fragile areas:
- [Issue]: current state, impact, possible improvement
- [Fragile invariant]: what breaks if violated, how to avoid
```

Omit sections that have nothing meaningful to say. Never pad with obvious content.

## Step 0: Determine dirty scope

1. Check whether `ai-docs/mental-model/` already exists.
   - **Exists →** Find the last-committed date of mental-model documents
     (`git log -1 --format="%aI" -- ai-docs/mental-model/`). Collect source files
     changed since that date (`git diff --name-only <commit> HEAD`). Map changed
     files to affected domains — a single changed file may dirty multiple domains.
   - **Does not exist →** Full rebuild.
2. If `$ARGUMENTS` names a specific domain, only rebuild that domain (but still
   check cross-domain coupling — if domain A references patterns from domain B
   that changed, flag it).
3. If `$ARGUMENTS` contains special instructions, carry them forward.

## Step 1: Explore source (subagent-delegated)

Dispatch one subagent per dirty domain. Each subagent receives:
- The list of relevant source files for that domain
- The analysis directive (NOT "list types and functions" but rather):

> Analyze this domain for a developer who needs to modify it.
> For each relevant source file:
> 1. What modification patterns exist? (common change scenarios and their file-by-file path)
> 2. What implicit contracts exist between modules? (ordering, data flow, sync obligations)
> 3. What coupling exists? (changes here → must also change there)
> 4. Where are the extension points? (registries, enums, plugin interfaces, config)
> 5. What is fragile? (invariants that break silently, known debt)
> Be concrete: cite file paths, function names, specific types.

Run subagents in parallel. The number and grouping of agents is a judgment call —
optimize for thorough coverage while keeping the main context window small.

## Step 2: Write / update mental-model documents

Using subagent analyses, create or update documents under `ai-docs/mental-model/`.

- Follow the document format above.
- Every claim should be traceable to a specific file/function.
- If a domain no longer exists, remove its document. When uncertain, flag for the user.
- Prefer concrete examples over abstract descriptions. "Follow how X is registered
  in init()" is better than "add entries to the registry."
- Cross-reference other domain docs when relevant (e.g., "see [other-domain].md
  §[Section] for how this connects").

## Step 3: Update overview.md and _index.md

**overview.md**: Project-wide concerns that don't belong to a single domain:
- Package/module dependency graph
- Shared patterns and conventions used across the project
- Cross-domain modification recipes (e.g., "add a new feature end-to-end")

**_index.md**: Update the documentation reference section and operational state if needed.

## Step 4: Summary

Print for the user:
- Dirty scope: which domains were rebuilt and why
- Documents created / updated / removed
- Confidence notes: areas where the analysis may be incomplete — flag for manual review
