---
title: "Claude/Pi 세션용 공통 권한 의도와 실제 적용 상태 보고"
related:
  261115-research-ws-unified-agent-tool-surface: "research context only; owner 853 approved capture, not permission policy"
  261114-refactor-ws-common-session-interface: "owner 1050 permits bounded T0 controls first; its Phase 2 later absorbs them without changing keys, lineage or input ownership"
  261115-feat-ws-session-hire-approval: "worker-profile Phase 2 owns the exact selection-bearing hire-approval amendment; hiring authority and display grouping remain unchanged"
  261115-feat-ws-worker-profile-session-spawn: "its Phase 1 follows permission Phase 2 and owns worker routes/dialog composition; its Phase 2 owns the hire-approval amendment"
  261115-feat-ws-hosted-codex-adapter-contract: "deferred Codex permission mapping belongs with its adapter; not a prerequisite here"
  261114-feat-ws-hosted-antigravity-acp-adapter: "deferred agy permission mapping belongs with its adapter; not a prerequisite here"
  261117-feat-ws-agent-tools-mailbox-routing: "its Decision 13 is the source of Decision 3's ws/agent.spawn exception; it adds the create-time admission and worker-side explicit-level persistence for agent.spawn children"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: c34a82c839d45331
sage-review-completeness-reviewed: c34a82c839d45331
---

# Common permission intent and truthful enforcement

## Background

Sessions need common permission intent without pretending that harnesses enforce
identical boundaries. The owner selected three levels and truthful per-harness
enforcement reporting. Launch must remain usable on a harness that cannot enforce
the requested level, including Pi workers; a permission label is not a sandbox.

Authority: owner answers (998)–(1004) on board topic `unified-toolset`, relayed
by MAIN on **2026-11-15 at 06:54:39**, clarified at **07:45:14**. The latter
corrects the literal enum, launch admission, legacy defaults, storage and five-phase
scope below. The research ticket's Confirmed Decisions supply research-capture
authority only; its narrative and the permission brief supply supporting evidence.

## Execution Boundary (2026-11-15)

MAIN **2026-11-15 16:45:17Z** authorizes **Phase 1 documentation only** on
this retained lane. CSI (`261114-refactor-ws-common-session-interface`)
Phase 1 has merged; the source audit below refreshes the contract onto its
registry/admission sites. Permission Phase 2 runtime now waits for **CSI Phase 2
to land and a new MAIN dispatch**. CSI adoption supplies implementation seams,
not permission policy or execution authority. Leave this ticket in `ready/`.

This dispatch supersedes the earlier scheduling boundary in Decision 14:
MAIN 09:23:25 / 09:23:59 withdrew the brief 09:22:10 go-now instruction and
held admission work for CSI Phase 1. The stopped worker made no runtime edit.
The owner decisions below remain unchanged. Permission Phase 2 remains the
prerequisite for worker-profile Phase 1 and agent-card dispatch; Phases 2–5
are outside this documentation run. Only this ticket may change; source reads,
document assertions, ticket validation and independent lite review are the
verification scope.

## Decisions

1. **(998), corrected by MAIN 07:45:14 — common intent.** The literal enum is
   `read-only`, `workdir-write`, `full`, ordered from least to most permissive.
   Report `enforced`, `partial` or `advisory` separately. Read-only requests
   inspection without mutation; workdir-write requests mutation within the
   session working directory; full removes common-intent restrictions, not
   capability allowlists. These are intent labels, not proof of confinement or
   a new network policy.
2. **(999), owner amendment — nudge and launch.** Inability to enforce the
   level never blocks either a lead or worker, including Pi. Launch with a
   light one-line nudge: `Permission <level> is <quality> on <harness>: <limit>.`
   This does not waive capability checks, permission admission, invalid input
   or actual launch failures. Reject the brief's fail-closed worker option.
3. **(1000), clarified — current-parent admission on every launch.** Recompute
   the session's level and the actual authority parent's current level for
   create/start, revive, restart and resumed launches. A child may not exceed
   that parent. Anyone already authorized to control the target may lower;
   only the user may raise. Agents cannot runtime-raise themselves or children.
   `full` requires the user's choice after a warning. Store no parent ceiling;
   `ResolvedDefinition` remains a capability snapshot, not permission state.
   Root sessions have no actual parent and need no parent ceiling. For a
   child, unresolved/missing required parent state returns explicit
   parent-admission-pending/error until resolved; never guess a ceiling or
   substitute a fabricated minimum. This is failure to satisfy (1000), not
   harness unenforceability under (999). Parent recomputation does not itself
   add a parent-change process-killing/restart mechanism. Exception for
   `ws/agent.spawn` children (owner (74)=b, 2026-11-17, recorded in
   `261117-feat-ws-agent-tools-mailbox-routing` Decision 13): a child may be
   created at any level up to its parent's current level, `full` included,
   without the user's warning; only a child above its parent is refused.
   The spawn-supplied level is stored as the child's explicit selection
   (Decision 8), always lower than or equal to the parent's current level,
   never a raise. Their authority parent is the daemon-written Runner spawn
   record (`Record.Parent`), which owner (84)=a (2026-11-17) accepts as the
   (1177)=a authority-parent record for the `ws/agent.spawn` path only;
   agent-tools implements it. A lead parent with no computable common level
   (no Runner record, as for an external CLI lead, or a backend
   `legacyLeadPermission` does not derive, as for hosted Codex and
   Antigravity leads until Phases 3 and 4) caps its `ws/agent.spawn`
   children at `workdir-write`, extending Decision 4's legacy derivation
   that already maps a Pi lead to `workdir-write` (lead decision (86),
   2026-11-17, recorded in agent-tools Decision 13); this is a derived
   ceiling, not a guessed one, and a missing recorded child parent still
   fails admission. Every other worker path's ceiling stays deferred
   pending (1177).
4. **(1001) — preserve legacy defaults.** Omitted selections retain Claude
   `auto` and the existing fixed worker tool loadouts. Derive a display from
   current native mode/tool settings without writing it back. Mark it
   `legacy-derived`; deriving `full` from existing bypass/unrestricted tools
   is observation of legacy behavior, not a fresh explicit grant.
5. **(1002) — separate common field.** Add `permission_level` at the existing
   launch/control boundary; retain native `permission_mode` and observed
   `active_mode`. Keep permission and capability dimensions independent.
6. **(1003) — per-launch backend translation and verification.** Compute the
   mapping each launch and inspect what the harness actually reports. Separate
   requested intent, observed native settings and demonstrated enforcement.
   Missing observations or unproved confinement yield conservative quality
   and the nudge, not an unenforceable-level refusal. Future Codex/agy settings
   are per run, never global.
7. **(1004) — five bounded phases.** Phase 1 is contract/compatibility docs,
   Phase 2 Claude/Pi, Phase 3 Codex launch path, Phase 4 agy, Phase 5 broader
   child-ceiling and ws launch-approval integration. Phase 2 includes (1000)
   safety on every path it introduces; Phase 5 does not defer those checks.
   Future adapters do not block the first two phases.
8. **Technical storage choice, within MAIN's authorization.** Store the current
   explicit selection in `<SessionDir>/permission.json` as
   `{"permission_level":"workdir-write"}`. Any authorized caller may lower and
   persist this selection; only the user may raise it. Durable lowering prevents
   a revive/restart from restoring a higher old choice. A missing file means
   legacy derivation. Use the session-local, private atomic-sidecar pattern;
   tolerate and retain unknown members when updating this file. Read it on
   reopen and every actual launch. Unreadable/invalid existing selection is
   an explicit admission error, not silent legacy fallback. Never store
   computed enforcement, derived legacy values, native observations or a
   parent ceiling there. No field is added to Record, Profile, registration or
   key records. Prefer this sidecar
   over a tolerant Record member because CLI and hosted sessions share the
   session-directory home, whereas a CLI session need not have an agent Record.
9. **Technical API choice.** Use the existing Runner launch/control seams
   (`LeadOptions`, `SpawnSpec`, `StartSpec`, settings handling), with transient
   `permission_level`; accepted explicit selections update the sidecar under
   the lower/raise rule. Compute State fields
   `permission_level`, `permission_source` (`explicit` or `legacy-derived`) and
   `permission_enforcement` (`enforced`, `partial`, `advisory`). Keep native
   observations in existing `permission_mode` / `active_mode` and tool-loadout
   evidence; do not fabricate a native mode for Pi. Absence of a current
   observation is shown as pending/unknown evidence, not `enforced`.
10. **Technical support preflight.** Extend each harness entry of existing
    `GET /support/choices` with
    `permission_levels:["read-only","workdir-write","full"]` only when the
    reusable backend contract and lead/settings selection paths are supported.
    This signal does not imply that sibling worker creation has landed. Before
    sending a selection, new HTTP clients require the chosen level in that
    entry from the current server; worker clients additionally require the
    sibling's worker support. Missing/unknown support keeps legacy controls
    usable but sends no selection-bearing request. Recheck after reconnect/server
    replacement, and do not retry a selection blindly against another server.
    Selection-bearing HTTP requests use `/permission` variants of existing
    operations: `POST /agents/leads/permission` and
    `POST /agents/<id>/settings/permission`. The sibling owns
    `POST /agents/workers` for default/legacy permission only and
    `POST /agents/workers/permission` for explicit `permission_level` and,
    for user full selection, `permission_full_ack`. These delegate to existing
    Runner owners; legacy URLs remain intact. On the inspected old router,
    lead creation's nonempty action gives 405 (`http.go:154–157`), default
    worker creation's nonempty id/empty action gives 405 (`http.go:175–176`),
    and worker explicit selection plus `settings/permission` fall to 404
    (`http.go:229–230`). None reaches a creation/settings write. An intermediate
    server supporting default workers but not explicit selection must reject
    `/agents/workers/permission` with zero creations; neither missing support
    nor a daemon swap permits fallback to `/agents/workers`.
    New MCP callers additionally require `permission_level` in the discovered
    input schema before sending it. A missing signal or refused new path is
    not permission to drop intent and launch on a legacy URL.
