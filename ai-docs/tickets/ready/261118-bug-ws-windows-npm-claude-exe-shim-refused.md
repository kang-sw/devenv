---
title: Windows npm-installed Claude is refused because its cmd-shim launches an exe, not node
related:
  261112-bug-ws-hosted-windows-cmd-shim-host-pid-mismatch: origin of the shim unwrap this extends
  261113-research-hosted-detection-by-session-identity: longer-term alternative to PID identity
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 960c324adc71f580
sage-review-completeness-reviewed: 960c324adc71f580
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

The error comes from `cmdShimArgv` (`agents-plugin-tool/internal/wsagent/backend.go#L506-L523`, the message at :509),
reached from `harnessCommand` (backend.go:461-485). 261112 added the unwrap so a
hosted start never runs a `.cmd` under `cmd.exe`: the holder records the spawned
PID as `HostPID`, and ws-mcp `resolveHostedSession`
(`internal/mcp/process_env.go#L280-L325`) and `hostedLoginGuard`
(`internal/mcp/hosted_login.go:175`) require it to equal the harness PID. The
unwrap accepts only the npm node shape: the text must contain `SET "_prog=node"`
and the launch line must match `"%_prog%"  "%dp0%\<script>" %*`
(`cmdShimLine` backend.go:496, `cmdShimNode` :500). 261112 verified only the
npm-installed Pi (a node shim) and reasoned that Claude's native installer
yields `claude.exe`, which passes through unchanged (backend.go:475-479).

Current `@anthropic-ai/claude-code` npm releases ship a native binary, so npm's
cmd-shim generator writes a shim whose launch line runs the target directly,
with no `_prog`. The owner supplied the real file from an affected install
(2026-11-18 17:58 KST), verbatim:

```bat
@ECHO off
GOTO start
:find_dp0
SET dp0=%~dp0
EXIT /b
:start
SETLOCAL
CALL :find_dp0
"%dp0%\node_modules\@anthropic-ai\claude-code\bin\claude.exe"   %*
```

The dev PC itself has only the native install
(`%USERPROFILE%\.local\bin\claude.exe`) and no npm `claude.cmd`. The only
Claude spawn path is `ClaudeBackend.Start` (`internal/wsagent/claude.go:328`, its `harnessCommand` call at :329),
used for both hosted leads and child agents; the dashboard registers
`ClaudeBackend{}` with an empty `Command`, so it looks `claude` up on PATH
(`cmd/ws-mcp/dashboard.go:267`).

## Decisions

Confirmed by the owner on the board, 2026-11-18 18:25 KST ((1378)-(1382) ok, (1383) a).

1. (1378) The fix extends the shim unwrap in `harnessCommand`/`cmdShimArgv`
   (`internal/wsagent/backend.go`), so every harness start (Claude and Pi,
   leads and children) keeps one resolution path. 261112 placed the unwrap
   there and `internal/wsagent/cmdshim_test.go` pins that path.
2. (1379) A cmd-shim whose launch line runs a quoted `%dp0%`-relative `.exe`
   target followed by `%*` is unwrapped to that exe, started directly with the
   caller's args. Rejected: falling back to a PATH scan for some `claude.exe`
   - the shim's own target is the install the user's PATH chose, and parsing
   it is deterministic where a PATH scan could start a different install.
3. (1380) The new test fixture `testdata/npm-cmd-shim-claude.cmd` is the
   owner-supplied real shim in `## Background`, byte-for-byte (three spaces
   before `%*`), stored CRLF like `testdata/npm-cmd-shim-pi.cmd`. Rejected: a
   hand-written or regenerated approximation - the real file is what affected
   users have.
4. (1381) Compatibility tier T0: local process spawning only; no MCP tool,
   hello, store shape, tracker event, or manifest field changes.
5. (1382) Verification is Go unit tests on w5090 plus the Windows legs of
   ws-mcp CI; after release the owner confirms on an affected npm install. No
   local `go test`, ws executables, or live Claude start on the dev PC.
