---
title: ws-mcp hook-driven interrupt delivery
parent: 260503-epic-ws-agent-workflow-stability
related:
  260503-feat-ws-mcp-worktree-orchestrator-lock: introduced current-call serialization and initial inbox-backed agents.interrupt
  260503-feat-agents-plugin-async-agent-calls: async worker lifecycle and cancel surface
  260429-research-host-neutral-ws-plugin: host-neutral ws plugin architecture anchor
completed: 2026-05-04
---

# ws-mcp hook-driven interrupt delivery

## Background

`260503-feat-ws-mcp-worktree-orchestrator-lock` added durable
`agents.interrupt` messages and a working Linux smoke path, but the active
Codex delivery path currently relies on a worker-side inbox watcher that sends
an interrupt signal to the active Codex subprocess. That is too fragile to make
the cross-platform contract: Windows signal behavior is not equivalent to Unix
`SIGINT`, forced process termination can break JSONL pipes, and killing Codex is
closer to cancellation than interrupt delivery.

Follow-up hook testing on Codex CLI 0.128.0 / WSL2 Linux corrected the earlier
diagnosis. Inline `PostToolUse` hooks configured through `-c
features.codex_hooks=true` and `-c hooks.PostToolUse=...` do fire. The mismatch
is semantic: unlike the Claude prior art, Codex `PostToolUse` `exit 2` does not
stop the subprocess and hand control back to the wrapper. Codex treats the hook
stderr as feedback injected into the next model step and continues the turn.
That behavior is sufficient for cooperative interrupt delivery, but it should
be implemented as the primary path instead of relying on signals.

## Decisions

- Treat `agents.interrupt` as durable message delivery, not cancellation. If an
  active agent ignores the message, the lead can retry the interrupt or use
  `agents.cancel` for truly urgent stop-the-work behavior.
- Keep two message states: `pending` means the runtime has not yet injected the
  message into a Codex input path, and `delivered` means the runtime did inject
  it. `delivered` does not claim that the model complied with the instruction.
- Use event logs, not extra inbox states, for delivery route details such as
  `inbox.delivered_via_hook`, `inbox.delivered_via_resume`, or future
  re-offer diagnostics.
- Preserve the Claude prior art as an adapter pattern, but do not copy its
  control-flow assumption into Codex. Claude can stop a loop on hook exit 2;
  Codex uses hook feedback as model input.
- Keep process termination under `agents.cancel`. A best-effort soft signal may
  remain a non-contract optimization only if it is platform-gated and cannot
  corrupt normal interrupt delivery.

## Constraints

- Do not infer delivery from LLM response content. Heuristic acknowledgement
  detection would add clutter and false confidence.
- Do not mark messages as "handled" or "complied"; the runtime can only record
  queueing and input-path injection facts.
- Avoid pipe-breaking behavior in the normal interrupt path. Partial JSONL
  parsing should remain crash/cancel recovery, not the expected interrupt flow.
- Keep the filesystem inbox as the source of truth so delivery survives MCP
  server restarts, worker subprocess boundaries, and host variance.

## Prior Art

- `claude-plugin/bin/ws-named-agent` uses hook-triggered mailbox checks to break
  out of a Claude call loop and then drains queued outbox content.
- Codex hooks receive a JSON object on stdin. For `PostToolUse`, plain stdout is
  ignored, but stderr with `exit 2` is injected as hook feedback. JSON stdout
  using `decision: "block"` plus `hookSpecificOutput.additionalContext` also
  reaches the next model step.
- Codex documentation says `PostToolUse` `decision: "block"` does not undo the
  completed tool result; it records feedback, replaces the tool result with that
  feedback, and continues the model.

## Phases

### Phase 1: Hook-delivered inbox messages

Make `agents check-inbox` the primary active-turn delivery path for Codex:

- atomically claim pending inbox messages and mark them `delivered` when the
  hook injects them into Codex;
- emit the lead messages through a Codex-supported hook channel, preferably
  stderr with `exit 2` or JSON stdout if that proves clearer in code;
- log the route as `inbox.delivered_via_hook`;
- keep pending messages delivered at the start of the next backend call when no
  active hook claims them.

Success criteria:

- A minimal Codex hook smoke proves that an interrupt queued during an active
  tool-using turn is injected without terminating the subprocess.
- The worker no longer needs to signal or kill Codex for normal
  `agents.interrupt` delivery.
- Inbox state remains two-state (`pending` and `delivered`) with route details
  in runtime/event logs.

### Result - 2026-05-04

Implemented hook-driven inbox delivery for active Codex turns. The internal
`agents check-inbox` helper now claims pending messages through an inbox
delivery lock, marks them `delivered`, writes lead-message feedback to stderr,
and exits 2 so Codex injects the feedback into the next model step. Delivery
routes are logged as `inbox.delivered_via_hook` or
`inbox.delivered_via_resume`; the inbox state machine remains two-state and
does not infer model compliance.

### Phase 2: Demote signal interruption to cancel-only behavior

Remove signal-based active interrupt from the normal Codex runner path:

- delete or disable the worker-side `InterruptPending` goroutine for
  `agents.interrupt`;
- ensure partial JSONL interrupted-result handling is retained only for
  cancellation, timeout, or unexpected subprocess failure;
- document that `agents.cancel` is the supported path for urgent process stop.

Success criteria:

- Windows builds do not depend on `os.Interrupt` or forced process kill for
  interrupt delivery.
- `agents.interrupt` never relies on broken pipes or nonzero Codex process exit
  as its success condition.
- Existing `agents.cancel` behavior remains available for stop-the-work cases.

### Result - 2026-05-04

Removed the normal interrupt path's worker-side inbox watcher and Codex
subprocess signal/kill behavior. `RunnerRequest` no longer carries an
`InterruptPending` callback, and `CodexRunner` no longer sends `os.Interrupt`
for `agents.interrupt`. Nonzero Codex exits are treated as backend failures
instead of resumable interrupt delivery.

### Phase 3: Documentation and smoke repair

Update runtime documentation and local smoke coverage:

- correct `ai-docs/ref/codex-integration.md` to say Codex inline hooks fire on
  the tested WSL2 host, but `PostToolUse` exit 2 injects feedback rather than
  terminating the subprocess;
- update `ai-docs/ref/ws-agent-runtime.md` with the two-state inbox contract and
  cancel/interrupt boundary;
- add or update Go tests around hook command behavior and inbox state
  transitions without relying on LLM response heuristics.

Success criteria:

- `go test ./...` passes in `agents-plugin-tool`.
- A local Codex smoke verifies hook-driven delivery for an active turn and a
  no-active-call smoke verifies next-call delivery.
- Documentation no longer describes normal interrupt delivery as signal-driven.

### Result - 2026-05-04

Updated Codex and ws agent runtime docs to describe the tested hook semantics:
inline `PostToolUse` hooks fire on WSL2/Codex CLI 0.128.0, and `exit 2` injects
feedback instead of stopping the subprocess. Go tests now cover hook delivery,
resume delivery, two-state inbox transitions, and the absence of signal-driven
interrupt retry behavior.
