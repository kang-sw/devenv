---
title: ws dashboard Activity Feed API
parent: 260518-epic-ws-dashboard-activity-console
related:
  260518-feat-ws-dashboard-activity-read-model: absorbs this feed-only API scope into a reviewable read-model slice
  260517-feat-ws-dashboard-workroot-activity: source projection to generalize from named agents to feed items
  260513-feat-async-exec-output-reader: future exec jobs should fit the feed item model after that runtime exists
related-mental-model:
  - ws-web-dashboard
  - named-agent-runtime
---

# ws dashboard Activity Feed API

## Background

This feed-only ticket was dropped during Activity Console cascade refinement.
The accepted implementation sequence treats the Activity Feed snapshot and
selected transcript backfill as one backend read model because they share source
resolution, redaction, degraded-state handling, and UI contract verification.

The active replacement is
`260518-feat-ws-dashboard-activity-read-model`.