6. (1383) Accepted shapes are exactly the npm node shim (today) and the
   direct-`.exe` shim (new). Any other `.cmd`/`.bat` keeps being refused, and
   the refusal message names the shim path and says how to fix it: install
   Claude's native build, or point the claude command at the real exe.
   Rejected: (b) unwrapping shims that run another interpreter (`_prog` other
   than node, e.g. bun); (c) retrying PATH for a `claude.exe` after a refusal -
   both add guessing for shapes nobody has reported.
   This supersedes 261112's strict-parse rule (a3795bf8: only `SET
   "_prog=node"` shims) in part: the direct-exe shim is now also accepted;
   every other shape stays refused.

Design-review clarifications (2026-11-18, lead auto-confirmed reviewer issues
marked autonomous; they refine the decisions above without changing scope):

- `cmdShimArgv` serves Pi too (`harnessCommand(..., "pi")` in `pi.go` and
  `pi_probe_cache.go`), so the refusal remedy is harness-aware: the Claude
  wording applies only when `harnessCommand`'s `name` is `claude`; any other
  harness gets a neutral remedy (install that harness's native build, or put
  its real executable ahead of the shim on PATH). Wrap or reword in
  `harnessCommand`, which already has `name`; `cmdShimArgv` keeps its
  signature.
- No setting configures the Claude command (`ClaudeBackend{}` has an empty
  `Command`, `cmd/ws-mcp/dashboard.go:267`), so "point the claude command at
  the real exe" is phrased as PATH order: put the directory holding the native
  `claude.exe` (for example `%USERPROFILE%\.local\bin`) ahead of `%APPDATA%\npm`.
- The exe-shim match is strict: try the existing node-shim match first; then
  accept only a launch line that starts with a quoted `"%dp0%\<relative
  path>.exe"` (no `..` segment, no absolute path) and ends with `%*`, with
  any whitespace between and an optional trailing CR. The resolved exe must
  exist, like the node-shim path's script; a missing target is refused with
  the Decision 6 message.
- The fixture ends with CRLF after `%*`, like `npm-cmd-shim-pi.cmd`;
  `testdata/.gitattributes` (`*.cmd -text`) already preserves it.
- Decision 5's post-release confirmation also checks the premise that the npm
  package's `bin\claude.exe` is the Claude process itself (not a launcher that
  spawns a child): a hosted Claude lead must start without a HostPID mismatch.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)
- Convention: ai-docs/manuals/release-compatibility.md (declared for agents-plugin-tool/internal/)
- Convention: ai-docs/manuals/performance.md (declared for agents-plugin-tool/)

## Prior Decisions

