---
title: 사용자 실행 agy의 MCP clientInfo로 agy 하니스 식별
related:
  261120-feat-ws-agy-usage-activity-trigger: its Phase 2 (lead-call trigger) is inert for user-started agy until this lands
  261118-refactor-ws-harness-name-branches: umbrella; detection must stay descriptor-driven, not a new name branch
dropped: 2026-11-20
---

# Identify user-started agy from its MCP clientInfo

## Background

ws MCP resolves a session's harness from the `/mcp` hello (`WS_MCP_HARNESS`)
or from the `initialize` `clientInfo.name`
(`internal/mcp/server_harness.go`, `detectHarnessFromInitializeParams` and
`normalizedHarness`). Hosted agy children are identified through the hello
the daemon sets from the persisted backend record. A user-started agy lead has
no such hello, and its `clientInfo.name` matches nothing, so
`currentHarness()` stays empty. As a result, the agy usage trigger on lead
calls (261120-feat-ws-agy-usage-activity-trigger Phase 2) never fires for it.

Probe evidence (2026-11-20, agy 1.5.13, one owner-approved run in ACP mode
against a stub stdio MCP server, with no prompt and no login):
- `initialize` params:
  `{"clientInfo":{"name":"antigravity-client","version":"v1.0.0"},"protocolVersion":"2025-11-25","capabilities":{"elicitation":{"form":{},"url":{}},"roots":{"listChanged":true}}}`
- `server/discover` comes first and carries the same clientInfo under
  `_meta["io.modelcontextprotocol/clientInfo"]`. agy falls back to
  `initialize` when the server does not implement it, and ws answers unknown
  methods with -32601.
- The client version `v1.0.0` is unrelated to the agy release, so the name
  looks hardcoded in agy's MCP client layer. Whether interactive mode and the
  HTTP transport send the same name is inferred, not captured.
- Nothing in the repository sets up a ws MCP entry for a user-started agy.

## Open Decision Queue

Last ID: (1469)

(1465) [open] [announced] Map the agy MCP client name through the agy backend descriptor (a registration field listing MCP clientInfo names that resolve to its `ManualHarness`) rather than a literal name branch in `server_harness.go` - integrator directive and umbrella 261118-refactor-ws-harness-name-branches: prefer backend-descriptor callbacks over agy name checks in common code.
(1466) [open] [technical] Exact case-insensitive match on `antigravity-client` over a prefix or substring match - an exact name cannot capture an unrelated client; the version is ignored because it does not track agy releases.
(1467) [open] [technical] Pin the match with the captured `initialize` params stored as testdata (agy 1.5.13), driven through the real `initialize` handling, over a hand-written fixture - the test then fails when the recorded traffic no longer resolves.
(1468) [open] [announced] Compatibility T0: an in-process detection change with no change to the hello, the stores, tracker events, or manifests; an old binary simply leaves agy unidentified as today - AGENTS.md `### Compatibility Tiers`, T0 definition.
(1469) [open] [policy] Scope of user-side agy setup: this ticket only detects agy once it is connected; nothing ships a ws MCP entry for a user-started agy today.
- a: Out of scope. The user adds the entry by hand (`agy mcp add ...`); this ticket stays detection-only.
- b: Also document the manual `agy mcp add` step for ws in the shipped docs.
- c: A separate ticket for installer support of agy (writing agy's ws MCP entry).
> a for this ticket, with c raised separately if user-started agy leads are a real use: detection is needed whichever way the entry is created, and installer support touches a downstream-visible surface.

## Phases

### Phase 1: Resolve agy from its MCP clientInfo

Pending the queue.

Verification:
- A narrow `./internal/mcp` test, `-run` filtered: the recorded agy
  `initialize` resolves `currentHarness()` to `agy` through the descriptor,
  and a client name absent from every descriptor still resolves to empty.
- Existing Codex, Claude and Pi detection tests still pass.
- `go vet ./internal/mcp`.


## Resolution (2026-11-20)

Dropped on the owner's direction (2026-11-20): a user-started, non-hosted agy CLI was never meant to be supported. The agy usage trigger only needs daemon-hosted agy sessions, which 261120-feat-ws-agy-usage-activity-trigger Phase 1 already covers at turn end. The probe finding (agy 1.5.13 sends MCP clientInfo name "antigravity-client") is kept in the Background for reference.
