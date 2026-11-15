---
title: "Worker-profile sessions: assigned tasks, mailbox reports, and owner-approved creation"
blocked-by: 261115-feat-ws-permission-profile#phase2
related:
  261115-feat-ws-permission-profile: "Phase 2 supplies reusable permission controls/backend and existing-worker admission; worker Phase 1 composes its new posture/routes/dialog afterward"
  261115-feat-ws-session-hire-approval: "Phase 2 consumes hiring Phases 1–2; add profile lead | worker while preserving other hiring contracts"
  261114-research-ws-clerk-session-role: "contrast only: clerk authoring parity is not this worker profile's authority"
  261114-refactor-ws-common-session-interface: "owner 1050=a permits bounded T0 Runner controls first; its Phase 2 later absorbs these controls"
  261114-epic-ws-common-session-foundation: "preserve foundation ordering; no whole-epic dependency"
  261114-feat-ws-hosted-antigravity-acp-adapter: "deferred adapter; no Antigravity implementation or runtime claim here"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 6936cf7330d939f6
sage-review-completeness-reviewed: 6936cf7330d939f6
---

# Worker-profile sessions

## Background

An owner-created session should be able to act as a bounded worker instead
of a workflow lead: execute its assigned task and report by mailbox, with a
short worker manual and no ticket/run/review orchestration or session-creation
powers. The owner selects the owning lead in the new-session dialog. An
externally started session can enter the same worker posture through one
bootstrap command. The session-hire approval flow also offers the worker
profile, with the owner's final choice controlling creation.

Authority: owner (1009)–(1014) on board `worker-session-profile`, relayed by
MAIN on 2026-11-15 at 07:02:38, and (1017)=a at 07:04:53.

## Decisions

1. **(1009)=a — worker posture.** One user-facing worker profile gets a
   short manual: execute the assigned task and report results by mail.
   Ticket/run/review orchestration and session spawning are blocked;
   subagents are limited to `explore` + `general-purpose`.
2. **(1010)=a — owning lead and address.** The user selects a reachable
   owning lead in the new-session dialog. Automatically mail that lead the
   worker address and editable one-line intro; default the intro to `worker`.
   The session page offers copy-address. The selected lead is an introduction
   recipient, not an authority parent or automatic turn-report destination.
3. **(1011)=a — external entry.** Add one worker bootstrap command alongside
   external-lead `ferrule` guidance. It returns the worker key, short manual
   and reachable mailbox address. Reuse a hosted worker's bound non-lead key.
   An existing live lead is never silently downgraded or rekeyed: return an
   explicit migration-needed result when identity cannot safely be preserved.
4. **(1012)=ok — subagents.** Enforce the two allowed categories only where
   demonstrated on that harness. Otherwise use a one-line nudge plus the
   manual rule, describing the restriction as advisory rather than enforced.
5. **(1013)=ok — bounded authority.** Reuse delegate/non-lead key scope plus
   an explicit MCP allowlist. The delegate gate alone admits ticket tools and
   is insufficient. Existing sessions, lead bootstrap and role meanings stay
   unchanged; this is not the mini-lead or clerk profile.
6. **(1014)=ok — separate permission dimension.** Consume the permission
   sibling's `permission_level: read-only | workdir-write | full`, alongside
   native `permission_mode`. Sibling-owned computed State fields are
   `permission_level` and `permission_enforcement: enforced | partial | advisory`.
   Preserve Claude
   `auto` and fixed worker tool-list defaults; never write derived display
   back. Only the user raises a level, with a warning before explicit `full`;
   anyone authorized to control the target may lower it. Agents cannot raise
   themselves or children. Every child launch, including revive/restart,
   checks the actual parent's CURRENT level, not a persisted ceiling. An
   actual child whose current parent cannot be resolved gets a visible
   parent-admission-pending/error, not a guessed ceiling; a root with no
   parent is distinct. This is admission under (1000), not unenforceability
   under (999). A selected owning lead or hire display group creates no parent.
   Unenforceable intent permits launch with the sibling's one-line nudge;
   this never permits a worker-to-lead authority fallback. Authority for
   these bounds is MAIN's 07:45:14 clarification of (998)–(1004).
