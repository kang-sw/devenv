# Mental Model Index

Cross-domain patterns and shared conventions for the devenv workflow system.

## Domains

| Domain | File | Scope |
|--------|------|-------|
| spec-system | `mental-model/spec-system.md` | Spec stems, anchors, frontmatter tools |
| workflow-routing | `mental-model/workflow-routing.md` | /proceed routing contracts, prefix-stage delegation, write-ticket artifact protocol |
| executor-wrapup | `mental-model/executor-wrapup.md` | Shared wrapup playbook for executor-series skills: doc pipeline, commit gate, ticket update |
| doc-tooling | `mental-model/doc-tooling.md` | Mental-model authoring toolchain: forge-mental-model, mental-model-updater (forge authority, Domain Rules handling, Stale Rules output), and add-rule (rule classification and routing) |
| tools | `mental-model/tools.md` | Standalone binary tools built in tools/: claude-watch (TUI session viewer) and claude-dash (worktree PTY multiplexer); coupling to named-agent registry |

## Directory Hierarchy

Domain docs live in one of two shapes:

- **Flat file** — `mental-model/<domain>.md`. Single-concern domain.
- **Directory** — `mental-model/<domain>/index.md` + `mental-model/<domain>/<sub>.md`. Multi-concern domain; `index.md` carries cross-cutting context and inherited `## Domain Rules`, each child file covers one sub-concern.

Promotion from flat to directory is triggered by code-structure change
observed in the diff — the `mental-model-updater` agent splits a flat doc
when the underlying module directory splits. The hierarchy is encoded in
the file path; no frontmatter `parent:` link is maintained.

Ancestor loading is an invariant: any agent loading a sub-domain doc
must also load the parent `index.md` before starting work, so inherited
Domain Rules are visible. See `claude-plugin/infra/mental-model-conventions.md` for the full contract.

## Domain Rules

Each domain doc may carry a `## Domain Rules` section holding user-authored
prescriptions for AI agents working in that domain. Rules are scoped to the
domain — analogous to `## Architecture Rules` in `CLAUDE.md`, but local.
They are authored via `/add-rule` or manual edit. No agent modifies rule
content autonomously; `mental-model-updater` may only promote them upward
during splits, or flag them as stale in its output.

## Shared Conventions

**Stem format:** `{#YYMMDD-slug}` — six-digit date prefix, hyphen, lowercase slug.
This regex (`\{#\d{6}-[\w-]+\}`) is the shared protocol between `ws-generate-spec-stem`,
`ws-list-spec-stems`, and `ws-spec-build-index`. A format change requires updating all three.

**Stem storage:** anchors live in spec document body text only — never in frontmatter.
Any code that looks for stems must grep document content, not parse YAML.
