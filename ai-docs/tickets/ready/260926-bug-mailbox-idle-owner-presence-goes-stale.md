---
title: Idle mailbox owner's presence goes stale, and the Pi waiter omits --root
related:
  260926-bug-pi-execute-worker-steals-mailbox-owner: sibling — its PID-on-rebind fix keeps a reclaimed record heartbeat-eligible
  260926-bug-pi-children-inherit-mailbox-identity: sibling — same mailbox dogfood investigation
  260913-feat-cross-session-mailbox-core: source — presence heartbeat and liveness policy changed here
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 306738de11c30b09
sage-review-completeness-reviewed: 306738de11c30b09
---

# Idle mailbox owner's presence goes stale, and the Pi waiter omits --root

## Background

Two gaps surfaced while diagnosing
`260926-bug-pi-execute-worker-steals-mailbox-owner`:

1. **Presence staleness.** A named-inbox presence `LastSeen` refreshes only
   opportunistically, on mailbox tool calls (`refreshMailboxPresenceHeartbeat`).
   A blocking `mailbox wait` never refreshes it. Any idle lead, on every
   harness, therefore reads as dead to `lookup_peers` after the 10-minute
   liveness threshold, and another process may reclaim its name — the window
   the execute-worker used to take over the lead's record. The
   `mailboxPresenceLive` comment already names a background liveness
   mechanism as deferred.
2. **Pi waiter root.** Pi's `buildMailboxWaitArgv` passes a worktree/clone
   `--slug` but no `--root`, so the CLI's store resolution falls back to the
   launcher's PWD-based project-root detection. When Pi was launched from a
   directory other than the session's cwd, the wait checks the wrong store.

## Decisions

- **`ws-mcp serve` runs a background presence heartbeat for its active mailbox
  identity**, refreshing `LastSeen` while its PID holds the presence record.
  Liveness now means "the serving process is alive and ticking"; the
  10-minute threshold catches stopped or suspended processes (a goroutine
  ticker does not detect a wedged request loop). The tick interval stays well
  inside the liveness threshold (for example the existing 1-minute
  `mailboxHeartbeatThrottle`), and the ticker stops with the server. The
  existing opportunistic refresh on mailbox tool calls stays. A worktree/clone
  identity heartbeats against the root it registered with. The ticker follows
  whatever record the server currently holds by name and PID: after a rebind
  that keeps or rewrites the record PID to this process it keeps refreshing,
  and after a PID mismatch or conflict it skips, as the PID-gated
  `refreshMailboxPresenceHeartbeat` already does.
  - Rejected: `mailbox wait` refreshing `LastSeen` on each poll (owner-gated).
    It covers only sessions with an armed wait.
  - Rejected: the Pi extension periodically calling a mailbox tool. Pi-only.
- **The Pi waiter passes `--root <ctx.cwd>` to `mailbox wait`**, the same root
  the bridge's bootstrap `ferrule` uses.
  - Rejected: setting `WS_MCP_PROJECT_ROOT` in the wait subprocess env
    instead; `--root` is the CLI's declared input for a worktree/clone slug.

## Constraints

- Read before editing: `ai-docs/manuals/shipped-surface-boundary.md` (all
  `agents-plugin-tool/` output) and `ai-docs/manuals/ws-mcp.md`
  (`agents-plugin-tool/internal/mcp/`).
- Update the `mailboxPresenceLive` doc comment, which currently describes the
  background liveness mechanism as out of scope. Describe what the ticker
  actually detects: a stopped or suspended process, not a wedged request loop.
- This ticket can land independently of
  `260926-bug-pi-execute-worker-steals-mailbox-owner`; the heartbeat is
  PID-gated either way. Both touch `mailbox_runtime.go`: whichever lands
  second rebases onto the other, and if this ticket lands second it adds a
  test that the ticker keeps a reclaimed, PID-rewritten record fresh.
- A process that lost the name (PID mismatch or conflict) never heartbeats the
  record; the existing never-resurrect rule stands.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)

## Prior Decisions

