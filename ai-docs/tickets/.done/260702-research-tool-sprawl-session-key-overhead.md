---
title: investigate tool-sprawl and session_key threading overhead across the wsflow surface
sage-review: required
completed: 2026-07-02
---

# investigate tool-sprawl and session_key threading overhead across the wsflow surface

## Context

Found during a v0.31.1 dogfooding pass that exercised the full wsflow tool
surface (54 tools). All 54 tools are deferred, so each tool's first use in a
session pays a schema-load round trip; covering the full surface required
several separate load batches. Separately, nearly every tool requires a
`session_key` parameter, so a lead ends up re-injecting the same key on
essentially every call.

This may be partly inherent to how MCP tool deferral and the harness work
(not fully within wsflow's control), so this is framed as an open research
question rather than a committed feature.

## Suggestion

Investigate whether a curated bundle of commonly used read/query tools could
be pre-loaded together (reducing load-batch count), and whether a
session-scoped default key (so most calls don't need to pass `session_key`
explicitly) is feasible given the MCP/harness constraints. Treat this as
research: determine what's fixable on the wsflow side versus what's inherent
to the MCP + harness deferral model before committing to any specific
implementation.


## Resolution (2026-07-02)

Both questions resolve to "inherent to the MCP protocol / harness, not fixable on the wsflow server side." No code change identified; closing as findings-only.

**Q1 — Tool deferral / bundling.** `tools/list` (agents-plugin-tool/internal/mcp/server.go:222-223, `handle()`) returns one flat `[]map[string]any` from `s.filteredTools()` (server.go:3758) in a single synchronous response — every advertised tool's full schema is included in that one payload, with no pagination, grouping, category, or lazy-schema field anywhere in the wire shape. `filteredTools()` only filters by role/product-mode (mercenary-hidden, permanently-hidden, tool-allow-list); it has no concept of "bundle" or "load group". Confirmed against ai-docs/spec/mcp-tools.md and ai-docs/spec/plugin-runtime.md: neither documents any bundling/deferred-loading primitive, because none exists — the server has nothing to opt into. The per-tool "first use pays a schema-load round trip" behavior is Claude Code's own client-side `ToolSearch` deferred-tool UX layered on top of the already-fully-delivered `tools/list` response; it is not a second protocol round trip to the wsflow server, and wsflow has no lever to change how/when the harness chooses to surface schemas it already received. Nothing to fix wsflow-side; the harness UX is out of scope for this server.

**Q2 — Session-scoped default `session_key`.** Not architecturally possible without a protocol change, and undesirable even if it were. Evidence:
- `ServeStdio` (server.go:127-180) reads one shared stdio pipe and dispatches every JSON-RPC request through the same `*Server`/`sessionStore` with no per-caller connection object — lead and every subagent sharing the same MCP connection are indistinguishable at the transport layer. There is no connection-scoped identity for the server to bind a default key to.
- Session state itself is intentionally *not* process/connection affine: `sessionStore` (session_auth.go:71-109) is a flat filesystem store, one JSON file per key under `keys/<key>.json`, read fresh from disk on every lookup (`readRecord`, session_auth.go:359-372) — by design, so "a fresh MCP server instance... resolves a key by reading its file" (mcp-tools.md:109-112). There is no in-memory "current session" to default from even within one process lifetime.
- This statelessness is a deliberate, already-shipped architectural decision (`#260610-ephemeral-session-auth-model`, mcp-tools.md:84-152), replacing an earlier persistent actor/authority model specifically to close a "wrong-tree footgun" where root-omitted calls silently mutated the wrong worktree. A connection- or process-scoped implicit default would reopen exactly that footgun for any concurrent multi-root/multi-agent session (multiple worktrees, lead + delegates sharing a connection) — worse, since MCP gives no per-call caller identity to scope the default to. Reintroducing a default is a net regression against the invariant `#260610` was built to guarantee (mandatory explicit key, no keyless fallback to a foreign root).

**Verdict:** both concerns are inherent to (1) the MCP `tools/list`/harness deferred-loading model and (2) the statelessness of MCP JSON-RPC over a shared stdio transport, respectively. No follow-up ticket created — pursuing either would mean fighting the protocol or reverting a deliberate, already-justified security/correctness fix. Not worth pursuing further absent a host-side (Claude Code) API change for (1) or an MCP transport change for (2), neither of which wsflow controls.
