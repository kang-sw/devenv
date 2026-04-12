# Mental-Model Conventions

Canonical reference for mental-model document content, format, and doctrine.

## Definition

Mental-model documents capture operational knowledge for modifying the codebase:
module contracts, coupling maps, extension points, common mistakes, technical debt.

Not API references, type listings, or source paraphrases — those are derivable from code.

## Inclusion Test

Record a fact only when **both** conditions hold:

1. Ignorance causes a wrong outcome.
2. Not derivable from reading entry-point files in under 30 seconds.

**Never record:** type/struct field listings, function signatures, API route enumerations,
source-paraphrasing descriptions, information already in `_index.md`.

## Structure

- Index: `ai-docs/mental-model.md` — cross-domain patterns, crate graph, shared conventions. No frontmatter.
- Domain docs: `ai-docs/mental-model/<domain>.md` — flat directory, one file per cross-cutting concern.
- Target 60–120 lines per domain. Split past 150; merge thin documents that are always read together.

## Frontmatter

Every domain file begins with YAML frontmatter:

```yaml
---
domain: <name>
description: "<one-line summary of what this domain covers>"
sources:
  - <directory-pattern>/
related:
  <domain-name>: "<one-line coupling or contract description>"
---
```

- `domain`: matches the filename stem (e.g., `auth` for `auth.md`).
- `description`: one-line summary of the domain's scope, for quick orientation.
- `sources`: directory-level patterns only — no file paths. The updater maintains this field.
- `related`: optional map of domain name → one-line coupling/contract note. Omit unrelated domains.

## Commit Stamp

Every commit that updates mental-model documents must include `(mental-model-updated)` in
the commit message body. This marks the checkpoint for future updater runs.

## Document Format

```markdown
---
domain: <name>
description: "<one-line summary>"
sources:
  - <directory-pattern>/
related:
  <domain>: "<coupling or contract>"
---

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
- "When adding [X], forgetting [Y] → [wrong outcome]"
Focus on mistakes that cause wrong outcomes.

## Technical Debt
- [Issue]: current state, impact, possible improvement
```

Omit empty sections. Omit `related` from frontmatter when no cross-domain coupling exists.

## Doctrine

Mental-model documents exist so that a developer modifying the codebase
does not produce wrong outcomes from ignorance of implicit contracts.
Every authoring choice optimizes for **modification-relevant knowledge
density**: only facts that pass the inclusion test (wrong-outcome risk
AND not quickly derivable from source) earn space. When a rule is
ambiguous, apply whichever interpretation better preserves the density
of modification-relevant knowledge while excluding derivable content.
