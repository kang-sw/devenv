# Fix: wsagent auto-close test red after the read-cache-b merge

## Context

develop `34e03ae24` merges three branches:
- conv-auto-close (`eed79fc4c`): finished Pi children close after their report
  is delivered.
- read-cache-b (`3c589250e`): its item 3b (`253ea7b41`) changes how runner state
  is read on hosted lead pages. It touches `wsagent/runner_conv.go`,
  `permission_store.go`, `runner.go` and `wsinbox/inbox.go`: inbox entries are
  read once, and `permission.json` is reread only on a stat change.
- read-cache-a (`195d21cab`).

On w5090, the tree with read-cache-a+b merged fails one test. Log:
`D:/Temp/merged-vet/out3.txt`.

```
--- FAIL: TestFinishedPiWorkerClosesAfterItsReportAndSendReopensIt (0.03s)
    auto_close_test.go:75: the closed worker's process still runs
FAIL	github.com/kang-sw/devenv/internal/wsagent
```

The same package passed on the tree with conv-auto-close and inbox-readdir but
without read-cache-b (`D:/Temp/merged-vet/out.txt`). So the suspect is an
interaction between auto-close and the read-cache-b runner/inbox caches, for
example close reading stale cached state. It may also be a timing flake.

## Outcome

1. Decide whether this is a real regression or a flake.
   - Rerun the single test with `-count=20` on w5090, both on develop and on
     `eed79fc4c`.
2. If it is real, find the root cause and fix it in the production code. Keep
   both features' intent:
   - finished Pi children are closed and their process is stopped;
   - unchanged runner Gets do zero file reads.
   - Do not weaken the test.
3. If it is a flake, make the test deterministic without hiding a real failure,
   for example by waiting on the real stop signal instead of a fixed check, and
   explain why.

## Constraints

- No local `go test`, Go test executables, ws/ws-mcp exes, the launcher or the
  daemon on this PC (antivirus rule). Go runs only on w5090:
  `D:/sys_lock.sh remote w5090 run auto-close-fix 25 --sync 'D:/Workspace/Repos/.ws-worktrees/devenv/flyaway-fifteen-judgingly' -- 'export PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && ...'`
  - Redirect output to a file under `D:/Temp/auto-close-fix/`. Do not pipe to
    tail, because that loses the exit code.
- Final check: `go vet ./... && go test -count=1 ./internal/wsagent ./internal/wsinbox ./internal/mcp`.
- Tier: state it in the commit (expected T0).
- Simplest complete change. Commit on `impl/develop/auto-close-runner-cache-fix`
  with your own model's Co-Authored-By trailer. Do not push.
- Temp files only under `D:/Temp/auto-close-fix/`.

## Report

- real or flake, with the evidence;
- the root cause;
- the commit and files;
- the w5090 result.
