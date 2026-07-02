---
title: tickets_move to ready bypasses spec-address gate with no warning at the primitive layer
sage-review: completed
---

# tickets_move to ready bypasses spec-address gate with no warning at the primitive layer

## Context

Found during a v0.31.1 dogfooding pass. Per ticket conventions, promoting a
ticket into `ready/` should have spec addressing (`spec:`, `spec-remove:`, or
a `## Spec Impact` section) except for `epic`/`research`/`workset` categories.
That gate is documented and enforced only in the `lead-write-ticket` playbook
layer. Calling the `tickets_move` MCP primitive directly to move a `chore`
ticket to `ready/` succeeded silently with no spec addressing and no warning
of any kind.

The layer separation itself (playbook owns policy, primitive stays
mechanical) is understood to be intentional and is not being questioned here.
The gap is that a lead who calls the primitive directly — bypassing the
playbook, whether intentionally or by habit — gets no signal that it is
violating the documented convention.

## Suggestion

Add a soft, non-blocking warning to `tickets_move`'s response when moving a
non-epic/research/workset ticket to `ready/` without detected spec addressing,
e.g.: "ready gate is normally enforced by lead-write-ticket; no spec
addressing detected." The move should still succeed — this is advisory, not a
new hard gate at the primitive layer.

## Spec Impact

Target: `ai-docs/spec/mcp-tools.md`. Caller-visible change: `tickets_move` to
`ready` emits a soft (non-blocking) warning when no spec addressing is
detected, noting the gate is normally enforced by `lead-write-ticket`.
Contract-first spec: no.
