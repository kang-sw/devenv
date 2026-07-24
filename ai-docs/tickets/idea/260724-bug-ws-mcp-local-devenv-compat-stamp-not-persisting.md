---
title: local-devenv compatibility stamp never persists, so every first-connect re-enters the forced go build
---

# local-devenv compatibility stamp never persists, so every first-connect re-enters the forced go build

## Background

Under the active local-devenv dogfood loop (`agents-plugin/.local-devenv-runtime`
present), the ws-mcp launcher fingerprints the live Go source at startup and, on a
stamp miss, runs a synchronous `go build ./cmd/ws-mcp` before handing stdout to
JSON-RPC. On a cold Go build cache this can exceed the MCP `startup_timeout_sec:
30` and the harness marks the `ws` server failed; a manual reconnect builds
against a warm cache and connects. This first-connect failure was observed
2026-07-24 at session start.

The forced-build-vs-timeout behavior itself is known/documented
(`ai-docs/ref/ws-mcp.md` "Local Devenv Repair"; `.done` ticket
`260506-bug-ws-mcp-launcher-startup-delay`; launcher comment near
`agents-plugin/bin/ws-mcp-launcher.py:519`). This ticket captures a **distinct,
fresh finding** underneath it.

## Fresh finding

The compatibility stamp (`.compatibility.json`) is **absent everywhere in the
plugin cache** (`find … -name .compatibility.json` empty across all versions). The
stamp is written only after build+validate succeeds (`ws-mcp-launcher.py:868`),
but a startup killed by the 30s host timeout dies mid-startup, after the binary is
installed but before the stamp is written. Because the stamp never lands, the
fast-path guard (around `:852`) never engages, so **every first-connect re-enters
the forced-build branch even when the source is unchanged** — the stamp is not
protecting steady-state startups as designed.

## Evidence

- `~/.claude/plugins/cache/kang-sw-devenv/ws/0.36.1/.runtime/linux-amd64/ws-mcp`
  mtime 20:26 == the failing session's `history.jsonl` mtime (rebuilt at connect
  time, not pre-staged).
- No `.compatibility.json` anywhere under the plugin cache.
- Live Go source edited 16:25–17:02 that day (incl. the 0.36.1 release commit),
  guaranteeing a fingerprint miss.

## Possible directions

- Write the stamp (or a "build succeeded, binary at <hash>" breadcrumb) as early
  as the binary is installed, or decouple stamp persistence from the
  validate-then-stamp tail so a later timeout kill does not erase the fast-path.
- Consider building to a temp path and atomically swapping + stamping, so a killed
  startup never leaves a stamped-but-unvalidated or unstamped-but-built state.
- Or pre-warm/serialize the local-devenv build off the startup path so first
  connect never blocks on a cold build.

## Notes

Dogfood surprise surfaced while diagnosing the session-start MCP connection
failure the user asked about. Diagnosis confidence ~0.8 that the forced build vs.
30s timeout is the failure; the stamp-not-persisting is the actionable residue.
Not yet reduced to a deterministic repro.