11. **User warning and authority.** Reuse the local/own-Origin control gate,
    applied also to permission raises on worker sessions. Before a trusted
    user request selecting `full`, the dialog warns and requires confirmation;
    the request carries `permission_full_ack:true`, required for that explicit
    full selection. An agent cannot satisfy user authority by supplying this
    flag. Existing native-mode controls remain, but may not bypass the common
    user-only raise check. No new human-provenance/authentication mechanism is
    claimed by the existing trusted gate. Decision 3's `ws/agent.spawn`
    exception is outside this rule: an agent-created child at its parent's
    level, `full` included, needs neither the warning nor the flag.
12. **Compatibility.** The bounded exception is **T0 only** across the shipped
    change: retain old calls and native UI behavior, preserve selection bytes
    and compute optional projections. MAIN's earlier T0/T2 storage latitude
    does not waive the later T0-only exception. No protocol/schema bump,
    native-field rename or T3 boundary. If pinned writer/reopen or caller
    fixtures cannot establish T0, return the failed path to the lead rather
    than claim T2 recovery or ship a display/behavior regression.
13. **Owner (1050)=a, MAIN 07:58:13 — bounded foundation exception.** These
    permission-level and worker/lead controls may precede
    `261114-refactor-ws-common-session-interface#phase2`, using current Runner
    seams, T0 only, with keys, authority lineage and input ownership unchanged.
    This is an explicit exception to the earlier authority-UI ordering, not a
    general foundation bypass. Forward: that interface's Phase 2 absorbs these
    controls. Historical foundation tickets are not rewritten here.
14. **Technical delivery boundary.** Decision 19, confirmed by MAIN
    09:23:25 / 09:23:59 and EXEC4 09:23:38, holds admission work until CSI
    Phase 1 merges; use its merged admission sites without a parallel rewrite.
    Permission Phase 2 produces reusable
    permission controls, backend admission for existing workers, and lead /
    settings selection routes. It does not produce new worker-posture routes
    or compose the worker dialog. Worker-profile Phase 1 follows permission
    Phase 2 and owns both worker creation routes, posture/owning-lead/bootstrap
    behavior and dialog composition using these controls. Worker-profile
    Phase 2 owns its exact selection-bearing hire-approval amendment; it does
    not wait for an undefined permission Phase 5 producer.
15. **Owner (1875)=a, 2026-11-20 — Phase 2 closeout gaps stay in Phase 2.**
    The Phase 2 closeout audit (merge `88029f6e8`) found three Phase 2
    clauses still missing on develop; all three are finished inside Phase 2,
    not moved to Phase 5 or dropped: (1875) the reusable common permission UI
    with reconnect-aware positive preflight and the full-warning
    cancel/confirm flow (the dashboard still reads and sends only native
    `permission_mode`); (1876) current-parent admission for workers started
    outside `agent.spawn` (legacy and Pi workers still launch without an
    explicit common authority); (1877) the exported worker permission input,
    State/control projection, and an authority-accurate worker launch nudge
    computed after the stored `agent.spawn` authority is read. Phase 2 gets
    its whole-phase `### Result` only after these land with the Focused
    Verification Contract cases. Rejected: moving them to Phase 5 (Decision 14
    forbids Phase 5 from postponing Phase 2 child checks) or dropping them.

### Directional compatibility evidence

Source baseline for the following inspection: `90eacda7e`. Paths beginning
`internal/` below are under `agents-plugin-tool/`. These distinguish current
behavior from a future implementation target, not completed compatibility tests.

| Surface / direction | Writer, reader and reopen consequence | Verdict / settlement boundary |
|---|---|---|
| Existing `permission_mode`; old caller → new handler, old record → new reader | Creation, settings and accepted native reports write it (`internal/wsagent/runner.go#L630-L645,#L926-L944,#L2695-L2705`); launch reads it (`claude.go:108–119`); Record read/write is `record.go:278–301`. New reopen reads the same field. | **T0 target:** preserve omission/defaults and native calls; no rename or migration. |
| New `permission.json` / `permission_level`; new selection writer → pinned old session writers → new reopen | New code writes only this sidecar. Current Record writes only `agent.json` (`internal/wsagent/record.go:277–303`); registration writes/removes only `registration.json` (`internal/wsdashboard/registry.go:282–328`). Label updates touch only `label.json` (`label_route.go:91–105`). Those writers leave the new bytes intact; new code must read them on reopen. Whole-directory pruning remains the existing lifetime (`registry.go:331–385`), not a permission-store migration. | **T0 storage candidate**, subject to pinned old-write/new-reopen and lifecycle fixtures. No overwrite/reconstruction or T2 claim. Missing sidecar preserves old sessions. Old launch enforcement is not implied by byte retention. |
| New HTTP `permission_level` / `permission_full_ack` and distinct paths; old caller → new handler, new caller → old handler | Current typed request decode ignores unknown members (`internal/wsagent/http.go:268–286`), so explicit fields on default URLs alone are unsafe. On the pinned old router, `POST /agents/leads/permission` returns 405 (`http.go:154–157`), sibling default `POST /agents/workers` returns 405 (`http.go:175–176`), and sibling explicit `POST /agents/workers/permission` plus `POST /agents/<id>/settings/permission` return 404 (`http.go:229–230`). None reaches `CreateLead`, `Create` or `SetSettings`. New handlers reuse existing Runner/write gates. No old-server write; reopen does not replay requests. | **T0 extension target** with old/default calls unchanged and positive preflight. Explicit worker intent uses only `/agents/workers/permission`; refusal never retries on `/agents/workers` or another legacy URL. |
| New explicit worker client → intermediate default-worker-only server | The sibling's default route may exist before explicit selection support. Its handler must reject `/agents/workers/permission` before creating any session, rather than decode/discard selection on `/agents/workers`. This is a required intermediate-server fixture, not a claim about an uninspected released version. No creation/write on refusal, and no request replay on reopen. | **T0 target** only with zero creations on unsupported selection and no client fallback. Permission Phase 2 supplies the reusable contract; worker-profile Phase 1 owns both worker routes and these integration assertions. |
| New `permission_levels` support member; new producer → old UI / new client → old choices | Existing `SupportHarness` is a response projection (`internal/wsagent/support.go:215–225`), serialized by choices (`support.go:400–413,619–622`); UI reads named harness/model/effort members (`internal/wsdashboard/assets/dashboard-leads.js:43–59`). Old UI ignores the additive member without writing it. New client treats absent member as unsupported; reopen reads it afresh from the current server. | **T0 target** for additive discovery and gated use, not inferred from availability/model support. No new manifest or stored support bit. |
| Computed State `permission_level`, `permission_source`, `permission_enforcement`; new producer → old display | State is separate from Record (`internal/wsagent/runner.go:1053–1146`); JSON output is `http.go:288–304`. Old UI reads the same named native fields (`internal/wsdashboard/assets/dashboard-leads.js:22–26,92–110`), ignores new members and writes none. New reopen recomputes projections from selection and launch evidence. | **T0 target**: old native controls/display stay unchanged, not relabeled as common enforcement. No persisted snapshot/registration projection. A display regression is not accepted under owner 1050. |
| Optional MCP `permission_level`, where the phase adds it; old call → new handler, new call → old server | `pi.spawn` current schema is `internal/mcp/pi_tools.go:529–543`; precheck rejects undeclared members before handler writes (`tool_arguments.go:65–128`). The legacy call remains unchanged; new client must discover support before sending. Reopen cannot repair an unsupported call. | **T0 addition target with discovery gating**; no removal/rename. Ungated new arguments meet old refusal, not a semantic fallback. A model-supplied lowering is explicit state, never user authority to raise. |
| Rejected Profile/Record/registration/key permission members | Strict Profile rejects unknown fields (`internal/wsagent/profile.go:219–238,413–429`): a new persisted field makes an old read fail (**T3**, bytes retained but old reader blocked). Current Record preserves unknown top-level members (`record.go:157–210`), not nested typed `resolved_definition` (`record.go:31–35`). | No changes to these stored shapes. Record tolerance is not needed for the selected sidecar; no arbitrary-old-version or nested-field T2 claim. |

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)
- Use the existing Runner/backend/process/holder owners; preserve identity,
  lineage, approvals and one input/mail delivery owner. This ticket does not
  replace native tools with ws-mcp or introduce a universal sandbox/runtime.
- The nudge addresses unenforceable **permission intent**, not a bypass of
  capability validation. Existing tool/MCP allowlists, key-mint exclusions and
  trusted control gates remain effective. A read-only native tool list alone
  does not establish read-only behavior of allowed ws tools, other servers,
  plugins or subprocesses.
