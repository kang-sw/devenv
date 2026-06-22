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

### Status convention

- **`idea/` = discussion scratchpad.** Tickets may be created at `idea/` early and
  updated freely as design is explored. No sage review, no spec-address gate.
  `create-ticket` tip for `idea/` creation: "promoting to `todo/` will trigger
  sage review."
- **`todo/` = semi-frozen artifact.** Landing here signals that design is committed
  and sage-reviewed. Casual design changes should demote back to `idea/` first.
- **`ready/` = picked up for implementation.**

### Write-ticket judge gate

`lead-write-ticket` classifies every write request before routing:

| Intent | Target |
|--------|--------|
| Proceed context — implementation starting now | `ready/` |
| Design confirmed (new ticket or idea/ promotion) | `todo/` + sage ask |
| Substantive design change to existing `todo/` | demote to `idea/`, continue discussion |
| Non-substantive edit to `todo/` (wording, detail, related links) | `todo/` stays, no re-review |
| Any edit to `idea/` | `idea/` stays, no sage |

### Sage review mechanics

- **Opt-in via config**: `sage_review: auto | ask | off` (default: `off`).
  Resolves through session > proj > user config layers.
- **Trigger**: any write landing at `todo/+` when config is on.
- **Fresh reader framing**: sage receives ticket + all linked docs (full repo
  access); zero conversation context injected. Goal: produce a coherent
  implementation plan sketch. Inability to do so is a signal.
- **Verdict-based, not direct-edit**: reviewers return structured verdicts; lead
  synthesizes and writes `## Blocked (YYYY-MM-DD)` + updates frontmatter. Sage
  does not edit the ticket body directly.
- **`sage-review` frontmatter field**: `pending | completed | blocked | skipped`.
  Set to `pending` on creation at `todo/+`. `skipped` bypasses the hard lock with
  an audit trail.
- **MCP transition enforcement**: the ticket transition tool (260620) checks the
  `sage-review` field when config is on — fails transition on `pending | blocked`.
  Sage-agnostic: field read only.
- **Block output**: lead writes `## Blocked (YYYY-MM-DD)` section (h2, consistent
  with ticket heading convention) from reviewer verdict; sets `sage-review:
  blocked`. Actual design edits are then discussed and demote the ticket to
  `idea/` before a new review cycle.

### Reviewer model

Two reviewers run in parallel:

| Reviewer | Default tier | Dimensions |
|----------|-------------|------------|
| Design | `large` (configurable) | design coherence, duct-tape detection, right-problem check, autonomous-vs-missing gap judgment |
| Completeness | `medium` (configurable) | ticket structure, missing fields, "unclear to fresh reader" items |

Aggregation: design `block` → final block regardless of completeness. Completeness
`concern` → lead judgment on whether to block.

Config keys (all scalar, layerable):

```
sage_review: auto | ask | off
sage_review_design_tier: large
sage_review_completeness: true | false
sage_review_completeness_tier: medium
```

### `create-ticket` MCP tool

New `ws/create_ticket(session_key, stem, initial_state)`:
- Auto-prefixes today's date to semantic `stem`.
- Writes frontmatter stub (title placeholder, `sage-review: pending` for `todo/+`).
- Returns `{path, tip}`:
  - `idea/`: tip = "promoting to `todo/` will trigger sage review."
  - `todo/+`: tip = "run sage review before promoting further."

### `create-ticket` and `lead-write-ticket` relationship

`create-ticket` is an MCP tool (stub write + frontmatter). `lead-write-ticket` is
a playbook that calls `create-ticket` for the actual file creation, then wraps it
with consent gate, intent review, spec-address check, and cross-ticket decision
review. Different layers — no conflict.

### `ask` mode interaction

When `sage_review: ask`, after the ticket write the lead agent asks the user
whether to run sage review. A "no" answer sets `sage-review: skipped` and
proceeds. A "yes" (or `auto` mode) runs the reviewers.

### `skipped` write authority

`sage-review: skipped` is set only when the user explicitly declines in `ask`
mode. It is never set when `sage_review: off` — in that mode the `sage-review`
field is not checked by the transition tool at all.

### 260620 coupling

260620 implements the transition tool. The `sage-review` pre-condition hook
belongs in Phase 1 of 260620 (alongside the mutation path — natural fit).
This ticket builds the sage reviewer playbooks, `create-ticket` tool, and config
integration on top.

## Open Questions

- `todo/ → ready/` re-review: ticket may have gained implementation phases since
  `todo/` write. Re-run sage or treat `completed` as sufficient? Affects Phase 3
  transition hook condition.
- `create-ticket` `parent:` inference: should the tool query the session's active
  epic context and pre-fill `parent:`? Affects Phase 1 implementation scope.

## Phases

### Phase 1: `create-ticket` MCP tool

New `ws/create_ticket(session_key, stem, initial_state)` Go MCP handler.
Auto-prefixes date, writes frontmatter stub with `sage-review: pending` for
`todo/+`, returns `{path, tip}`. Tip is non-empty for both `idea/` (reminder) and
`todo/+` (sage prompt).

### Phase 2: Reviewer playbooks + lead-write-ticket integration

- `ticket-reviewer-design` playbook: receives ticket path, reads ticket + linked
  docs, attempts implementation plan sketch, returns structured verdict covering
  design coherence, duct-tape, right-problem, and autonomous-vs-missing gaps.
- `ticket-reviewer-completeness` playbook: receives ticket path, returns structured
  verdict on ticket structure and clarity gaps.
- `lead-write-ticket` gains the judge gate (see Decisions) and invokes both
  reviewers in parallel after ticket commit when `sage_review` config is `auto |
  ask` and landing status is `todo/+`. Lead synthesizes verdicts and writes
  `## Blocked` section on block.

**Verdict schema** (each reviewer emits):

```
verdict: pass | concern | block

issues:
  - title: <short label>
    severity: critical | important | minor
    detail: <what is unclear or wrong>
    resolution: autonomous | missing
```

**Lead synthesis output** (written to `## Blocked` section on block):

```markdown
## Blocked (YYYY-MM-DD)

### Design Reviewer — <verdict>

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | ...   | important | missing   |

### Completeness Reviewer — <verdict>

| # | Title | Severity |
|---|-------|----------|
| 1 | ...   | important |
```

Aggregation rule: design `block` → final block regardless of completeness result.
Completeness `concern` → lead judgment on whether to set `blocked`.

### Phase 3: Config integration + 260620 transition hook

Add all four `sage_review*` keys to the config schema (session > proj > user).
Coordinate with 260620: add the `sage-review` pre-condition check gated on
`sage_review` config. If 260620 ships first without the hook, append an Edition to
its Phase result.
