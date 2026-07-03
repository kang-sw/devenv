---
title: tickets_move to ready bypasses spec-address gate with no warning at the primitive layer
sage-review: completed
completed: 2026-07-02
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

## Result - 2026-07-02

Implemented in `agents-plugin-tool/internal/wsdoc/tickets_mutate.go`:

- `readyGateWarning` derives the ticket category from the stem
  (`YYMMDD-<category>-<slug>`) and skips the check for `epic`, `research`, and
  `workset` categories, matching the `lead-write-ticket` exemption list.
- Spec addressing is detected the same way the playbook layer defines it: a
  confirmed `spec:` or `spec-remove:` frontmatter entry, or a `## Spec Impact`
  heading anywhere in the ticket body.
- When `tickets.move` promotes a non-exempt ticket to `ready/` with neither
  signal present, `TicketMutateResult.Tip` gets the advisory line: "ready gate
  is normally enforced by lead-write-ticket; no spec addressing detected." The
  move still succeeds — no new hard gate was added at the primitive layer.
- Added `appendTip` so this warning composes with the existing sage-review
  posture tip on the same upward-to-ready move instead of overwriting it.
- Updated `ai-docs/spec/mcp-tools.md` (`{#260620-ticket-move-tool}`) to
  document the new advisory behavior.

Verification: added focused unit tests in
`agents-plugin-tool/internal/wsdoc/tickets_mutate_test.go` covering the
warning-present case, the still-succeeds case, each spec-addressing signal
(`spec:`, `spec-remove:`, `## Spec Impact`) suppressing the warning, all three
exempt categories suppressing the warning, and the warning composing with the
sage-review tip. Ran `go test ./... -count=1` from `agents-plugin-tool/` —
all 12 packages passed (`cmd/ws-mcp`, `internal/execjob`, `internal/mcp`,
`internal/textreader`, `internal/wsagent`, `internal/wsconfig`,
`internal/wsdoc`, `internal/wsgit`, `internal/wskey`, `internal/wsrsrc`,
`internal/wsstate`, `internal/wsstore`).
