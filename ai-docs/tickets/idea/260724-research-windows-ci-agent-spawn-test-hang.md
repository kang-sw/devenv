---
title: Windows CI `go test ./...` hangs on agent-backend-spawn tests
related:
  260724-bug-windows-mcp-mid-session-disconnect: origin
related-mental-model:
  - mcp-runtime
---

# Windows CI `go test ./...` hangs on agent-backend-spawn tests

## Background

Ticket `260724` Phase 3a added a "Run Go tests (Windows)" step (`go test ./...`)
to the `ws-mcp-release.yml` windows-smoke job so the Windows-only parent-death
watch code is exercised in CI. A local Windows smoke run (native go1.26.3
windows/amd64, repo copied off the `\\wsl.localhost` UNC path to a native temp
dir) revealed that this step is at risk of hanging.

## Observation

Running `go test ./...` on native Windows:

- **Green:** `internal/mcp` (panic recovery), `cmd/ws-mcp` (parent-watch, after
  the helper fix in commit `818f57f6`), `internal/wsstore` (SQLite Phase 4),
  plus `wsconfig`, `wsdoc`, `wsgit`, `wskey`, `wsstate`.
- **Hang → 600s package timeout:** `internal/wsagent`. The timeout panic dump
  caught `TestInterruptQueuesInboxAndHookDeliversMessages` blocked deep in
  agent `Register`. A non-spawning test in the same package
  (`TestSQLiteAgentMetadataRoundTripIncludesContractFields`) passes in ~8s, so
  package setup and the SQLite path are fine — the hang is isolated to tests
  that spawn a real agent backend (e.g. `TestRunCurrentUsesClaudeBackendRunner`,
  the interrupt/hook/async-worker tests).

This suite passes on Linux CI (the ticket's final integrated run reported 12
packages ok including `wsagent`), so the divergence is Windows-specific: the
backend-spawn path resolves quickly on Linux but blocks on Windows in this
environment.

## Open questions

- Does GitHub Actions `windows-latest` hit the same hang, or is it an artifact
  of launching the toolchain under a WSL-spawned `powershell.exe` (process
  model / no real backend binary present)? The local environment was also
  incomplete (the `ai-docs/` sibling was not copied, which independently failed
  `wsrsrc/TestRetiredAPIGuidanceNotShipped` on a missing `..\..\..\ai-docs\spec`
  path — a copy artifact, not a defect).
- If real Windows CI hangs too, the Phase 3a gate needs one of: scoping the
  Windows test step to the Windows-relevant packages (`./cmd/ws-mcp/...`,
  `./internal/mcp/...`, `./internal/wsstore/...`), a shorter `-timeout` with a
  package allowlist, or Windows build/skip guards on the backend-spawn tests.
- Is the Windows spawn-path block a latent product bug (agent backend runner
  behaving differently on Windows) rather than only a test concern? Worth a
  focused look before trusting agent orchestration on Windows.

## Next step

Let the release workflow's windows-smoke job be the arbiter on real CI, or
reproduce on a full Windows checkout (all sibling trees present, backend
available). Promote to `todo/` if CI confirms the hang.
