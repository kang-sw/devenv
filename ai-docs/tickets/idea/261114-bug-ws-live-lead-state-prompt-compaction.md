---
title: Live lead output disagrees with dashboard idle state and prompt admission
related:
  261114-feat-ws-prompt-refusal-conversation-notice: Deferred conversation visibility and old-dashboard compatibility design
sage-review-design: blocked
sage-review-completeness: blocked
---

# Live lead output disagrees with dashboard idle state and prompt admission

## Background

On 2026-11-14 MAIN's owner reported continued lead output while the dashboard did not show the lead working. A prompt failed with the exact visible error:

```text
prompt refused: Agent is already processing. Specify streamingBehavior ('steer' or 'followUp') to queue the message.
```

The owner reported steering recovered with the context-compacted notice. Permanent inability to prompt is a hypothesis, not an established condition. This is separate from fork-parent identity loss.

## Evidence

- Native MAIN session `rebelrepugnantstonewareemcee/pi/session.jsonl`: line 818 at 05:37:30.719Z preserves the owner report; line 819 at 05:38:01.645Z requests preservation; line 832 at 05:38:51.093Z records compaction completion and revive with key `specks-snowbound-framing`.
- Native ring `20261114T053725.320023000Z-ffbb48f5.json` preserves the original report, `kind:user`, `origin:whiteboard`; SHA-256 `dcfbc5e5d34341e44afe43d5944ba7877e7f1fda6bd271d50e0da33cc80170b7`. Receipt and native-log times differ.
- Fresh read-only health/OS capture confirmed daemon PID 68420, version `0.50.0-dev`, content `d2c343446a56e870bc982d3e55860d6f99dcb0dc54f7a46c70ac2e5c92d52424`, start 01:05:40Z. Current MAIN harness PID 45986 and holder `haa2adea79247` were present; a later snapshot reported `thinking`, not historical incident state.
- Current source `agents-plugin-tool/internal/wsagent/pi.go:476-478` sends a plain prompt without `streamingBehavior`; `:545-546` has a distinct steer command; `:624-636` checks `isStreaming || isCompacting`; `:731-735` maps prompt refusal to turn end. These are tracing boundaries, not an established cause or loaded-source identity.
- Full local capture: `75748f2c-01-live-lead-and-fork-parent-incident-capture.md` in generated review paths, shared with the root lead and MAIN. It preserves registration/feed fields, native chronology, process identity and explicit gaps without tokens or private reasoning.

## Verified defect

VERIFIED (pre-repair fact-check snapshot): The Pi RPC mapper unconditionally ends a runner turn for a handled prompt, a refused prompt, or a lead compact-command response, even when `agent_start` has already established an active native run (`agents-plugin-tool/internal/wsagent/pi.go#L729-L763`, `agents-plugin-tool/internal/wsagent/pi.go#L782-L784`, `agents-plugin-tool/internal/wsagent/pi.go#L976-L983`). A command response does not establish that the independently running agent settled. Installed Pi permits extension commands while streaming and reports handled after their handler returns; the handler has session-control methods (`/opt/homebrew/lib/node_modules/@earendil-works/pi-coding-agent/dist/core/agent-session.js#L1699-L1710`, `#L1782-L1805`; RPC response emission: `/opt/homebrew/lib/node_modules/@earendil-works/pi-coding-agent/dist/modes/rpc/rpc-mode.js#L314-L338`). The fork's working-tree regression `TestPiMapperCommandAnswerDoesNotSettleActiveRun` failed at that snapshot all four command-response interleavings with an unexpected TurnEnd (`agents-plugin-tool/internal/wsagent/pi_test.go#L411-L449`; command: `cd agents-plugin-tool && go test ./internal/wsagent -run '^TestPiMapperCommandAnswerDoesNotSettleActiveRun$' -count=1`).

INFERRED: This can produce runner/dashboard divergence: the runner consumes TurnEnd by ending its turn and releasing held input, while a still-active native run rejects a subsequent plain prompt; that refusal again maps to TurnEnd (`agents-plugin-tool/internal/wsagent/runner.go#L2616-L2626`, `#L2727-L2774`; native busy guard: `/opt/homebrew/lib/node_modules/@earendil-works/pi-coding-agent/dist/core/agent-session.js#L1749-L1761`). This establishes a bounded source defect, not the historical incident's cause or loaded-source identity.

The mapper's `agent_start`, response `prompt`/`compact`, and `agent_settled` cases are the repair boundary. Regression tests must distinguish the reproducible mapper defect from proof of the historical incident's cause.

## Decisions

- The owner assigned EXEC 3 to fix this Important bug on 2026-11-14; MAIN forwarded execution authority. Preserve genuine native-run activity across command answers rather than treating a command's completion as native settlement. Do not relax Pi's prompt-admission guard or silently queue/resubmit a refused owner message.
- Compatibility: T0 transparent internal state repair; no tool, argument, shared-store shape, protocol, tracker event, or configuration surface changes.
- Repair only the verified state-tracking defect. Historical permanent deadlock, exact incident causation, and compaction recovery remain unproven.
- Owner decision (723)=a, relayed by MAIN on 2026-11-14, supersedes (715): ship the bounded busy-state repair with the exact refusal fact in the existing `State.Error` field while preserving `StatusRunning`. It is visible only when the controls menu is opened; conversation-history visibility is not claimed. No field, format, notice kind or shared-ring representation changes are authorized (T0).
- Rejected for this repair: adding a conversation refusal notice now. Capture that separately in `261114-feat-ws-prompt-refusal-conversation-notice`; its design must account for older dashboards that label unknown notice kinds as usage limits.

