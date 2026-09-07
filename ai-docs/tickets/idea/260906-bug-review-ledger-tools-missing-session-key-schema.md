---
title: Review marker requires a session key absent from its advertised schema
---

# Review marker requires a session key absent from its advertised schema

## Background

During the authorized ws release on 2026-09-06, the connected MCP tool
`review.marker(format: "json")` returned:

```text
mandatory_session_key: root-aware ws tools require a session_key; if you are the lead, obtain one per ws:workflow-manual and pass it
```

The advertised schema exposed only optional `bootstrap` and `format`, so the
schema-conforming call could not satisfy the runtime's session requirement.
Retrying the same read with the existing lead `session_key` as an additional
top-level field succeeded and returned the review frontier. This host permits
that extra argument; downstream hosts may not.

## Phases

### Phase 1: Reconcile review-tool session arguments

Investigate the missing schema field and align the advertised contract with
runtime session requirements. Check the adjacent `review.stamp` surface,
whose currently advertised fields also omit `session_key`; its failure was
not reproduced at capture time. Preserve existing ledger semantics. The repair
mechanism and acceptance matrix remain for triage; this capture does not
authorize an additional release-scope code change.
