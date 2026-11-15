# approved-002 — FAILED during baseline browser startup

## Scope and frozen inputs

MAIN01:01:56 authorized only the `os/signal` alias and required references,
minimal local compile/vet gate wiring, and one browser batch after a green gate.
Baseline remains `5585844a794a031a770ce4171c5e05daf1bd3353`; result is
`a57290575bf57b506e05f6b8180c67cf56c87f65`. The driver/geometry/assertions/timing
are unchanged from b0af7556 (SHA-256
`c109f426e58e7e97a65aa9ce84e9bff6c34f8f92f41bd87cb9645b713aed92e9`).
Prior approved-001, b0/daf checkpoints and all ten original untracked artifacts
remain untouched.

## VERIFIED green compile-only/vet gate

`../preflight-002/manifest.json` records the exact Go 1.26.2 executable resolved
from the retained compiler receipt, not a new version probe:
`/opt/homebrew/Cellar/go/1.26.2/libexec/bin/go`.

In BOTH baseline/result source snapshots, the actual commands were:

```sh
/opt/homebrew/Cellar/go/1.26.2/libexec/bin/go test -c ./internal/wsdashboard -o <absolute retained side.test>
/opt/homebrew/Cellar/go/1.26.2/libexec/bin/go vet ./internal/wsdashboard
```

All four commands exited 0 with empty complete outputs. The exact expanded
binary paths, working directories and offline environment are in that manifest.
No server or browser launched during preflight. Elapsed 19.115 seconds including
cleanup; independent final process list empty, no signals/errors.

Retained preflight binaries and SHA-256:

- `../preflight-002/baseline.test`:
  `199c9da99de71aa40cbdbfa519dcee8f389c6dd4019a288b6380fbb8a4ccda80`.
- `../preflight-002/result.test`:
  `558551093e7a8f6e94ce6580cc04bd7ef7b15bcb8dcbf74936fc1745897b38a6`.

## Failed single browser batch

`../freeze-002.json` was reported before start. The exact executed invocation:

```sh
python3 ai-docs/tickets/assets/261115-feat-ws-whiteboard-viewed-panel-anchor/browser-approved/run.py --result a57290575bf57b506e05f6b8180c67cf56c87f65 --batch approved-002 --preflight-receipt /Users/kang-sw/.ws-worktrees/devenv/symptom-sandbox-swan/ai-docs/tickets/assets/261115-feat-ws-whiteboard-viewed-panel-anchor/browser-approved/preflight-002/manifest.json
```

Both batch builds exited 0. Both fixture servers reached ready and returned 0
on cleanup. The browser exited 1 during `baseline-1800-queued` startup, before
any behavioral check or baseline/result geometry comparison (`checks: []`).
The complete raw stack is in `browser.txt`; its error is:

```text
Error: page script: Failed to read the 'serviceWorker' property from 'Navigator': Service worker is disabled because the context is sandboxed and lacks the 'allow-same-origin' flag.
```

**VERIFIED:** normal shell and content-frame documents returned HTTP 200, with
baseline asset identity `f5d4d7a241df28e6`. `results.json` and
`document-ledger.json` retain URLs, frame identities, headers and body prefixes:
`http://127.0.0.1:55892/s/livesession` and
`http://127.0.0.1:55892/s/livesession?frame=1`. A diagnostic `failure.png` is
retained. The service-worker exception is not an HTTP failure; no request is
proven to have caused it. Underlying script attribution remains UNKNOWN, and no
product-contract defect or acceptance pass is established. No repair or retry
was made; no review or closure followed.

Batch elapsed 12.519 seconds including cleanup. No cap expired. Both outer
`cleanup.json` and `browser-cleanup.json` independently record empty final
process lists, no cleanup signals, and no errors.

## Unexpected provider-process side effect

**VERIFIED:** outer `cleanup.json` tracked PID 58444,
`/Users/kang-sw/.local/bin/codex app-server`, parent PID 58001 (the baseline
fixture). This executable was not requested by the worker or driver and violates
the intended no-provider-process isolation. Do not claim that boundary held.
Its source attribution and any external provider/network activity are UNKNOWN;
no new probe or runtime action was performed to investigate. The independent
final process list confirms that this tracked process was absent after cleanup.
This side effect is an explicit unresolved handoff item.

## Retained batch binaries and artifacts

- `baseline.test` SHA-256:
  `e1930de19221124325303fbfd41a273ca250ebad688ef75d39726e3ddf9a775c`.
- `result.test` SHA-256:
  `e4465211aae704c662f262364806a656ce7a6c873938856d9c2db7c0a13f7571`.

Both binary pairs, complete source/asset manifests, immutable source/build-input
archives and dirty local artifact trees are retained. Do not release/delete the
worktree before preserving them. No download, dependency install, source repin,
self-merge or shared-daemon action occurred. Further execution requires MAIN
disposition; the final lead-owned integration gate remains develop1612e099c.
