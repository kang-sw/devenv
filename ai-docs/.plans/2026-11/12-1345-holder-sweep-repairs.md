# Plan: repair the review sweep's Important findings in the hosted holder runner

Source: peer review sweep `00fd2e5d9..62304312b` (ledger 940fc053e), routed to
this session. Findings files (read both first; they are the evidence):
`/home/swkang/.cache/ws@kang-sw-devenv/proj/dac18b1d/review-paths/769fe701-01-findings.md`
and `/home/swkang/.cache/ws@kang-sw-devenv/proj/dac18b1d/review-paths/d169a2be-03-sweep-agent-holder.md`.

## Outcome

Fix, with a regression test each:

- **P1** `agents-plugin-tool/internal/wsagent/runner.go:239-268` `holdersScanned`
  (from 72c9d851e): a daemon close during the first holder scan must not stop
  live hosted lead holders. Conv loops that end on quit never set `owned[id]`,
  and the conv snapshot is taken before `List()`. Fix as routed: skip the
  orphan sweep once `r.quit` is closed, and call `List()` before taking the
  conv snapshot.
- **P2** `runner.go:1189-1192` `rowForeign` (from 6a991d903): registry rows
  rewritten by a pre-range (<= 0.48.16) daemon read as foreign, and
  `release()` then kills the running holder. Fix as routed: guard with
  `!row.Agent` (verify against the finding that this is the right
  discriminator; if the finding's evidence says otherwise, follow the
  evidence and say so in the commit).
- **P5** `runner.go:1063-1066` and `1559-1561` `checkReattach` trusts
  `Manager.List()` before the first scan fills it, so a second `--resume` can
  start beside the live one. Make reattach decisions wait for (or refuse
  before) the first completed holder scan.

Investigate only, no code, and report a proposed fix with evidence for:

- **P3** Windows `.cmd` shim: the holder child PID is `cmd.exe`, so `HostPID`
  mismatches and the hosted session is refused its key (`pi.go` `startLead`,
  `claude.go:313`, `runner.go:1498`, `agents-plugin-pi` `hosted-session.ts:95-105`).
- **P4** `holder.py` `RING_BYTES` 4 MiB with no `RingBytes` set by the runner:
  a long stream-json session wraps and a rejoin loses turn state. Note that
  the hosted context meter now sends one ~29 KB `get_context_usage` answer per
  model per session (claude.go `askContextUsage`).

## Constraints

- Compatibility tier T0: no record, registry, snapshot, or MCP shape change.
  Stop and report if a fix needs one.
- Stay within `internal/wsagent` and `internal/wsholder` (plus tests). Do not
  touch the minors the peer recorded as ledger follow-ups.
- Heavy `go build`/`go test`/`go vet` only through
  `~/.local/bin/ws-rgo -C agents-plugin-tool -m <min> -- <go args>`.

## Verification

- A failing-then-passing test per fix (P1 quit during first scan leaves live
  holders running; P2 a pre-range row is not foreign; P5 no second start before
  the first scan).
- `go test ./internal/wsagent/... ./internal/wsholder/...` and `go vet`
  through ws-rgo; `git diff` reviewed against this plan.
- One commit per fix, or one commit naming P1/P2/P5, on
  `impl/develop/holder-sweep-repairs`; the plan file rides the first commit.
