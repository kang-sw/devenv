---
title: Opaque route params do not reach deterministic handlers
related:
  260904-refactor-enter-affordance-rename-route-opaque: introduced the opaque published schema
plans:
  phase-1: 2026-09/06-1600-route-opaque-params-handler-mismatch
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 3e4ea2908bd5cf3f
sage-review-completeness-reviewed: 3e4ea2908bd5cf3f
completed: 2026-09-06
---

# Opaque route params do not reach deterministic handlers

## Background

Downstream reports repeated inability to call `route.resolve_proceed` through
the ws 0.45.0 advertised `session_key` plus opaque `params` schema. The patch
`ada0ba19` changed the published schema while intentionally leaving the parsers
unchanged. This capture records the observed regression, not an approved API
redesign.

## Evidence

Reproduced against the connected MCP server on 2026-09-06 with the review
session's `session_key` and this argument fragment:

```json
{"params":{"target":{"kind":"ticket-path","value":"probe"},"facts":{}}}
```

- `route.resolve_proceed` returns `route.resolve_proceed: target is required`.
  `parseProceedInput` reads top-level `args["target"]`; the handler does not
  unwrap `params` before parsing.
- `route.resolve_implement` returns `entered implement mode; todo list replaced
  (5 items)`. This is not a deterministic routing verdict: the typed path is
  selected only when top-level `target` exists, and wrapped input falls through
  to legacy mode entry. An agenda containing the original `params` therefore
  does not establish successful routing.
- The published schema and top-level parser inputs are tested separately;
  their integration through a wrapped `params` call is not covered by those
  assertions.

The downstream additionally reports that Claude Code sends undeclared
top-level objects as strings, producing `target must be an object`. That host
conversion was not independently reproduced here. The downstream's bypass to
`lead-implement` is an incident workaround, not a confirmed equivalent route.

## Spec Impact

Update the opaque route input contracts in `ai-docs/spec/mcp-tools.md` and
`ai-docs/spec/workflow-skills.md`: public calls carry `session_key` at the
envelope and routing fields inside `params`. Both resolvers must validate and
route that payload and persist the resulting deterministic agenda/todos.
Update the shared skill Fact Contracts and their generated distribution copies
to match. No new routing policy or tool rename is in scope.

## Phases

### Phase 1: Reconcile the route input contract

Make the advertised opaque `params` payload reach the existing typed parsers
for both tools. Keep `session_key` outside `params`; put `target`, `facts`,
optional policy, and format inside it as applicable. A wrapped implement call
must never silently select legacy mode entry. Preserve existing unwrapped
call behavior as compatibility rather than expanding this bug fix into removal
of functionality; document the wrapped shape as the canonical public call.
Reject malformed or ambiguous envelopes before mutating agenda or todos.
With `params` present, require an object and allow only `session_key` and
`params` at the outer envelope; reject an inner `session_key`. This makes
mixed typed or legacy outer fields unambiguous errors without precedence
rules. Wrapped input always uses typed routing, including validation of a
missing target. Without `params`, retain both existing top-level typed routing
and the existing implement legacy mode-entry path.

Align skill call examples and Fact Contracts, regenerate required resources,
and update the affected specs. Add regressions that call both tools with the
advertised wrapper and assert actual verdict, normalized agenda, and derived
todos, plus malformed wrapper and compatibility coverage. Run the relevant
MCP and resource/distribution checks. The prior schema-only interpretation in
260904-refactor-enter-affordance-rename-route-opaque is superseded by the user's
2026-09-06 authorization to repair this transport mismatch; routing decisions
themselves remain unchanged.

### Result (1653a118) - 2026-09-06

Both public wrappers now reach the existing typed resolvers. Envelope validation
retains outer-only session authentication and rejects non-object, mixed, and
nested-session-key inputs before agenda/todo writes. Wrapped implement calls
cannot silently select legacy mode entry; unwrapped typed and legacy calls
remain compatible. Implementation: `24a2392f`; spec reconciliation: `93b09c18`.

Independent correctness and test reviews found the same Important omission:
the executable call steps still used top-level arguments despite corrected
Fact Contracts. [fixed] in relay #1 (`1653a118`): both call steps now wrap
routing fields, rendered-playbook guards reject the old examples, and the
resource manifest and wsflow mirror were regenerated. Neither review found a
handler correctness defect. No Critical findings or unresolved dispositions;
Important closure uses implementer verification without an extra review round.

Verification on final implementation `1653a118`: full
`go test ./internal/mcp ./internal/wsrsrc -count=1` passed (55.734s/0.339s),
`go build ./...` passed, wrapper and rendered-playbook regressions passed,
and wsflow unittest discovery passed all 10 tests. `spec_index.verify` and
`git diff --check` passed. No pre-fix red test was run; the initial defect was
reproduced against the connected pre-fix MCP server. Final evidence is from
the source-built tests/build, not a refreshed downstream installed server.

The implementation branch is retained for review/merge; no merge, push,
version bump, or release was performed. No separate mental-model update was
needed because the repaired invariant is covered by the authoritative specs.
Documentation closeout compaction is skipped: the spec commit precedes a
source/test fix, and the remaining suffix is a single ticket-closeout commit.
