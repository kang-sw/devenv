---
title: Codex hosted adapter range-versioned catalog contract and bounded lead plan
parent: 261114-epic-ws-common-session-foundation
related:
  261110-epic-ws-dashboard-agent-host: "preserve hosted lead ownership, trusted-origin approval and holder survival"
  261114-refactor-ws-common-session-interface: "done; its Phase 2 prerequisite is satisfied (the earlier blocked-by is removed)"
  261117-feat-ws-harness-capability-catalog: prerequisite
  261117-feat-ws-hosted-antigravity-adapter: "next stage-3 harness, ordered after Codex"
  261116-research-ws-hosted-session-first-tier: "evidence and Confirmed Decisions"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 1f03b99ebfd35647
sage-review-completeness-reviewed: 1f03b99ebfd35647
---

# Codex hosted adapter contract and first lead slice

> Revised 2026-11-17 under the hosted-first plan (epic `261114-epic-ws-common-session-foundation`, stage 3): the exact-version contract below is replaced by the range-based catalog version policy; Codex is the first stage-3 harness, and the Pi hosted backend will be ported from this adapter (25).

## Background

Codex already has ws plugin/MCP, configuration and mailbox-hook support, but that is not a daemon-hosted conversation backend. The former short app-server account/rate-limit usage probe is no longer in the tree: Codex usage acquisition moved to file-selected OAuth REST at `4c50ad056` (2026-11-16, an ancestor of this branch), and no non-test Go file under `agents-plugin-tool/internal` mentions `app-server` (search returned nothing); it was never a sustained conversation owner. There is no Codex `Backend` / `Process` under `agents-plugin-tool/internal/wsagent` (no codex file there; checked 2026-11-17 on `track/agent-v2`).

