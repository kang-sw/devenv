---
title: libws-harness MVP planning research
related:
  260521-research-libws-harness-agent-substrate: source architecture discussion and recovered design constraints
  260514-research-ws-web-dashboard-direction: dashboard harness-library direction and owner-auth control-plane context
  260514-epic-ws-web-dashboard-mvp: eventual dashboard milestone that may absorb harness visibility work
  260429-research-host-neutral-ws-plugin: host-neutral plugin and backend abstraction anchor
  260524-epic-async-exec-job-surface: adjacent activity/transcript source model for future dashboard projection
related-mental-model:
  - ws-web-dashboard
  - named-agent-runtime
  - mcp-runtime
  - plugin-runtime
---

# libws-harness MVP planning research

## Background

`260521-research-libws-harness-agent-substrate` captured the target substrate:
a Rust `libws-harness` core owns canonical run records, typed JSONL transport,
filesystem-backed state, prompt-oriented model loops, output framing,
interrupt/steering acknowledgements, and a `ToolHost` extension boundary.

The next discussion should not need to rediscover how to turn that research into
an implementation roadmap. This ticket exists as accepted backlog research for
planning the later MVP epic and child tickets. It should preserve the intended
slice boundaries, sequencing, and ticket-population notes so a future session can
write the epic and children without reopening settled design decisions.

This is a planning research ticket, not the implementation target. It should be
closed when the actual MVP epic and initial child tickets exist with recoverable
scope, non-scope, cross-child decisions, and phase boundaries.

## Planning Goal

Create a future MVP epic for the smallest useful `libws-harness` integration
that can live inside `ws-dashboard/` without destabilizing current dashboard,
MCP, or named-agent behavior.

The MVP should prove:

- `harness-core` can own typed run/event state.
- `harness-cli` can act as a JSONL transport over that core state.
- Model output is stream-framed as start, per-delta JSONL events, and completed.
- Stdin JSONL control inputs receive correlated `SystemAck` outcomes.
- Run records persist as append-only JSONL and can be replayed for transcript
  recovery.
- The dashboard can eventually consume harness runs as read-only activity or
  transcript data without becoming run/session authority.

## Proposed MVP Epic

Future epic stem suggestion:

```text
2605xx-epic-libws-harness-mvp
```

Suggested title:

```text
libws-harness MVP
```

Epic scope should include:

- Rust harness core run/event substrate in existing `ws-dashboard/crates/harness-core`.
- JSONL CLI transport in existing `ws-dashboard/crates/harness-cli`.
- Minimal deterministic backend such as echo or scripted output for tests.
- Append-only run event persistence.
- Output-frame contract for model text.
- Correlated control acknowledgement contract.
- Read-only dashboard projection planning or implementation as a later child.

Epic non-scope should explicitly exclude:

- Real Ollama integration unless the MVP core/CLI contract is already stable.
- MCP tool execution through `ToolHost`.
- Full context compaction implementation.
- READ directive materialization.
- Dashboard-started interactive chat as a first slice.
- Agent start/cancel/retry controls in the dashboard.
- Replacing existing ws named-agent runtime.

Epic cross-child decisions should preserve:

- `libws-harness` owns run records and filesystem-backed state.
- `harness-cli` stays a thin JSONL adapter and does not own state.
- Machine I/O is newline-delimited JSON, not pretty text.
- JSON schemas are typed Rust structures, likely `serde` enums.
- Model output is framed as started/delta/completed events.
- Control inputs carry ids and explicit policies; output includes correlated
  applied/queued/ignored/failed acknowledgements.
- Dashboard integration is initially read-only and owner-authenticated.
- `ToolHost` is a core extension point, but concrete MCP/ws-native tool
  execution is a later slice.

## Child Ticket Population Plan

### Child 1: JSONL Run Substrate

Suggested stem:

```text
2605xx-feat-libws-harness-jsonl-run-substrate
```

Purpose:

Implement the first reviewable harness substrate in `harness-core` and
`harness-cli`.

Scope:

- Define `RunInput` and `RunEvent` serde shapes.
- Define stable run/event ids enough for tests and event correlation.
- Add append-only `events.jsonl` persistence owned by `harness-core`.
- Add a deterministic echo or scripted backend.
- Add `harness-cli run` that reads stdin JSONL and writes stdout JSONL.
- Emit `model_output_started`, per-delta `model_output_delta`, and
  `model_output_completed`.
- Emit `system_ack` for recognized input ids, including non-success outcomes.
- Test newline parsing, event persistence, replay basics, output framing, and
  ignored/failed/queued acknowledgement paths where they exist.

Deferred:

- Real model backend calls.
- ToolHost execution.
- Dashboard routes.
- Context compaction.

Verification expectation:

- Rust unit tests for event serde and store append/replay.
- CLI integration-style tests that feed JSONL stdin and assert JSONL stdout.

