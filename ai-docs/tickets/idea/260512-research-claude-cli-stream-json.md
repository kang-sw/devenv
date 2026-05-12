---
title: Claude CLI stream-json runner contract
---

# Claude CLI stream-json runner contract

## Background

The ws Claude named-agent runner currently invokes Claude CLI with
`--output-format json` and parses one terminal JSON object after the process
exits. Codex and Gemini runners already use streaming JSON output, which gives
the runtime stronger incremental tracking for session ids, output deltas, and
terminal event shape diagnostics.

Claude CLI advertises `--output-format stream-json` and
`--include-partial-messages`, so the Claude runner may be able to move to the
same incremental parsing model. Capture the stream contract before changing the
runner because Claude stream event shapes may vary across CLI versions, tool
events, hook events, partial-message flags, and final result events.

## Questions

- Which event fields identify assistant text deltas versus final full text?
- Does the stream include both partial text and a full final result, and if so
  which event should own caller-facing output to avoid duplicate accumulation?
- Where do `terminal_reason`, `hook_stopped`, and hook lifecycle events appear
  when `--output-format stream-json` is enabled?
- Can the runner preserve the existing runtime-managed `--session-id` and
  `--resume` behavior while parsing the stream incrementally?
- What fallback or version gate is needed for older Claude CLI versions that do
  not support stream-json or partial-message flags?
- Should non-JSON stdout before completion remain a hard parse failure, matching
  Codex policy, or should Claude tolerate known hook/plugin notices?

## Probe Plan

Capture fixtures from representative Claude CLI calls before implementation:

- first call with runtime-provided `--session-id`
- resumed call with `--resume`
- partial assistant text with `--include-partial-messages`
- tool use plus final assistant text
- hook-stopped turn with current ws interrupt-hook delivery
- process failure or authentication failure

Use those fixtures to define parser tests before switching the live runner from
single-result JSON to stream-json.
