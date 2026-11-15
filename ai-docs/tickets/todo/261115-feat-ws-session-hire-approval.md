---
title: "Owner-approved nonblocking hosted-session hiring"
related:
  261114-research-ws-session-hire-approval: "source research; its Confirmed Decisions are this ticket's contract"
  261114-research-ws-clerk-session-role: "clerk decisions 720-722 stay unchanged"
---

# Owner-approved nonblocking hosted-session hiring

## Background

A lead should be able to ask for another daemon-hosted session without the
owner creating it by hand. The lead sends a nonblocking request. The
dashboard shows the owner an approval modal prefilled from that request; the
owner adjusts the launch settings and approves or rejects. The outcome
reaches the requester by mailbox. An approved session is treated exactly as
if the owner had created it from the rail. It is shown under the requester,
and the requester receives its notices, but the requester gains no control
over it.

The research ticket `261114-research-ws-session-hire-approval` settled the
contract. The owner moved it to implementation on 2026-11-15 ("구현으로
올리죠", relayed by MAIN). Only that research's Outcome Ledger
`Confirmed Decisions` are carried here as contract. Its Proposals and Open
Questions are not contract; each one this ticket needs is in the Open
Decision Queue below.

## Decisions

Carried from the research's Confirmed Decisions (IDs are the owner's):

- **750 / 751 - hiring root flag.** Add a root-session option to the session
  settings UI. Only the owner turns it on, through the UI, and the server
  verifies the call. The model can never set it. Only a session with the flag
  on may request hiring; the server refuses every other caller. A flagged root
  cannot become another session's child.
- **750 - nonblocking.** A request returns at once. Accepting a request does
  not mean a session exists. The creation, rejection or expiry outcome arrives
  later by mail.
- **750 - approval modal.** The modal names the requester. The owner can
  adjust harness, model and effort with the existing new-session controls,
  then approve, or reject with a reason.
- **752 - no chaining.** A hired session is never a root, holds no hiring
  entitlement, and cannot hire.
- **753 - compatibility.** Target **T0 additive**: new MCP tools and optional
  fields only. No clerk encoding, no change to existing identity or gate
  semantics, no shared-store rewrite, and no T2.
- **765 - trust boundary.** Approval, rejection and flag changes use the
  existing trusted dashboard control gate (loopback plus exact Origin). It does
  not prove human provenance; that stays deferred.
- **766 / 770 - request text.** `reason` is the owner-facing explanation.
  `recovery_context` is longer text that lets the requester recover, after
  compaction, why it asked. Approve, reject and expiry mail echo
  `recovery_context`. Requested harness, model and effort only prefill the
  modal; the owner's adjusted values are final.
- **767 - expiry.** No cap on pending requests. A pending request expires
  after 30 minutes, and the requester gets an `expired` mail.
- **768=c / 775=a - authority.** An approved session has exactly the
  authority of an owner-UI-created session. The relationship is display
  grouping under the requester in the rail plus routing of
  approval/creation notices to the requester. It is not MCP authority
  lineage. The requester cannot stop or close it. Its root flag cannot be
  turned on while it is grouped under a requester. It does not stop when the
  requester closes. Rejected: authority lineage (775 option b), requester
  stop/close, requester-bounded authority.
- **769=a - CLI leads.** The owner can also enroll a non-hosted CLI lead,
  through the same toggle on that session's rail row.
- **786=ok - roster.** Add a read-only roster tool. Each row holds exactly:
  mailbox address, harness, model, label, idle/busy state, running subagent
  count and last activity. T0 additive. It grants no control or write
  authority.
- **788=d - strengths.** No strengths field anywhere. The owner tells the
  lead verbally, and the lead keeps it in its own agenda.
- **782=a - allocation.** The lead gives new work to the least-loaded peer and
  rebalances when a peer has more than three unstarted items. This is lead
  behavior guidance; it confers no stop/close powers.

Settled 2026-11-15 by MAIN (board IDs; MAIN relay 05:43Z):

- **(952) technical, MAIN - tool exposure.** The request tool is listed in
  every lead-scope session's tool list. The server refuses callers without the
  hiring flag ("hiring is not enabled for this session"). This keeps the tool
  list static and T0. Rejected: listing it only for flagged roots, which needs
  a tools/list refresh on every flag change (T1).
