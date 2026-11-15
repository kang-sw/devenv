---
title: "Common session interface: distinct CLI/hosted entry, shared access and parity adoption"
parent: 261114-epic-ws-common-session-foundation
blocked-by: 261114-refactor-ws-hosted-common-contract-pi-claude-parity#phase1
related:
  261110-epic-ws-dashboard-agent-host: "preserve existing hosted session lifecycle and product scope"
  261113-bug-ws-hosted-session-sentinel-bootstrap-mints-foreign-key: "hosted identity adoption evidence; no root-issuance policy choice"
  261114-bug-ws-fork-parent-identity-loss: "lineage evidence, not authorization to alter fork authority"
  261114-feat-ws-conversation-audit-event-store: "downstream consumer after this ticket's Phase 2 (682); existing audit envelope stays audit-owned"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 0c3b54a649adcc6d
sage-review-completeness-reviewed: 0c3b54a649adcc6d
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
7. **MAIN refinement assignment, 2026-11-14:** Phase 1 includes the remaining
   backend admission/registration descriptor, lead-profile validation,
   dashboard harness choices and explicit unsupported-operation contract.
   Reuse the existing backend registry and process/event boundary; preserve
   Claude/Pi names, defaults, native integrations and controls. This is T0
   contract refinement, not authorization to implement another harness or
   assert that the broader foundation is already complete. MAIN described
   (799)=a as assumed; no independently confirmed owner runtime grant is
   inferred.

8. **Backend registration (ODQ 01, MAIN ack 2026-11-15):** one
   `BackendRegistration` value per backend name in `wsagent.Options.Backends`
   (replacing the bare `Backend` value) carries `Backend`, `LeadProfile`
   (builtin lead profile name; empty means it cannot host a lead) and
   `PermissionModes` (nil means the control is absent). `ChoiceLister` stays a
   type assertion on `Backend` (496d8de1). The single composition root stays
   `cmd/ws-mcp/dashboard.go`. Rejected: a second descriptor map or a runtime
   loader, which would split ownership.
9. **Registry-driven admission (ODQ 02):** `Profile.validate`,
   `ProfileValueSchema`'s backend enum, `LeadProfileFor`/`CreateLead` and
   `Daemon.RequestHire`'s harness check consult the registered set instead of
   fixed Claude/Pi checks. Production registers only Claude and Pi, so accepted
   and rejected inputs stay identical. Rejected: fixed enums, which reject a
   registered extension.
10. **No new stored name or field (ODQ 03):** this ticket writes no new backend
    name into stored profiles and adds no `Profile` field; extensibility is
    proven with fake registrations in tests only. Strict `ParseProfile` on
    older binaries refuses unknown fields and backend names
    (`internal/wsagent/profile.go:219-246`), so either would be a T3 read
    boundary. A future adapter grades its own tier.
11. **Choices payload (ODQ 04):** `GET /support/choices` `SupportHarness`
    gains optional `lead` (bool) and `permission_modes` (string list, omitted
    when absent). The dashboard's lead launch buttons and permission-mode
    selector render from them and fall back to today's fixed Claude/Pi buttons
    when no entry carries `lead`. T0: old readers ignore unknown fields.
    Rejected: a new route.
12. **Unsupported-operation contract (ODQ 05):** typed errors at the Runner
    boundary - existing `ErrUnsupported`, plus `ErrBackendUnavailable`,
    `ErrInvalidProfile`, `ErrLaunchFailed` - wrap today's errors so error
    text, HTTP statuses, stop/restart fallbacks, held-message behavior and
    trusted-origin checks stay identical. Rejected: new HTTP codes or body
    fields without a consumer.
13. **Roster unchanged (ODQ 06):** `session.roster` keeps its seven-field
    projection, including idle/zero for unavailable telemetry, as approved in
    session-hire Phase 1 (`5c06ac1ea`). The no-fabrication rule binds surfaces
    this ticket adds; roster exposure of telemetry absence is a later
    consumer's.