- Permission Phase 2 exports reusable permission controls/backend admission
  without depending on new worker-posture surfaces. After it lands,
  `261115-feat-ws-worker-profile-session-spawn#phase1` owns both worker routes
  and worker/posture/owning-lead/bootstrap dialog composition; that sibling's
  Phase 2 owns the exact selection-bearing hire-approval amendment. Its metadata
  sidecar is separate. `owning_lead` and hired/display grouping do not replace
  actual authority-parent lineage when checking a permission ceiling.
- Owner 1050 authorizes only the bounded current-Runner exception above;
  common-session-interface Phase 2 later absorbs these controls. Keys,
  authority lineage and single input ownership remain unchanged.
- Applicable implementation manuals: `ai-docs/manuals/shipped-surface-boundary.md`
  for plugin/tool surfaces; `ai-docs/manuals/release-compatibility.md` for tool
  internals; `ai-docs/manuals/ws-mcp.md` for MCP wiring;
  `ai-docs/manuals/skill-authoring.md` for authored prompts/conventions; and
  `ai-docs/manuals/wsflow-mirroring.md` for mirrored package surfaces. Project
  tickets/manual citations stay in this ticket, not shipped downstream text.
- Verification uses isolated fixtures; live harness/provider confinement tests
  require separate authorization. No approved permission-feature visual
  reference is supplied. Reuse the current dialog rather than invent a new
  layout or alternate launch owner.
- Worker requests use the sibling's `profile: lead|worker` posture and its
  positive preflight; omitted profile retains lead. Its owning-lead choice,
  editable one-line introduction (default `worker`), and worker-metadata
  sidecar stay sibling-owned. This ticket supplies `permission_level` and the
  computed State enforcement fields; it adds no ws key scope or authorization
  role and does not reuse worker metadata as permission storage.

## Prior Art and Source Evidence

Current-source references in this section and the translation table are under
`agents-plugin-tool/`, audited at **b59c76212329c0d613769191e804ae5eb86f8117**.
The directional matrix in Decisions instead retains its historical
**90eacda7e4956aded76cfc092d20c41e76d2cc1a** snapshot. Neither source inspection
nor an existing test body is an executed permission-feature fixture.

- `internal/wsagent/profile.go:76–138,293–315`: capability defaults are off;
  lead profiles load the user's setup and exclude capability fields;
  `permission_mode` is native state, absent mode defaults to `auto`.
  `runner.go:683–698` now clears it when the resolved backend registration has
  no native permission modes (production Pi), not by a fixed non-Claude check.
  These are current facts, not a common-level default or a `Pi = full` policy.
- `internal/wsagent/claude.go:102–133,135–210,267–321`: leads pass native mode
  and stdio prompts; workers isolate settings and use `dontAsk` plus tool/MCP
  allowlists, with init validation. This proves current launch wiring, not
  that `plan = read-only` or `acceptEdits = workdir-write` with filesystem
  and network confinement. Preserve unrelated capability mismatch failures.
- `internal/wsagent/pi.go:156–207,255–290,345–367`: workers construct native
  and MCP tool allowlists and reject unexpected tools; lead arguments do not
  select a native permission mode. `--no-approve` is project trust, not a
  permission/sandbox control. The brief's Pi no-sandbox finding is supporting
  prior research, not a newly executed probe. Pi workspace/network confinement
  is not established by this source inspection.
- `agents-plugin-pi/src/execute-gateway.ts:1–103`: the ws-built approval flow
  distinguishes gated execute-workers from general workers and also has
  ungated lead execution paths. It is not universal Pi sandbox enforcement.
- `internal/wsagent/runner.go:2218–2219` passes `WS_MCP_ALLOWED_TOOLS`;
  `internal/mcp/server.go:730–732,5665–5667,5689–5696` enforces tool admission.
  Existing capability narrowing compares tools and exact MCP server definitions
  (`internal/wsagent/profile.go:445–466`); it does not compare permission levels.
- `internal/wsagent/http.go:127–146,205–214` retains trusted local/own-origin
  controls; `runner.go:984–1005` accepts native mode changes only for leads
  whose registration declares modes (production Claude). A trusted-route check
  is not proof that an arbitrary request came from a human; the selected
  warning/gate contract does not claim otherwise.
- `261114-refactor-ws-common-session-interface`, Decisions 2/7 and Phase 1:
  labels are not permission proofs; backend descriptors belong at the current
  registry boundary; do not map Claude strings onto another harness by
  assumption.

### Launch, recovery and UI seams

- `Backend.Start` / `StartSpec` / `LeadSpec` (`internal/wsagent/backend.go:21–82`),
  registry-backed `Runner.backend` / `startBackend` (`runner.go:801–816`) and
  computed State (`runner.go:1115–1209`) remain the implementation owners.
  Native `permission_mode` can change from accepted harness reports
  (`runner.go:2795–2806`); it is not immutable intent. Current `LeadOptions`
  (`runner.go:566–575`), `SpawnSpec` (`spawn.go:66–90`) and `StartSpec` have no
  common permission input yet.
- The label sidecar pattern is `internal/wsdashboard/label_route.go:21–31,75–106`.
  The planned permission sidecar shares the session directory's lifetime.
  Existing Reopen rewrites only the record and keeps that directory
  (`internal/wsagent/handoff.go:231–288`); permission retention remains a
  fixture target, not implemented reopen behavior. Pruning can delete an
  expired session directory (`internal/wsdashboard/registry.go:331–388`),
  except the runner-owned agent and handed-off Pi conversation cases its body preserves.
  Stop/Restart are `runner.go:936–955`; restart delegates to `startProcess`
  (`runner_recovery.go:224–238`), which dispatches to `startWorker`/`startLead`
  (`runner.go:2138–2159,2294–2334`). Reattach uses a capability snapshot
  (`runner.go:2431–2457`), not a permission ceiling; fixtures cover
  changed/missing snapshots (`internal/wsagent/profile_spawn_test.go:202–293`).
- UI reuse is `leadRows` / `rpLeadMake`
  (`internal/wsdashboard/assets/dashboard-leads.js:85–134,142–220`) and
  `WsRail.leadBody` (`internal/wsdashboard/assets/rail.js:653–663`).
  Extend choices/State rather than adding another launch owner. Worker posture,
  `owning_lead`, `intro`, `worker.bootstrap`, default `/agents/workers`, explicit
  `/agents/workers/permission` and `/support/choices.worker` remain sibling-owned
  Phase 1 consumers after permission Phase 2, not producers needed by it.

### CSI handoff source audit (2026-11-15)

**VERIFIED — snapshot and ancestry.** Retained branch
`impl/develop/foyer-cause-shore` was refreshed by
`55ef92679a2d7c0edc510f224bc146056d6dbbd0`, whose parents are documentation head
`b07aad292dedb86266d69bc3bf581f653bfa2fb5` and source snapshot
`b59c76212329c0d613769191e804ae5eb86f8117`. The latter includes both
`59a283670` and CSI Phase 1 merge `41ffdd22e`; only this permission ticket
differs between that source snapshot and the refreshed head. No further sync
is part of this run. EXEC4's map reported against `1cead26bf` was a discovery
hint; the sites below were read at the actual snapshot instead.

Paths in this table are relative to `agents-plugin-tool/`. **Contract target**
means later Phase 2 work, not code delivered by CSI or this audit.

| VERIFIED current site | Current fact and permission consumer boundary |
|---|---|
| `internal/wsagent/registration.go:10–28`; `internal/wsagent/runner.go:109–115`; `cmd/ws-mcp/dashboard.go:263–268` | Runtime-only `BackendRegistration{Backend,LeadProfile,PermissionModes}` lives in `wsagent.Options.Backends` (not a type named `RunnerOptions`). `BackendRegistrations()` copies metadata; production registers only Claude/Pi, with native modes only for Claude. **Contract target:** reuse this registry and `Backend.Start`, not a parallel descriptor/admission map. Native `PermissionModes` is not common `permission_levels` support or enforcement evidence. |
| `internal/wsagent/registration.go:54–84`; `internal/mcp/agent_backends.go:7–14`; `internal/wsagent/profile.go:219–247,293–315,470–500` | Profile validation/schema consume the runner registry; standalone readers retain builtin fallback. Historical Pi reads still accept `LeadPermissionModes`, while Pi launch ignores them. Keep the strict stored Profile unchanged; transient common input/sidecar remain separate targets. |
| `internal/wsagent/builtin.go:64–76`; `internal/wsagent/runner.go:596–657`; `internal/wsagent/handoff.go:81–102`; `internal/wsagent/registration.go:114–117`; `internal/wsdashboard/session_hire.go:157–161` | `LeadProfileFor` supplies lead eligibility to create, takeover and `AdmitsLead`/hiring. `create` resolves/validates the actual override backend before minting. Its validation uses `Profile.validate`'s registered lead check, not a second permission predicate. **Contract target:** add common admission through these owners without altering identity, hiring authority or worker-sibling ownership. |
| `internal/wsagent/runner.go:606–614,683–698,984–1005,3323–3349`; `internal/wsagent/backend.go:182–237`; `internal/wsagent/registration.go:95–111` | Native-mode admission uses the registry and retains Pi's historical input validation. Empty registered modes clear the launch mode and refuse runtime mode control. Optional controls remain per process; unsupported settings retain stop/lazy-resume fallback. Typed categories wrap existing error text/status. **Contract target:** common user-only raises and durable lowering must also cover native-mode changes, without inventing Pi native modes or replacing existing control/failure semantics. |
| `internal/wsagent/support.go:218–229,248–252,415–443,650–651`; `internal/wsdashboard/assets/dashboard-leads.js:42–67,85–134,142–220`; `internal/wsdashboard/assets/rail.js:653–663` | Choices add optional `lead` and native `permission_modes`; UI consumes them with historical Claude/Pi fallback. `leadChoices` currently caches choices. None is a positive common-level signal. **Contract target:** add separate `permission_levels`, fresh preflight after replacement/reconnect and selection-only URLs, while preserving these legacy controls. Worker routes/dialog composition remain sibling-owned. |
| `internal/mcp/session_context.go:9–98` | `sessionAccess{mode}` / `resolveSession` share adoption, not process launch or permission state. Parent/root/scope come from key-store lookup; display nesting and conversation `ParentRef` are not authority selectors. **Contract target:** resolve actual current authority-parent permission without storing a ceiling or treating reporting/grouping as authority. The parent-to-current-selection lookup and missing-parent errors are unimplemented fixture obligations, not guarantees of this resolver. |
| `internal/mcp/server.go:2296–2307,2399–2407,2445–2452`; `internal/mcp/workflow_manual.go:328–330,427–440`; `internal/mcp/playbook_tools.go:1012–1029` | Lookup, hosted bootstrap, re-login, manual adoption and child mint use the core. Rendering/validation, owner rebinding and delivery remain at their existing callers. A future card bootstrap wraps this core; no card store, selector, key scope or input owner is introduced here. CSI Phase 2 moves remaining consumers but cannot authorize permission runtime work. |

