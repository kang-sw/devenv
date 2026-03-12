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
- **Common mistakes**: silent-failure footguns when modifying the domain
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
- **Right-sized documents.** A domain document that grows past ~200 lines or mixes
  concerns a developer would rarely need together is a candidate for splitting.
  Conversely, thin documents that are always read together can be merged. Optimize
  for the reader's working set.
- **Incremental by default.** Only rebuild domains affected by changed files, unless
  a full rebuild is explicitly requested.

## Document Format

Each domain document follows this structure:

```markdown
<!-- verified: <short-hash> (<YYYY-MM-DD>) -->
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

Mark recipes for features that don't exist yet as **(planned)** — these are
forward-looking guidance that should be revisited when the feature lands or
the design changes.

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

## Common Mistakes
Concrete "don't forget" warnings for frequent modification scenarios:
- "When adding [X], forgetting [Y] → [silent failure / crash / data corruption]"
Focus on mistakes that fail silently — compiler-caught omissions need no documentation.

## Technical Debt
Known limitations and fragile areas:
- [Issue]: current state, impact, possible improvement
- [Fragile invariant]: what breaks if violated, how to avoid
```

Omit sections that have nothing meaningful to say. Never pad with obvious content.

## Step 0: Determine dirty scope

1. Check whether `ai-docs/mental-model/` already exists.
   - **Exists →** Read the `<!-- verified: <hash> (<date>) -->` watermark from each
     domain document. For documents with a watermark, use that commit as the diff base.
     For documents without one, fall back to
     `git log -1 --format="%H" -- ai-docs/mental-model/`. Collect changed source files
     via `git diff --name-only <base> HEAD`. Map changed files to affected domains —
     a single changed file may dirty multiple domains.
   - **Does not exist →** Full rebuild.
2. If `$ARGUMENTS` names a specific domain, only rebuild that domain (but still
   check cross-domain coupling — if domain A references patterns from domain B
   that changed, flag it).
3. If `$ARGUMENTS` contains special instructions, carry them forward.

## Step 1: Explore source (subagent-delegated)

Dispatch one subagent per dirty domain. Each subagent receives:
- The list of relevant source files for that domain
- The analysis directive — focus on operational knowledge, not API listings:

> Analyze this domain for a developer who needs to modify it.
> For each relevant source file:
> 1. What modification patterns exist? (common change scenarios and their file-by-file path)
> 2. What implicit contracts exist between modules? (ordering, data flow, sync obligations)
> 3. What coupling exists? (changes here → must also change there)
> 4. Where are the extension points? (registries, enums, plugin interfaces, config)
> 5. What is fragile? (invariants that break silently, known debt)
> 6. What common mistakes would a developer make? (forgetting a required step,
>    changes that fail silently, easy-to-miss propagation sites)
> 7. Distinguish patterns that exist in the code today from scaffolded/planned features.
>    Mark planned features clearly.
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
- Tag recipes for not-yet-implemented features as **(planned)**.

## Step 3: Verify & watermark (subagent-delegated)

After writing/updating documents, dispatch one **verifier subagent** per updated
domain. The verifier cross-checks the written document against actual source and
recent git history, then reports corrections.

### Verifier subagent prompt

Read the prompt from `verifier-agent.md` in this skill's directory. Each verifier
subagent receives that prompt plus:
- The full content of the mental-model document to verify
- The output of `git log --oneline --stat` for files relevant to this domain
  (from the previous watermark commit to HEAD, or last 30 commits if no watermark)

### Processing verifier output

- **[HIGH]** corrections: apply directly to the document.
- **[LOW]** items: add if clearly relevant; otherwise collect for Step 5 summary.
- **[STALE]** items: rewrite the recipe to reflect current implementation, or remove
  if the recipe is now covered by a non-planned pattern.

### Watermark

After corrections are applied, set (or update) the watermark at the top of each
verified document:
```html
<!-- verified: <short-hash> (<YYYY-MM-DD>) -->
```

## Step 4: Update overview.md and _index.md

**overview.md**: Project-wide concerns that don't belong to a single domain:
- Package/module dependency graph
- Shared patterns and conventions used across the project
- Cross-domain modification recipes (e.g., "add a new feature end-to-end")

**_index.md**: Update the documentation reference section and operational state if needed.

## Step 5: Summary

Print for the user:
- Dirty scope: which domains were rebuilt and why
- Documents created / updated / removed
- Verifier results: corrections applied, low-confidence items and incomplete areas for manual review
