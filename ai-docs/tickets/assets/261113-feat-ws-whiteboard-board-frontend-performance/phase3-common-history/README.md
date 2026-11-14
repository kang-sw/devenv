# Common-history build/test evidence

No source edits or commits. Original LOOK failure remains open.

## Identities
- baseline: b84a0de4879168f4c4c0b1bfbd74ceeaad7d7bb7
  - Git module tree: 7fa7f7360ae4fd779340141fba839f8e59190d25
  - Export file inventory SHA256: 1afba1686abc7e4573b5d7e6c740373335b8fa86a1846ca3c5ae257bd8be2f00
  - Archive SHA256: 57d1f3295f774c5a41a444f0b40188f2b0ae99c24f7cd65abc5344deeb1fb573
  - stream.js SHA256: 5a7e86ab9dc8624e2296335d113d69dcd349aaa65c0e0cbb8765aa806634555a
  - Binary SHA256: c1c1d27cd096cc1de64bb2468f7823243d32b14d73ceff57d392edf07710e9d4
- candidate: 2b3e54f9ae745bc77f3fdd9840136306593e37ad
  - Git module tree: 441841742d7f7db617e596fba78ff92e68e0f717
  - Export file inventory SHA256: cf553e7ada05290945a3c9bfa7f6caf61ca9887128ae034fd4cf2bc7b593bd31
  - Archive SHA256: b66f66e77da81214c7361b418fceaf4ec719a99c243c9e49ff8d763aacd7427f
  - stream.js SHA256: 5a7e86ab9dc8624e2296335d113d69dcd349aaa65c0e0cbb8765aa806634555a
  - Binary SHA256: 43c4befb1a871fbc51cd64ee092515cbc7e85c959f58dc79bd6ab16174e17dc2

## Commands

All commands use the sanitized environment recorded in manifest.json. The inherited Go module cache is read-only dependency input; no dependencies are installed.

### baseline-build
Working directory: `/tmp/ws-board-topics-common-history.EWq6tq/exports/baseline/agents-plugin-tool`
Command: `go test -c -count=1 -o /tmp/ws-board-topics-common-history.EWq6tq/bin/baseline.test ./internal/mcp`
Exit: 0; elapsed: 10.193 seconds.
Full output: `logs/baseline-build.log`
```text

```

### candidate-build
Working directory: `/tmp/ws-board-topics-common-history.EWq6tq/exports/candidate/agents-plugin-tool`
Command: `go test -c -count=1 -o /tmp/ws-board-topics-common-history.EWq6tq/bin/candidate.test ./internal/mcp`
Exit: 0; elapsed: 9.835 seconds.
Full output: `logs/candidate-build.log`
```text

```

### baseline-suites
Working directory: `/tmp/ws-board-topics-common-history.EWq6tq/exports/baseline/agents-plugin-tool`
Command: `go test -timeout 20m ./internal/wsdashboard ./internal/wswhiteboard -count=1`
Exit: 0; elapsed: 141.559 seconds.
Full output: `logs/baseline-suites.log`
```text
ok  	github.com/kang-sw/devenv/internal/wsdashboard	139.552s
ok  	github.com/kang-sw/devenv/internal/wswhiteboard	22.282s

```

### candidate-suites
Working directory: `/tmp/ws-board-topics-common-history.EWq6tq/exports/candidate/agents-plugin-tool`
Command: `go test -timeout 20m ./internal/wsdashboard ./internal/wswhiteboard -count=1`
Exit: 0; elapsed: 144.919 seconds.
Full output: `logs/candidate-suites.log`
```text
ok  	github.com/kang-sw/devenv/internal/wsdashboard	142.402s
ok  	github.com/kang-sw/devenv/internal/wswhiteboard	19.218s

```

### candidate-mcp-subset
Working directory: `/tmp/ws-board-topics-common-history.EWq6tq/exports/candidate/agents-plugin-tool`
Command: `go test -timeout 20m ./internal/mcp -run 'Dashboard|Whiteboard|Board|Panel|Theme|Closed|Topic|Wiki' -count=1`
Exit: 0; elapsed: 67.312 seconds.
Full output: `logs/candidate-mcp-subset.log`
```text
ok  	github.com/kang-sw/devenv/internal/mcp	66.566s

```

### candidate-focused
Working directory: `/tmp/ws-board-topics-common-history.EWq6tq/exports/candidate/agents-plugin-tool`
Command: `go test -timeout 20m ./internal/wsdashboard -run 'TestSplitBoardTopics|TestBoardTopics|TestPageStreamsRideWebSocket' -count=1`
Exit: 0; elapsed: 11.475 seconds.
Full output: `logs/candidate-focused.log`
```text
ok  	github.com/kang-sw/devenv/internal/wsdashboard	10.890s

```