The ticket was first captured on 2026-11-14 as a docs-only plan while the owner was asleep, with (799)=a assumed rather than confirmed. The foundation prerequisite (the common interface's Phase 2) has since landed. Under the hosted-first plan the capability/protocol/fixture contract is expressed as capability-catalog values per `harness@version` range; the first implementation is one hosted lead conversation, after the catalog ticket; execution is approved (Decision 4).

(799)=a was "assumed while the owner was asleep" and was not confirmed (03). Owner 2026-11-17 22:10 answered (03): the protocol is the Codex **app-server** (sustained JSON-RPC control), and (70)=b moved Codex and Antigravity ahead of the single session holder, approving execution of this ticket now (with (66)=a, (67)=a). The installed CLI on the home machine is `codex-cli 0.160.0`.

## Decisions

1. **Reuse the current execution boundary.** A future adapter attaches at `Backend.Start` / `Process` / normalized events beneath the existing Runner and holder. Do not create a second daemon/runtime, use `execjob` as a conversation owner, or duplicate mailbox/input delivery.
2. **Protocol: app-server (owner (03), 2026-11-17 22:10).** The adapter drives a sustained `codex app-server` process over its JSON-RPC protocol. Rejected: per-invocation `exec --json` / `exec resume` (no sustained bidirectional control for interrupt, approvals or mid-turn delivery). Method signatures and the instruction-injection mechanism are pinned in Phase 1 from the installed version and its official documentation; for every claimed capability record the version range it holds for, the build/platform evidence and the matching documentation.
3. **Bound the first implementation to one hosted lead conversation.** The intended first slice is create/resume/send/interrupt, ordered normalized output and holder reattach. Approval handling must preserve supported decisions and stable correlation rather than silently approve or reinterpret a richer choice. Unsupported handoff, permission modes, steer and native-subagent telemetry must remain explicit until their exact support is settled.
4. **Gates.** `261114-refactor-ws-common-session-interface#phase2` has landed. Execution approval was granted by the owner on 2026-11-17 ((70)=b, (66)=a). Phase 1 maps Codex values onto the catalog item list already settled in the catalog ticket's Decision 3 and may run beside the catalog's Phase 1; Phase 2 depends on the catalog's Phases 1-3 (schema, HTTP MCP admission, startup check).
5. **Range-based version policy replaces the exact-version contract (20)=a, (21)=a, (22).** Codex values live in the catalog per `harness@version` with `since` / `until` ranges; version branches in adapter code only where data cannot express the difference (protocol translation). At session start read the version and lightly check the init shape, demoting mismatched capabilities to `unsupported`. Verification is per-version recorded stream fixtures; the guaranteed upper bound is the newest version with a fixture. Out of range, above or below the lower bound, allow loudly with the nearest-version profile, an "unverified version" dashboard warning and an error only on actual failure.
6. **Stage ordering (25).** Claude is the reference, then Codex (this ticket), then Antigravity, then Pi. The Pi hosted backend is a port of this Codex adapter and catalog profile (Pi is driven only with GPT models), not a separate Pi-first design.
7. **Delegation is `ws/agent.*` (06).** Hosted sessions delegate through `ws/agent.*`, not native subagents. Owner (08)=c: whether a harness's native subagents can be removed is decided per harness at stage 3, so Phase 1 probes and records it for Codex.
8. **Compatibility target: T0.** Preserve current Pi/Claude controls, native integrations, identities, readable state and existing API/holder/event semantics. Phase 1 records the concrete tier verdict for every new descriptor/config/serialization surface in its `### Result`, with source lines, including an old daemon reopening a hosted Record whose new Codex `Backend` value it does not know. T2, root-key policy changes and a store/protocol rewrite are not authorized.

## Capability Matrix (range-based)

Current target protocol: **Codex app-server** (Decision 2); installed version `codex-cli 0.160.0` (`codex --version`). The supported version range and per-row evidence are **not yet verified** (Phase 1). This is the finite contract matrix to complete, not a declaration of API support. Each completed row maps to a catalog item and carries the version range (`since` / `until`) it holds for, official contract citation, fixture identity and `native` / `ws-substitute` / `unsupported` classification. Historical versions below are evidence labels, not the chosen hosted target.

| Capability | Recorded evidence and version boundary | Required contract / fixture before readiness |
|---|---|---|
| Process protocol | `ai-docs/manuals/codex-integration.md` records `exec --json` / `exec resume`; their original invocation notes are not tied to a new hosted target. No Go app-server client remains in the tree (usage moved to OAuth REST, `4c50ad056`); the installed `codex app-server` exists (`codex app-server --help`: default `--listen stdio://`, `generate-json-schema` / `generate-ts` subcommands), so nothing about its sustained protocol is proven by existing ws code. | Pin app-server framing, initialization handshake and sustained bidirectional control at the installed version (protocol already settled, Decision 2). |
| Create / resume / attach | Recorded exec notes describe CLI-assigned thread IDs and nonexistent-thread failure. No hosted holder/session adoption is proven. | New/resumed thread identity, existing holder reattach without duplicate launch, missing/invalid session and changed-version resume fixtures. |
| Send / interrupt / steer | The 2026-09-13 Codex **0.154.0** macOS re-probe found `PostToolUse` side-effect-only; Stop-hook reentry is a turn-boundary path, not active-turn steer. | Pin real send/cancel controls; distinguish next-turn queueing, active-turn steer, cancellation, turn end and process exit. Unsupported steer is not fabricated support. |
| Approvals / pause | Existing manual bypass flags are probe history, not production policy. No current hosted approval contract is established. | Request/answer/reject/pause IDs, richer choice mapping, duplicate/stale answers and trusted-origin checks; settle any loss of expressiveness. |
| Ordered events / recovery | Recorded `--json` stdout differs from rollout JSONL. Native transcript parsing is not a live event mapper. | Content/tool/usage/error lifecycle, ordering, unknown messages, EOF, replay/gaps, reconnect and queued-input release fixtures. |
| Config home / authentication | The **0.154.0** re-probe says `--ignore-user-config` still uses `CODEX_HOME` for authentication. Account-slot support is not implemented by this plan. | Correct harness-owned config/account home, scrubbed inherited ws identity, MCP/key/physical-PID adoption and isolation; no login or credential mutation now. |
| Instructions / workflow access | The local manual records `model_instructions_file` replacement semantics, not the other adapters' append-system-prompt contract. | Pin how approved workflow/context reaches the session without silently replacing native defaults; preserve the adopted common access boundary. |
| Subagents / idle safety | The **0.154.0** ephemeral re-probe did not establish daemon-backed subagent hooks. Missing activity is not known zero. | Supported producer/parent/status evidence or explicit unknown/unsupported capability; conservative idle behavior until actual running-state evidence exists. |
| Model / effort / UI admission | Common-session Phase 1 is being refined for registration/admission/profile/choice metadata. Codex model names are not the harness shorthand. | Capability-consistent defaults, choices and unsupported-operation behavior through the settled descriptor; no hard-coded claim of full Pi/Claude parity. |

## Constraints

- Probes and provider calls against the installed `codex` CLI and the owner's existing login are authorized (owner 2026-11-17 22:10). Do not log in, log out, switch accounts or mutate credentials or the user's Codex config home; run probes with an isolated or read-only configuration where the protocol allows.
- Consult exact-version official documentation for the installed version. Existing reports/manuals are supporting prior evidence; do not present them as current installed-binary or account-backed acceptance.
- Do not use `PostToolUse` to implement mid-turn delivery on the contradicted 0.154.0 assumption, reuse dangerous bypass flags as approval policy, or treat rollout-file events as stdout protocol events.
- Keep ws identity, parent lineage, single input/mail ownership, approval correlation and holder replay/recovery in their current owners. One lead slice does not authorize daemon-spawned native children or account switching.
- Implementation manuals later include `ai-docs/manuals/shipped-surface-boundary.md`, `ai-docs/manuals/release-compatibility.md` and `ai-docs/manuals/ws-mcp.md` where applicable; authored/mirrored surfaces retain their matching manuals.
- Go test location (owner (68), 2026-11-17 22:10): run Go tests locally on the macOS home machine. The earlier "remote w5090 only" rule belonged to the owner's company Windows setup and does not apply here.
- Release-gate file-touch rule (owner, 2026-11-17): minimize daemon file touches at the root; files for backup/restore only; deltas (append) not rewrites; no heartbeat rewrites; interactive paths never wait on file I/O.
- No main push, version bump or release. Work lands on `track/agent-v2` through the lead. Replacing the shared daemon on this machine is the lead's call, not a worker's.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin-tool/)
- Convention: ai-docs/manuals/release-compatibility.md (declared for agents-plugin-tool/internal/)

