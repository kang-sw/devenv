---
name: skill-lint
description: >
  Scan skill and infra docs for portability issues. Reports bare
  --system-prompt relative paths that break outside the devenv repo.
argument-hint: "[path — optional, defaults to full claude/ scan]"
---

# Skill Lint

## Invariants

- Read-only. Never edit files.
- Scan scope is fixed: `claude/skills/**/SKILL.md` and `claude/infra/*.md`.
- Report every finding before summarizing — never suppress.

## On: invoke

1. Run the scan:
   ```bash
   grep -rn -F -- '--system-prompt claude/infra/' claude/skills/ claude/infra/ \
     | grep -v 'skill-lint/SKILL.md'
   ```
2. For each matching line, format as:
   ```
   <file>:<line>: --system-prompt claude/infra/<name> → use $(ws-infra-path <name>)
   ```
3. Output summary: `N portability issue(s) found.` or `No portability issues found.`

## Doctrine

Skill-lint optimizes for **finding coverage per invocation** — every offending
line in the scan scope must appear in the report. When a rule is ambiguous,
apply whichever interpretation surfaces more findings rather than fewer.
