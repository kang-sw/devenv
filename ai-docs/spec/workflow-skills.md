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
  - Implementation Skills
    - `/edit`
    - `/implement`
    - Pre-invocation Context Survey — `project-survey`
    - `/proceed`
    - `/sprint` — Session Container
      - Sprint Continue Detection
      - Sprint Session Loop
      - Sprint-Aware Project Survey
      - Sprint Wrap-up
      - Sprint Implementation Delegation
  - Reconstruction
    - `/forge-spec`
    - `/forge-mental-model`
  - Agent Orchestration Primitives
    - `ws-new-agent`
    - `ws-call-agent`
    - `agent-compression.md`
    - `ws-infra-path`
    - `ws-proj-tree`
    - `ws-review-path`
  - Utility Skills
    - `/add-rule`
    - `/ship`
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

Pre-injects a project map via `ws-proj-tree` at skill start — a rendered view of `ai-docs/` structure, spec stats, and active tickets. {#260425-discuss-proj-tree}

At the end of a discussion turn, always suggests `/write-spec` as the next step. Also offers `/write-ticket` to capture decisions as a ticket.

When a mental-model domain file is read, the skill checks its last commit date via `git log -1 --format="%ai" -- ai-docs/mental-model/<domain>.md`. If the date is more than 90 days ago, the skill surfaces a staleness warning for that domain. No frontmatter field is needed; git history is the source. {#260423-discuss-mental-model-staleness-warning}

When the user requests a ticket status transition (promoting `idea/` → `todo/`, or dropping a ticket), the skill performs the `git mv`, invokes `/write-spec` to add or remove `🚧` entries for caller-visible behaviors, and commits both changes together.

> [!note] Planned 🚧
> The promotion handler will run `/write-spec` **before** `git mv`, ensuring a spec entry exists before the ticket reaches `todo/` status. This aligns with write-ticket's spec-gate, which blocks `todo/`-or-higher status when no spec entry exists. {#260425-discuss-promotion-handler-order}

> [!note] Constraints
> - No source files are created or modified during a `/discuss` session.

### `/write-spec` {#260421-write-spec}

Creates or updates a spec file under `ai-docs/spec/` describing caller-visible behavior with anchor-keyed entries. Entry format: `## Feature Name {#YYMMDD-slug}` for implemented features; `## 🚧 Feature Name {#YYMMDD-slug}` for planned ones.

A `judge: spec-impact` gate fires first: if the topic has no caller-visible behavioral change, the skill exits immediately without writing anything.

After writing, runs `ws-spec-build-index` to rebuild the `features:` frontmatter index. Applies a `judge: directory-vs-flat` to decide between a flat file and a directory structure.

> [!note] Constraints
> - Never includes ticket references in `🚧` markers — implementation traceability flows through commit `## Spec` sections referencing the spec-stem.
> - Never edits the `features:` frontmatter block manually — `ws-spec-build-index` owns it.

### `/write-ticket` {#260421-write-ticket}

Creates or edits a ticket file under `ai-docs/tickets/`. Captures scope, phases, constraints, and rejected alternatives. Optionally adds a `spec:` frontmatter field listing spec-stems the ticket implements. Always suggests `/proceed` after authoring, which auto-routes to skeleton, plan, or implementation.

Status directories: `idea/` → `todo/` → `wip/` → `done/` (or `dropped/`). Ticket stem format: `YYMMDD-type-slug`.

A `judge: spec-gate` blocks creation of `todo/`-or-higher tickets when no spec entry covers the topic, and suggests `/write-spec` first. `idea/` creation is ungated.

> [!note] Planned 🚧
> `judge: spec-gate` will also fire on `idea/` → `todo/` promotion moves, not only on direct `todo/` creation. {#260425-write-ticket-spec-gate-promotion}

> [!note] Constraints
> - Does not advance a ticket's status directory — status changes happen via `git mv` during implementation.

### `/write-skeleton` {#260421-write-skeleton}

Crystallizes public contracts as interface stubs and integration tests before implementation begins. The lead identifies contract directives (type signatures, API shapes, invariants), delegates writing to a subagent, reviews, and commits. Updates the ticket `skeletons:` frontmatter with the commit hash.

Suggests `/edit` or `/implement` as the next step based on scope and session warmth — does not auto-invoke.

> [!note] Constraints
> - Stub files must compile; integration tests are written but not required to pass (implementation is absent).
> - The lead does not write skeleton code directly — subagent delegation only.

### `/write-plan` {#260421-write-plan}

Researches the codebase and produces a committed plan file at `ai-docs/plans/YYYY-MM/DD-hhmm.<name>.md`. Three modes:

- **Warm** — lead drafts from session context, reading source as needed.
- **Survey** — a survey-writer subagent explores the codebase; lead drafts from the output.
- **Deep** — a plan-writer subagent handles full exploration and drafting; lead reviews and accepts or rejects.

Updates the ticket `plans:` frontmatter with the plan path. `/proceed` passes the plan path directly to the next implementation skill.


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

Delegated implementation cycle: an implementer subagent writes code; three review-partition subagents review in parallel; the lead merges and runs the doc pipeline. Suited for cold sessions or wide-scope work.

Review partitions: {#260424-implement-file-based-review}
- **Correctness** — logic, error paths, contracts, security.
- **Fit** — conventions, naming, reuse, patterns.
- **Test** — test file quality, coverage of new code paths, assertion validity.

Reviewers write full findings to `ws-review-path`-allocated files; stdout returns only a `[clean|non-clean]: <brief>` summary line. The lead reads summaries only — full findings are not consolidated in lead context. When non-clean, the lead passes file paths directly to the implementer, which reads them independently. The implementer applies judgment: correctness, contract, and security findings are addressed; style findings conflicting with established patterns may be deprioritized. `ws-review-path` files are deleted in the Cleanup step.

Two invocation modes based on the current branch: **main-branch mode** (invoked from `main`/`master`/`trunk`) presents the user approval gate before merging; **feature-branch mode** (invoked from any other branch) skips the gate and auto-merges after a clean review. The feature → main merge remains the user's responsibility in feature-branch mode. Use `--main-branch <name>` to override the default main-branch names. {#260422-implement-feature-branch-mode}

Pre-merge, dispatches `spec-updater` first and waits for it to commit, then dispatches `mental-model-updater` and waits. Ambiguous stems from `spec-updater` are surfaced at the report/approval gate before merge proceeds.

> [!note] Implementation Gap · 2026-04-23
> Pre-merge doc pipeline output (spec-updater and mental-model-updater file changes) is not guaranteed to be committed before the merge step runs.

### Pre-invocation Context Survey — `project-survey` {#260424-project-survey-auto-invoke}

At the start of each `/edit` and `/implement` run, the `project-survey` agent is auto-invoked with the implementation brief. In `/discuss`, the model triggers it on-demand via `judge: needs-survey` when the topic references components not yet read this session or the discussion shifts to a new domain. {#260424-discuss-on-demand-survey}

The agent returns a `[Must|Maybe]`-tiered reference list. Output per tier:
- **Spec entries** — stem, entry title, and one-line summary from the spec body.
- **Mental-model entries** — path and a one-line relevance note.
- **Ticket entries** — stem, ticket title, and unresolved phase titles.

Tiers:
- **`[Must]`** — directly covers behavior, patterns, or constraints required before starting.
- **`[Maybe]`** — tangentially related; useful when uncertain.

Search scope: `ai-docs/spec/`, `ai-docs/mental-model/`, and active ticket directories (`idea/`, `todo/`, `wip/`). Source code references are out of scope.

> [!note] Constraints
> - `done/` and `dropped/` ticket directories are excluded from the search scope.
> - Source code file references are not produced by this agent.
> - In `/discuss`, the survey does not fire for session-continuity queries — those draw from session state or git log.

### `/proceed` {#260421-proceed}

Auto-router: assesses an implementation target and chains to the correct pipeline without executing implementation steps itself. Judges:

- **Context warmth** — warm session routes to `/edit`; cold routes to `/implement`.
- **Existing artifacts** — skips `/write-skeleton` and `/write-plan` if already present.
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

### `/sprint` — Session Container {#260425-sprint}

A multi-task session container that replaces the manual `/discuss` → `/proceed` chain for feature-branch work. Sprint holds the full lifecycle — routing, implementation, and wrap-up — in a single persistent session. All doc-pipeline steps are deferred to wrap-up; each task commits only source changes.

> [!note] Constraints
> - Sprint operates only on `sprint/`-prefixed branches.
> - The doc pipeline (`ws:spec-updater`, `ws:mental-model-updater`) is suppressed during task execution and runs once at wrap-up.

#### Sprint Continue Detection {#260425-sprint-continue}

On invoke, sprint checks whether the current branch starts with `sprint/`. If so, it offers three options: **continue** the session, run **wrap-up**, or **abandon**. Otherwise, it asks for a sprint name and creates a `sprint/<name>` branch.

A `sprint/` branch that exists and has not been merged to main signals that wrap-up has not yet run. This is the sole persistent-state signal — no external file or TaskCreate is used.

#### Sprint Session Loop {#260425-sprint-session-loop}

Accepts requests inside the sprint session and routes via `judge: delegate`:

- Questions or explanations → inline answer
- Codebase exploration → Explore agent dispatch
- Design discussion → inline discussion loop (no `/write-spec` auto-chaining)
- Simple edits → direct edit, no doc pipeline
- Complex implementation → implementation delegation, no doc pipeline

#### Sprint-Aware Project Survey {#260425-sprint-aware-survey}

At session start, and on demand when the domain shifts mid-session, sprint dispatches a Sonnet-overridden survey agent that reads commit messages on the `parent..HEAD` range, cross-references them against spec and mental-model files, and returns a staleness-annotated `[Must|Maybe]` tier list. Entries where recent commits suggest the doc may be out of date are annotated with `[stale?]`.

#### Sprint Wrap-up {#260425-sprint-wrapup}

Triggered by an explicit user done signal. The hardcoded wrap-up procedure:

1. Reads the full branch diff (`git diff parent..HEAD`).
2. Spec-update loop (max 2 iterations): registers `ws:spec-updater` with active-edit instructions (strip 🚧, add new entries, remove dropped entries) and the full commit list. Lead reviews `git diff ai-docs/spec/` after each iteration and accepts or rejects. Sonnet first; escalates to Opus on iteration 2. Force-accepted at iteration 2 to guarantee termination.
3. Dispatches `ws:mental-model-updater` with a note that docs may be stale — explore thoroughly.
4. Runs `executor-wrapup` (existing tickets only — sets `## Result` and advances state; no new ticket creation).
5. Emits a post-hoc report to the user (entries added, stripped, or removed).
6. Suggests branch merge or deletion.

The sequential spec → mental-model order ensures `ws:mental-model-updater` sees any 🚧 strips committed by the spec-update loop.

> [!note] Constraints
> - Wrap-up runs once per sprint, not per task.
> - Completing wrap-up and merging or deleting the sprint branch closes the sprint.

#### Sprint Implementation Delegation {#260425-sprint-implementation-delegation}

Delegated implementation within a sprint uses `ws-call-agent` with two review partitions in parallel: **Correctness** (logic, error paths, contracts, security) and **Fit** (conventions, naming, reuse, patterns). The Test partition is omitted. Reviewers write full findings to `ws-review-path`-allocated files; the lead reads summaries only. When non-clean, the lead relays file paths to the implementer, which reads them directly.

## Reconstruction

### `/forge-spec` {#260421-forge-spec}

From-scratch spec reconstruction for a project. The skill:
1. Archives existing `ai-docs/spec/` to `ai-docs/ref/old-spec/YYMMDD/` — requires explicit user confirmation.
2. Surveys the codebase with four parallel Sonnet subagents (structure, tickets, old spec, commits).
3. Presents domain candidates to the user; confirmed list locks into `TaskCreate` tasks for resumability.
4. Per domain: surveys again with four parallel subagents; presents a behavior brief; asks the user to classify each behavior (caller-visible? implemented or planned?); authors spec entries only after explicit confirmation.
5. Runs `ws-spec-build-index` after each file write.

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

## Agent Orchestration Primitives

Bash-callable infra scripts at `claude/infra/` that replace `TeamCreate`/`SendMessage`/`TeamDelete` for subagent coordination. Skill leads invoke these directly via the `Bash` tool.

### `ws-new-agent` {#260425-ws-new-agent}

`ws-new-agent <agent-name> [--agent <type>] [--system-prompt <path>] [--model <opus|sonnet|haiku>]`

Creates a named agent registry entry at `.git/ws@<repo-dir-name>/agents/<agent-name>.json`.

- `--agent <type>` — agent type forwarded to the `claude` CLI (e.g., `Explore`, `general-purpose`).
- `--system-prompt <path>` — reads the file at `<path>` and stores its content in the registry. Use `$(ws-infra-path <docname>)` for infra docs.
- `--model` — model level (`opus`, `sonnet`, `haiku`). Defaults to `sonnet`.

Registry JSON fields:
- `uuid` — fresh random UUID v4. Not deterministic; enables compression-triggered refresh without branch or name changes.
- `model`, `agent_type`, `system_prompt` — agent configuration, restored on compression handoff.
- `token_count` — cumulative input tokens tracked by `ws-call-agent`; reset to `0` on creation.
- `compressed_at` — `false` on creation; `true` immediately after a compression handoff.

Overwrites the file if already present. Callers must not clobber a live session.

### `ws-call-agent` {#260424-ws-call-agent}

`ws-call-agent <agent-name> "<prompt>"`

Calls a registered agent by name and delivers its plain-text response to stdout. Agent configuration (model, agent type, system prompt) is read from the registry entry created by `ws-new-agent`. Auto-creates a new session or resumes the existing one based on whether a session file exists in `~/.claude/projects/`.

Use `$(ws-infra-path <docname>)` in `ws-new-agent --system-prompt` to ensure portability across downstream projects. {#260424-infra-path-portability}

Auto-compression is transparent: when cumulative token usage for the session exceeds 120K, `ws-call-agent` extracts the original intent (one-shot Haiku call), injects `agent-compression.md` into the current session to produce a structured handoff document, re-registers the agent with a fresh UUID, and replays the original prompt to the new session. The caller receives the fresh agent's response without any visible interruption. {#260425-ws-call-agent-auto-compression}

> [!note] Constraints
> - Output is the agent's plain-text response. No JSON is exposed to callers.
> - Exit code is 1 when the underlying call reports an error.
> - Compression is skipped for one call immediately after a handoff to prevent cascade triggering.

### `agent-compression.md` {#260425-agent-compression-doc}

Infra document at `claude/infra/agent-compression.md`. Injected by `ws-call-agent` as the next user turn into an agent approaching the 120K token threshold.

Instructs the agent to produce a structured handoff document without reading any new files:
- Original purpose and action plan.
- Work summary (1–2 lines per completed item).
- `[Must]` / `[Maybe]` skills for the next agent.
- `[Must]` / `[Maybe]` docs for the next agent.
- Execution log of concrete actions relevant to the forwarded intent.

### `ws-infra-path` {#260425-ws-infra-path}

`ws-infra-path <docname>` → prints the absolute path to a named infra doc, resolved from the plugin's own `infra/` directory regardless of CWD.

Use in `--system-prompt` arguments so callers work in downstream projects:

```bash
ws-new-agent implementer --model sonnet --system-prompt "$(ws-infra-path implementer.md)"
ws-call-agent implementer "<prompt>"
```

> [!note] Constraints
> - Bare `claude/infra/<name>` paths in `--system-prompt` break in downstream projects where `claude/infra/` does not exist relative to CWD.
> - Exits non-zero when the named doc is not found.

### `ws-proj-tree` {#260425-ws-proj-tree}

`ws-proj-tree` — prints a structured project map to stdout: the `ai-docs/` directory tree (excluding `tickets/` and `spec/`), spec file stats (feature count, `🚧` count, ticket refs), and active tickets grouped by status (`wip` → `todo` → `idea`).

Used by `/discuss` as pre-injected project context at skill start.

### `ws-review-path` {#260425-ws-review-path}

`ws-review-path <stem1> [<stem2> ...]` — prints one path per stem under `/tmp/claude-reviews/`, for use as review-findings sinks. Creates the directory if absent. {#260425-ws-review-path-non-deterministic}

Path format: `/tmp/claude-reviews/<pwd-hash>-<run-id>-<stem>.md`. `pwd_hash` is the first 8 chars of `shasum "$PWD"` — scopes paths to the current project. `run_id` is 8 random alphanumeric chars generated once per call — prevents collisions across concurrent invocations.

> [!note] Constraints
> - Caller must capture all output lines from a single invocation and hold them as literals — paths are not reproducible after the call returns.
> - Always pass all stems in one call; separate calls produce different `run_id`s and break co-invocation grouping.

## Utility Skills

### `/add-rule` {#260424-add-rule-skill}

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


### `/bootstrap` {#260421-bootstrap}

Scaffolds a new project (`fresh` mode) or upgrades an existing one (`upgrade` / `adopt` modes) to the canonical `CLAUDE.md` template structure. Performs a surgical merge: never overwrites project-specific sections; marks unresolvable conflicts inline with `<!-- CONFLICT: ... -->`.

Creates the `ai-docs/` directory structure (`tickets/`, `spec/`, `mental-model/`, `plans/`, `ref/`) and a `.gitignore` entry for `.local.md` files.

Legacy project detection: when `ai-docs/spec/` or `ai-docs/mental-model/` is absent after bootstrapping, the skill suggests running `/forge-spec` followed by `/forge-mental-model` to establish the documentation baseline. {#260423-bootstrap-legacy-forge-routing}

