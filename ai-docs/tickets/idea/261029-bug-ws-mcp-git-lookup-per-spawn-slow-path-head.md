---
title: ws-mcp resolves the git binary on every spawn, so a slow PATH head multiplies its cost
---

# ws-mcp resolves the git binary on every spawn, so a slow PATH head multiplies its cost

## Background

Dogfood capture from the v0.48.6 pre-flight (2026-10-29, WSL2 machine).

Every git spawn in ws-mcp passes the bare name `"git"` to `exec.Command` /
`exec.CommandContext`, so Go runs `exec.LookPath` on each call and stats every
`PATH` entry ahead of the binary. On the dogfood machine `PATH` begins with a
Windows directory injected by a desktop app
(`/mnt/c/Users/<user>/AppData/Roaming/orca/wsl-managed-cli/<hash>`); each stat
under `/mnt/c` crosses the WSL interop filesystem and costs about 22 ms
(200 × `test -e` there: 4.5 s, versus 0.009 s under `/usr/local/bin`).

Measured effect:

- `TestAcquireWarningRelaySuffix` spawns git 305 times: 6.4 s with the
  machine's `PATH`, 0.8-1.0 s with the `/mnt/` entries removed.
- The whole `internal/mcp` package (738 top-level tests, serial): 466 s versus
  66 s. At 466-581 s it sits next to the 600 s default `go test` package
  timeout, which is how the slowdown surfaced.
- The runtime pays the same per-call cost: production spawn sites that name
  `"git"` without a cached path are `internal/wsgit/git.go:28`,
  `internal/wsindex/git.go:110`, `internal/mcp/server.go:3748`,
  `internal/wsstate/paths.go:285`, `internal/wsdoc/tickets_scope.go:240`,
  `internal/wsdoc/tickets_sage_freshness.go:288`, and
  `internal/wsdoc/project_tree.go:98,111`. About 20 test files also spawn
  `"git"` directly.

Any downstream machine with a slow or network-backed directory early in
`PATH` sees the same multiplier, so this is not specific to this repository.

Prior speedups were CI-side only and did not touch local runs: e03c89961
(Windows `internal/mcp` shards) and 8c64a9414 (Windows 30 min package
timeout).

### Separate open item: plain `go test ./...` did not finish

During the same pre-flight a plain `go test ./...` in `agents-plugin-tool`
ran for 30 minutes without finishing and was killed (exit 143). Slowness alone
does not explain it: an overrun package would panic at the 10 min default
timeout and exit. Unconfirmed hypothesis: after a test binary died, a child it
had spawned (for example a hang-ssh transport) kept the stdout pipe open, so
the `| tail` consumer never saw EOF. Not reproduced yet.

## Open Questions

Unsettled; to be decided before promotion.

- Fix shape: resolve git once per process (`exec.LookPath` at first use,
  cached absolute path) and use it at every spawn site, versus a shared spawn
  helper that owns resolution, versus leaving runtime code alone and fixing
  only test helpers.
- Whether test helpers that spawn `"git"` directly should switch to the same
  resolved path, and how a test that deliberately overrides `PATH` keeps
  working with a cached path.
- Whether the 30-minute stall is reproducible and related at all, or belongs
  in its own ticket.
