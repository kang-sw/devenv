---
title: "ws MCP pi.* recursion: Pi workers spawn Pi children through the daemon as router"
parent: 261112-epic-ws-mcp-pi-subagents
related:
  261112-feat-ws-mcp-pi-spawn-claude-lead-path: prerequisite
---

# ws MCP pi.* recursion (child 1b)

## Background

Child 1 (`261112-feat-ws-mcp-pi-spawn-claude-lead-path`) gives a Claude lead
the `pi.*` tools on the daemon's `wsagent.Runner`, with a parent record on
the spawn spec and a mailbox completion report. Its `pi-worker` profile has
no `pi.*`, so a Pi worker can run only leaf playbooks. The user requires Pi
workers to run spawning playbooks too (the elevated ticket worker spawns
reviewers and implementers), so a Pi worker needs `pi.*`.

Facts gathered 2026-11-12 (paths under `agents-plugin-tool/internal/`):

- A Pi child's ws tools come from an attached stdio ws-mcp the runner
  registers through the Pi MCP bridge (`wsagent/pi.go` `piMCPServers`,
  `writePiBridge`), started with `WS_MCP_ALLOWED_TOOLS` and
  `WS_MCP_NO_DASHBOARD` (`wsagent/runner.go`). That server has no daemon
  handle, so child 1's `pi.*` refuses there (child 1 Decision 3).
- Every Pi child is already the daemon's own process; the daemon persists
  its record and resumes it.

## Decisions

1. **Recursion is in scope.** The `pi-worker` profile gains `pi.*`, subject
   to the depth cap (Decision 4). Rejected: leaf-only Pi workers (the user
   requires spawning playbooks on Pi).
2. **Daemon as router.** A Pi child's attached ws-mcp stays direct, but its
   `pi.*` calls forward to the daemon's runner over the daemon's local HTTP
   API, authenticated by a per-child capability token the runner puts in
   that child's env. The daemon records the calling child as the new child's
   parent from the token, so the daemon owns the whole tree. Rejected: the
   Pi extension path (children run with extensions off, and an extension
   subagent is spawned outside the daemon's tree); moving the child's whole
   ws-mcp onto the daemon `/mcp` proxy (it changes the attached server's
   mailbox identity and dashboard registration for one tool family).
3. **Completion to a Pi parent is steered.** When a child's parent is a Pi
   conversation, the runner delivers the child's report (child id, status,
   final assistant message) into the parent conversation as a steer while
   the parent's turn runs, and as a new prompt when the parent is idle. A
   Claude lead parent still gets child 1's mailbox mail. Rejected: queueing
   the report until the parent's turn ends (the user prefers steer, as a
   native subagent completion arrives mid-turn); a mailbox for Pi children
   (a Pi child has no mailbox push, and the daemon already holds the parent
   process).
4. **Depth cap 2.** Lead -> Pi worker -> Pi grandchild; a `pi.spawn` from a
   depth-2 child refuses with a message naming the cap. Rejected: 3 levels;
   a knob; no cap (cost runaway).
5. **Epic Decision 8 is amended.** Pi-harness lead sessions (lead-pi, with
   their own extension) still do not see `pi.*`; runner-spawned Pi workers
   do. This ticket edits the epic's Decision 8 text to say so.

## Constraints

- Lands after child 1; builds on its spawn spec parent field and report
  path.
- Settle how `pi-worker`'s explicit `WS_MCP_ALLOWED_TOOLS` listing of
  `pi.*` meets child 1's exposure gates (`dashboard.pi_spawn_enabled`, Pi
  version, daemon handle): the attached ws-mcp has no daemon handle and may
  read the knob from a different manifest set (design review of child 1,
  2026-11-12).
- Read `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/ws-mcp.md`, `ai-docs/manuals/release-compatibility.md`,
  and `ai-docs/manuals/wsflow-mirroring.md` before editing.

## Phases

### Phase 1: Pi workers spawn through the daemon

- Runner: per-child capability token in the child's env; HTTP endpoint the
  attached ws-mcp's `pi.*` forwards to; parent recorded from the token;
  depth computed from the parent chain and capped.
- Attached ws-mcp: `pi.*` listed for `pi-worker` and forwarded to the
  daemon; refused with a clear message when the token or daemon is absent.
- Report delivery to a Pi parent: steer while its turn runs, prompt when
  idle.
- Epic Decision 8 text amended.

Verification:
- Go tests: token-authenticated spawn records the child as parent; a forged
  or missing token refuses; depth-2 spawn refuses; report to a Pi parent is
  steered mid-turn and prompted when idle; a Claude parent still gets mail.
- A real run: a Claude lead spawns a Pi worker that spawns a Pi grandchild;
  the grandchild's report reaches the worker, the worker's reaches the lead.
