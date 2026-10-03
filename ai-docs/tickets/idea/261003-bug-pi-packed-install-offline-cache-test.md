---
title: "Pi packed-install test fails on an uncached offline dependency"
related:
  261002-feat-pi-model-output-tps-display: prior offline packed-install failure recorded in its Result
---

# Pi packed-install test fails on an uncached offline dependency

## Background

During the user-requested Pi 1.0.0 upgrade dogfood, `npm test` in
`agents-plugin-pi` reported 1941 tests: 1937 pass, 1 fail, 3 skipped.
The failure at `test/web-package.test.ts:17` was npm `ENOTCACHED` for
`undici-8.11.2.tgz` in the offline packed-install path. The host executable
was `/opt/homebrew/bin/pi`; the adapter's development Pi dependencies were
still 0.84.4. This observation alone does not establish a Pi 1.0 regression.

The Result of `261002-feat-pi-model-output-tps-display` already records an
unrelated offline `ENOTCACHED` failure in this same test. This idea captures
the recurring test-environment failure separately from that completed feature.
No implementation or offline-test policy has been selected.

## Phases

### Phase 1: Diagnose the packed-install test failure

Scope and remedy remain unsettled. Preserve the distinction between an
uncached dependency and a runtime compatibility regression when investigating.
