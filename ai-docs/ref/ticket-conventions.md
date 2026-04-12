# Ticket Conventions

Canonical reference for ticket structure, naming, and lifecycle.

## Path & Naming

- Path: `ai-docs/tickets/<status>/YYMMDD-<category>-<name>.md` — `YYMMDD` is creation date, never changes on move.
- Categories: `bug`, `feat`, `refactor`, `chore`, `research`, `epic`.
- Reference tickets by **stem only** (e.g., `260115-feat-foo-bar`), never by full path.

## Status Flow

- Status is directory-based only: `idea/` → `todo/` → `wip/` → `done/` (or `dropped/`). Never duplicate status in frontmatter.
- Move tickets with `git mv`; no cross-link updates needed.
- Add `started:` date on move to `wip/`, `completed:` date on move to `done/`.

## Phases

- Phase numbers are sequential and **stable** — mark dropped phases `[dropped]`, never renumber.
- One phase touches **one cohesive component** unless the change is inherently cross-component.
- Each phase has its own success criteria or test surface.
- Structure as `### Phase N: <title>` sections. Note inter-phase dependencies explicitly.

## Stems

- Ticket stems are **immutable absolute references** — history is queried by stem (`git log --grep`).
- If a ticket's concept changes fundamentally, create a new ticket that absorbs the old scope and move the old ticket to `dropped/`.

## General

- Tickets are write-once intent documents.
- All ticket content must be in English regardless of conversation language.

## Templates

### Frontmatter

```yaml
---
title: <title>
related:             # optional; list of stems with inline comments
  - 260301-feat-foo  # prerequisite
parent:              # optional; epic stem (e.g., 260401-epic-auth-rewrite)
plans:               # maps phases to plan path stems under ai-docs/plans/ (without .md)
  phase-1: 2026-03/28-1430.event-serialization
skeletons:           # maps phases to skeleton commit hashes
  phase-1: abc1234
started:             # YYYY-MM-DD, added on move to wip/
completed:           # YYYY-MM-DD, added on move to done/
---
```

Both `plans:` and `skeletons:` list only phases that have artifacts — omit phases without a plan or skeleton (no null placeholders). Absence means "not yet created" or "not needed."

### Body (actionable: `feat`, `bug`, `refactor`, `chore`)

```markdown
# <title>

## Background

<problem or goal — what and why>

## Phases

### Phase 1: <title>

<goals, constraints, rationale, rejected alternatives, suggested approaches>

### Phase 2: <title>

...
```

Optional sections — add between `## Background` and `## Phases` when relevant:

- `## Decisions` — design choices with rationale and rejected alternatives.
- `## Constraints` — non-obvious boundaries (performance, compatibility, etc.).
- `## Prior Art` — existing patterns or components to reuse.

### Body (category = `research`)

```markdown
# <title>

## Background

<question or context>

## <Topic heading>

<findings, decisions, rejected alternatives>
```

Research tickets have no phases. Sections after `## Background` are freeform topic headings.

### Epic body (category = `epic`)

- Body defines **scope and decomposition**, not implementation spec.
- Lists child ticket stems (or planned descriptions).
- Child tickets set `parent:` in frontmatter pointing back to the epic stem.
- Epic moves to `done/` when its scope is satisfied.
