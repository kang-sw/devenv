---
title: Workflow Skills
summary: User-invocable /commands that orchestrate AI-assisted development workflows in downstream projects.
features:
  - Canonical Workflow Chain
  - Session Skills
    - Session Bootstrap — `/enter-session`
    - Session Sealing — `/exit-session`
  - Planning Skills
    - `/discuss`
    - `/write-spec`
    - `/write-ticket`
    - `/write-skeleton`
    - `/write-plan`
    - `/write-mental-model`
  - Implementation Skills
    - `/edit`
    - `/implement`
    - `/parallel-implement`
    - Pre-invocation Context Survey — `project-survey`
    - `/proceed`
  - Reconstruction
    - `/forge-spec`
    - `/forge-mental-model`
  - Utility Skills
    - 🚧 `/add-rule`
    - `/ship`
    - `/team-lead`
    - `/bootstrap`
---

# Workflow Skills

The `ws` plugin provides a set of user-invocable `/commands` covering the full development workflow: session bootstrap, planning, spec authoring, implementation, and release. Skills suggest chaining but never auto-chain — each step requires explicit user invocation or the `/proceed` auto-router.

## Canonical Workflow Chain {#260421-workflow-chain}

The standard pipeline runs in this order:

```
/discuss → /proceed
               ↓
/write-spec → /write-ticket
                                             ↓
                   /write-skeleton? → /write-plan? → /edit
                                                  → /implement
                                                  → /parallel-implement
```

Each skill recommends the next step at completion. `/proceed` is the auto-router invoked after `/write-ticket`; it selects and chains skeleton, plan, and implementation stages based on existing artifacts and session context.

> [!note] Constraints
> - Skills do not auto-chain — user invocation or `/proceed` is always required.
> - `/write-spec` is optional: the `judge: spec-impact` gate inside it exits early when no public behavior is affected.
> - `/write-skeleton` is optional: required only when public contracts need crystallization before implementation.
> - `/write-plan` is optional: `/proceed` skips it when the scope is small or the session is warm.

## Session Skills

### Session Bootstrap — `/enter-session` {#260421-enter-session}

Bootstraps main-agent context at session start and emits a structured `## Briefing` containing:
- `### Context` — branch name, recent work summary, active ticket, and open threads.
- `### Recommended next` — a backtick-quoted skill invocation with a one-line reason.

Two paths: **fast-path** when `ai-docs/_continue.local.md` matches the current HEAD (reads the continuation file directly); **bootstrap** otherwise (forks a `clerk` subagent to synthesize context from git log and active tickets).

> [!note] Constraints
> - Produces no source edits or file writes — context synthesis only.

### Session Sealing — `/exit-session` {#260421-exit-session}

Seals the session by writing compressed working memory to `ai-docs/_continue.local.md` with a HEAD hash header. The payload covers four sections: mental state, next concrete step, open threads, and pending user directives. Optionally creates a WIP auto-commit for uncommitted changes.

The file is consumed by `/enter-session`'s fast-path on the next session start.

> [!note] Constraints
> - Overwrites `_continue.local.md` — previous continuation is discarded.
> - `.local.md` is gitignored; continuation state is machine-local.

## Planning Skills

### `/discuss` {#260421-discuss}

Facilitates exploratory discussion of approach or direction. Reads spec, mental-model, and ticket files on demand; dispatches Explore subagents for codebase questions. Produces no source edits.

At the end of a discussion turn, always suggests `/write-spec` as the next step. Also offers `/write-ticket` to capture decisions as a ticket.

