---
title: Show the observed current model for dashboard sessions and subagents
related:
  261115-bug-ws-claude-binary-provenance-and-resume-model: complementary prevention and launch provenance; observed identity would expose recurrence
  261110-feat-ws-dashboard-agent-panel-telemetry: landed per-agent telemetry and output-rate semantics to reuse
  261110-feat-ws-dashboard-session-rail: landed RailRow and session association to reuse
  261110-feat-ws-dashboard-headless-lead-sessions: hosted Claude/Pi evidence path; distinct from CLI adapters
  261002-feat-pi-model-output-tps-display: established output-rate definition; not a new throughput metric
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: ec3915233443fdfe
sage-review-completeness-reviewed: ec3915233443fdfe
---

# Show the observed current model for dashboard sessions and subagents

## Background

The owner cannot tell which model a session or subagent is actually running
from the dashboard. MAIN relayed owner request 813 on 2026-11-14 at 23:02:22Z:
a resumed Claude session reportedly used `claude-opus-4-6` for approximately
250 turns despite the configured `opus` alias now referring to
`claude-opus-5-5`, and subagents inherited that model. This incident is a
reported motivation, not independently verified transcript evidence.

Expose observed execution identity so a configured alias or resume choice
cannot silently conceal the model that generated the latest assistant output.

MAIN's later investigation motivated
`261115-bug-ws-claude-binary-provenance-and-resume-model` (owner-approved idea
832): explicit binary/version provenance and fresh-resume `--model` hardening.
Keep that prevention work separate from this observational display; showing the
actual model would make a recurrence visible without changing which model runs.

## Decisions

- **Owner request 813: observed model identity.** Show the current model ID
  separately for each session and each subagent, using the harness transcript's
  latest assistant-message model field or equivalent evidence for Claude, Pi
  and Codex. A configured alias alone does not establish the observed model.
- **(814), owner-confirmed: concrete identity and evidence provenance.** Display
  the concrete observed model ID, not a configured alias. Display explicit
  unknown when no model is observed. Where only prior evidence is available,
  show `last observed: model · time` provenance; never substitute the configured
  alias as the actual model. MAIN relayed the owner's exact answer on
  2026-11-14 at 23:07:47Z: `814=ok`, with the additional instruction that the
  concrete model, not the settings alias, must be shown to reveal this incident.
- **(815), owner-confirmed: visible changes.** Use a compact previous-to-current
  indicator with the evidence timestamp. Show the full change history only in
  session detail. Add no alert or automatic model-control behavior. MAIN
  relayed the owner's exact answer `815=ok` on 2026-11-14 at 23:07:47Z.
- **(819), MAIN-relayed scope addition: recorded effort.** Show effort next
  to the model only where the harness records it; omit absent effort and never
  infer it from defaults or aliases. The agent panel's separate effort stat
  already landed in `07c94a095` / `f8d26d5a3`, with Korean wording in
  `57f297e79`; reuse it rather than implementing it again. This adds no token
  accounting mechanism.
- **(835)=a, owner-approved presentation.** MAIN's 2026-11-15T03:12:00Z grant
  records selection of hover-popup A, the compact card (panel p159, e2686).
  Use the tracked A-only light/dark references below. Keep session rows and
  their last-message presentation at existing height; place the observed-model
  information in the hover popup, not a replacement second row or enlarged
  header. Keep the current agent panel's layout; fold its existing model/effort
  display and model hover into the same per-agent evidence/provenance contract.
  B, the taller section card, is rejected.
- **(836)=ok, owner-approved behavior.** The popup is read-only and
  pointer-transparent. An idle session shows the last turn's t/s, labelled as
  such; absent evidence stays absent. Five-hour and weekly limits remain
  exclusively in the bottom meter, not in this popup. Remove only the rail
  row's reply-ID tooltip because it overlaps the popup; retain all other
  tooltips. These rules override unsettled notes in the preserved mockup source.
