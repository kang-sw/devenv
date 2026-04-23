# Mental Model Index

Cross-domain patterns and shared conventions for the devenv workflow system.

## Domains

| Domain | File | Scope |
|--------|------|-------|
| spec-system | `mental-model/spec-system.md` | Spec stems, anchors, frontmatter tools |
| workflow-routing | `mental-model/workflow-routing.md` | /proceed routing contracts, prefix-stage delegation, write-ticket artifact protocol |
| executor-wrapup | `mental-model/executor-wrapup.md` | Shared wrapup playbook for executor-series skills: doc pipeline, commit gate, ticket update |
| doc-tooling | `mental-model/doc-tooling.md` | Mental-model authoring toolchain: forge-mental-model and mental-model-updater contracts, task-naming resume mechanism, commit-stamp checkpoint |

## Shared Conventions

**Stem format:** `{#YYMMDD-slug}` — six-digit date prefix, hyphen, lowercase slug.
This regex (`\{#\d{6}-[\w-]+\}`) is the shared protocol between `generate-spec-stem`,
`list-stems`, and `spec-build-index`. A format change requires updating all three.

**Stem storage:** anchors live in spec document body text only — never in frontmatter.
Any code that looks for stems must grep document content, not parse YAML.
