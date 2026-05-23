---
title: agents root schema remains visible
related:
  260505-bug-plugin-managed-default-root-discovery: supersedes stale Windows root-discovery backlog item
  260505-feat-mcp-session-default-root: depends on the completed ws.setup session-root contract
spec:
  - 260505-mcp-session-default-root
  - 260505-named-agent-mcp-tools
related-mental-model:
  - mcp-runtime
  - named-agent-runtime
---

# agents root schema remains visible

## Background

The settled root model is that the lead session chooses the current repository
once through `ws.setup(root)`. Normal workflow and named-agent calls should then
omit `root`; explicit `root` exists only as a compatibility override for broken
host startup, tests, and exceptional multi-root recovery.

Current behavior still exposes the wrong affordance. The live Codex tool surface
can show optional `root` parameters on `agents.register`, `agents.call`,
`agents.wait`, `agents.result`, `agents.status`, `agents.tail`, `agents.cancel`,
and `agents.erase`. That pushes callers and generated guidance toward passing
the repository root on every named-agent operation, even though root is a
session-context decision owned by the lead agent, not per-agent workflow state.

The source tree also carries mixed signals: raw `agents.*` schemas still include
`root`, while later schema filtering and docs say public `agents.*` schemas
should omit it. This makes stale plugin caches, generated tool metadata, tests,
and reference text easy to desynchronize.

## Decisions

- Keep `ws.setup(root)` as the public root-session setup surface.
- Remove `root` from the advertised/generated public `agents.*` schemas
  end-to-end, not only from one post-processing path.
- Keep hidden compatibility acceptance of explicit `root` in dispatch for now,
  so older callers and tests can recover when host startup did not bind a root.
- Do not remove `root` from all root-aware MCP tools in this slice. Repository
  document, Git, API-doc, and path tools may still need explicit compatibility
  overrides until their callers are audited separately.
- Do not make skills root-aware. Workflow skill text should call `ws.setup`
  when the session root must be established and otherwise omit `root`.

## Phases

### Phase 1: Hide agents root schemas end-to-end

Make `agents.*` root arguments invisible across the actual public MCP schema
surface used by hosts. The implementation should remove `root` from the raw
advertised schema for named-agent tools rather than relying only on a
late filtering pass, then keep or simplify the defensive filter as appropriate.

The phase should cover at least:

- `agents.register`
- `agents.call`
- `agents.wait`
- `agents.result`
- `agents.status`
- `agents.tail`
- `agents.cancel`
- `agents.erase`
- compatibility aliases or debug surfaces when they are publicly advertised

Dispatch may continue to tolerate explicit `root` as a hidden compatibility
override, but generated schemas, runtime reference examples, skill guidance, and
tests must not present it as the normal caller surface.

Verification should prove that:

- `tools/list` for full ws mode does not advertise `root` on public `agents.*`
  tools;
- generated/host-visible tool metadata used by Codex no longer includes
  `root` for those tools after a plugin/runtime refresh;
- root-omitted `agents.*` calls work after `ws.setup(root)`;
- explicit `root` compatibility calls still work if intentionally preserved;
- no-agent/wsflow mode does not reintroduce an agent root schema surface.