- **Supersession.** (835)/(836) supersede (816), (824) and (825). Earlier
  second-row placement and pending-variant requirements are historical, not
  implementation gates. (814)/(815)'s provenance and change-history contract
  remains in force.
- **Compatibility: T0 additive boundary.** Dashboard assets are T0. The
  separate telemetry phase adds optional lead evidence/history fields through
  the existing intake/snapshot path; keep existing agent telemetry meanings and
  legacy lead-agent rejection intact. Unknown optional fields must be tolerated,
  missing fields must degrade honestly, and an older writer must not destroy
  retained new history. See the source audit below for the writer caveat. No
  new tracker event, required MCP argument, config/rsrc field, schema/protocol
  bump or T2 is approved. Escalate a design that cannot meet T0 to the lead.
- **Preparation authority.** MAIN owns policy and decision IDs. This revision
  prepares `todo/` documentation for lead-owned fact population, independent
  design/completeness gates and later readiness; it authorizes no implementation
  or runtime activation. The badge lane retains higher priority and is separate.
  Owner (899)=a's standing direction removes duplicated lead reverification
  and progress ceremony; hand back the substantive documentation, not a request
  to repeat asset/log checks.

## Constraints

- Preserve the distinction between configured/requested model and observed
  execution model. Do not turn display work into model switching, resume
  correction, provider selection or automatic enforcement.
- Apply approved A and (836), preserving compact geometry and both themes.
  Per MAIN's 03:17:55Z UI direction, implementation runs relevant checks
  (including affected wsi18n checks) then returns a head; at most one quick
  sanity screenshot, with no screenshot-proof rounds or viewport matrix.
  Owner-visible result/reference side-by-side comparison after merge and an
  authorized swap is visual acceptance, followed by fast fixes from feedback.
  The A/B source is archival evidence, not permission to implement B.
- Reuse existing context, cost, turn-time and per-agent telemetry rather than
  collecting parallel totals. Label estimates/incomplete values and last-observed
  evidence honestly. Snapshot heartbeat time is not a model-observation time.
- Preserve per-session and per-subagent identity across the supported harnesses;
  one parent's model is not proof of a child's model.
- Treat response evidence, request/RPC state, spawn input and configuration as
  distinct provenance. Existing adapter `Model` fields alone do not prove the
  latest assistant response's concrete identity. A parent's evidence never
  supplies a missing child's value; an unsupported path shows explicit unknown.
- Keep model switching, binary/resume hardening, badge work and asset-split work
  outside this ticket. Source inspection below is pinned to landed `develop`,
  not this retained documentation checkout's older source.
- Read `ai-docs/manuals/shipped-surface-boundary.md` before edits under shipped
  package paths and `ai-docs/manuals/release-compatibility.md` before internal
  tooling changes. If MCP paths are touched, also read
  `ai-docs/manuals/ws-mcp.md`; if shared skills/resources are touched, apply the
  declared skill-authoring and wsflow-mirroring manuals.

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin/hooks/`, `agents-plugin-wsflow/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)

## Approved Reference and Provenance

Assets live in
[`../assets/261115-feat-ws-dashboard-observed-session-model/`](../assets/261115-feat-ws-dashboard-observed-session-model/).

- Normative layout A: [dark](../assets/261115-feat-ws-dashboard-observed-session-model/824-session-hover-varA-1280-dark.png)
  and [light](../assets/261115-feat-ws-dashboard-observed-session-model/824-session-hover-varA-1280-light.png),
  each 1280 × 800. The compact card places identity/phase and reply ID above
  concrete model/recorded effort, context, three stats (t/s, turn time, child
  count), and a short cost/start/last-tool footer.
- A-specific supplementary evidence:
  [stopped/last-observed](../assets/261115-feat-ws-dashboard-observed-session-model/hover-a4.png)
  and [model change](../assets/261115-feat-ws-dashboard-observed-session-model/chg-a.png).
  The change crop includes mockup provenance dots; they are not product UI.
