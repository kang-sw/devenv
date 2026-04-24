---
title: polish-plugin-docs skill
spec:
  - 260424-polish-plugin-docs
related-mental-model:
  - workflow-routing
  - executor-wrapup
---

# polish-plugin-docs skill

## Background

The ws plugin's `claude/skills/`, `claude/agents/`, and `claude/infra/` documents accumulate drift over time — redundant phrasing, interleaved rationale, and authoring-rule violations that erode readability under attention pressure. A local `.claude/skills/polish-plugin-docs/` skill runs an automated review + simplification cycle to address this without manual per-file effort.

## Decisions

- **Location:** `.claude/skills/` (local Claude Code skill, not ws plugin). Invoked as `/polish-plugin-docs`, not `/ws:polish-plugin-docs`.
- **Model split:** opus for review (reasoning quality), sonnet for writing (volume + simplification).
- **Per-file writer calls:** one `ws-call-agent` call per file to prevent context accumulation across files.
- **Dual-reviewer in loop:** fresh opus reviewer catches new issues introduced by the writer; resumed opus reviewer (session from Phase 1) reads only `git diff` and confirms prior findings were resolved. These roles are complementary, not redundant.
- **`--no-ff` merge:** user merges the branch manually after skill exits; `--no-ff` preserves the review history as a non-linear branch.
- **Rejected:** single-reviewer loop — a fresh reviewer alone cannot verify resolution of its own prior findings without re-reading everything.

## Constraints

- Scope is fixed: `claude/skills/`, `claude/agents/`, `claude/infra/` (`.md` files only).
- Excludes `claude/CLAUDE.home.md` and `claude/bin/`.
- Writer must receive `ai-docs/ref/skill-authoring.md` verbatim in its system prompt — without it, sonnet defaults to general prose cleanup rather than authoring-rule-guided simplification.
- `ws-declare-agent` must be called at skill start for all agent slots to clear stale sessions from prior runs.

## Phases

### Phase 1: Skill scaffold + initial reviewer

Create `.claude/skills/polish-plugin-docs/SKILL.md`. Implement:
- Invariants section covering scope, excluded files, and agent slot declaration.
- `On: invoke` handler:
  1. `ws-declare-agent` for all slots: `reviewer-init`, `reviewer-fresh`, `reviewer-resume`.
  2. `git checkout -b docs/polish-plugin-docs`.
  3. Collect target files: `find claude/skills claude/agents claude/infra -name "*.md"`, excluding `claude/CLAUDE.home.md`.
  4. `ws-call-agent opus --agent reviewer-init --system-prompt <document-reviewer path>` with all target file contents in the prompt. Expect Phase 1 findings report (Critical / Important / Minor).

Success criterion: skill file passes `skill-authoring.md` invariant checklist; initial reviewer produces a findings report.

### Phase 2: Writer loop (per-file + commit)

Implement the writer step inside the review loop:
1. For each target file: `ws-call-agent sonnet --system-prompt <writer-prompt path>` with the file contents and current findings. Writer simplifies and returns the updated file content. Lead writes the file.
2. After all files processed: `git add` changed files + `git commit`.

Writer system prompt (`claude/infra/polish-writer.md`) must include:
- `skill-authoring.md` content verbatim (the authoring criteria).
- Instruction: simplify expression, do not change behavioral meaning.
- Instruction: resolve findings from the provided findings report.

Success criterion: writer produces per-file edits; commit is created after each round.

### Phase 3: Dual-reviewer loop + exit

Implement the loop condition and dual-reviewer step:
1. `ws-call-agent opus --agent reviewer-fresh` (new session each round) with all target file contents. Expect Phase 1 or Phase 2 output.
2. `ws-call-agent opus --agent reviewer-resume` (resumes `reviewer-init` session) with `git diff HEAD~1` output only. Expect Phase 1 or Phase 2 output.
3. If either reviewer emits Phase 1 (findings): merge findings, pass to next writer round (back to Phase 2 handler). Increment round counter.
4. If both emit Phase 2 (Final report) or round counter reaches 3: exit loop.
5. Skill outputs a summary: rounds completed, final reviewer reports, branch name, merge instruction (`git merge --no-ff docs/polish-plugin-docs`).

Success criterion: loop exits cleanly on dual Final report or round limit; skill emits merge instruction.
