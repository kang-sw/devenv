---
title: Runtime binary staging copy
related:
  260513-research-streamable-http-mcp-transport: adjacent daemon/launcher lifecycle discussion
related-mental-model:
  - plugin-runtime
  - mcp-runtime
  - claude-compatibility
---

# Runtime binary staging copy

## Background

Plugin-managed MCP startup currently executes the repaired runtime binary from
the plugin cache runtime directory. On Windows, executing files inside a plugin
cache can interact poorly with plugin updates, cache replacement, or deletion
because the running executable can keep the file locked.

This improvement is independent from a future HTTP daemon transport. The launcher
can reduce update-time file conflicts by copying the selected runtime executable
to a deterministic staging path and executing that staged copy instead of the
plugin-cache source binary.

## Decisions

- The staged executable name must include the runtime version deterministically,
  such as `ws-mcp-v0.26.1.exe` or the platform-appropriate equivalent.
- Each runtime version should be copied at most once per staging location during
  normal operation. Later launches of the same version reuse the staged binary
  after validating that it still matches the selected runtime.
- Staging must not change the runtime compatibility contract. The launcher still
  validates the source runtime against `runtime.json` before choosing what to
  stage.
- Cleanup is best effort. Stale staged binaries from old versions may be removed
  opportunistically, but cleanup failure must not break MCP startup.

## Phases

### Phase 1: Add deterministic staging execution

Update the plugin launcher so it stages the selected runtime binary into a
versioned filename before final execution, especially for Windows plugin-managed
startup. Preserve existing repair, checksum, compatibility-stamp, and local dev
runtime behavior.

The phase should define the staging directory, filename scheme, validation rules
for reusing an existing staged copy, and final environment variables such as
`WS_MCP_RUNTIME_BINARY` after staging is introduced.

Suggested verification:

- Launcher tests cover first-copy and same-version reuse behavior.
- Existing runtime repair and compatibility tests continue to pass.
- Windows-oriented behavior is covered without requiring a live Claude session.

### Phase 2: Add best-effort staged runtime cleanup

Add opportunistic cleanup for stale staged runtime binaries from older versions.
Cleanup should be bounded, best effort, and ignored on permission or lock
failures. It must not delete the currently selected staged binary.

Suggested verification:

- Cleanup skips the current version.
- Cleanup ignores locked or undeletable files.
- Cleanup failure does not prevent final MCP startup.