**VERIFIED historical provenance; INFERRED consequences, not executed fixtures.**
Re-read the full pinned router dispatch and the Record, registration and label
writer bodies at `90eacda7e4956aded76cfc092d20c41e76d2cc1a`, separately from
current source. The directional matrix's old native-mode/admission coordinates
remain historical, not current edit sites. Its Record read/write shorthand
`record.go:278–301` stops before the final write: the complete functions are
`279–292` and `294–303`, with the destination at `302`. The pinned fixture table
below already uses `279–302`; retain the closing line too when copying bodies.

- Record uses `recordPath` (`internal/wsagent/record.go:258`) and private atomic
  replacement (`307–336`); registration writes/removes only its own file
  (`internal/wsdashboard/registry.go:282–329`, with `SessionDir:229–231`).
  Label set/clear is `internal/wsdashboard/label_route.go:93–106`, using
  `internal/wsdashboard/endpoint.go:97–123` for private replacement. These
  destinations leave a sibling selection file untouched by source inspection;
  exact byte retention, new-reader reopen and unknown-member updates remain
  the unexecuted fixtures below, including CLI homes without an agent record.
- Pinned POST dispatch (`internal/wsagent/http.go:112–231`) implies 405 for
  `/agents/leads/permission` and `/agents/workers`, and 404 for
  `/agents/<id>/settings/permission` and `/agents/workers/permission`, after
  JSON-body and trusted-gate prerequisites pass. Each unsupported branch ends
  before creation/settings calls. This is source reasoning, not an HTTP run.
- Pinned/current pruning (`internal/wsdashboard/registry.go:347–388`) removes
  an expired ordinary directory wholesale, while preserving its runner-owned
  agent-record and terminal-Pi exceptions. Sidecar retention ends with deletion;
  old launch enforcement does not follow from byte retention. Existing tests
  named below are regression anchors, not new sidecar or sandbox coverage.

### Claude/Pi translation contract

**Contract target, unimplemented at the audited snapshot.** This table is a
technical selection, not proof that flags provide a sandbox.
`full` never adds a tool, server, plugin or ws capability to a fixed worker.
Build an effective launch copy rather than writing a narrowed loadout back to
Profile config. Preserve existing unexpected-tool/server/plugin checks.

| Path | Explicit `read-only` | Explicit `workdir-write` | Explicit user `full` | Actual observation / conservative quality |
|---|---|---|---|---|
| Claude lead | Native `plan`; keep the existing native integration/approval owner. | Native `acceptEdits`; no inferred root/network sandbox. | Native `bypassPermissions`, only after trusted user confirmation. | Inspect init/reported mode via existing `active_mode` events (`internal/wsagent/claude.go:236–241,873–875,1061–1071`; `internal/wsagent/backend.go:297–300`). Read-only is at most `partial`; workdir-write and full are `advisory` without broader enforcement evidence. If actual mode differs or is absent, report it and nudge; do not kill solely for the common-intent mismatch. |
| Claude worker | Keep `dontAsk`; intersect native Tools/rules with existing Read/Grep/Glob entries, recompute `--tools`/`--allowedTools` from that copy. Keep ws_tools/servers within existing capabilities. | Keep fixed Tools/rules, ws_tools/servers and `dontAsk`. | Same fixed capability loadout and `dontAsk`; no bypass or extra tool. | Existing init verifies actual tool/server/plugin surface and `dontAsk` (`internal/wsagent/claude.go:267–321`). Read-only is `partial` when read-family narrowing is observed; allowed MCP/plugin behavior is not proven read-only. Workdir-write/full are `advisory`, not root-confined. Existing capability mismatch refusal stays. |
| Pi lead | Preserve the existing lead setup; advise read-only intent rather than claim global tool mediation. | Preserve setup; advise working-directory intent, no native sandbox flag. | Preserve setup; no invented native permission mode. | `piLeadArgs` has no permission control (`internal/wsagent/pi.go:345–367`); ws extension has ungated paths (`agents-plugin-pi/src/execute-gateway.ts:1–103`, repository-relative). All three are `advisory`; inspect available actual loadout evidence without treating visibility as confinement. |
| Pi worker | Intersect native Tools with existing read/grep/find/ls entries; keep ws_tools/servers within capability allowlists. | Keep fixed capability loadout; no new sandbox/network promise. | Same fixed capability loadout; never add native bash/edit/write or MCP access absent from the profile. | Compare actual declared tools/updates using existing checks (`internal/wsagent/pi.go:156–207,255–290`). Read-only is `partial` when narrowed tools are observed, because MCP/extension effects remain outside that proof. Workdir-write/full are `advisory`. Pi unenforceability always launches with the nudge. |

For legacy omission, keep native flags/tools unchanged and do not create a
sidecar. Derive Claude lead `plan` → read-only, `bypassPermissions` → full,
other currently supported modes → workdir-write, with the table's conservative
quality. Derive a worker read-only only when its entire known native/ws surface
is read-only and no unclassified server/plugin is present; the unchanged coding
loadout projects workdir-write/advisory, not a confinement claim. The unchanged
Pi lead setup likewise projects workdir-write/advisory: no native bypass mode
exists to observe. This preserves default lead/worker admission without treating
an available shell as an explicit full grant. An unclassifiable surface remains
unresolved evidence rather than a fabricated full ceiling. Every projection is
`permission_source:legacy-derived`, never written back. Unavailable parent
settings are unresolved admission state, not evidence of full authority.

At every actual launch, load current explicit selection or derive legacy intent,
then resolve the actual parent's current intent and check ordering before backend
start. An above-parent selection gets an explicit admission refusal, not silent
clamping/writeback. Missing required parent gets pending/error; a root has no
such requirement. Serialize selection updates and admission with existing
conversation control ownership so concurrent lower/raise/start cannot use a
stale pre-check. Persist accepted lowering before acknowledging it, and retain
it through stop/revive/restart. On holder reattach, recompute admission without
starting a duplicate process or trusting its old permission ceiling. Existing
process/output recovery ownership stays unchanged.

## Prior Decisions

- 261114-refactor-ws-common-session-interface (2026-11-14, Decisions): "Existing key adoption and parent/owner relationships remain valid; mailbox reachability and profile labels are not permission proofs." — bearing: constrains
- 261114-refactor-ws-hosted-common-contract-pi-claude-parity (2026-11-14, Decisions): "Pi/Claude parity is the release criterion, not simply compiling a generic interface." — bearing: constrains
- 52dc582b (2026-11-13, commit): "Claude keeps settings sources, hooks, permissions and user MCP servers. Effective plugin settings include its POSIX canonical local store; child launch behavior remains unchanged." — bearing: constrains
- 261113-feat-ws-pi-mini-lead-profile (2026-11-13, Decisions): "Selection is by existing `agent.profile.<name>` name; no separate permission enum." — bearing: constrains
- 1a82ac03 (2026-11-15, commit): "A false TurnEnd would release held input, while broadening Notice would change unrelated producers; both alternatives were rejected." — bearing: constrains
- 261115-feat-ws-dashboard-search-drawer-answer-agent (2026-11-15, Result): "tests establish scoped Claude Read/Grep/Glob launch rules and Pi read-only tools at project cwd, not actual Claude Grep/Glob permission enforcement." — bearing: constrains

