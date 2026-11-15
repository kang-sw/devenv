# approved-001 — FAILED at baseline compilation

## Frozen authority and inputs

MAIN 00:23:47 approved one fail-fast offline batch only. Integration merge:
`7589b5910fc50759fa8524f267b5e0209f491bad`.

- Baseline: `5585844a794a031a770ce4171c5e05daf1bd3353`.
- Result: `b0af755624cc13929f4a384145d5787d76e3dbdb`.
- Pre-start source/asset Git hashes and bounds: `../freeze.json`.
- Exact executed command:

```sh
python3 ai-docs/tickets/assets/261115-feat-ws-whiteboard-viewed-panel-anchor/browser-approved/run.py --result b0af755624cc13929f4a384145d5787d76e3dbdb --batch approved-001
```

## Full raw build failure

`baseline-build.txt` contains the complete compiler output:

```text
# github.com/kang-sw/devenv/internal/wsdashboard [github.com/kang-sw/devenv/internal/wsdashboard.test]
internal/wsdashboard/gitbadge.go:410:6: signal already declared through import of package signal ("os/signal")
	internal/wsdashboard/board_viewed_browser_test.go:9:2: other declaration of signal
```

**VERIFIED classification:** the common fixture's `os/signal` import name
collides with an existing package declaration. This is fixture compilation,
not an environment, baseline asset-layout, port, holder or browser/product
predicate failure. No fix or retry was made after failure.

`manifest.json` retains all exact commands, the offline environment, phase
return codes and SHA-256 bindings. Baseline compilation returned 1; the
supervisor returned 1 after 1.87 seconds including cleanup. No cap expired.

No result build, executable binary, fixture server or browser was reached.
Consequently no executable hash, HTTP request/response, geometry, screenshot
or product acceptance is available; none is implied by the source manifests.
There is no failing request URL/body because execution stopped before any HTTP.

## Retention and lifecycle

The exact common fixture, browser driver and runner are retained beside their
raw frozen Git reads. `baseline-sources.json` records every baseline build-input
file's SHA-256, including assets/templates and identical fixture overlay.
The immutable `baseline.tar`, `baseline-build-inputs.tar` and read-only
`baseline-source/` remain locally retained, not committed as duplicate source
or binary artifacts. Preserve this directory before releasing the worktree.
No executable was produced to retain.

`cleanup.json` independently records `final_ps: []`, no cleanup signals, and no
errors for the tracked PID/start-time identities. No shared endpoint, daemon,
provider, real holder or dependency installation was used. Prior `browser/`
evidence is unchanged.

No independent review, Phase Result, ticket closure, develop integration or
runtime deployment followed this failed batch. Further execution requires
MAIN disposition. MAIN's latest final integration advisory is develop
`1612e099c`, superseding the earlier `088ecf679` and `6db68ec00` gates; this
batch intentionally retained its approved 558 baseline instead of importing
those later changes.
