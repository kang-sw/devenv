---
title: Bound and filter ws-agent-list output
---

# Bound and filter ws-agent-list output

## Background

`ws-agent-list` currently returns every entry in the persistent RPC agent registry, including dormant agents, without filtering, sorting, pagination, or a response limit. Dormant records survive reload through the sidecar and are retained until capacity eviction, so the response can grow toward the default 256-record registry cap. This makes active agents and recent recovery targets progressively harder to find and spends model context on stale entries.

The list should remain a reliable recovery surface without emitting the full dormant history by default.

## Decisions

- The default response should show every active agent and only a bounded recent subset of dormant agents.
- The response should state how many older dormant records were hidden and direct callers to an explicit broader query.
- Add status filtering so callers can request running, idle, or dormant records deliberately.
- Add bounded pagination through a limit plus cursor (or an equivalent stable continuation contract) rather than an unbounded all-record response.
- Preserve lookup and recovery by alias or agent ID even when a record is omitted from the default list response.
- Preserve the registry capacity, dormant eviction, sidecar persistence, parking, and revival semantics; this ticket changes list discovery, not agent lifetime.
- Do not delete dormant session files merely because a record is hidden from or paged out of list output.

## Phases

### Phase 1: Add bounded, filterable agent listing

Define and implement the list request and response contract so default output prioritizes active work and recent dormant recovery targets. Provide explicit status selection, a caller-controlled bound, stable continuation, and a hidden/remaining count. Choose exact defaults and cursor representation during design without weakening the constraints above.

Verification must cover a registry near its 256-record capacity, mixed running/idle/dormant states, deterministic page traversal without duplicates or omissions, hidden-count accuracy, alias and ID recovery for records outside the first page, and unchanged capacity-eviction and sidecar-revival behavior.