When a mental-model domain file is read, the skill checks its last commit date via `git log -1 --format="%ai" -- ai-docs/mental-model/<domain>.md`. If the date is more than 90 days ago, the skill surfaces a staleness warning for that domain. No frontmatter field is needed; git history is the source. {#260423-discuss-mental-model-staleness-warning}

> [!note] Constraints
> - No source files are created or modified during a `/discuss` session.

### `/write-spec` {#260421-write-spec}

Creates or updates a spec file under `ai-docs/spec/` describing caller-visible behavior with anchor-keyed entries. Entry format: `## Feature Name {#YYMMDD-slug}` for implemented features; `## 🚧 Feature Name {#YYMMDD-slug}` for planned ones.

A `judge: spec-impact` gate fires first: if the topic has no caller-visible behavioral change, the skill exits immediately without writing anything.

After writing, runs `spec-build-index` to rebuild the `features:` frontmatter index. Applies a `judge: directory-vs-flat` to decide between a flat file and a directory structure.

> [!note] Constraints
> - Never includes ticket references in `🚧` markers — implementation traceability flows through commit `## Spec` sections referencing the spec-stem.
> - Never edits the `features:` frontmatter block manually — `spec-build-index` owns it.

### `/write-ticket` {#260421-write-ticket}

Creates or edits a ticket file under `ai-docs/tickets/`. Captures scope, phases, constraints, and rejected alternatives. Optionally adds a `spec:` frontmatter field listing spec-stems the ticket implements. Always suggests `/proceed` after authoring, which auto-routes to skeleton, plan, or implementation.

Status directories: `idea/` → `todo/` → `wip/` → `done/` (or `dropped/`). Ticket stem format: `YYMMDD-type-slug`.

> [!note] Constraints
> - Does not advance a ticket's status directory — status changes happen via `git mv` during implementation.

### `/write-skeleton` {#260421-write-skeleton}

Crystallizes public contracts as interface stubs and integration tests before implementation begins. The lead identifies contract directives (type signatures, API shapes, invariants), delegates writing to a subagent, reviews, and commits. Updates the ticket `skeletons:` frontmatter with the commit hash.

Suggests `/edit`, `/implement`, or `/parallel-implement` as the next step based on scope width and session warmth — does not auto-invoke.

> [!note] Constraints
> - Stub files must compile; integration tests are written but not required to pass (implementation is absent).
> - The lead does not write skeleton code directly — subagent delegation only.

### `/write-plan` {#260421-write-plan}

Researches the codebase and produces a committed plan file at `ai-docs/plans/YYYY-MM/DD-hhmm.<name>.md`. Three modes:

- **Warm** — lead drafts from session context, reading source as needed.
- **Survey** — a survey-writer subagent explores the codebase; lead drafts from the output.
- **Deep** — a plan-writer subagent handles full exploration and drafting; lead reviews and accepts or rejects.

Updates the ticket `plans:` frontmatter with the plan path. `/proceed` passes the plan path directly to the next implementation skill.

### `/write-mental-model` {#260421-write-mental-model}

Rebuilds or updates `ai-docs/mental-model/` with operational knowledge for modifying the codebase. Delegates all source exploration to subagents. After writing, updates `ai-docs/mental-model.md` (the index) and `ai-docs/_index.md`.

> [!note] Constraints
> - Describes operational knowledge for code modifiers, not behavior for end users — contents are not caller-visible spec material.


## Implementation Skills

### `/edit` {#260422-edit-skill}

Owner-direct single-scope implementation: the main agent reads source, makes edits, verifies via build and test commands, and commits — no subagent delegation for the implementation itself. Best suited for warm sessions with well-understood, narrow scope.

After implementation, dispatches `spec-updater` first and waits for it to commit, then dispatches `mental-model-updater` and waits. The sequential order ensures `mental-model-updater`'s spec-diff check captures any 🚧 strips committed by `spec-updater`. If `spec-updater` reports ambiguous stems, surfaces them to the user. Then updates ticket status and `_index.md`, and emits a completion report. {#260423-doc-pipeline-spec-updater}

> [!note] Implementation Gap · 2026-04-23
> Doc pipeline output (spec-updater, mental-model-updater changes, `_index.md` refresh, ticket status update) is dispatched and awaited, but the resulting file changes are not guaranteed to be committed before the skill exits.

> [!note] Constraints
> - Escalates to `/implement` when scope grows beyond direct-edit bounds.
> - Suggests `/write-skeleton` when none exist and scope warrants them, but does not auto-invoke it.

### `/implement` {#260422-implement-skill}

Delegated implementation cycle: an implementer subagent writes code; two review-partition subagents (correctness + fit) review in parallel; the lead merges and runs the doc pipeline. Suited for cold sessions or wide-scope work.

Review partitions:
- **Correctness** — logic, error paths, contracts, security.
- **Fit** — conventions, naming, reuse, patterns.

Two invocation modes based on the current branch: **main-branch mode** (invoked from `main`/`master`/`trunk`) presents the user approval gate before merging; **feature-branch mode** (invoked from any other branch) skips the gate and auto-merges after a clean review. The feature → main merge remains the user's responsibility in feature-branch mode. Use `--main-branch <name>` to override the default main-branch names. {#260422-implement-feature-branch-mode}

Pre-merge, dispatches `spec-updater` first and waits for it to commit, then dispatches `mental-model-updater` and waits. Ambiguous stems from `spec-updater` are surfaced at the report/approval gate before merge proceeds.

> [!note] Implementation Gap · 2026-04-23
> Pre-merge doc pipeline output (spec-updater and mental-model-updater file changes) is not guaranteed to be committed before the merge step runs.

### `/parallel-implement` {#260421-parallel-implement}

Parallel implementation across N disjoint scope units. Spawns one implementer+reviewer pair per scope; lead serializes all build and test execution requests; lead commits each scope sequentially after all reviewers report clean.

Two invocation modes based on the current branch: **main-branch mode** (invoked from `main`/`master`/`trunk`) presents the user approval gate after all scopes are committed; **feature-branch mode** (invoked from any other branch) skips the gate and merges directly. The feature → main merge remains the user's responsibility in feature-branch mode. Use `--main-branch <name>` to override the default main-branch names.

Requires disjoint file sets — overlapping scopes cause merge conflicts.

Pre-merge (before the report/approval gate), dispatches `spec-updater` first and waits for it to commit, then dispatches `mental-model-updater` and waits. Ambiguous stems are surfaced at the report/approval gate before merge. After merge, the doc pipeline retains only `_index.md` refresh and ticket status update.

> [!note] Implementation Gap · 2026-04-23
> Both pre-merge doc pipeline output (spec-updater, mental-model-updater changes) and post-merge output (`_index.md` refresh, ticket status update) are not guaranteed to be committed before the skill exits.

> [!note] Constraints
> - Only one build/test command runs at a time (lead-serialized) — concurrent implementers share the working tree.

### Pre-invocation Context Survey — `project-survey` {#260424-project-survey-auto-invoke}

At the start of each run, `/edit`, `/implement`, `/parallel-implement`, and `/discuss` auto-invoke the `project-survey` agent with the implementation brief or query. The agent returns a `[Must|Maybe]`-tiered reference list of relevant documentation the implementer should read before starting work.

- **`[Must]`** — spec entries, mental-model sections, or active tickets directly covering behavior, patterns, or constraints required before starting.
- **`[Maybe]`** — tangentially related documents; useful when uncertain.

Search scope is limited to `ai-docs/spec/`, `ai-docs/mental-model/`, and active ticket directories (`idea/`, `todo/`, `wip/`). Source code references are out of scope — `/write-plan` survey-mode covers that gap.

The survey fires transparently — no additional caller invocation is required.

> [!note] Constraints
> - `done/` and `dropped/` ticket directories are excluded from the search scope.
> - Source code file references are not produced by this agent.

### `/proceed` {#260421-proceed}

Auto-router: assesses an implementation target and chains to the correct pipeline without executing implementation steps itself. Judges:

- **Context warmth** — warm session routes to `/edit`; cold routes to `/implement`.
- **Existing artifacts** — skips `/write-skeleton` and `/write-plan` if already present.
- **Scope width** — wide or parallelizable scope routes to `/parallel-implement`.
- **Direct-edit** — trivial changes skip the full pipeline and route directly to `/edit`.

Announces the chosen path before invoking the first skill.

> [!note] Constraints
> - Stops and asks for clarification when scope is too vague to route.

> [!note] Extended behavior — full-pipeline routing {#260422-proceed-full-pipeline-routing}
> Two prefix judges fire before the existing pipeline judges:
> - **`judge: needs-spec`** — always invokes `/write-spec`; its own `judge: spec-impact` gate exits without writing if no spec work is needed.
> - **`judge: needs-ticket`** — auto-invokes `/write-ticket` when the target is a vague inline description with no clear scope. Clear-scope inline descriptions currently bypass `/write-ticket` and proceed directly to implementation. Exception: exploratory targets stop and route back to `/discuss`.
>
> With this change, `/proceed` is a valid entry point from any conversation state, including immediately after `/discuss` or mid-discussion with no ticket path argument.

## Reconstruction

### `/forge-spec` {#260421-forge-spec}

From-scratch spec reconstruction for a project. The skill:
1. Archives existing `ai-docs/spec/` to `ai-docs/ref/old-spec/YYMMDD/` — requires explicit user confirmation.
2. Surveys the codebase with four parallel Sonnet subagents (structure, tickets, old spec, commits).
3. Presents domain candidates to the user; confirmed list locks into `TaskCreate` tasks for resumability.
4. Per domain: surveys again with four parallel subagents; presents a behavior brief; asks the user to classify each behavior (caller-visible? implemented or planned?); authors spec entries only after explicit confirmation.
5. Runs `spec-build-index` after each file write.

> [!note] Constraints
> - No spec entry is written without explicit user classification of caller-visible status and implementation status.
> - Uses `TaskCreate` with `forge-spec-<domain>` prefix for cross-compact resume detection — renaming tasks breaks resume.


### `/forge-mental-model` {#260423-forge-mental-model-skill}

From-scratch mental-model construction for a project. Mirrors the `/forge-spec` pattern:
1. Surveys the codebase with parallel subagents to identify domain candidates.
2. Presents the domain list to the user; confirmed list locks into `TaskCreate` tasks for resumability.
3. Per domain: surveys internals, drafts operational knowledge, authors the domain file.
4. If `ai-docs/spec/` exists, embeds relevant spec stems inline per mental-model conventions.
5. Updates `ai-docs/mental-model.md` index after all domains complete.

`disable-model-invocation: true` — hidden from the model's skill palette; user-triggered only.

A soft `judge: spec-gate` fires first: if no spec is found, warns that stem cross-references will be absent but proceeds without blocking.

> [!note] Constraints
> - No domain file is written without completing the survey step for that domain.
> - Uses `TaskCreate` with `forge-mental-model-<domain>` prefix for cross-compact resume detection — renaming tasks breaks resume.
> - Replaces `/write-mental-model` for from-scratch use cases.

## Utility Skills

### 🚧 `/add-rule` {#260424-add-rule-skill}

Rule authoring skill. Accepts a natural-language rule description, classifies it as cross-cutting or domain-scoped, and writes it to the appropriate document:

- **Cross-cutting** (applies across the entire codebase, regardless of which area is being worked in) → appended to `## Architecture Rules` in `CLAUDE.md`.
- **Domain-scoped** (applies when working in a specific domain area) → written to `## Domain Rules` in the relevant `ai-docs/mental-model/<domain>.md`.

**Routing behavior:** when classification is unambiguous, the skill states the proposed target and writes without waiting for confirmation. When ambiguous (cross-cutting vs domain-scoped, or uncertain which domain), it presents candidates and asks the user to choose. When no matching domain doc exists, it proposes creating one with appropriate frontmatter.

**Refusals:**
- Does not modify existing rule content — adds new rules only.
- Does not write the same rule to both `CLAUDE.md` and a mental-model doc simultaneously.

### `/ship` {#260421-ship}

Release workflow: reads a project config from `ai-docs/ship/<proj>.md` (`.local.md` overlay for private targets); bumps the version, creates a git tag, builds, and publishes. Blocked on user confirmation before the publish step.

On first invocation with no config present, creates a template for the user to fill in.

> [!note] Constraints
> - `.local.md` config takes precedence over `.md` and is gitignored — for private deploy targets only.

### `/team-lead` {#260421-team-lead}

Team orchestration mode: creates and manages a named team via `TeamCreate`/`TeamDelete`; spawns subagents into the team; coordinates inter-agent communication; shuts the team down cleanly at session end.

Loaded automatically by `/implement` and `/parallel-implement` before any team operation. Can also be invoked directly for manual team management.

> [!note] Constraints
> - Team state is session-scoped — no persistence across sessions.

### `/bootstrap` {#260421-bootstrap}

Scaffolds a new project (`fresh` mode) or upgrades an existing one (`upgrade` / `adopt` modes) to the canonical `CLAUDE.md` template structure. Performs a surgical merge: never overwrites project-specific sections; marks unresolvable conflicts inline with `<!-- CONFLICT: ... -->`.

Creates the `ai-docs/` directory structure (`tickets/`, `spec/`, `mental-model/`, `plans/`, `ref/`) and a `.gitignore` entry for `.local.md` files.

Legacy project detection: when `ai-docs/spec/` or `ai-docs/mental-model/` is absent after bootstrapping, the skill suggests running `/forge-spec` followed by `/forge-mental-model` to establish the documentation baseline. {#260423-bootstrap-legacy-forge-routing}

> [!note] Planned 🚧
> Migration item added to `CLAUDE.template.md` checklist: prompts existing downstream project owners to re-evaluate entries in `## Architecture Rules` (CLAUDE.md) and architectural conventions in `_index.md`, reclassifying domain-scoped rules into `ai-docs/mental-model/<domain>.md` via `/add-rule`. Applied on the next `/bootstrap upgrade` run. {#260424-bootstrap-architecture-rules-migration}
