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

MAIN relayed the following later owner answers at `2026-11-14T13:22:52Z`
and the final parameter naming at `2026-11-14T13:23:10Z`:

```text
765=ok
767=^대기 요청에는 한도 없으나 만료는 30분으로요.
768=c^사실상 사용자가 UI로 세션 추가한 취급입니다. 요컨대 모달만 대신 띄워주는 수준의 조작.
769=a
766=^이거 파라미터 이름은 좀 더 같이 논의해주면 좋겠는데, 취지는 이렇습니다:

why는 오너에게 보여줄 텍스트,
why_context는 그 사이에 세션이 압축되거나 해서, 나중에 오너가 승인한 걸 에이전트가 받았을 때 이 에이전트를 왜 요청했었는지를 다시 복구하기 위한 긴 맥락 텍스트에요.
770=a
```

The English contract below uses the settled names `reason` and
`recovery_context`; the original `why` names are historical provenance only.
Decision 768 changes lifecycle framing to owner-UI-created equivalence but
does not itself settle whether the earlier requester-child relationship is
MCP authority lineage or display/reporting grouping. Preserve that distinction
as an open reconciliation, not an inferred reversal of 750/752.

## Authority table

| Session | Root and lineage | Hiring request | Authoring and other management |
|---|---|---|---|
| Existing root, flag off | Existing root identity is unchanged | Denied | Existing lead authority is unchanged |
| Root with owner-enabled flag | Cannot become another session's child | May submit a nonblocking proposal; the server verifies eligibility | Existing lead authoring; owner decides creation and launch settings; model cannot set the flag |
| Approved hired session | 750/752 specify requester's nonroot child; 768 requires owner-UI-created handling. Authority lineage versus display/reporting grouping remains unresolved | Earlier 752 denies chained hiring; 768 does not silently repeal that restriction | Use owner-UI-created lifecycle handling; hired-session authoring and management mapping remain open, not automatically clerk-equivalent |
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
  provenance: `agents-plugin-tool/internal/wsdashboard/access.go:156–169`.
  Decision 765 explicitly accepts that existing threat boundary; it does not
  claim stronger human authentication has already been implemented.

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
- **765:** Use the existing trusted approval-control boundary: loopback plus
  exact Origin. Stronger human provenance is deferred to a future Tauri
  token. This is an explicit trust boundary, not proof against an arbitrary
  local process that can issue HTTP requests.
- **766/770:** `reason` is the owner-facing explanation. `recovery_context` is
  the longer context that lets the requester recover why it asked after
  compaction. Echo that recovery context in approve, deny, and expiry mail.
  Request harness/model/effort values prefill the modal; the owner's adjusted
  choices are final. The former `why`/`why_context` names are superseded.
- **767:** There is no pending-request count cap. Pending requests expire
  after 30 minutes and notify the requester with an `expired` mail.
- **768=c:** Treat an approved session as if the owner had added it through
  the UI; the request effectively prefills the owner's modal. Do not select
  automatic child stopping on parent close. The relationship between this
  lifecycle choice and 750/752's nonroot-child framing remains open below.
- **769=a:** Allow the owner to enroll a non-hosted CLI lead through the same
  root toggle on that CLI session's rail row.
- The authority table preserves clerk 720–722. Clerk authoring parity does not
  confer hiring eligibility or session-management powers.

### Proposals

These are non-authoritative candidates, not an implementation contract.

- Preserve the API sketch with the now-settled explanation parameter names:
  `ws/daemon.request_hosted_session(harness, model?, tier?, label, reason, recovery_context)`.
  The identifier, option types, tier/default resolution, effort argument
  representation, and exact response shape are not finalized here. Owner
  adjustment in the modal remains final regardless of request defaults.
- Candidate hired-session authoring default: reuse ordinary owner-UI-created
  hosted-session authoring rather than silently adopting clerk parity. This
  is a proposal only; settle the 768 versus 750/752 authority mapping first.
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

- **Authority reconciliation:** after 768=c, is the requester-child relation
  an MCP authority parent or only display/reporting grouping? Does 752's
  nonroot/no-hiring restriction remain unchanged, or does the owner intend
  UI-created equivalence to revise it? MAIN must settle any reversal; this
  draft does not choose one.
- What authoring and resource authority does an approved hired session
  receive? The ordinary UI-created authoring candidate above is not settled
  merely by placing the session under a requester in the rail.
- How are unresolved proposals and their 30-minute expiry handled across
  requester closure, eligibility loss, or daemon restart? Do not infer that
  approved sessions stop with the requester.
- Are any active-session limits intended? The confirmed absence of a pending
  cap is not an invented active-child quota.
- What input-size, storage, and display treatment does long
  `recovery_context` need? Its requester-recovery purpose and outcome-mail
  echo are settled; exact fields, retention, and modal presentation are not.
- How does `tier` resolve initial model/effort defaults before the owner sees
  the modal? Owner-adjusted launch values always win.

### Rejected Alternatives

- Model-controlled flag enablement and hiring by unflagged roots, clerks,
  workers, or hired children conflict with confirmed eligibility and 751–752.
- Promoting hired children to roots or allowing chained hiring conflicts with
  the earlier 752 decision. Any intended change under 768 must be explicitly
  reconciled rather than silently implemented.
- The proposed maximum of two pending requests and automatic parent-close
  child stopping were not selected: 767 has no pending cap and 768=c uses
  owner-UI-created lifecycle handling.
- Making the request block until owner approval conflicts with the confirmed
  nonblocking mailbox workflow.
- Treating the research or documentation merge as implementation authority,
  or reinterpreting existing identity gates to force T0, is not approved.
