---
title: "Clerk session role: parent lineage with lead-equivalent authoring"
parent: 261114-epic-ws-common-session-foundation
related:
  261114-bug-ws-fork-parent-identity-loss: "observed lineage gap; narrow 676 repair rejected in favor of this design"
  261114-refactor-ws-common-session-interface: "existing foundation boundary; this research adds no implementation gate"
  260926-bug-pi-execute-worker-steals-mailbox-owner: "explicit lateral-fork exception and named-inbox ownership protection"
  260926-bug-pi-children-inherit-mailbox-identity: "all spawned children clear inherited mailbox identity, including forks"
---

# Clerk session role

## Background

Owner (705)=b, relayed by MAIN on 2026-11-14 at 12:02, selects design work
inside the common-session foundation rather than the narrow (676) fork fix.
Original owner wording:

> 이건 사실 우리 쪽 role의 gap으로 생각하고 있습니다. 문서 등 리드와 동등한 툴 저작 권한을 갖는 일종의 "sub-lead" role이 필요하다고 생각해요.

English translation: this is a gap in our roles; a kind of sub-lead role is
needed with tool-authoring authority equivalent to a lead, including documents.

Owner answers relayed by MAIN on 2026-11-14 at 12:08 settle the conceptual
role name as **clerk**, authoring scope and new-fork mapping below. The earlier
quoted term is historical; the creation date remains 261114 under the owner's
explicit semantic-rename instruction.

## Design coverage

Separate parent lineage for display/ownership from authoring authority and
from top-level lead identity. Study whiteboard read/write, notes, tracker,
wiki and document authoring together, rather than treating one passing tool
call as proof of a complete role. No wire value, stored field, capability
encoding or migration is selected by this research capture.

Trace native session/discussion forks and nested/dormant relaunches into
server classification and each tool gate. Include context visibility, parent
config inheritance, board/tracker enablement, mailbox startup/ownership and
negative authorization cases. Compare clerks with existing leads, parented
lead keys, delegates/leaves and store-scoped support conversations. Clerk
powers are authoring only, not session creation/management or global config;
existing sessions remain unchanged.

## Constraints

- Design only: no implementation, ready promotion, new agents/forks, live key
  changes, source build/tests, or release. Existing sessions/keys remain intact.
- Preserve (681)/(682): additive UTC audit filters and audit backend after
  interface Phase 2; UI remains after audit backend Phase 2. Owner's explicit
  `681,682=ok` reinforcement changes neither the queue nor these contracts.
- This research is not a new prerequisite for the existing interface or epic
  implementation. Any eventual implementation dependency is captured later.
- **T2 risk is explicit and unapproved.** Changing parent/role meanings,
  top-level eligibility, tool gates, root issuance, wire semantics or shared
  store interpretation may be breaking. Prefer a demonstrated T0/T1 route;
  actual T2 requires explicit owner approval, minor-release planning and the
  affected schema/protocol guard. The conceptual clerk name and (705)/(720)–
  (722) intent do not select on-disk encoding or approve a T2 change.
- Design against `ai-docs/manuals/release-compatibility.md`,
  `ai-docs/manuals/ws-mcp.md` and
  `ai-docs/manuals/shipped-surface-boundary.md`; authored workflow changes
  additionally require skill-authoring and applicable mirroring guidance.

## Outcome Ledger

### Verified Findings

- VERIFIED from `5fa2e7cb8` and the completed
  `260926-bug-pi-execute-worker-steals-mailbox-owner`: native and discussion
  forks were intentionally exempted as lateral leads, with no policy parent;
  their parent key traveled through `WS_PI_PARENT_SESSION_KEY`. Adding a
  policy parent was explicitly rejected there because it changes lineage/key
  semantics. The historical rationale must be reassessed, not called an
  accidental wiring omission.
- VERIFIED from that ticket and
  `260926-bug-pi-children-inherit-mailbox-identity`: parent-less non-lead mints
  must not rebind a named inbox; all Pi spawned children, including forks,
  clear inherited `WS_MAILBOX`/`WS_MAILBOX_AUTO`. A new role must not recreate
  inbox theft, phantom peers or competing presence ownership.