- **(952) technical, MAIN - roster callers.** Any lead-scope session may call
  the roster. Its rows are read-only and already visible in the dashboard.
  Rejected: flagged roots only.
- **(952) technical, MAIN - names.** `session.request_hire` and
  `session.roster`, next to the existing `session.children`. Rejected:
  `daemon.request_hosted_session` / `daemon.list_children_sessions` (the
  research examples; "children" misdescribes a roster that includes peers),
  `daemon.request_session` / `daemon.list_sessions`. Neither selected name
  exists yet (`rg -n 'session\.(request_hire|roster)' agents-plugin*/`
  excluding generated/dependency directories returns no matches).
  `session.children` is defined/dispatched in
  `agents-plugin-tool/internal/mcp/server.go#L828-L829,#L2513-L2584,#L4444-L4460`;
  it enumerates key descendants, not display groups. `mailbox.lookup_peers`
  is defined in that file at #L5233 and implemented in
  `agents-plugin-tool/internal/mcp/mailbox_tools.go#L329-L473`; it lists
  named presence, self and the caller's correspondents, not a load roster.
- **(952) technical, MAIN - restart.** Pending proposals persist in a new
  file in the daemon state directory (T0). After a restart they reappear and
  keep their 30-minute wall-clock expiry. Any that expired while the daemon
  was down send their `expired` mail. Rejected: memory only, which drops
  requests silently and breaks the 750/767 mail promise.
- **(952) technical, MAIN - recheck.** Eligibility is checked again at
  approval. If the requester has lost the hiring flag, the request cannot be
  approved; it is closed with a `withdrawn` mail. A closed requester does not
  block approval: the outcome mail waits on its reply-id (768=c). Rejected:
  withdrawing on any requester change; no recheck.
- **(952) technical, MAIN - recovery_context size.** At most 16 KiB. It is
  stored with the proposal, shown collapsed in the modal with a character
  count, and echoed in full in every outcome mail. Rejected: 4 KiB;
  unlimited.
- **(952) technical, MAIN - no tier.** The request carries optional
  `harness`, `model` and `effort`. Missing values fall back to the
  new-session popover's defaults. Rejected: a tier word resolved through
  agent bindings, which would be a second resolution path to explain.
- **(952) technical, MAIN - grouping field: SUPERSEDED** by the owner's
  05:48Z directive and (954) below. No new grouping field or `hired_by`
  record is added. Runner `ParentRef` (`agents-plugin-tool/internal/wsagent/record.go#L100-L105,#L221-L239`)
  and MCP `sessionEntry.parent` (`agents-plugin-tool/internal/mcp/session_auth.go#L19-L31`)
  are still not reused: they carry turn-report routing and authority lineage,
  which 775 excludes.
- **(952) technical, MAIN - trusted gate.** The approve, reject and
  flag-toggle endpoints use the existing trusted gate in
  `agents-plugin-tool/internal/wsagent/http.go` (`opts.Trusted`: local plus
  own-origin), which is 765's boundary. The actual call is
  `r.opts.Trusted(req)` at #L127-L146; its default is
  `wsdashboard.TrustedWrite` (`agents-plugin-tool/internal/wsagent/runner.go#L229-L230`),
  checking loopback peer plus loopback Host and the daemon's own Origin
  (`agents-plugin-tool/internal/wsdashboard/access.go#L157-L169`;
  `agents-plugin-tool/internal/wsdashboard/inbox_routes.go#L148-L160`).
  Rejected: a new auth path.