## Prior Art

- `ai-docs/manuals/codex-integration.md`: existing plugin/MCP operations, distinct stdout/rollout formats, instruction replacement, and explicitly versioned 0.154.0 hook re-probe superseding 0.128.0 steering claims.
- `agents-plugin-tool/internal/wsdashboard/plan_usage_probe.go`: Codex usage probe; it now reads usage over file-selected OAuth REST (`plan_usage_probe.go#L215`, commit `4c50ad056`), so it is no longer app-server prior art. `plan_usage_cmd_unix.go` does not exist in the tree (`git ls-files | grep plan_usage_cmd` returned nothing; it was removed with the app-server launcher).
- A 2026-11-14 harness-readiness inventory (`shared/docs/harness-interface-readiness.md` at `951af006a`) is not in this repository; do not depend on it. Re-ground boundaries from current source.

## Prior Decisions

- 261117-feat-ws-harness-capability-catalog (2026-11-17, Decisions): "Admission conditions. A harness is unusable without them: (0) headless JSON event streaming (the adapter converts the format); (3) HTTP MCP server injection (24)." — bearing: supports
- 261116-research-ws-hosted-session-first-tier (2026-11-16, Confirmed Decisions): "(25) Implementation and verification order is Claude, Codex, Antigravity, then Pi; Pi is driven only with GPT models, so the Pi hosted backend is a port of the Codex adapter" — bearing: supports
- 261114-epic-ws-common-session-foundation (2026-11-14, Cross-Child Decisions): "Codex and Antigravity are both tier-1 hosted harnesses (02). (799)=a for Codex was assumed while the owner was asleep and is not confirmed (03)" — bearing: supports
- 261117-feat-ws-hosted-antigravity-adapter (2026-11-17, Decisions): "Stage-3 child, ordered after Codex (25), ahead of the holder ((70)=b). Implementation and verification order is Claude, Codex, Antigravity, then Pi." — bearing: constrains
- 261116-bug-ws-exe-launch-elimination 74924b4c (2026-11-16, commit): "item 4 (codex app-server -> REST, (1207)=a) stays parked at parked/exe-phase1-item4-codex d9fd94543 and follows after the swap acceptance." — bearing: constrains
- 0086eb14 (2026-09-13, commit): "Left open: whether Codex subagent turns fire hooks at all — the isolated ephemeral exec could not spawn a subagent (needs the shared app-server daemon); flagged for a daemon-backed probe." — bearing: supports
- c6267c3f (2026-11-15, commit): "Explicitly retain an unexpected side effect: cleanup tracked a codex app-server process parented by the baseline fixture despite intended provider isolation." — bearing: constrains
- 261113-feat-ws-dashboard-rail-usage-gutter (2026-11-13, Decisions): "primary source is a short-lived `codex app-server` process answering `account/rateLimits/read` (the Codex CLI manages its own token)" — bearing: context only; superseded by OAuth REST at `4c50ad056`, so it is not working app-server prior art

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | Phase 2 touches agents-plugin-tool/internal/wsagent (new Codex backend, fixtures), the catalog profile and the startup check; Phase 1 edits ai-docs manual/ticket evidence |
| scope.surface | cross-module | Backend/Process boundary at agents-plugin-tool/internal/wsagent/backend.go#L21-L28 and #L99-L116, plus Runner, holder and the capability catalog |
| scope.new_public_symbol | yes | a Codex Backend registered beside the existing backends; the exported name is not named in the ticket |
| scope.new_type_contract | yes | Codex values in the harness capability catalog per harness@version range and a Process implementation of the existing interface; no exact signature named yet |
| scope.test_surface | new-files | recorded app-server stream fixtures and adapter tests; precedent agents-plugin-tool/internal/wsagent/testdata/pi-tool-turn.jsonl; no Codex test or fixture exists (search of wsagent for codex files returned only test mentions) |
| complexity.reuse_points | confirmed | existing Backend/Process interface in agents-plugin-tool/internal/wsagent/backend.go read at #L21-L28 and #L99-L116 |
| complexity.side_effect_risk | high | spawns a sustained external process that owns approvals, input delivery and holder replay beneath the live Runner |
| risk.correctness | high | ordered event mapping, interrupt versus turn end, reattach without duplicate input, and unverified 0.160.0 protocol details |
| risk.fit | moderate | the Backend/Process boundary exists, but app-server JSON-RPC is bidirectional and may not fit the one-way Events/Send shape without extension |
| risk.test | high | acceptance needs a real hosted Codex conversation with the owner's login; fixtures must be recorded from the installed CLI |
| risk.security_or_contract | high | approval correlation and trusted-origin checks, credential and CODEX_HOME handling, and catalog compatibility tier (T0 target) |