- `body.html`, `new.css` and `script.html` are exact source fragments copied
  from `/tmp/mk824-build/`, not an A-only or self-contained runnable page.
  They contain both A and rejected B, fixture values, older source coordinates
  and pre-(836) open notes. Preserve their bytes for provenance; this ticket's
  decisions select A and override those notes. No B/comparison image is an
  approved reference. See [asset provenance](../assets/261115-feat-ws-dashboard-observed-session-model/README.md)
  for the approved-A/source-fragment distinction and origin.

## Source Evidence

**VERIFIED:** read-only `git show` / search of landed `develop` at
`ddf95a78e42108133d48056534375752f83a98b9`; line citations below refer to that
revision, not documentation HEAD `742a82487`. Search symbols are the navigation
anchors. No build, test, browser or live harness probe was run for preparation.

| Reuse / boundary | Evidence at the pinned revision | Consequence |
|---|---|---|
| RailRow and session association | `agents-plugin-tool/internal/wsdashboard/pageview_rail.go:115-175,556-588` (`RailRow`, `railRow`) | Existing reply ID, harness, label, working/approval/failure, child count, last tool and worktree fields; snapshot already available to the builder. Context/cost/turn figures are not yet RailRow fields. |
| Context, cost and turn-time evidence | `agents-plugin-tool/internal/wslive/snapshot.go:196-240,260-295` (`UsageContext`, `UsageCost`, `Activity`, `Lead`); `agents-plugin-tool/internal/wsdashboard/pageview_band.go:94-122,135-154,174-175` (`bandView`, `contextBar`, `costText`) | Plumb existing snapshot figures/formatters into the popup; retain unknown context, cache-hit and cost estimate/incomplete semantics. `Lead` has turn start/origin and previous-turn duration, but no model/effort/t/s/history. |
| Existing agent telemetry | `agents-plugin-tool/internal/wslive/snapshot.go:297-324`; `agents-plugin-tool/internal/wslive/state.go:124-132,569-600,627-629,649-668` | Model/effort/context/rate already exist per child. Lead agent events explicitly reject telemetry; absent child fields retain prior values and ended children clear rate. Do not relabel these semantics as new observed-response proof. |
| Landed effort presentation | `agents-plugin-tool/internal/wsdashboard/assets/dashboard.tmpl:211` (`astat`); `assets/dashboard.css:227-240` under the same directory; `pageview.go:233-253`; commits `07c94a095`, `f8d26d5a3`, `57f297e79` | Model and effort are separate spans, including narrow layout. Preserve this landed work and existing non-reply-ID titles; model hover must distinguish recorded/requested versus observed evidence. |
| Claude evidence seams | `agents-plugin/hooks/claude-mod/dashboard.ts:534-537,1076-1090,1496-1505,1582-1584`; `agents-plugin-tool/internal/wsagent/claude.go:698-707` (`claudeMessage`) | Mod child telemetry uses request model/effort, initially seeded from spawn; the lead branch does not collect it. Hosted assistant message has a concrete model field. Reuse parsing/rate seams but do not call a spawn alias an observed response. |
| Pi evidence seams | `agents-plugin-pi/src/spawner.ts:849-855`; `agents-plugin-pi/src/dashboard-feed.ts:365-395`; `agents-plugin-tool/internal/wsagent/testdata/pi-tool-turn.jsonl:6,12` | Pi child's RPC state supplies provider/model and thinking level, and its own display feeds agent events. Hosted fixtures contain assistant model/thinkingLevel. Distinguish state from latest-output evidence and preserve direct/deep child association. |
| Intake and snapshots, actual cross-version surfaces | `agents-plugin-tool/internal/mcp/live_snapshot.go:213-214,502,518-530,580-603`; `agents-plugin-tool/internal/wslive/snapshot.go:31-33,411-420`; `agents-plugin-tool/internal/wslive/state.go:72-74` | Event JSON and snapshot JSON are extensible; missing fields read empty and unknown fields are ignored. Optional kinds have capability advertisement. The writer marshals typed state, not an unknown-field-preserving round trip: history preservation across older writers is not established. |
| Tooltip boundary | `agents-plugin-tool/internal/wsdashboard/assets/dashboard.tmpl:366-368` (`hitip`, `rrow`) | Remove only the labelled rail `.rid` reply-ID title. Keep harness/status, timestamps, child-message, controls and other titles; do not apply a blanket tooltip deletion from the mockup notes. |

