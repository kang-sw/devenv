# Plan: 260503-feat-ws-mcp-git-read-primitives

## Steps

1. Add an internal Git package that runs native Git with argv slices and returns
   structured status, diff, log, and merge-base results.
2. Wire `ws-mcp git status|diff|log|merge-base` CLI subcommands to that package.
3. Wire MCP tools `git.status`, `git.diff`, `git.log`, and `git.merge_base` to
   the same package and add tools/list schemas.
4. Update `agents-plugin/runtime.json` tool and command metadata.
5. Add focused Go tests for Git helper behavior and MCP visibility.
6. Verify with `go test ./...`, runtime metadata parsing, CLI smoke, plugin
   validation, and `git diff --check`.

## Notes

- Keep output JSON for the first slice so MCP and CLI have the same stable
  shape.
- Keep `git.commit` out of this implementation.
- Record any dogfood gap from `write-code` orchestration in the ticket result.