- VERIFIED as MAIN/D-supplied source evidence, not independently re-probed by
  this docs lane: under `agents-plugin-tool/`,
  `internal/mcp/session_auth.go:30-31` defines `isTopLevelLead` as lead plus
  empty parent; `server.go:752-763` gates tools; `workflow_manual.go:402-409`
  disables board/tracker knobs for the non-top-level case;
  `session_auth.go:516-550` handles parent config inheritance;
  `server.go:2397-2410` skips mailbox startup. These are design audit sites,
  not proof of equivalence to the currently loaded runtime.
- VERIFIED: `261114-bug-ws-fork-parent-identity-loss` preserves missing-parent
  key/row evidence and explicitly leaves loaded/source equivalence unverified.
  No source repair or retroactive parent rewrite is established here.

### Confirmed Decisions

- Owner (705)=b requests parent-lead lineage for display/ownership and
  lead-equivalent authoring for whiteboard read/write, notes, tracker, wiki
  and documents without becoming a top-level lead; the confirmed role name
  is now **clerk**.
- **(720)=c and (722)=c**, exact owner quote `720,722=c`, relayed by MAIN
  on 2026-11-14 at 12:08: clerks author their own resources plus explicitly
  delegated parent resources through an explicit resource matrix (720).
  New forks become clerks; existing sessions remain unchanged (722).
- **(721)=a**, exact owner answer relayed at the same time:

  > 721=a^흠, 생각해보니까 세션 생성을 안 주고, "clerk". role로 이름을 명명하는 게 나을 수도 있겠다는 생각이 좀 드네요.

  English translation of the note: on reflection, omit session creation and
  name the role clerk. MAIN settles **clerk**, not the earlier design label.
  Authoring parity only: no session creation, session management or global
  configuration powers. This is a conceptual name, not a selected wire,
  capability or stored role value.
- Design belongs inside `261114-epic-ws-common-session-foundation`. The
  narrow (676) fix is not the selected execution path. Existing (681)/(682)
  contracts, prerequisites and queue remain unchanged.
- Current authority is research/documentation only, not a chosen role
  representation, wire/store change or T2 approval.

### Proposals

- INFERRED, non-authoritative: evaluate lineage, resource authoring and
  top-level/session-management authority as separate design dimensions,
  using existing gates and regression fixtures before proposing changes.
- INFERRED, non-authoritative: produce a per-tool/resource access matrix,
  fork/relaunch mapping and mixed-version/legacy-key scenario table. Reuse
  current primitives where possible; select no encoding through this outline.

### Open Questions

- What concrete resource matrix expresses own-resource access and explicit
  parent delegation under (720), including tracker/wiki stores and note
  layers, without broadening (721)'s authoring-only powers?
- How should new native/discussion forks carry clerk identity and parent
  lineage under (722), including nesting/resume, while preserving all
  existing sessions unchanged? Wire/store/capability encoding remains open.
- What mailbox presence, display and ownership behavior is needed beyond
  protected parent ownership and existing reply channels? No mailbox opt-in
  or parent-inbox access is implied by authoring equivalence.
- Can a chosen design preserve old clients/keys/gates additively? If not,
  identify the exact T2 surface and return it for owner approval before an
  implementation ticket is made executable.

### Rejected Alternatives

- Owner (705)=b rejects the narrow (676) repair as the selected path: merely
  threading the parent does not supply the missing lead-equivalent authoring
  role and can trigger existing non-top-level gates.
- (720)=c rejects own-only and unrestricted parent-resource access in favor
  of own resources plus explicitly delegated parent resources.
- (721)=a rejects session creation/management/global-config powers for clerk.
  (722)=c rejects changing all existing forks or an opt-in-only mapping in
  favor of new forks becoming clerks while existing sessions stay unchanged.
- The old fork exemption remains historical compatibility evidence, not a
  reason to ignore (722)'s new-fork intent. Promoting a clerk to top-level or
  silently choosing a new protocol/store role is not authorized.
