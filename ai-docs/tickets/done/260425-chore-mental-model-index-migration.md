---
title: Move mental-model.md to mental-model/index.md
related-mental-model:
  - doc-tooling
  - workflow-routing
completed: 2026-04-25
---

# Move mental-model.md to mental-model/index.md

## Background

`ai-docs/mental-model.md` is titled "Mental Model Index" and functions as the
top-level index for the `ai-docs/mental-model/` directory. The directory already
uses the `<domain>/index.md` pattern for sub-domain hierarchies (e.g.,
`mental-model/doc-tooling/index.md`). The top-level index does not follow this
same convention — it sits outside the directory it describes.

Moving `ai-docs/mental-model.md` → `ai-docs/mental-model/index.md` makes the
hierarchy fully consistent: at every level, the index of a directory is
`index.md` inside that directory.

## Decisions

- **`ws-proj-tree` needs no changes.** It renders the filesystem dynamically;
  after the move, `index.md` will appear inside `mental-model/` naturally.
- **`ws-list-mental-model` needs logic changes, not just path substitution.**
  Currently it hardcodes `Path('ai-docs/mental-model.md')` and renders it
  as "overview" outside the domain listing. After the move, `discover_docs()`
  would pick up `index.md` as a flat domain doc and `domain_key()` would return
  `'.'` — a broken key. The fix: detect `mental_model_dir / 'index.md'` at the
  root, exclude it from `flat_docs`, and render it as the overview.
- **Bootstrap template** (`CLAUDE.template.md`) references the old path and
  includes a `git mv ... mental-model.md` migration instruction. Both must be
  updated to reflect the new canonical layout.

## Phases

### Phase 1: File move and ws-list-mental-model script update

Move the file and fix the Python script logic:

1. `git mv ai-docs/mental-model.md ai-docs/mental-model/index.md`
2. In `claude/bin/ws-list-mental-model`:
   - Change `overview = Path('ai-docs/mental-model.md')` → `overview = Path('ai-docs/mental-model/index.md')`
   - In the tree header comment: change `../mental-model.md` → `index.md`
   - In `discover_docs()`: after collecting `flat_docs = sorted(mental_model_dir.glob('*.md'))`, exclude any `index.md` at the root of `mental_model_dir` (the root overview). This prevents the root `index.md` from being processed as a domain doc with key `'.'`.
   - In `render_domain` call for overview: update the `doc_name` argument from `'../mental-model.md'` to `'index.md'`.

Success criteria: `ws-list-mental-model` runs without error; the root overview
renders as `"overview"` domain, not as a flat domain doc.

### Result (62b3328) - 2026-04-25

Completed as planned. Five script changes applied to `ws-list-mental-model`:
excluded `index.md` from `flat_docs` via list comprehension, updated the
overview path, tree header comment, and `render_domain` doc_name arg. A missed
hardcoded line (`#   overview (../mental-model.md)` in the tree section) was
found during verify and fixed before commit.

### Phase 2: Documentation path reference updates

Mechanical path substitution in 9 files. Every occurrence of `ai-docs/mental-model.md`
becomes `ai-docs/mental-model/index.md`:

- `claude/infra/mental-model-conventions.md` — Structure section definition
- `claude/agents/mental-model-updater.md` — read step + update step
- `claude/agents/project-survey.md` — context read step
- `claude/infra/skeleton-writer.md` — read step
- `claude/skills/forge-mental-model/SKILL.md` — commit stamp pattern + update step
- `claude/skills/write-plan/plan-writer.md` — read step
- `claude/skills/write-plan/SKILL.md` — read step
- `claude/skills/write-plan/survey-writer.md` — read step
- `claude/skills/bootstrap/CLAUDE.template.md` — layout description + migration instruction

For `bootstrap/CLAUDE.template.md`, additionally update the migration instruction that
currently reads `git mv ai-docs/mental-model/overview.md ai-docs/mental-model.md`
— this instruction no longer applies; the canonical layout now places the top-level
index at `ai-docs/mental-model/index.md` from the start.

Success criteria: `grep -r "mental-model\.md" claude/ ai-docs/mental-model/` returns
no matches outside of historical ticket/plan files.

### Result (00f1872) - 2026-04-25

10 files updated (9 planned + `ai-docs/spec/workflow-skills.md` discovered during
the final grep verification). Bootstrap template: standalone `mental-model.md`
directory listing line removed; unified into the `mental-model/` line. Post-implementation
review (b573f5c) fixed two minor issues: tree header overview guard and a redundant
OR clause in forge-mental-model's commit-stamp invariant.
