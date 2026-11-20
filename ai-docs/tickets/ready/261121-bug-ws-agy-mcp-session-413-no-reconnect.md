---
title: "대용량 MCP 요청의 413 오류와 agy 세션 복구 누락"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 1dfa4d29d0136321
sage-review-completeness-reviewed: 1dfa4d29d0136321
---

# Large MCP requests return 413 and hosted agy lacks MCP recovery

## Background

An observed hosted agy `whiteboard.update` with a large SVG panel received HTTP 413; the reporter observed that agy then lost ws MCP tools for the rest of that session. The request and agy aftermath are field observations, **not reproduced from a recorded agy trace in this ticket**. The server-side cause is verified: `agents-plugin-tool/internal/wsdashboard/proxy_http.go:33-36` sets `httpMCPBodyLimit` to 65,535 bytes because the HTTP request is forwarded to the default scanner; `:112-119` answers an oversized body with HTTP 413, even though it includes a JSON-RPC error body. `agents-plugin-tool/internal/wsdashboard/proxy_http_test.go:567-586` pins that response. The route forwards accepted bodies as newline-delimited JSON to the same MCP server (`proxy_http.go:16-25,399-408`).

The stdio intake uses `bufio.NewScanner(in)` without a larger buffer (`agents-plugin-tool/internal/mcp/server.go:190-203`). A scanner token error ends the serve loop (`:234-235,312-315`), so an oversized stdio request can end that MCP connection. This is a source-backed failure mode, not a reproduced stdio incident. The HTTP and stdio limits must be handled together.

Hosted agy's current recovery gap is verified in ws code: `agents-plugin-tool/internal/wsagent/backend.go:166-171,247-256` defines and selects an optional MCP controller; Claude's hosted lead supplies it (`internal/wsagent/claude.go:505-511,565-593`), while `internal/wsagent/agy.go:230-250` supplies no MCP controller. `internal/wsagent/runner_recovery.go:91-95` returns immediately without one. The runner only starts a check when it sees an MCP report or check event (`internal/wsagent/runner_events.go:243-249`); agy does not presently emit either from its MCP path. A manual lead process restart exists at `agents-plugin-tool/internal/wsagent/runner.go#L1427-L1437`, and the ordinary idle restart requires a known broken/stale state (`internal/wsagent/runner_recovery.go:190-226`). The claim that agy itself treats every 413 as fatal is **inferred from the reported field behavior**, not established by the inspected ws source or ACP documentation.

