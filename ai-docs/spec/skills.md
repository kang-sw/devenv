---
title: Skill System
summary: User-invocable workflow skills that power AI-assisted development in downstream projects.
features:
  - Workflow Skills
    - Session Bootstrap
    - Discussion
    - Ticket Authoring
    - Spec Authoring
    - Skeleton Authoring
    - Planning
    - Implementation
    - Parallel Implementation
    - Auto-Routing
  - Maintenance Skills
    - Mental Model
    - Spec Maintenance
    - Bootstrap

stems:
---

# Skill System

The devenv skill system is a set of user-invocable `/commands` that orchestrate AI-assisted
development workflows. Each skill is a self-contained procedure; skills may suggest chaining
to each other but do not auto-chain without user approval.

Skills are authored in this repo and symlinked into downstream projects via `~/.claude/skills/`.

## Workflow Skills

Used in sequence as a ticket moves through its lifecycle.

### Session Bootstrap

`/enter-session [optional context]` — Synthesizes recent work, active tickets, and session
state into a compact briefing. Invoke at session start or after context loss.

Produces a structured Briefing with `### Context` and `### Recommended next` sections.
The recommended skill is a `/`-prefixed token; the user decides whether to follow it.

Two execution paths:
- **Resume** — if a continuation file exists and HEAD matches, fast-paths from the prior
  session's pre-synthesized payload.
- **Bootstrap** — forks a clerk subagent to scan git log and active tickets; emits a
  fresh briefing.

### Discussion

`/discuss [topic, ticket path, or question]` — Iterative brainstorming over approach or
direction. Reads spec docs and mental-model docs on-demand as topics emerge. Concludes by
offering to persist conclusions via `/write-ticket` or `/write-mental-model`.

No source edits during discussion. Implementation details are researched via subagents,
not direct source reads. The Project Map (tickets + spec inventory) is injected at entry.

### Ticket Authoring

`/write-ticket [topic or existing ticket path]` — Creates or edits a ticket under
`ai-docs/tickets/`. Each ticket captures the full decision context for its phases: goals,
constraints, rationale, rejected alternatives, suggested approaches (pseudo-code, data
shapes). Codebase-derived details stay in plans, not tickets.

After authoring, prompts whether a spec update is warranted for public-facing changes.
Delegates routine status moves and cross-ticket inspection to a clerk subagent.

### Spec Authoring

`/write-spec [area name or spec file path]` — Creates or updates external-perspective spec
documents under `ai-docs/spec/`. Describes what the system does from a caller's viewpoint,
not how it is built.

Key annotation conventions:
- `### 🚧 Feature [ticket-stem/pN]` — unimplemented; linked to the tracking ticket phase.
- `> [!note] Planned 🚧 [ticket-stem/pN]` — planned change to an existing feature.
- No `🚧` marker — implemented and verified against the codebase.

Directory structure: one directory per area when multiple sub-docs are needed;
flat file for self-contained surfaces. `spec-build-index` regenerates the `features:`
frontmatter after every write.

### Skeleton Authoring

`/write-skeleton [ticket-path]` — Writes public interface stubs and integration tests
before any implementation code. Establishes locked contracts that plan and implementation
must satisfy. Stub tests fail by design — do not run before implementation.

The skeleton commit is the acceptance-criteria baseline for the subsequent implementation.

### Planning

`/write-plan [ticket-path or description]` — Researches the codebase and produces
implementation guidance. Two modes:

- **Survey** — reconnaissance; discovers patterns, names types, maps integration points.
  Reusable across sessions.
- **Deep** — full architectural brief; self-contained for a fresh implementer. One per
  ticket phase; takes precedence over ticket sketches where they diverge.

Plan file is committed before implementation begins. Lead never reads source directly —
all exploration is subagent-delegated.

### Implementation

`/implement <plan-path or inline brief> [--ticket <stem>]` — Delegates a single-scope
implementation cycle to an implementer+reviewer pair. Lead does not read source or write
implementation code.

Acceptance criteria: skeleton stubs compile and integration tests pass. User approves the
reviewer's report before merge. A mental-model update runs automatically after approval.

### Parallel Implementation

`/parallel-implement <ticket/plan/scope description>` — Runs multiple implementer+reviewer
pairs concurrently across disjoint scopes. Scopes must share no modified files. Lead
serializes commits after all pairs report; user approves the aggregate report before merge.

Use when the skeleton defines two or more independent scopes with separate test paths.

### Auto-Routing

`/proceed <ticket or description>` — Assesses available artifacts and routes through
the canonical pipeline automatically. Announces the chosen path before executing.

Pipeline order: skeleton → (optional) plan → implement or parallel-implement.
Stops and prompts if the scope is too vague for routing. Does not read source directly.

## Maintenance Skills

Invoked reactively when project state needs to be updated.

### Mental Model

`/write-mental-model [domain or "rebuild"]` — Rebuilds or updates internal architecture
documentation under `ai-docs/mental-model/`. Applies the inclusion test from
`.claude/infra/mental-model-conventions.md`. Dispatches one subagent per affected domain
in parallel.

Invoke after implementation changes that affect architecture, coupling, or extension points.

### Spec Maintenance

`/write-spec` (see above) also handles spec updates post-implementation: remove `🚧` from
confirmed-implemented features, add `Planned` callouts for newly-planned changes.

The `spec-updater` agent (spawned from write-ticket or manually) strips `🚧` markers
when their linked ticket phases complete and flags bare `🚧` entries with no ticket link.

### Bootstrap

`/bootstrap [fresh | upgrade]` — Scaffolds a new project or upgrades an existing one
to match the canonical `CLAUDE.template.md`. Auto-detects mode:

- **Fresh** — no `CLAUDE.md` exists; full scaffold.
- **Upgrade** — version tag present; applies migration checklist items above current version.
- **Adopt** — no version tag; audits which items apply, then adds the tag.

Merges surgically — never overwrites project-specific sections (Architecture Rules,
custom standards). Flags conflicts inline with `<!-- CONFLICT: ... -->`.
