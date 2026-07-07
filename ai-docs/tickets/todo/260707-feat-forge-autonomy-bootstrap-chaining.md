---
title: "Narrow forge-spec confirmation loop to auto-proceed; add bootstrap same-session forge chaining"
related:
  260707-feat-doc-coverage-live-bootstrap-alarm: complementary cross-session safety net for the same forgetting risk this ticket's Phase 2 addresses within a single session
sage-review: required
---

# Narrow forge-spec confirmation loop to auto-proceed; add bootstrap same-session forge chaining

## Background

`lead-forge-spec` and `lead-forge-mental-model` are long-running documentation
reconstruction skills. `lead-forge-mental-model` already auto-applies verifier
findings tagged `[HIGH]`/`[STALE]`/`[BLOAT]` without stopping
(`agents-plugin/rsrc/lead-forge-mental-model/lead-forge-mental-model.md:169-173`)
and only defers `[LOW]` findings to a final summary report (`:204`). It also
already has a "soft gate" precedent that warns and proceeds instead of
blocking (`:31-44`, `judge: spec-gate (soft)` at `:214-216`).

`lead-forge-spec` has three confirmation points instead:
- Archive/`git mv` confirmation (`agents-plugin/rsrc/lead-forge-spec/lead-forge-spec.md:14,
  35-36`) — destructive, gated as "Always ask" per the repo Approval Protocol
  (`AGENTS.md`).
- Domain-list confirmation, once per run (`:97-101`, invariant `:15`).
- Per-ambiguous-item classification confirmation inside the "User
  classification loop" (`:178-186`, invariant `:15`: "No spec entry is written
  without user confirmation of caller-visible status and implemented/planned
  classification") — this can fire repeatedly across a large survey and is
  the actual source of user-perceived "yes spam."

Separately, `lead-bootstrap` only suggests running `lead-forge-spec` /
`lead-forge-mental-model` once, on the fresh-install path only
(`agents-plugin/rsrc/lead-bootstrap/lead-bootstrap.md:46`). It never chains
into them, and there is a real risk that a user who completes `lead-forge-spec`
in a long session forgets to also run `lead-forge-mental-model` before ending
the session.

## Decisions

- Only the **per-ambiguous-item classification loop** in `lead-forge-spec`
  converts to auto-proceed. The skill classifies ambiguous items using its
  own best judgment, writes an inline marker in the spec entry noting the
  ambiguity (mirroring `lead-forge-spec`'s existing placeholder-note pattern
  at `:196`), and collects all such markers into the final summary report
  instead of stopping per item.
- The archive/`git mv` confirmation gate is **not** touched — it is a
  destructive operation and stays "Always ask" per the repo Approval
  Protocol. Full auto-proceed across all three gates was considered and
  rejected: it would blur a genuinely destructive-action gate with a
  productivity-costing repetitive gate that has a much lower blast radius.
- The domain-list confirmation (both skills, once per run) is **not**
  touched — it is low-frequency and high-leverage (a wrong domain list
  invalidates the whole survey), so it is not "yes spam" and removing it
  would trade a cheap check for expensive rework risk.
- `lead-forge-mental-model`'s existing auto-apply/soft-gate behavior is
  reference precedent for this change, not itself modified by this ticket.
- `lead-bootstrap` gains a same-session chaining step: immediately after
  `lead-forge-spec` completes (in a session where bootstrap or the user
  initiated it), autonomously ask whether to run `lead-forge-mental-model`
  next, and invoke it on a yes answer. This only covers the same-session
  case; the cross-session "forgot entirely, or session ended in between"
  case is handled separately by
  `260707-feat-doc-coverage-live-bootstrap-alarm`.

## Phases

### Phase 1: Narrow lead-forge-spec's per-item confirmation to auto-proceed

Rewrite the "User classification loop"
(`agents-plugin/rsrc/lead-forge-spec/lead-forge-spec.md:178-186`) and its
invariant (`:15`) so caller-visible/implemented-or-planned classification for
ambiguous items is decided autonomously, with an inline marker written into
the spec entry for each ambiguous classification, and all markers surfaced in
the final summary report. Leave the archive gate (`:14, 35-36`) and
domain-list confirmation (`:97-101`, invariant `:15`) untouched. Update the
skill doctrine section if it references the removed per-item gate.

### Phase 2: lead-bootstrap same-session forge-spec → forge-mental-model chaining

Extend `lead-bootstrap` so that when it runs or triggers `lead-forge-spec` in
a session (not limited to the fresh-install path — cover upgrade/adopt paths
too, consistent with the existing index-health-check routing table at
`agents-plugin/rsrc/lead-bootstrap/lead-bootstrap.md:86-96`), it asks after
`lead-forge-spec` completes whether to also run `lead-forge-mental-model`,
and invokes it on a yes answer.

## Spec Impact

Observable workflow-behavior change to `lead-forge-spec` and `lead-bootstrap`
(removes a documented confirmation invariant, adds an autonomous chaining
step) — needs spec addressing before `ready/` promotion. Likely area:
`ai-docs/spec/workflow-skills.md`. Not addressed yet; left for the
implementation-survey pass that promotes this ticket.