**INFERRED:** optional telemetry/provenance additions can meet T0 through the
existing intake/snapshot seams, but reader tolerance alone does not establish
writer-safe history storage. Implementation design must demonstrate additive
storage and older-writer behavior before claiming compatibility. Existing
model controls are separate (`agents-plugin-tool/internal/wsagent/claude.go:395-419`,
`pi.go:587-596`); observing identity must not invoke them.

**Bounded fact population (2026-11-15):** `git rev-parse develop` confirmed
`ddf95a78e42108133d48056534375752f83a98b9`; all additional source references
below are at that exact landed revision. These are implementation seams and
fixture contents, not current installed-harness acceptance.

| Path / evidence class | Source-grounded mapping and remaining boundary |
|---|---|
| CLI Claude, own and child response | `agents-plugin/hooks/claude-mod/register.ts#L1684-L1689` routes `turn.step` by `agentId`, with request model/effort separate from the step-end result. `dashboard.ts#L1520-L1537` receives `result.usage?.model` but uses it only for child rate-window identity; lead output is separately finalized. This is the candidate response-model seam for both own and child output, not proof that every hook version supplies a concrete ID. `dashboard.ts#L1496-L1505` records request effort only for children. Preserve that recorded-request provenance; absent response model stays unknown. |
| Hosted Claude, own and child response | `agents-plugin-tool/internal/wsagent/claude.go#L698-L707,L901-L930` decodes assistant model and separates child lines by `parent_tool_use_id`. `claude_alerts.go#L49-L66` joins only known lead-spawned child tool-use IDs to task IDs and forwards the answering model. Unmatched/deeper child lines are not proof of the lead's or another child's model. `testdata/claude-lead-settings.jsonl#L8-L14` records CLI 2.1.322, a concrete assistant model, UUID and original timestamp. The typed assistant message has no effort field; settings acknowledgments and `claude_alerts.go#L25-L36` effort-carrier spawn parsing are not response-effort evidence. Omit effort when no appropriately attributed recorded value exists. |
| CLI Pi, own and direct/deep children | `agents-plugin-pi/src/dashboard-feed.ts#L787-L807` receives own `message_end`; `src/spawner.ts#L3355-L3380` receives each direct RPC child's message events. `src/output-rate.ts#L53-L69` already reads the completed assistant's provider/model, independently of RPC `get_state`. `src/spawner.ts#L849-L882` instead sets `observedModel`/`observedEffort` from RPC state: those names do not make them response proof. `src/dashboard-feed.ts#L365-L395` relays deeper descendants' owner-supplied display telemetry with parent IDs; that path needs additive provenance, not inheritance from a parent. |
| Hosted Pi response and recorded effort | `agents-plugin-tool/internal/wsagent/pi.go#L750-L760,L869-L875` receives raw assistant `message_end` but its typed `piMessage` currently drops provider/model/thinkingLevel. `testdata/pi-tool-turn.jsonl#L6-L12` supplies concrete provider/model, timestamp and recorded `thinkingLevel` on assistant messages: extend this parser rather than read launch settings. Native child rows travel through `pi.go#L1024-L1049`, which takes only agent/permission events naming a child; daemon-spawned children have their own runner conversations. Preserve each producer's identity/provenance. |
| Codex own and child evidence | `agents-plugin/.codex-plugin/hooks.json` wires only the mailbox Stop hook. `agents-plugin-tool/cmd/ws-mcp/mailbox.go#L487-L496` decodes only stop-hook-active/cwd; its comment about additional model/transcript fields is not a response fixture. Bounded `git grep` for the alternatives `turn_context`, `collab_agent_spawn` and `rollout-` at the pinned revision under `agents-plugin-tool/internal`, `agents-plugin/hooks` and `agents-plugin/bin` returned no matches. No own/child response-evidence parser or fixture was established by this search. Use approved unknown independently for each missing path; do not build a hosted Codex backend here. The separate `261115-feat-ws-hosted-codex-adapter-contract` remains idea, not this ticket's prerequisite. |
| Existing rate limitation | Hosted Pi's `pi.go#L1103-L1108,L1148-L1155` computes output over first-update-to-message-end elapsed time, not the established block-span/reasoning-aware window. Do not silently present it as equivalent to `agents-plugin-pi/src/output-rate.ts#L53-L71`. Hosted Claude's `claude_alerts.go#L44-L48` explicitly has no honest output rate without partial messages. Missing eligible rate stays absent; this ticket does not authorize a new streaming/holder architecture. |
| Durable history and older writers | Both snapshot writers reconstruct typed state: `agents-plugin-tool/internal/mcp/live_snapshot.go#L303-L310,L580-L603` and `agents-plugin-tool/internal/wsagent/record.go#L350-L365,L451-L458`. Neither is an unknown-field-preserving history store. In contrast, hosted `Record` preserves unknown top-level JSON through `record.go#L150-L209`, covered by `record_test.go#L11-L99`; this does not cover older pre-preservation writers or CLI sessions with no runner record. Storage selection must protect history from all applicable snapshot rewrites, not just tolerate unknown reads. |
| Replay and deduplication | `agents-plugin-tool/internal/wsagent/runner.go#L2520-L2535,L2566-L2596` tracks holder sequence and replay; `record.go#L378-L383` applies replay quietly without rewriting ring entries. Claude `claude.go#L546-L552,L901-L915` recognizes bounded UUID replay around compaction. These are reuse points, not a model-history dedup implementation. Preserve original evidence time and stable producer/message identity across repeated intake, resume and holder replay; a receipt/heartbeat timestamp alone cannot identify a new model change. |