- 260913-feat-cross-session-mailbox-core (2026-09-13, Result 18d50953): "a definitively-alive PID does NOT override recency to "alive" (idle-dropout stays recency-bound, deliberately deferred to `260913-feat-cross-ses..." — bearing: supports
- 6d786057 (2026-09-13, commit, 260913-feat-cross-session-mailbox-wake): "Chose to keep mailboxPresenceLive's dead-PID check asymmetric (dead PID overrides recency to dead; alive PID does not override recency to alive) rather than a full OS-liveness-based redesign" — bearing: constrains
- 6d786057 (2026-09-13, commit, 260913-feat-cross-session-mailbox-wake): "Root-threading (Critical #1) is the load-bearing fix: ... a real, session-bound root available at the mailbox call site ... rather than trying to make process-level cwd resolution reliable" — bearing: constrains
- 18d50953 (2026-09-13, commit): "Removed the mcp-package-local mailboxProcessDead entirely ...: wsstate already owns a tested cross-platform version" — bearing: constrains
- 260913-research-cross-session-mailbox (2026-09-13, Confirmed Decisions): "self-registration: the server self-registers presence from the env at startup, keyed by the mailbox name, with liveness (heartbeat/pidfile) and duplicate-live-name detection." — bearing: supports
- a8a181fc (2026-09-18, commit, 260917-feat-ws-pi-mailbox-waiter-slug-wake): "No --root threading for a worktree/clone-scoped slug: out of this ticket's stated scope ..., and the CLI's existing owner-gate degrade already keeps a wrong/unreachable slug safe by construction" — bearing: supports
- a8a181fc (2026-09-18, commit, 260917-feat-ws-pi-mailbox-waiter-slug-wake): "Extracted buildMailboxWaitArgv as a pure argv builder out of createSubprocessWait ... so the slug/no-slug argv shapes are unit-testable without spawning a real subprocess." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/mcp/mailbox_runtime.go, agents-plugin-tool/internal/mcp/server.go or agents-plugin-tool/cmd/ws-mcp/main.go serve lifecycle, agents-plugin-pi/src/mailbox-waiter.ts, agents-plugin-pi/src/index.ts arm site |
| scope.surface | cross-module | Go ws-mcp serve heartbeat plus Pi adapter waiter argv; exported TS buildMailboxWaitArgv and SubprocessWaitOptions change |
| scope.new_public_symbol | no | none; the Go ticker can stay unexported and the Pi change extends an existing exported builder |
| scope.new_type_contract | yes | SubprocessWaitOptions gains a root option in agents-plugin-pi/src/mailbox-waiter.ts; Go mailbox state must retain the registered root |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/mailbox_runtime_test.go, agents-plugin-tool/internal/mcp/mailbox_tools_test.go, agents-plugin-pi/test/mailbox-waiter.test.ts |
| complexity.reuse_points | confirmed | refreshMailboxPresenceHeartbeat PID-gated write and mailboxNow clock seam in mailbox_runtime.go; ServeStdio ctx.Done loop in server.go; CLI --root flag in cmd/ws-mcp/mailbox.go |
| complexity.side_effect_risk | moderate | a background goroutine periodically writes the shared mailbox store under its file lock for the server's lifetime |
| risk.correctness | moderate | ticker lifecycle, retaining the registered root for worktree/clone identities, and interaction with conflict and rebind paths must preserve never-resurrect |
| risk.fit | low | reuses the existing PID-gated heartbeat write and the CLI's already-declared --root input |
| risk.test | moderate | needs a new injectable ticker; timing-based mailbox tests have a Windows flake history per 260917-bug-exec-mcp-windows-test-timing-margin-flake |
| risk.security_or_contract | moderate | changes the liveness semantics that gate named-inbox reclaim by another process |

## Phases

### Phase 1: Keep an idle owner live and pass the waiter root

Implement both decisions.

Verification:

- Go tests in `agents-plugin-tool/internal/mcp/` with an injectable clock or
  ticker: an identity-holding server refreshes `LastSeen` with no tool calls;
  a worktree/clone-scoped identity refreshes `LastSeen` in the store under
  its registered root; a server whose record PID differs does not; the ticker
  stops on shutdown.
- Pi unit test on `buildMailboxWaitArgv`: a root option emits `--root <root>`.
  The index.ts arm site passes `ctx.cwd` as that root; if no existing test
  seam reaches the arm site, extract the `SubprocessWaitOptions` construction
  into a small pure helper and test that.
- Existing suites pass: `go test ./internal/mcp/ ./cmd/ws-mcp/` in
  `agents-plugin-tool/` and `npm test` in `agents-plugin-pi/`.
