---
title: "sage_review config has no lead-facing setter or tuning catalog knob"
parent: 260605-epic-ws-playbook-factory-pivot
related:
  260622-feat-sage-review-ticket-gate: introduced the layered sage_review resolver contract
  260626-feat-surface-sage-review-posture: consumes the resolved posture at ticket boundaries
  260814-refactor-config-collapse-tuning-knobs-to-list-tune: landed the registry-backed config.list and config.tune surface this ticket extends
sage-review: required
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: bc61f58601174650
sage-review-completeness-reviewed: bc61f58601174650
completed: 2026-09-11
---

# sage_review config has no lead-facing setter or tuning catalog knob

## Background

The layered config resolver already owns `sage_review`, and ticket creation,
movement, and Sage gating already consume its resolved value. However,
`config.list` reports the raw override without advertising `sage_review` as a
tunable catalog knob, `config.tune` rejects the key as unknown, and
`lead-tune` has no handler for review posture. A lead therefore cannot apply a
user-requested Sage default without editing config JSON by hand.

This surfaced again when the user asked for Sage review to be required across
all harnesses. The concrete stored value for that preference is a global
`sage_review: auto` override. The product builtin is already `auto`; the
currently resolved `ask` comes from a global override. This ticket enables the
lead to restore that global setting through the supported tuning surface rather
than changing the product default.

## Settled contract

- Add `sage_review` to the static config registry as a resolver-backed,
  no-agent-visible knob written and reset through `config.tune`.
- Accept exactly `off`, `ask`, and `auto`. Their ticket-boundary postures are
  `skipped`, `recommended`, and `required`, respectively.
- Preserve `ScopeProject` as the declared default write scope. Expose the
  resolver-supported `session`, `project`, and `global` scopes through the
  catalog and reject values or scopes outside the advertised schema.
- `sage_review` is not harness-specific. Do not expose or require a harness
  selector; a global override applies across harnesses through ordinary config
  resolution.
- Extend `lead-tune` with a Sage-review handler and tune-target route. It must
  obtain the knob, values, writer, and scope choices from `config.list`, map the
  user's review posture to `off|ask|auto`, confirm the concrete proposal, and
  call `config.tune` rather than editing config storage directly.
- Keep `sage_review_design_tier`, `sage_review_completeness`, and
  `sage_review_completeness_tier` outside this ticket.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/rsrc/, agents-plugin-wsflow/skills/, agents-plugin-tool/internal/wsdoc/conventions/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)
- Read `ai-docs/manuals/ws-mcp.md` before editing
  `agents-plugin-tool/internal/mcp/`.
- Read `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/skill-authoring.md`, and
  `ai-docs/manuals/wsflow-mirroring.md` before editing the shipped `lead-tune`
  playbook or its wsflow mirror.
- Preserve the generic registry as the source of config value schema, allowed
  and default scopes, harness applicability, no-agent visibility, and writer
  metadata. Do not add a new per-key MCP setter.
- Preserve explicit confirmation before any project or global write.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/mcp/config_registry.go, agents-plugin-tool/internal/wsconfig/scope.go, agents-plugin/rsrc/lead-tune/lead-tune.md, agents-plugin-wsflow/rsrc/lead-tune/lead-tune.md |
| scope.surface | cross-module | config.list/config.tune catalog behavior spans MCP runtime and both shipped lead-tune playbooks |
| scope.new_public_symbol | no | extends the existing config.list/config.tune key catalog; no new MCP tool is proposed |
| scope.new_type_contract | no | no new Go type or MCP tool signature is proposed; the existing key-addressed catalog gains a value enum |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/session_state_test.go and agents-plugin-tool/internal/mcp/prompt_override_test.go cover Sage resolution and config catalog behavior |
| complexity.reuse_points | confirmed | existing configRegistry and buildTuningCatalog/config.tune generic paths in agents-plugin-tool/internal/mcp/config_registry.go and server.go |
| complexity.side_effect_risk | moderate | writes may persist a ticket-review default at project or global scope |
| risk.correctness | moderate | an incorrect enum, scope, or posture mapping changes ticket-boundary review behavior |
| risk.fit | moderate | lead-tune must preserve catalog-driven routing and the wsflow mirror |
| risk.test | moderate | existing catalog, scope, authority, no-agent, and ticket-boundary integration coverage must be extended |
| risk.security_or_contract | moderate | config.tune remains lead-authorized and its public catalog contract gains a persistent preference |

## Phases

### Phase 1: Make Sage review posture tunable through the generic config surface

Add the `sage_review` registry entry, catalog projection, generic write/reset
coverage, and `lead-tune` routing in the full and wsflow shipped surfaces.
Verify that:

- `config.list` advertises `sage_review` with writer/reset metadata, the exact
  value enum, project default scope, all resolver-supported write scopes, no
  harness applicability, and the current resolved value;
- `config.tune` writes and resets session, project, and global overrides,
  rejects invalid values/scopes and conflicting reset arguments, enforces lead
  authority, and remains available in no-agent mode for this key;
- the full and wsflow `lead-tune` surfaces route skipped/recommended/required
  intent to `off|ask|auto`, use catalog metadata, confirm the selected scope,
  and stay mirrored; and
- temporary-config integration coverage demonstrates that global `auto`
  resolves to required ticket-boundary Sage posture without changing the
  builtin default or depending on a harness selector.

### Result (b0248e51) - 2026-09-11

- Added the resolver-backed `sage_review` registry entry and catalog projection
  with `off|ask|auto`, reset metadata, project default scope, and advertised
  session/project/global scopes; it remains visible and writable in no-agent
  mode through generic `config.tune`.
- Updated the shared `lead-tune` playbook and regenerated byte-identical wsflow
  resources so skipped/recommended/required requests map to `off|ask|auto`
  after catalog-driven scope selection and confirmation.
- Corrected generic scalar reset and catalog-current resolution to retain the
  caller session key, so catalog and reset responses report the same effective
  posture as ticket boundaries.
- Verification: `go test ./internal/mcp -count=1 -run 'TestConfig(TuningCatalogProjectsPromptAndSchemaKnobs|TuneSageReviewScopesAndValidation|TuningCatalogNoAgentShape)|TestServeStdioGlobalSageReviewAutoRequiresTicketBoundaryReview|TestPlaybookPrint(LeadTuneUsesWorkflowPreferenceCatalogKnobs|WsflowLeadTuneOmitsFullWsOnlyCatalogKnobs)'`; `WS_SKILLS_ROOT=../agents-plugin/skills go test ./... -count=1`; `python3 -m unittest discover ../agents-plugin-wsflow/tests`; `git diff --check`.
- Reviews: fit passed; correctness found and round-two verified the
  session-effective catalog/reset correction; test review found and round-two
  verified no-agent write/reset coverage.


## Resolution (2026-09-11)

Implemented the registry-backed Sage review tuning surface, mirrored lead-tune route, and regression coverage.
