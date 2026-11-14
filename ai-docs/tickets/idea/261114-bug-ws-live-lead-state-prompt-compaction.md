---
title: Live lead output disagrees with dashboard idle state and prompt admission
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

## Constraints and unknowns

No contemporaneous raw refused request, Pi get_state answer, idle UI snapshot or exact feed transition was recovered. The existing daemon log had no matching refusal text. Later health/snapshot evidence cannot reconstruct those states. Loaded bundle/source equivalence and causality of compaction recovery remain unverified. Historical isIdle/isCompacting notes are not proof.

Capture was read-only: no live prompt, restart, compaction, launcher, source edit or cleanup. MAIN retains the evidence-preservation hold until it judges the capture sufficient; this ticket grants no runtime replacement or implementation authority.

## Phases

### Phase 1: Establish the activity and prompt-admission divergence

Use the preserved exact incident window to distinguish native run/compaction state, displayed activity and prompt/steer dispatch. Identify recoverable telemetry gaps before proposing a bounded repair. Return any behavior/API decision through MAIN; do not infer permanent refusal or relax admission from this capture alone.
