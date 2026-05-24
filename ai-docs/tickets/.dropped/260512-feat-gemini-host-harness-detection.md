---
title: Gemini host harness detection
related:
  260512-feat-gemini-named-agent-backend: separated from named-agent Gemini backend support
related-mental-model:
  - named-agent-runtime
---

# Gemini host harness detection

## Background

Gemini CLI is now supported as a ws named-agent backend, but host harness
detection still recognizes Codex and Claude hosts only. That is intentional for
the backend work: `backend: gemini` means ws should invoke Gemini CLI as a
delegate, while host harness means the MCP client currently calling ws is itself
Gemini or another host.

TBA: add Gemini host harness detection after observing the MCP initialize and
request metadata emitted by Gemini CLI or its MCP integration.

## Constraints

- Do not infer host harness from a `gemini-*` model name; Codex or Claude may
  intentionally register a Gemini-backed named agent.
- Preserve backend detection and host harness detection as separate concepts.
- Avoid changing portable model alias defaults until Gemini host metadata is
  observed and tested.

## Phases

### Phase 1: Capture Gemini host metadata

Observe Gemini CLI MCP initialize payloads, request `_meta`, environment
signals, and any stable product identifiers.

TBA.

### Phase 2: Add harness detection

Update host harness detection, normalized harness handling, alias defaults,
tests, and docs if stable Gemini host markers exist.

TBA.

## Drop Reason

Dropped on 2026-05-24. Google announced the consumer transition from Gemini CLI
to Antigravity CLI on 2026-05-19, with Gemini CLI and Gemini Code Assist IDE
extensions stopping requests for Google AI Pro, Ultra, and free individual use
on 2026-06-18:
https://developers.googleblog.com/an-important-update-transitioning-gemini-cli-to-antigravity-cli/

Per user direction, ws should stop carrying Gemini CLI host-harness detection as
active backlog. Any future Antigravity host or backend support should be scoped
as a new ticket from observed Antigravity CLI/MCP behavior instead of reviving
this Gemini-specific follow-up.