## Phases

### Phase 1: Settle the range-based capability catalog values and fixture contract

With the app-server protocol settled (Decision 2) and the installed `codex-cli 0.160.0`: probe the app-server and complete the finite matrix with the selected version range, official contract evidence and recorded stream fixtures. Map each row to a catalog item (catalog Decision 3) with its grade and `since` / `until`. Cover thread create/resume/send/interrupt/optional steer, permissions, normalized event order, holder recovery, config/account home and workflow access. Check both admission conditions (headless JSON event streaming; HTTP MCP server injection) and decide native subagent removability (08)=c. Keep unsupported operations and unresolved telemetry visible. Return choices that change product/permission/compatibility contracts to the lead rather than selecting an API from a different version or assuming parity. If app-server's bidirectional JSON-RPC (server-initiated approval requests, for example) does not fit the existing `Backend` / `Process` shape, describe the needed extension and return it to the lead; do not extend the common interface in this phase.

Deliverables: the completed matrix, the admission evidence, the removability verdict and the compatibility verdict (Decision 8) go in this phase's `### Result`; recorded fixtures go under `agents-plugin-tool/internal/wsagent/testdata/` beside the existing Pi fixture.

Verification: every claimed supported capability has a matching version-ranged source and a recorded fixture or a finite planned-fixture expectation, with explicit unknowns; planned fixtures are distinguished from recorded ones; both admission conditions carry probe evidence.

### Phase 2: Implement one hosted lead conversation after the foundation gate

Depends on this ticket's settled Phase 1 and the capability catalog ticket's Phases 1-3 (`261117-feat-ws-harness-capability-catalog`); execution approval is granted (Decision 4). Implement only the agreed hosted-lead slice through the existing Runner/backend/holder boundary, the catalog profile and the startup version/init-shape check. Preserve truthful unsupported controls, one input/mail owner, approvals, identity and ordered recovery. No implicit native-child, multi-account or terminal-handoff parity expansion. The backend serves the lead role only: it must not make `codex.<tier>` spawnable as an `agent.spawn` worker (batch-review coherence C2; the tier listing rule lives in `261117-feat-ws-agent-tools-mailbox-routing`).

Verification: isolated protocol and real-adapter/fake-holder fixtures for create/resume/send/interrupt, ordered content/tools/usage/errors, approvals, unknown events, holder reattach/replay gaps and no duplicate input. Run Pi/Claude regressions against the exact candidate. A real hosted Codex lead conversation on the installed CLI with the owner's login is the acceptance check; no fixture result is invented.