- **(952) technical, MAIN - outcome mail.** Outcomes use the existing reply-id
  system mail (`agents-plugin-tool/internal/wsmailbox/replyid.go#L150-L165`),
  which can queue an outcome before any child exists. `PostSystem` only
  appends the senderless envelope; its caller must wake the waits, as
  `PiTurnReporter` does with `wswake.Mailbox.Wake()`
  (`agents-plugin-tool/internal/mcp/pi_tools.go#L497-L519`). Env-less
  reply-channel publication/opening is a separate existing mechanism
  (`agents-plugin-tool/internal/mcp/mailbox_tools.go#L439-L448`); the unread
  piggyback checks `MailboxReplyOpened` (#L566-L613), so posting alone is
  not proof of requester wake/delivery. Rejected: a new notification channel.
- **(952) technical, MAIN - creation path.** Approval calls the existing
  hosted-lead creation (`Runner.CreateLead`) with the owner's final values,
  so the result matches a rail-created session (768=c). CreateLead mints a
  parent-less lead key, so the hired session is a top-level lead in MCP
  terms. 775 wants exactly that; the session still never gets the hiring
  flag (752). The rail POST calls `CreateLead` in
  `agents-plugin-tool/internal/wsagent/http.go#L148-L164`; creation delegates
  to `LeadKeys.MintLead` in
  `agents-plugin-tool/internal/wsagent/runner.go#L549-L568,#L607-L612` and
  `AgentKeyStore.MintLead` writes `roleLead` with an empty parent
  (`agents-plugin-tool/internal/mcp/agent_keys.go#L32-L43`).
  Rejected: a separate creation path.
- **(952) technical, MAIN - CLI flag storage.** The CLI-lead flag (769=a) is
  an optional field next to the dashboard's existing per-session record for
  that lead, not in the lead-key registry. The registry is authority state,
  and a flag change must not touch it. The existing CLI dashboard row is
  `<daemon-state-dir>/sessions/<reply-id>/registration.json`
  (`agents-plugin-tool/internal/wsdashboard/registry.go#L32-L34,#L75-L140,#L198-L200,#L227-L230`),
  normally `~/.cache/ws@kang-sw-devenv/dashboard/sessions/<reply-id>/registration.json`
  (`agents-plugin-tool/internal/wsdashboard/endpoint.go#L37,#L61-L70`;
  `agents-plugin-tool/internal/wsstate/paths.go#L90-L101`), not a hosted
  `agent.json`. Dashboard-owned per-session metadata already
  lives alongside it as `label.json`
  (`agents-plugin-tool/internal/wsdashboard/label_route.go#L17-L45,#L91-L105`).
  The flag store is settled by (957): the MCP process caches and
  rewrites the typed registration during registration, heartbeat and end
  (`agents-plugin-tool/internal/mcp/dashboard_sessions.go#L327-L375,#L411-L470`),
  while `Registration` ignores rather than preserves unknown JSON fields
  (`agents-plugin-tool/internal/wsdashboard/registry.go#L236-L256,#L282-L315`).
  A daemon-only flag added to that row can therefore be lost on an older
  or stale writer. Resolved by (957) below: the flag lives in a
  dashboard-owned sidecar.
- **(952) technical, MAIN - phase split.** Phase 1 covers the server
  lifecycle and tools, Phase 2 the dashboard UI, Phase 3 shipped guidance.
  Phase 2 edits files under the split freeze, so Phase 1 can land first.
  Rejected: one phase.
- **(953) announced - no active limit.** There is no limit on active hired
  sessions; owner approval already gates each creation (767 removed only the
  pending cap). Rejected: a per-requester cap.
- **(953) announced - no turn reports.** The hired session's turn reports do
  not go to the requester. The requester gets only creation, rejection,
  expiry and withdrawal mail; otherwise the two talk through normal mailbox.
  This follows 775's "approval/creation notices". Rejected: routing every turn
  report to the requester like a spawned child, which would make it a
  supervisor.

Owner answers 2026-11-15 (MAIN relays 05:47Z and 05:48Z) and the items they
raised:

- **(948)=d, owner - roster scope = the daemon's session hierarchy.** "지금
  세션 리스트는 사용자가 끌어다가 다른 세션 밑에 놓을 수 있잖아요? 그거대로,
  "자기 밑에 있는 세션"으로 한정할 수 있지 않을까 싶습니다." and "그리고
  로스터의 경우에도 별도의 새 상태를 추가하는 게 아니라, 데몬의 세션 위계를
  그대로 쓰는 걸로 하죠. 즉, 사용자가 자식을 끌어다 루트에 놔버리면 roster에
  보이지 않는 상태가 된다는 맥락입니다." The roster lists exactly the
  sessions nested under the caller in the dashboard rail hierarchy, the tree
  the owner edits by drag-to-nest. A session the owner drags to the top level
  drops off its former parent's roster. This is the owner's chosen scope even
  though it narrows 782: least-loaded assignment sees only the caller's nested
  sessions. Rejected: hired children plus same-project leads (the earlier
  recommendation), the whole daemon.
- **(949)=c, owner - same worktree.** "리드와 같은 워크트리로 둡니다." The
  hired session runs in the requester's worktree. Whether the owner may still
  change it in the modal is not decided; the arrival-flow mockup shows it
  read-only and the owner judges it under (951). Rejected: the requester's
  canonical git root as an editable default; a request-named root.
- **(950)=a, owner - nonblocking arrival.** "이건 목업 좀 하죠." A request
  does not steal focus. It shows a chip on the requester's rail row, a
  top-bar badge and a short toast; the badge or a chip opens one queue of
  pending requests (oldest first), and the modal opens from the queue with a
  pager. Mockup: `ai-docs/ref/brand/mockup/session-hire/arrival-flow.png`
  (source `arrival-flow.html`), judged under (951). Rejected: a modal that
  pops up on arrival.
- **(952)=ok, (953)=ok, owner.**
- **(954) technical, MAIN-accepted 05:50Z - grouping = the rail hierarchy.**
  Supersedes the grouping-field part of (952). Drag-to-nest writes a
  dashboard-only tree: `POST /rail/layout` attach ops update
  `rail-layout.json` in the daemon directory,
  `workspaces[<ws>].children[<parent reply-id>]`, one level deep and within one
  workspace (`agents-plugin-tool/internal/wsdashboard/rail_layout.go`,
  `rail_layout_route.go`). Approval places the new session under the requester
  with that same attach op. The requester's reply-id is kept only in the
  proposal, for outcome mail. Only rail rendering reads the tree (wsdashboard
  `rail_layout*.go`, `pageview_rail.go` `orderRailView`, `daemon.go`);
  nothing in wsagent, mcp or wsmailbox reads it. Rail nesting therefore
  carries visibility only: no report routing, no authority lineage, no
  stop/close. 775 stays intact. Rejected: a separate `hired_by` field, which
  would disagree with the rail once the owner drags the session out.
- **(955) announced, from 750 and 775 - nesting and the flag exclude each
  other.** A session with the hiring flag cannot be attached under another
  session: the layout route refuses with a new reason code (T0 additive). The
  flag cannot be turned on for a session nested under another. Because the
  hierarchy is one level deep, a nested session never has children of its
  own.
- **(957) technical, auto-confirmed from fact population - flag storage.**
  The hiring flag, for hosted and CLI sessions alike, lives in a
  dashboard-owned sidecar beside `label.json` in
  `<daemon-state-dir>/sessions/<reply-id>/`, written only by the trusted
  dashboard route, following the `label_route.go` pattern. Rejected: a field
  in `registration.json`, which the MCP process rewrites without preserving
  unknown fields, so older or stale writers would drop it; the hosted
  `agent.json` alone, which would give CLI leads (769=a) a second store.
- **(958) technical, auto-confirmed from fact population - roster telemetry.**
  Roster rows reuse the rail row's projection, because the roster's scope is
  the rail tree:
  - busy = `leadWorking`, which excludes idle and permission-wait;
  - running subagents = `levelOneRunning`, direct children only;
  - last activity = the row's `LastTool`, shown as unknown when it is 0;
  - model = the observed model `sessionHover` reads, unknown when it is
    missing or stale, never the requested launch value;
  - an ended or adapter-less session reports not busy and 0 subagents.

  The daemon serves the rows from a new read-only route that
  `session.roster` proxies (T0). Rejected: the index view's projection, which
  counts permission waits as busy and recurses into subagents, so it would
  disagree with the rail the owner sees.

## Open Decision Queue

Last ID: (958)

Local IDs from the first draft were re-minted on the board: (939) -> (948), (947) -> (949), (948) -> (950), (956) -> (951).

(951) [open] [policy] Accept the mockups as the Phase 2 visual reference, including whether the worktree field stays read-only
- Mockups: `ai-docs/ref/brand/mockup/session-hire/approval-modal.png` (modal fields, the authority note, the root toggle and its disabled state) and `ai-docs/ref/brand/mockup/session-hire/arrival-flow.png` (chip, badge, toast, queue of three, modal with pager and a read-only worktree). The arrival-flow frame C supersedes the first mockup's editable root.
- Alternative a: accept both as is (worktree read-only).
- Alternative b: accept with changes the owner names (for example an editable worktree).
> Recommend a: (949)=c already fixes the worktree, and a read-only field keeps the modal honest about it.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
  Tool descriptions, refusal text and playbook text must make sense in a
  downstream project.
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)
  Every new tool, argument, record field and state file is T0 additive.
  Anything that turns out not to be T0 goes back to the lead before it lands.
