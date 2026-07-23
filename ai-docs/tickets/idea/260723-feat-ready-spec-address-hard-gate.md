---
title: "Promote ready spec-address from soft-warn to a hard gate"
related:
  260723-research-spec-collocator-subagent: blocker — the hard gate needs the collocator's ergonomic path before it can land
parent: 260723-epic-ticket-write-reshape
---

# Promote ready spec-address from soft-warn to a hard gate

## Background

Today the ready spec-address check is **soft**: `tickets_mutate.go` (~179-206)
emits a warning when a ticket moves to `ready/` without `spec:` / `spec-remove:` /
`## Spec Impact`, but does not block. A real spec-stale incident occurred where a
ready change carried inaccurate spec addressing. The reshape epic classifies
ready spec-address as a **catastrophic-to-forget** follow-on, so it belongs on
the hard side of the must-not-forget filter.

Under the verify-commit-gate model (`260723-feat-ticket-write-verify-commit-gate`)
this becomes a mechanical check at commit: a ready-landing ticket whose
caller-visible phase has no confirmed spec anchor / `spec-remove:` / `## Spec
Impact` is **rejected**, not warned.

## Blocker

**This ticket must not land before `260723-research-spec-collocator-subagent`.**
A hard gate with no ergonomic satisfaction path blocks the lead with nothing to
do but re-dig specs by hand — the exact cost the collocator removes. Sequence:
collocator first, then flip soft→hard.

## Phases

### Phase 1: Flip ready spec-address soft-warn to hard reject

Move the spec-address presence check from advisory warning to a hard gate in the
verify-commit path (or `tickets.move` upward path if the gate lands first).
Preserve the existing exemptions: `epic` / `research` / `workset` remain ungated
(`tickets_mutate.go` ~171-177). The check enforces **presence** of spec
addressing (mechanical); the judgment of whether the spec text *actually*
addresses the phase remains semantic and stays with the lead/collocator, not this
gate.
