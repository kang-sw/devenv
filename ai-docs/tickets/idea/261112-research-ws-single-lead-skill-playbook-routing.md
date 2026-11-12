---
title: "One lead skill (lead-do) routes to playbooks instead of many state-like lead skills"
related:
  260605-epic-ws-playbook-factory-pivot: earlier move of skill bodies into rendered playbooks
---

# One lead skill (lead-do) routes to playbooks instead of many state-like lead skills

## Background

User idea (2026-11-12), stated as a weak opinion with the direction still
open:

- Collapse the lead skills to a single `/lead-do`. It would say: load the
  workflow manual if it is not loaded yet; then run the skill that fits,
  according to the descriptions the exposed `lead-*` MCP surface gives, or
  the one the user named. A deduplicated list of playbooks with their
  descriptions would be pasted in, perhaps by rendering the playbook list in
  the workflow manual itself.
- Motivation: skills have become playbook launchers while the daemon does most
  of the work, so the autonomous behaviour mixed into skill bodies reads as
  behavioural noise. Orchestration sessions are long, and because the skills
  are designed to behave like states, implementation work keeps bouncing
  between `lead-run`, `lead-delegate`, and `lead-discuss`.
- Alternatives the user floated:
  - put the discussion, run, and delegate methodology directly into the
    workflow manual and let the agent route with no skill concept at all;
  - keep on-demand loading, if loading only when needed is actually right.
- Counterweight the user named: what the user can invoke is exposed as
  skills, so user-invocable entry points still matter.

## Questions to investigate

- How much of each lead skill body today is routing and state guidance versus
  a playbook launch; which parts the daemon already owns.
- What the per-turn context cost is of (a) many skills loaded on demand,
  (b) one skill plus a rendered playbook list, (c) everything in the workflow
  manual.
- Whether the "state" framing (one active skill at a time) causes the observed
  bouncing, or whether the bouncing comes from routing rules between skills.
- What a user-invocable surface looks like under each option (slash entries
  on Claude Code, Codex, Pi), and what downstream projects lose or gain.
- How wsflow's thin skill shims and the mirroring rules change under each
  option.

## Outcome Ledger

### Verified Findings

### Confirmed Decisions

### Proposals

- A single `lead-do` skill plus a deduplicated playbook list rendered in the
  workflow manual (user, weak opinion).
- No skills: discussion, run, and delegate methodology in the workflow manual,
  agent-routed (user, floated).

### Open Questions

- Which of the three shapes (single entry skill, manual-only, on-demand
  skills as today) to pursue, if any.
- How user-invocable entry points survive under the chosen shape.

### Rejected Alternatives