- Convention: ai-docs/manuals/skill-authoring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin-wsflow/rsrc/`, `agents-plugin-wsflow/skills/`, `agents-plugin-tool/internal/wsdoc/conventions/`)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin/hooks/`, `agents-plugin-wsflow/`)
  Both apply to any playbook or skill text (Phase 3).
- Clerk decisions 720-722 and existing worker delegation are unchanged.
- Dashboard strings go through the i18n tables for all 11 locales.
- `dashboard.js` and `dashboard.css` are under a split freeze until the split
  ticket lands. Phase 2 targets the post-split files (pending
  `261114-refactor-ws-dashboard-whiteboard-asset-split`, currently ready
  without a Result or published move map in this checkout); existing reuse
  symbols below are pre-split anchors, not settled post-split filenames.

## Prior Decisions

- 261114-research-ws-session-hire-approval (2026-11-14, Confirmed Decisions): "The hired-child relation is grouping and reply routing only" — bearing: constrains
- 52dc582b (2026-11-13, commit): "T0: plugin_isolation is additive (default|isolated), with absent legacy values treated as default" — bearing: constrains
- 261113-bug-ws-pi-completion-mail-sender-unknown / ca701289 (2026-11-13, commit): "Add optional display-only envelope.label; preserve From and ReplyTo as addresses so Pi reports never become mailbox correspondents." — bearing: constrains
- 261112-feat-ws-mailbox-undelivered-notice / c9ae4d75 (2026-11-13, commit): "queued mail is the exact unread signal; no liveness heuristic or claim hook is needed." — bearing: supports
- 261112-feat-ws-mcp-pi-spawn-claude-lead-path / 7f40e811 (2026-11-13, commit): "The parent reference is a typed {kind, reply_id} so a later Pi-parent path can reuse the record and report hook (the recursion ticket)." — bearing: constrains
- 261110-feat-ws-dashboard-workspaces / 907d3b85 (2026-11-12, commit): "A terminal ws-mcp that outlived its harness re-reads the row under the lock before each write and stops once the row is hosted" — bearing: constrains
- 261110-feat-ws-dashboard-headless-lead-sessions / 72c9d851 (2026-11-11, commit): "The lead row is written with agent:false so the mcp session source serves its board, contacts, and mail history like any lead's" — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | Phase 1 spans agents-plugin-tool/internal/wsagent/{http,runner,record}.go, internal/mcp/{server,session_auth,mailbox_tools,dashboard_sessions}.go and internal/wsmailbox/replyid.go; Phase 2 spans internal/wsdashboard and internal/wsi18n; Phase 3 reaches shared agents-plugin/rsrc/ and its wsflow/Pi copies. |
| scope.surface | cross-module | New public MCP tools session.request_hire and session.roster, trusted HTTP controls, persisted proposals/optional record fields, dashboard UI and shipped lead guidance cross the MCP/runner/dashboard/mailbox boundary. |
| scope.new_public_symbol | yes | session.request_hire and session.roster are selected new tool names; source search finds neither registered today. Existing adjacent definitions are agents-plugin-tool/internal/mcp/server.go#L4444-L4460,#L5233. |
| scope.new_type_contract | yes | Request arguments, seven-field roster rows, proposal lifecycle persistence and the hiring-flag sidecar, a rail-layout refusal code and a roster route are additive serialized contracts; exact field/file shapes are the worker's, within (957)/(958). |
| scope.test_surface | existing | Existing extension points: agents-plugin-tool/internal/mcp/{session_auth_test,hosted_session_test,mailbox_tools_test,pi_tools_test,playbook_tools_test}.go; internal/wsagent/record_test.go; internal/wsdashboard/{index_test,railscript_test,label_route_test,pageview_hover_test}.go; internal/wsi18n/keys_test.go; internal/wsrsrc/{pi_mirror_test,wsflow_mirror_test,skills_manifest_test}.go. No new test filenames are specified. |
| complexity.reuse_points | confirmed | Read Runner.CreateLead, TrustedWrite, ParentRef/sessionEntry.parent, PostSystem plus PiTurnReporter wake, CLI registration/label sidecar, rail/index telemetry, leadRows/leadChoices/rpLeadMake and WsRail.leadBody. Current anchors are pre-split; no complete hiring lifecycle or shared harness-selector component exists. |
| complexity.side_effect_risk | high | Approval launches a real hosted lead; trusted flag writes, durable restart/expiry handling and asynchronous mailbox outcomes must remain correlated without granting requester control. |
| risk.correctness | high | Eligibility rechecks, exactly-once creation/outcomes under repeated approval or restart, stale CLI writers and differing telemetry definitions can violate explicit no-chaining/mail/roster guarantees. |
| risk.fit | moderate | Existing launch/control/mail/UI seams fit the approved reuse direction, but CLI metadata ownership and post-split UI targets still need settlement; reuse must not turn display grouping into authority lineage. |
| risk.test | high | New asynchronous persistence/expiry/recheck coverage is not supplied by existing fixtures; clerk encoding is unselected, telemetry absence varies by harness and the split move map is not landed. |
| risk.security_or_contract | high | Owner-only flag/approval, lead versus non-lead gates and no requester stop/close authority cross trust boundaries; T0 requires flag persistence survive older/stale writers without a key-registry or schema reinterpretation. |

