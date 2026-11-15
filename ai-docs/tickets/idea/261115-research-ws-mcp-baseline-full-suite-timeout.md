---
title: Investigate the pinned baseline MCP full-suite timeout
related:
  261115-feat-ws-whiteboard-committed-effects-in-conversation: comparison context only; this baseline excludes that candidate
---

# Investigate the pinned baseline MCP full-suite timeout

## Background

MAIN requested full baseline/candidate MCP JSON timing and an independent candidate browser check for the committed-effects work. The unchanged baseline itself reached its ten-minute test alarm. Capture the suite-cost or lifecycle investigation separately from the feature; neither a cause nor a repair mechanism is established.

## Outcome Ledger

### Verified Findings

- Baseline `d0ad8a9f1863e15b2bcc2d5f2d22eb062c920c14` was a clean isolated full checkout. Its command was `go test -p=1 -parallel=1 ./internal/mcp -count=1 -timeout=10m -json` from `agents-plugin-tool/`, using Go 1.26.2, an explicit sanitized environment, existing dependency/build caches, `GOPROXY=off`, `GOSUMDB=off`, `GOTOOLCHAIN=local`, `GOMAXPROCS=2`, and fresh per-command ws cache/config roots.
- The run began on 2026-11-14 at 22:51:32.711645 UTC and ended at 23:01:35.837059 UTC with exit 1. Monotonic command wall time was 603.126657542 seconds; the package terminal event reported failure with Elapsed 600.918 seconds.
- JSON parsing reported no invalid lines or build failures. It contained 1,611 completed pass events, zero completed fail events and three skips, including nested tests. `TestProvisionWorktreeDefinitionChangeReadopts` had a run event without a terminal event. That is censored execution, not proof that this test caused the timeout or independently hung.
- The stage recorded no surviving processes in its test process group. That is not a host-wide inactivity claim.
- The parent read the completed run metadata. Raw JSON, separate stderr, metadata, per-test summary and process snapshots are retained under the session's external artifact directory `ws-777-json-timing.pEidiy`; the parent session `useable-curvature-immovably` records their exact location. These are local evidence artifacts, not shipped dependencies.
- The full candidate comparison was omitted under the previously accepted cumulative budget rule to preserve the independent browser and final cleanup reserves. This run yields no completed full candidate-versus-baseline timing comparison.

### Confirmed Decisions

MAIN's comparison assignment requires a slow-baseline finding to be captured separately, rather than treating it as automatic feature acceptance. No test exclusion, retry, timeout increase, dependency download or product repair was authorized by this capture.

### Proposals

None selected. Inspect complete timing and timeout stacks, fixture/child lifetimes and environment constraints before choosing between suite-cost and lifecycle hypotheses.

### Open Questions

- Which completed top-level tests dominate baseline cost, without double-counting descendants?
- Did the ten-minute alarm reflect aggregate suite cost, an unfinished fixture/child lifecycle, or another condition?
- Which bounded, isolated follow-up would distinguish those hypotheses without shared-daemon or network access?

### Rejected Alternatives

No causal or implementation alternative is settled. Baseline timeout alone does not establish that the committed-effects candidate is independent of the earlier candidate timeout or that its slowdown is acceptable.
