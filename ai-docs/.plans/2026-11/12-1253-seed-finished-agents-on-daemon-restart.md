# Plan: finished subagent rows survive a daemon restart

Ticket: `261112-bug-ws-hosted-finished-subagents-lost-on-idle-resume`
(idea/; its title and hypothesis are outdated, see the corrected cause below).

## Outcome

After the dashboard daemon restarts (for example, a dev rebuild replaces it),
a hosted session's finished subagent rows (`done` / `failed`) that were in its
`snapshot.json` are still shown, within the existing `FinishedAgentCap`.

## Corrected cause (lead-verified, 2026-11-12)

- An idle stop plus resume keeps the same per-session live state.
  `processEnded` marks running agents failed through `endAgents`, and
  `c.live` survives.
- A daemon restart loses the rows. `Runner.Start` calls `startConv`
  (`agents-plugin-tool/internal/wsagent/runner.go` about 300-320), which
  builds a fresh `newLiveWriter` and immediately calls `writeSnapshot()`. That
  overwrites the previous `snapshot.json`. Nothing reads `snapshot.json` back
  (`wslive.ReadSnapshot` is used only by dashboard readers).

## Settled constraints (user (11)/(13) a)

- In `startConv`, before the first `writeSnapshot`, read the previous
  `snapshot.json` (when present and parseable) and seed its finished agents
  into the new state. A small `wslive.State` method is acceptable for this.
- Seed only agents whose status is not running. Agents the old snapshot shows
  running are out of scope here. A separate ticket will notify the lead about
  agents a restart killed, and how running agents of a rejoined lead are
  restored belongs to that work. Do not seed them as running. Leave them out.
- A missing, unreadable, or foreign snapshot never blocks session start. It
  seeds nothing.
- `retain` and `FinishedAgentCap` still apply after seeding.
- Compatibility tier: T0. This reads an existing file, and the snapshot and
  record shapes are unchanged.

## Scope and files

`internal/wsagent/runner.go` (startConv) and/or `internal/wsagent/record.go`
(liveWriter), `internal/wslive/state.go` (seed method), and tests in
`internal/wslive/state_test.go` and `internal/wsagent/runner_test.go` or the
nearest existing test file. Stop and report if more is needed.

## Verification

- Unit test: a State seeded from a snapshot holding done, failed, and running
  agents keeps the done and failed rows and drops the running ones. The cap
  still applies.
- Runner-level test: a conv started over a session dir whose `snapshot.json`
  holds a finished agent writes a snapshot that still lists it.
- Run Go tests through `~/.local/bin/ws-rgo` only, never locally.