## Phases

### Phase 1: Daemon proposal lifecycle, hiring flag, and the request and roster tools

- Hiring flag: a dashboard-owned sidecar per session row, hosted or CLI
  ((957)). It is set only through a trusted dashboard endpoint (trusted gate).
  It cannot be set for a session nested under another (775, (955)), and never
  for a hired session (752). The rail layout route refuses to attach a flagged
  session under another ((955)).
- `session.request_hire`: takes `reason` and `recovery_context` (both
  required; `recovery_context` at most 16 KiB), an optional label, and
  optional `harness`/`model`/`effort` (no tier). The tool is listed for
  every lead-scope session. It refuses callers without the flag (750/751),
  hired sessions (752), and non-lead scopes. It returns at once with a request
  id and a line saying the outcome will arrive by mail (750).
- Proposal lifecycle: pending -> approved | rejected | expired (30 min,
  767) | withdrawn (recheck). Pending proposals persist across a daemon
  restart (restart). Eligibility is checked again at approval (recheck).
- Approval creates the session through `Runner.CreateLead` (creation path)
  with the owner's final values, in the requester's worktree ((949)). It
  attaches the new session under the requester in the rail hierarchy
  ((954)) and sends the outcome by reply-id system mail
  (outcome mail). Every outcome mail echoes `recovery_context` (766/770). No
  turn reports go to the requester (no turn reports).