7. **(1017)=a — hiring.** `session.request_hire` carries optional
   `profile: lead | worker`; omission preserves lead behavior. The approval
   modal shows it and the user can change it. Persist both requested and
   approved values in the hire proposal; creation uses the approved value.
   A worker result requires the same selected reachable owning lead as direct
   creation. Existing hiring eligibility, approval and no-chaining rules stay.

8. **(1050)=a, MAIN 07:58:13 — bounded Runner exception.** The owner permits
   permission-level and worker/lead controls before
   `261114-refactor-ws-common-session-interface#phase2`, using the current
   Runner with T0 changes only. Keys, lineage and input/mail ownership remain
   unchanged. That foundation Phase 2 later absorbs these controls. This
   exception does not authorize a new authority model or another backend.

### Technical contract

- Store worker metadata in `<SessionDir>/worker.json`, separate from the
  sibling's `<SessionDir>/permission.json`:
  `{"profile":"worker","owning_lead":"<reachable mailbox address>","intro":"worker"}`.
  Retain unknown sidecar members on updates. Missing metadata does not convert
  an existing session to worker. Keep role enforcement in the non-lead key,
  transport admission and fixed tool surface, not just in a display label.
- Add `worker.bootstrap {root, owning_lead, intro?, session_key?}`. On a fresh
  external entry it binds a delegate key and worker-restricted MCP connection;
  on a bound worker it returns the same identity/manual/address. Validate the
  required owning-lead recipient before creation; use trimmed single-line
  intro text, defaulting blank input to `worker`. A bound live lead returns
  migration-needed without changing identity. The connection's allowlist must
  apply before dispatch even when a call omits `session_key`, and be restored
  before tool exposure on reconnect. This is a distinct worker operation, not
  a new alias for lead minting.
- MCP allowlist: `worker.bootstrap`, `mailbox.send`, `mailbox.recv`,
  `mailbox.lookup_peers`, `mailbox.history`, `git.status`, `git.diff`, `git.log`,
  `git.merge_base`, `git.commit`. Native task tools use existing fixed
  loadouts. No session, ticket/run/review, configuration-write or lead-bootstrap
  MCP tools are admitted. Native `explore`/general-purpose subagents follow
  decision 4, independently of permission intent.
- Add trusted `POST /agents/workers` with
  `{root, harness, owning_lead, intro?, model?, effort?}` for legacy/default
  permission behavior. Explicit permission selection uses the separate
  `POST /agents/workers/permission` with the same body plus `permission_level`
  and, for explicit `full`, `permission_full_ack:true`. Both reuse one worker
  creation/Runner owner and the local/own-Origin control gate. The default
  route never accepts explicit permission intent; only the `/permission`
  variant does. Keep `/agents/leads` behavior intact. Permission Phase 2
  supplies reusable controls/backend admission for existing workers; this
  ticket's Phase 1 owns these new posture creation routes and their dialog
  composition. Neither is a prerequisite of permission Phase 2. Test an
  intermediate worker-capable server without the permission variant after
  replacement: explicit selection produces zero creations and no fallback.
- Each harness entry of `GET /support/choices` gains
  `worker_profile_supported: true` only when the full worker creation path is
  available. The UI requires positive support before enabling/sending worker
  creation; explicit selection also requires the sibling's `permission_levels`
  support and uses the `/permission` variant. Refresh after replacement and
  never retry by dropping role/permission intent.
  `GET /agents/<id>` adds an optional computed `worker` object containing the
  sidecar metadata for worker-page controls. Derive the copyable address from
  the existing reply-id, rather than persisting another address field.
- Gate external bootstrap by discovered `worker.bootstrap` support. Gate hire
  `profile` by its discovered input schema and server-side worker support;
  old clients omitting it still request leads. Store `profile` and
  `approved_profile` in the hire proposal, plus final `owning_lead` and `intro`
  for approved worker creation; absent profile fields preserve lead behavior.
  The approval modal requires the owning-lead selection when worker is chosen.
  Phase 2 below produces the selection-bearing approval amendment; it is not
  a permission-sibling producer. Reject unsupported worker intent without
  creating anything or retrying as lead. Introduction mail stays separate
  from hire outcome mail and normal worker task reports.
