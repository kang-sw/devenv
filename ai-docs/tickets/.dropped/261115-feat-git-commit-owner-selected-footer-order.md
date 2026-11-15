---
title: "Allow structured Git commits to preserve an owner's required footer order"
---

# Allow structured Git commits to preserve an owner's required footer order

## Background

Dogfood: an owner required commit messages to end in `## AI Context` bullets, with a human-added co-author trailer later. The worker used `git.commit` with ticket summaries, which appended `## Ticket Updates` after the context and could not satisfy that ordering. The implementation commit is `60cb167ee`; the worker left its message unchanged because its branch contract prohibits amending commits.

`internal/wsgit/git.go` currently writes AI Context before Ticket Updates unconditionally (search `commitMessage`). Ticket-touching commits also infer summaries when `len(opts.UpdatedTickets) == 0`, so an explicit empty array is not a suppression mechanism even though the tool description says inference occurs when omitted.

## Open Questions

- How should an owner request a supported section order without hand-authoring a native Git commit or changing every caller's default?
- Should an explicit empty `updated_tickets` array suppress inference, and how should that differ from omission?

## Phases

### Phase 1: Settle and support the footer contract

After owner approval, choose the smallest explicit contract that preserves current defaults, permits the owner's required final context section, and makes omission versus suppression clear. Cover structured and automatically inferred ticket updates, plus co-author trailers. Settle compatibility before changing any tool argument; this idea authorizes no API or behavior change yet.

## Resolution
Triage 2026-11-15 (owner (1057)): DROP - A request to set footer order whose core is the same as the empty updated_tickets suppression problem; 261115-research-git-commit-empty-ticket-updates.