- `session.roster`: read-only rows with exactly the 786 fields, for the
  sessions nested under the caller in the rail hierarchy ((948), (954)). It
  reads that hierarchy for visibility only and is never an authority or
  routing source. Any lead-scope caller (roster callers). Telemetry per
  (958), served by a new read-only daemon route. Background evidence on the
  differing existing projections: `buildIndexView` treats any non-idle phase
  as busy and counts every running snapshot agent
  (`agents-plugin-tool/internal/wsdashboard/pageview.go#L1047-L1097`), while
  `railRow` uses `leadWorking` (excluding empty, idle and permission-wait
  phases) and `levelOneRunning` (direct children only)
  (`agents-plugin-tool/internal/wsdashboard/pageview_rail.go#L558-L590,#L624-L665`).
  Recursive direct/deep counts exist in
  `agents-plugin-tool/internal/wsdashboard/pageview.go#L386-L437`.
  `Registration.LastSeen` is max(registered, heartbeat, ended), `LastCallAt`
  is keyed MCP activity, `lastToolAt` is the last tool batch, and snapshot
  `UpdatedAt` includes heartbeats
  (`agents-plugin-tool/internal/wsdashboard/registry.go#L135-L139,#L216-L224`;
  `agents-plugin-tool/internal/wsdashboard/pageview_rail.go#L631-L655`;
  `agents-plugin-tool/internal/wsdashboard/index.go#L30-L32`). Busy semantics,
  count depth, last-activity source and absent/stale-adapter representation
  are settled by (958). Observed model evidence is already read by
  `sessionHover`, with missing/stale provenance rather than substituting
  requested launch settings
  (`agents-plugin-tool/internal/wsdashboard/pageview_hover.go#L10-L52`).
