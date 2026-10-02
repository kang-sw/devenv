---
title: lead-run worker spawn has no fallback when the ws:effort-* agent type is unavailable
---

# lead-run worker spawn has no fallback when the ws:effort-* agent type is unavailable

## Background

Dogfood surprise, 2026-10-02 (Claude Code lead, ws@0.46.26 installed):
lead-run's Spawn step 6 prescribes `subagent_type: "ws:effort-<effort>"`
and falls back to `general-purpose` only when the resolved effort is not one
of the five known values. In this session `Agent(subagent_type:
"ws:effort-high")` failed with "Agent type 'ws:effort-high' not found"; the
session's agent list contained no `ws:effort-*` types even though the
installed plugin manifest registers `./claude-agents/effort-*.md` and the
files exist. The lead retried with `general-purpose` and the worker's own
reviewer spawn hit the same miss.

## Open questions

- Why the registered plugin agents were absent from the session (plugin
  snapshot refreshed after session start, harness version, or registration
  shape).
- Whether the shipped spawn text should name an explicit fallback for an
  unavailable agent type, and what it loses (the effort pin) when it does.
