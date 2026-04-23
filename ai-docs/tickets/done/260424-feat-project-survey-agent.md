---
title: Project Survey Agent — Pre-invocation Context Survey
spec:
  - 260424-project-survey-auto-invoke
related-mental-model:
  - workflow-routing
  - executor-wrapup
---

# Project Survey Agent — Pre-invocation Context Survey

## Background

Execution skills (`edit`, `implement`, `parallel-implement`) and `discuss` have no automated way to front-load relevant documentation before starting work. The implementer must manually identify and read relevant spec sections, mental-model docs, and active tickets — error-prone in cold-context sessions.

`project-survey` is a Haiku agent that fires at the start of each run. Given the implementation brief, it exhaustively enumerates and reads all docs in three ai-docs tiers (spec, mental-model, active tickets), then returns a `[Must|Maybe]`-tiered reference list the calling skill uses to guide its own reads or inject into a delegate's prompt.

## Decisions

- **Agent-only, no skill wrapper**: not user-invocable directly; spawned by other skills via `Agent(subagent_type="project-survey")`. No standalone user use case.
- **Haiku as default model**: race tests against Sonnet on the narrowed scope showed Haiku finds the same primary docs when given exhaustive enumeration instructions. Cost-to-quality ratio favors Haiku.
- **Scope: spec / mental-model / active tickets only — no source code**: source code gap is covered by `write-plan` survey-mode and Explore agents. Restricting scope keeps doc count small (~15–20 docs), enabling exhaustive enumeration rather than inferential search, which eliminates Haiku's semantic-reasoning disadvantage.
- **Exhaustive enumeration over inferential search**: list all files in scope first, read all, then rank. At this scale, exhaustive is faster and more reliable than grep-based inference.
- **2-tier output `[Must|Maybe]`, not 3**: at this document scale a 3rd tier absorbs everything that isn't `[Must]`, giving consumers no actionable signal. Binary maps cleanly to "load unconditionally" vs "load on demand."

## Constraints

- `done/` and `dropped/` ticket directories are excluded from search scope.
- Source code file references must not be produced — `/write-plan` survey-mode covers that gap.
- The agent must list directory contents explicitly before reading, to prevent inferential path selection from missing docs.

## Phases

### Phase 1: Agent definition

Write `claude/agents/project-survey.md` following `ai-docs/ref/skill-authoring.md`.

The agent definition must specify:

- **Input**: implementation brief or query (natural language)
- **Model**: Haiku
- **Process**:
  1. List all files in `ai-docs/spec/`, `ai-docs/mental-model/`, `ai-docs/tickets/idea/`, `ai-docs/tickets/todo/`, `ai-docs/tickets/wip/`.
  2. Read every file found.
  3. Assign `[Must]` if the file directly covers behavior, patterns, or constraints needed before starting. Assign `[Maybe]` if tangentially related. Exclude if not relevant.
- **Output format**:

  ```
  ## Spec
  - [Must|Maybe] path  # one-line annotation

  ## Mental Model
  - [Must|Maybe] path  # one-line annotation

  ## Tickets
  - [Must|Maybe] path  # one-line annotation
  ```

  Omit empty sections. One annotation per item explaining relevance.

**Success criteria**: agent definition passes `ai-docs/ref/skill-authoring.md` invariant checklist; Haiku produces correct tiered output for a test brief.

### Result (bc4fa7a) - 2026-04-24

Agent written at `claude/agents/project-survey.md`. Invariant checklist passed. Model: haiku. Constraints: explicit Bash listing before reads (prevents inferential path selection), source code excluded, done/dropped excluded.

### Phase 2: Skill integrations

Add a `project-survey` dispatch step to four skills:

- `claude/skills/edit/SKILL.md` — first step of On: invoke, before any file reads.
- `claude/skills/implement/SKILL.md` — before implementer subagent spawn; inject result into prompt.
- `claude/skills/parallel-implement/SKILL.md` — before worker spawns; inject result into each worker prompt.
- `claude/skills/discuss/SKILL.md` — On: invoke step 1; front-loads relevant docs before discussion loop.

Integration pattern for each skill: spawn `project-survey` agent with the implementation brief → receive reference list → use as guidance for subsequent reads (`edit`, `discuss`) or inject into delegate prompt (`implement`, `parallel-implement`).

Depends on Phase 1 (agent must exist before skills reference it).

**Success criteria**: all four skills contain the project-survey dispatch step; `implement` and `parallel-implement` inject the reference list into their delegate prompts.

### Result (b630195) - 2026-04-24

Step 0 added to On: invoke Prepare in all four skills. implement and parallel-implement carry the captured list forward into delegate prompts. discuss uses it as initial reading queue. All changes are single-line insertions following the established step-numbering pattern.
