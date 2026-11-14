---
title: "Owner-approved nonblocking hosted-session hiring"
related:
  261114-research-ws-clerk-session-role: "preserve settled authoring and management boundaries"
  261114-bug-ws-fork-parent-identity-loss: "historical fork-lineage design context, not a new prerequisite"
---

# Owner-approved nonblocking hosted-session hiring

## Background

A lead should be able to request another daemon-hosted session without asking
the owner to create it manually. The owner reviews a hiring proposal, can
adjust launch settings, and approves or rejects it. The requester continues
working and receives the outcome by mailbox.

MAIN relayed the owner's `750=ok` at `2026-11-14T13:08:34Z` and
`751..753=ok` at `2026-11-14T13:11:24Z`. This research capture separates that
confirmed contract from implementation and policy choices still awaiting the
owner. It authorizes no implementation, real session creation, permission
migration, runtime change, or compatibility break.

## Authority table

| Session | Root and lineage | Hiring request | Authoring and other management |
|---|---|---|---|
| Existing root, flag off | Existing root identity is unchanged | Denied | Existing lead authority is unchanged |
| Root with owner-enabled flag | Cannot become another session's child | May submit a nonblocking proposal; the server verifies eligibility | Existing lead authoring; owner decides creation and launch settings; model cannot set the flag |
| Hired child | Attached to the requester; never a root | Denied; no chained hiring | Authoring parity is not settled by this feature; no new management or global-configuration authority is inferred |
| Clerk | Parent lineage; the settled new-fork scope applies, existing sessions are unchanged | No hiring entitlement | Own-resource and explicitly delegated parent-resource authoring under 720; no session creation, management, or global configuration under 721 |
| Worker | Existing bounded delegation identity | No hiring entitlement | Existing tool allowlists and file grants; no automatic clerk parity or new management powers |

The clerk rows preserve decisions 720–722 from
`261114-research-ws-clerk-session-role`. Existing bounded worker-subagent
mechanisms are not silently redesigned by the new hiring tool.

## Outcome Ledger

### Verified Findings

Evidence below is source-only, pinned to Git object `ec2320552`; it does not
assert equivalence with the running daemon.

- Hosted creation already accepts harness, model, effort, native integrations,
  and trusted-owner controls, but currently creates parent-less lead keys. It
  cannot yet create the requested parented hosted child:
  `agents-plugin-tool/internal/wsagent/http.go:137–164` and
  `agents-plugin-tool/internal/wsagent/runner.go:575–638`.
- Hosted settings controls persist through Runner records. Non-hosted CLI
  sessions do not have the same hosted settings surface:
  `agents-plugin-tool/internal/wsdashboard/assets/dashboard.js:8256–8262,8338–8348,8393–8396`.
- Current permission approval cards depend on a live harness request. Their
  interactions are prior art, not an existing pre-creation proposal lifecycle:
  `agents-plugin-tool/internal/wsdashboard/assets/dashboard.js:8434–8511` and
  `agents-plugin-tool/internal/wsagent/runner.go:2899–2920`.
- Reply-ID system mail can deliver a rejection before a child exists and wake
  the requester:
  `agents-plugin-tool/internal/wsmailbox/replyid.go:150–164` and
  `agents-plugin-tool/internal/mcp/pi_tools.go:502–519`.
- Runner ownership/reporting parent and MCP authority parent are separate
  representations. They are not equivalent to an explicit hiring-root flag:
  `agents-plugin-tool/internal/wsagent/record.go:221–238` and
  `agents-plugin-tool/internal/mcp/session_auth.go:19–31`.
- Generic parent configuration inheritance exists; an owner-controlled hiring
  flag must not become inheritable merely by using that path:
  `agents-plugin-tool/internal/mcp/session_auth.go:516–550`.
- Existing trusted UI controls check loopback/origin rather than human
  provenance. That mechanism alone does not establish the new requirement
  that the model cannot enable its own flag:
  `agents-plugin-tool/internal/wsdashboard/access.go:156–169`.

### Confirmed Decisions

- **750:** Add a root-session option to the hosted-session settings UI. Such a
  root cannot become another session's child. Only a root with the enabled
  flag can request hiring; the server rejects other callers.
- **750:** Requests are nonblocking. The lead may state that a creation or
  rejection notification will arrive after the owner decides; an accepted
  request is not a claim that a child already exists.
- **750:** The dashboard displays a hiring-proposal approval modal identifying
  the requester. The owner can adjust harness, model, and effort using the
  existing style of controls, then approve or reject with a reason.
- **750:** Approval creates a child of the requester. Creation or rejection
  with its reason is delivered to the requester by mailbox.
- **751:** Only the owner enables the root flag through the UI, with server
  verification. The model cannot enable that flag.
- **752:** Hired sessions are children, never roots, and cannot chain hiring.
- **753:** The compatibility target is **T0 additive**: one new MCP tool and an
  optional settings field. This does not approve a clerk encoding, a change
  to existing identity/gate semantics, a shared-store rewrite, or a T2 change.
- The authority table preserves clerk 720–722. Clerk authoring parity does not
  confer hiring eligibility or session-management powers.

### Proposals

These are non-authoritative candidates, not an implementation contract.

- Preserve the owner's API sketch for later schema settlement:
  `ws/daemon.request_hosted_session(harness, model?, tier?, label, why, why_context)`.
  The identifier, option types, precedence, effort representation, and exact
  response shape are not finalized by this research capture.
- Use one daemon-owned proposal lifecycle, owner-adjustable approval, and the
  existing Runner launch machinery plus a correlated requester mailbox
  outcome. Recheck eligibility immediately before creation. No storage
  representation, retention rule, event, endpoint, or correlation field has
  been selected.
- Keep hiring eligibility distinct from both Runner `ParentRef` and MCP
  lineage. Reuse existing approval UI interactions rather than pretending a
  not-yet-created child has a live permission request.

### Open Questions

These questions are unnumbered for MAIN to classify and mint in the owner
conversation. They are not settled defaults.

- What pending-request and active-child limits apply, and when do pending
  proposals expire? What happens across requester or daemon restart?
- Which parts of `why` and `why_context` appear in the approval modal, and what
  sensitive or large context should be omitted, bounded, or expandable?
- What happens to approved children and unresolved proposals when the parent
  closes or loses eligibility? Do existing parent/child lifecycle rules
  suffice, or is a new policy required?
- May non-hosted CLI leads, including MAIN's Claude CLI session, be enrolled
  as roots? If yes, where does the owner enable the flag without a hosted
  settings popover?
- How does `tier` relate to explicit model and effort selections, and which
  value wins after the owner adjusts the proposal?
- What threat boundary proves owner-only flag changes? Existing loopback and
  same-origin UI checks distinguish request admission, not a human owner
  from a local process that can issue HTTP requests.
- What authoring and resource authority does a hired child receive? Do not
  infer clerk parity from merely attaching the child to a parent.

### Rejected Alternatives

- Model-controlled flag enablement and hiring by unflagged roots, clerks,
  workers, or hired children conflict with confirmed eligibility and 751–752.
- Promoting hired children to roots or allowing chained hiring conflicts with
  752.
- Making the request block until owner approval conflicts with the confirmed
  nonblocking mailbox workflow.
- Treating the research or documentation merge as implementation authority,
  or reinterpreting existing identity gates to force T0, is not approved.
