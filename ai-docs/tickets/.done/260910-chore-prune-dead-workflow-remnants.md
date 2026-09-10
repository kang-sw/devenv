---
title: "Remove the stale tickets.move tip and unread Sage completeness config"
related:
  260726-feat-enter-verdict-scenario-output: owns the active doc-mode output contract; its fields remain untouched here
  260626-bug-sage-review-config-setter-missing: overlap; both touch sage_review_completeness (item 2), no hard conflict
  260910-refactor-ready-only-actionable-ticket-gates: owns the category-aware todo/ready fact and Sage boundary; supersedes former item 4
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: e0299e5b0789be35
sage-review-completeness-reviewed: e0299e5b0789be35
completed: 2026-09-10
---

# Remove the stale tickets.move tip and unread Sage completeness config

## Background

A source audit of the run/worker workflow (before the `epic/refound` ->
`develop` merge) surfaced a stale schema tip and an unread config item. The
schema tip has since been removed; the config item remains unread. Neither
causes an error today, but the config item still misdescribes the live workflow
surface.

1. **Removed `tickets.move` schema tip.** The MCP tool schema description now
   states that ready promotion and epic todo settlement resolve sage-review
   posture and actionable todo moves are ungated
   (`agents-plugin-tool/internal/mcp/server.go#L3439`). The former
   `spec-cleanup` tip was deleted with the ready spec-gate in `91621687` and
   remains absent from plugin source (`rg -n -F 'spec-cleanup'
   agents-plugin-tool` returned no results); the removal is visible to callers
   reading `tools/list`.

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
| scope.span | multi-file | agents-plugin-tool/internal/wsconfig/scope.go and agents-plugin-tool/internal/wsconfig/scope_test.go |
| scope.surface | public-interface | removing the registered config key changes config.list output |
| scope.new_public_symbol | no | none |
| scope.new_type_contract | no | removes no type or signature; the deleted config item is an existing registered value |
| scope.test_surface | existing | agents-plugin-tool/internal/wsconfig/scope_test.go covers registered default scopes and config-list resolution |
| complexity.reuse_points | not-applicable | removes one unread config item; no existing component is being reused |
| complexity.side_effect_risk | moderate | removing the registered config key changes config.list output while Sage category logic remains unchanged |
| risk.correctness | moderate | source searches show the config item is registered but never read by the Sage gate |
| risk.fit | low | the remaining change removes only a surface that no live decision path consumes |
| risk.test | low | existing config-scope coverage can pin the removal |
| risk.security_or_contract | moderate | config.list stops reporting a registered key |

## Phases

### Phase 1: Remove the two unconditional remnants

Confirm that the stale `spec-cleanup` tip remains absent (item 1); remove the
unread `sage_review_completeness` config item and its registration (item 2).
Update pinned tests accordingly and run `go build ./...`, `go vet ./...`, and
`go test ./...` to confirm no dangling references remain.

#### Edition (dc532266) - 2026-09-10

Item 1 was completed by `dc532266` while the related ready-only-gates ticket
rewrote the same `tickets.move` description. Treat that removal as verified
prior work and execute item 2 as the remaining implementation scope, adding
config-list regression coverage. Verification still confirms that
`spec-cleanup` is absent from `agents-plugin-tool` and runs the phase's full
build, vet, and test commands.

### Result (c0d10c44) - 2026-09-10

Removed the unread completeness config constant and registration. Fresh config
listing no longer advertises the retired setting; regression coverage also
confirms the Sage gate setting and both reviewer tier settings remain listed.
Existing stored arbitrary overrides retain their generic resolution behavior.
The category-based gate and doc-mode outputs were not changed. Item 1 remains
completed by `dc532266`, with no `spec-cleanup` source hits.

Verification: `go build ./...`, `go vet ./...`,
`TMPDIR=/private/tmp go test ./...`, `scripts/smoke-ws-mcp.sh ..`, and
`git diff --check` passed. The canonical temporary directory avoids the known
macOS symlink-alias test environment issue. One independent full-scope reviewer
returned clean; no unresolved findings or omitted phase work remain.
