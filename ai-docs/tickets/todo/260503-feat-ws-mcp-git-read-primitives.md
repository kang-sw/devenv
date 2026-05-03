---
title: ws-mcp Git read primitives
parent: 260503-epic-ws-mcp-vcs-reference-tools
related:
  260503-feat-agents-plugin-runtime-boundary: MCP runtime and launcher baseline
  260503-feat-agents-plugin-write-code-port: first consumer that needs portable start-commit, diff, and log inspection
  260503-epic-ws-mcp-vcs-reference-tools: parent roadmap for portable Git and reference tooling
---

# ws-mcp Git read primitives

## Background

The ported `write-code` skill still relies on lead-side raw Git commands for
start-commit capture, implementation range discovery, diff inspection, and
commit history checks. That is acceptable as a temporary lead action, but the
next harness layer (`implement`, `proceed`, and later `sprint`) will need a
portable MCP surface for these Git reads before shared skills can avoid
host-specific shell wording.

This ticket adds the first Git MCP slice: read-only wrappers around native Git.
The implementation should not reimplement Git semantics. It should call the
project's Git executable with argument arrays, return model-friendly structured
summaries, and keep CLI fallbacks for local development and Claude-compatible
usage.

## Decisions

- Implement only read-oriented Git operations in this ticket.
- Use native `git` through Go `exec.Command` with argument arrays, not shell
  string interpolation.
- Use MCP tool names under `ws/git.*`.
- Keep outputs structured enough for future workflow automation and readable
  enough for current model use.
- Leave `ws/git.commit` for a later ticket because commit creation includes
  staging ownership, AI Context formatting, and unrelated-change policy.

## Constraints

- Do not add destructive Git behavior such as reset, checkout, clean, merge, or
  push.
- Do not stage or commit files from these tools.
- Support optional `root` arguments consistently with existing ws-mcp tools.
- Treat path filters as argument-array entries after `--`.
- Preserve Windows path safety by avoiding shell parsing.
- Update runtime metadata so plugin launcher drift checks can detect stale
  binaries.
- Do not update specs or mental models on this branch.

## Phases

### Phase 1: Git read tool implementation

Add read-only Git tools to `ws-mcp` and matching CLI fallback subcommands.

Success criteria:

- `ws/git.status` reports branch/head state, clean/dirty status, and changed
  files from a porcelain-safe Git status command.
- `ws/git.diff` returns diff text for an optional revision range and optional
  path filters, with modes for full diff, stat, and name-only when practical.
- `ws/git.log` returns a bounded commit list with hash, subject, author/date,
  and optional body text.
- `ws/git.merge_base` returns the merge base for two revisions.
- All Git subprocess calls use argument arrays and never invoke a shell.
- CLI fallback subcommands mirror the MCP tool surface closely enough for local
  smoke testing.
- Unit tests cover argument construction, output parsing where applicable, and
  error handling.
- Verification covers `go test ./...`, plugin validation, runtime JSON parsing,
  CLI smoke on this repository, and `git diff --check`.

### Phase 2: Runtime metadata and workflow smoke

Expose the new tools through runtime metadata and smoke them against current
workflow needs.

Success criteria:

- `agents-plugin/runtime.json` lists the new `git.status`, `git.diff`,
  `git.log`, and `git.merge_base` tools and CLI commands.
- The installed or local runtime `tools/list` surface includes the new MCP
  tools.
- A local smoke proves that `write-code` style needs can be satisfied: capture
  `HEAD`, inspect a bounded log, inspect a diff range or empty diff, and read
  changed-file status.
- Any remaining gap for `write-code`, `implement`, or `proceed` is documented in
  this ticket before closing or moving to the next child ticket.
