---
title: "Common permission intent and truthful enforcement for Claude/Pi sessions"
related:
  261115-research-ws-unified-agent-tool-surface: "research context only; owner 853 approved capture, not permission policy"
  261114-refactor-ws-common-session-interface: "owner 1050 permits bounded T0 controls first; its Phase 2 later absorbs them without changing keys, lineage or input ownership"
  261115-feat-ws-session-hire-approval: "worker-profile Phase 2 owns the exact selection-bearing hire-approval amendment; hiring authority and display grouping remain unchanged"
  261115-feat-ws-worker-profile-session-spawn: "its Phase 1 follows permission Phase 2 and owns worker routes/dialog composition; its Phase 2 owns the hire-approval amendment"
  261115-feat-ws-hosted-codex-adapter-contract: "deferred Codex permission mapping belongs with its adapter; not a prerequisite here"
  261114-feat-ws-hosted-antigravity-acp-adapter: "deferred agy permission mapping belongs with its adapter; not a prerequisite here"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 96c5564256438fb6
sage-review-completeness-reviewed: 96c5564256438fb6
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

MAIN authorizes Phase 1 documentation only in this invocation. Phase 2 and
Decision 14's delivery boundary await the lead's check with EXEC4 against CSI
(`261114-refactor-ws-common-session-interface`) Phase 1's merged admission
sites. The contract below records intent and acceptance requirements; completing
Phase 1 neither settles that coordination nor authorizes admission edits.

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
   add a parent-change process-killing/restart mechanism.
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
    claimed by the existing trusted gate.
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
14. **Technical delivery boundary.** Permission Phase 2 produces reusable
    permission controls, backend admission for existing workers, and lead /
    settings selection routes. It does not produce new worker-posture routes
    or compose the worker dialog. Worker-profile Phase 1 follows permission
    Phase 2 and owns both worker creation routes, posture/owning-lead/bootstrap
    behavior and dialog composition using these controls. Worker-profile
    Phase 2 owns its exact selection-bearing hire-approval amendment; it does
    not wait for an undefined permission Phase 5 producer.

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

- `internal/wsagent/profile.go:74–135,292–305`: capability defaults are off;
  lead profiles load the user's setup and exclude capability fields;
  `permission_mode` is native Claude state, absent mode defaults to `auto`.
  `runner.go:630–645` clears it for non-Claude leads. These are current facts,
  not a selected common-level default or a `Pi = full` policy.
- `internal/wsagent/claude.go:102–130,134–210,264–318`: leads pass native mode
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
- `internal/wsagent/runner.go:2126–2127` passes `WS_MCP_ALLOWED_TOOLS`;
  `internal/mcp/server.go:714–717,5685–5693` enforces tool admission. Existing
  capability narrowing compares tools and exact MCP server definitions
  (`internal/wsagent/profile.go:433–459`); it does not compare permission levels.
- `internal/wsagent/http.go:127–146,199–207` retains trusted local/own-origin
  controls; `runner.go:926–944` currently changes native modes only for Claude
  leads. A trusted-route check is not proof that an arbitrary request came
  from a human; the selected warning/gate contract does not claim otherwise.
- `261114-refactor-ws-common-session-interface`, Decisions 2/7 and Phase 1:
  labels are not permission proofs; backend descriptors belong at the current
  registry boundary; do not map Claude strings onto another harness by
  assumption.

### Launch, recovery and UI seams

- `Backend.Start` / `StartSpec` / `LeadSpec` (`internal/wsagent/backend.go:21–82`),
  `Runner.backend` (`runner.go:743–757`) and computed State (`runner.go:1053–1146`)
  are the implementation owners. Native `permission_mode` can change from
  accepted harness reports (`runner.go:2695–2705`); it is not immutable intent.
