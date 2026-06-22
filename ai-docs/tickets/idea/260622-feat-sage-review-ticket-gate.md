---
title: Sage review — design-quality gate for ticket writes
related:
  260620-feat-ws-ticket-status-transition-tools: transition tool must check sage-review frontmatter field; this ticket builds the sage reviewer playbook and create-ticket surface on top
related-mental-model:
  - workflow-skills
---

# Sage review — design-quality gate for ticket writes

## Background

Lead agents at Sonnet tier can miss design-level problems at ticket-write time.
Large-model (Opus) sessions are too slow for routine mechanical work. The gap:
bad design decisions get locked into tickets — especially at `todo/` level where
system-wide design accumulates — before anyone stress-tests them from a fresh
perspective.

Proposed gate: after writing a ticket to `todo/` or above, optionally spawn a
large-tier subagent as a fresh implementer — full repo access, no conversation
context injected — and ask whether it can produce a coherent implementation plan.
If it cannot, the ticket is flagged before implementation starts.

## Decisions

- **Opt-in via config**: `sage_review: auto | ask | off` (default: `off`).
  Resolves through session > proj > user config layers. Both the playbook
  enforcement and the MCP transition hard-lock respect this config.
- **Trigger rule**: any ticket write landing at `todo/` or above triggers sage
  review when config is on. Covers: idea→todo, idea→ready (skip), todo→ready,
  new ticket created at todo/+.
- **Fresh reader framing**: sage receives the ticket and all linked docs (full
  repo access), but zero conversation context. Question: "can a fresh implementer
  produce a coherent implementation plan from this ticket?"
- **Block handling**: sage writes a `## Blocked (YYYY-MM-DD)` section (h2,
  consistent with ticket heading convention) with issue summary and suggested
  directions + sets `sage-review: blocked` in frontmatter. Actual body edits are
  elevated back to lead.
- **`sage-review` frontmatter field**: `pending | completed | blocked | skipped`.
  Set to `pending` on ticket creation at `todo/+`; updated by sage on review
  completion. `skipped` bypasses the hard lock with an audit trail.
- **MCP transition enforcement**: the ticket transition tool (260620) checks
  `sage-review` field when config is on — fails transition if `pending | blocked`.
  Tool is sage-agnostic: it only reads the field value, does not invoke sage.
- **`create-ticket` MCP tool**: new tool accepting `session_key`, semantic `stem`
  (no date prefix), and `initial_state`. Auto-prefixes today's date to stem.
  Writes frontmatter stub; sets `sage-review: pending` for `todo/+` tickets.
  Returns absolute path + tip text reminding caller to run sage review.
- **260620 coupling**: 260620 implements the transition tool with the
  `sage-review` pre-condition hook (minimal coupling — field read only). This
  ticket builds the sage reviewer playbook, `create-ticket` tool, and config
  integration on top.

## Open Questions

- Should `todo/ → ready/` re-run sage review (ticket may have gained
  implementation phases since the `todo/` write) or is one review per lifecycle
  sufficient?
- Reviewer playbook output format: structured `verdict: pass | concern | block` +
  items, or free-form with a verdict header?
- Should `create-ticket` infer a `parent:` from the session's active epic context?

## Phases

### Phase 1: `create-ticket` MCP tool

New `ws/create_ticket(session_key, stem, initial_state)` tool. Auto-prefixes date,
writes frontmatter stub, sets `sage-review: pending` for `todo/+`, returns
`{path, tip}` where `tip` is non-empty when `initial_state >= todo/`.

### Phase 2: Sage reviewer playbook + lead-write-ticket integration

New `ticket-reviewer-sage` playbook: receives ticket path, reads ticket + linked
docs, attempts to produce an implementation plan sketch, returns structured
verdict. `lead-write-ticket` invokes the playbook after ticket commit when
`sage_review` config is `auto | ask` and `initial_state >= todo/`. Sage updates
frontmatter to `completed` or `blocked`; adds `## Blocked` section on block.

### Phase 3: Config integration + 260620 transition hook

Add `sage_review: auto | ask | off` to the config schema (session > proj > user).
Coordinate with 260620: when that tool lands, add the `sage-review` pre-condition
check gated on `sage_review` config. If 260620 ships first without the hook,
append an Edition to its Phase result.
