---
title: "lead-run stop (c) requires an Edition even when the executed phase has no Result"
related:
  260911-feat-impl-derivation-hardening-branch-aware-select: dogfood incident; worker stopped before edits because Phase 3's pre-Select contract was structurally impossible
---

# lead-run stop (c) requires an Edition even when the executed phase has no Result

## Background

During `ws:lead-run` dogfooding, a ticket worker raised stop (c) before making
any source edit or writing a phase Result. The lead-run playbook nevertheless
requires the proposed resolution to be recorded as an `#### Edition` on the
executed phase. Ticket conventions allow an Edition only for a later pass under
an already completed phase's Result; an unimplemented phase remains editable
directly. Following the stop instruction literally would create an invalid
ticket shape, while following the convention would violate the lead-run
recovery procedure.

## Phases

### Phase 1: Align stop-(c) recovery with phase lifecycle

Make the recovery instruction distinguish an unimplemented phase from a
completed phase, preserve the raised design-review gate in both cases, and add
coverage for a stop raised before the first Result exists.
