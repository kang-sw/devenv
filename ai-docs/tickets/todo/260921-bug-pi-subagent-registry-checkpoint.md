---
title: "Pi adapter: keep a durable same-session subagent registry checkpoint"
related:
  260905-feat-ws-pi-push-only-child-reports: predecessor — introduced shutdown sidecar recovery for persistent Pi children
---

# Pi adapter: keep a durable same-session subagent registry checkpoint

## Background

The Pi adapter keeps its live subagent registry in memory. Its current recovery sidecar is written during shutdown and consumed and deleted during the next startup. If the lead process exits without completing a later shutdown snapshot, durable child homes and transcripts can remain while the registry index is lost. Knowing the former agent ID or alias is then insufficient because `ws-agent-list` and `ws-agent-send` consult only the reconstructed in-memory registry and do not discover child homes.

The recovery contract is intentionally limited to resuming the same parent Pi session. A new Pi session must not discover or adopt another session's children.

## Decisions

- Replace the shutdown-only, one-shot sidecar behavior with a persistent registry checkpoint owned by the parent Pi session identity.
- Keep the checkpoint after startup hydration. Reading it must not create a new loss window by deleting it.
- Persist semantically significant registry transactions, including membership, identity and alias changes, resumability metadata, and lifecycle transitions that affect whether a child can be resumed. High-frequency token, telemetry, subtree-watcher, and gutter-render events are not persistence boundaries.
- Restore checkpoint entries conservatively as dormant or interrupted. `ws-agent-send <alias-or-id>` is the explicit action that resumes a restored child.
- Do not automatically restart a child, resend a prompt, or replay pending questions, approvals, deliveries, or other in-flight work.
- Do not scan or adopt child homes belonging to a different Pi session. Separate sessions remain separate registry owners.
- Use a bounded full-registry snapshot rather than an append-only event log. Serialize writes per owner and atomically replace the checkpoint so an older completion cannot overwrite a newer revision.
- Prevent concurrent processes from acting as independent writers for the same parent-session checkpoint or resuming the same child generation. Recovery must not create a second writer for an existing subtree channel.
- Migrate an existing one-shot sidecar only after the persistent checkpoint has been committed successfully.

## Constraints

- Preserve the identity, alias, model and effort, tool/delegation configuration, session and prompt paths, fork context, and other resume metadata currently required to reconstitute a registry record.
- A registry mutation must not be reported as durably successful before its checkpoint transaction completes. Spawn and deletion paths may use explicit intermediate states so a crash cannot silently produce an unindexed process or resurrect a deleted record.
- A stored `running` value is historical evidence, not proof that a process survived restart.
- Checkpoint corruption or schema incompatibility must remain diagnosable; startup must not silently replace an unreadable checkpoint with an empty registry.
- Windows transient replacement contention must use bounded non-blocking retry rather than blocking the Pi event loop.
- Existing retention and capacity policies may still remove eligible dormant records and owned homes, but their registry removals must be reflected durably.

## Phases

### Phase 1: Durable same-session registry recovery

Introduce a versioned, revisioned checkpoint at a stable locator derived from the parent Pi session. Route recovery-significant registry mutations through one serialized asynchronous atomic-write path, and make startup hydrate the same-session registry without consuming the checkpoint. Retain compatibility with the current shutdown sidecar long enough to migrate an existing recoverable session safely.

Verify at least:

- A child spawned and checkpointed before an abrupt lead-process exit is listed after the exact parent Pi session is resumed.
- Both its raw ID and alias can address `ws-agent-send`, which resumes it from a dormant or interrupted state.
- Starting a different Pi session does not list or adopt the previous session's children.
- Startup hydration does not delete the authoritative checkpoint, and repeated same-session restarts preserve the registry.
- Spawn failure, alias reassignment, stop/park, resume, eviction, and deletion cannot leave an older snapshot authoritative.
- Token streaming, subtree publication, watcher duplication, and gutter repaint do not trigger checkpoint writes.
- A second writer for the same parent session is fenced before it can resume or publish for the same child generation.
- Corrupt or unsupported checkpoints are preserved and diagnosed rather than converted to an empty registry.
- Existing one-shot sidecars migrate only after the new checkpoint is durable.
