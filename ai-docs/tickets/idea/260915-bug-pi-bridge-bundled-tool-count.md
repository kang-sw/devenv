---
title: "Align the Pi bridge test with the bundled tool contract"
related:
  260915-bug-ws-pi-widget-context-value-removed: full-suite verification exposed the unrelated stale contract assertion
---

# Align the Pi bridge test with the bundled tool contract

## Background

The full `agents-plugin-pi` test suite fails at `test/bridge.test.ts:110`: `BUNDLED_RUNTIME.tools` contains 56 tools while the assertion hard-codes 54. The context-token implementation did not change `bridge.ts`, `runtime.json`, or this test, so the failure is pre-existing/unrelated to that correction. The assertion already compares the live captured set to the bundled names; investigate why the cardinality has drifted and update the intended contract or remove redundant fixed-count coupling with appropriate coverage.

## Phases

### Phase 1: Reconcile the bridge's bundled-tool cardinality assertion

Establish the intended bundled tool inventory and align the bridge regression contract without weakening its live-versus-bundled exact-set guarantee. Verify the full Pi adapter suite.