14. **One MCP adoption core (ODQ 07):** one internal resolver,
    `resolveSession`, returns the session context (key, scope, parent,
    canonical root, root, hosted flag, harness session). Phase 1 routes hosted
    `ferrule` adoption, re-login, `workflow_manual` adoption with sentinel
    refusal, and `playbook.render` child-key minting through it, preserving
    every result and refusal; Phase 2 moves the remaining consumers. A
    card-scoped bootstrap (owner 1072, EXEC2's lane) wraps this core rather
    than adding a fourth adoption body. Rejected: per-tool adoption bodies.
15. **Context selection codifies current behavior (ODQ 08):** an explicit
    `session_key` wins; hosted entry adopts `WS_HOSTED_SESSION_KEY` only when
    `registration.host_pid` validates; the root comes from the key record's
    canonical root; the parent comes only from `key.json.parent`, never from
    rail nesting or conversation `ParentRef`; an unhosted sentinel with a root
    still mints. Rejected: redesigning selection in a T0 extraction.
16. **No MCP or stored-shape change (ODQ 09, hard stop):** Phase 1 changes no
    MCP tool, argument, response field, `/mcp` hello or stored record shape.
    A need found during implementation is a stop (c) to the lead, never an
    in-place change.
17. **Dispatch (ODQ 10):** the worker runs at xlarge with one independent
    review; correctness, fit, test and contract all read high on identity and
    authority paths. Rejected: large.
18. **Verification (ODQ 11):** fixture contracts, not live-harness
    acceptance; the exact list is in Phase 1.

19. **Owner (1050)=a ordering, MAIN ack 2026-11-15 (ODQ 12):** the bounded T0
    permission-level and worker/lead controls of
    `261115-feat-ws-permission-profile` (Decision 13) and
    `261115-feat-ws-worker-profile-session-spawn` (Decision 8) may land before
    this ticket's Phase 2, which absorbs them. Neither was dispatched when
    Phase 1 was promoted, so Phase 1's registry admission (Decision 9) lands
    first; permission-profile Decision 14 and worker-profile rebase onto its
    admission sites rather than adding a parallel admission path. Rejected:
    holding Phase 1 behind them, or leaving the earlier ordering constraint
    unreconciled.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)
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
- New authority UI, new harness adapters and voice remain later consumers
  after this ticket's Phase 2, not its acceptance scope, except the bounded
  owner (1050)=a exception: `261115-feat-ws-permission-profile` (Decision 13)
  and `261115-feat-ws-worker-profile-session-spawn` (Decision 8) may land
  permission-level and worker/lead controls first on current Runner seams, T0,
  with keys, lineage and input ownership unchanged (Decision 19). Phase 2
  absorbs those controls; live reasoning summary
  transport/rendering has already landed independently in `7b047f682` and is
  outside this ticket's acceptance scope (agents-plugin-tool/internal/wsagent/backend.go#L337-L342;
  agents-plugin-tool/internal/wsagent/runner_summary.go#L32-L44).
- Public signatures, descriptor fields, context selection and the absence of
  new serialized fields are settled in Decisions 8-16. The board's proposed
  management checkbox, root-management powers and issuance restrictions
  remain unselected.
- A selectable hosted lead needs admitted/registered backend metadata,
  compatible profile validation and accurate launch choices, not merely a
  type implementing `Backend`. Keep unsupported controls explicit rather
  than converting missing support into success, automatic approval or a
  fabricated idle/subagent count. Full terminal handoff, permission-mode and
  native-child parity are not prerequisites for describing a bounded
  lead-only extension; retain existing supported Pi/Claude paths.

## Prior Art

- `261114-refactor-ws-hosted-common-contract-pi-claude-parity#phase1`, Result
  `44aeb62a8`: shared internal process controls, real-adapter/fake-holder parity
  fixtures, preserved native launch and holder delivery owners.
