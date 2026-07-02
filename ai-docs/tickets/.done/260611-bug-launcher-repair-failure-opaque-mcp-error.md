---
title: ws-mcp launcher repair failure surfaces only as opaque MCP -32000
related:
  260609-refactor-ws-spawn-runtime-deletion-session-auth: surfaced while dogfooding the unmerged M3 2c build on the Claude plugin install
  260525-bug-local-runtime-contract-marker: the local-devenv repair contract whose failure mode this concerns
completed: 2026-06-20
---

# ws-mcp launcher repair failure surfaces only as opaque MCP -32000

## Background

While dogfooding the unmerged M3 2c build on the Claude plugin install, the ws
MCP server failed to connect. `/mcp` reported only:

```
Failed to reconnect to plugin:ws:ws: -32000
```

`-32000` is the JSON-RPC reserved implementation-defined server-error code; the
MCP client emits it whenever a stdio server fails to come up. It carries no
detail about *why*.

## What was actually wrong

The Python launcher (`ws-mcp-launcher.py`) judged the runtime incompatible and
exited 1 with `incompatible ws-mcp runtime after repair` (the contract-hashed
binary the Claude path had downloaded from the GitHub release predated 2c and
lacked `ws.lead.prefer_mercenary`, so `tools_compatible` was False; the Claude
install path has no source-build fallback to repair it). That real reason was
printed to the launcher's **stderr only** — invisible to the MCP client, which
saw nothing but `-32000`.

Diagnosis required manually running the installed launcher under the same args
as `.mcp.json` and reading its stderr, plus breaking down
`runtime_fully_compatible` sub-checks by hand. A normal user would only see
`-32000` and have no path forward.

## Why it matters

Launcher startup/repair failure is a foreseeable dogfood and first-install
condition (missing release asset, version skew, incompatible cache binary,
failed local build). Collapsing all of it into an opaque `-32000` makes every
such failure a manual forensic exercise. The launcher already knows the precise
cause at `fail(...)` time.

## Possible follow-ups

- Have the launcher write a concise, durable failure breadcrumb the user can
  find without re-running it by hand — e.g. a `last-launch-error` file in the
  runtime dir, or a louder one-line summary naming the failing sub-check
  (version vs tools vs commands vs prompt-bundle) and the resolution hint
  (set `WS_MCP_BOOTSTRAP_BINARY`, place a `.local-devenv-runtime` contract, or
  check the release tag).
- Investigate whether the MCP client can surface any server stderr tail on a
  `-32000` connect failure, so the real reason reaches the user surface.
- Sibling finding from the same dogfood session, already resolved by
  `731cfb84`: the Claude install could not dogfood an unmerged/dev build at all
  because `local_devenv_cache_package` was gated to the `~/.codex` cache only;
  that commit extended it to `~/.claude`. This ticket is only about the
  *diagnostic opacity* that remains regardless of which repair path is taken.

## Notes

- Coordinate with `260611-bug-agent-context-exhaustion-opaque-failure` — both
  are "the real terminal reason is buried; the lead-facing surface is generic."
  The launcher case is process-startup; the agent case is runner invocation.
  Decide at triage whether a shared diagnostic-surfacing convention covers both.

## Resolution (2026-06-20)

Took the first follow-up: `fail()` now writes a best-effort durable breadcrumb
to `<runtime_dir>/last-launch-error` (timestamp + the precise `fail(...)`
reason) in both launchers. main() sets the breadcrumb dir as soon as
`runtime_dir` is resolved and clears the file on the success path, so
`last-launch-error` exists only when the most recent launch actually failed. A
user who sees a bare `-32000` connect failure now has a readable reason on disk
without re-running the launcher by hand. Early failures (before `runtime_dir` is
known — bad OS/arch) still go to stderr only, which is acceptable: those are
environment-obvious.

Verified both launchers: py_compile clean, and a functional check confirms the
breadcrumb is written with the reason on a post-set fail, skipped on an early
fail, and removed by the success-path clear.

Not taken here:
- Surfacing server stderr through the MCP client on `-32000` is outside the
  launcher's control (client behavior); the on-disk breadcrumb gives the user a
  path regardless.
- A shared diagnostic-surfacing convention with
  `260611-bug-agent-context-exhaustion-opaque-failure` (now promoted to `todo/`,
  DEEP) belongs to that ticket's runner-invocation work; the breadcrumb here is
  the process-startup half and does not block it.
