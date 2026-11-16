# CI: two Windows-only failures in the `windows-latest, rest` shard

The CI run on the review-repaired develop head failed two tests on `windows-latest` only (ubuntu passed them):

```
--- FAIL: TestGitMetadataLinkedPointersRefsIndexAndWorktrees (0.01s)
    git_metadata_test.go:52: unchanged metadata is nondeterministic
FAIL	internal/wsdashboard

--- FAIL: TestLiteralCodeNearLinearGrowth/tilde-per-log (0.07s)
    literal_performance_test.go:163: tilde-per-log 50014 bytes best-of-3=314.8µs
    literal_performance_test.go:163: tilde-per-log 100020 bytes best-of-3=4.9486ms
    literal_performance_test.go:163: tilde-per-log 200032 bytes best-of-3=11.5326ms
    literal_performance_test.go:175: fourfold-size growth: [314.8µs 4.9486ms 11.5326ms]
FAIL	internal/wsxmlcmd
```

## 1. `gitMetadataStamp` is not stable on Windows (product bug until proven otherwise)

`internal/wsdashboard/git_metadata_test.go` computes `gitMetadataStamp(linked)` eleven times on unchanged files and gets a different stamp on Windows. The latest change here is `5395c0410 fix(dashboard): stabilize Windows git metadata hints`; read it and its `## AI Context`.

- Find what in the stamp varies between calls with no file change on Windows: candidates are directory mtimes or sizes that Windows updates on read or enumeration, last-access time, map/directory iteration order, a stat of a path that the stamp call itself creates or touches, or `os.Stat` vs `os.Lstat` differences on the `.git` file pointer. Reason from the code and from Windows filesystem semantics; you cannot run on Windows (do not ssh). If you can reproduce a cause on macOS, do.
- Fix the product code so the stamp depends only on content-relevant metadata that does not change on read, while still changing when the files the test later edits change (the second half of the test). Do not weaken the test's determinism assertion. If the real fix needs a product decision, stop and report.
- The stamp feeds the dashboard's git metadata change detection; a nondeterministic stamp means repeated rail refreshes/reads on Windows, so this is a real bug, not only a test issue.

## 2. `TestLiteralCodeNearLinearGrowth` is timing-flaky on shared runners

The parser is linear; the failure is noise: the 50 KB best-of-3 is 0.3 ms, so a 5 ms scheduler/GC pause at the larger sizes trips the ratio check. Make the test robust without hiding real superlinear growth:

- e.g. more trials (best-of-N), `runtime.GC()` before each timed trial, and/or measure several iterations per trial so each sample is well above timer and scheduler noise, and compare ratios on those. Keep the absolute 200 KB bound.
- A real quadratic regression must still fail: show this by temporarily making the parse quadratic locally (do not commit that) or by an argument in the commit's `## AI Context`.

## Verification

- `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test ./internal/wsdashboard -run 'TestGitMetadata' -count=20` and `go test ./internal/wsxmlcmd -run 'TestLiteralCode' -count=10`; `GOOS=windows go vet` on both packages.
- The lead will run both on a real Windows host after your commits.

## Common rules

- The module is `go 1.23`: no newer standard-library APIs (for example `t.Chdir`).
- Never run the whole `./internal/wsdashboard` or `./internal/mcp` package; use narrow `-run`.
- Do not ssh, do not launch GUI, do not restart the shared daemon, do not run `scripts/dev.py rerun`.
- Public repository: no machine-local paths, hostnames, PIDs, or session keys in commits.
- One commit per fix, `## AI Context` in each. Commit on your branch only; do not merge.
- Return: commits, the root cause found for (1) with evidence, tests run with results, anything left unfixed.
