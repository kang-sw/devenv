---
title: Workflow Skills
summary: User-invocable /commands that orchestrate AI-assisted development workflows in downstream projects.
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
             /write-skeleton? → /implement
                                    ↓
                         (write-code | edit — routed internally)
```

Each skill recommends the next step at completion. `/proceed` is the auto-router invoked after `/write-ticket`; it selects and chains skeleton and implementation stages based on existing artifacts and session context. `/implement` applies its own `judge: execution-mode` to route between direct-edit (`ws:edit`) and delegated (`ws:write-code`) internally.

> [!note] Constraints
> - Skills do not auto-chain — user invocation or `/proceed` is always required.
> - `/write-spec` is optional: the `judge: spec-impact` gate inside it exits early when no public behavior is affected.
> - `/write-skeleton` is optional: required only when public contracts need crystallization before implementation.
> - Plan population is handled internally by `write-code` via `judge: plan-depth` — there is no separate `/write-plan` pipeline stage.

## Planning Skills

### `/discuss` {#260421-discuss}

Facilitates exploratory discussion of approach or direction. Reads spec, mental-model, and ticket files on demand; dispatches Explore subagents for codebase questions. Produces no source edits.

Pre-injects a project map via `ws-proj-tree` at skill start — a rendered view of `ai-docs/` structure, spec stats, and active tickets. {#260425-discuss-proj-tree}

At the end of a discussion turn, always suggests `/write-spec` as the next step. Also offers `/write-ticket` to capture decisions as a ticket.

When a mental-model domain file is read, the skill checks its last commit date via `git log -1 --format="%ai" -- ai-docs/mental-model/<domain>.md`. If the date is more than 90 days ago, the skill surfaces a staleness warning for that domain. No frontmatter field is needed; git history is the source. {#260423-discuss-mental-model-staleness-warning}

When the user requests a ticket status transition (promoting `idea/` → `todo/`, or dropping a ticket), the skill performs the `git mv`, invokes `/write-spec` to add or remove `🚧` entries for caller-visible behaviors, and commits both changes together.


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

Status directories: `idea/` → `todo/` → `done/` (or `dropped/`). Ticket stem format: `YYMMDD-type-slug`.

A `judge: spec-gate` blocks creation of `todo/`-or-higher tickets when no spec entry covers the topic, and suggests `/write-spec` first. `idea/` creation is ungated.

#### Ticket Queue {#260428-ticket-queue-convention}

`ai-docs/_index.md` maintains a `## Ticket Queue` section listing `todo/` tickets in intended implementation order. Format: one line per ticket — `` `stem` — purpose and dependency notes ``. Three touch points maintain the queue:

- `/write-ticket` adds an entry when a ticket is created or promoted to `todo/`.
- `/discuss` adds an entry when an `idea/` ticket is promoted to `todo/`.
- `executor-wrapup` removes the entry when a ticket moves to `done/`.


> [!note] Constraints
> - Does not advance a ticket's status directory — status changes happen via `git mv` during implementation.

### `/write-skeleton` {#260421-write-skeleton}

Crystallizes public contracts as interface stubs and integration tests before implementation begins. The lead identifies contract directives (type signatures, API shapes, invariants), delegates writing to a subagent, reviews, and commits. Updates the ticket `skeletons:` frontmatter with the commit hash.

Suggests `/implement` as the next step — does not auto-invoke. `/implement` internally routes to direct-edit or delegated based on `judge: execution-mode`.

> [!note] Constraints
> - Stub files must compile; integration tests are written but not required to pass (implementation is absent).
> - The lead does not write skeleton code directly — subagent delegation only.

## Implementation Skills

### `/write-code` {#260427-write-code-skill}