- Add `POST /session-hires/<request-id>/approve/permission` in this ticket's
  Phase 2, through the landed hire controller and existing Runner owner. Its
  final approval payload carries `permission_level`, plus
  `permission_full_ack:true` for explicit `full`, alongside the approved
  profile/harness/model/effort and worker metadata. Preserve the existing
  default approval operation; do not add selection fields to that old URL.
  Require trusted local/own-Origin owner control, user-only raises and the
  full warning/acknowledgement. Model authority or a requester profile is
  never permission authority. Bind an accepted full acknowledgement to this
  request's approval and final selection, not a reusable agent grant.
- Each selected harness choice advertises additive
  `hire_permission_supported:true` only with that complete approval handler.
  Explicit approval requires this current signal and the selected entry in
  `permission_levels`; a worker also requires `worker_profile_supported`.
  A replacement or intermediate hire-only router must reject the new exact
  path without creating a session or advancing approval state. Never fall
  back to default approval or drop the selected intent.
- Persist optional `approved_permission_level` in the proposal, preserving
  unknown proposal members. On approved creation, write that selection to
  session `permission.json` through the sibling's service. Do not persist
  acknowledgement as a latent grant or store a parent ceiling. Recompute the
  actual current-parent admission at every launch/revive/restart. Approval
  replay must not overwrite a later authorized persistent lowering or create
  a second session.
- Permission fields, full acknowledgement and launch mapping use the sibling
  contract unchanged: `permission.json` retains the current explicit selection,
  including an authorized actor's persistent lowering; only the user raises.
  No parent ceiling or derived legacy display is persisted; State quality is
  computed. Do not add a strict `Profile`
  member, key/registration field, protocol bump or new manifest knob.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
  Worker manual, notices and tool descriptions must resolve downstream.
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)
  Worker entry must not obtain lead authority to read a manual or use mail.
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)
  Target additive T0 contracts/storage with source-backed per-field grades,
  old-write/new-reopen fixtures and unchanged old callers. Anything outside
  owner (1050)'s T0-only exception returns to the lead; no T3 is approved.
- Convention: ai-docs/manuals/skill-authoring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin-wsflow/rsrc/`, `agents-plugin-wsflow/skills/`, `agents-plugin-tool/internal/wsdoc/conventions/`)
  Keep the manual scoped to assigned tasks and mail reports.
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin/hooks/`, `agents-plugin-wsflow/`)
  Apply existing generation/mirror checks to shared manual or skill changes.
- Do not rename or reinterpret existing `permission_mode` as permission
  level. Permission-profile implementation and its exact field contract are
  sibling-owned; this ticket consumes their agreed interface.
- This ticket owns the worker-profile amendment touchpoint to ready
  `261115-feat-ws-session-hire-approval`: its approved creation currently
  always uses `CreateLead`. Preserve its approval, nonblocking outcomes,
  no-chaining and grouping-without-authority contracts; workers cannot hire.
- The owning-lead selector, hire display grouping, key authorization lineage,
  and native subagent parentage are not interchangeable. Do not silently use
  the hire requester's identity as an authorization parent.
- Clerk research (720)–(722) is not worker-profile authority; this ticket adds
  no clerk encoding, authoring parity, or existing-fork reclassification.
- Scope is current Claude/Pi worker sessions; no new harness, clerk authority,
  global permission configuration or retroactive session migration.
- Phase 1 consumes `261115-feat-ws-permission-profile#phase2`: reusable
  controls, Claude/Pi permission backend, existing-worker admission and
  lead/settings paths, following its Phase 1 contract. The permission
  producer does not consume this ticket's new worker routes, posture or UI.
  This ticket's Phase 1 composes them after that producer lands; its Phase 2
  owns the exact permission-bearing hire approval amendment and also consumes
  hire-approval Phases 1–2. Under owner (1050)=a, neither phase
  waits for common-session-interface Phase 2 or future adapters. Preserve the
  current Runner, existing key/lineage semantics and one input/mail owner;
  common-session-interface Phase 2 later absorbs these controls.

## Prior Art and Source Evidence

