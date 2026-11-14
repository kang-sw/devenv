---
title: "Common session interface: distinct CLI/hosted entry, shared access and parity adoption"
parent: 261114-epic-ws-common-session-foundation
blocked-by: 261114-refactor-ws-hosted-common-contract-pi-claude-parity#phase1
related:
  261110-epic-ws-dashboard-agent-host: "preserve existing hosted session lifecycle and product scope"
  261113-bug-ws-hosted-session-sentinel-bootstrap-mints-foreign-key: "hosted identity adoption evidence; no root-issuance policy choice"
  261114-bug-ws-fork-parent-identity-loss: "lineage evidence, not authorization to alter fork authority"
  261114-feat-ws-conversation-audit-event-store: "downstream consumer after this ticket's Phase 2 (682); existing audit envelope stays audit-owned"
---

# Common session interface and parity adoption

## Background

Owner (674)=a approved drafting the common foundation's first interface ticket,
with (676)–(680)=ok. The complete workflow-context-projection board body sets
minimum interface -> current behavior/state migration -> extensions as the
order. The session-control-interface body remains design discussion, not
permission to impose its candidate root-management policy.

The landed `261114-refactor-ws-hosted-common-contract-pi-claude-parity#phase1`
consolidates internal Runner controls with Pi/Claude parity. This ticket builds
on that result to share workflow access across distinct CLI-plugin and
MCP-only hosted entry; it does not repeat the completed adapter refactor.

## Decisions

1. Keep the two entry modes distinct and share playbook, context and state
   access behind them. Preserve native plugin/extension behavior in modes that
   already use it. Rejected: treating MCP connectivity as native process
   control or replacing the existing runner with a second universal runtime.
2. Include only migration-essential session identity, existing authority and
   parent lineage. Existing key adoption and parent/owner relationships remain
   valid; mailbox reachability and profile labels are not permission proofs.
3. Adopt current state access with parity before extension work. Preserve
   sessions, history, boards, approvals, delegation ownership and recovery;
   existing-store compatibility is preferred to a storage rewrite.
4. Expose the session/event/adapter boundary audit needs without independently
   redefining audit's approved envelope, kinds, capture/redaction or call-ID
   contract (594–601). Audit remains a downstream consumer after Phase 2.
5. Compatibility target: additive T0. Preserve existing MCP/HTTP behavior,
   stored identities and readable historical records. Root issuance removal or
   blocking remains unchosen, possibly T2, and outside this ticket.
6. **MAIN (682), settled 2026-11-14:** Phase 2 adopts current context/state
   access with parity, without a store rewrite. The audit backend depends on
   this ticket's Phase 2; its UI keeps the backend's Phase 2 prerequisite.
   Rejected: adding a separate migration child or waiting for the whole epic.

## Constraints

- Implementation manuals: `ai-docs/manuals/shipped-surface-boundary.md` for
  plugin/tool paths; `ai-docs/manuals/release-compatibility.md` for tooling
  internals; `ai-docs/manuals/ws-mcp.md` if MCP wiring is touched;
  `ai-docs/manuals/skill-authoring.md` for authored workflow surfaces and
  `ai-docs/manuals/wsflow-mirroring.md` for mirrored package surfaces.
- Preserve one input/mail delivery owner, ordered event delivery, approval
  request/result correlation, holder attachment, replay gaps, recovery holds
  and unsupported-control behavior. Common access must not add a second
  delivery loop or weaken native backend capability differences.
- Preserve existing keys/state; do not retroactively rewrite fork lineage or
  fold peer leads into worker termination rules.
- New authority UI, new harness adapters, voice and live reasoning are later
  consumers after this ticket's Phase 2, not its acceptance scope.
- Before ready promotion, settle exact public signatures, context-selection
  behavior and any new serialized fields against current source. The board's
  proposed management checkbox, root-management powers and issuance
  restrictions are not selected mechanisms. This todo capture supplies no
  ready/Sage or implementation approval.

## Prior Art

- `261114-refactor-ws-hosted-common-contract-pi-claude-parity#phase1`, Result
  `44aeb62a8`: shared internal process controls, real-adapter/fake-holder parity
  fixtures, preserved native launch and holder delivery owners.
- `261113-bug-ws-hosted-session-sentinel-bootstrap-mints-foreign-key`: hosted
  bootstrap can adopt a foreign identity in the reported incident.
- `261114-bug-ws-fork-parent-identity-loss`: preserved historical key/row
  evidence; loaded/source equivalence and the historical fork-policy rationale
  require separate verification. Do not infer a completed repair.

## Prior Decisions

- Owner (674)=a and (676)–(680)=ok: capture the interface -> parity -> extension
  order in a docs-only lane; implementation is not authorized by this capture.
- `261114-refactor-ws-hosted-common-contract-pi-claude-parity#phase1`, Decision
  3 and Result `44aeb62a8`: native integrations remain active; holder transport
  and delivery policy stay in their existing owners — bearing: constrains.
- MAIN (682): audit's exact foundation producer is this ticket's Phase 2;
  current-state adoption is not a new store format — bearing: constrains.

## Phases

### Phase 1: Establish the minimum shared access contract

At the existing execution/access boundary, establish the shared playbook,
context and state access contract for separate CLI-plugin and MCP-only hosted
entry. Carry migration-essential identity, existing authority and parent
lineage and make the common session/event/adapter boundary explicit for later
consumers. Reuse current owners rather than introducing parallel stores,
delivery queues or a new workflow interpreter. Leave the audit envelope in the
existing audit ticket.

Verification boundary: isolated fixtures for both entry modes, existing key
adoption and parent propagation, current authority preservation, common context
and state access, event ordering and unsupported operations. Assert that old
entry points and historical state remain usable. Exact interfaces and affected
project commands are settled during refinement before ready promotion.

### Phase 2: Adopt current access with behavior and state parity

Depends on this ticket's landed Phase 1. Move existing access consumers onto
the shared boundary while preserving sessions, history, boards, approvals,
delegation ownership and recovery. Use compatibility access to existing stores;
no storage-format conversion or permission-policy change is implied (682).

Verification boundary: compare CLI-plugin and hosted paths against existing
behavior fixtures for start/resume, identity/lineage, input/mail single
ownership, approval correlation, history/board visibility, delegation ownership
and holder/replay recovery. Check historical reads and native Pi/Claude controls
separately. Record source fixture results separately from any later authorized
live acceptance. Completion is the precise foundation prerequisite for
`261114-feat-ws-conversation-audit-event-store`; audit's UI still depends on
that backend's Phase 2, not directly on this interface or whole epic.
