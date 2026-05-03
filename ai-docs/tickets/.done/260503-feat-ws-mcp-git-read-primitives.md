---
title: ws-mcp Git read primitives
parent: 260503-epic-ws-mcp-vcs-reference-tools
related:
  260503-feat-agents-plugin-runtime-boundary: MCP runtime and launcher baseline
  260503-feat-agents-plugin-write-code-port: first consumer that needs portable start-commit, diff, and log inspection
  260503-epic-ws-mcp-vcs-reference-tools: parent roadmap for portable Git and reference tooling
plans:
  phase-1: 2026-05/03-260503-feat-ws-mcp-git-read-primitives
completed: 2026-05-03
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

### Result (86d1a9c, caa8176) - 2026-05-03

Implemented the first read-only Git primitive slice in `ws-mcp`. The new
`internal/wsgit` package invokes native Git through `exec.Command` argument
arrays and backs both the MCP and CLI surfaces. Added MCP tools `git.status`,
`git.diff`, `git.log`, and `git.merge_base`, plus CLI fallbacks under
`ws-mcp git status`, `ws-mcp git diff`, `ws-mcp git log`, and
`ws-mcp git merge-base`.

The first implementation commit added status parsing from porcelain v2, JSON
diff/log/merge-base result shapes, MCP schema entries, CLI routing, runtime
metadata, and focused tests. Review cycle 1 found a correctness issue: `range`
values for `git.diff` and `git.log` could be option-like strings such as
`--output=...`, which would violate the read-only contract. The fix commit
rejects option-like revision fields before invoking Git and also validates
`git.merge_base` revision fields. Path filters remain allowed after `--`.

The fix commit also expanded coverage for MCP handler calls, invalid input,
runner error propagation, and CLI JSON schema assertions. Verification covered
`go test ./...` from `agents-plugin-tool`, runtime JSON parsing, local CLI
smoke for status/log/diff/merge-base on this repository, local MCP tools/list
smoke showing all four Git tools, installed cache binary smoke after rebuilding
the local runtime, `claude plugin validate agents-plugin`, Windows compile-only
coverage for `cmd/ws-mcp`, and `git diff --check`.

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

### Result (c340c4b, e88fe1e, 86d1a9c, caa8176) - 2026-05-03

Completed the runtime metadata and workflow smoke. `agents-plugin/runtime.json`
now lists `git.status`, `git.diff`, `git.log`, and `git.merge_base` in the MCP
tool surface, plus matching CLI command metadata using `git.merge-base` for the
CLI spelling. A local `tools/list` smoke against `go run ./cmd/ws-mcp serve
--stdio` reports all four Git tools.

Dogfooded the new `ws:write-code` workflow for this implementation. The lead
created the ticket, brief, and plan checkpoints; the named implementer created
the implementation and review-fix commits; partitioned reviewers found and
verified meaningful issues. The workflow also exposed orchestration gaps:
`agents.oneshot` project survey timed out and left a nested Codex/MCP process
that required manual cleanup, `agents.wait` and `agents.status` hit the host
120-second tool-call ceiling even when async work later completed, and two
re-review workers needed manual process cleanup after `agents.cancel` marked
state cancelled but left child Codex processes alive. These are runtime issues
for a follow-up ticket, not blockers for the Git read primitive itself.

The final local smoke proved the intended `write-code` needs: status returns
changed-file state, log returns bounded commit metadata and optional bodies,
diff can inspect the implementation range in `name_only` mode, and merge-base
returns the expected `HEAD` hash for `HEAD`/`HEAD`. The local runtime binary was
rebuilt into `agents-plugin/.runtime/darwin-arm64/ws-mcp` and copied into the
current Codex plugin cache by replacing the old file, not overwriting it in
place. The active MCP process still needs a Codex/MCP restart before this
session's live MCP tool list can serve the new Git tools.
