---
title: "Retire the workset ticket convention"
related:
  260726-feat-verify-ticket-graph-advisories: the epic/workset boundary analysis that surfaced this; its Out of Scope carries the supporting measurements
  260624-epic-pre-release-cleanup: currently categorized `epic` but shaped as a workset (`## Items` + `### 1.`-`### 8.`, zero `parent:` children); it remains outside this retirement (ai-docs/tickets/todo/260624-epic-pre-release-cleanup.md; `rg -n '^parent: 260624-epic-pre-release-cleanup$' ai-docs/tickets` returned nothing)
  260713-workset-workflow-dogfood-bugs: the sole open workset; close it as dropped after verifying its listed tickets remain independent
  260910-refactor-ready-only-actionable-ticket-gates: preserves ready as the actionable implementation queue while this category retires
parent: 260909-epic-ws-worker-interpreter-refoundation
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: d8cced4412bf89e1
sage-review-completeness-reviewed: d8cced4412bf89e1
completed: 2026-09-10
---

# Retire the workset ticket convention

## Background

`workset` was introduced for one job: **group tickets that must be handled in one
pass even when their categories and parents differ.** The goal-mode `lead-run`
flow now drains the scoped actionable `ready/` queue, giving the same explicit
mixed-parent execution without a second board artifact to maintain.

The user confirmed the convention is retirable on this basis (2026-07-26).

This does **not** transfer workset's role to `epic`. Workset's role went to a
runtime loop, not to another document category. An epic that absorbed
"project-wide common agenda" would re-create workset immediately after deleting
it, so `epic`'s identifier stays **single-outcome decomposition**.

## Decision

Remove `workset` as a ticket category. The grouping need is served by the goal
loop over `ready/`; the hierarchy need is served by `epic` + `parent:`; the
non-hierarchical annotation need is served by frontmatter `related:`.

## Compatibility Decisions

- **Stop new creation, preserve historical reads.** Remove `workset` from every
  authoring inventory, template, checklist, and accepted-type error, while
  retaining enough category recognition for active and archived workset stems
  to remain queryable and closable. Historical files are not rewritten.
- **Drop the sole open workset after verifying its references.**
  `260713-workset-workflow-dogfood-bugs` owns no child relationship; its listed
  tickets retain their own statuses and history. Close the workset as dropped
  during retirement without creating a replacement grouping ticket.
- **Leave the workset-shaped cleanup epic alone.**
  `260624-epic-pre-release-cleanup` is historical backlog with an epic stem.
  Reclassifying or restructuring it does not help remove the workset category
  and remains outside this ticket.
- **One implementation slice.** Remove the live authoring surface and preserve
  legacy parsing together, so there is no intermediate release that still
  invites creation of a category already decided for retirement.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/rsrc/, agents-plugin-wsflow/skills/, agents-plugin-tool/internal/wsdoc/conventions/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsdoc/tickets_template.go, agents-plugin-tool/internal/wsdoc/tickets_checklist.go, agents-plugin-tool/internal/wsdoc/conventions/ticket-conventions.md, agents-plugin/rsrc/lead-ticket/lead-ticket.md, agents-plugin/rsrc/lead-workflow-manual/lead-workflow-manual.md, and wsflow mirrors |
| scope.surface | public-interface | tickets.template and tickets.checklist expose the accepted category list in agents-plugin-tool/internal/mcp/server.go#L3462-L3482 |
| scope.new_public_symbol | no | removes an accepted category from existing interfaces; no new symbol named |
| scope.new_type_contract | no | no new type or signature named |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/tickets_template_test.go, agents-plugin-tool/internal/mcp/tickets_checklist_test.go, and agents-plugin-tool/internal/wsdoc/tickets_mutate_test.go cover accepted categories and legacy lifecycle behavior |
| complexity.reuse_points | confirmed | existing query and close operations are registered in agents-plugin-tool/internal/mcp/server.go#L3384-L3406 |
| complexity.side_effect_risk | moderate | category recognition must remain for historical reads and close operations |
| risk.correctness | moderate | authoring rejects workset while legacy stems must remain readable and closable |
| risk.fit | moderate | ws and wsflow mirrors plus conventions must agree |
| risk.test | moderate | existing category-matrix tests require coordinated updates |
| risk.security_or_contract | moderate | tickets.template and tickets.checklist change their caller-visible accepted-category contract |

## Phases

### Phase 1: Retire workset authoring while preserving legacy reads

Remove `workset` from the ticket conventions, ticket-system concepts,
`lead-ticket`, bootstrap template, creation schema, template and checklist
accepted types, and their wsflow mirrors. Keep existing workset stems readable
through ticket query, graph, status, and close operations; retain the internal
nonimplementation classification only where that compatibility requires it.

Move `260713-workset-workflow-dogfood-bugs` to `.dropped/` after confirming its
listed tickets remain independently discoverable. Do not rewrite archived
worksets or the workset-shaped cleanup epic. Regenerate embedded resources and
manifests through their existing generators.

Verify new workset creation, templates, and checklists fail with an accepted-
category list that omits workset; active and archived workset stems remain
queryable; the open workset closes without changing listed tickets; no shipped
skill or convention recommends workset; ws/wsflow mirrors agree; and existing
epic, research, and actionable ticket behavior remains intact.

### Result (f262e488) - 2026-09-10

Retired workset creation, templates, checklists, accepted-category errors, and
live authoring guidance. Historical category recognition remains available for
query, status, graph, and close operations. Shared bootstrap v0050 removes old
guidance without converting worksets into epics or rewriting legacy files;
resource mirrors and manifests were regenerated.

Dropped `260713-workset-workflow-dogfood-bugs` in `4c00f454`. All eight included
tickets were independently resolved, and review compared their paths and blob
IDs against the base: three done, two dropped, one idea, and two todo, with no
content or status changes and no parent relationship to the workset. Existing
archived worksets and the cleanup epic were unchanged.

Verification: `go test ./...` passed with canonical macOS `TMPDIR`;
`scripts/smoke-ws-mcp.sh ..` passed; all 10 wsflow package tests passed;
resource, mirror, and skills-manifest generators passed; `git diff --check`
passed. New regressions cover rejection before filesystem mutation in each
authorable status, accepted-category lists, active and archived legacy
lifecycle operations, related links, and independent ticket preservation.
Correctness, fit (including fresh-reader prose audit), and test reviews were
clean; correctness review also passed the focused wsdoc/MCP tests.

Decisions: creation rejects the retired category specifically, preserving
unrelated existing category behavior; historical bootstrap migration exclusions
remain explicit compatibility rules. The first full-suite run exposed a new
test's incorrect terminal-status expectation, corrected to `.done`/`.dropped`,
and reproduced the already captured
`260910-bug-route-ticket-absolute-path-symlink-alias`; canonical `TMPDIR` avoided
that unrelated environment failure without changing path-resolution source.
No unresolved review findings or deferred retirement scope.

## Out of Scope

- Changing `epic` semantics. Epic stays single-outcome decomposition; see
  Background.
- Changing goal-mode `lead-run`. This ticket consumes that mechanism's
  existence as a premise; it does not modify it.
- The workflow manual's **Ticket System Concepts** epic-vs-workset rationale
  paragraph is in scope for Phase 1, but the broader manual restructure is not.
