# Plan: export the mailbox legacy files before releasing the dashboard lock

## Goal

Close a narrow mail-loss race found by the track/agent-v2 compatibility audit.
On shutdown the memory-first mailbox store exports its state to the legacy
files (`mailbox.json` per scope, `mailbox-replyids.json`, `mailbox-history/`)
so an older 0.50.x daemon restarted over the same state dir serves the mail.
Today `wsdashboard` `Daemon.Close` releases the dashboard lock
(`internal/wsdashboard/daemon.go` around line 738) before `close(d.done)`, and
`ClosePersistent` with its legacy export runs only after `daemon.Done()`
(`cmd/ws-mcp/dashboard.go` around lines 169-178). An old-version successor can
take the lock and write mail into a legacy file while the export is still
running; the export then assigns `*current = want` (`internal/wsmailbox/legacy.go`
around lines 488-492) and overwrites that mail, which is lost.

## Settled constraints

- Fix by ordering: the legacy export completes before the dashboard lock is
  released, so no successor can start serving during it. Do not switch the
  export to merge semantics.
- Keep the new-to-new successor path (`persistentLockWait`, `wsmailbox/ring.go`
  around line 44) working.
- Behavior otherwise unchanged; no new config, no schema or record change
  (compatibility tier stays T2 for the mailbox store, T0 for this change).
- Minimal change: move or hook the close call; no new abstraction layer.

## Verification

- A focused Go test that shows the export finishes before the lock is
  released (or that a successor acquiring the lock observes completed legacy
  files).
- One passing full `go test ./...` of `agents-plugin-tool` on remote w5090
  (never local go test on this PC). Use `bash D:/sys_lock.sh remote w5090 run
  <who> <min> --sync '<this worktree>' -- 'export PATH=$HOME/sdk/go/bin:$PATH;
  cd agents-plugin-tool && go test ./... -count=1'`. The two
  `TestTopicStateBadge*` tests fail on w5090 because Node is missing (baseline
  on develop); exclude them and record that.

## Out of scope

Merging, pushing, version bumps, any other compatibility edge from the audit.
