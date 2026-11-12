# Plan: 0.49 Windows CI is green for wsagent, wsholder, and hosted MCP

Source: the 0.49.0 ship stopped at the CI gate. Run 43194048680 (workflow
`ws-mcp-ci.yml`) on `release/0.49.0` (0626d5f5e, cut from develop d56caca44)
is red on Windows only; ubuntu is green. Failed log (copy):
`/tmp/claude-1000/ws-ci049/ci-049-failed.log` (or
`gh run view 43194048680 --log-failed`). The peer lead fixes the other
packages (wsdashboard rail path separators and node argv length,
wswhiteboard rail_offsets, wsledger TestExclusive, wstrackerhost rename under
lock) on its own branch `impl/develop/windows-ci-049`; do not touch those.

## Outcome

Windows CI passes for this partition:

1. `internal/wsagent` (rest shard): about 40 lead tests fail with "timed out
   waiting for a process under a holder" (lead_command_test.go:86,
   lead_runner_test.go:63 TestLeadSpawnRunsUnderAHolder, lead
   handoff/http/recovery/takeover tests), plus lead_runner_test.go
   ~1703/1733/1777 "timed out waiting for ... on the retry's process".
   Find the cause from evidence, do not assume it. One suspect: the Windows
   npm cmd-shim unwrap (merge 0e455291b, `harnessCommand` /
   `cmdShimArgv` in `backend.go`) refusing or mis-resolving a test fake
   harness that is a `.cmd`/`.bat` or otherwise not an npm shim; another:
   the holder process/script itself on Windows. Check whether these tests
   passed on Windows before 0.49's range (`gh run list --workflow
   ws-mcp-ci.yml`, earlier runs) to localize the regression.
2. `wsagent` TestResumeCommand (lead_takeover_test.go:465): got
   `pi --session 'C:\Users\RUNNER~1\...\session.jsonl'` (quoted), want
   unquoted. Decide whether the code or the expectation is wrong for a
   Windows path (the resume command is shown to a user to paste into their
   shell); fix the wrong side.
3. `internal/wsholder` manager_test.go:691
   TestRefusedBreakawayStartsAHolderMarkedNotRestartSafe (holder
   RestartSafe:false, want restart-safe), and `internal/wsdashboard`
   holders_live_test.go:111 TestDaemonRestartReattachesToARunningHolder
   (holder listed, child running, assertion fails). Holder-side; yours.
4. `internal/mcp` live_hosted_test.go:127
   TestHostedLiveHandsSnapshotsOverInMemory: slot path `C:\Users\runneradmin\...`
   vs want `C:\Users\RUNNER~1\...` (8.3 short name vs long name; one side is
   EvalSymlinks'd). Fix by making both sides canonical the same way; prefer
   fixing the comparison/canonicalisation over special-casing the test.

## Settled constraints

- Fix real product defects in product code; fix test-only Windows
  assumptions in tests. Say which each fix is in the commit.
- Do not weaken a test to skip on Windows unless the behaviour is genuinely
  not applicable there; if you skip, justify it in the commit and report it.
- Compatibility tier T0. Stop and report if a fix needs a record, registry,
  snapshot, holder protocol, or MCP shape change.
- Files: `internal/wsagent`, `internal/wsholder`, `internal/mcp` (the
  hosted live slot path), and `internal/wsdashboard/holders_live_test.go`
  (plus whatever holder-side code it exercises). Stop and report before
  touching anything in the peer's list above.
- Heavy `go build`/`go test`/`go vet` only through
  `~/.local/bin/ws-rgo -C agents-plugin-tool -m <min> -- <go args>`
  (Linux). Cross-check compile with `GOOS=windows` where ws-rgo allows.
- Windows cannot run locally: verify by pushing the impl branch to origin
  (`git push origin impl/develop/windows-ci-hosted`, never force onto
  anything but this branch) and dispatching
  `gh workflow run ws-mcp-ci.yml --ref impl/develop/windows-ci-hosted`,
  then reading the run (`gh run watch` / `gh run view --log-failed`).
  Iterate until this partition's tests are green on Windows. Failures in
  the peer's packages are expected and not yours. Never push develop or
  main.

## Verification

- Linux: `go test` for the touched packages and `go vet` through ws-rgo
  stay green.
- Windows CI on `impl/develop/windows-ci-hosted`: every test listed in
  items 1-4 passes; report the run id and the remaining failures (all
  expected to be in the peer's packages).
- `git diff` reviewed against this plan before each commit; one commit
  per root cause; the plan rides the first commit.