Source baseline: `667f0f64a`. `internal/` paths below are under
`agents-plugin-tool/`; search symbols locate targets, line ranges support facts.

| Surface | Current evidence and consequence |
|---|---|
| New-session dialog | `agents-plugin-tool/internal/wsdashboard/assets/dashboard-leads.js:73–112,120–184` (`leadRows`, `rpLeadMake`) builds harness/model/effort/Claude permission-mode/plugin-isolation controls and posts `/agents/leads`. `assets/rail.js:609–619` (`leadBody`) serializes them. There is no worker/owning-lead/intro field in these forms. |
| Creation and authority | `agents-plugin-tool/internal/wsagent/http.go:89–96,148–164` calls `Runner.CreateLead`; `runner.go:549–568,593–619` selects a lead profile and lead mint versus non-lead mint. `internal/mcp/agent_keys.go:12–29,32–43` already mints a parent-less delegate key for a non-lead conversation versus a parent-less lead key for a hosted lead. Reuse the non-lead scope, not a new role. |
| Role gate is not orchestration isolation | `agents-plugin-tool/internal/mcp/server.go:107–144,715–755,5714–5732` defines lead/delegate/leaf and refuses lead-only tools to non-leads, but delegate scope still admits many ticket/playbook tools. `internal/wsagent/builtin.go:74–100` distinguishes `pi-worker` from the ticket-capable `pi-mini-lead`. Worker orchestration exclusion needs an effective surface/manual consistent with (1009), not simply a different label. The keyed gate applies only to a supplied known key; keyless lead bootstrap stays open, while `toolAllowed` runs before that gate (`agents-plugin-tool/internal/mcp/server.go#L714-L755`). `WS_MCP_ALLOWED_TOOLS` can therefore narrow the attached server independently of role, but an external worker's transport/admission must also be covered; a manual alone is not keyless-call enforcement. |
| Manual entry | `agents-plugin-tool/internal/mcp/workflow_manual.go:324–337,391–408` guards the hosted key and renders `lead-workflow-manual`; `server.go:144` makes `workflow_manual` lead-only. Current parented lead-scope implementation workers reading that manual are not evidence that a delegate key can read it. A worker entry/manual remains to be supplied without lifting lead gates. |
| Mail reachability | `agents-plugin-tool/internal/mcp/mailbox_tools.go:439–448` opens an env-less reply-id channel through self-lookup; `internal/wsagent/runner.go:482–486` starts the daemon mail watcher only for `rec.Lead`. `internal/mcp/lead_mail.go:16–23,79–118` provides the existing watch/input-delivery seam. A non-lead hosted worker needs mail input/delivery work, not only an address label. `mailChanged` already claims through `LeadMailbox.Drain`, persists `Record.Mail`, then calls `kick` (`agents-plugin-tool/internal/wsagent/runner.go#L1400-L1431`); `Drain` accepts a known key without a lead-role check (`agents-plugin-tool/internal/mcp/lead_mail.go#L153-L170`). The watcher attachment and row-ownership/input path must be extended coherently, not duplicated. |
| Subagent controls | `agents-plugin-tool/internal/wsagent/profile.go:75–127` has built-in/ws tool allowlists, no subagent-type field. `claude.go:154–209` constructs `--tools`, `dontAsk` and allowed rules. `pi.go:154–197` builds/checks the declared tool surface. Claude's `ToolBase` extracts the tool name while `claudeAllowedRules` retains the full rule, providing a candidate `Agent(<type>)` permission-rule seam, not verified native type enforcement (`agents-plugin-tool/internal/wsagent/profile.go#L198-L205`; `agents-plugin-tool/internal/wsagent/claude.go#L148-L165,#L197-L209`). Pi has concrete adapter authority/depth/tool ceilings and trusted prompt classification (`agents-plugin-pi/src/delegation-policy.ts#L8-L37,#L86-L137`), plus general `ws-agent-spawn` and `explore` tools, but no `general-purpose`/`subagent_type` selector in the spawn schema (`agents-plugin-pi/src/spawner.ts#L4612-L4665`). These controls are not an already-wired worker-session type allowlist; mapping the two allowed categories to each harness is technical work, with manual+nudge only for the unenforceable remainder. |
| External bootstrap anchor | The existing external-lead MCP command is `ferrule(root: ...)`: `bootstrapToolName = "ferrule"`, dispatched to `handleLeadLogin` (`agents-plugin-tool/internal/mcp/server.go#L115-L123,#L864-L865`), and shipped session-setup guidance explicitly instructs it (`agents-plugin/rsrc/lead-workflow-manual/lead-workflow-manual.md#L29-L39`). Fresh lead-manual sentinel handling is an additional entry, not a missing dashboard anchor. `handleLeadLogin` already parses `capability`, including non-lead scope, but default scope is lead; known non-lead keyed callers cannot invoke ferrule, and hosted top-lead identity is fixed (`agents-plugin-tool/internal/mcp/server.go#L724-L755,#L2370-L2413`). The new adjacent one-command worker manual/address entry must respect those existing gates without first logging in as lead. |