Prior-decision reconciliation: owner (1002) adds the common permission dimension
without changing the mini-lead capability-profile selection above. Owner
(1050)=a (MAIN 2026-11-15 07:58:13) explicitly permits these bounded T0 controls
before the interface's Phase 2, superseding only that ordering for this slice;
keys, authority lineage and input ownership stay unchanged. Forward: the
interface's Phase 2 absorbs these controls. The autonomous coherence fixes keep
that exception and establish a one-way producer boundary: permission Phase 2
exports the reusable contract; worker-profile Phase 1 then composes worker
routes/dialogs, and worker-profile Phase 2 owns its hire-approval amendment.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsagent/{runner,backend,claude,pi,http}.go and internal/wsdashboard/assets/{dashboard-leads,rail}.js; possible MCP wiring. Permission Phase 2 exports reusable controls and existing-worker admission; new worker routes/dialog composition are sibling Phase 1 consumers. |
| scope.surface | cross-module | Launch/control input and computed enforcement reporting cross runner, native backends, HTTP/MCP and dashboard. Producer order is permission Phase 2 -> worker-profile Phase 1 -> worker-profile Phase 2 hire amendment; no reverse dependency on new worker surfaces. |
| scope.new_public_symbol | yes | MAIN 07:45:14 requires a separate new common field; its exact public wire spelling is a lead technical call. |
| scope.new_type_contract | yes | read-only / workdir-write / full and enforced / partial / advisory need input/projection contracts, with per-launch current-parent admission. |
| scope.test_surface | existing | internal/wsagent/{claude_test,pi_test,lead_http_test,lead_runner_test,profile_spawn_test,record_test}.go; internal/wsdashboard/lead_isolation_test.go. |
| complexity.reuse_points | confirmed | Read Backend.Start/StartSpec/LeadSpec, Runner.backend, State versus Record, native launch builders, HTTP trusted gate and leadRows/leadBody. |
| complexity.side_effect_risk | high | Changes launch permissions and parent admission while preserving native approvals, recovery and capability ceilings. |
| risk.correctness | high | Every revive/restart must recompute parent current-level admission without persisting ceilings or writing derived legacy levels back. |
| risk.fit | high | Current runner/projection seams fit Claude/Pi, but latest authority expands to five phases including currently absent hosted Codex/agy adapters; exact launch producers remain technical settlement. |
| risk.test | high | Existing fixtures do not prove common-level enforcement; new admission, reopen, skew and UI cases must distinguish flags from confinement. |
| risk.security_or_contract | high | User-only escalation and child ceilings cross trusted-route/model boundaries; ignored HTTP inputs and strict Profile/MCP readers affect compatibility. |

## Phases

### Phase 1: Settle the contract and compatibility documentation

Record the literal common enum, lower/raise admission, current-parent lookup,
selection sidecar, support signal, request/State contracts, backend mapping
matrix and directional T0 evidence above. Retain owner 1050's bounded exception
and the interface Phase 2 absorption note. Define the pinned old-server/writer
fixtures before runtime edits. This phase is documentation, not permission to
launch providers, install harnesses or claim confinement from a flag.

Verification: capture and intent review against confirmed (998)–(1004)/(1050)
and the lead's technical settlements; check all new fields/paths in the
compatibility matrix, all five phases and source-backed fixture selectors.

### Result (5b066c229) - 2026-11-15

- Completed the documentation contract for owner (998)–(1004)/(1050): literal
  intent/quality enums, admission requirements, current-selection sidecar,
  request/State/support fields, Claude/Pi mappings and five bounded phases.
- Added exact pinned writer/router and existing fixture provenance, byte-retention
  and new-reader recovery assertions, lifecycle boundaries and zero-write refusal
  prerequisites. Corrected two old-router source ranges after inspection.
- Verification: `git diff --check` passed. Focused Python assertions passed for
  all five phases, contract fields/routes, the execution boundary and every added
  fixture anchor in both the pinned Git object and current source.
  `tickets.verify` passed; its sage-freshness advisory is accepted for this
  documentation-only elaboration and scope restriction, with no policy change.
- Independent lite review of `59fe9b82d..5b066c229` found no correctness or
  test-integrity findings. No runtime compatibility or confinement is claimed.
- Deferred: Phase 2, admission edits and Decision 14 settlement await the lead's
  check with EXEC4 against CSI Phase 1's merged admission sites. Runtime tests,
  live launches, providers, network/install, shared-runtime operations and broad
  tests were outside this authorized documentation slice. The ticket stays ready.

#### Edition (c4975ad76) - 2026-11-15

- Completed the newly dispatched Phase 1 documentation audit only. Recorded
  exact source snapshot `b59c76212329c0d613769191e804ae5eb86f8117` and refresh
  merge `55ef92679`, verified both parents and CSI Phase 1 ancestry, and
  refreshed current source citations onto the landed registry/adoption owners.
- Preserved the entire Decisions section and prior Phase 1 plan/Result
  verbatim. MAIN 16:45:17Z restricts execution to documentation; Phase 2 now
  waits for CSI Phase 2 landing **and** a new MAIN dispatch. This scheduling
  restriction does not amend owner (998)–(1004)/(1050) permission policy.
- Personally read the cited current sites and pinned `90eacda7e` router/writer
  bodies. Recorded source facts separately from future contract requirements
  and unexecuted fixtures. Corrected the Record writer's historical range
  shorthand without rewriting its preserved directional matrix; no old-write,
  HTTP refusal, current-parent or confinement acceptance is claimed.
- Verification: offline `python3 /tmp/permission-doc-assertions.py` passed
  unchanged Decisions/history, five phases, scope/intent assertions, exact
  ancestry/ticket-only diff, 66 current source-site assertions across 22 files,
  28 pinned source assertions and 12 fixture anchors in both snapshots.
  Its initial UI-coordinate assertion failed because two new citations
  missed the function declarations; source diagnosis corrected both citations
  before the passing run. `git diff --check` and `tickets.verify` passed.
  The sage-freshness advisory is accepted for evidence refresh and MAIN's
  execution restriction only; no owner policy or permission behavior changed.
- One fresh independent lite review of `55ef92679..c4975ad76` found no
  correctness, security-contract or documentary test-integrity findings.
  No re-review ran under the one-pass allocation. Unresolved: none.
- Deferred/omitted: runtime edits, executable acceptance fixtures, providers,
  harness launches, installation/download/network/SSH, daemon operations,
  broad tests/CI, push, merge and ticket closure. The ticket remains `ready/`;
  Phases 2–5 remain unimplemented and outside this dispatch.

### Phase 2: Implement and verify Claude/Pi permission controls

Execution is held by the current Execution Boundary: CSI Phase 2 must land and
MAIN must dispatch anew. Owner 1050's bounded policy exception is unchanged;
landing CSI is not itself permission to run this phase. Build on this ticket's
Phase 1 contract and the landed registry/admission owners audited above, not
another admission path. Implement selection persistence, transient
launch input, support preflight, lead/settings selection routes, computed State,
the Claude/Pi mapping table, and reusable permission controls through existing
Runner/UI owners. Include backend admission for existing worker start/control
paths. Do not implement new worker-posture routes or worker dialog composition;
worker-profile Phase 1 consumes this landed phase to deliver those surfaces.
Neither ticket grants a new key scope, authority lineage or input owner.

Apply (1000) on **every path this phase introduces**, including existing worker
start/control, revive/restart, native-mode changes that raise common permission,
and child launches via existing entry points. Resolve actual current parent, distinguish
root from missing parent, persist authorized lowering, and reject agent raises
or above-parent requests. Full selection requires user warning/acknowledgement.
Keep native `permission_mode`, old omitted calls, default `auto`, fixed worker
capabilities and capability mismatch checks. For harness unenforceability,
**launch both leads and workers, Pi included, with the one-line nudge**.

Verification: isolated fixtures cover the complete table, actual native-mode /
tool observations, conservative quality, omitted legacy projection with no
writeback, explicit selection/reopen, durable agent lowering, user-only raise,
full warning cancel/confirm, current-parent lowering between launches,
missing-parent pending/error, root admission, races, holder reattach and no
duplicate process/input. Reusable-control and lead/settings UI fixtures cover
legacy native controls, missing support and daemon-swap refusal without fallback.
Keep a contract fixture for the exported worker permission input/State, without
requiring new worker routes/dialogs to exist. Worker-profile Phase 1 owns the
later route/dialog composition and independence fixtures. Include pinned
old-write/new-reopen and session lifecycle cases below. These checks prove the
exercised wiring, not a universal sandbox.

#### Edition (3a0953c0c) - 2026-11-15

**Phase 2a backend/root-lead slice — UI deferred to Claude mockup pass.**

MAIN's **2026-11-15 17:20:53Z** disposition supersedes the total runtime hold
only for this narrowed Phase 2a. The appended Blocked section records the
historical total stop; its statement that no runtime resumption was authorized
applies before this new disposition. Root-lead Phase 2a is now authorized; only
the worker-lineage slice remains blocked pending (1177). The Phase 2 plan above
and owner decisions remain unchanged. This is an authorized partial delivery, not completion of
Phase 2: **worker current-parent ceiling deferred pending (1177)**;
**UI deferred to Claude mockup pass**. MAIN owns (1177), which remains
unanswered here.

Phase 2a implements lead selection persistence, transient lead launch input,
additive support preflight, lead/settings selection HTTP routes, computed State,
Claude/Pi lead mapping, root-lead admission, durable authorized lead lowering,
user-only lead raises and server-side full acknowledgement. Unenforceable
permission intent produces a launch nudge for both leads and workers. Existing
worker start/control behavior and capability checks stay unchanged; no common
permission level is applied to workers, and worker State/choices must not imply
otherwise. No lineage source, key-parent change or ParentRef authority is added.

