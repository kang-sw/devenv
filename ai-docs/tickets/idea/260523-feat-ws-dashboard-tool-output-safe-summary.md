---
title: Improve safe summaries for Codex tool output blocks
parent: 260514-epic-ws-web-dashboard-mvp
related:
  260523-feat-ws-dashboard-activity-console-tail-ribbon-polish: recent transcript density polish left tool output placeholders too generic
spec:
  - 260521-ws-dashboard-activity-console-read-model
  - 260522-ws-dashboard-activity-console-transcript-expansion
related-mental-model:
  - ws-web-dashboard
---

# Improve safe summaries for Codex tool output blocks

## Background

Dogfood of a live subquery agent showed Codex tool output transcript blocks
rendering only "Tool output captured". That text is not the original tool
output; it is the dashboard transcript normalizer's conservative placeholder
for Codex `function_call_output` and `custom_tool_call_output` records.

The current behavior avoids leaking raw native payloads, but it gives too little
information for Activity Console inspection. The block should remain safe and
bounded while surfacing useful one-line context such as output size, structured
status, short non-sensitive first line, or omitted-content reason.

## Follow-Up Questions

- Which Codex tool-output fields are safe to summarize without exposing private
  paths, prompts, session ids, or raw command output?
- Should expanded detail expose bounded raw JSON for local owner-only dogfood,
  or stay normalized-only until a redaction policy is stronger?
- Should tool output summaries differ for MCP calls, shell commands, patch
  applications, and generic function outputs?