## Constraints

- No contemporaneous raw refused request, Pi get_state answer, idle UI snapshot or exact feed transition was recovered. The existing daemon log had no matching refusal text. Later health/snapshot evidence cannot reconstruct those states. Loaded bundle/source equivalence and causality of compaction recovery remain unverified. Historical isIdle/isCompacting notes are not proof.
- No shared-daemon replacement, reload, restart, live owner prompt, or destructive cleanup is authorized by this ticket. Source verification uses isolated fixtures; rollout remains lead-owned.
- Tests and review-worker startups remain held until EXEC3 receives the heavy slot after EXEC1. The earlier package/race results predate the private recovery-probe delta; they do not verify that delta. The existing blocked review posture remains until the decision delta is re-reviewed.
- Read `ai-docs/manuals/shipped-surface-boundary.md` before tooling edits and `ai-docs/manuals/release-compatibility.md` before internal tooling edits. Do not couple shipped strings to this repository's ticket or incident identifiers.

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/release-compatibility.md (declared for agents-plugin-tool/internal/)

## Prior Decisions

- e3fe4108 (2026-11-12, commit): "isStreaming is AgentSession._isAgentRunActive, cleared only at agent_settled, so it covers retries, auto-compaction, and queued continuations" — bearing: constrains
- 261112-feat-ws-dashboard-slash-commands (2026-11-12, commit f460afb7): "Pi commands go through a commandRunner interface: /compact maps to RPC compact with all args as instructions" — bearing: constrains
- 261110-feat-ws-dashboard-headless-lead-sessions (2026-11-11, commit 72c9d851): "An idle after a result is that turn's own and is ignored (mapper `open`), so it cannot close the next turn a flushed send already began." — bearing: supports
- dab9f6c3 (2026-11-08, commit): "Interrupt sends abort without waiting, because pi answers abort only after the run settles. The turn ends at agent_settled and no error is reported" — bearing: constrains
- 261106-feat-ws-dashboard-live-snapshot-pi (2026-11-07, Result): "The lead is idle when it is outside its own `agent_start` to `agent_settled` turn and `isOwningAgentIdle()` holds, which includes the compaction hold." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | Mapper: agents-plugin-tool/internal/wsagent/pi.go; private request registration helper: agents-plugin-tool/internal/wsagent/stream.go; existing mapper and runner tests: agents-plugin-tool/internal/wsagent/pi_test.go, agents-plugin-tool/internal/wsagent/lead_command_test.go, agents-plugin-tool/internal/wsagent/lead_runner_test.go. |
| scope.surface | internal | Unexported piMapper state and mapLine behavior; no exported interface changes. |
| scope.new_public_symbol | no | None required by Phase 1. |
| scope.new_type_contract | no | Private activity tracking only; existing Event and Pi RPC shapes remain unchanged. |
| scope.test_surface | existing | agents-plugin-tool/internal/wsagent/pi_test.go:411-449; agents-plugin-tool/internal/wsagent/lead_command_test.go:367-451. |
| complexity.reuse_points | confirmed | Read piMapper.mapLine/endTurn and newLeadHarness with fake holder output in lead_command_test.go:367-451. |
| complexity.side_effect_risk | moderate | TurnEnd releases held owner input and changes live activity through runner.go:2616-2626,2727-2774. |
| risk.correctness | high | Incorrect activity clearing can release prompts into a busy run or retain a settled run; abort and later cycles must remain correct. |
| risk.fit | moderate | Repair is bounded to Pi native lifecycle tracking; preserve command-only completion and existing steer semantics. |
| risk.test | moderate | Four mapper interleavings reproduce failure, but fake-holder tests cannot establish historical/live incident causality. |
| risk.security_or_contract | moderate | Owner-message admission and refusal visibility must remain intact; no silent replay, queuing, or admission-guard relaxation. |

## Phases

### Phase 1: Preserve native activity across Pi command responses

Track native-run activity from `agent_start` through `agent_settled` in the Pi mapper. While that run is active, handled prompt and successful compact responses must not end it; refused prompt/compact responses preserve their refusal text in the existing `State.Error` field while retaining `StatusRunning`, without failing or ending the unrelated active run. This is controls-menu-only visibility, not a conversation notice. When no native run is active, existing command-only completion and refusal behavior remains unchanged. Native settlement still ends the run and clears the tracked activity, including abort and later-run cycles. A streaming `get_state` recovery probe must restore activity when a replay lost `agent_start`; reconcile its answer before later stream records, ignoring a stale answer if a newer native start/settlement occurred since the request. Compaction-only state is not a native agent run. Do not modify steer semantics or automatically replay rejected input.

Verification: add deterministic handled-prompt, refused-prompt, successful-compact and refused-compact interleavings, with assertions that only native settlement releases the active run and later idle commands still complete. Assert accurate refusal text in the existing error field while status remains running, controls-menu visibility, no new conversation notice, and no notice/serialized-format changes. Cover real runner/dashboard activity and steering admission with the existing fake-Pi process seam, including wrapped-ring recovery with adjacent state and command answers, stale probes after newer start/settlement, and compaction-only state; run focused mapper tests, the wsagent package suite and race-enabled lifecycle coverage. Obtain independent review before handoff for integration. Record remaining historical/live acceptance limits explicitly.

## Blocked (2026-11-14)

### Design Reviewer — block

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | Refusal visibility requires an explicit decision. | important | missing |

### Completeness Reviewer — pass

| # | Title | Severity |
|---|-------|----------|
