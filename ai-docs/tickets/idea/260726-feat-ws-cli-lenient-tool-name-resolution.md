---
title: "ws-cli lenient tool-name resolution + normalized-uniqueness guard"
sage-review-design: required
---

# ws-cli lenient tool-name resolution + normalized-uniqueness guard

## Background

`ws-cli` / `ws-mcp` `call` and `tools` currently require the exact registered
tool name. During a recovery where the host MCP connection was down but the
daemon remained available, an agent driving the daemon-hosted CLI confused
separator styles: it transliterated `ws/project_tree` to `project.tree` and hit
`unknown tool` on the first call. This is high-stakes — the daemon CLI is the
recovery lever when the host MCP surface is down, and it failed at step one on
a trivial naming variant.

The current server inventory already mixes separators (`runtime.read` uses a
dot and `project_tree` an underscore in
`internal/mcp/server_schemas.go:31,549`). `tools/list` serves the live filtered
inventory (`server.go:365-374`; `server_inventory.go:71-114`), so an agent has
no reliable way to guess the exact separator. Resolution should be lenient
about separator style and an optional `ws/` prefix, while the canonical names
surfaced in the `tools` listing stay unchanged.

## Decisions

- **Lenient resolution (authoritative, do not re-litigate).** Resolve tool names
  by normalizing: strip any leading `ws/` prefix and treat `.` and `_` as
  interchangeable. All of `ws/project_tree`, `ws/project.tree`, `project_tree`,
  and `project.tree` must resolve to the same tool.
- **Only RESOLUTION becomes lenient.** The canonical names emitted by the
  server's filtered `tools/list` inventory are unchanged; leniency applies to
  how an inbound name is matched against that registered set, not to what is
  advertised.
- **Never silently mis-route (hard constraint).** Lenient matching must never
  route to the wrong tool. A build-failing uniqueness guard over the normalized
  key of every registered tool is mandatory (Phase 2), not optional.

## Constraints

- The daemon CLI's `hostedCLI.mcpCommand` sends `call` through `tools/call`, but
  its `tools <name>` form does an exact local match over the live `tools/list`
  response (`cmd/ws-mcp/hosted_cli.go:527-590`). The normalization function must
  be shared so `Server.callTool` and that CLI lookup resolve identically against
  the current server inventory.
- Do not renumber or rewrite existing switch case labels; canonicalize the
  inbound name to the exact registered name before the switch instead.

## Prior Art

- Resolution site (server): `Server.callTool` parses `params.Name` and applies
  only descriptor aliases before the dispatch switch
  (`internal/mcp/server_dispatch.go:22-38,218`; unknown names are rejected at
  `:189-192` on schema lookup or by the dispatch default).
- Registered-name source of truth: `tools()` in
  `internal/mcp/server_schemas.go` (including `runtime.read` at `:31` and
  `project_tree` at `:549`), augmented with session-hire, Pi and agent
  definitions at `:1183-1185`, then filtered for the connection by
  `server_inventory.go:71-114` and served at `server.go:365-374`.
- CLI entry point: `hostedCLI.mcpCommand`
  (`cmd/ws-mcp/hosted_cli.go:527-590`) passes the call name to `tools/call` and
  matches `tools <name>` exactly against the returned inventory.

## Phases

### Phase 1: lenient resolver + normalization function

- Add a normalization function (e.g. `normalizeToolKey(name string) string`)
  that strips a leading `ws/` prefix and folds `.` and `_` to a single canonical
  separator, producing the lookup key.
- Add a resolver that maps an inbound name to the exact registered tool name by
  comparing normalized keys against the registered set derived from
  the current server inventory. On a unique match, substitute the canonical
  registered name; on no match, preserve today's `unknown tool` behavior.
- Apply the resolver at the server chokepoint: canonicalize `params.Name` in
  `Server.callTool` (`internal/mcp/server_dispatch.go:22-38`) before the dispatch
  switch, so existing mixed-separator case labels receive the exact canonical
  string and stay untouched.
- Apply the same shared normalization to the exact `tools <name>` match in
  `hostedCLI.mcpCommand` (`cmd/ws-mcp/hosted_cli.go:566-588`). `call` inherits
  the server fix through `tools/call`, but verify both forms end to end through
  the daemon-hosted CLI.
- **Open sub-point:** case handling. Existing registered names appear to be all
  lowercase; match that convention. If normalization also case-folds, confirm no
  case-based collisions and note the choice explicitly. Decide during
  implementation and record the decision in this ticket's Result.

### Phase 2: normalized-uniqueness test guard

- Add a mandatory test that computes `normalizeToolKey` for EVERY registered
  tool in the server's canonical `tools()` inventory, including the appended
  session-hire, Pi and agent definitions, and fails the build if any two
  DISTINCT registered tools collide under normalization.
- This guard is the safety net for the hard constraint: mixed-separator
  registered names (e.g. `spec_stem.generate` alongside dot/underscore siblings)
  make normalized collisions plausible, so the test must run over the live
  registry rather than a hand-maintained list, and must fail loudly with the
  colliding names.
- The uniqueness invariant must hold for the resolver in Phase 1 to be safe;
  land or run this guard together with Phase 1 so a future tool addition that
  introduces a collision breaks the build rather than silently mis-routing.