Verification is offline and focused (`-run` only), using fake processes/holders,
lead mapping/admission/control fixtures and pinned old-write/new-reopen/router
fixtures. No live harness/provider, full suite, network, installation, Windows
SSH, daemon operation, push or merge is authorized. UI acceptance and the worker
ceiling fixtures are deferred, not passing evidence. The ticket remains `ready/`
without a whole-Phase 2 Result; independent lite review precedes head handoff.

#### Edition (0cb28bb9a) - 2026-11-15

**Phase 2a server contract and scoped verification — UI deferred to Claude mockup pass.**

The authorized root-lead slice landed through `0cb28bb9a`, following the private
selection store in `fc40c36d9` and launch/control spine in `3ac89f37c`.
**worker current-parent ceiling deferred pending (1177)** remains in force;
this records partial delivery only, not a Phase 2 Result or permission to resume
worker application. Independent review and final handoff evidence follow.

Stable server contract for the Claude-owned UI pass:

- `GET /support/choices` adds optional per-harness `permission_levels` with the
  ordered enum only for registered supported lead/settings selection paths.
  Existing `lead`, `available` and native `permission_modes` stay separate.
  This is not worker support; worker State omits all three common fields.
- `POST /agents/leads/permission` requires `root`, `harness` and a nonempty
  `permission_level`. Optional existing launch choices are `model`, `effort`
  and `plugin_isolation`. Explicit `full` also requires
  `permission_full_ack:true`. It returns HTTP 201 with ordinary State.
- `POST /agents/<id>/settings/permission` requires `permission_level`, takes
  optional `model`, `effort` and `permission_full_ack`, and returns HTTP 200
  with State. Validation precedes mutation; accepted selection/settings run
  in one existing conversation-loop action. Worker selection is refused
  without mutation. No worker creation route is added.
- Both selection routes require the existing trusted local/own-Origin gate:
  nonlocal viewers receive 404 and local clients without own Origin receive
  403. Only that server control owner supplies user authority; an
  acknowledgement boolean is not authority. Missing/invalid level, missing
  full acknowledgement, null/nonboolean acknowledgement, unknown fields and
  invalid model/effort receive 400 before writes. New variants deliberately
  do not combine `permission_mode` with common intent, avoiding ambiguous
  competing selectors. Legacy native URLs remain the native-mode interface.
- Selection-bearing requests on legacy creation/settings URLs receive 400
  instead of silently losing intent. Legacy calls with no common fields retain
  their defaults and native behavior. On a session with an explicit common
  selection, native changes cannot bypass user-only raises; an explicit raise
  to full must use the acknowledged selection route. The public Runner
  `SetPermissionLevel` seam permits authorized lowering only. No MCP argument
  or tool schema is changed in this slice.
- Lead State adds computed `permission_level`, `permission_source` (`explicit`
  or `legacy-derived`) and `permission_enforcement`. Missing native observation
  is `advisory`, never `enforced`; observed Claude `plan` makes read-only at
  most `partial`. Pi remains `advisory` with no fabricated native mode.
  Explicit Claude mappings are `plan`, `acceptEdits`, `bypassPermissions`;
  Pi retains its native setup. Omission keeps Claude `auto` and writes no
  selection. Each actual root-lead launch/reattach reads current selection
  through existing process/key ownership; no parent ceiling is stored.
- The one-line nudge is emitted through the existing Runner diagnostic log at
  launch and when Claude reports a changed mode. Worker starts receive a
  diagnostic about their unchanged legacy loadout and deferred common posture;
  an unclassified legacy surface is labelled unresolved, not a granted level.
  Worker tools, native modes, controls, capability checks and lineage are
  unchanged. No user-visible UI warning or sandbox proof is claimed here.

UI obligations remain unimplemented: obtain fresh positive per-harness support
before a selection-bearing request and after reconnect/server replacement;
require an available lead-capable harness; keep native controls usable when
common support is absent; send explicit intent only to the selection variants;
never retry a refused selection on a legacy URL. Warn before full selection,
allow cancellation without a request, and set acknowledgement only on confirmed
submission. Present intent, native observation and conservative enforcement
separately; absent observation is pending/unknown evidence. Do not infer worker
support from lead choices. The server-swap fixture models a caller against the
exact old router; it does not establish production UI/client behavior.

Compatibility evidence is scoped **T0**: the selection writer/reader in
`internal/wsagent/permission_store.go` replaces only private `permission.json`,
retains unknown members and rejects corrupt existing selections. The pinned
`90eacda7e4956aded76cfc092d20c41e76d2cc1a` Record, registration and label bodies
are compared byte-for-byte to Git before isolated execution. Fourteen old-write
checkpoints retain exact selection bytes; the current reader recovers the
lowered value and a later authorized update retains unknown members. CLI homes
without `agent.json`, hosted retention and expired-directory deletion are
covered. Exact old HTTP dispatch returns the documented 405/404 statuses with
zero creation/settings calls and no session files after valid JSON/trusted-gate
prerequisites. Optional State/choices members do not change stored typed shapes;
legacy native-call and extension-mode anchors pass. These fixtures do not prove
old-binary permission enforcement, Windows runtime behavior or arbitrary-version
compatibility. Pinned writer fixtures use Unix replacement helpers and leave
optional current Profile/TurnResult aliases unpopulated.

Verification from `agents-plugin-tool/`, with complete output read:

```sh
GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test ./internal/wsagent -race -v -count=1 -run '^TestPermission'
GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test ./internal/wsagent -v -count=1 -run '^(TestClaudeArgsAllOffByDefault|TestPiArgsAllOffByDefault|TestLeadControlRoutesTakeTheTrustedGate|TestLeadControlRoutesThroughTheDaemonsGate|TestLeadResumesInTheModeTheHarnessReports|TestPiHeldParentSnapshotNeverInventedOnReattach|TestRecordUnknownMembersRoundTrip|TestRegisteredPermissionModeCreateLeadAdmission|TestRegisteredPermissionModeSetAdmission)$'
```

Both nonempty selectors passed (4.632 s and 0.836 s respectively); no baseline
failure appeared in these runs. Earlier leaf fixture corrections concerned a
whitespace-sensitive JSON assertion, an absent worker profile and a missing
fake ws-mcp attachment, not relaxed runtime expectations. The initial runtime
commit was refused solely for an Edition heading suffix; the heading format
was corrected and `tickets.verify` passed before commit. No broad suite,
provider/harness launch, UI acceptance, network, SSH, daemon operation, push or
merge ran. The existing phase plans, owner decisions and prior Results remain
unchanged.

#### Edition (c5f54ed4b) - 2026-11-15

**Phase 2a delivery and independent fix verification — UI deferred to Claude mockup pass.**

The root-lead/backend slice is delivered through `c5f54ed4b`; this is not a
whole-Phase 2 Result. **worker current-parent ceiling deferred pending (1177)**
remains unchanged, and the ticket stays `ready/` for its remaining scope.
No worker permission application, new lineage, UI or later-adapter work landed.

The first independent full-scope lite review of
`4f78654d6..97faaedd55` found two Critical defects and one Important test-oracle
issue. The single correction pass in `c5f54ed4b` addressed all three:

- Permitted native controls now deliver their literal accepted mode through
  the existing process control; common intent is mapped only when selection
  establishes a target or an actual process launches. The regression requires
  literal `auto` delivery for a trusted native request on an explicit
  workdir-write lead, separately checks requested versus observed mode, and
  retains the same holder across reattach.
- Replay completion reconstructs pending explicit selection delivery from the
  current sidecar and native target, then uses the existing safe-boundary
  control owner. The regression lowers full to read-only during an active
  turn, replaces the daemon before any plan observation, replays the old
  bypass observation, and requires literal `plan` delivery to the same holder
  with one launch and no duplicate input. Stop/resume/reopen retain the lower
  selection. A refused replay reconciliation keeps the holder and conservative
  advisory evidence, with diagnostics rather than a kill solely for mismatch.
- Launch expectations independently pin literal `plan`, `acceptEdits` and
  `bypassPermissions`, rather than deriving expected flags from the mapping
  implementation. Pi still has no fabricated native mode.

MAIN's **2026-11-15 17:58:19Z** disposition explicitly required one additional
fresh read-only review after the correction pass, overriding the original lite
no-re-review allocation only for the fixes against `97faaedd55` and the original
three findings. That reviewer inspected
`97faaedd55..c5f54ed4b` and returned **clean: all three fixed, zero remaining
Critical, Important or Minor findings**. Both reviewers used source/test
inspection, not independent test execution; no broader second sweep ran.

Review artifacts:

- Initial findings:
  `/Users/kang-sw/.cache/ws@kang-sw-devenv/proj/17da6bdc@97ab43fd/review-paths/f43f06f5-01-permission-phase2a-lite.md`.
- Fresh bounded fix verification:
  `/Users/kang-sw/.cache/ws@kang-sw-devenv/proj/17da6bdc@97ab43fd/review-paths/2aea2386-01-permission-phase2a-fix-verification.md`.

Final verification from `agents-plugin-tool/`, with complete output read:

```sh
GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test ./internal/wsagent -race -v -count=1 -run '^TestPermission'
GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test ./internal/wsagent -v -count=1 -run '^(TestClaudeArgsAllOffByDefault|TestPiArgsAllOffByDefault|TestLeadControlRoutesTakeTheTrustedGate|TestLeadControlRoutesThroughTheDaemonsGate|TestLeadResumesInTheModeTheHarnessReports|TestPiHeldParentSnapshotNeverInventedOnReattach|TestRecordUnknownMembersRoundTrip|TestRegisteredPermissionModeCreateLeadAdmission|TestRegisteredPermissionModeSetAdmission)$'
GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test ./cmd/ws-mcp -v -count=1 -run '^(TestAgentMCPCommandUsesTheStartingPackagesLauncher|TestAgentMCPCommandFailsWithoutALauncher)$'
```