**Remaining implementation proof:** select and test the additive durable history
path for CLI and hosted producers, including older writers, replay/order and
repeated evidence. No existing all-harness full model-history store or complete
Codex evidence mapping is claimed. The audit store ticket
`261114-feat-ws-conversation-audit-event-store` is todo with unimplemented phases;
its approved audit envelope/store is separate work, not a landed dependency to
assume here. These are technical implementation obligations under the existing
T0/unknown rules, not newly selected policy or architecture. Return any inability
to satisfy those rules to MAIN before implementation expands scope.

Existing test extension points (located, not run):
`agents-plugin-tool/internal/wslive/state_test.go` (`TestReaderIgnoresUnknownFields`,
`TestSubagentTelemetryRoundTripsAndKeepsAbsentFields`,
`TestBatchViewJSONIsUnchangedByTelemetry`),
`agents-plugin-tool/internal/wsagent/record_test.go` (`TestRecordUnknownMembersRoundTrip`),
`agents-plugin-tool/internal/wsagent/pi_test.go`,
`agents-plugin-tool/internal/wsdashboard/rail_test.go`,
`agents-plugin-tool/internal/wsdashboard/pageview_test.go`,
`agents-plugin-tool/internal/wsi18n/keys_test.go`,
`agents-plugin/hooks/claude-mod/dashboard.test.ts`, and
`agents-plugin-pi/test/dashboard-feed.test.ts`. Extend fixtures for identity and
history; their existence is not passing coverage of this new contract.