Core delegated implementation primitive. Operates on the current branch (branch creation is the caller's responsibility). Steps:

1. Reads target; spawns `project-survey`; writes a brief (`ai-docs/plans/YYYY-MM/DD-<stem>.brief.md`) — the implementer's sole context source.
2. Applies `judge: plan-depth` (soft): **as-is** (no plan file) / **survey** (Sonnet populates a reference map) / **research** (Opus produces a step-by-step plan).
3. Spawns an implementer subagent and up to three review-partition subagents in parallel. {#260424-implement-file-based-review}
4. Runs a relay loop (max 3 cycles): implementer responds to each finding with `[fixed]`, `[won't fix: <reason>]`, or `[deferred: <reason>]`; reviewers respond `[accepted]` or `[maintained]`. At cycle 2, the lead adjudicates maintained disputes; at cycle 3, unresolved disputes escalate to the caller.
5. Deletes review path files and outputs commit range + test status to the caller.

Review partitions:
- **Correctness** — logic, error paths, contracts, security.
- **Fit** — conventions, naming, reuse, patterns.
- **Test** — test file quality, coverage of new code paths, assertion validity.

Reviewers write full findings to `ws-review-path`-allocated files; stdout returns only a `[clean|non-clean]: <brief>` summary line. Won't-fix is allowed for style conflicts with established patterns or out-of-scope suggestions; not allowed for correctness, security, or contract violations.

### `/edit` {#260422-edit-skill}

Direct-edit primitive. The lead reads source, edits, verifies, and commits — no subagent delegation for the edit itself. One named-agent reviewer covers correctness and fit (both partition docs concatenated into a single system prompt). Relay loop capped at 2 cycles; the lead applies fixes directly without won't-fix negotiation. After cleanup, invokes `ws:update-spec` on the edit's commit range. Outputs commit range, test status, and spec result to the caller. Callers own mental-model updates.

> [!note] Constraints
> - Escalates to `/implement` when scope grows to multi-file with new public API or cross-module without established pattern.
> - Suggests `/write-skeleton` when none exist and scope warrants them, but does not auto-invoke it.

### `/implement` {#260422-implement-skill}

Implementation harness. Routes to `ws:write-code` or `ws:edit` based on `judge: execution-mode`, then runs the shared doc pipeline, report/approval gate, and merge.

- **Direct-edit** — single file, purely internal change, no new public symbols: routes to `ws:edit`.
- **Delegated** — any cross-file touch, new public contract, or explicit delegation requested: creates an `implement/<scope>` branch and routes to `ws:write-code`.

After the primitive returns, invokes `ws:update-spec` (lead-driven) then dispatches `ws:mental-model-updater` (sequential, spec first so 🚧 strips are visible to the mental-model updater). User approves before merge. Merge uses `ws-merge-branch` (squash for 1 commit, `--no-ff` for 2+). {#260423-doc-pipeline-spec-updater}

> [!note] Constraints
> - User approval is unconditional — no code reaches the target branch without it.
> - If `write-code` escalates at cycle 3, unresolved reviewer disputes are listed in the approval report for user decision.

### `/update-spec` {#260428-update-spec-skill}

Lead-driven spec audit for a commit range. The lead (not a subagent) loads `spec-conventions.md` and `write-spec/SKILL.md`, scans each commit for caller-visible behavior changes, adds missing spec entries, strips `🚧` markers where the corresponding stem appears in the commit log, and removes entries flagged with `removed: <stem>` in commit bodies. Runs `ws-spec-build-index` and commits if any file was modified.

Called by `/edit` (after cleanup, on the edit's own commit range), `/implement` (as the first doc pre-pass step, before `mental-model-updater`), and `/sprint` (at wrap-up, with the `$PARENT..HEAD` range). Can also be invoked standalone.

> [!note] Constraints
> - No subagent delegation — the lead applies `judge: spec-impact` inline.
> - Only adds entries for confirmed-implemented features; never adds `🚧` entries autonomously.
> - On borderline cases, errs toward adding an entry.

### Pre-invocation Context Survey — `project-survey` {#260424-project-survey-auto-invoke}

`project-survey` is auto-invoked at the start of each `/write-code` run (before the brief is written) and embedded into the brief's `## References` section. Survey output serves as the focused doc list for the plan-population agent and the implementer. In `/discuss`, the model triggers it on-demand via `judge: needs-survey` when the topic references components not yet read this session or the discussion shifts to a new domain. {#260424-discuss-on-demand-survey}

The agent returns a `[Must|Maybe]`-tiered reference list. Output per tier:
- **Spec entries** — stem, entry title, and one-line summary from the spec body.
- **Mental-model entries** — path and a one-line relevance note.
- **Ticket entries** — stem, ticket title, and unresolved phase titles.

Tiers:
- **`[Must]`** — directly covers behavior, patterns, or constraints required before starting.
- **`[Maybe]`** — tangentially related; useful when uncertain.

Search scope: `ai-docs/spec/`, `ai-docs/mental-model/`, and active ticket directories (`idea/`, `todo/`). Source code references are out of scope.

> [!note] Constraints
> - `done/` and `dropped/` ticket directories are excluded from the search scope.
> - Source code file references are not produced by this agent.
> - In `/discuss`, the survey does not fire for session-continuity queries — those draw from session state or git log.

### `/proceed` {#260421-proceed}

Auto-router: assesses an implementation target and chains to the correct pipeline without executing implementation steps itself. All implementation paths terminate in `/implement` — routing to `/edit` directly is no longer a proceed concern. Judges:

- **Existing artifacts** — skips `/write-skeleton` if already present.
- **Needs-skeleton** — routes to `/write-skeleton` before `/implement` when public contracts are absent.

Announces the chosen path before invoking the first skill. `/implement`'s internal `judge: execution-mode` handles the direct-edit vs. delegated split.

> [!note] Constraints
> - Stops and routes to `/discuss` when the target is exploratory rather than actionable.

> [!note] Extended behavior — full-pipeline routing {#260422-proceed-full-pipeline-routing}
> Two prefix judges fire before the pipeline judges:
> - **`judge: needs-spec`** — always invokes `/write-spec`; its own `judge: spec-impact` gate exits without writing if no spec work is needed.
> - **`judge: needs-ticket`** — auto-invokes `/write-ticket` when the target is an inline description. Exception: exploratory targets stop and route back to `/discuss`.
>
> `/proceed` is a valid entry point from any conversation state, including immediately after `/discuss` or mid-discussion with no ticket path argument.

### `/sprint` — Session Container {#260425-sprint}

A multi-task session container that replaces the manual `/discuss` → `/proceed` chain for feature-branch work. Sprint holds the full lifecycle — routing, implementation, and wrap-up — in a single persistent session. All doc-pipeline steps are deferred to wrap-up; each task commits only source changes.

> [!note] Constraints
> - Sprint operates only on `sprint/`-prefixed branches.
> - The doc pipeline (spec audit, `ws:mental-model-updater`) is suppressed during task execution and runs once at wrap-up.

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
2. **Spec-update pass (lead-driven)**: the lead loads `spec-conventions.md` and `write-spec/SKILL.md`, then scans each commit for caller-visible behavior changes (`judge: spec-impact`). For each impacted area, the lead reads the relevant spec file and adds new entries if missing (running `ws-generate-spec-stem` for each new anchor). The lead then strips `🚧` from headings whose stems appear in the commit log and removes any corresponding `> [!note] Planned 🚧` callout blocks. Commits with `removed: <stem>` in their `## Spec` section trigger manual removal of the corresponding entry. `ws-spec-build-index` runs after any modification; all spec changes commit together.
3. Dispatches `ws:mental-model-updater` with a note that docs may be stale — explore thoroughly. (Runs after spec-update so the updater sees any stripped 🚧 entries.)
4. Runs `executor-wrapup` (existing tickets only — sets `## Result` and advances state; no new ticket creation).
5. Emits a post-hoc report to the user (entries added, stripped, or removed).
6. Suggests branch merge or deletion.

> [!note] Constraints
> - Wrap-up runs once per sprint, not per task.
> - Completing wrap-up and merging or deleting the sprint branch closes the sprint.

#### Sprint Implementation {#260425-sprint-implementation-delegation}

Sprint routes implementation tasks to the shared primitives:
- Single-file isolated change → `ws:edit` (direct-edit, one named-agent reviewer covering correctness+fit).
- Multi-file or new-pattern → `ws:write-code` (delegated; three review partitions; won't-fix + 3-cycle cap).

Both primitives self-clean their review path files. No doc pipeline runs during a sprint task — deferred to wrap-up.

### `/workflow` {#260428-workflow-skill}

Loads the WS orchestration primitives reference into session context. Invoked via Skill tool at the start of `/discuss` and `/sprint` sessions; users may also invoke directly with `/workflow`.

Content is session-resident: unlike `ws-print-infra` output (which is cleared by compaction), skill content loaded via the Skill tool survives context compaction. Re-invoking `/workflow` after a compaction restores the reference without re-running any shell commands.

The skill has no side effects — reading it is the act of invocation.

> [!note] Constraints
> - Does not replace `ws-print-infra` for agent Bash tool contexts; those agents use `ws-print-infra` or `workflow-for-agent.md` auto-injection.
> - Skills that invoke `/workflow` via `ws:workflow` Skill tool do so in their `On: invoke` step; they do not call `ws-print-infra ws-orchestration.md`.

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

Bash-callable infra scripts at `claude-plugin/infra/` that replace `TeamCreate`/`SendMessage`/`TeamDelete` for subagent coordination. Skill leads invoke these directly via the `Bash` tool.

### `ws-new-named-agent` {#260425-ws-new-named-agent}

`ws-new-named-agent <agent-name> [-p <prompt>]... [--model <opus|sonnet|haiku>] [--no-doc-system]`

Creates a named agent registry entry at `.git/ws@<repo-dir-name>/agents/<agent-name>.json`.

- `-p <name-or-path>` — resolves against `infra/prompts/` first, then `infra/`, then cwd. Multiple `-p` flags accepted; bodies concatenated with `---` separators. The first document whose frontmatter declares `model:` sets the agent's default model tier.
- `--model` — explicit model; overrides frontmatter model from `-p`.
- `--no-doc-system` — suppress auto-injection of `workflow-for-agent.md`.
- Legacy flags `--agent <type>` and `--system-prompt <path>` still accepted.

Registry JSON fields:
- `uuid` — fresh random UUID v4. Not deterministic; enables compression-triggered refresh without branch or name changes.
- `model`, `agent_type`, `system_prompt` — agent configuration, restored on compression handoff.
- `token_count` — cumulative input tokens tracked by `ws-call-named-agent`; reset to `0` on creation.
- `compressed_at` — `false` on creation; `true` immediately after a compression handoff.

Overwrites the file if already present. Callers must not clobber a live session.

#### `workflow-for-agent.md` auto-injection {#260428-named-agent-doc-system-injection}

When `ws-named-agent new` stores the system prompt, it prepends `claude-plugin/infra/workflow-for-agent.md` to the caller-supplied content (falling back to `doc-system.md` for pre-refactor installs where the file may still exist). Stored form: `[workflow-for-agent content]\n\n---\n\n[caller system prompt]`. Every agent receives basic doc-layer orientation and the safe primitive subset without callers having to request it.

Pass `--no-doc-system` to suppress injection. Use this for narrow-role agents (e.g., sprint-survey, project-survey, compression helpers).

If neither file exists (non-ws projects), the flag is silently ignored.

### `ws-call-named-agent` {#260424-ws-call-named-agent}

`ws-call-named-agent <agent-name> "<prompt>"`

Calls a registered agent by name and delivers its plain-text response to stdout. Agent configuration (model, agent type, system prompt) is read from the registry entry created by `ws-new-named-agent`. Auto-creates a new session or resumes the existing one based on whether a session file exists in `~/.claude/projects/`.

Use `$(ws-infra-path <docname>)` when a path string is needed outside `ws-new-named-agent` (e.g., piping into `cat`). For `ws-new-named-agent`, pass the bare stem via `-p <stem>` instead — resolution is plugin-relative by design. {#260424-infra-path-portability}

Auto-compression is transparent: when cumulative token usage for the session exceeds 120K, `ws-call-named-agent` extracts the original intent (one-shot Haiku call), injects `agent-compression.md` into the current session to produce a structured handoff document, re-registers the agent with a fresh UUID, and replays the original prompt to the new session. The caller receives the fresh agent's response without any visible interruption. {#260425-ws-call-named-agent-auto-compression}

> [!note] Constraints
> - Output is the agent's plain-text response. No JSON is exposed to callers.
> - Exit code is 1 when the underlying call reports an error.
> - Compression is skipped for one call immediately after a handoff to prevent cascade triggering.
> - Auto-compression applies to the `claude` backend only. The `codex` backend has compression disabled; token count is tracked for observability but no handoff occurs.

### `ws-named-agent erase` {#260429-ws-named-agent-erase}

`ws-named-agent erase <name>` — removes the named agent's registry entry and its associated Claude session file.

Deletes `<registry-dir>/<name>.{json,outbox.txt,output.txt}` and, when a UUID is stored in the registry entry, globs `~/.claude/projects/*/<uuid>.jsonl` to remove the corresponding session file.

> [!note] Constraints
> - Exits non-zero if the agent is not found.
> - Session file deletion is best-effort: if the session file has already been removed, erase continues without error.

### `ws-oneshot-agent` {#260429-ws-oneshot-agent-skill-doc}

Wraps the new → call → erase triad into a single invocation. Use when a task requires full tool access but no session persistence.

```bash
ws-oneshot-agent -p <prompt-stem> [-p <stem2>] [--model <tier>] [--no-doc-system] - <<'PROMPT'
...
PROMPT
```

The agent name is an ephemeral `_oneshot_<8hex>` identifier. Registry and session files are cleaned up via an EXIT trap regardless of call outcome. Output flows to stdout identically to `ws-call-named-agent`.

Distinct from `ws-subquery`: the spawned agent has full tool access; `ws-subquery` is non-interactive with no tool use.

> [!note] Constraints
> - At least one `-p` stem is required.
> - `workflow-for-agent.md` is injected by default; pass `--no-doc-system` to suppress.

### `agent-compression.md` {#260425-agent-compression-doc}

Infra document at `claude-plugin/infra/agent-compression.md`. Injected by `ws-call-named-agent` as the next user turn into an agent approaching the 120K token threshold.

Instructs the agent to produce a structured handoff document without reading any new files:
- Original purpose and action plan.
- Work summary (1–2 lines per completed item).
- `[Must]` / `[Maybe]` skills for the next agent.
- `[Must]` / `[Maybe]` docs for the next agent.
- Execution log of concrete actions relevant to the forwarded intent.

### `ws-infra-path` {#260425-ws-infra-path}

`ws-infra-path <docname>` → prints the absolute path to a named infra doc, resolved from the plugin's own `infra/` directory regardless of CWD.

Use when a path string is needed outside `ws-new-named-agent` — for example, to pipe into another tool:

```bash
cat "$(ws-infra-path impl-playbook.md)"
```

For `ws-new-named-agent`, pass the bare stem via `-p <stem>` instead:

```bash
ws-new-named-agent implementer -p implementer
ws-call-named-agent implementer "<prompt>"
```

> [!note] Constraints
> - Bare `claude-plugin/infra/<name>` paths break in downstream projects where `claude-plugin/infra/` does not exist relative to CWD. Always use `-p <stem>` with `ws-new-named-agent` or `$(ws-infra-path <docname>)` for other path contexts.
> - Exits non-zero when the named doc is not found.

### `ws-proj-tree` {#260425-ws-proj-tree}

`ws-proj-tree` — prints a structured project map to stdout: the `ai-docs/` directory tree (excluding `tickets/` and `spec/`), spec file stats (feature count, `🚧` count, ticket refs), and active tickets grouped by status (`todo` → `idea`).

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

