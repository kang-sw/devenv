# Listed ws tool size guard: 2,000 B -> 1,500 B

## Goal

Hosted agy (Antigravity CLI 1.5.12) silently drops every ws tool whose
definition is larger than about 1.75 KB, so an agy lead sees 70 of 79 ws
tools. Owner answer (162)=a, 2026-11-19: lower the listed-tool size guard for
ALL connections from 2,000 B to 1,500 B and trim every tool over it. This is a
static diet; there is no per-connection or agy-only schema.

Evidence (agy's own schema cache, `C:\Users\user\.gemini\antigravity-cli\mcp\ws\<tool>.json`,
file size in bytes): dropped = tickets_query 1766, dashboard_intake 1800,
agent_spawn 1813, note_write 1833, git_merge 1848, note_query 1866,
rationale_query 1893, wiki_update 1925, git_commit 1983. Largest kept =
worktree_release 1729. agy's file size is a little larger than the guard's
measure, so 1,500 B by the guard leaves margin.

## Work

1. `agents-plugin-tool/internal/mcp/agent_tool_size_test.go` (~lines 34-65):
   the guard measures name + description + inputSchema as compact JSON per
   listed tool. Change the limit from 2000 to 1500. Update the comment there
   to say why: agy drops tool definitions above about 1.75 KB.
2. Run the test (on w5090) to get the full list of tools over 1,500 B. It can
   be more than the 9 above. Trim each one until it passes:
   - shorten the tool description and argument descriptions first; keep the
     meaning a caller needs, drop repetition and examples;
   - schema keywords `default`, `minimum`, `maximum` may be removed from the
     schema when the handler already applies the same default/limit in code
     (verify in the handler; if the handler relies on the schema, keep the
     keyword or move the fact into a short description phrase);
   - do NOT remove, rename, or retype any argument, and do not change which
     arguments are required. Behavior must stay identical.
3. If a description is also checked by another test (golden text, catalog,
   doc drift), update that test or file consistently.

## Constraints

- Read first: `ai-docs/manuals/ws-mcp.md`,
  `ai-docs/manuals/shipped-surface-boundary.md` (tool descriptions are text
  shipped to agents), `ai-docs/manuals/release-compatibility.md`.
- Compatibility: T0 (descriptions and advisory schema keywords only; no
  argument added, removed, or retyped). State this in `## AI Context`.
- No local `go test` and no ws executables on this PC (antivirus). Build and
  test on remote w5090 only:
  `D:/sys_lock.sh remote w5090 run tool-size 20 --sync 'D:/Workspace/Repos/.ws-worktrees/devenv/groove-animosity-playroom' -- 'export PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && go build ./... && go vet ./internal/mcp/... ./cmd/ws-mcp/... && go test ./internal/mcp/... ./cmd/ws-mcp/...'`
  If any other package's test reads tool descriptions, run it too. If w5090 is
  unavailable, stop and report; never fall back to a local run.
- Temp files only under `D:/Temp/tool-size/`, deleted after.
- Simplest change; no new mechanism, no per-harness branch.

## Done when

- One commit on `impl/track/agent-v2/tool-size-1500` with the guard change,
  the trims, and this plan file. Message: `fix(ws-mcp): ...` with
  `## AI Context` (owner (162)=a, agy cutoff evidence, T0, which tools were
  trimmed and how). No Co-Authored-By trailer.
- Report: commit hash, the before/after size of each trimmed tool (guard
  measure), any keyword you kept because the handler relied on it, w5090
  results, any surprise.
