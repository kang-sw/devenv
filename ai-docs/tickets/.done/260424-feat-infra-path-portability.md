---
title: Infra path portability fix + /skill-lint utility
spec:
  - 260424-infra-path-portability
---

# Infra path portability fix + /skill-lint utility

## Background

Skills and infra docs in this repo use `--system-prompt claude/infra/<docname>` to
inject agent system prompts via `ws-call-agent`. The `ws-call-agent` script resolves
this path via `cat "$2"` — relative to the current working directory. In the devenv
repo itself this works, but downstream projects have no `claude/infra/` directory
under their root, so every `ws-call-agent` call with a bare relative path silently
fails or errors.

The correct pattern is `--system-prompt $(load-infra <docname>)`: `load-infra` is a
bin script that resolves the absolute path of any infra doc from its own script
directory (the plugin root), making it CWD-independent.

Known affected files at time of writing:
- `claude/skills/implement/SKILL.md` — implementer and all three reviewer spawns
- `claude/infra/ws-orchestration.md` — usage pattern examples

This class of bug likely appears elsewhere in the codebase. Phase 1 fixes known
occurrences; Phase 2 adds a linter so future occurrences are caught before shipping.

## Decisions

- **Scan scope for /skill-lint**: SKILL.md files in `claude/skills/` and infra docs
  in `claude/infra/`. The linter is a local utility skill, not a CI check — it runs
  on demand and reports findings without auto-fixing.

- **Findings format**: file path + line number + the offending pattern + the
  corrected form. Machine-readable enough to copy-paste fixes; human-readable enough
  to scan quickly.

- **No auto-fix in Phase 2**: linter reports only. Auto-fix risks clobbering
  multi-line constructs or heredoc contexts; manual application is safer for a first
  pass. Revisit if the fix pattern proves stable.

## Phases

### Phase 1: Fix known --system-prompt bare-path callers

Replace all `--system-prompt claude/infra/<docname>` occurrences in:
- `claude/skills/implement/SKILL.md` (implementer spawn + three reviewer spawns)
- `claude/infra/ws-orchestration.md` (usage examples)

Replacement pattern:
```
# Before
--system-prompt claude/infra/implementer.md

# After
--system-prompt $(load-infra implementer.md)
```

Also scan the full `claude/` tree for any other occurrences of
`--system-prompt claude/infra/` and fix them in the same commit.

Success: `grep -r '--system-prompt claude/infra/' claude/` returns no results.

### Phase 2: Implement /skill-lint utility skill

Create `claude/skills/skill-lint/SKILL.md`. The skill:

1. Scans `claude/skills/**/SKILL.md` and `claude/infra/*.md` for the pattern
   `--system-prompt claude/infra/` (bare relative path).
2. Reports each finding as: `<file>:<line>: --system-prompt claude/infra/<name> → use $(load-infra <name>)`.
3. Exits with a summary: N findings, or "No portability issues found."

The skill uses Bash grep to implement the scan — no subagent delegation needed.

Success: `/skill-lint` runs and correctly identifies the pattern (verified against a
deliberately introduced test occurrence, then removed).