### Directional compatibility evidence

The grades below are implementation targets against the inspected baseline,
not completed test results. Preserve the cited old behavior and verify every
new writer/read/reopen path before claiming T0.

| Surface / direction | Source-backed writer, reader and reopen consequence | Grade / verification |
|---|---|---|
| `worker.json`: `profile`, `owning_lead`, `intro` | New worker code writes this sidecar. Old Record writes replace only `agent.json` (`internal/wsagent/record.go:279–303`); registration writes/removes touch only `registration.json` (`internal/wsdashboard/registry.go:282–328`); label writes only `label.json` (`label_route.go:91–105`). Those old writes leave worker bytes intact for new reopen. Pruning can remove the whole expired session directory (`registry.go:331–385`), the existing session lifetime. | T0 storage candidate. Test all fields and retained unknown members through old writes/new reopen and stop/restart/reopen, plus existing expiry/deletion. Do not rely on this display metadata to preserve authority; the existing non-lead key and explicit tool surface remain decisive. |
| New `worker.bootstrap` and its `root`, `owning_lead`, `intro`, `session_key` arguments | Lead entry remains `ferrule` (`server.go:115–123,864–865,2354–2414`). Current unknown-tool dispatch refuses before a handler write (`server.go:2315–2316`); new clients require discovery and never fall back to lead bootstrap. Reopen reuses an existing bound worker key/sidecar, not a new role value. | T0 additive-tool target with support gating. Test unsupported old server/no write, unchanged ferrule behavior, repeat bootstrap identity and explicit migration-needed for a live lead. |
| Worker HTTP bodies: `owning_lead`, `intro`, existing root/harness/model/effort; sibling `permission_level`, `permission_full_ack` only on the selection variant | Old `/agents/leads` ignores undeclared JSON fields (`http.go:89–96,269–287`) then creates a lead (`:163`), so it is not used for worker intent. Old `POST /agents/workers` has nonempty id and empty action and returns **405** (`http.go:107–117,177–180`); `/agents/workers/permission` has action `permission` and returns **404** via the unknown-action branch (`:229–230`). Neither reaches creation. The selection variant also avoids the capability-GET/server-swap race. | T0 additive-route target: new clients gate support; old routers reject without writes, no intent-dropping retry; old lead routes/callers remain unchanged. Test exact-match/suffix routing and replacement between GET and POST, including a worker-capable server without the permission variant: zero creations, no default/lead fallback. |
| Support `worker_profile_supported`, `hire_permission_supported`; optional State `worker` object (`profile`, `owning_lead`, `intro`) | SupportHarness/Choices serialize named response members (`support.go:215–225,401–420,620–622`); current UI reads named harness/model/effort fields (`assets/dashboard-leads.js:43–59`). State is separate from Record (`runner.go:1053–1146`), emitted by `http.go:288–304`. An old UI ignores new optional members without changing existing controls or data. New UI derives the worker mailbox address from existing reply-id; new reopen reads the sidecar/recomputes the projection. | T0 transparent-response target only while all existing behavior stays intact. Missing support means unsupported, not a lead default. Do not persist these projections in snapshots, keys or registration. Sibling permission State fields retain its own per-field grade. |
| Hire request/proposal `profile`, `approved_profile`, final worker `owning_lead`, `intro`, optional `approved_permission_level` | The related ticket introduces `session.request_hire` and a new proposals file; neither exists in this baseline. Current MCP precheck refuses undeclared arguments before writes (`internal/mcp/tool_arguments.go:65–128`). New clients require discovered `profile` support; new readers map absent profile to legacy lead behavior. New proposal readers/writers must retain these fields and unknown proposal members through lifecycle updates before creating the approved profile. | T0 addition target, not a grade inherited from the hire ticket. Test request/approval/restart/old-write/new-reopen against its landed Phase 1 store, including any intervening hire-only writer. A byte-losing writer must be fixed before landing this amendment; no T2 recovery is assumed. |
| Hire selection route and `permission_level` / request-scoped `permission_full_ack` | No `/session-hires` handler or selected-name collision exists in this baseline. Current mux registers `/agents` and `/agents/`, not `/session-hires` (`internal/wsdashboard/daemon.go:725–742`); unmatched paths reach `handleIndex`, which rejects every path except `/` (`pages.go:1133–1139`), so this baseline returns 404 without a hire write. The future hire-only router does not exist here; pin its landed Phase 1 commit and verify exact/suffix dispatch before implementing the amendment. | T0 additive-route target with gated support, not proof about an invented legacy hire endpoint. Test a swap after support GET to both the pinned old router and an intermediate hire-only router: zero creations and zero approval transitions, no prefix acceptance/fallback. Proposal new-read/old-write/new-reopen must retain approved intent; session reopen uses the permission sidecar and current parent, not a persisted ceiling or reusable ack. |
| Rejected placement in Profile, key, registration; existing tolerant Record | `Profile.ParseProfile` rejects unknown fields (`profile.go:219–238`), a T3 read boundary if extended; `Record.resolved_definition` is typed Profile (`record.go:35,157–162`). Key/registration writers serialize fixed typed shapes (`internal/mcp/session_auth.go:45–93`; `session_layout.go:215–235,302–323`; `registry.go:236–254,282–316`). Record separately preserves unknown TOP-LEVEL members (`record.go:149–213,279–303`), not nested unknown Profile members. | No worker fields are added to these shapes. Fixed-shape loss has no demonstrated reopen reconstruction and is not T2. Record top-level tolerance remains valid evidence, but is not needed for the selected sidecar. |

