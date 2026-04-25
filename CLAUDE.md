# CLAUDE.md — devenv

## Project Memory

Read in this order at every session start, before any other action:

1. **Preamble** — read `ai-docs/_index.md`. Project-level truth that no
   session should re-derive. Prune aggressively: if derivable from code
   or commit history, delete.
2. **Local** — read `ai-docs/_index.local.md` if it exists. .gitignored.
   Machine-bound context (paths, env vars, build config) and personal
   session notes.
3. **Project arc** — run `git log --oneline --graph -50`. Trajectory and
   topic clusters at a glance.
4. **Recent history** — run `git log -10`. Decision rationale via AI Context
   sections. Fades as history grows.

## Response Discipline

- **Evidence before claims.** Run verification commands and read output before
  stating success. Never use "should pass", "probably works", or "looks correct."
- **No performative agreement.** Never respond with "Great point!", "You're
  absolutely right!", or similar. Restate the technical requirement, verify
  against the codebase, then act (or push back with reasoning).
- **Actions over words.** "Fixed. [what changed]" or just show the diff.
  Skip gratitude expressions and filler.

## Code Standards

Skill and agent documents follow `ai-docs/ref/skill-authoring.md` — read it before authoring or auditing any skill/agent file.

**Architecture rule — skill/agent compliance:** Before committing any write to `claude/skills/`, `claude/agents/`, or `claude/infra/`, run `ai-docs/ref/skill-authoring.md`'s invariant checklist against every Invariants/Constraints line added or modified, verify the Doctrine names a finite resource with a generator clause, and confirm no rationale is interleaved in handler or process steps. Applies to implementers and the lead alike.

## Architecture Rules

- Shell state does not persist between Bash tool calls; any value a later call needs from an earlier call must be read from tool output into conversation context and interpolated as a literal in the subsequent call — never via shell variables.

## Workflow

### Approval Protocol

- **Auto-proceed**: Typo fixes, formatting, dotfile tweaks, single-skill edits
  that follow existing patterns.
- **Ask first**: New skills/agents, cross-skill interface changes, template
  changes that affect downstream projects, convention changes.
- **Always ask**: Deleting skills/agents, changing canonical flows, modifying
  migration checklist semantics.

### Commit Rules

Auto-create git commits, each covering one logical unit of change.
Include an **AI context** section in every commit message recording design decisions,
alternatives considered, and trade-offs — focus on _why_ this approach was chosen.

```
<type>(<scope>): <summary>

<what changed — brief>

## AI Context
- <decision rationale, rejected alternatives, user directives, etc.>

## Ticket Updates                          # optional — only when ticket-driven
- <ticket-stem>[: <optional-label>]
  > Forward: <what future phases must know>
```

### Context Window Discipline

- Source code is ground truth; load only docs relevant to the current task. Update drifted docs on contact.

## Project Knowledge

**Language:** All AI-authored artifacts — commit messages, code comments,
documents — must be in English regardless of conversation language.

**Template maintenance:** When adding a migration checklist item that
supersedes an earlier one, mark the old item `[obsoleted by vNNNN]`.

<!-- Inclusion test: if breaking this rule makes a skill produce
     wrong results, it belongs here. Everything else goes in
     _index.md (context) or skills (process). -->

<!-- Template Version: v0015 -->