## Prior Decisions

- 261110-feat-ws-dashboard-agent-panel-telemetry / c2fd2c99 (2026-11-10, commit): "Rejected: deriving the model from usage.model for display (the step input is what the request named, as asked)" — bearing: constrains
- 261110-feat-ws-dashboard-headless-lead-sessions / a27d1bf6 (2026-11-11, commit): "Output rate is not mapped: without --include-partial-messages each content block arrives whole" — bearing: constrains
- bc379e63 (2026-11-13, commit): "Record decoding is lenient at agents-plugin-tool/internal/wsagent/record.go:149 and preserves unknown members through re-encoding, so older writers retain the field." — bearing: supports
- 261113-feat-ws-dashboard-session-rail-tree / 51429395 (2026-11-14, commit): "One 'order' list per workspace holds every group's ids. Only the relative order within a group matters" — bearing: constrains
- 261112-bug-ws-hosted-finished-subagents-lost-on-idle-resume (2026-11-12, Resolution): "a new live writer seeds the previous snapshot's done and failed agents (wslive.State.SeedFinished)" — bearing: constrains
- 261112-bug-ws-hosted-pi-subagents-missing-from-dashboard (2026-11-12, Resolution): "rows revived from the sidecar at restart carry no model/effort" — bearing: constrains
- 261115-feat-ws-hosted-codex-adapter-contract (2026-11-15, Decisions): "A future adapter attaches at Backend.Start / Process / normalized events beneath the existing Runner and holder." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wslive/{snapshot,state}.go, internal/wsagent/{claude,pi,record,runner}.go, internal/mcp/live_snapshot.go, internal/wsdashboard/pageview_rail.go and assets, agents-plugin/hooks/claude-mod/dashboard.ts, agents-plugin-pi/src/{spawner,dashboard-feed}.ts |
| scope.surface | cross-module | adapter intake, exported wslive.Event/Lead/Agent and snapshot JSON, runner response parsing, dashboard RailRow and agent presentation |
| scope.new_public_symbol | yes | Phase 1 requires optional exported evidence/history fields on shared Go event/snapshot types; exact names are implementation detail |
| scope.new_type_contract | yes | optional per-session/per-agent observed model, source/time, effort and history through intake/snapshot; existing telemetry meanings remain unchanged |
| scope.test_surface | existing | wslive/state_test.go, wsagent/record_test.go and pi_test.go, wsdashboard/rail_test.go and pageview_test.go, wsi18n/keys_test.go, Claude dashboard.test.ts and Pi dashboard-feed.test.ts |
| complexity.reuse_points | confirmed | RailRow snapshot association, contextBar/costText, Claude step-end usage.model and hosted child reply mapping, Pi message_end and output-rate tracker, Record unknown-member preservation, holder replay sequence |
| complexity.side_effect_risk | moderate | new persisted evidence crosses two snapshot writers and replay paths; observing must not invoke existing model controls |
| risk.correctness | high | request versus response provenance, independent children, original timestamps and replay deduplication can otherwise report a false model/change |
| risk.fit | moderate | Go daemon plus CLI/hosted adapters and shared rail SSE must retain compact A geometry and the landed separate agent effort stat |
| risk.test | high | no complete Codex own/child response fixture or all-writer durable-history proof exists; fixture extension and mixed-version replay checks remain necessary |
| risk.security_or_contract | high | T0 requires retained history survive older writers while preserving lead-agent telemetry rejection, redaction and BatchView JSON exclusions |

## Phases

### Phase 1: Additive observed identity, recorded effort, rate and history telemetry

After lead-owned fact population and readiness gates, extend the existing data
path with optional per-session/per-agent observed concrete model, evidence
source/time, recorded effort where available, t/s and model-change history.
Keep configured/requested identity separate. Reuse existing adapter/parser/rate
seams, with separate mappings for CLI and hosted sessions; a missing supported-
harness path returns explicit unknown rather than guessed parity. Preserve
existing agent telemetry meanings and keep per-agent provenance independent.
This phase supplies data for Phase 2, not a new model selector or UI proposal.