All three nonempty selectors passed (14.531 s, 2.452 s and 0.907 s respectively).
The last selector compiles the production registration composition and checks
launcher construction using temporary inert files; it executes no launcher or
harness. `git diff --check` passed. The first correction-test run passed both
actual-control assertions but failed a supplemental post-Stop history check:
`fakeHolders.Stop` deletes that holder and its stored history. The test now
checks original-input duplication before deliberate Stop and exact new inputs
on each later holder; no implementation behavior was weakened for the fixture.

The preceding server/JSON contract and bounded T0 evidence remain applicable.
The correction adds only transient reconciliation state, not a stored field,
protocol/schema change, second process/input owner or permission source.
Diagnostics still go to Runner.Log only. Actual warning dialogs, production
client preflight/reconnect behavior, worker admission, live confinement, Windows
execution and all deferred phases remain unimplemented or unverified as already
recorded. No acceptance beyond Phase 2a is claimed.

#### Edition (e9ba42f29) - 2026-11-21

**Decision 15 backend closeout slice — common permission UI remains open.**

This Edition delivers (1876) and (1877) through `e9ba42f29`; it is not a
whole-Phase 2 Result. The dashboard common-permission UI, reconnect-aware
positive preflight and full-warning cancel/confirm flow in (1875) remain for a
separate executor after the mockup. No dashboard UI file changed, and the
ticket remains `ready/`.

`SpawnSpec.PermissionLevel` is now the exported worker permission input
(`internal/wsagent/spawn.go:90-94`). Agent-tool wire input remains the existing
`authority` member and maps to that field. An explicit value is admitted before
creation, stored in the existing private `permission.json`, and read again
before process start; omission keeps conservative legacy derivation without
writeback. Worker State now computes `permission_level`, `permission_source`
and `permission_enforcement` through the existing optional State members
(`internal/wsagent/runner.go:1627-1631`). The existing public lowering seam and
trusted `POST /agents/<id>/settings/permission` route now control workers while
keeping child model/effort fixed and requiring user authority for raises.

Workers created outside `agent.spawn` now resolve their own current explicit or
legacy-derived level and their recorded authority parent's current level at
creation and every fresh process launch. Root workers need no parent ceiling;
missing/unresolved conversation parents and unclassified worker capability
surfaces refuse admission instead of guessing. A parent lowered while a child
process is held does not cause a retroactive kill, but the child's next launch
is refused. The launch nudge is computed only after the stored/derived level is
read and distinguishes root workers from workers whose current-parent admission
passed; neither wording treats a capability loadout as confinement evidence.

The existing server surface needed by (1875) is present: support choices expose
lead common levels, `POST /agents/leads/permission` owns selection-bearing lead
creation, and `POST /agents/<id>/settings/permission` owns trusted common-level
changes, now including workers. The common dashboard control, fresh preflight
after reconnect/server replacement and full-warning client flow are still the
unimplemented UI consumers; no new worker route was added here.

Compatibility remains **T0**. The public Go input is additive and omission
preserves old caller behavior (`spawn.go:90-94,146-152`); State additions use
`omitempty` (`runner.go:1627-1631`); no MCP tool/argument, HTTP route, Record,
Profile, registration or key-store shape changed. Explicit worker input reuses
the private sidecar writer, which retains unknown JSON members
(`permission_store.go:140-163`). The pinned old Record/registration/label
writers preserve exact sidecar bytes at fourteen checkpoints and the new reader
recovers and updates them while retaining unknown members
(`permission_store_test.go:366-409`). Legacy omission writes no sidecar, so an
old binary neither needs to understand a new stored field nor destroys one;
reopening with the new binary recovers every retained explicit selection.

The allocated lite review found one Critical over-admission path and one
Important nudge accuracy issue. The single correction pass in `5e7a8f4bb`
classifies every native/ws/plugin/MCP capability surface before deriving a
legacy worker level and gives root workers authority-accurate wording, with
creation/restart mixed-loadout and root/child nudge regressions. Per the lite
review contract, no re-review ran. Review artifact:
`/Users/kang-sw/.cache/ws@kang-sw-devenv/proj/17da6bdc@c4d08aee/review-paths/6a6d88e8-01-261115-feat-ws-permission-profile-decision15-backend-lite.md`.

Verification from `agents-plugin-tool/`, with complete output read:

```sh
GOCACHE=/tmp/ws-permission-go-cache GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test ./internal/wsagent -race -v -count=1 -run '^TestPermission(WorkerInputStateAndControl|LegacyWorkerMixedLoadouts|CurrentParentAdmission|MissingParentAdmission)$'
GOCACHE=/tmp/ws-permission-go-cache GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test ./internal/wsagent -race -v -count=1 -run '^TestPermission'
GOCACHE=/tmp/ws-permission-go-cache GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test ./internal/wsagent -race -v -count=1 -run '^(TestAgentSpawnCapsAtAHostedLeadsLevel|TestRefusedWakeUnderAHostedLead|TestHeldWorkerChildLoadsBeforeLowerCeilingParent|TestPermissionWorkerInputStateAndControl|TestPermissionCurrentParentAdmission|TestPermissionMissingParentAdmission)$'
GOCACHE=/tmp/ws-permission-go-cache GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test ./internal/mcp -v -count=1 -timeout 45m -run '^TestAgent'
```

All four nonempty selectors passed (1.970 s, 6.164 s, 2.174 s and 2.141 s).
The MCP selector required local-loopback sandbox permission for its temporary
HTTP listeners; it used no provider or network service. `git diff --check`
passed. No live provider, daemon stop/swap, broad suite, dashboard UI test,
network service, push or merge ran.

Dogfood note: the first lite reviewer was spawned read-only, but the rendered
review wrapper requires writing its allocated findings artifact; it returned a
summary while leaving that artifact empty. A fresh workdir-write reviewer
recovered the review. Separately, `workflow_manual` was unavailable from the ws
tool surface, so this run used the repository manuals and rendered playbooks.

### Phase 3: Add the Codex per-run permission launch path

Deferred until the Codex adapter provides its settled exact-version launch
contract and separately authorized implementation; its idea status does not
block Phases 1–2. Build on this ticket's contract and the actual adapter producer,
not a guessed universal API. Map common intent into per-run sandbox/approval
settings, verify reported actual settings, and expose honest quality for MCP
and other paths outside proven confinement. No global Codex profile/config edit,
model-catalog substitution or silent auto-approval. Reuse the same sidecar,
preflight, State and every-launch admission; preserve existing Pi/Claude behavior.

Verification: version-pinned fake/protocol fixtures for each level, configured
versus observed settings, unavailable observations, nudge-and-launch, legacy
omission, child admission and T0 callers/storage. Provider-backed sandbox checks
require separate authorization. Exact adapter methods come from its settled
contract, not this documentation capture.

### Phase 4: Add the agy per-run permission launch path

Deferred under the Antigravity adapter's owner-resume/account and exact-version
contract gates; not a prerequisite for Phases 1–2. Translate through the admitted
public adapter with per-run settings, actual launch verification and the same
common contract. Keep unknown deny/sandbox semantics explicit, preserve richer
approval choices, and avoid global settings or silent approval substitution.

Verification: pinned adapter/protocol fixtures for the three mappings, actual
settings, partial/advisory reporting, launch despite unenforceability, existing
admission and T0 compatibility. Account-backed/live acceptance remains separately
authorized. This phase does not implement the whole adapter or waive its gates.

### Phase 5: Complete cross-path child ceilings and ws launch-approval integration

Integrate the established permission contract across remaining ws launch,
bootstrap, revive and approval-dialog consumers as their producers become
available, including future-adapter children and nested trees. The worker
sibling's exact selection-bearing hire-approval amendment is owned by its
Phase 2, not an undefined producer in this generic integration phase. This
broadens coverage; it does **not** postpone any Phase 2 child/raise check. Keep the same
per-launch current-parent lookup and durable current selection; never persist a
parent ceiling or infer permission ancestry from rail/display grouping.

Verification: cross-entry/nested-child fixtures, parent lowering between
launches, resume/revive and concurrent control changes, unresolved actual
parents, authorized lowering, rejected model raises (a `ws/agent.spawn` child at
its parent's level, `full` included, is Decision 3's exception, not a raise),
warning/approval correlation,
fixed capability ceilings and no second input owner. Add optional MCP inputs
only with discovery gating and unchanged legacy calls. Re-grade every added
surface T0; existing native approval behavior and Pi/Claude regressions remain.

## Focused Verification Contract

Commands below are implementation acceptance selectors, **not runs claimed by
this docs-only revision**. Run from `agents-plugin-tool/` when implementation
is authorized; the new `TestPermission*` names are fixtures to add, not existing
coverage. Read complete output and reject empty selector runs.

- Existing regression anchors:
  `go test ./internal/wsagent -count=1 -run '^(TestClaudeArgsAllOffByDefault|TestPiArgsAllOffByDefault|TestLeadControlRoutesTakeTheTrustedGate|TestLeadResumesInTheModeTheHarnessReports|TestPiHeldParentSnapshotNeverInventedOnReattach|TestRecordUnknownMembersRoundTrip)$'`.
