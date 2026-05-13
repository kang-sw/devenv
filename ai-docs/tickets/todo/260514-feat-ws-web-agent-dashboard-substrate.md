---
title: ws web agent dashboard substrate
parent: 260514-epic-ws-web-dashboard-mvp
related:
  260513-feat-async-exec-output-reader: adjacent persisted process output and reader-agent pattern
related-mental-model:
  - named-agent-runtime
  - mcp-runtime
---

# ws web agent dashboard substrate

## Background

The dashboard should make ws named-agent state visible without requiring the
browser to parse internal cache files directly. The daemon should expose stable
view models over wsstate/wsagent behavior and stream useful lifecycle updates.

This ticket may expose missing runtime API needs, such as active agent listing.
Any such API changes should be split or specified before ready promotion.

## Phases

### Phase 1: Add agent view-model API

Expose workspace-scoped named-agent summaries with status, backend/model,
session id where available, current-call state, last output metadata, and
follow-up actions.

### Phase 2: Add event and diagnostic views

Expose bounded recent events, tails, stdout/stderr diagnostics, and current-call
metadata through daemon APIs that preserve context limits.

### Phase 3: Add dashboard panels and commands

Register agent list/detail panels plus commands for status, tail, result,
cancel, erase, and future call/register workflows as appropriate.

### Phase 4: Verify cache and lifecycle resilience

Check empty cache, missing agent files, stale running pids, completed calls,
failed calls, and multiple worktrees without leaking one workspace into another.
