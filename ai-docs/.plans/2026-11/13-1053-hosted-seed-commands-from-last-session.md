# Hosted session: seed the slash-command list from the last session before the first send

Owner 2026-11-13, whiteboard (31)=b.

## Problem

A dashboard-hosted lead created with no message has no harness process until
the first send (`CreateLead` -> `create` -> `startConv` starts nothing; the
process starts in `kick` -> `startProcess`). So an empty session has no
command list, and the composer's `/` autocomplete shows nothing. Since
3347970c3 (e0b14a3ad), a launch sends `initialize` and the answer becomes the
snapshot's `commands`, so the list arrives as soon as the process starts.

## Decision (owner (31)=b)

Do not start a process at creation. Seed a newly created hosted session's
command list from the most recent earlier hosted session of the same project
(session root / cwd) and the same agent profile (harness) that has a non-empty
command list. The first launch's `initialize` answer (or a turn's init)
replaces the seeded list as today. Rejected: eager process start at creation
(a process and its MCP servers per unused session; Pi never exits); keeping
the current behaviour.

## Scope

- Find where the hosted lead's snapshot `commands` are stored and served
  (wsagent snapshot / wslive / wsdashboard; `lead_command_test.go`,
  `TestClaudeLeadLaunchAsksForCommands`, `newLiveWriter` in record.go are
  starting points). Pick the simplest seed point: at session creation, copy
  the commands from the newest matching earlier session's persisted snapshot
  (or an in-memory last-known list keyed by project + profile, if the daemon
  already keeps one; persisted is preferred so it survives a daemon restart).
- Applies to hosted Claude and hosted Pi alike if both expose commands the
  same way; if Pi's list source differs, cover Claude and state what Pi does.
- The seeded list must be visibly the same shape the composer already reads;
  no composer/JS change should be needed. If one is, keep it minimal.
- Matching key: same project root and same profile/harness. No match -> no
  seed (today's behaviour).
- Compatibility tier: T0 (no store shape or protocol change). If seeding
  needs a new persisted field or file, stop and report before adding it.

## Out of scope

- Restarting the daemon or any session; creating live sessions (the lead does
  the live check).

## Verification

- Go tests on w5090 (clone note `remote-build-w5090`): a created session with
  a matching earlier session gets that session's commands before any send; no
  match -> none; a launch's `initialize` answer replaces the seed; different
  profile or project -> no seed.
- `go vet` and `go test ./internal/wsagent/... ./internal/wsdashboard/...
  ./internal/wslive/... -count=1` (plus any touched package) pass.
- State the root cause/approach in the commit's `## AI Context`; state what
  was not checked live.
