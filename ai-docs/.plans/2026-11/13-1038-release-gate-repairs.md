# Release-gate repairs before the ws patch release

## Problem

The release-gate review of `f01fed037..3634ebdf6` returned NEEDS FIX.
Findings: `C:\Users\user\.cache\ws@kang-sw-devenv\proj\de04b4ac\review-paths\f1d0e1e3-01-findings.md`
("Must fix before release" items 1-4; the "minor follow-ups" list is out of
scope). develop has since gained 1b1d0a9de (tracker pencil edit row) and
1d274570f (subagent badge + hosted slash commands); this branch is off that
develop.

## Outcome

- Item 1: fix the JS quoting in `tracker_quickadd_test.go`, then run every
  node-backed dashboard test (the `runTrackerCases`-style tests and any other
  test that skips without node) with node present, and fix whatever else
  fails there, including in tests or assets from 1b1d0a9de / 1d274570f. A
  test failure that reveals a real asset bug: fix the asset minimally and say
  so; anything larger, stop and report.
- Item 2: resync `agents-plugin-pi/bin/ws-mcp-launcher.py` to the ws copy;
  add a Go test (CI runs Go) asserting the three launcher copies are
  byte-identical, if the repo has no such test yet.
- Item 3: update the two `ai-docs/manuals/ws-mcp.md` env-table rows.
- Item 4: a hosted.test.ts test that runs the decoded probe script under
  python on the current OS (mirror into agents-plugin-wsflow); run the
  claude-mod hosted tests and the launcher python tests locally and report
  pass/fail (pre-existing failures noted as such, not fixed).
- One commit per item or one commit total, worker's call; `## AI Context`
  cites the findings path.

## Verification

- Node exists only on this Windows machine (`C:/Program Files/nodejs`), as
  does a local Go (`C:/Program Files/Go/bin`). w5090 has Go but no node. So:
  `go test ./internal/wsdashboard/... -count=1` locally with node on PATH,
  wrapped in the local lock (`bash D:/sys_lock.sh run <who> <min> -- <cmd>`);
  confirm in the output that the node-backed tests ran (not skipped).
- Full `go test ./...` on w5090 (remote lock, clone note
  `remote-build-w5090`).
- Report what ran where, pass/fail, and what was not run.