- New Phase 2 fixtures:
  `go test ./internal/wsagent -count=1 -run '^TestPermission(LaunchMatrix|LegacyNoWriteback|SelectionLoweringRecovery|CurrentParentAdmission|MissingParentAdmission|FullUserGate|RouteSupportAndSwap|SidecarOldWriteNewReopen|SidecarLifecycle)$'`.
- Existing dashboard entry point, extended with permission assertions:
  `go test ./internal/wsdashboard -count=1 -run '^TestLeadPopoverLaunchBehavior$'`
  (`testdata/lead-popover.test.cjs`); include absent support, three levels,
  `permission_full_ack`, cancel and current State quality for reusable controls
  and lead/settings consumers. Worker-profile Phase 1 separately tests composed
  worker dialog payloads: default permission goes to `/agents/workers`, explicit
  `permission_level` plus required full acknowledgement goes only to
  `/agents/workers/permission`.

### Pinned fixture construction

Pin the old writer/router to **90eacda7e4956aded76cfc092d20c41e76d2cc1a**.
The following provenance was inspected in that Git object; paths are relative to
`agents-plugin-tool/`. These are fixture definitions, not executed test results.

| Planned fixture / assertion | Pinned implementation | Existing fixture anchor |
|---|---|---|
| `SidecarOldWriteNewReopen`: Record read/modify/write preserves the separate selection bytes. | `internal/wsagent/record.go`, `readRecord` / `writeRecord` (279–302), writing only `agent.json`. | `internal/wsagent/record_test.go`, `TestRecordUnknownMembersRoundTrip`. Its unknown-Record-member assertion is an anchor, not sidecar coverage. |
| `SidecarOldWriteNewReopen`: registration replacement and removal leave selection intact, including a CLI home with no agent Record. | `internal/wsdashboard/registry.go`, `WriteRegistration` / `RemoveRegistration` (282–328), touching only `registration.json`. | `internal/wsdashboard/index_test.go`, `TestRegistrationRoundTripIgnoresUnknownFields`; add the removal case. |
| `SidecarOldWriteNewReopen`: setting and clearing a label leave selection intact. | `internal/wsdashboard/label_route.go`, `writeSessionLabel` (93–105), touching only `label.json`. | `internal/wsdashboard/label_route_test.go`, `TestLabelRouteSavesShowsAndResets`. |
| `SidecarLifecycle`: live hosted records retain selection; ordinary expired session-directory pruning removes it with the session. | `internal/wsdashboard/registry.go`, `PruneRegistrations` (347–385). | `internal/wsdashboard/index_test.go`, `TestPruneRegistrationsDropsOnlyExpiredSessions`, `TestPruneRegistrationsAgesRetiredLeadsByTheirRow`, `TestPruneRegistrationsSparesAgentConversations`. |
| `RouteSupportAndSwap`: unsupported selection paths never reach creation/settings, with the status outcomes below. | `internal/wsagent/http.go`, `(*Runner).serve` dispatch (112–231) and permissive typed `decode` (268–286). | `internal/wsagent/lead_http_test.go`, `TestLeadControlRoutesTakeTheTrustedGate`; retain its local/own-Origin gate setup. |

For the later implementation, retain exact pinned function bodies and required
helpers in isolated test fixtures with their source path and full commit ID;
compare them to the Git object rather than substituting the new writer/router.
Use temporary session directories and fake backends/holders. No old executable,
provider, installed harness or shared daemon is needed. Preserve the old route's
JSON-body and trusted-gate prerequisites so a 403 does not masquerade as the
expected unsupported-path refusal. Assert zero creation/settings calls and no
new session/selection writes, not merely the response status.

For each old-writer case, seed a current explicit selection (including a durably
lowered value) and an unknown sidecar member, save the exact bytes, perform the
pinned old write, and compare bytes before the new reader reopens. Assert that
reopen reads the same current selection and that the unknown member survives a
subsequent authorized update by the new writer. Separately cover missing sidecar
with no legacy writeback, malformed/invalid and unreadable existing selections,
stop/revive/restart, and hosted and CLI session homes. Selection retention ends
with ordinary session deletion; old-binary enforcement is not established.

Pin old HTTP route dispatch in a fixture and assert these decisions with
**zero creation/settings calls**, not just status checks:

| POST path | Pinned baseline response |
|---|---|
| `/agents/leads/permission` | 405 |
| `/agents/<id>/settings/permission` | 404 |
| `/agents/workers` (sibling default/legacy route) | 405 |
| `/agents/workers/permission` (sibling explicit selection) | 404 |

Permission Phase 2 verifies its lead/settings paths and exported consumer
contract without depending on new worker route implementations. Worker-profile
Phase 1 owns both worker route assertions and an intermediate-server fixture
that accepts default `/agents/workers` but rejects `/agents/workers/permission`
with zero creations. An explicit-selection client must never retry the default
route, including after a positive preflight followed by a server swap.

Cover preflight absent/present, replacement after preflight, unsupported tool
schema, unchanged old requests to new handlers and preserved native display.
Sidecar T0 is byte retention and new-reader recovery, not proof that an old
binary enforces the new permission feature.

## Sage Review Round 1 (2026-11-15)

### Design Reviewer — block

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | Cross-ticket [261115-feat-ws-permission-profile, 261115-feat-ws-worker-profile-session-spawn]: The tickets assign incompatible meanings to the worker creation route. | critical | autonomous |
| 2 | Cross-ticket [261115-feat-ws-permission-profile, 261115-feat-ws-worker-profile-session-spawn]: Permission Phase 2 consumes surfaces owned by its dependent. | important | autonomous |

### Completeness Reviewer — pass

| # | Title | Severity |
|---|-------|----------|

## Sage Review Round 2 (2026-11-15)

MAIN's **2026-11-15 17:12:58Z** dispatch authorized Phase 2 backend/runtime
only after CSI Phase 2 landed. The worker inspected exact base
`4f78654d6ca2113ad13d610f57dc05cd0696980b` on
`impl/develop/foyer-cause-shore` and stopped under **stop (c)** before runtime
edits. The lead accepted the stop; no runtime resumption is authorized.

**VERIFIED — the required actual authority-parent relation is missing from the
inspected hosted-worker seams.** Paths below are relative to
`agents-plugin-tool/` at that base.

- Production supplies `AgentKeyStore` to the Runner
  (`cmd/ws-mcp/dashboard.go:262–273`). Worker minting deliberately creates a
  parentless delegate key; `ensure` rejects a nonempty key parent and recreates
  a missing key without one (`internal/mcp/agent_keys.go:12–17,27–30,93–119`).
- Top-level Pi spawn passes a reporting `ParentRef`, not the calling key or a
  separate authority-parent input (`internal/mcp/pi_tools.go:154–156,307–327`;
  `internal/wsagent/spawn.go:66–109`). Nested spawn likewise passes only the
  holder's `ParentConversation` reply-id (`internal/wsagent/children.go:312–322`).
- CSI explicitly excludes conversation `ParentRef` from authority selection
  and projects lineage from the key store
  (`internal/mcp/session_context.go:9–12,114–120`). Tokens identify their holder
  (`internal/wsagent/children.go:108–186`); holder metadata records worker
  identity/capability, not its authority parent (`internal/wsagent/holder.go:82–89`).
  Worker restart calls the same parentless-key `Ensure`
  (`internal/wsagent/runner.go:2174–2188`).

**INFERRED — implementation consequence.** These sources cannot supply the
actual current parent required by Decision 3 after restart without promoting
`ParentRef` to authority or introducing/changing durable lineage. Neither is
approved. An empty worker-key parent does not establish that the spawned
conversation is an authority root. This bounded source inspection, corroborated
by an independent read-only researcher, is not an exhaustive absence proof.

**Awaiting MAIN's contract/design disposition.** A separately approved durable
actual-parent binding, supplied at authenticated spawn boundaries with explicit
legacy missing-parent treatment, was suggested as a prerequisite correction.
This is an unsettled suggestion, not an approved design, storage choice or
permission-policy amendment. Compatibility also requires settlement: adding
parents to worker keys would conflict with the inspected `Ensure` contract;
no transparent alternative or T0 acceptance is established by this inspection.

Phase 2 backend runtime and UI remain unimplemented. No permission-feature
fixtures or acceptance ran, and no Phase 2 Result is recorded. The ticket stays
`ready/`; UI remains deferred to the Claude mockup pass, and Phases 3–5 remain
outside this dispatch. Owner decisions, phase plans and existing Results are
unchanged.

## Owner Decisions (2026-11-16)

- (84)=a (2026-11-17, recorded in Decision 3 and
  261117-feat-ws-agent-tools-mailbox-routing Decision 13): for
  `ws/agent.spawn` children only, the daemon-written Runner spawn record is
  the (1177) authority-parent record.

- (1177)=a: the worker-side ceiling uses a separate authority-parent record,
  recomputed from the parent's current permission after restart (see
  261115-feat-ws-worker-profile-session-spawn).
- (1180)=ok: the permission-picker mockup (develop `4352415f3`) is approved
  as drawn. The new-session default is "default (existing behavior)", starting
  in `auto`. The native permission mode stays under "Advanced". There is no
  silent fallback when a mode is unsupported. The header permission chip
  stays. The "full" level opens a warning that needs its checkbox before
  start. Workers see nothing until (1177) lands.