## Prior Decisions

- Owner (1050)=a, MAIN relay 2026-11-15 07:58:13: bounded current-Runner permission/worker/lead controls may precede common-session-interface Phase 2, T0 only, with keys/lineage/input ownership unchanged; Phase 2 later absorbs the controls — bearing: supersedes that prerequisite for this bounded slice only.
- 261115-feat-ws-session-hire-approval (2026-11-15, Decisions): "The hired session's turn reports do not go to the requester." — bearing: constrains
- 261113-feat-ws-pi-mini-lead-profile (2026-11-13, Decisions): "Keep builtin `pi-worker` unchanged; add builtin `pi-mini-lead`; `pi.spawn` gains an optional `profile` argument." — bearing: constrains
- 261110-feat-ws-dashboard-workspaces / 6a991d90 (2026-11-12, commit): "The guarded step is a file lock on registration.json.lock taken by both the login guard and the runner" — bearing: constrains
- 261110-feat-ws-hosted-session-mode-flag / ff312f48 (2026-11-11, commit): "The hosted top-lead bootstrap behaves as a re-login with the hosted key (same validation, owner rebind, response shape) and never falls back to minting" — bearing: constrains
- 261007-bug-pi-session-key-reissued-on-resume / e0839823 (2026-10-07, commit): "A re-login combined with parent_session_key or a non-lead capability is refused rather than ignored" — bearing: constrains
- 260926-bug-pi-execute-worker-steals-mailbox-owner / 163552b7 (2026-09-26, commit): "Omitted capability still resolves to lead via parseCapabilityScope" — bearing: constrains
- 260913-feat-cross-session-mailbox-core (2026-09-13, Result): "Self-address surface and `lookup_peers`'s self entry are gated by the same `mailboxOwnerCheck` used for send/recv" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/{wsagent,mcp,wsdashboard,wsmailbox}, shared worker manual and mirrored guidance; Pi adapter restriction seams in agents-plugin-pi/src. |
| scope.surface | cross-module | Public bootstrap command, creation HTTP and session.request_hire profile contract cross runner, MCP, dashboard and mailbox. |
| scope.new_public_symbol | yes | worker.bootstrap, /agents/workers, /agents/workers/permission and /session-hires/<request-id>/approve/permission are additive entry points. Source searches found no current tooling/dashboard name collision; the landed hire router still requires exact-dispatch verification. |
| scope.new_type_contract | yes | worker metadata/bootstrap, worker routes/support/State and hire profile/approved_profile/approved_permission_level plus hire_permission_supported are specified here. Shared permission meaning/service stays sibling-owned; this ticket produces its hire approval transport/persistence integration. |
| scope.test_surface | existing | agents-plugin-tool/internal/wsagent/{lead_http_test,record_test,builtin_test,claude_test}.go; internal/mcp/{hosted_session_test,mailbox_tools_test}.go; internal/wsdashboard/railscript_test.go; internal/wsrsrc mirror tests. New filenames not specified. |
| complexity.reuse_points | confirmed | Read Runner.Create/CreateLead, AgentKeyStore.Mint, keyed gate plus MCP allowlist, LeadMailbox.Watch/Drain, Record unknown-member preservation, leadRows and WsRail.leadBody. |
| complexity.side_effect_risk | high | Session creation, key minting, asynchronous mail delivery and restart authority must agree. |
| risk.correctness | high | Unsupported old HTTP silently drops worker intent; mailbox/watch ownership and restart metadata can diverge. |
| risk.fit | moderate | Existing runner/mail/tool-rule seams fit; owner 1050=a settles the bounded foundation exception. External non-lead transport restriction and identity reuse require implementation coverage. |
| risk.test | high | Mixed-version direction, actual subagent enforcement and non-lead mail lifecycle need new focused cases beyond existing fixtures. |
| risk.security_or_contract | high | Non-lead keyed scope alone leaves keyless bootstrap and orchestration surface gaps; worker-to-lead fallback is prohibited. |