- 261112-bug-ws-hosted-windows-cmd-shim-host-pid-mismatch (2026-11-12, commit a3795bf8): "Parsing is strict: the shim must carry npm cmd-shim's `"%_prog%" "%dp0%\<script>" %*` line and its `SET "_prog=node"` fallback." — bearing: constrains
- 261112-bug-ws-hosted-windows-cmd-shim-host-pid-mismatch (2026-11-12, commit a3795bf8): "The fixture is a byte-exact copy of a real npm-generated pi.cmd (CRLF); a testdata/.gitattributes `*.cmd -text` keeps the root `eol=lf` rule from normalising it." — bearing: supports
- 261112-bug-ws-hosted-windows-cmd-shim-host-pid-mismatch (2026-11-12, commit a3795bf8): "Rejected (ticket): matching an ancestor PID in ws-mcp and the Pi extension; looking up cmd.exe's child after the spawn." — bearing: constrains
- 261112-bug-ws-hosted-windows-cmd-shim-host-pid-mismatch (2026-11-12, commit a3795bf8): "A user-set Command goes through lookPath only on Windows (needed to see a PATHEXT-resolved .cmd); elsewhere it passes through unresolved as before." — bearing: constrains
- 261112-bug-ws-hosted-windows-cmd-shim-host-pid-mismatch (2026-11-13, Resolution): "Verified on this Windows host 2026-11-13 with the npm-installed Pi (pi.cmd): a dashboard-hosted Pi lead row records host_pid 6548, which is node.exe running pi-coding-agent's cli.js." — bearing: supports
- 261112-bug-ws-hosted-windows-cmd-shim-host-pid-mismatch (2026-11-12, commit a3795bf8): "After the unwrap, holder/child error messages name `node.exe` (they use filepath.Base(cmd.Path)); cosmetic, left as is." — bearing: supports
- 46250ca4 (2026-11-18, commit): "Owner supplied the real claude.cmd from an affected install (17:58 KST): it runs %dp0%\node_modules\@anthropic-ai\claude-code\bin\claude.exe directly with no _prog, confirming the inferred shape." — bearing: supports
- f333db88 (2026-11-18, commit): "cmdShimArgv (wsagent/backend.go:496-510) accepts only the npm node shim; 261112 verified only Pi; the dev PC has no npm claude.cmd, so the exe-target shim shape is inferred." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | internal/wsagent/backend.go, internal/wsagent/cmdshim_test.go, new internal/wsagent/testdata/npm-cmd-shim-claude.cmd |
| scope.surface | internal | cmdShimArgv and harnessCommand are unexported; no exported symbol changes |
| scope.new_public_symbol | no | none |
| scope.new_type_contract | no | none; cmdShimArgv keeps its signature (backend.go#L506) |
| scope.test_surface | existing | internal/wsagent/cmdshim_test.go (TestCmdShimArgvRefusesWhatIsNotANodeShim and four siblings) |
| complexity.reuse_points | confirmed | cmdShimArgv, harnessCommand, cmdShimLine pattern, readPiShim fixture helper and testdata/.gitattributes in cmdshim_test.go |
| complexity.side_effect_risk | moderate | cmdShimArgv is shared by every harness start (Claude and Pi leads, children, Pi probes at pi.go#L444, pi_probe_cache.go#L122) |
| risk.correctness | moderate | new regex must accept the real CRLF shape with three spaces before %* and keep refusing other shapes; not runnable on the dev PC |
| risk.fit | low | extends the existing unwrap in the place 261112 put it, per Decision 1 |
| risk.test | moderate | Go tests run only on w5090 and CI; the fixture's CRLF bytes must survive git (testdata/.gitattributes *.cmd -text) |
| risk.security_or_contract | moderate | starts an executable path parsed from a shim file; the refusal text is user-facing; T0 per Decision 4 |

## Phases

### Phase 1: Unwrap exe-target cmd-shims

Goals:
- Extend `cmdShimArgv` per the settled decisions so the npm-installed Claude starts as the exe itself (its PID is the harness's), keeping the node-shim behavior and the refusal for shapes outside the settled scope, with the refusal message reworded per Decision 6.
- Add the owner-supplied fixture and tests beside the existing ones in `internal/wsagent/cmdshim_test.go`: the exe-target shim unwraps (CRLF and LF, case-insensitive extension), a missing exe target is refused, and the existing node-shim and refusal tests still pass.

Verification: `go vet` and `go test ./internal/wsagent/...` on w5090 (accepted pre-existing reds elsewhere only: TestTopicStateBadgeResearchingMetadata, TestTopicStateBadgeClockAndReconciliation); Windows CI legs green at release.

Constraints:
- Manuals that apply: `ai-docs/manuals/shipped-surface-boundary.md` (the error text reaches users), `ai-docs/manuals/release-compatibility.md`, `ai-docs/manuals/performance.md` (one shim read per start; no new file touches on interactive paths).
- No local `go test`, Go tools, gofmt, ws/ws-mcp executables, launchers, or Electron on the dev PC; Go runs on w5090 only.
- Rejected in 261112 and still rejected: matching an ancestor PID in ws-mcp or the Pi extension, and finding cmd.exe's child after spawn.

### Result (8218d3d1e) - 2026-11-18

Landed:
- `cmdShimArgv` (`internal/wsagent/backend.go`) tries the npm node-shim match first, then a strict exe match `cmdShimExe`: a line that starts with a quoted `"%dp0%\<relative>.exe"` (case-insensitive extension), ends with `%*`, has only spaces/tabs between, and may end with a CR. It refuses a target with a `..` segment, a volume (`:`), or a rooted path. The exe must exist. It is returned as the program with no leading args.
- Shape refusals wrap the sentinel `errShimRefused`; `harnessCommand` appends a harness-aware remedy (`shimRemedy`): for `claude`, install Claude Code's native build or put the directory holding the native `claude.exe` (for example `%USERPROFILE%\.local\bin`) ahead of the shim's directory on PATH; for any other harness, a neutral equivalent. A node shim with no node on PATH keeps its own error.
- Fixture `testdata/npm-cmd-shim-claude.cmd`: the owner-supplied shim byte-for-byte, CRLF (`git ls-files --eol`: `i/crlf w/crlf attr/-text`).
- Tests in `cmdshim_test.go`: `TestCmdShimArgvUnwrapsTheExeShim` (CRLF, LF, `.EXE`), `TestCmdShimArgvRefusesExeShimEdges` (parent segment, volume, rooted, extra args, non-exe, no `%dp0%`, missing target), the end-to-end unwrap of the npm `claude.cmd` through `harnessCommand`, and `TestHarnessCommandRefusalNamesARemedy` (claude vs pi wording). The existing refusal test now also asserts `errShimRefused`.

Verification (w5090): `go vet ./internal/wsagent/...` OK; `go test ./internal/wsagent/...` ok; the 9 cmd-shim/harnessCommand tests PASS with `-count=1 -v`. `gofmt -l ./internal/wsagent` lists only `wake_test.go`, which is untouched by this change and pre-existing. The Windows CI legs and the owner's post-release check on an affected npm install (Decision 5) are still outstanding.

Decisions:
- The remedy names the shim's actual directory instead of the literal `%APPDATA%\npm`. For npm global installs it is that directory, and it stays accurate for other prefixes.
- Lite review left one Minor open: the exe match is per-line, so a hand-written `.cmd` that has extra setup lines before an npm-shaped exe launch line would also be unwrapped. The node-shim match is per-line in the same way, so this is consistent and low-risk. Left as is.
