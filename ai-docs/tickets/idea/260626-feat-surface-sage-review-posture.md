---
title: "Surface sage-review posture without a config/frontmatter dive"
parent: 260605-epic-ws-playbook-factory-pivot
related:
  260625-feat-ws-session-state-machine: motivating-feature
---

# Surface sage-review posture without a config/frontmatter dive

## Background

During the 260625 dogfood the lead noted friction: determining whether the
review ("sage") step applies for the current work requires manually digging into
state every time, instead of being visible at the decision point.

## Findings (grounded)

- `config.tuning` exposes NO review/sage knob. The current tuning surface is
  limited to: `prompt.PreferSubagentInvocationGuidance`,
  `prompt.UserPreferenceSection`, `workflow.prefer_subagent`,
  `workflow.prefer_mercenary`, `agents.tier`. So "check config for sage-review"
  has no direct knob today.
- `sage-review` is a TICKET FRONTMATTER field: `tickets.create` stamps
  `sage-review: pending` on `todo`/`ready` tickets (not `idea`), and promotion
  `idea -> todo` is reported as the trigger ("promoting to 'todo/' will trigger
  sage review").
- Net: the lead-perceived "check config" is really a check of ticket frontmatter
  (or a separate review configuration not surfaced by `config.tuning`). The
  authoritative source of "is review required for THIS work" is not surfaced at
  the moment the lead derives an implement checklist.

## Proposed direction

- Confirm the authoritative source of sage-review enablement (ticket
  `sage-review:` frontmatter vs. a review config) and document it.
- Surface the resolved review posture at the decision points the session-state
  machine already owns:
  - Echo it in `ws.enter.implement` output, so deriving the implement checklist
    already reports whether the Review step applies (and ideally defaults
    `need_review` from it instead of requiring the caller to pass it blind).
  - Include a one-line review/delegation posture reminder at `ws.workflow_manual`
    load, so it survives compaction without a file dive.

## Open question

- Is there a global "sage review on/off" toggle that should become a
  `config.tuning` knob, distinct from the per-ticket `sage-review:` field?
