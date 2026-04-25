---
name: lint
description: >
  Three-partition parallel review of claude/ plugin documents and ai-docs/.
  Checks logical consistency, broken references, and downstream portability.
argument-hint: "[no args — scans full plugin + project docs]"
---

# Lint

## Invariants

- Read-only. Never edit any file.
- Spawn all three reviewer agents in a single response turn — do not wait between spawns.
- Pass concrete repo-root paths in each agent prompt — never assume agent CWD.
- Aggregate all findings into one consolidated report after all three return.

## On: invoke

### 1. Orient

```bash
git rev-parse --show-toplevel
```

Store result as `<repo-root>`. Plugin docs live at `<repo-root>/claude/`. Project
docs live at `<repo-root>/ai-docs/`.

### 2. Spawn (one Agent call each — all three in a single response turn)

**Reviewer 1 — Logical Consistency** (`ws:document-reviewer`, sonnet)

Scope: `claude/skills/`, `claude/infra/`, `claude/agents/`

```
Repo root: <repo-root>

Review all SKILL.md files under <repo-root>/claude/skills/, all .md files under
<repo-root>/claude/infra/, and all .md files under <repo-root>/claude/agents/.

Check logical consistency only:
- Invariants or procedure steps that contradict each other across files
- A procedure step that references an artifact created later in the same procedure
- Agent spawn prompts that reference a mandate or behavior not defined in the target
  agent doc (cross-check with claude/agents/)
- The same tool described with conflicting signatures or behavior in different docs
  (e.g., two files disagree on ws-call-named-agent argument order)
- Judge names called in a handler (judge: <name>) but not defined in the same file
  (### judge: <name>)

Return findings as: [Critical|Important|Minor] <file>: <description>
Do not flag broken file references or portability issues — those belong elsewhere.
```

**Reviewer 2 — Broken References** (`ws:document-reviewer`, sonnet)

Scope: `claude/skills/`, `claude/infra/`, `claude/agents/`, `ai-docs/`

```
Repo root: <repo-root>

Review all .md files under <repo-root>/claude/skills/, <repo-root>/claude/infra/,
<repo-root>/claude/agents/, and <repo-root>/ai-docs/.

Check broken references only — use Bash/Glob to verify existence on disk:
- Files mentioned or linked that do not exist
- Commands named in bash blocks not present in <repo-root>/claude/bin/ and not in
  standard PATH
- ws-print-infra or ws-infra-path arguments that name infra docs not present under
  <repo-root>/claude/infra/
- Spec anchors ({#YYMMDD-slug}) referenced in tickets or docs that do not appear in
  any spec file under <repo-root>/ai-docs/spec/
- Mental-model or spec files listed in ai-docs/_index.md that do not exist on disk

Return findings as: [Critical|Important|Minor] <file>: <description>
Do not flag logical consistency issues or portability issues.
```

**Reviewer 3 — Downstream Portability** (`ws:code-reviewer`, sonnet)

Scope: all of `claude/` (skills/, agents/, infra/, bin/)

```
Repo root: <repo-root>

Review all files under <repo-root>/claude/.

This plugin is installed into downstream projects where <repo-root>/claude/ is not
present on the filesystem. Check for patterns that will fail outside this repository:

- Bare `claude/infra/<doc>` used as a --system-prompt value instead of
  `$(ws-infra-path <doc>)` — the bare path only resolves inside the plugin repo
- Hardcoded absolute paths (e.g., /Users/..., /home/...) in scripts or docs
- Relative path assumptions that only hold when CWD is the repo root
- Commands referenced in bash blocks that are not in claude/bin/ and not reliably in
  downstream PATH
- Shell scripts in claude/bin/ missing a shebang line or using a shebang tool not
  guaranteed in downstream environments
- References to tools that have been renamed or deleted; known cases include:
  load-infra (→ ws-print-infra), review-path (→ ws-review-path),
  list-mental-model (→ ws-list-mental-model), list-spec-stems (→ ws-list-spec-stems),
  spec-build-index (→ ws-spec-build-index), generate-spec-stem (→ ws-generate-spec-stem),
  merge-branch (→ ws-merge-branch), subquery (→ ws-subquery),
  ws-agent (deleted), ws-declare-agent (deleted)

Return findings as: [Critical|Important|Minor] <file>:<line>: <description>
```

### 3. Aggregate

After all three return, produce one consolidated report:

```
## Findings

[Critical] [consistency|references|portability] <file>: <description>
...
[Important] [consistency|references|portability] <file>: <description>
...
[Minor] [consistency|references|portability] <file>: <description>
...

N total finding(s).
```

Group by severity (Critical first). Tag each finding with its partition.
End with total count or `No issues found.`

## Doctrine

Lint optimizes for **finding coverage per partition** — three focused, non-overlapping
reviewer mandates prevent duplicate findings and keep each reviewer's context narrow.
When a mandate boundary is ambiguous, assign the finding to the partition whose mandate
most closely describes the root cause.
