---
title: Remove the orphaned review-adjudicator delegate once the graded relay lands
related:
  260831-refactor-severity-graded-per-slice-review-relay: prerequisite — that ticket revives implementer-elevated but leaves review-adjudicator with no reachable trigger, making it the deletion candidate
  260828-refactor-per-slice-review-relay: first made both delegates dormant; retained them as an explicit deletion-is-later decision
---

# Remove the orphaned review-adjudicator delegate once the graded relay lands

## Background

`260828` (commit `4575f634`) made both the `review-adjudicator` and
`implementer-elevated` delegate playbooks dormant (retained, not invoked),
noting that deleting functionality is an AGENTS.md "Always ask" decision left to
a later ticket. `260831-refactor-severity-graded-per-slice-review-relay` revives
`implementer-elevated` (as the Critical-ceiling escalation) but deliberately does
**not** restore `review-adjudicator` — the cycle-counting/re-review routing it
provided is now lead-owned in the shared clause. That leaves
`review-adjudicator` with no reachable trigger anywhere: a genuine orphan.

## Scope

- After `260831` lands, confirm `review-adjudicator` is fully unreferenced (no
  playbook prose, no `session_state.go` routing, no test golden that requires it
  beyond negative/forbidden pins). Search `review-adjudicator` across
  `agents-plugin/`, `agents-plugin-tool/`, `agents-plugin-wsflow/`, and
  `ai-docs/`.
- If fully orphaned, delete the `review-adjudicator` rsrc playbook and its
  manifest entry, regenerate the wsflow mirror + both `manifest.json` hashes,
  and prune the mental-model bullet (or mark it removed).
- Deleting a delegate is an AGENTS.md "Always ask" action — confirm with the
  user before the delete lands, even though the analysis is captured here.

## Constraints

- Depends on `260831`; do not delete before that lands, because the reachability
  argument rests on its graded-relay shape.
- Keep `implementer-elevated` — it is revived, not orphaned.

## Phases

### Phase 1: Confirm orphan status and delete review-adjudicator

Verify no reachable reference remains, remove the playbook + manifest entry,
regenerate mirrors/manifests, and update the mental-model. Verification: build/
vet/test green, wsflow tests pass, and a repo-wide `review-adjudicator` search
returns only historical/ticket references.
