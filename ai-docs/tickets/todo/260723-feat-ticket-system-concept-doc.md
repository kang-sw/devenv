---
title: "Single session-loaded ticket-system concept doc; playbook references instead of re-glossing"
related:
  260624-feat-tickets-template-tool-and-convention-diet: precedent — moved type templates out of the convention doc; this addresses the conceptual gap that offloading left
  260702-research-destructive-dedup-methodology: guardrail-vs-restatement discipline bounds what the concept doc may absorb
  260723-feat-ticket-write-verify-commit-gate: sibling — once verify owns mechanical guardrails, the doc carries only concepts
parent: 260723-epic-ticket-write-reshape
sage-review-design: required
sage-review-completeness: required
---

# Single session-loaded ticket-system concept doc; playbook references instead of re-glossing

## Background

Audit of the ticket-write chain found the concepts are **procedural and
scattered**, not explained once:

- Status dirs (`idea/`/`todo/`/`ready/`), spec addressing, phases, and
  epic/workset semantics are each explained 2-3 times across
  `ticket-conventions.md`, `lead-write-ticket.md` judge tables, and Go constants —
  in different vocabularies (declarative rule vs. judge-decision criterion vs.
  bare mechanical call).
- **Type prefixes** (`feat`/`bug`/`refactor`/`chore`) are never semantically
  distinguished anywhere — just listed flatly.
- **Sage review** has zero conceptual explanation in any doc; its rationale and
  posture semantics live only as Go code comments, invisible to a reading agent.

This is the residue of the token-saving diet: pushing text into Go tools reduced
per-invocation tokens but evaporated or duplicated the conceptual home. Each
decision point re-derives concept meaning, which is itself part of the weight.

## Decisions

- **One layered-explanation concept doc** covering: what each status dir means and
  when to use it; the semantic distinction between type prefixes; what sage review
  is, why it exists, and what its postures mean; what spec addressing is for; the
  phase model; epic vs. workset.
- **Session-level grounding, not per-invocation.** The doc is loaded once (session
  bootstrap / on demand), like AGENTS.md — not re-loaded on every write-ticket
  call. This lets the playbook shed its inline re-glosses and become terser.
- **Concepts only, never guardrails.** Any invariant that `ticket.verify`
  (`260723-feat-ticket-write-verify-commit-gate`) can mechanically enforce stays in
  verify; the doc must not dissolve a hard guardrail into soft prose the model may
  not honor (per `260702`'s guardrail-vs-restatement caution).
- The playbook and convention doc **reference** the concept doc for meaning and
  keep only the mechanical call sequence + hard invariants.

## Phases

### Phase 1: Author the concept doc and de-duplicate the glosses

Write the single concept doc. Then strip the duplicated conceptual glosses from
`ticket-conventions.md` and `lead-write-ticket.md`'s judge tables, replacing them
with a reference to the concept doc — removing only *restatements*, never a
guardrail verify() does not yet own. Confirm net token reduction on the
write-ticket path and that no hard invariant was softened in the move.

## Spec Impact

- Target spec area: `documentation-system` (introduces the ticket-system concept
  doc as a session-loaded grounding artifact and its relationship to the
  convention/playbook chain).
- Expected caller-visible change: ticket-system concepts are explained once in a
  referenced doc; convention + playbook stop re-glossing.
- Contract-first spec: no — the doc's content is authored during implementation;
  the spec entry is a closeout describing the new grounding artifact's role.
