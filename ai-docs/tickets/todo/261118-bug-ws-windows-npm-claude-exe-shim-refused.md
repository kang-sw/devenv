---
title: Windows npm-installed Claude is refused because its cmd-shim launches an exe, not node
related:
  261112-bug-ws-hosted-windows-cmd-shim-host-pid-mismatch: origin of the shim unwrap this extends
  261113-research-hosted-detection-by-session-identity: longer-term alternative to PID identity
---

# Windows npm-installed Claude is refused because its cmd-shim launches an exe, not node

## Background

On Windows, ws refuses to start Claude for users whose `claude` resolves to the
npm global shim:

```text
C:\Users\<user>\AppData\Roaming\npm\claude.cmd is not an npm cmd-shim for node; a .cmd would run under cmd.exe, whose PID is not the harness's
```

The owner reports (2026-11-18) that many users have Claude installed this way,
so hosted Claude leads and Claude child agents fail outright for them.

The error comes from `cmdShimArgv` (`agents-plugin-tool/internal/wsagent/backend.go:509`),
reached from `harnessCommand` (backend.go:461-485). 261112 added the unwrap so a
hosted start never runs a `.cmd` under `cmd.exe`: the holder records the spawned
PID as `HostPID`, and ws-mcp `resolveHostedSession`
(`internal/mcp/process_env.go:281-292`) and `hostedLoginGuard`
(`internal/mcp/hosted_login.go:175`) require it to equal the harness PID. The
unwrap accepts only the npm node shape: the text must contain `SET "_prog=node"`
and the launch line must match `"%_prog%"  "%dp0%\<script>" %*`
(`cmdShimLine` backend.go:496, `cmdShimNode` :500). 261112 verified only the
npm-installed Pi (a node shim) and reasoned that Claude's native installer
yields `claude.exe`, which passes through unchanged (backend.go:475-479).

Current `@anthropic-ai/claude-code` npm releases ship a native binary, so npm's
cmd-shim generator writes a shim whose launch line runs the target directly
(`"%dp0%\node_modules\@anthropic-ai\claude-code\...\claude.exe" %*`, no
`_prog`). That shape is inferred: the dev PC has only the native install
(`%USERPROFILE%\.local\bin\claude.exe`) and no npm `claude.cmd`. The only
Claude spawn path is `ClaudeBackend.Start` (`internal/wsagent/claude.go:329`),
used for both hosted leads and child agents; the dashboard registers
`ClaudeBackend{}` with an empty `Command`, so it looks `claude` up on PATH
(`cmd/ws-mcp/dashboard.go:267`).

## Open Decision Queue
Last ID: (1383)

(1378) [open] [announced] The fix extends the shim unwrap in `harnessCommand`/`cmdShimArgv` (`internal/wsagent/backend.go`) so every harness start (Claude and Pi, leads and children) keeps one resolution path - 261112 placed the unwrap there and its tests (`internal/wsagent/cmdshim_test.go`) pin that path.
(1379) [open] [technical] Recognize a cmd-shim whose launch line runs a quoted `%dp0%`-relative `.exe` target followed by `%*`, and start that exe directly with the caller's args, over falling back to a PATH scan for some `claude.exe` - the shim's own target is the install the user's PATH chose, and parsing it is deterministic where a PATH scan could start a different install.
(1380) [open] [technical] The new test fixture is produced by the cmd-shim generator bundled with the locally installed npm (offline, `node_modules/npm/node_modules/cmd-shim`) for an `.exe` target, kept CRLF like the existing `testdata/npm-cmd-shim-pi.cmd`, over a hand-written guess - no real npm `claude.cmd` exists on the dev PC, and the generator's output is the shape users actually have.
(1381) [open] [announced] Compatibility tier T0: the change is local process spawning; no MCP tool, hello, store shape, tracker event, or manifest field changes - AGENTS.md `### Compatibility Tiers` lists the cross-version surfaces and none is touched.
(1382) [open] [announced] Verification is Go unit tests on w5090 plus the Windows legs of ws-mcp CI, and after release the owner confirms on an affected npm install; no local `go test`, ws exes, or live Claude start on the dev PC - the owner's antivirus rule (no local go test or ws executables) and the dev PC having no npm `claude.cmd`.
(1383) [open] [policy] Which shim shapes the unwrap accepts beyond today's node shim.
- Context: today only the npm node shim is accepted; everything else is refused with the error above. Other managers (pnpm, yarn, bun, scoop) write their own shim shapes; none is reported failing yet.
- a: the node shim plus the direct-`.exe` shim only; any other `.cmd` keeps refusing, with an error that names the shim and says how to fix it (install Claude's native build, or point the claude command at the real exe).
- b: also any shim that runs another interpreter (`_prog` other than node, e.g. bun) on a script, launching that interpreter directly.
- c: (a) plus, when a `.cmd` is refused, retry PATH for a `claude.exe` before failing.
> a: it fixes the reported shape with the least parsing, and a clearer error turns the rest into a one-step user fix; b and c add guessing for shapes nobody has reported.

## Phases

### Phase 1: Unwrap exe-target cmd-shims

Goals:
- Confirm the real shim shape: read `npm view @anthropic-ai/claude-code bin version` (registry metadata only) and generate the shim with the local npm's bundled cmd-shim for that bin target; record both in the Result.
- Extend `cmdShimArgv` per the settled decisions so the npm-installed Claude starts as the exe itself (its PID is the harness's), keeping the node-shim behavior and the refusal for shapes outside the settled scope.
- Add the generated fixture and tests beside the existing ones in `internal/wsagent/cmdshim_test.go`: the exe-target shim unwraps (CRLF and LF, case-insensitive extension), a missing exe target is refused, and the existing node-shim and refusal tests still pass.

Verification: `go vet` and `go test ./internal/wsagent/...` on w5090 (accepted pre-existing reds elsewhere only: TestTopicStateBadgeResearchingMetadata, TestTopicStateBadgeClockAndReconciliation); Windows CI legs green at release.

Constraints:
- Manuals that apply: `ai-docs/manuals/shipped-surface-boundary.md` (the error text reaches users), `ai-docs/manuals/release-compatibility.md`, `ai-docs/manuals/performance.md` (one shim read per start; no new file touches on interactive paths).
- No local `go test`, Go tools, gofmt, ws/ws-mcp executables, launchers, or Electron on the dev PC; Go runs on w5090 only.
- Rejected in 261112 and still rejected: matching an ancestor PID in ws-mcp or the Pi extension, and finding cmd.exe's child after spawn.
