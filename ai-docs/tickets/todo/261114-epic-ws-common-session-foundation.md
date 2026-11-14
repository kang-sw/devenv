---
title: "Common session foundation: minimum interface, parity migration, then extensions"
sage-review-design: required
related:
  261110-epic-ws-dashboard-agent-host: "existing hosted-agent product scope; this epic supplies a shared foundation without replacing its child plans"
  261113-bug-ws-hosted-session-sentinel-bootstrap-mints-foreign-key: "identity-adoption regression evidence, not authority to restrict root issuance"
  261114-bug-ws-fork-parent-identity-loss: "lineage evidence; owner 705=b selects clerk design rather than narrow 676 fix"
  261114-research-ws-clerk-session-role: "design-only child: parent lineage and own/explicitly delegated parent-resource authoring; 720–722 confirmed, no top-level powers"
  261114-feat-ws-conversation-audit-event-store: "audit backend consumes common-session-interface Phase 2 (682), not whole-epic completion"
  261114-feat-ws-dashboard-conversation-audit-view: "UI still consumes audit-event-store Phase 2"
---

# Common session foundation

## Scope

Owner (674)=a, with (676)–(680)=ok, authorizes this documentation map and first
interface ticket. Delivery order is minimum interface, current behavior/state
migration with parity, then extensions.

1. Separate CLI-plugin entry from MCP-only hosted entry while sharing
   playbook, context and state access. Include only the minimum session
   identity, existing authority and parent lineage needed for migration.
2. Adopt that boundary while preserving sessions, history, boards, approvals,
   delegation ownership and recovery. Prefer existing-store compatibility and
   observable parity over rewriting storage.
3. Connect authority UI, additional harnesses, voice, live reasoning and audit
   to concrete producer tickets/phases as those consumers are captured.

## Non-Scope

This map does not authorize source execution, a replacement universal runtime,
new approval powers or root-key issuance restrictions. Existing MCP root
issuance removal/blocking is unchosen and may be T2; MAIN/owner must settle it
separately. Existing session keys and historical state are preserved.

## Child Tickets

- `261114-research-ws-clerk-session-role` — owner (705)=b, with (720)=c,
  (721)=a and (722)=c relayed 2026-11-14 at 12:08: design clerk parent-lead
  lineage for display/ownership and lead-equivalent whiteboard, notes,
  tracker, wiki and document authoring for own resources plus explicitly
  delegated parent resources. No session creation/management/global config.
  New forks become clerks; existing sessions remain unchanged. Reassess the
  historical fork exemption and tool gates; no wire/store encoding or T2
  change is chosen. This research adds no prerequisite gate to the existing
  interface or epic implementation.
- `261114-refactor-ws-common-session-interface` — first child: Phase 1 defines
  the minimal shared access/identity/event-adapter boundary; Phase 2 adopts
  current access with state/behavior parity. Its prior art is the landed
  `261114-refactor-ws-hosted-common-contract-pi-claude-parity#phase1`, not a
  replacement for that completed result.
- `261114-feat-ws-conversation-audit-event-store` — reuse the existing backend
  ticket, after `261114-refactor-ws-common-session-interface#phase2` (682).
  Backend Phase 1 captures/stores events and ring call IDs; Phase 2 serves the
  API, including additive UTC selection (681).
- `261114-feat-ws-dashboard-conversation-audit-view` — reuse the existing UI
  ticket, still after `261114-feat-ws-conversation-audit-event-store#phase2`.

### Extension dependency map

The foundation producer below is concrete even where the consumer ticket is
not yet authored. Consumer-specific contracts remain for future capture; none
waits for the whole epic.

| Follow-on | Consumer | Foundation prerequisite | Additional confirmed prerequisite |
|---|---|---|---|
| Authority UI | Not yet authored | `261114-refactor-ws-common-session-interface#phase2` | No additional prerequisite settled |
| Additional harnesses | Existing `261114-feat-ws-hosted-antigravity-acp-adapter`; other consumers not yet authored | `261114-refactor-ws-common-session-interface#phase2` | Antigravity retains landed `261114-refactor-ws-hosted-common-contract-pi-claude-parity#phase1` as prior foundation; its own Phase 2 follows its Phase 1 exact-version contract and separately authorized account/access |
| Voice | Not yet authored | `261114-refactor-ws-common-session-interface#phase2` | No additional prerequisite settled |
| Live reasoning | Not yet authored | `261114-refactor-ws-common-session-interface#phase2` | No additional prerequisite settled |
| Audit backend | Existing `261114-feat-ws-conversation-audit-event-store` | `261114-refactor-ws-common-session-interface#phase2` (682) | Backend Phase 2 follows its Phase 1 store/mapping/call IDs |
| Audit UI | Existing `261114-feat-ws-dashboard-conversation-audit-view` | Via backend's adopted interface | `261114-feat-ws-conversation-audit-event-store#phase2` remains the exact dispatch gate |

The completed parity and deferred Antigravity tickets were read from develop
because this older docs-only base does not contain them. Their exact stems and
phases above are retained without copying or rewriting those tickets here.

## Cross-Child Decisions

- CLI-plugin and MCP-only hosted entry remain distinct. Common access does
  not make MCP attachment a substitute for harness-native integration or
  process control.
- Preserve identity, existing authority, lineage, state ownership and one
  owner of input/mail delivery through migration. Mailbox reachability and
  profile names do not independently grant approval authority.
- Reuse the existing Runner/adapter boundary and native Pi/Claude behavior.
  The completed internal parity consolidation does not establish that all
  workflow/context/state consumers have migrated.
- Interface and parity adoption precede extensions. MAIN (682), settled
  2026-11-14, puts current context/state parity adoption in the first interface
  ticket's Phase 2; no separate migration child is created. Audit waits for
  that phase, not for all harnesses, voice or authority UI. Its backend/API/UI
  ordering and approvals 594–611 survive.
- Owner (705)=b selects clerk role design, not the narrow (676) parent
  wiring fix. (720)–(722) settle own/explicitly delegated parent-resource
  authoring only and new-fork mapping with existing sessions unchanged.
  The research separates lineage/ownership, authoring and top-level powers;
  any breaking gate/role/store/protocol change remains an explicit T2 risk
  requiring separate owner approval. No implementation is authorized by this
  design capture. Owner `681,682=ok` also reinforces the
  existing UTC-filter and audit/interface prerequisites; the queue is unchanged.
- Compatibility target is additive T0 with existing entry points and stored
  state readable. Any non-additive protocol/store/MCP change requires separate
  tier settlement; T2 is not authorized here.

## Completion Criteria

- Done: the owner accepts minimum shared access and current-state parity,
  every captured child is done or explicitly deferred/dropped, and the owner
  closes this epic.
- Dropped: the owner drops the direction.
- Deferred: unscheduled extensions remain proposals, not implicit
  implementation approval or whole-epic prerequisites.