## Phases

### Phase 1: Direct and external worker sessions with bounded authority and mail

Depends on `261115-feat-ws-permission-profile#phase2`, including its Phase 1
contract, reusable permission controls/backend, existing-worker admission and
lead/settings paths. That producer does not depend on this ticket's new
posture, worker creation routes or composed dialog. Consume its landed service
and controls here, with current-parent admission on each new launch path. Owner
(1050)=a permits this current-Runner slice before common-session-interface
Phase 2; keep keys, lineage and input/mail ownership unchanged.

Implement the worker sidecar, explicit tool admission, short manual,
`worker.bootstrap`, worker creation routes and support signal from the
technical contract. The dialog requires a reachable owning lead and offers
an editable one-line intro; mail its address/intro automatically and expose
copy-address on the page. Add actual non-lead hosted mail watch/input delivery
through the existing single-owner path. Preserve direct lead creation,
existing fixed native task tools, and the identity of a bound worker. Existing
live leads receive migration-needed rather than an implicit conversion.
Apply proven native subagent-category controls or manual+nudge per (1012).

Verification:

- Extend focused `internal/wsagent`, `internal/mcp`, `internal/wsdashboard`
  and `internal/wsmailbox` fixtures for delegate mint, effective allowlist
  (including keyless calls/reconnect), lead-only/orchestration refusals,
  unchanged old lead calls, short manual and the two allowed subagent
  categories. Test enforced versus advisory results on Claude/Pi fixtures.
- Cover reachable-lead validation, intro default/edit, automatic introduction,
  actual mail task/report exchange, copy-address equality, no inherited
  named-inbox theft, and one input owner through stop/restart/reopen.
- Cover external fresh/repeat/bootstrap identity, hosted bound-key reuse and
  migration-needed with no lead-key rewrite. Round-trip all sidecar fields
  through pinned old writers and new reopen; keep existing expiry semantics.
