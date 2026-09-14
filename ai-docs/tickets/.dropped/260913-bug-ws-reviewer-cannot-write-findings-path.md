---
title: "Allow read-only code reviewers to persist required findings artifacts"
related:
  260911-feat-ws-pi-held-push-batch-delivery: discovered during partitioned review
dropped: 2026-09-13
---

# Allow read-only code reviewers to persist required findings artifacts

## Background

The bundled code-review playbooks require every reviewer to write its detailed report to the invocation's findings path, but a Pi reviewer spawned from those playbooks can receive only read-only repository tools and no tool capable of writing the generated cache path. During the held-push batch review, the reviewer returned the detailed findings inline and explicitly reported that the required artifact could not be written.

This mismatch weakens review persistence and makes a correct reviewer invocation impossible despite supplying a valid generated findings path.

## Phases

### Phase 1: Align reviewer authority with findings-path persistence

Determine the narrowest safe persistence mechanism for generated review artifacts, then update the reviewer dispatch/runtime contract so a read-only reviewer can write only its assigned findings path without gaining repository edit authority. Add a Pi integration regression proving a spawned reviewer can create the required report while remaining unable to modify the checkout.


## Resolution (2026-09-13)

Duplicate capture absorbed by `260912-bug-ws-pi-readonly-reviewer-artifact-contract`. Preserve this capture's assigned-findings-path containment, no-checkout-write regression, and held-push provenance in the canonical ticket.