- Verification: Go tests for:
  - flag gating: unflagged, hired, worker and clerk callers are refused, and
    the model cannot set the flag;
  - each lifecycle edge with its mail, including the `recovery_context` echo
    and expiry on a fake clock;
  - restart persistence and expiry-while-down mail;
  - the approval recheck (flag lost -> `withdrawn`; requester closed ->
    still approvable);
  - the requester's inability to stop or close the hired session;
  - the roster's fields and scope;
  - compatibility: the existing tool list and records are unchanged apart
    from the additions.

  Use focused `-run` selections from `agents-plugin-tool/`. The existing
  MCP CI shard uses `go test -count=1 -timeout 30m -run "$pattern" ./internal/mcp`
  (`agents-plugin-tool/scripts/ci-test-shard.sh#L33-L48`); this is a configured
  shard budget, not evidence that a full package run requires or finishes
  within 30 minutes.

### Phase 2: Dashboard approval modal, rail cues, and the root toggle

- Build to the accepted mockups ((951)):
  `ai-docs/ref/brand/mockup/session-hire/approval-modal.png` and
  `ai-docs/ref/brand/mockup/session-hire/arrival-flow.png`.
- The root toggle goes in hosted-session settings and on CLI lead rail rows
  (769=a). It is disabled, with a reason, for a grouped session (775).
- Rail and arrival ((950)): a hire chip on the requester's row, a top-bar
  badge, a short toast, and pending rows while requests are open. The badge
  or a chip opens one queue (oldest first); the modal opens from it with a
  pager. Nothing steals focus. After approval the hired session shows nested
  under the requester ((954)).
- Modal contents:
  - requester identity and reason;
  - recovery context, collapsed, with a character count;
  - harness/model/effort controls reusing the new-session popover's controls;
    current reuse seams are `leadChoices`, `leadOptions`, `leadRows` and
    `rpLeadMake` in
    `agents-plugin-tool/internal/wsdashboard/assets/dashboard.js#L8243-L8433`.
    `leadRows` already shares model/permission/effort rows with hosted settings;
    the harness segment is inline in `rpLeadMake`, not a standalone control.
    Defaults are Claude (Pi if Claude unavailable), empty model/effort,
    permission mode `auto` and plugin isolation `default`. The request body
    helper is `WsRail.leadBody`
    (`agents-plugin-tool/internal/wsdashboard/assets/rail.js#L609-L619`).
    These establish reusable controls/choices, not an existing hiring modal;
  - label, and the requester's worktree ((949); read-only unless (951) says
    otherwise);
  - expiry countdown;
  - reject reason, Reject, and Approve & create.

  Approve and reject go through the trusted endpoints (trusted gate).
- Strings in all 11 locales.
- Verification:
  - node harness tests for the modal: prefill, owner edits win, reject with
    or without a reason, countdown, disabled toggle;
  - the focused wsdashboard Go tests and wsi18n;
  - a side-by-side comparison of a headless capture against the mockup PNG.

### Phase 3: Shipped lead guidance

- Lead-facing guidance in the workflow manual or playbook covers:
  - when to hire;
  - that the request is nonblocking and the outcome arrives by mail;
  - that `recovery_context` must be enough to resume after compaction;
  - the roster and 782 allocation;
  - keeping strengths in the agenda (788=d).

  Wording stays downstream-neutral and is mirrored to wsflow and Pi through
  the existing pipelines.
- Verification: manifest/mirror tests and product-mode playbook checks.
