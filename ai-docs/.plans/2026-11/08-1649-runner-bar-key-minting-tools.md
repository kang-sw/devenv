# Plan: runner agents can never mint a lead key

Branch `delegate/runner-bar-key-minting-tools` (worktree
`/Users/kang-sw/.ws-worktrees/devenv/foe-breath-retying`), based on develop.
Commit here; do not merge or push.

Authority: user decision on the board, (457) a. Ticket
`ai-docs/tickets/idea/261030-bug-workflow-manual-sentinel-mints-lead-key-for-subagents.md`.
Read `ai-docs/manuals/shipped-surface-boundary.md` before editing under
`agents-plugin-tool/`.

## Context (lead-verified)

- A runner agent's ws-mcp is filtered by `WS_MCP_ALLOWED_TOOLS`, set from the
  agent definition's `ws_tools` (`internal/wsagent/runner.go` ~L649-666), and
  `internal/mcp/server.go` ~L685 refuses any call outside it. With empty
  `ws_tools` no ws-mcp is attached at all.
- The runner gives the agent a delegate key (`internal/mcp/agent_keys.go`).
- Two ws tools mint a parent-less lead key from no key: `ferrule`
  (`server.go` ~L2304, caller-chosen capability) and `workflow_manual` with
  the fresh sentinel (`workflow_manual.go` ~L375). A definition that lists
  either hands the agent a lead key.

## Fix

1. Agent definition validation (`internal/wsagent/profile.go`, the
   `ws_tools` loop ~L194) refuses `ferrule` and `workflow_manual` with an
   error naming why (they mint a lead session key). A refused definition is
   a clear failure for the profile author; prefer it over silently stripping.
   Keep the barred names in one place (a small set beside the validation) and
   comment the trap.
2. Tests: validation refuses each barred name; an ordinary list
   (`tickets.query`) still validates. If the config path that stores
   `agent.profile.<name>` validates through the same function, one test
   there too.
3. Append to the ticket a dated `## Status (2026-11-08)` note: runner-spawned
   agents are closed by (1) (commit hash); the remaining case is in-process
   subagents sharing the lead's ws-mcp, which needs a per-call caller signal
   (uninvestigated: claude-mod hook agent id, Pi spawn env); the ticket stays
   open for that.

## Verification

`cd agents-plugin-tool && gofmt -l . && go vet ./internal/wsagent/ && go test -count=1 ./internal/wsagent/`
(plus the config package test if step 2 touches it).

## Return

Commit hashes, changed files, test output, anything left open.
