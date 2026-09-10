---
title: "Prune dead workflow remnants surfaced by the refoundation audit: stale move tip, unread config, inert doc-mode fields, and the unused todo-landing design gate"
related:
  260726-feat-enter-verdict-scenario-output: landing-order; that ticket (todo, sage-design passed) plans to keep doc_mode as a live verdict axis, so item 3's removal must sequence behind or reconcile with it
  260626-bug-sage-review-config-setter-missing: overlap; both touch sage_review_completeness (item 2), no hard conflict
---

# Prune dead workflow remnants surfaced by the refoundation audit

## Background

A source audit of the run/worker workflow (before the `epic/refound` ->
`develop` merge) surfaced four dead or stale remnants left by earlier tracks —
the spec/mental-model retirement and the worker-interpreter refoundation. None
cause an error today; each is misleading metadata or unreachable/unread code
worth removing so the shipped surface matches actual behavior. Grouped here as
one low-risk cleanup.

1. **Stale `tickets.move` schema tip.** The MCP tool schema description
   (`agents-plugin-tool/internal/mcp/server.go:3413`) still reads "Downward
   moves from ready/ return a spec-cleanup tip." The spec-cleanup tip was
   deleted with the ready spec-gate in `91621687`; `TicketsMove`
   (`internal/wsdoc/tickets_mutate.go`) now only emits sage-review-posture tips.
   The string is the only `spec-cleanup` hit left in the tree and is visible to
   any caller reading `tools/list`.

2. **Unread `sage_review_completeness` config item.** Declared and registered
   (`internal/wsconfig/scope.go:37-40`) with a doc comment claiming it "controls
   whether the completeness reviewer runs alongside the design reviewer," but it
   is never read by the gate logic — completeness applicability is decided
   solely by category in `sageReviewStageRequirement`
   (`internal/wsdoc/tickets_mutate.go:381-394`). Dead config: either wire it or
   remove it (and its declaration/registration).

3. **Inert `doc_mode` / `doc_reason` / `need_doc` resolver fields.** After the
   spec/mental-model retirement these survive on `implementVerdict` /
   `implementAgenda` (`internal/mcp/implement_resolver.go:109, 129-131`) and are
   still rendered as literal output (`:1068, :1094-1097`), but nothing consumes
   them — `implementTodoVerdict`, the struct that drives todo derivation, has no
   such field (`internal/mcp/session_state.go:381-407`). Cosmetic output with no
   gate or todo behind it *today* — but `todo/260726-feat-enter-verdict-scenario-output`
   (sage-design passed, currently blocked on an unresolved design finding) plans
   to keep `doc_mode` as one of ~6 live verdict axes narrated in enter.implement's
   scenario output. So the "no consumer" premise holds only for the current
   todo-derivation path; a planned consumer exists. Removing these fields is
   therefore a landing-order call against 260726, not a free removal — reconcile
   or sequence before touching item 3.

4. **Unused `landing: "todo"` design gate (needs a semantics decision, not a
   silent removal).** `SageGate` fully implements and tests a todo-landing
   design-only gate (`internal/wsdoc/tickets_sage.go:161-177`), but no shipped
   playbook invokes it — `lead-ticket.md:89` is the only `sage_gate` call site
   and uses `landing: "ready"`. This matches the settled direction that sage
   review is ready-only. But the Go path is reachable by direct tool invocation,
   and upward `tickets.move` to `todo/` still stamps a design posture, so this is
   a protocol-surface question, not dead code: does ready-only mean the
   todo-landing design gate should be removed outright, or kept as a latent API
   that shipped prose simply never calls? Resolve this before touching it; it is
   the one item here that changes API semantics.

## Decisions

- **Items 1-2 are pure dead-code / stale-metadata removals** (auto-proceed
  class): a stale schema string and an unread config key. Removing them cannot
  change observable behavior.
- **Item 3 is gated on a landing-order call** against
  `260726-feat-enter-verdict-scenario-output`, which plans to keep `doc_mode`
  live. It is not the free removal it first appeared to be; sequence or
  reconcile with 260726 before removing the fields.
- **Item 4 is gated on a semantics call** (always-ask: protocol/API change):
  does ready-only mean the todo-landing design gate is removed outright, or kept
  as a latent API shipped prose never calls? It may split into its own follow-up
  if the answer needs wider review.
- The cleanup lands items 1-2 regardless; items 3 and 4 each proceed only once
  their gating question is answered.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for
  agents-plugin-tool/, agents-plugin/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for
  agents-plugin-tool/internal/mcp/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for
  agents-plugin/rsrc/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for
  agents-plugin/rsrc/)

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/mcp/server.go, agents-plugin-tool/internal/wsconfig/scope.go, agents-plugin-tool/internal/wsdoc/tickets_mutate.go, agents-plugin-tool/internal/mcp/implement_resolver.go, agents-plugin-tool/internal/mcp/session_state.go, agents-plugin-tool/internal/wsdoc/tickets_sage.go, agents-plugin/rsrc/lead-ticket/lead-ticket.md |
| scope.surface | public-interface | tools/list-visible MCP tool schema description text, and the doc_mode/doc_reason/need_doc fields on implementVerdict/implementAgenda returned in enter.implement JSON output |
| scope.new_public_symbol | no | none |
| scope.new_type_contract | no | none, removes existing struct fields and a config item, adds no new type or signature |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/implement_resolver_test.go, agents-plugin-tool/internal/mcp/session_state_test.go, agents-plugin-tool/internal/wsdoc/tickets_sage_test.go already cover these exact surfaces |
| complexity.reuse_points | not-applicable | pure removal of dead code, config, and fields; no existing component is being reused |
| complexity.side_effect_risk | moderate | item 3's doc_mode/doc_reason/need_doc removal collides with todo/260726-feat-enter-verdict-scenario-output, whose Phase 1-3 plan keeps doc_mode as a live decision-trace/scenario axis |
| risk.correctness | low | grep confirms zero current consumers for items 1-3 (spec-cleanup string, ItemSageReviewCompleteness, doc_mode/doc_reason/need_doc); no decision path changes |
| risk.fit | moderate | item 4 is an explicit protocol/API-semantics question the ticket itself gates, and item 3 conflicts with 260726's (todo/, blocked) plan to keep doc_mode live |
| risk.test | low | Phase 1 updates pinned tests that already exercise these exact surfaces, plus go build/vet/test |
| risk.security_or_contract | moderate | item 4 changes reachable direct-invocation API/protocol semantics for landing=todo; the ticket itself names this as its one API-semantics item |

## Phases

### Phase 1: Remove the unconditional remnants (items 1-2) and record the gated decisions

Delete the stale `spec-cleanup` tip string (item 1); remove the unread
`sage_review_completeness` config item and its registration, or wire it if a
reason to keep it emerges (item 2). Update pinned tests accordingly and run
`go build/vet/test ./...` to confirm no dangling references remain.

Items 3 and 4 are gated and do not land here by default. For item 3, record the
landing-order decision against `260726-feat-enter-verdict-scenario-output`
(remove `doc_mode`/`doc_reason`/`need_doc` now, sequence behind 260726, or leave
them) in the Result; remove the fields only if the decision is "remove now." For
item 4, record the semantics decision (remove the todo-landing design gate vs
keep it as latent API) in the Result; act on it only if the decision is "remove"
and does not warrant a separate ticket.
