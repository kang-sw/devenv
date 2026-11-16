---
title: "Release bumps stale the native Pi payload and its producer is locale-sensitive"
related:
  261114-bug-ws-installed-plugin-missing-native-pi-payload: "closed packaging prerequisite; Forward in 44a8688cd requires this release-flow follow-up"
---

# Release bumps stale the native Pi payload and its producer is locale-sensitive

## Background

Dogfood findings #9/#11, 2026-11-16. The native payload's first-party
fingerprint includes `agents-plugin-pi/package.json`'s version, so every release
bump invalidates the committed payload even without a functional source change.
The v0.50.1 recovery refreshed it manually (`d81b2eae4`); the Forward in
`44a8688cd` and the closed prerequisite's Edition and Resolution explicitly
leave release-flow integration open.

Today's producer run also required `PYTHONUTF8=1` on Windows under CP949 to read
a dependency manifest. The checkout lacked esbuild until a locked dependency
install. These are owner-reported producer failures; the exact manifest path,
exception and producer command were not supplied. Do not infer that earlier
closure-encoding fixes already cover this read path.

## Reproduction

1. In a disposable checkout with a fresh prepared payload, perform only the
   normal release package-version bump, then run the source-freshness check.
   Observe the stale first-party fingerprint.
2. Run the producer under Windows CP949 without `PYTHONUTF8=1`, retaining the
   failing dependency-manifest read. Compare with explicit UTF-8 mode.
3. Compare an unprepared checkout with a locked dependency install; retain the
   missing-esbuild diagnostic and exact prerequisites.

These are future reproduction steps, not commands run by this docs capture.

## Candidate fix directions

- Regenerate/verify the payload as part of the release bump flow, with explicit
  producer dependency preparation; or exclude only the version from the
  first-party fingerprint while preserving meaningful freshness checks.
- Make all text-manifest reads explicitly UTF-8 rather than depending on the
  host locale or an undocumented environment override.
- Report missing producer tooling clearly; do not make downstream launch or
  update install npm dependencies.

## Decisions

Compatibility tier: **n/a** for producer/release orchestration and encoding
fixes that leave carried manifests, runtime surfaces and installed readers
unchanged. If fingerprint/manifest semantics change across versions, establish
that reader/writer trace and a concrete T0–T3 tier before readiness; do not
assume a version-independent fingerprint is already approved.

## Open questions

- Should release bumps regenerate payloads, or should version-only changes be
  omitted from the fingerprint?
- Where can locked producer preparation run while preserving the existing
  Node-less release validation lane?
- Which dependency-manifest read failed under CP949, and are other reads equally
  locale-dependent?

## Phases

### Phase 1: Make release payload freshness and producer prerequisites reproducible

Recover the exact encoding failure, settle the release/fingerprint alternative,
and implement the bounded release-flow and explicit UTF-8 correction. Follow
packaging/mirroring and shipped-surface manuals for affected package paths.
Verify version-only bumps, real source/dependency changes, CP949 versus UTF-8,
missing esbuild, and package-only ws/wsflow freshness without a downstream npm
requirement. Record exact before/after checks during authorized implementation.
No builds, tests or payload generation are performed by this capture.
