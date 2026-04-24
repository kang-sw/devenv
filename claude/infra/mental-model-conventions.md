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
- Domain docs: `ai-docs/mental-model/<domain>.md` — flat file for a single-concern domain.
- Sub-domain docs: `ai-docs/mental-model/<domain>/index.md` + `ai-docs/mental-model/<domain>/<sub>.md` — directory layout for a domain covering multiple sub-concerns.
- Target 60–120 lines per domain. Split past 150; merge thin documents that are always read together.

## Directory Hierarchy

When a domain grows to multiple distinct sub-concerns, promote the flat
`<domain>.md` to a directory: `<domain>/index.md` carries cross-cutting
context and inherited `## Domain Rules`; each `<domain>/<sub>.md` covers
one sub-concern. This mirrors the `ai-docs/spec/<area>/` layout introduced
in template v0022.

Example:

```
ai-docs/mental-model/
  spec-system.md              # flat — single-concern domain
  workflow-routing.md
  doc-tooling/
    index.md                  # parent — cross-cutting context + promoted Domain Rules
    mental-model-updater.md   # sub-concern
    forge-mental-model.md     # sub-concern
```

Promotion from flat to directory is driven by code-structure change, not
authorial preference — the `mental-model-updater` agent splits a flat doc
when the diff shows the underlying module splitting into sub-directories.

### Ancestor loading (invariant)

Any agent loading `mental-model/<domain>/<sub>.md` MUST also load
`mental-model/<domain>/index.md` before starting work. Ancestor docs are
loaded first, so inherited `## Domain Rules` are visible before any edit
or reasoning begins. A load that skips the ancestor violates this contract.

The hierarchy is encoded in the file path — no frontmatter `parent:` link
is maintained. `list-mental-model` renders sub-domains indented under their
parent to surface the relationship to callers.

## Domain Rules

A domain doc may carry a `## Domain Rules` section containing user-authored
prescriptions for AI agents working in that domain. Rules describe patterns
the agent must follow when implementing code in this domain — analogous to
`## Architecture Rules` in `CLAUDE.md`, but scoped to the domain the doc
covers.

Authoring and modification invariants:

- Rules are authored via `/add-rule` or manual user edit.
- No agent modifies rule content autonomously.
- `mental-model-updater` may **promote** rules upward during splits
  (sub-domain → parent `index.md`) — position changes only, never content
  changes, and never downward.
- When a rule appears inconsistent with current code behavior,
  `mental-model-updater` flags it in a `## Stale Rules` output block — it
  does not edit the rule. The user resolves via `/add-rule` or manual edit.
- Ancestor `index.md` rules apply transitively to every sub-domain beneath
  it. Loading a sub-domain doc without the ancestor would miss inherited
  rules; see Ancestor loading invariant above.

Example:

```markdown
## Domain Rules

- All auth flows go through `AuthService.login` — no direct token issuance.
- New storage backends register via `StorageRegistry.register(name, impl)`.
```

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

## Spec Cross-References

When a mental-model domain covers behavior that has a corresponding spec entry, reference the spec stem inline in body text (e.g., `{#260421-spec-stem}`). This makes the spec entry discoverable from the domain doc.

- Grep for `{#stem}` across `ai-docs/mental-model/` to find which domain documents a given spec entry. No back-reference in spec files is needed — one-directional reduces dual-maintenance risk.
- When a spec stem is renamed (`renamed-spec: old → new` commit convention), any mental-model file referencing the old stem must be updated in the same commit.

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