### Child 2: Harness Read-Only Dashboard Projection

Suggested stem:

```text
2605xx-feat-ws-dashboard-harness-activity-projection
```

Purpose:

Expose persisted harness runs to the dashboard as read-only activity/transcript
data without making the dashboard canonical run authority.

Scope:

- Add daemon read-only projection over configured or discovered harness run
  stores.
- Return bounded run summaries and transcript/event blocks.
- Preserve owner-auth route boundaries.
- Avoid exposing host paths, raw store paths, backend-private metadata, or
  pairing/session data.
- Fit the projection toward Activity Console source-neutral item and transcript
  concepts where practical.

Deferred:

- Dashboard controls to start, interrupt, cancel, or retry runs.
- Live watch streams.
- Tool execution or approvals.
- Compaction visualization beyond stored event display.

Verification expectation:

- Daemon route tests for auth rejection, success, bounded output, redaction, and
  malformed event handling.
- Frontend work should be a later child unless this ticket intentionally owns a
  visible read-only pane.

### Child 3: Prompt Profile And Backend Adapter Sketch

Suggested stem:

```text
2605xx-research-libws-harness-prompt-profile
```

Purpose:

Plan the prompt/profile layer before adding a real local model backend.

Scope:

- Decide the minimal `PromptProfile` or equivalent run config shape.
- Capture how system prompt, model/backend selection, output expectations,
  compaction policy, and tool policy attach to a run.
- Decide whether local Ollama is the first real backend child or whether a
  subprocess/scripted backend should mature further first.
- Preserve host-neutral compatibility with MCP, Agents skills, and Codex plugin
  packaging.

Deferred:

- Real prompt library authoring.
- ToolHost MCP execution.
- READ directive implementation.

### Child 4: Optional ToolHost Contract

Suggested stem:

```text
2605xx-feat-libws-harness-toolhost-contract
```

Purpose:

Introduce the `ToolHost` trait and tool-call event states after the run substrate
is stable.

Scope:

- Define the trait boundary and tool capability metadata.
- Emit `tool_call_requested` and accept matching `tool_result` over JSONL.
- Keep actual MCP/ws-native execution out of scope unless another child owns it.
- Test pending/completed/failed tool-call state transitions.

Deferred:

- MCP client implementation.
- Sandboxed file IO.
- Dashboard tool approvals.
- `ws.explore` namespace.

## Sequencing Recommendation

Start with Child 1. It is the smallest slice that proves the core contract and
does not require dashboard UI, real models, MCP tool routing, or compaction.

Run Child 2 only after Child 1's event file shape is stable enough for a daemon
projection. If the Activity Console work is still active, align Child 2 with the
source-neutral Activity Item and Transcript Block concepts rather than adding a
separate dashboard-only transcript vocabulary.

Run Child 3 before any real Ollama backend implementation. The user explicitly
expects prompt design to carry most of the harness value; adding a backend before
the prompt/profile contract risks producing a thin model wrapper without the
intended workflow semantics.

Run Child 4 after the substrate and prompt/profile boundaries are clear. ToolHost
is important, but it can expand scope quickly and should not block the MVP event
loop.

## Spec Planning Notes

Before promoting feature children to `ready/`, planned spec coverage will likely
belong in `ai-docs/spec/ws-web-dashboard/index.md` only for dashboard-visible
behavior and in another spec if the harness substrate is treated as a separate
runtime surface. During epic creation, decide whether to add a new spec file for
`libws-harness` or to extend the dashboard spec.

Suggested split:

- Harness core/CLI JSONL behavior: new spec entry or file if it is public enough
  to be reused by MCP/CLI/dashboard adapters.
- Dashboard projection behavior: `ws-web-dashboard` spec.
- ToolHost/MCP execution behavior: likely `mcp-tools` or a future harness spec,
  depending on whether the behavior is generic or ws-specific.

## Risks To Preserve

- If the CLI owns state, dashboard and MCP adapters will either shell out or
  diverge from CLI semantics.
- If model output is raw stdout, callers cannot reliably multiplex transcript
  display, system acknowledgements, and tool/control events.
- If dashboard integration starts with mutation controls, it may blur the
  existing dashboard boundary that forbids dashboard MCP/session authority.
- If ToolHost execution lands before event persistence, tool behavior will drive
  the substrate shape instead of fitting the run record.
- If real Ollama integration lands before prompt/profile design, the MVP may
  look complete while missing the core workflow value.

## Closeout Criteria

Close this research ticket when:

- The MVP epic exists.
- Initial child tickets exist or are explicitly listed as planned children.
- The epic records scope, non-scope, cross-child decisions, and completion
  criteria.
- The first implementation child is narrow enough to promote through spec gating
  without reopening the architecture discussion.
- Follow-up tickets preserve the model-output frame, stdin control ack, core
  state ownership, and dashboard read-only boundary.
