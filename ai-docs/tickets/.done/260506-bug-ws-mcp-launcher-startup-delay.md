---
title: ws-mcp launcher startup delay
related:
  260503-feat-agents-plugin-runtime-boundary: runtime launcher and Windows plugin-managed startup parent slice
spec:
  - 260506-launcher-hot-path-compatibility-cache
  - 260506-runtime-capabilities-single-probe
skeletons:
  phase-3: 7d13546
related-mental-model:
  - plugin-runtime
completed: 2026-05-10
---

# ws-mcp launcher startup delay

## Background

Windows plugin-managed startup showed that the native `ws-mcp.exe` runtime starts
quickly, but the Python launcher can spend almost the whole configured startup
timeout in compatibility preflight before handing control to the runtime.

Measured upstream on Windows:

- Direct `ws-mcp.exe serve --stdio` initialize plus `tools/list`: about 386 ms.
- Python launcher startup for the same initialize plus `tools/list`: about
  27,481 ms.
- Configured MCP `startup_timeout_sec`: 30 seconds.

The reported breakdown points at launcher-side validation, not the Go runtime:

- `version_only`: about 330 ms.
- `tools_compatible`: about 457 ms.
- `commands_compatible`: about 12,372 ms.
- `prompt_bundle_compatible`: about 328 ms.
- Full compatibility check: about 13,439 ms.
- Runtime contract surface at report time: 45 MCP tools and 36 CLI commands.

The current Python launcher calls full compatibility validation on the hot path.
That validation runs `version`, a temporary `serve --stdio` plus `tools/list`,
one CLI subprocess per command declared in `runtime.json`, and `runtime info` for
the prompt bundle hash. On Windows, each runtime subprocess has meaningful
startup cost, so command-surface fanout dominates.

The launcher also calls the full validation twice in the normal path:

```python
if not runtime_fully_compatible(binary, contract, runtime_dir):
    install_runtime(...)

if not runtime_fully_compatible(binary, contract, runtime_dir):
    fail(...)
```

When the runtime is already compatible, the first call returns true and should be
enough. The second full pass makes normal startup land close to the 30-second MCP
startup timeout on the measured Windows host. Antivirus, disk contention, Python
startup variance, or slower process creation can push normal startup over the
timeout.

## Decisions

- Treat this as a launcher hot-path bug, not a Go MCP server startup bug.
- Do not treat raising `startup_timeout_sec` as the primary fix. It is only a
  mitigation because it leaves the launcher doing expensive repeated validation.
- Keep runtime validation semantics equivalent: version range, MCP tool surface,
  CLI command surface, and prompt bundle hash still need protection against stale
  cache-local binaries.
- Promotion to `todo/` updated `ai-docs/spec/plugin-runtime.md` with the startup
  preflight behavior and performance boundary. The existing spec already covered
  stale-runtime validation, and now also tracks the hot-path compatibility cache
  behavior.

## Open Questions

- Should the POSIX `agents-plugin/bin/ws-mcp-launcher` keep parity with the
  Python launcher, or is it now a compatibility fallback that can remain slower?
- Should a compatibility stamp be invalidated by binary mtime, binary hash, or
  both? Mtime is cheap but weaker; hash is stronger but adds startup I/O.
- Should the new capability command be named `runtime capabilities`,
  `runtime contract`, or another stable contract-oriented name?
- Should old runtimes without the capability command fall back to the current
  fanout check once, or be treated as incompatible and repaired immediately?

## Phases

### Phase 1: Avoid duplicate full compatibility checks

Cache the result of the first `runtime_fully_compatible()` call in the Python
launcher. Re-run full validation only after an install or repair action changes
the runtime binary.

This is the lowest-risk fix. It should roughly halve the measured compatible
startup cost on the reported Windows host, from about 27 seconds to about 13
seconds.

Success criteria:

- Compatible installed runtimes perform only one full compatibility pass before
  process handoff.
- Missing or incompatible runtimes are still repaired and then validated before
  handoff.
- Launcher stdout remains reserved for MCP JSON-RPC.

### Result (d3ae5c9) - 2026-05-06

The Python launcher now stores the initial compatibility result in `main()` and
only reruns full validation after install or repair changes the runtime binary.
Compatible installed runtimes no longer perform the duplicated full preflight.

Verification used a fake-runtime smoke that counted launcher subprocesses and a
real source-tree launcher smoke. The fake runtime showed first startup using the
full validation path and second startup skipping back to final handoff.

### Phase 2: Add a compatibility stamp for hot-path reuse

Add a launcher-managed compatibility stamp keyed by the runtime contract and the
installed runtime binary identity. When the stamp matches, skip `tools/list`,
command-surface fanout, and prompt-bundle probing on startup.

Suggested key material:

- `runtime.json` content hash.
- Runtime binary path.
- Runtime binary mtime or content hash.
- Plugin version and required MCP range.

This should make steady-state startup approach direct runtime startup instead of
paying validation cost on every MCP server launch.

Success criteria:

- First startup after install, repair, runtime change, or contract change still
  validates the full required surface.
- Later startups with unchanged runtime and contract skip expensive validation.
- Corrupt, missing, or stale stamps fail closed into full validation.
- Stamp writes do not print to stdout and do not corrupt stdio MCP startup.

### Result (d3ae5c9) - 2026-05-06

