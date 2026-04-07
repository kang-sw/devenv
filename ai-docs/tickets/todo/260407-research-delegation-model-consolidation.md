---
title: "Delegation model consolidation — sprint redesign, shared spawn patterns, agent decolonization"
related:
  - 260405-research-marathon-delegation-hardening  # marathon-specific subset
---

# Delegation Model Consolidation

After extracting shared infrastructure (`infra/impl-playbook.md`,
`infra/impl-process.md`, `infra/agents/`), the delegation model across
skills has clear structural problems. This ticket captures the remaining
design decisions.

## Problem

Three entry points (implement, sprint, marathon) compose four concerns
(implementation discipline, lifecycle, team communication, session model)
in inconsistent ways. Agent role files now live in `infra/agents/` but
their content still assumes marathon context. Sprint carries full
lifecycle ceremony despite being "lightweight."

## Completed (this session)

- Extracted `infra/impl-playbook.md` (subagent-safe) and `infra/impl-process.md` (top-level only).
- Merged execute-plan into implement; deleted execute-plan.
- Moved `marathon/agents/` → `infra/agents/`; updated all references.
- Updated install.sh for per-entry infra symlinks.

## Phase 1: Sprint redesign

Strip lifecycle ceremony (review, doc updates, merge) from sprint. Sprint
becomes "interactive direct coding session" — when full ceremony is needed,
invoke `/implement` or delegate to an implementer. Clarify the boundary:

- Sprint: interactive, main agent works directly, lightweight
- Marathon: interactive, main agent delegates, team orchestration
- Implement: one-shot, full lifecycle

Decision needed: does sprint keep any delegation capability, or is it
purely direct-execution? Current skill says "default direct, delegate
when user directs" — is that the right boundary?

## Phase 2: Delegation spawn pattern

The core primitive the user wants: an implementer-reviewer pair that
communicates directly, with the lead receiving only the final report.

Two options:
- **B: Thin skill** (`/delegate` or `/delegated-implement`) — 20 lines,
  invariants + spawn template. "Discuss directly, delegate mechanical
  work via infra/agents/."
- **C: Infra pattern only** — `infra/patterns/impl-review-pair.md` or
  similar, usable from any session without a skill. CLAUDE.md one-liner
  makes it discoverable.

Decision needed: B vs C. Key question: does the pattern need enough
framing to justify a skill, or is it simple enough that any agent can
use it from a template?

## Phase 3: Agent content decolonization

Agent files in `infra/agents/` still reference marathon-flavored
conventions (e.g., `_common.md` mentions marathon branching). Review
each file and generalize language so they work cleanly from any
calling context (sprint, implement, bare session, future skills).

Check if `marathon/ask.sh` should also move to `infra/` or remain
marathon-specific.

## Phase 4: Marathon simplification

With agents externalized and spawn patterns shared, marathon should
shrink. Evaluate whether marathon's orchestration overhead (bootstrap.sh,
TeamCreate, sub-branches, merge ceremony) is still justified or can be
simplified.

Related: `260405-research-marathon-delegation-hardening` covers
marathon-specific lead discipline issues.
