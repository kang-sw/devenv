---
title: "Explicit mailbox multi-recipient sends with public To/CC and hidden BCC"
---

# Explicit mailbox multi-recipient sends with public To/CC and hidden BCC

## Background

Owner decision (787)=a approves explicit multi-recipient mailbox sends, not scope-wide broadcast. A lead can address several known peers in one call while each receiver can see the public audience. The settled contract comes from MAIN's 2026-11-14 relay of the owner's answers; the source investigation `shared/docs/mailbox-multi-recipient.md` is supporting evidence, not authority for its additional proposals.

Existing routing, ordinary envelope queues and per-session history remain the substrate. Public recipients must survive existing body-based delivery and rendering without adding stored envelope fields.

## Decisions

- **Accepted argument shape — (787)=a:** `mailbox.send` accepts `to: string | string[]`, with optional `cc: string[]` and `bcc: string[]`. Preserve existing scalar addressing. To, CC and BCC are delivery destinations, not passive labels.
- **Independent fan-out:** the server delivers one ordinary envelope per recipient. This is explicit finite addressing; there is no all-peers broadcast, stored group, magic mailbox slug or membership expansion.
- **Public context in content:** the server prepends a generated public To/CC header to the content body. All copies carry public To/CC context; BCC is never shown in that public header or disclosed to co-recipients. Preserve the supplied message body after the generated context. Do not depend on a new structured envelope/header field to make the audience visible.
- **Compatibility: T0, additive arguments only.** Retain the current mailbox store and envelope shapes, including ordinary history records. No new store fields, shared-message object, schema-version change, protocol change or grouped-history replacement belongs to this feature.
- **Owner correction: no schema-check or old-daemon guidance.** The daemon serves the tool list; a session that sees `cc` already sees the new daemon's tool surface. Reject the source report's proposed capability-probing instruction, old-daemon argument fallback and additional schema-discovery gate.
- **Only residual rollback instruction:** add one line to the rollback runbook: "After rolling back the daemon, terminal sessions refresh their tool list." This is not a new feature-negotiation procedure or a release-time approval gate.

## Constraints

- This is accepted `todo/` backlog, not ready promotion or implementation authority. No mail is sent and no live daemon is changed while authoring this ticket.
- Read `ai-docs/manuals/shipped-surface-boundary.md` and `ai-docs/manuals/release-compatibility.md` before changing the tooling; read `ai-docs/manuals/ws-mcp.md` for changes under `agents-plugin-tool/internal/mcp/` and the rollback instruction. If shipped workflow guidance changes, also read `ai-docs/manuals/skill-authoring.md` and the matching `ai-docs/manuals/wsflow-mirroring.md` obligations.
- Preserve the existing named-address and reply-ID routing, sender/reply identity, queue ownership, receive/drain behavior, history access and normal reply-to-sender semantics. A public audience is display context, not authority over other sessions or a task-claiming mechanism.
- BCC means hidden from other mail recipients, not secrecy from the sender, local operator or ordinary tool-argument audit. Do not expose the BCC list in delivered body content, recipient history or public dashboard expansion. Do not claim BCC prevents a recipient from revealing their own presence by replying.
- Keep ordinary envelope independence: a recipient's receive must not consume another recipient's copy. Enqueue is not delivery acknowledgement; do not introduce an exactly-once or cross-store atomicity promise.
- Keep errors and successful routing observable without turning fan-out into a new persistent message protocol. This capture does not select a new batch-result schema, numerical fan-out limit, automatic retry, reply-all operation or alias-to-session identity scheme from the investigation's proposals.
- Existing host adapters must display public To/CC through the body they already receive. A host-specific structured-header rollout is not a prerequisite to this T0 feature.

## Phases

### Phase 1: Add explicit fan-out without changing mailbox storage

Implement the settled argument shape and server-generated public audience context on the existing send path. Deliver separate ordinary envelopes to the explicit To/CC/BCC recipients, preserving route-appropriate sender/reply identity and current receive/history semantics. Update the advertised tool arguments and relevant send documentation/tests, without adding the rejected schema-check or old-daemon guidance.

Add only the settled terminal-tool-list refresh sentence to the daemon rollback runbook. Do not expand this into broadcast, group membership, a store migration, reply-all, a new dashboard grouping feature or a new harness protocol.

Verification:

- Existing scalar To addressing still routes correctly. Array To, CC and BCC reach their explicit destinations through independent ordinary copies, including supported named-address/reply-ID routes.
- Every delivered copy shows the generated public To/CC header and original message body, while no co-recipient can recover the BCC list from its received envelope/body/history or dashboard rendering.
- Receiving one copy leaves other recipients' copies intact. Existing sender/reply identity, queue/drain and per-session history behavior remain functional.
- Store/envelope serialization remains unchanged and no schema-version bump is introduced. Existing body-based host delivery renders the public audience without a new adapter metadata contract.
- The advertised arguments match `to: string | string[]`, optional `cc: string[]`, optional `bcc: string[]`; no all-peers selector is added.
- The rollback runbook contains the one settled refresh sentence and no new schema-check/old-daemon guidance is introduced.
- Record exact focused test commands and results during implementation. Live peer mail, daemon replacement and cross-session smoke require their own authorization; this docs capture performs none.
