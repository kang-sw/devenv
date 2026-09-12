---
title: "Close the release-sweep ready-category bypass and stale Landing Lens findings"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 136c067fd8c72513
sage-review-completeness-reviewed: 136c067fd8c72513
completed: 2026-09-12
---

# Close the release-sweep ready-category bypass and stale Landing Lens findings

## Background

The cumulative review of
`15f212c7df8f8eb3d77cf4212d4af74c8ca4094c..6c4660cdf103903eb3935c7af82e28173ece4d68`
found two Important issues. First, `tickets.move` rejects non-implementation
categories at the `ready/` boundary, but direct `tickets.create_empty` creation
still accepts a research ticket with `initial_state: "ready"`. The existing
success test proves that this path can place a board artifact into the
implementation queue. Second, the ignored project-local
`ai-docs/_review.local.md` Landing Lens still requires updates to the retired
spec and mental-model layers, contradicting the current repository authority.

## Decisions

- Direct ready creation rejects `epic`, `research`, and legacy `workset`
  categories under the same board-artifact rule as `tickets.move`.
- Rejection occurs without creating or modifying a ticket file. Ordinary
  `idea/` and `todo/` creation behavior remains unchanged.
- Replace the direct-ready success regression with rejection and no-write
  coverage for the barred categories.
- Rewrite the local Landing Lens to require meaningful tests for
  caller-visible behavior changes and compliance with applicable prescriptive
  conventions. It must not refer to spec or mental-model layers.

## Constraints

- Keep discovery-query defaults and Sage-review category behavior unchanged.
- `ai-docs/_review.local.md` remains project-local and ignored; do not force-add
  it to Git.
- Do not rewrite historical commits to repair the separately adjudicated
  `f268ffd2` commit-message heading omission.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsdoc/ticket_create.go, agents-plugin-tool/internal/wsdoc/ticket_create_test.go, ai-docs/_review.local.md |
| scope.surface | public-interface | tickets.create_empty changes its externally callable ready-creation behavior in agents-plugin-tool/internal/mcp/server.go#L1272-L1294 |
| scope.new_public_symbol | no | existing tickets.create_empty tool and TicketCreate function are changed; no new symbol is specified |
| scope.new_type_contract | no | TicketCreateOptions already carries InitialState in agents-plugin-tool/internal/wsdoc/ticket_create.go#L11-L16 |
| scope.test_surface | existing | agents-plugin-tool/internal/wsdoc/ticket_create_test.go#L170-L190 and agents-plugin-tool/internal/wsdoc/workset_retirement_test.go#L10-L33 |
| complexity.reuse_points | confirmed | nonImplementationCategories already defines epic, research, and workset in agents-plugin-tool/internal/wsdoc/tickets_mutate.go#L310-L321 |
| complexity.side_effect_risk | moderate | TicketCreate creates the destination directory and writes the stub after its preconditions (agents-plugin-tool/internal/wsdoc/ticket_create.go#L97-L112) |
| risk.correctness | high | accepting a board artifact in ready can place it in the implementation queue |
| risk.fit | moderate | direct creation must match the established ready-boundary category rule |
| risk.test | moderate | regression coverage must assert rejection and no filesystem write for each intended category |
| risk.security_or_contract | high | tickets.create_empty is an MCP-facing contract whose ready-state acceptance changes |

## Phases

### Phase 1: Apply and verify the two release-sweep fixes

Close both Important findings with focused regression coverage, run the full Go
suite and the existing plugin verification suites, and independently review the
result against the decisions above.

### Result (fd38f30c) - 2026-09-12

Direct ready creation now rejects epic, research, and legacy workset categories
through the shared board-artifact category set before filesystem access. Tests
assert the ready-specific error, absence of new directories/files, and preservation
of existing ticket content. Workset ready rejection precedes its retired-authoring
check; idea/todo retirement behavior remains unchanged. Updated stale comments
that described the move guard as the only ready boundary.

The ignored local Landing Lens now requires meaningful caller-visible behavior
tests and compliance with applicable prescriptive conventions. It remains outside
Git as requested.

Verification: `TMPDIR=/private/tmp go test ./...` passed after the final source
change; `scripts/smoke-ws-mcp.sh ..` passed; Python unittest discovery passed
58 ws tests and 11 wsflow tests. `git diff --check` passed. Canonical TMPDIR
avoids the known macOS temporary-directory alias issue. No dedicated ticket-doc
test exists; closure uses the ticket lifecycle tool and diff checks.

Independent correctness and fit reviews were clean. Test review found that the
initial workset assertion exercised only its prior retirement guard; the shared
ready guard was moved earlier and error assertions were strengthened. The focused
second review confirmed the Important finding resolved. No unresolved findings.
