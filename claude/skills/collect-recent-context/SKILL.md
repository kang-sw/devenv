---
name: collect-recent-context
description: >
  Collect recent work context (branch/working-tree state, recent git
  history, active ticket scan, commit-log forward notes) as a structured
  English report. Runs forked as a clerk subagent with session-start git
  history pre-injected. Internal — invoked by `/enter-session`, not
  directly by users.
argument-hint: "[ticket-stem or scope hint — optional]"
context: fork
agent: clerk
user-invocable: false
---

# Collect recent context

Scope hint: $ARGUMENTS

## Pre-injected state

The harness ran these before handing control to clerk. Treat as authoritative — do not re-run.

### Current branch
!`git branch --show-current`

### Working tree
!`git status --short`

### Project arc
!`git log --oneline --graph -50`

### Recent history
!`git log -10`

## Your task

You are clerk in a forked read-only context. Your sole job is to emit the structured English state report below. You do **not** route, recommend next actions, or apply any workflow judgment — the caller (`/enter-session`) handles interpretation. You do not edit anything. You do not read plan files, skeleton files, or source code. Skip clerk's default `## Clerk report` template — the report format below governs.

### 1. Scan active tickets

Scan `ai-docs/tickets/wip/` and `ai-docs/tickets/todo/`. If the scope hint names a ticket stem, include it regardless of its status directory.

For each active ticket:

- Read the ticket file. Extract: phase list, `skeletons:` frontmatter entries (phase names only, not file content), `plans:` frontmatter entries (phase names only, not file content).
- Run `git log --grep=<ticket-stem>` (no `-p`, commit messages only). In matching commits, locate `## Ticket Updates` sections. Extract: which phases are marked complete, and the most recent forward notes verbatim.

### 2. Classify owner state

From the pre-injected state above:

- Extract the current branch name. Note whether it matches `implement/<scope>`.
- Extract the last-commit hash and subject from the top of the `git log -10` output.
- If uncommitted changes exist (per `git status --short`), classify them: WIP on a known phase, orphan changes, or merge conflict.

### 3. Emit the report and terminate

Your final message must be **exactly** the report below — no preamble, no explanation, no `## Clerk report`, no recommendations, no routing suggestions, no text before or after. Output in English regardless of any other context.

```
## Recent context

### Owner state
- Branch: <name>
- Branch pattern: <implement/<scope> | other>
- Last commit: <hash> <subject>
- Working tree: <clean | N modified | WIP on phase X | orphan changes | merge conflict>

### Active tickets

#### <ticket-stem> (<wip|todo|idea>)
- Phases: <comma-separated list>
- Skeletons present: <phase names, or "none">
- Plans present: <phase names, or "none">
- Completed phases (per git log): <list, or "none">
- Forward notes (most recent, verbatim):
  - <bullet>
  - <bullet>

(repeat per ticket; if no active tickets, replace this whole subsection with a single line: "No active tickets.")

### Orphan signals
- <anything noteworthy that doesn't fit above, or "none">
```

Omit any bullet whose value would be empty or a placeholder — no stub text.

## Doctrine

collect-recent-context optimizes for **owner-context economy** — git history, ticket reads, and commit-log extraction happen inside a forked clerk subagent, and only the compact English state report crosses the fork boundary back to the caller. No routing, no judgment, no briefing emission — those responsibilities belong to `/enter-session`. When a rule is ambiguous, apply whichever interpretation preserves the collect/interpret split.
