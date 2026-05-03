# Brief: 260503-feat-ws-mcp-git-read-primitives

## Intent

Add the first read-only Git surface to `ws-mcp` so shared workflow skills can
inspect status, diffs, logs, and merge bases through explicit MCP and CLI tools
instead of relying on raw shell wording.

## Approach

- Add a small internal Git helper package under `agents-plugin-tool/internal/`
  that shells out to native `git` using `exec.Command` argument arrays.
- Expose `git.status`, `git.diff`, `git.log`, and `git.merge_base` through the
  MCP server in `internal/mcp/server.go`.
- Add matching CLI subcommands under `ws-mcp git ...` in `cmd/ws-mcp/main.go`.
- Update `agents-plugin/runtime.json` so launcher drift checks know about the
  new MCP tools and CLI fallback commands.
- Add focused Go tests for command argument construction, status/log parsing,
  MCP tools/list visibility, and CLI behavior where practical.

## Constraints

- Keep the tools read-only: no staging, committing, reset, checkout, merge,
  clean, push, or branch mutation.
- Never invoke Git through a shell string; pass every argument as its own argv
  entry.
- Path filters must be appended after `--`.
- Keep `ws/git.commit` out of scope.
- Do not update specs or mental models on this branch.
- Preserve existing public tool names and CLI behavior.

## Out of scope

- Commit creation or commit-message formatting.
- Generated ticket/spec reference graph tooling.
- Project-specific build or test command execution.
- Native Windows runtime smoke beyond compile-safe code and tests.

## Details

Expected MCP tools:

- `git.status`
  - Inputs: optional `root`.
  - Behavior: run a porcelain-safe Git status command and report branch/head
    state, whether the worktree is clean, and changed files.
- `git.diff`
  - Inputs: optional `root`, optional `range`, optional `paths`, optional
    `mode` with `full`, `stat`, or `name_only`.
  - Behavior: run `git diff` with the selected mode and optional `--` paths.
- `git.log`
  - Inputs: optional `root`, optional `range`, optional `limit`, optional
    `include_body`.
  - Behavior: return a bounded commit list with hash, subject, author/date, and
    body when requested.
- `git.merge_base`
  - Inputs: optional `root`, required `base` and `head`.
  - Behavior: return the merge-base hash.

JSON text responses are acceptable for the first slice if they are stable and
model-readable. CLI output may be JSON for parity.

## References

- `[Must]` `ai-docs/tickets/todo/260503-feat-ws-mcp-git-read-primitives.md` -
  source ticket, phases, constraints, and verification surface.
- `[Must]` `agents-plugin-tool/cmd/ws-mcp/main.go` - CLI command routing and
  fallback command patterns.
- `[Must]` `agents-plugin-tool/internal/mcp/server.go` - MCP tool handlers and
  tools/list schema definitions.
- `[Must]` `agents-plugin/runtime.json` - runtime drift metadata to update.
- `[Must]` `agents-plugin-tool/internal/mcp/server_test.go` - MCP stdio test
  pattern and tools/list assertions.
- `[Maybe]` `agents-plugin-tool/cmd/ws-mcp/main_test.go` - CLI-adjacent test
  style.
- `[Maybe]` `agents-plugin-tool/scripts/smoke-ws-mcp.sh` - local smoke script
  if useful to extend.
