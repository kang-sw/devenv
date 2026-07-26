---
title: "ws-cli lenient tool-name resolution + normalized-uniqueness guard"
sage-review-design: required
---

# ws-cli lenient tool-name resolution + normalized-uniqueness guard

## Background

`ws-cli` / `ws-mcp` `call` and `tools` currently require the exact registered
tool name. During a real MCP-down recovery, an agent driving the CLI fallback
confused separator styles: it transliterated `ws/project_tree` to `project.tree`
and hit `unknown tool` on the FIRST call. This is high-stakes — the CLI fallback
is the fatal-recovery lever when the MCP surface is down, and it failed at step
one on a trivial naming variant.

The registered tool names themselves already mix separators (`runtime.info` uses
a dot; `project_tree` uses an underscore — see `internal/mcp/server.go:529`
switch labels and the `tools/list` builder around `internal/mcp/server.go:3890`),
so an agent has no reliable way to guess the exact separator. Resolution should
be lenient about separator style and an optional `ws/` prefix, while the
canonical names surfaced in the `tools` listing stay unchanged.

## Decisions

- **Lenient resolution (authoritative, do not re-litigate).** Resolve tool names
  by normalizing: strip any leading `ws/` prefix and treat `.` and `_` as
  interchangeable. All of `ws/project_tree`, `ws/project.tree`, `project_tree`,
  and `project.tree` must resolve to the same tool.
- **Only RESOLUTION becomes lenient.** The canonical names emitted by the
  `tools` listing (`listMCPTools()` output) are unchanged; leniency applies to
  how an inbound name is matched against that registered set, not to what is
  advertised.
- **Never silently mis-route (hard constraint).** Lenient matching must never
  route to the wrong tool. A build-failing uniqueness guard over the normalized
  key of every registered tool is mandatory (Phase 2), not optional.

## Constraints

- Both the MCP `tools/call` path and the CLI `call` path funnel through
  `Server.callTool` (`internal/mcp/server.go:485`), but the CLI `tools`
  subcommand does its own local lookup over `listMCPTools()` and does NOT go
  through the server (`cmd/ws-mcp/main.go:951`). The normalization function must
  be shared so both the server dispatch and the CLI-local lookup resolve
  identically.
- Do not renumber or rewrite existing switch case labels; canonicalize the
  inbound name to the exact registered name before the switch instead.

## Prior Art

- Resolution site (server): `Server.callTool` `switch params.Name`
  (`internal/mcp/server.go:529`); the unmatched `default:` returns
  `errorResponse(..., -32602, "unknown tool: %s")` at `internal/mcp/server.go:1720`.
- Registered-name source of truth: `listMCPTools()`
  (`cmd/ws-mcp/main.go:934`) and the `tools/list` builder around
  `internal/mcp/server.go:3890` (e.g. `"name": "project_tree"`).
- CLI entry points: `toolsCommand` (`cmd/ws-mcp/main.go:951`, iterates
  `listMCPTools()` matching `toolName != name`, fatals `tool not found`) and
  `callCommand` (`cmd/ws-mcp/main.go:1000`, sets `request.Params.Name = name`
  verbatim).

## Phases

### Phase 1: lenient resolver + normalization function

- Add a normalization function (e.g. `normalizeToolKey(name string) string`)
  that strips a leading `ws/` prefix and folds `.` and `_` to a single canonical
  separator, producing the lookup key.
- Add a resolver that maps an inbound name to the exact registered tool name by
  comparing normalized keys against the registered set derived from
  `listMCPTools()`. On a unique match, substitute the canonical registered name;
  on no match, preserve today's `unknown tool` behavior.
- Apply the resolver at the server chokepoint: canonicalize `params.Name` in
  `Server.callTool` (`internal/mcp/server.go:485`) BEFORE the `switch params.Name`
  at `internal/mcp/server.go:529`, so the existing mixed-separator case labels
  receive the exact canonical string and stay untouched.
- Apply the same shared normalization at the CLI-local lookup in `toolsCommand`
  (`cmd/ws-mcp/main.go:951`) so `ws-mcp tools <name>` resolves leniently too;
  `callCommand` inherits the fix through the server path but should be verified.
- **Open sub-point:** case handling. Existing registered names appear to be all
  lowercase; match that convention. If normalization also case-folds, confirm no
  case-based collisions and note the choice explicitly. Decide during
  implementation and record the decision in this ticket's Result.

### Phase 2: normalized-uniqueness test guard

- Add a mandatory test that computes `normalizeToolKey` for EVERY registered
  tool (enumerated via `listMCPTools()` / the server tool registry) and fails
  the build if any two DISTINCT registered tools collide under normalization.
- This guard is the safety net for the hard constraint: mixed-separator
  registered names (e.g. `spec_stem.generate` alongside dot/underscore siblings)
  make normalized collisions plausible, so the test must run over the live
  registry rather than a hand-maintained list, and must fail loudly with the
  colliding names.
- The uniqueness invariant must hold for the resolver in Phase 1 to be safe;
  land or run this guard together with Phase 1 so a future tool addition that
  introduces a collision breaks the build rather than silently mis-routing.
