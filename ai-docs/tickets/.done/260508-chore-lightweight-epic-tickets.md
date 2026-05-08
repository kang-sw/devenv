---
title: lightweight epic tickets
related:
  260503-epic-ws-agent-workflow-stability: prior example of an epic reduced to a roadmap after child tickets carried implementation slices
spec:
  - 260508-lightweight-epic-ticket-conventions
  - 260508-write-ticket-epic-child-boundary
related-mental-model:
  - documentation-system
  - workflow-skills
completed: 2026-05-08
---

# lightweight epic tickets

## Background

Epic tickets should act as milestone tracking boards, not as accumulated design
logs or implementation plans. When an epic absorbs detailed phase discussion,
future sessions have to re-read stale debate before they can identify the
current child work. That makes the epic less useful as a recovery artifact even
when the captured details were individually valuable.

The intended model is lightweight: keep the epic focused on scope,
decomposition, cross-child decisions, and completion state. Move detailed
discussion and implementation intent into child tickets. A child ticket may
cover multiple phases when those phases form one cohesive reviewable unit.

## Decisions

- Treat epics as milestone boards and decomposition records.
- Keep detailed discussion, approaches, constraints, and implementation phases
  in child tickets.
- Allow one child ticket to carry multiple phases when the phases share one
  cohesive review and implementation surface.
- Keep only durable cross-child decisions in the epic itself.
- Prefer child-ticket creation over growing an epic when new detail exceeds the
  board-level scope.

## Value Judgment

This policy trades some local convenience for better long-session recovery.
Keeping details in an epic can feel faster during discussion, but it turns the
parent into a second source of truth once child tickets exist. Moving detail
down keeps ownership clear: the epic answers "what remains and how is it
partitioned"; child tickets answer "what exactly should be done and why."

The gray area is cross-child architecture. The epic should keep only decisions
that govern multiple children. A decision that affects one implementation slice
belongs in that child ticket even if the discussion happened while looking at
the epic.

## Phases

### Phase 1: Update ticket conventions

Harden ticket conventions so epic bodies are intentionally lightweight. Define
the expected epic contents:

- scope and non-scope;
- child ticket board;
- cross-child invariant decisions;
- done, dropped, or deferred criteria.

State that detailed discussion and implementation phases move to child tickets.

### Result (24668e0) - 2026-05-08

Implemented the lightweight epic convention in the bundled ticket conventions
and Claude compatibility copy. Epic bodies now define scope, non-scope, child
ticket boards, cross-child invariant decisions, and done/drop/defer criteria;
detailed discussion and implementation phases belong in child tickets.

### Phase 2: Update workflow skills and guidance

Update `lead-write-ticket` and related planning guidance so epic creation and
editing preserve the lightweight board model. When new detail appears during
epic discussion, the skill should create or update a child ticket instead of
expanding the epic body.

### Result (24668e0) - 2026-05-08

Updated Codex and Claude write-ticket guidance so epic operations update only
the board ticket, while child-ticket creation or edits run as separate
write-ticket invocations with their own lifecycle, commits, and output
artifacts. Review follow-up also updated proceed routing so epic ticket paths
stop before skeleton or implementation and route through child tickets instead.

### Phase 3: Refresh specs and mental models

Update documentation-system and workflow-skills specs or mental models as
needed so future ticket work keeps the same epic/child boundary.

### Result (24668e0) - 2026-05-08

Stripped the planned markers from the documentation-system and workflow-skills
specs and refreshed the documentation-system and workflow-skills mental models.
Verification passed with `go test ./internal/wsdoc ./internal/mcp`,
`go test ./...`, and `git diff --check` on the implementation branch.
