---
title: "Reconcile lead-review fix handoff with ticket-only lead-run"
---

# Reconcile lead-review fix handoff with ticket-only lead-run

## Background

The `lead-review` NEEDS FIX procedure tells the lead to generate a review
artifact and invoke `lead-run` with that artifact as the implementation
contract. The current `lead-run` procedure is ticket-only: it point-resolves a
ticket stem, requires ticket Route Facts, and rejects `completion: ad_hoc`.
During the 2026-09-12 release sweep this made the documented direct handoff
unexecutable and required an additional ticket-authoring pass.

## Phases

### Phase 1: Reconcile the review-to-fix handoff contract

Determine whether review findings should be captured as an actionable ticket
before `lead-run` or whether `lead-run` should regain an explicit reviewed
artifact route, then align the two playbooks and their contract coverage.