Record previous-to-current changes with evidence timestamps for the compact
marker and full history in session detail. Preserve last-observed identity/time
when evidence stops; retain last-turn t/s for idle sessions with that provenance,
without fabricating a rate for unknown or stopped evidence. Reuse the established
output-rate definition and document harness limitations. Reuse context/cost/turn
figures; no new counters or per-batch snapshot usage are needed.

Verification (implementation, not performed by this preparation):
- Fixture coverage for Claude, Pi and Codex, CLI/hosted where supported: alias
  versus response disagreement, request/state versus output evidence, resume,
  parent/child/deep-child independence, mid-session change, missing effort,
  unknown and stale/last-observed evidence with the original observation time.
- History order, repeated evidence deduplication, resume/replay and the selected
  durable storage path; idle last-turn t/s versus working/unknown/stopped states.
  Preserve existing ended-subagent rate suppression unless an additive
  last-turn field explicitly supplies separate provenance.
- New/old adapter and daemon combinations, absent/unknown optional fields,
  existing lead-agent rejection, and old-writer history preservation. Confirm
  T0 without changing MCP arguments, tracker kinds, config/rsrc contracts,
  snapshot field meanings or `BatchView` JSON. Any incompatible requirement
  returns to the lead; no T2 approval is implied.

### Phase 2: Approved compact session hover and current-agent model provenance

MAIN's explicit 2026-11-15 03:33:37Z execution grant supersedes the original
Phase 1 dependency: implement Phase 2 first as a UI-only head; Phase 1 follows
separately and remains unfinished. Render approved A using existing RailRow,
context/cost/turn-time and per-agent telemetry only. Missing output identity,
recorded effort, eligible last-turn rate and history stay unknown or omitted;
request/state model values are labelled separately, never promoted to observed
output. No new intake, persistence, protocol, history or adapter collection is
a prerequisite for this head. Render approved A from the tracked references, using existing
RailRow association and snapshot context/cost/turn-time figures. Keep rail rows,
last-message text and header height unchanged. Retain the current agent panel's
landed layout/separate effort stat while exposing the correct per-agent model
and its provenance in the existing display/model hover. Show the timestamped
previous-to-current indicator compactly; full history belongs in session detail.
Apply (836): pointer-transparent/read-only popup, idle last-turn t/s, bottom-meter-
only five-hour/weekly limits, and removal of only the row reply-ID tooltip.

Verification (implementation, not performed by this preparation):
- Implement, run relevant rendering/state/escaping checks and affected wsi18n
  checks, then return the head. Use fixtures for hover lifecycle across SSE,
  idle last-turn labels, absent effort, unknown/stale provenance, independent
  agents, tooltip scope, pointer transparency and unchanged model controls.
  Lite review targets logic/state risk, not pure CSS/layout/order.
- At most one quick sanity screenshot; no screenshot-proof rounds or
  multi-viewport/browser matrix. After merge and an authorized swap, show
  implementation results beside the tracked approved A reference for lightweight
  owner-visible acceptance, including compact geometry and light/dark intent.
  Preserve row/header heights, model wrapping, separate effort, context/three
  stats and the supplementary A states; fix owner on-screen feedback promptly.
- Only the reply-ID tooltip disappears; other tooltips and rail navigation
  remain. Plan limits stay in the bottom meter. Visual acceptance is the
  post-merge owner feedback step, not a pre-head proof gate.

This fact-population pass used only source inspection, contract trace and diff
checks; approved A references and requested source-fragment provenance remain
preserved, not a raw hash-catalog readiness gate. Builds, suites, browser captures,
providers, live probes and daemon/session changes are deferred to separately
authorized implementation verification; none is evidence supplied by this revision.
