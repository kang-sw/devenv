---
title: "Hosted-env test fixture defaults to agentless mode, hiding agent-backed tools"
---

# Hosted-env test fixture defaults to agentless mode

## Dogfood surprise (2026-11-10)

Reported by the 0.48.17 gate fix round 2 delegate (daemon-hosted proxy):
"hosted test fixtures default to agentless (envNoAgent set), which hides
agent-backed tools like exec.spawn; the test clears it explicitly."

## Evidence

- `agents-plugin-tool/internal/mcp/hosted_env_paths_test.go:56`
  `hostedServerEnv` seeds `envNoAgent` (`WS_MCP_NO_AGENT`) to `"1"` (`:62`)
  in its base map before merging the caller's `set`.
- A test that needs an agent-backed tool must know to pass `envNoAgent: ""`,
  as `hosted_env_paths_test.go:182` does. A test that forgets gets a server
  whose tool list silently lacks those tools, so a hosted-path regression in
  an agent-backed tool would be untested rather than failing.

## Directions (not decided)

- Default the fixture to agent mode and opt into agentless per test, or
- keep the default but make a call to a hidden tool fail loudly in the
  fixture (for example assert the expected tool set), or
- document the default at the helper with the reason it is agentless.

## Constraints

- Test-only change in `internal/mcp`; run Go tests via
  `~/.local/bin/ws-rgo`, never locally.
