---
title: "Ticket-write reshape — verify-commit gate, must-not-forget tool collapse, concept-doc consolidation"
related:
  260605-research-ws-native-subagent-pivot: MCP tools should not own model-spawn orchestration; this epic keeps mutation labor in thin MCP tools but moves validation to a deterministic gate
  260630-epic-skill-playbook-diet: extends the diet's Lever B (MCP-ification) but partially inverts it — collapses mutation validation into one gate instead of adding tools
  260627-bug-write-ticket-bypasses-tickets-create: prose-only fix left zero mechanical backstop against direct-file-edit bypass; the verify-commit gate closes exactly that hole
  260702-research-destructive-dedup-methodology: guardrail-vs-restatement discipline governs what the concept doc may absorb vs what must stay a mechanical check
sage-review-design: completed
---

# Ticket-write reshape — verify-commit gate, must-not-forget tool collapse, concept-doc consolidation

## Scope

Reshape the ticket-write path around a single organizing principle: **each action's
follow-on procedure is enforced in proportion to how catastrophic forgetting it is.**

Deliverables (child tickets):

1. **Verify + commit-gate mechanical backstop.** A deterministic `ticket.verify`
   check hosted at `git.commit`'s existing validation slot, run before every
   commit that touches a ticket file. It owns all mechanical guardrails (stem
   format, status/dir consistency, frontmatter integrity, sage-posture
   presence/value, spec-address presence, phase/Result structure). It is the
   first mechanical chokepoint for every commit routed through `ws/git.commit`,
   and it can also be called voluntarily mid-edit for red-green feedback.
   **Caveat (open):** hosting the gate inside `ws/git.commit` does not cover a
   raw `git commit` issued from the shell after a hand-edit; a *truly* universal
   chokepoint needs either a git pre-commit hook or a workflow mandate that all
   ticket-touching commits route through `ws/git.commit`. The verify-gate child
   must resolve which before leaning on the "every path" premise.
2. **Mutation-tool collapse by the must-not-forget filter.** Keep thin tools only
   where they bundle a *catastrophic-to-forget* follow-on; free the rest to
   agent free-edit under the verify floor. Surface each action's must-know
   follow-on as tool/gate return prose at action time instead of front-loading
   all procedure into the playbook.
3. **Ticket-system concept doc.** One layered-explanation document (what
   idea/todo/ready mean, feat/bug/refactor/chore distinctions, what sage review
   is and why) loaded once per session; the playbook references it instead of
   re-glossing concepts across convention + judges + Go constants.

## Non-Scope

- **Delegation/fork reshape** is a sibling standalone, not a child of this epic:
  `260723-refactor-fork-removal-prefer-subagent`.
- **spec-collocator internals** beyond the interface the hard spec-address gate
  needs — owned by the research child `260723-research-spec-collocator-subagent`.
- No change to the *semantic* sage-review judgment itself (design/completeness
  reviewer criteria); only where its result stamp is written and checked.

## Child Tickets

- `260723-feat-ticket-write-verify-commit-gate` - verify() + commit-gate backstop, then must-not-forget tool collapse (2 phases)
- `260723-feat-ticket-system-concept-doc` - single session-loaded concept doc; playbook references not re-glosses
- `260723-feat-ready-spec-address-hard-gate` - promote ready spec-address from soft-warn to hard; **blocked on** the collocator child (idea/)
- `260723-research-spec-collocator-subagent` - fresh-subagent mechanism that detects spec impact so the lead need not re-dig specs (idea/)

## Cross-Child Decisions

- **verify = mechanical floor, sage = semantic ceiling.** verify() checks only
  file-state-deterministic guardrails and confirms the sage *result* was
  persisted; it never judges prose quality, faithfulness, or design soundness —
  those stay with the sage reviewers.
- **sage stamp is lead single-writer.** Design/completeness reviewers run
  concurrently and return verdict prose only; they never write frontmatter
  (avoids a concurrent-write race). The lead reads the verdicts, judges, and
  writes the `completed`/`blocked` posture via a thin lead-only `sage.stamp`
  tool that also renders the `## Blocked` companion.
- **Hard spec-address gate is blocked on the collocator.** Promoting ready
  spec-address to hard must not land before `260723-research-spec-collocator-subagent`
  delivers an ergonomic path, or the gate blocks the lead with no way to satisfy it.
- **The concept doc carries concepts, never guardrails.** Any invariant that
  verify() can mechanically enforce stays in verify; the doc must not dissolve a
  guardrail into soft prose (per 260702's guardrail-vs-restatement caution).
- **Existing hard/soft choices are the seed classification.** ready→sage is
  already hard (`tickets_mutate.go`); close→phases-resolved is already soft
  (`UnresolvedPhases`). The reshape makes this the explicit design axis.

## Completion Criteria

- Done: verify-commit gate lands and owns the mechanical guardrail set; the
  mutation-tool surface is collapsed per the filter with action-time obligation
  prose; the concept doc exists and the playbook references it. The hard
  spec-address gate may remain deferred behind the collocator without blocking
  epic closure.
- Dropped: if the verify-commit-gate backstop proves infeasible at the
  `git.commit` layer and no equivalent chokepoint exists.
- Deferred: hard spec-address gate + spec-collocator, until the collocator is
  designed.