- The label sidecar pattern is `internal/wsdashboard/label_route.go:21–31,75–105`.
  Permission state shares the session directory's existing lifetime. Reopen
  retains it (`internal/wsagent/handoff.go:231–288`); pruning can delete an
  expired session directory (`internal/wsdashboard/registry.go:331–385`).
  Stop/Restart and native start paths are `runner.go:875–897,2067–2077,2202–2244`.
  Reattach uses a capability snapshot (`runner.go:2339–2350`), not a stored
  permission ceiling; fixtures cover changed/missing snapshots
  (`internal/wsagent/profile_spawn_test.go:202–293`).
- UI reuse is `leadRows` / `rpLeadMake`
  (`internal/wsdashboard/assets/dashboard-leads.js:73–110,120–184`) and
  `WsRail.leadBody` (`internal/wsdashboard/assets/rail.js:609–619`).
  Extend choices/State rather than adding another launch owner. Worker posture,
  `owning_lead`, `intro`, `worker.bootstrap`, default `/agents/workers`, explicit
  `/agents/workers/permission` and `/support/choices.worker` remain sibling-owned
  Phase 1 consumers after permission Phase 2, not producers needed by it.

### Claude/Pi translation contract

This table is a technical selection, not proof that flags provide a sandbox.
`full` never adds a tool, server, plugin or ws capability to a fixed worker.
Build an effective launch copy rather than writing a narrowed loadout back to
Profile config. Preserve existing unexpected-tool/server/plugin checks.

| Path | Explicit `read-only` | Explicit `workdir-write` | Explicit user `full` | Actual observation / conservative quality |
|---|---|---|---|---|
| Claude lead | Native `plan`; keep the existing native integration/approval owner. | Native `acceptEdits`; no inferred root/network sandbox. | Native `bypassPermissions`, only after trusted user confirmation. | Inspect init/reported mode via existing `active_mode` events (`claude.go:232–237,849,1040`; `backend.go:297–300`). Read-only is at most `partial`; workdir-write and full are `advisory` without broader enforcement evidence. If actual mode differs or is absent, report it and nudge; do not kill solely for the common-intent mismatch. |
| Claude worker | Keep `dontAsk`; intersect native Tools/rules with existing Read/Grep/Glob entries, recompute `--tools`/`--allowedTools` from that copy. Keep ws_tools/servers within existing capabilities. | Keep fixed Tools/rules, ws_tools/servers and `dontAsk`. | Same fixed capability loadout and `dontAsk`; no bypass or extra tool. | Existing init verifies actual tool/server/plugin surface and `dontAsk` (`claude.go:264–318`). Read-only is `partial` when read-family narrowing is observed; allowed MCP/plugin behavior is not proven read-only. Workdir-write/full are `advisory`, not root-confined. Existing capability mismatch refusal stays. |
| Pi lead | Preserve the existing lead setup; advise read-only intent rather than claim global tool mediation. | Preserve setup; advise working-directory intent, no native sandbox flag. | Preserve setup; no invented native permission mode. | `piLeadArgs` has no permission control (`pi.go:345–367`); ws extension has ungated paths (`execute-gateway.ts:1–103`). All three are `advisory`; inspect available actual loadout evidence without treating visibility as confinement. |
| Pi worker | Intersect native Tools with existing read/grep/find/ls entries; keep ws_tools/servers within capability allowlists. | Keep fixed capability loadout; no new sandbox/network promise. | Same fixed capability loadout; never add native bash/edit/write or MCP access absent from the profile. | Compare actual declared tools/updates using existing checks (`pi.go:156–207,255–290`). Read-only is `partial` when narrowed tools are observed, because MCP/extension effects remain outside that proof. Workdir-write/full are `advisory`. Pi unenforceability always launches with the nudge. |

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

### Phase 2: Implement and verify Claude/Pi permission controls

Depends on this ticket's landed Phase 1, not the future foundation or adapter
Results under owner 1050's exception. Implement selection persistence, transient
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
parents, authorized lowering, rejected model raises, warning/approval correlation,
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
