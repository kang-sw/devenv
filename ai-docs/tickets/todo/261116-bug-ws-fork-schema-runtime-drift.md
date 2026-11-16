---
title: "ws-fork advertised schema disagrees with runtime behavior"
related:
  261112-feat-ws-mixed-version-compat-guards: "distinguish schema/runtime drift from a host caching an older tool inventory"
---

# ws-fork advertised schema disagrees with runtime behavior

## Background

Dogfood #16, 2026-11-16: the lead reports that the `ws-fork` tool's schema and
runtime disagree. This worker did not observe a fork invocation or its schema
failure directly. The exact advertised schema, request, runtime response,
argument names, host/profile and version pins were not supplied. Do not infer
which field or producer is wrong, or conflate this with the separate fork
identity/authority incidents.

## Decisions

Compatibility target: **T0** for reconciling the advertised contract with
supported calls while preserving legacy callers. The concrete tier remains
unsettled until Phase 1 identifies the schema writer and runtime consumer;
removing supported calls or adding mandatory arguments is not preapproved.

## Phases

### Phase 1: Reproduce and reconcile the fork schema/runtime mismatch

Recover the exact schema, call and response from one pinned host/runtime pair.
Compare tool registration, validation and dispatch, including mixed-version or
cached-schema controls. Is this a stale host schema, adapter mapping drift or
runtime contract defect? Choose the correction from that evidence rather than
inventing a new fork interface. Add a contract check that a request admitted by
the advertised schema reaches the intended handler and that valid legacy calls
retain their behavior. Preserve fork authority/lineage boundaries. This is a
backlog capture, not a fork probe or implementation.
