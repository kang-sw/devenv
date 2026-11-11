---
title: "Hosted lead sessions: find a source for the subagent output rate"
related:
  - 261110-feat-ws-dashboard-headless-lead-sessions
  - 261110-feat-ws-dashboard-agent-panel-telemetry
---

# Hosted lead sessions: find a source for the subagent output rate

Phase 2 of 261110-feat-ws-dashboard-headless-lead-sessions (merged 40d746a6b)
maps subagent model, effort, and context tokens from the hosted lead's stream,
but not the output rate the ticket's verification named. The hosted lead runs
without partial messages, so every content block arrives whole and its token
count is the request's starting value: the stream carries no honest rate.

## Questions

- Turn on partial messages and filter them at the holder, so they do not
  flood the holder ring? Cost: stream volume and holder work per token delta.
- Is there another source (transcript files, usage fields on completed
  messages with timestamps, harness telemetry) that gives a usable rate
  without partial messages?
- Or drop the output-rate field for hosted sessions and say so in the panel.

## Also open from Phase 2 (Minor review findings)

- `error: rate_limit` with no other typed cause maps to usage limit; the
  failure shapes come from the Orca ADE fallback until a live capture exists.
- Spawn records for Agent calls that never start a task stay in memory until
  the process ends.
- The MCP check goroutine is not tracked by the runner's wait group and can
  outlive runner close by up to a minute.
- "Nothing restarts" test assertions use a 100 ms sleep.
