---
title: Codex hosted adapter exact-version contract and bounded lead plan
parent: 261114-epic-ws-common-session-foundation
blocked-by: 261114-refactor-ws-common-session-interface#phase2
related:
  261110-epic-ws-dashboard-agent-host: "preserve hosted lead ownership, trusted-origin approval and holder survival"
  261114-refactor-ws-common-session-interface: "implementation prerequisite: landed Phase 2; static contract preparation can proceed now"
---

# Codex hosted adapter contract and first lead slice

## Background

Codex already has ws plugin/MCP, configuration and mailbox-hook support, but that is not a daemon-hosted conversation backend. The existing app-server usage probe is a short account/rate-limit query, not a sustained conversation owner. MAIN's harness-readiness investigation found no current Codex `Backend` / `Process` under `agents-plugin-tool/internal/wsagent` at its inspected revision `951af006a`.

MAIN assigned a dedicated idea/todo plan capture on 2026-11-14 under docs/tickets-only direction, describing (799)=a as assumed while the owner was asleep. This records that dispatch provenance, not an independently confirmed owner runtime answer. Prepare an exact-version capability/protocol/fixture contract now; the first later implementation is one hosted lead conversation, after the common interface's landed Phase 2 and separate execution approval.

## Decisions

1. **Reuse the current execution boundary.** A future adapter attaches at `Backend.Start` / `Process` / normalized events beneath the existing Runner and holder. Do not create a second daemon/runtime, use `execjob` as a conversation owner, or duplicate mailbox/input delivery.
2. **Pin evidence before choosing the protocol.** Compare sustained app-server control with per-invocation `exec` / `exec resume`. No protocol, library version, method signature or instruction-injection mechanism is selected by this idea. The target binary/library version remains unselected; record exact version/build/platform and matching official documentation for every claimed capability before readiness.
3. **Bound the first implementation to one hosted lead conversation.** The intended first slice is create/resume/send/interrupt, ordered normalized output and holder reattach. Approval handling must preserve supported decisions and stable correlation rather than silently approve or reinterpret a richer choice. Unsupported handoff, permission modes, steer and native-subagent telemetry must remain explicit until their exact support is settled.
4. **Keep the foundation gate.** `261114-refactor-ws-common-session-interface#phase2` is the implementation prerequisite, not whole-epic completion. Static documentation preparation now does not bypass that gate or authorize ready promotion/adapter execution.
5. **Compatibility target: T0.** Preserve current Pi/Claude controls, native integrations, identities, readable state and existing API/holder/event semantics. Classify actual new descriptor/config/serialization surfaces before readiness; T2, root-key policy changes and a store/protocol rewrite are not authorized.

## Exact-Version Capability Matrix

Current target version/build/platform: **not selected or verified**. This is the finite contract matrix to complete, not a declaration of API support. Each completed row must carry the exact target version, official contract citation, fixture identity and supported/unsupported/unknown classification. Historical versions below are evidence labels, not the chosen hosted target.

| Capability | Recorded evidence and version boundary | Required contract / fixture before readiness |
|---|---|---|
| Process protocol | `ai-docs/manuals/codex-integration.md` records `exec --json` / `exec resume`; their original invocation notes are not tied to a new hosted target. The Go app-server usage client only proves its narrow request path. | Compare app-server versus exec process ownership, framing, initialization and sustained bidirectional control at one pinned version; choose through explicit settlement. |
| Create / resume / attach | Recorded exec notes describe CLI-assigned thread IDs and nonexistent-thread failure. No hosted holder/session adoption is proven. | New/resumed thread identity, existing holder reattach without duplicate launch, missing/invalid session and changed-version resume fixtures. |
| Send / interrupt / steer | The 2026-09-13 Codex **0.154.0** macOS re-probe found `PostToolUse` side-effect-only; Stop-hook reentry is a turn-boundary path, not active-turn steer. | Pin real send/cancel controls; distinguish next-turn queueing, active-turn steer, cancellation, turn end and process exit. Unsupported steer is not fabricated support. |
| Approvals / pause | Existing manual bypass flags are probe history, not production policy. No current hosted approval contract is established. | Request/answer/reject/pause IDs, richer choice mapping, duplicate/stale answers and trusted-origin checks; settle any loss of expressiveness. |
| Ordered events / recovery | Recorded `--json` stdout differs from rollout JSONL. Native transcript parsing is not a live event mapper. | Content/tool/usage/error lifecycle, ordering, unknown messages, EOF, replay/gaps, reconnect and queued-input release fixtures. |
| Config home / authentication | The **0.154.0** re-probe says `--ignore-user-config` still uses `CODEX_HOME` for authentication. Account-slot support is not implemented by this plan. | Correct harness-owned config/account home, scrubbed inherited ws identity, MCP/key/physical-PID adoption and isolation; no login or credential mutation now. |
| Instructions / workflow access | The local manual records `model_instructions_file` replacement semantics, not the other adapters' append-system-prompt contract. | Pin how approved workflow/context reaches the session without silently replacing native defaults; preserve the adopted common access boundary. |
| Subagents / idle safety | The **0.154.0** ephemeral re-probe did not establish daemon-backed subagent hooks. Missing activity is not known zero. | Supported producer/parent/status evidence or explicit unknown/unsupported capability; conservative idle behavior until actual running-state evidence exists. |
| Model / effort / UI admission | Common-session Phase 1 is being refined for registration/admission/profile/choice metadata. Codex model names are not the harness shorthand. | Capability-consistent defaults, choices and unsupported-operation behavior through the settled descriptor; no hard-coded claim of full Pi/Claude parity. |

