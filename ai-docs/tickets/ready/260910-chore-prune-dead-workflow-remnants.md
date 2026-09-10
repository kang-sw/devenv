---
title: "Remove the stale tickets.move tip and unread Sage completeness config"
related:
  260726-feat-enter-verdict-scenario-output: owns the active doc-mode output contract; its fields remain untouched here
  260626-bug-sage-review-config-setter-missing: overlap; both touch sage_review_completeness (item 2), no hard conflict
  260910-refactor-ready-only-actionable-ticket-gates: owns the category-aware todo/ready fact and Sage boundary; supersedes former item 4
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 07f7f634b28026d4
sage-review-completeness-reviewed: 07f7f634b28026d4
---

# Remove the stale tickets.move tip and unread Sage completeness config

## Background

A source audit of the run/worker workflow (before the `epic/refound` ->
`develop` merge) surfaced two unconditional remnants left by earlier tracks:
stale schema text and an unread config item. Neither causes an error today, but
both misdescribe the live workflow surface.

1. **Stale `tickets.move` schema tip.** The MCP tool schema description
   (`agents-plugin-tool/internal/mcp/server.go#L3439`) still reads "Downward
   moves from ready/ return a spec-cleanup tip." The spec-cleanup tip was
   deleted with the ready spec-gate in `91621687`; `TicketsMove`
   (`agents-plugin-tool/internal/wsdoc/tickets_mutate.go#L266-L282`) now emits
   sage-review-posture tips on upward moves and missing-route-facts and
   ready-posture tips on `ready/` moves. The string is the only `spec-cleanup`
   hit in plugin source (`rg -n -F 'spec-cleanup' agents-plugin-tool`) and is
   visible to any caller reading `tools/list`.

2. **Unread `sage_review_completeness` config item.** Declared and registered
   (`agents-plugin-tool/internal/wsconfig/scope.go:37-40,77`) with a doc comment claiming it "controls
   whether the completeness reviewer runs alongside the design reviewer," but it
   is never read by the gate logic — completeness applicability is decided
   solely by category in `sageReviewStageRequirement`
   (`agents-plugin-tool/internal/wsdoc/tickets_mutate.go:381-394`). Remove the
   dead item and its declaration/registration.

The audit also found the unused general `landing: "todo"` Sage path. That is no
longer cleanup scope: `260910-refactor-ready-only-actionable-ticket-gates` owns
the confirmed protocol decision to reject actionable todo review while keeping
todo landing for explicit epic design settlement.

The surveyed `doc_mode` / `doc_reason` / `need_doc` values are active
`enter.implement` output derived from policy, not dead fields. Their contract
remains untouched here and with
`260726-feat-enter-verdict-scenario-output`, which plans to use `doc_mode` as a
live scenario axis.

## Decisions

- **Items 1-2 are pure dead-code / stale-metadata removals** (auto-proceed
  class): a stale schema string and an unread config key. Removing them cannot
  change Sage gate behavior, but removing the registered key removes it from
  `config.list` output (`agents-plugin-tool/internal/wsconfig/scoped_show.go#L59-L63`).
- **The doc-mode fields remain.** They are an active output contract and their
  related scenario ticket owns future use; this cleanup does not remove or
  revise them.
- **The former item 4 is transferred.** The related ready-only-gates ticket owns
  its protocol change and verification; this cleanup does not edit the todo
  Sage path.
- The cleanup lands only items 1-2.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for
  agents-plugin-tool/, agents-plugin/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for
  agents-plugin-tool/internal/mcp/)

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/mcp/server.go, agents-plugin-tool/internal/wsconfig/scope.go, and agents-plugin-tool/internal/wsdoc/tickets_mutate.go |
| scope.surface | public-interface | tools/list-visible MCP schema description text changes; the unread config item is removed |
| scope.new_public_symbol | no | none |
| scope.new_type_contract | no | removes no type or signature; the deleted config item is an existing registered value |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/server_test.go and internal/wsconfig/scope_test.go cover the MCP tools/list and config-scope surfaces |
| complexity.reuse_points | not-applicable | removes stale description text and one unread config item; no existing component is being reused |
| complexity.side_effect_risk | moderate | removing the registered config key changes config.list output while leaving Sage category logic unchanged |
| risk.correctness | moderate | source searches identify the schema text as stale and the config item as unread, while registration still exposes the key |
| risk.fit | low | the change removes only surfaces that no live decision path consumes |
| risk.test | low | existing schema/config coverage can pin the removals |
| risk.security_or_contract | moderate | the schema description is corrected and config.list stops reporting a registered key |

## Phases

### Phase 1: Remove the two unconditional remnants

Delete the stale `spec-cleanup` tip string (item 1); remove the unread
`sage_review_completeness` config item and its registration (item 2). Update
pinned tests accordingly and run `go build ./...`, `go vet ./...`, and
`go test ./...` to confirm no dangling references remain.
