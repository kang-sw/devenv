---
title: "spec-collocator — fresh-subagent spec-impact detection so the lead need not re-dig specs"
related:
  260723-epic-ticket-write-reshape: motivating epic; unblocks the hard spec-address gate child
  260723-feat-ready-spec-address-hard-gate: this mechanism is the prerequisite that makes that hard gate ergonomically satisfiable
---

# spec-collocator — fresh-subagent spec-impact detection so the lead need not re-dig specs

## Background

Promoting the ready spec-address gate from soft-warn to hard
(`260723-feat-ready-spec-address-hard-gate`) creates an ergonomics problem: the
gate blocks a ready move until the ticket addresses its spec impact, but the
**lead should not re-dig the spec tree itself** to discover which anchors a
ticket touches. Doing so is context-heavy and pulls spec bodies into the lead's
window — exactly the front-loading the reshape epic is trying to avoid.

This ticket researches a **spec-collocator**: a fresh (stateless) subagent that,
given a ticket path or phase description, detects spec impact and returns a
compact structured answer, so the lead only adjudicates a proposal rather than
performing the survey.

## Motivating incident

A real spec-stale issue occurred previously where a ready-promoted change did not
carry accurate spec addressing. The hard gate is the enforcement response; the
collocator is what makes that enforcement livable.

## Open questions

- **Interface contract.** Input: ticket path + phase(s), or a diff. Output shape:
  candidate affected `spec:` anchors (with confidence + why), a "new spec needed"
  signal, or "no caller-visible behavior." What exactly does the lead receive to
  adjudicate?
- **Relation to existing tools.** How does it compose with `ws/specs.find`,
  `specs.status`, and `references.trace`? Is the collocator a thin orchestration
  over those, or does it read spec bodies the lead shouldn't?
- **Where it plugs in.** Does the hard spec-address gate call it automatically on
  a blocked ready move, or does the lead invoke it on demand? Return-prose vs
  structured verdict.
- **Fresh vs context-holder.** Under the fork-free delegation model
  (`260723-refactor-fork-removal-prefer-subagent`) the collocator is a clean
  fresh spawn — it runs from the ticket artifact, not inherited conversation
  context. Confirm no context-dependency sneaks in.
- **Trust boundary.** The lead must still confirm the collocator's proposal
  before it satisfies a hard gate; the collocator informs, it does not authorize.
- **Cost.** Is a per-ready-move subagent spawn acceptable overhead, or should it
  be cached/opt-in?
