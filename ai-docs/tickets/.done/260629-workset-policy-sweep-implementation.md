---
title: "Policy sweep implementation (post-260629 triage)"
completed: 2026-06-30
---

# Policy sweep implementation (post-260629 triage)

## Context

Follow-up operating board for the `feature/ferrule` pre-merge triage sweep (`260629-workset-pre-merge-idea-backlog-sweep`). Every ticket here had a policy decision pinned in that sweep. Divided into two tracks: fixes that can proceed immediately, and two design-first tickets that require a joint session before implementation.

## Tickets

### Track 1 — Ready to implement

- `260619-research-ws-delegate-continuity-host-neutral-fallback` — docs fix: replace SendMessage-assuming continuity tip with host-neutral guidance
- `260617-feat-fresh-reader-audit-playbook` — build fresh-reader-audit delegate playbook
- `260626-feat-user-preference-save-routing` — prose: lead-tune = ws workflow prefs, lead-add-rule = project rules; add cross-reference
- `260629-design-enter-proceed-no-obsolete-target-route` — lead-proceed scope resolution: add phase-completion check
- `260627-bug-write-ticket-bypasses-tickets-create` — lead-write-ticket + lead-discuss/sprint: enforce ws/tickets.create routing (A+B combined)
- `260620-bug-ws-prompt-override-no-unset-path` — Go MCP: make harness optional in config.prompt.set/unset

### Track 2 — Design session first (implement together)

- `260525-bug-lead-implement-delegation-pre-edit-guard` — pre-edit predicate gate; verdict form and insertion point to be decided
- `260627-bug-enter-implement-direct-edit-policy-gap` — enter.implement direct-edit override channel; schema direction to be decided

## Planned References

- None.

## Focus

Implement the policy-character decisions resolved in the 260629 ferrule pre-merge triage sweep.

## Exit Criteria

- Done: all Track 1 tickets implemented and closed; Track 2 design session held and both tickets implemented.
- Deferred: research-class tickets and epic-owned items from the parent sweep remain in their own tracks.


## Resolution (2026-06-30)

All Track 1 (6 tickets) and Track 2 (2 tickets) implemented and closed. Commits: b7195a6c (direct-edit gate schema + playbook), 4a89053e (5 playbook fixes), 71d082eb (Go harness optional).
