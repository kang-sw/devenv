---
name: lint
description: >
  Comprehensive workflow audit. Finds portability bugs, broken cross-file
  references, structural non-compliance, undefined judgment refs, and
  cross-file logic errors across all plugin skill, agent, and infra docs.
argument-hint: "[no args — scans full plugin + project docs]"
---

# Lint

## Invariants

- Read-only. Never edit any file.
- Locate plugin root via `which load-infra` before any scan — never assume CWD equals plugin root.
- Run all five mechanical checks before spawning the semantic agent.
- Report every finding; never suppress or silently group.
- Omit a check category from the report only when it has zero findings.

## On: invoke

### 1. Orient

```bash
PLUGIN_ROOT="$(dirname "$(dirname "$(which load-infra)")")"
PROJECT_ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
```

Collect file lists:
```bash
SKILL_FILES=$(find "$PLUGIN_ROOT/claude/skills" -name "SKILL.md")
AGENT_FILES=$(find "$PLUGIN_ROOT/claude/agents" -name "*.md")
INFRA_FILES=$(find "$PLUGIN_ROOT/claude/infra" -name "*.md")
SPEC_FILES=$(find "$PROJECT_ROOT/ai-docs/spec" -name "*.md" 2>/dev/null)
TICKET_FILES=$(find "$PROJECT_ROOT/ai-docs/tickets/todo" "$PROJECT_ROOT/ai-docs/tickets/wip" \
  -name "*.md" 2>/dev/null)
```

### 2. Mechanical checks

Run each check. Accumulate findings labeled by check letter.

**A — Portability: bare plugin-relative paths**

Skill, agent, and infra docs must not reference plugin-internal filesystem paths as literals.
These paths resolve correctly inside the plugin repo but fail in every downstream project.

Scan for the offending patterns:
```bash
grep -rn -F 'claude/infra/'  $SKILL_FILES $AGENT_FILES $INFRA_FILES
grep -rn -F 'claude/skills/' $SKILL_FILES $AGENT_FILES $INFRA_FILES
grep -rn -F 'claude/agents/' $SKILL_FILES $AGENT_FILES $INFRA_FILES
grep -rn -F 'claude/bin/'    $SKILL_FILES $AGENT_FILES $INFRA_FILES
```

For each hit: inspect whether the containing code block carries a `# Before`, `# Bad`, or `# Wrong`
comment — if so, annotate as "(intentional negative example)" and do not count as a finding.

Fix hints by pattern:
- `--system-prompt claude/infra/<name>` → `--system-prompt $(ws-infra-path <name>)`
- `claude/bin/<script>` used as a path → bare `<script>` name (in PATH after plugin install)
- Any other `claude/<dir>/` literal → obtain the path via the appropriate bin script

**B — load-infra misuse (content where a path is expected)**

`load-infra` outputs file content. Passing its output to any argument that expects a file path is wrong.

```bash
grep -rn 'system-prompt.*$(load-infra\|$(load-infra.*system-prompt' \
  $SKILL_FILES $AGENT_FILES $INFRA_FILES
```

Fix: `--system-prompt $(ws-infra-path <name>)`.

**C — Structural compliance per skill-authoring.md**

Each `SKILL.md` must contain both `## Invariants` and `## Doctrine`:
```bash
for f in $SKILL_FILES; do
  grep -qF '## Invariants' "$f" || echo "$f: missing ## Invariants"
  grep -qF '## Doctrine'   "$f" || echo "$f: missing ## Doctrine"
done
```

Each agent `*.md` must contain `## Constraints`, `## Process`, `## Output`, and `## Doctrine`:
```bash
for f in $AGENT_FILES; do
  for sec in '## Constraints' '## Process' '## Output' '## Doctrine'; do
    grep -qF "$sec" "$f" || echo "$f: missing $sec"
  done
done
```

**D — Undefined judgment references**

For each `SKILL.md`: every `judge: <name>` call must resolve to a `### judge: <name>` definition
within the same file.

```bash
for f in $SKILL_FILES; do
  grep -o 'judge: [a-z-]*' "$f" | sed 's/judge: //' | sort -u | while read name; do
    grep -qF "### judge: $name" "$f" \
      || echo "$f: 'judge: $name' called but not defined in this file"
  done
done
```

**E — Missing infra doc targets**

For every `load-infra <doc>` and `ws-infra-path <doc>` call, verify the named doc exists
under `$PLUGIN_ROOT/claude/infra/`.

```bash
grep -rn 'load-infra [a-zA-Z._-]*\|ws-infra-path [a-zA-Z._-]*' \
  $SKILL_FILES $AGENT_FILES $INFRA_FILES \
  | while IFS=: read file line rest; do
      doc=$(echo "$rest" | grep -o '\(load-infra\|ws-infra-path\) [a-zA-Z._-]*' | awk '{print $2}')
      [[ -z "$doc" || -f "$PLUGIN_ROOT/claude/infra/$doc" ]] \
        || echo "$file:$line: references non-existent infra/$doc"
    done
```

### 3. Semantic check

Spawn an Explore agent. Provide:
- All file paths collected in step 1 (plugin + project docs).
- The plugin root path.

Instruct it to read every file and report findings in three sub-categories:

1. **Anchor cross-references** — `{#YYMMDD-slug}` tokens in ticket files that do not appear
   in any spec file. Report: `<ticket-file>: references {#<slug>} not found in any spec`.

2. **Step numbering** — `On:` handler sections in SKILL.md files where the numbered step
   list has gaps, duplicates, or a step that references a step number that does not exist
   in the same handler.

3. **Named cross-file references** — one file references a named section, doc, or script
   that does not exist (e.g., `load-infra executor-wrapup.md` where `executor-wrapup.md`
   is absent from infra/). Exclude findings already covered by Check E.

### 4. Report

```
## A — Portability
<file>:<line>: <offending pattern> → <fix hint>

## B — load-infra misuse
<file>:<line>: <offending call> → use $(ws-infra-path <name>)

## C — Structural compliance
<file>: missing <section>

## D — Undefined judgments
<file>: 'judge: <name>' called but not defined

## E — Missing infra targets
<file>:<line>: references non-existent infra/<doc>

## F — Semantic (cross-file integrity)
<finding>
```

End with: `N total finding(s) across K categories.` or `No issues found.`

## Doctrine

Lint optimizes for **finding coverage per run** — every detectable issue across
the full scanned scope must appear in the report with enough location context for
the reader to fix it directly. When a check result is ambiguous, report with a
confidence qualifier rather than suppress.