- `261113-bug-ws-hosted-session-sentinel-bootstrap-mints-foreign-key`: records
  the historical foreign-key incident; current hosted `workflow_manual` refuses
  sentinel/foreign parent-less keys, and top-lead `ferrule` reuses the hosted key
  (agents-plugin-tool/internal/mcp/workflow_manual.go#L323-L332;
  agents-plugin-tool/internal/mcp/server.go#L2398-L2409).
- `261114-bug-ws-fork-parent-identity-loss`: preserved historical key/row
  evidence; loaded/source equivalence and the historical fork-policy rationale
  require separate verification. Do not infer a completed repair.
- MAIN's `shared/docs/harness-interface-readiness.md`, section 5 (found at
  `/Users/kang-sw/.cache/ws@kang-sw-devenv/proj/17da6bdc/shared/docs/harness-interface-readiness.md`,
  absent from this worktree and `/Users/kang-sw/devenv/shared/docs/`), inspected
  source at `951af006a`: `Backend.Start` / `Process` and generic registry
  dispatch already exist; strict profile/lead admission, composition-root
  registration and UI harness lists still name Claude/Pi. Relevant source
  anchors are `agents-plugin-tool/cmd/ws-mcp/dashboard.go`,
  `internal/wsagent/{backend.go,profile.go,builtin.go,runner.go,support.go}`
  and `internal/wsdashboard/assets/{dashboard-leads.js,dashboard-selection.js,dashboard-composer.js,rail.js}`
  after the asset split (agents-plugin-tool/internal/wsdashboard/pages.go#L40-L55;
  agents-plugin-tool/internal/wsdashboard/assets/dashboard-leads.js#L21-L55,#L120-L130;
  agents-plugin-tool/internal/wsdashboard/assets/rail.js#L609-L619). Package-relative
  `internal/` paths here are under `agents-plugin-tool/`. Re-ground those
  anchors at ready promotion; the report is static evidence, not executed
  acceptance or approval of every proposed helper/API.

## Prior Decisions

- Owner (674)=a and (676)–(680)=ok: capture the interface -> parity -> extension
  order in a docs-only lane; implementation is not authorized by this capture.
- `261114-refactor-ws-hosted-common-contract-pi-claude-parity#phase1`, Decision
  3 and Result `44aeb62a8`: native integrations remain active; holder transport
  and delivery policy stay in their existing owners — bearing: constrains.
- MAIN (682): audit's exact foundation producer is this ticket's Phase 2;
  current-state adoption is not a new store format — bearing: constrains.
- 1087e32b (2026-11-15, commit): "Owner1050 explicitly permits these bounded T0 current-Runner controls before common-session-interface Phase2, with keys, lineage and input ownership unchanged; the foundation later absorbs them." — bearing: constrains (reconciled in Constraints)
- 261114-refactor-ws-dashboard-whiteboard-asset-split (2026-11-15, Decisions): "Mechanical split only — (793)=a. Move existing behavior into responsibility-sized files and update its loading/embedding and tests." — bearing: constrains
- 261114-epic-ws-common-session-foundation (2026-11-14, Cross-Child Decisions): "CLI-plugin and MCP-only hosted entry remain distinct. Common access does not make MCP attachment a substitute for harness-native integration or process control." — bearing: supports
- 261114-feat-ws-conversation-audit-event-store (2026-11-14, Decisions): "This backend depends on 261114-refactor-ws-common-session-interface#phase2: parity adoption of current context/state access, without a store rewrite." — bearing: constrains
- 261114-refactor-ws-hosted-common-contract-pi-claude-parity (2026-11-14, commit): "Use optional control handles, not backend-wide boolean promises: Claude worker method sets include lead-only controls, while Pi model/effort support still depends on the argument" — bearing: constrains
- 52dc582b (2026-11-13, commit): "Claude keeps settings sources, hooks, permissions and user MCP servers. Effective plugin settings include its POSIX canonical local store; child launch behavior remains unchanged." — bearing: constrains
- 7f40e811 (2026-11-13, commit): "The parent reference is a typed {kind, reply_id} so a later Pi-parent path can reuse the record and report hook (the recursion ticket)." — bearing: constrains
- 496d8de1 (2026-11-08, commit): "Availability goes through ChoiceLister.Executable() plus lookPath, so ClaudeBackend{Command} and PiBackend{Command} are honoured. A backend without ChoiceLister counts as available whenever registered." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/cmd/ws-mcp/dashboard.go; agents-plugin-tool/internal/wsagent/{backend,profile,builtin,runner,support}.go; agents-plugin-tool/internal/wsdashboard/assets/{dashboard-leads,dashboard-selection,dashboard-composer,rail}.js; identity access also reaches agents-plugin-tool/internal/mcp. |
| scope.surface | cross-module | Shared CLI/hosted access and admission cross MCP, Runner, profiles and dashboard HTTP choices; existing Backend.Start and Process are agents-plugin-tool/internal/wsagent/backend.go#L24-L28,#L92-L110. |
| scope.new_public_symbol | yes | `wsagent.BackendRegistration`, typed Runner errors and optional `SupportHarness` `lead`/`permission_modes` (Decisions 8, 11, 12); `resolveSession` is MCP-internal (Decision 14). |
| scope.new_type_contract | yes | Phase 1 requires backend admission/registration metadata and shared identity/context/state access; existing processControls is per launched instance, not backend-wide metadata (agents-plugin-tool/internal/wsagent/backend.go#L182-L220). |
| scope.test_surface | existing | agents-plugin-tool/internal/wsagent/common_contract_test.go#L57-L132,#L206-L272 and controls_test.go#L14-L99; agents-plugin-tool/internal/wsdashboard/lead_isolation_test.go#L10-L39. Phase 1 fixtures are listed in its verification boundary; no tests were run during population. |
| complexity.reuse_points | confirmed | Read Options.Backends and generic backend/startBackend dispatch (agents-plugin-tool/internal/wsagent/runner.go#L109-L115,#L779-L795), controlsFor, ChoiceLister/SupportHarness, Profile and leadRows/leadBody. |
| complexity.side_effect_risk | high | Admission and access changes intersect process launch, stable keys, held mail/input, approval correlation and holder recovery (agents-plugin-tool/internal/wsagent/runner.go#L603-L711,#L1443-L1513,#L2241-L2350). |
| risk.correctness | high | Capability absence, argument-dependent restart fallback and replay holds must remain distinct; absent turn probes retain the hold (agents-plugin-tool/internal/wsagent/runner.go#L2566-L2610,#L3262-L3290). |
| risk.fit | high | Existing generic execution fits reuse, with admission, signatures and context selection settled in Decisions 8-15 across strict profiles, fixed lead defaults and HTTP/UI choices (agents-plugin-tool/internal/wsagent/profile.go#L219-L246; builtin.go#L64-L72; runner.go#L580-L599). |
| risk.test | high | Existing Pi/Claude fixtures do not establish the planned extensible admission or CLI/hosted shared-access migration; unsupported telemetry, mixed-version writes and current-context selection need explicit fixture contracts. |
| risk.security_or_contract | high | Preserve hosted key adoption and parent authority separately from display grouping; strict Profile and MCP argument readers plus trusted-origin approval writes constrain T0 (agents-plugin-tool/internal/mcp/workflow_manual.go#L323-L332; agents-plugin-tool/internal/wsagent/http.go#L127-L146,#L217-L226). |

## Phases

### Phase 1: Establish the minimum shared access contract

At the existing execution/access boundary, establish the shared playbook,
context and state access contract for separate CLI-plugin and MCP-only hosted
entry. Carry migration-essential identity, existing authority and parent
lineage and make the common session/event/adapter boundary explicit for later
consumers. Reuse current owners rather than introducing parallel stores,
delivery queues or a new workflow interpreter. Leave the audit envelope in the
existing audit ticket.

Include a backend admission/registration descriptor contract at the current
registry/composition boundary. It must account for backend identity and
availability, eligible hosted-lead profiles, existing model/effort choices and
supported optional controls without creating a second process owner. Reconcile
profile validation and dashboard launch choices with that admitted set so a
registered extension is not still rejected by a fixed Claude/Pi enum, and the
UI does not offer an unavailable harness or unsupported control. Retain a
single explicit composition-root registration; do not turn this into a
runtime plugin loader or replace `Backend.Start` / `Process` from scratch.

State unsupported-operation behavior at the existing process-control and
product boundaries: distinguish absent optional control, unavailable backend,
invalid profile and launch failure; preserve current error/fallback/restart
semantics and trusted-origin approval checks. Do not map Claude permission
strings onto another harness by assumption, equate EOF with turn completion,
or infer known-zero subagent activity from unavailable telemetry. The
registration value, admission sites, choices fields, typed errors, adoption
core and selection rules are Decisions 8-16.

Verification boundary (fixture contracts; not provider/live-delivery acceptance):

- existing fixtures stay green: `TestCommonContract*`,
  `TestProcessControlsMatchTheLaunchedProcess`,
  `TestBackendSelectionKeepsNamesAndUnavailableErrors`,
  `TestLeadPopoverLaunchBehavior`;
- new fake-registration fixtures cover admission, profile schema,
  `RequestHire`, choices `lead`/`permission_modes` and the UI fallback,
  without installing or launching a new harness;
- table tests pin each unsupported-operation path's error, status and
  fallback, including approval correlation and capability absence;
- `resolveSession` fixtures cover both entry modes (CLI-plugin and hosted),
  sentinel refusal, re-login mismatch, child minting with parent, and
  historical key records;
- runs: `go test ./internal/wsagent` in full; `./internal/mcp` and
  `./internal/wsdashboard` only with `-run` on the touched areas.

### Result (306474cb0) - 2026-11-15

- Added runtime-only `BackendRegistration` at the existing composition root.
  Registry-driven profile/schema, lead and hiring admission retain production
  Claude/Pi names and defaults; fake registrations establish extensibility.
  Lead eligibility is checked on the resolved profile backend before minting,
  including overridden lead profiles. Existing eligible cross-backend overrides
  remain valid.
- Added the approved optional HTTP choices metadata and registry-driven launch
  buttons/permission selectors, with the legacy Claude/Pi fallback. Lead-only
  choices do not become tracker-support selections. Typed internal errors retain
  existing text, HTTP status and control/restart/approval behavior.
- Extracted `resolveSession` for hosted bootstrap, re-login, manual adoption and
  render child minting. Key records remain the identity/root/parent authority;
  CLI and hosted entry remain distinct. Historical split and legacy records
  are readable without a read-time rewrite. No card storage path or selector
  was introduced; a future card bootstrap can wrap this core.
- Compatibility: **T0**, no new MCP tool/argument/response field, hello field or
  stored shape. Old readers still read/write the unchanged `Profile` shape
  (`internal/wsagent/profile.go:76,219`); production schema values are pinned
  equal to the prior builtin schema by `TestRegisteredBackendAdmission`.
  New descriptors and the session projection are runtime-only
  (`internal/wsagent/registration.go:14`; `internal/mcp/session_context.go:12`).
  HTTP choices add only optional fields (`internal/wsagent/support.go:218`);
  old JS readers ignore them, while new JS falls back when old responses have
  no lead markers (`internal/wsdashboard/assets/dashboard-leads.js:50`).
  Existing key readers/writers are unchanged (`internal/mcp/session_auth.go:205`;
  `internal/mcp/session_layout.go:303`), so reopening needs no reconstruction
  or migration. Paths in this paragraph are under `agents-plugin-tool/`.
- One independent Lite review found one Important issue: an overridden lead
  profile could target a worker-only backend. Fixed it and added refusal-without-
  minting plus eligible-override fixtures. No Critical or unresolved review
  finding remains; no second review ran, per Lite allocation.
- Verification used `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`, `-count=1`:
  - Full `go test ./internal/wsagent -timeout 900s`: completed in 311.971 s,
    **baseline-red**, not a passing suite. A complete repository archive of
    exact base `f1ab88a53` completed in 319.547 s with the identical 128 failing
    test/subtest names and identical 91 assertion locations. The stale bare-text
    user-envelope expectations/timeouts remain untouched, per the lead's
    baseline disposition; no added failure was observed.
  - `go test ./internal/mcp -run 'Test(ResolveSession|HostedFerrule|HostedFlag|UnhostedFerrule|HostedWorkflowManual|Ferrule|Relogin|TheHostedSessionsOwnLogin|ConfigTuneAgentProfile|ConfigListAgentProfile|ConfigRegisteredAgentProfile|RenderMints|RenderMintsLeadScoped|PlaybookRenderDelegateImplementer|PlaybookRenderToolTierOverride|ConfigGet)' -timeout 300s -v`:
    37 top-level tests passed (8.535 s).
  - `go test ./internal/wsdashboard -run 'Test(Hiring|LeadPopoverLaunchBehavior|LeadIsolation)' -timeout 180s -v`:
    15 top-level tests passed (1.568 s), including the extended Node launch UI
    fixture and fake-registry hiring checks.
  - `go test ./cmd/ws-mcp -run 'Test.*(Hosted|Lead|Token)' -timeout 180s`:
    passed (33.250 s). Added registry, error, capability-absence and resolved-
    backend regression fixtures also passed in the focused contract run and
    introduced no failure in the final full package run. `git diff --check`
    passed.
- Scope: Phase 1 only. Phase 2/current-state consumer adoption and ticket
  closure remain pending. Verification is fixture-only, not live-provider
  acceptance; no shared daemon restart, protected port access, Electron,
  installation, push or branch integration was performed.

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
