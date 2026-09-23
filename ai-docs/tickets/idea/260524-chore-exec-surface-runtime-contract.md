---
title: Exec surface runtime contract closeout
parent: 260524-epic-async-exec-job-surface
related:
  260524-feat-exec-job-core-text-readers: introduces the core exec tool surface
  260524-feat-exec-output-ask: introduces the model-backed output question surface
related-mental-model:
  - mcp-runtime
  - plugin-runtime
---

# Exec surface runtime contract closeout

## Background

The exec surface changes the public MCP tool inventory, runtime capability
contract, wsflow no-agent hidden-tool behavior, and possibly CLI mirror surface.
Each implementation child should update the metadata it directly touches, but a
final closeout child is useful after the accepted `exec.*` surface is stable so
the package contract cannot drift.

## Decisions

- Treat this as a contract audit and packaging closeout, not as a new exec
  behavior slice.
- Full ws may expose `exec.*` tools; wsflow no-agent mode must hide the entire
  exec surface because it is an arbitrary execution surface and `exec.ask` is
  agent-backed.
- Runtime capability output and plugin `runtime.json` manifests must match the
  final accepted MCP tool surface.
- CLI mirror policy remains the only intentionally open decision: include CLI
  mirrors only for surfaces that are useful and safe as local operator commands.
  The closeout should document either the included mirrors or the reason MCP-only
  behavior is sufficient.

## Phases

### Phase 1: Close exec runtime and package contracts

After the core and ask children settle the final MCP tool names, audit and align:

- `tools/list` and explicit `tools/call` visibility;
- `runtime.capabilities` tool and command names;
- `agents-plugin/runtime.json`;
- `agents-plugin-wsflow/runtime.json` exact no-agent contract;
- wsflow hidden-tool tests and package drift tests;
- CLI mirror inclusion or explicit MCP-only rationale;
- release validation docs or ship checklist references when needed.

Update `mcp-tools`, `plugin-runtime`, and related mental-model docs before
promoting this child to `ready`.
