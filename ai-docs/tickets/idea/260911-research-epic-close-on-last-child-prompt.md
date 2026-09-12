---
title: "Research a reliable epic-close procedure: detect when an epic's last child is done and prompt the user to close the parent"
related:
  260911-refactor-epic-ready-exception-reduction: prerequisite-context; that refactor makes epics soft living boards that never auto-close and lands a minimal lead-run terminal nudge as an interim guard — this ticket researches the robust replacement
---

# Research a reliable epic-close procedure

## Background

Under the soft-epic model (`260911-refactor-epic-ready-exception-reduction`) an
epic is a living board that never becomes an execution target and is never
auto-closed. The existing epic close condition is stale, and nothing reliably
tells the lead or the user "this epic's work is finished — close it?" That
refactor lands one interim line at the `lead-run` terminal (surface a
fully-childed epic for a close decision), but the terminal only fires on a drain
cycle and only sees what that cycle drained. An epic whose last child closed
outside a `lead-run` drain — a direct `run`, a `delegate`, a manual close — would
still float. This ticket researches where and how to detect epic completion
reliably and what the close prompt should be.

## Trigger placement

Where should "an epic's last open child just reached `.done/`" be detected, so
the signal is not missed regardless of how the child closed? Candidates to weigh:

- **At ticket close (`tickets.close` / the worker's close path).** Closest to the
  real event: when any ticket moves to `.done/`, check whether it has a `parent:`
  epic and whether that epic now has no open children, and emit a tip. Catches
  every close path, not just `lead-run`. Cost: child enumeration on every close.
- **At the `lead-run` terminal (the interim spot).** Cheap and already surfaces
  to the user, but only fires on a drain cycle and only for epics the cycle
  touched.
- **As a `tickets.verify` advisory.** A periodic "these epics look complete"
  sweep, decoupled from any single close; runs when the lead already verifies.
- **A dedicated read tool / query.** An on-demand "which epics are fully childed"
  query the lead can run, no automatic trigger.

Resolve which one (or which pair — an eager tip plus a sweep backstop) best
balances reliability against per-close cost, and whether the detection belongs in
Go (deterministic child enumeration) or in lead prose.

## Child enumeration and completion definition

What exactly counts as "done" for an epic? Its direct `parent:`-linked children
all in `.done/`? Does a `.dropped/` child count as resolved? How are multi-level
epics (an epic whose child is itself an epic) handled — does the leaf epic close
first and bubble up? Define the completion predicate precisely, and confirm the
child graph is cheap to compute from stems + frontmatter without a full index.

## Prompt shape and authority

The close itself stays a user decision (consistent with the soft-epic,
judgment-over-rules posture). What does the lead surface, and when — a tip in the
next natural status, an Open Decision Queue item, or an explicit prompt? The lead
should also be able to note "not yet — more children coming" without being
re-nagged every cycle. Consider a lightweight "acknowledged, keep open" marker so
a deliberately-open completed-looking epic does not resurface indefinitely.

## Relationship to the interim guard

Specify how the robust mechanism supersedes or subsumes the minimal `lead-run`
terminal nudge from the refactor: does the terminal line get removed, kept as one
of several trigger sites, or folded into the shared detection helper? The
research should land with a concrete recommendation the follow-up implementation
ticket can execute.
