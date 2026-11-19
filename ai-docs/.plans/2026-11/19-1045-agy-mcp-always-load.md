# agy: mark ws tools always-loaded so large schemas are not lazy-dropped

## Goal

Dogfood fix for `ai-docs/tickets/ready/261118-feat-ws-agy-tool-names-and-worker-role.md`
on `track/agent-v2` (worktree
`D:\Workspace\Repos\.ws-dashboard-worktrees\devenv\261118-0937`).

A hosted agy lead (agy 1.5.12, ACP) sees only 70 of the 79 ws tools. The 9
missing ones are exactly the 9 largest serialized tools (cutoff between 1729
and 1766 bytes): tickets_query, git_commit, git_merge, note_write, note_query,
wiki_update, agent_spawn, rationale_query, dashboard_intake. agy classifies
them as lazy (`mcp.SplitEnabledToolsByEagerLazy`), but the hosted session has
neither a `Lazy:` list nor a `call_mcp_tool` tool, so they are unreachable.
agy's changelog documents the server-side override: an MCP server marks a tool
always loaded by setting `_meta: {"antigravity/alwaysLoad": true}` on it in
its `tools/list` response.

## Change

On agy hosted connections only, add `"_meta": {"antigravity/alwaysLoad": true}`
to every tool definition in the `tools/list` response (merge with any existing
`_meta`; ws sets none today).

- Drive it from the harness descriptor on `BackendRegistration`
  (`wsagent/registration.go`), the same way `ToolNameScheme` reaches the
  connection (decision (136): no new agy-name branch in common code). A
  boolean descriptor field (for example `ToolsAlwaysLoad`) that agy declares
  is the expected shape; pick the simplest one that fits the existing
  plumbing of `ToolNameScheme` into `filteredTools`/`publicToolDefinition`
  (`internal/mcp/server_inventory.go`).
- Claude, Codex and Pi connections: `tools/list` output byte-identical to
  today. Do not add `_meta` for them.
- Do not shrink or otherwise change schemas. The 2,000 B listed-tool guard
  measures what it measures today; if `_meta` is added after the guard or the
  guard test measures the base definition, keep it that way and say so.

Compatibility: T0. `_meta` is an MCP-spec optional field on Tool; older
daemons simply do not send it and agy keeps its current behavior.

## Constraints

- Read before editing: `ai-docs/manuals/ws-mcp.md`,
  `release-compatibility.md`, `performance.md`, `shipped-surface-boundary.md`.
- No local `go test` and no ws executables on this PC (antivirus). Build and
  test on remote w5090 only:
  `D:/sys_lock.sh remote w5090 run agy-alwaysload <min> --sync 'D:/Workspace/Repos/.ws-dashboard-worktrees/devenv/261118-0937' -- 'export PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && go build ./... && go vet ./internal/mcp/ ./internal/wsagent/ && go test ./internal/mcp/ ./internal/wsagent/'`.
  If sys_lock reports w5090 unavailable, stop and report; do not fall back to
  a local build or test.
- Temp files only under `D:/Temp/agy-alwaysload/`, deleted after.
- Surgical; simplest complete change; no new abstraction beyond the
  descriptor field.

## Verification

Unit tests pin:
- an agy-scheme connection's `tools/list`: every tool carries
  `_meta["antigravity/alwaysLoad"] == true`;
- a Claude (default) connection's `tools/list`: no `_meta` on any tool.
w5090 `go build ./...`, `go vet`, `go test` of the two packages pass.

## Done when

- Commit 1 on `track/agent-v2`: the change, tests, and this plan file.
  Message carries `## AI Context` (root cause, why `_meta` alwaysLoad over
  shrinking schemas, tier T0) and the ticket stem
  `261118-feat-ws-agy-tool-names-and-worker-role` under `## Ticket Updates`.
- Commit 2: append `#### Edition (<commit-1 short hash>) - 2026-11-19` at the
  end of the ticket's Phase 1 section (after its Result and existing
  Editions), two or three lines stating the fix.
- No Co-Authored-By trailer on either. Stage only your files: the worktree
  has an unrelated uncommitted edit of
  `ai-docs/tickets/todo/261118-feat-ws-display-transcript-memory-first.md`;
  leave it untouched and unstaged.
- Report: commit hash, files, w5090 results, any surprise.