- Exercise unsupported old routes and tools without writes or lead fallback,
  including daemon replacement after support GET and before creation POST.
  Explicit permission requests use only the selection-bearing path. Include
  an intermediate worker-capable server lacking `/agents/workers/permission`:
  zero creations and no retry through `/agents/workers` or a lead route.
- Exercise independent role/permission controls, sibling computed quality,
  unchanged Claude `auto`/worker defaults, persistent authorized lowering,
  user-only raises/full warning, and actual CURRENT parent admission at
  create/revive/restart. An unresolved actual parent yields visible pending/
  error; root absence is distinct. Permission non-enforceability alone gives
  a nudge and does not block launch.
- Run touched i18n/manual/manifest and wsflow/Pi mirror checks. Use focused
  existing test harnesses; static fixtures are not live provider acceptance.

### Phase 2: Owner-approved worker hiring

Depends on this ticket's landed Phase 1 and
`261115-feat-ws-session-hire-approval#phase1` and `#phase2` for its proposal
lifecycle and approval modal. It does not wait for the permission sibling's
future-adapter phases or common-session-interface Phase 2 ((1050)=a).

Carry `profile: lead | worker` from hire request through proposal persistence,
modal prefill, owner change and approved creation. Worker approval requires
an owning lead and editable/defaulted intro; use Phase 1's worker creation
path and metadata. Require positive worker/schema support and preserve role
intent across replacement. Produce the exact
`POST /session-hires/<request-id>/approve/permission` amendment and its
`hire_permission_supported` signal here. The landed permission service supplies
selection/admission semantics, not this hire transport. Validate trusted
owner authority, user-only raises and request-bound full acknowledgement;
persist `approved_permission_level`, then apply it to session `permission.json`
through that service without a parent-ceiling snapshot or latent model grant.
Pin the landed hire Phase 1 router/store baseline before claiming old-version
dispatch or preservation evidence. Preserve existing eligibility,
nonblocking outcome mail, expiry/withdrawal, display-only grouping and
no-chaining. Automatic worker introduction and normal task reports do not
become supervisor turn reports to the hire requester.

Verification: focused hire lifecycle/modal fixtures for omitted-profile lead
behavior, worker requests, user-changed profile winning in both directions,
required owning-lead selection, intro/copy-address, restart/repeated approval,
unsupported server/replacement, selection-path enforcement and no authority
escalation. Require `hire_permission_supported` plus `permission_levels`, and
worker support for a worker result. Swap to the pinned pre-amendment and
intermediate hire-only routers between GET and POST: require exact/suffix
path rejection, zero creations/approval transitions and no default fallback.
Test default approval unchanged, model/requester inability to grant a level,
full acknowledgement bound to this request/final selection, optional
`approved_permission_level` through old-write/new-reopen, all other proposal
fields/unknown members, and permission-sidecar recovery with actual current
parent admission on every launch. Repeated approval/recovery must not reapply
a higher old selection over a later authorized lowering. Compare changed
hire controls against that ticket's tracked modal baseline; no separate
worker visual reference is approved.

Forward to common-session-interface Phase 2: absorb these bounded controls
into the shared session boundary with behavior/state parity, retaining their
T0 contracts, existing keys/lineage and single input/mail owner.

## Sage Review Round 1 (2026-11-15)

### Design Reviewer — block

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | The hiring permission API has no defined producer. | important | autonomous |
| 2 | Cross-ticket [261115-feat-ws-permission-profile, 261115-feat-ws-worker-profile-session-spawn]: The tickets assign incompatible meanings to the worker creation route. | critical | autonomous |
| 3 | Cross-ticket [261115-feat-ws-permission-profile, 261115-feat-ws-worker-profile-session-spawn]: Permission Phase 2 consumes surfaces owned by its dependent. | important | autonomous |

### Completeness Reviewer — pass

| # | Title | Severity |
|---|-------|----------|

## Owner Decisions (2026-11-16)

- (1177)=a: the "current parent ceiling" for a worker's permission comes from a
  new authority-parent record. The authenticated parent session writes it at
  spawn, and after a restart the ceiling is recomputed from that parent's
  current permission. The existing worker key format stays unchanged. This
  needs its own design-and-compatibility ticket first (T0 candidate) before
  implementation. Option b (snapshot at spawn only) was rejected.