ACP v1 documents `session/resume` with a complete `mcpServers` list and says it reconnects those servers ([ACP v1 session setup](https://github.com/agentclientprotocol/agent-client-protocol/blob/main/docs/protocol/v1/session-setup.mdx#resuming-sessions)). [Antigravity's ACP documentation](https://antigravity.google/docs/ide/extensions/acp/#sessions) confirms agy supports resume. ws already sends `mcpServers` on `session/new` and `session/resume` (`internal/wsagent/agy.go:279-325`), and the test fake advertises resume (`internal/wsagent/agy_lead_test.go:87-118`). These sources do **not** prove that a second `session/resume` on a live agy ACP process repairs a failed MCP connection without disrupting a running turn or background work. That behavior needs a focused read-only capability check before implementation is settled.

## Decisions

1. **Repair request framing across both transports with one shared bound, 64 MiB ((1940), value revised 2026-11-21, owner confirmation pending as (1945)).** (1940) set 16 MiB, but `1b0899f66` (261121-bug-ws-dogfood-context-routing-gaps) since landed `maxMCPRequestBytes = 64 << 20` for stdio (`agents-plugin-tool/internal/mcp/server.go:95,208`) and pins a 17 MiB `path.write` through `ServeStdio` (`TestPathWriteRefusesSymlinkReplacementAndAcceptsOversizedContent`, `internal/mcp/generated_path_write_test.go`); 16 MiB would regress that. The shared constant therefore takes the landed 64 MiB value; if (1945) restores 16 MiB, the `path.write` artifact cap drops with it in the same change. A single exported request-size constant in a new leaf package `internal/mcpframe` (no project imports), imported by both `internal/mcp` and `internal/wsdashboard`, bounds both HTTP and stdio and replaces `maxMCPRequestBytes`; it cannot live in `internal/mcp`, because `internal/mcp` already imports `internal/wsdashboard` (e.g. `dashboard_config.go`, `dashboard_inbox.go`) and the reverse import would be a cycle. HTTP reads at most bound+1 bytes; an oversized body gets HTTP 200 with JSON-RPC error `-32600` whose `id` is recovered from the bounded prefix when parseable, else `null`. Stdio replaces `bufio.Scanner` with a `bufio.Reader` line loop that discards the rest of an oversize line, replies the same per-request error, and keeps serving; later requests stay usable. Rejected alternatives: raise only the HTTP cap, which would feed a larger line into the stdio scanner and end the shared session (`proxy_http.go:33-36`, `mcp/server.go:202,312-315`); raise `Scanner.Buffer` alone, because a Scanner still ends the loop on the first too-long token; per-transport limits, which drift apart.
2. **Hosted agy MCP recovery: the existing stale path already restarts fresh + `session/resume`; Phase 2 records the probe and pins it ((1941); revised 2026-11-21 from the recorded probe).** The Phase 2 probe ran (agy 1.5.13, `agy acp --process-per-session` against a loopback mock MCP server named `ws`; full record goes in the Phase 2 Result). Findings: agy emits no ACP MCP-server status (signal i absent); every failed ws tool call — tool error, HTTP 413/401/503, EOF — produces the same `tool_call_update` with `status: "failed"` and no error text, so transport failures are not distinguishable on the wire (signal ii absent); a single HTTP 413 did not kill the ws server on 1.5.13, but 401 followed by server loss made agy silently unregister `mcp_ws_*` with no ACP event at all. Recovery: a second `session/resume` on the live process (path a) registers a hash-suffixed `ws_<hex>` server and leaves `mcp_ws_*` dead, breaking the reserved `mcp_ws_*` naming, so it is rejected; a fresh agy process plus `session/resume` (path b) re-registers `ws` cleanly with history preserved. Decision: no new agy controller. The common stale path already supplies path (b) for every backend: a rejoined process whose route moved or whose HTTP-only start was incomplete is marked `mcpStale` (`internal/wsagent/runner_process.go:397-418`) and restarted at replay completion by `restartIfDue` (`:683-688`; `internal/wsagent/runner_recovery.go:207-229`) without consulting a controller, and agy's open uses `session/resume` whenever the recorded session ID is set (`internal/wsagent/agy.go`). Phase 2 therefore records the probe and pins that existing behavior for agy with a regression. A silent agy-side drop with no daemon-side cause is not detectable from ACP or the daemon on agy 1.5.13 and stays covered by the existing manual lead restart; the Result records that limit. Rejected: path (a) (renames the server); heuristics on `tool_call_update` failures (indistinguishable from tool errors, would restart on ordinary tool failures); a periodic tools probe (agy exposes no ACP tool list); a fatal-HTTP-status trigger from `/mcp/session` (no safe attribution for 401/403, and it would need a new route-to-runner contract).
3. **Defer `ws/reload_harness` (owner, 2026-11-21).** This ticket covers transport and agy recovery only. Rejected alternative: add a new `ws/reload_harness` tool now; the owner deferred it and will revisit only if removing 413 plus agy reconnect still leave manual recovery necessary.
4. **Compatibility: provisional T0, patch eligible.** The intended change keeps the existing `/mcp/session` route, JSON-RPC method/argument names, and stored data shape (`proxy_http.go:29-30,105-153`; `mcp/server.go:245-258`). An old caller sending a previously accepted request still receives the same success path; a new daemon accepts a larger message and gives a JSON-RPC error for excess size. An older daemon still returns 413 for a large new request (`proxy_http.go:112-119`), so large-payload support requires the updated daemon; no old data is written or overwritten, and reopening with the new daemon has no data to reconstruct. Rejected alternative: classify this as T3 solely because the error HTTP status changes; there is no old-version refusal, store migration, or protocol/API removal in the proposed scope. Re-grade before release if implementation changes a cross-version call, session, or store contract (`ai-docs/manuals/release-compatibility.md:52-76`).

## Constraints

Matching `AGENTS.md` Implementation Conventions rows (paths are relative to the repository root; AGENTS.md#L181-L190):

| paths | manual |
|---|---|
| `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/` | `ai-docs/manuals/shipped-surface-boundary.md` |
| `agents-plugin-tool/internal/mcp/` | `ai-docs/manuals/ws-mcp.md` |
| `agents-plugin-tool/internal/` | `ai-docs/manuals/release-compatibility.md` |
| `agents-plugin-tool/` | `ai-docs/manuals/performance.md` |

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)
- Convention: ai-docs/manuals/release-compatibility.md (declared for agents-plugin-tool/internal/)
- Convention: ai-docs/manuals/performance.md (declared for agents-plugin-tool/)

- Keep the line reader bounded without allocating an unbounded request body; preserve the HTTP token/session path and unrelated `/agents` request limits (`internal/wsagent/http.go:86,391-400`).
- Count added file operations per MCP request and agy recovery action; the performance manual requires memory-first interactive paths and measurement for process restarts (`ai-docs/manuals/performance.md:13-47,63-73`).

## Prior Decisions

- 261117-feat-ws-hosted-antigravity-adapter (2026-11-18, Result): "MCPSession.Connected closes when the session's token first resolves on the daemon's route." — bearing: constrains
- 5d018785 (2026-11-14, commit): "Register all canonical intake operations from connection ingress, preserving legacy concurrency before syntactic v1 and adding a bounded actual-callback quiescence barrier before the first optional mutation." — bearing: constrains
- 261110-feat-ws-dashboard-headless-lead-sessions (2026-11-11, commit): "Auto restart only for a server seen connected in this process: a server that failed from the start would most likely fail a restart the same way, so a restart loop is avoided and the user restarts by hand." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/mcp/server.go, agents-plugin-tool/internal/wsdashboard/proxy_http.go, agents-plugin-tool/internal/wsagent/agy.go and tests |
| scope.surface | cross-module | shared request bound crosses internal/mcp and internal/wsdashboard; recovery crosses internal/wsdashboard and internal/wsagent |
| scope.new_public_symbol | yes | Decision 1 requires an exported bound in the new leaf package internal/mcpframe, imported by internal/mcp and internal/wsdashboard (internal/mcp already imports internal/wsdashboard, so the bound cannot live in internal/mcp) |
| scope.new_type_contract | no | Decision 2 maps an ACP-observed signal onto the existing MCP check event path (`internal/wsagent/runner_events.go:243-249`); no new cross-module type |
| scope.test_surface | existing | agents-plugin-tool/internal/wsdashboard/proxy_http_test.go and agents-plugin-tool/internal/wsagent/agy_lead_test.go exist; Phase 1 requests a stdio regression |
| complexity.reuse_points | confirmed | existing MCPSession token attachment and Runner.Restart path at agents-plugin-tool/internal/wsagent/mcp_session.go and runner.go |
| complexity.side_effect_risk | high | changes shared MCP framing and may restart a hosted lead process |
| risk.correctness | high | oversized lines must preserve later requests and agy recovery must restore tool calls |
| risk.fit | high | live-process resume safety and the failure signal require a probe and design resolution |
| risk.test | high | needs real serve-loop regression and installed agy ACP probe |
| risk.security_or_contract | high | changes JSON-RPC error handling and a bearer-token-gated MCP transport |

## Blocked (2026-11-21)

Phase 1 waits for owner answer (1945) on the shared request bound (64 MiB as landed by `1b0899f66`, or 16 MiB with a lower `path.write` cap). Phase 2 is settled. Remove this section when (1945) is recorded in Decision 1.

## Phases

### Phase 1: One request bound for HTTP and stdio MCP intake

Implement Decision 1. A representative `whiteboard.update` or equivalent tool call with a payload above 64 KiB (and below the shared bound) must succeed over both transports. A request above the shared bound must receive a per-request JSON-RPC error without terminating the session; a following ordinary call on the same session must succeed. Update `proxy_http_test.go`'s 413 assertion and `TestServeStdioRejectsPathWriteBeyondRequestLimit` (now a per-request error plus continued serving, not `token too long`), and add a stdio regression that exercises the real serving loop, not only the parser. Re-confirm the compatibility tier against the final diff.

### Phase 2: Hosted agy MCP recovery

Implement Decision 2. Record the already-run probe (command, agy version, observed wire behavior, path a/b outcomes) in this phase's Result. Add a regression: a fake agy lead whose rejoined process is marked `mcpStale` (moved route) is restarted at replay completion as a fresh process that opens with `session/resume` on the recorded session ID; no new production code unless that regression fails, in which case fix the seam it exposes and say so in the Result. Do not add `ws/reload_harness`.

Recommended implementation tier: **large** for both phases (Phase 1 crosses the HTTP proxy and stdio framing; Phase 2 crosses hosted agy ACP behavior and recovery).