## Constraints

- Current work is documentation only: no install, login, auth/config-home operation, executable/provider/daemon probe, source patch, build or test. The matrix identifies future checks, not commands authorized now.
- Consult exact-version official documentation when Phase 1 assessment is separately authorized. Existing reports/manuals are supporting prior evidence; do not present them as current installed-binary or account-backed acceptance.
- Do not use `PostToolUse` to implement mid-turn delivery on the contradicted 0.154.0 assumption, reuse dangerous bypass flags as approval policy, or treat rollout-file events as stdout protocol events.
- Keep ws identity, parent lineage, single input/mail ownership, approval correlation and holder replay/recovery in their current owners. One lead slice does not authorize daemon-spawned native children or account switching.
- Implementation manuals later include `ai-docs/manuals/shipped-surface-boundary.md`, `ai-docs/manuals/release-compatibility.md` and `ai-docs/manuals/ws-mcp.md` where applicable; authored/mirrored surfaces retain their matching manuals.
- Corrected (778) reserves runtime replacement, swap smoke and profiling to MAIN. Mail eventual develop merge hashes immediately; this capture authorizes no main push/version bump or live delivery.

## Prior Art

- `ai-docs/manuals/codex-integration.md`: existing plugin/MCP operations, distinct stdout/rollout formats, instruction replacement, and explicitly versioned 0.154.0 hook re-probe superseding 0.128.0 steering claims.
- `agents-plugin-tool/internal/wsdashboard/plan_usage_probe.go` and `plan_usage_cmd_unix.go`: narrow app-server prior art, not a conversation backend or authorization to run the probe.
- MAIN's `shared/docs/harness-interface-readiness.md`, especially sections 3–6: static boundary/gap inventory at `951af006a`. Re-ground against the chosen implementation revision; the shared artifact is not a downstream dependency or approval of all proposals.

## Phases

### Phase 1: Settle the exact-version capability and fixture contract

Complete the finite matrix with selected version/build/platform, official contract evidence and a protocol/fixture plan. Reconcile app-server versus exec, thread create/resume/send/interrupt/optional steer, permissions, normalized event order, holder recovery, config/account home and workflow access. Keep unsupported operations and unresolved telemetry visible. Return choices that change product/permission/compatibility contracts to MAIN rather than selecting an API from a different version or assuming parity.

Verification: documentation review that every claimed supported capability has a matching exact-version source and finite fixture expectation, with explicit unknowns and phase boundaries. Distinguish a planned fixture from one actually run. No install/probe/account action or new executable fixture collection is part of the current capture.

### Phase 2: Implement one hosted lead conversation after the foundation gate

Depends on this ticket's settled Phase 1 and landed `261114-refactor-ws-common-session-interface#phase2`, plus separate implementation approval. Implement only the agreed hosted-lead slice through the existing Runner/backend/holder boundary and settled admission metadata. Preserve truthful unsupported controls, one input/mail owner, approvals, identity and ordered recovery. No implicit native-child, multi-account or terminal-handoff parity expansion.

Verification: separately authorized isolated protocol and real-adapter/fake-holder fixtures for create/resume/send/interrupt, ordered content/tools/usage/errors, approvals, unknown events, holder reattach/replay gaps and no duplicate input. Run Pi/Claude regressions against the exact candidate when execution is granted. Real CLI/account acceptance and daemon delivery remain separately authorized and reported; no fixture result is invented by this plan.