Added a launcher-managed `.compatibility.json` stamp under the selected runtime
directory. The stamp records schema version, runtime contract hash, plugin
version, required MCP range, runtime binary path, size, and mtime. Matching
stamps skip full validation; missing, unreadable, stale, or mismatched stamps
fall back to full validation. Install and repair paths clear the stamp and write
a new one only after the replacement runtime validates successfully.

Verification confirmed that touching the runtime binary invalidates the stamp
and forces the full validation path again. Real source-tree `version` and stdio
`tools/list` smokes passed with cached compatibility and no launcher stdout
pollution.

### Phase 3: Add a single runtime capability command

Add one public `ws-mcp` runtime command that reports the runtime surfaces needed
for launcher compatibility in a single process. The launcher can then validate
version, MCP tools, CLI commands, and prompt bundle hash from one JSON response
instead of starting a temporary MCP server and 36 CLI subprocesses.

Likely command shape:

```text
ws-mcp runtime capabilities
```

Likely JSON payload:

```json
{
  "version": "...",
  "source_commit": "...",
  "mcp_protocol": "2025-03-26",
  "prompt_bundle": {
    "source_commit": "...",
    "content_sha256": "...",
    "prompts": []
  },
  "tools": [],
  "commands": []
}
```

Contract decisions:

- The command name is `ws-mcp runtime capabilities`.
- The command returns JSON to stdout and reserves stderr for diagnostics.
- The response describes the full lead MCP tool surface required by
  `runtime.json`, independent of caller-local tool profile filters.
- Old runtimes without the command may use a bounded fallback validation path,
  but a successful fallback must still write the same launcher compatibility
  stamp only after full validation succeeds.

Estimated work from code survey: small-to-medium, about 1.5 to 2 focused
implementation days. The runtime already owns most of the data, but the change
adds a public CLI contract and needs launcher integration, tests, docs, and
release metadata updates.

Likely implementation areas:

- `agents-plugin-tool/cmd/ws-mcp/main.go` for the new `runtime` subcommand.
- `agents-plugin-tool/internal/mcp/server.go` or a nearby helper for enumerating
  the lead MCP tool surface without starting `serve --stdio`.
- `agents-plugin/bin/ws-mcp-launcher.py` for single-probe compatibility.
- `agents-plugin/bin/ws-mcp-launcher` if POSIX launcher parity is required.
- `agents-plugin/runtime.json` to include the new public command.
- Go CLI tests and MCP tool-surface tests.
- `ai-docs/spec/plugin-runtime.md` and `ai-docs/ref/ws-mcp.md`.

Risks:

- Old runtimes do not have the new command, so the launcher needs an intentional
  transition path.
- The command must return the lead/full required tool surface, independent of
  inherited `WS_MCP_TOOL_PROFILE` or `WS_MCP_ALLOWED_TOOLS`, or it must exactly
  emulate the current launcher environment.
- Command enumeration can drift from `main.go` if the list is manually
  maintained.
- Once added to `runtime.json.commands`, the new command becomes part of the
  runtime compatibility contract.

Success criteria:

- New compatible runtimes can be validated with one runtime subprocess.
- Validation remains semantically equivalent to the current version, tool,
  command, and prompt-bundle checks.
- The launcher handles old runtimes predictably through repair or a bounded
  fallback path.
- Windows steady-state startup is no longer sensitive to command count growth.

### Result (df1afdd) - 2026-05-06

Added `ws-mcp runtime capabilities` as a single-process JSON probe for runtime
version, source commit, MCP protocol version, prompt bundle metadata, full lead
MCP tool names, and the public CLI command surface. The Python launcher now
tries that probe before legacy validation and returns to the existing full
validation fanout when the command is absent, invalid, incomplete, or
incompatible.

The delegated review cycle found and fixed two issues before merge: invalid
explicit server roots now fail closed instead of falling through to
`WS_MCP_PROJECT_ROOT`, and launcher fallback tests now exercise the real
capabilities probe path plus version, protocol, prompt-bundle, tool, and command
mismatch cases. Final correctness, fit, and test re-reviews were clean.

Verification passed on Linux/source-tree execution with `go test ./...`,
`go test ./cmd/ws-mcp -run
TestRuntimeCapabilitiesCommandReportsLauncherContractSurface`, Python launcher
unit tests, Python compile checks, and `git diff --check`. Native Windows
installed-cache verification remains a release/post-ship validation item.

### Phase 4: Startup timeout mitigation

Consider raising `startup_timeout_sec` only after the hot-path fixes are designed.
This can reduce intermittent failures while users upgrade, but it must not be
treated as the root fix.

Success criteria:

- Timeout changes are justified as compatibility mitigation, not as the primary
  performance solution.
- Documentation remains clear that normal steady-state startup should not spend
  tens of seconds in launcher preflight.

### Result (downstream verified) - 2026-05-10

Downstream installed-plugin verification confirmed that the launcher startup
delay is resolved after the hot-path compatibility cache and
`runtime capabilities` single-probe changes. No timeout increase is needed for
the normal path.

The 30-second startup timeout remains unchanged. It is still an upgrade-buffer
knob if future release validation finds an old-runtime transition problem, but
this ticket's root cause was fixed in launcher validation behavior rather than
papered over by raising the timeout.
