# agy: direct-tool allowlist, `call` as the default path

## Goal

A fresh hosted agy lead (Antigravity CLI) lists only 73 of 79 ws tools plus
`call`. agy drops the six LARGEST tool definitions (git_commit, git_merge,
rationale_query, tickets_acquire, tickets_close, wiki_update; 1,441-1,448 B
in agy's cache, next-largest kept is 1,434 B). Before this morning's 1,500 B
diet the cut sat between 1,729 and 1,766 B, so the cut moved down with the
tools: most likely agy keeps a total budget and drops the largest tools first.
More trimming only moves the cut.

Owner decision (199)=a, 2026-11-19 15:44: on agy, list a short allowlist of
tools directly and make `call` the default path for every other ws tool.

## Work

1. Harness descriptor: add a direct-tool allowlist fact on
   `BackendRegistration` next to `CallFallback` (find where agy sets
   `CallFallback`; it is the only harness that does). Only agy sets the list.
   No harness-name branch in common code.
2. tools/list for a connection whose descriptor has the allowlist: list the
   allowlisted tools plus `call`, nothing else. Every other connection is
   unchanged (Claude, Codex, Pi: full list, no `call`).
3. Dispatch is unchanged: a direct call to an unlisted tool by its canonical
   or underscore name still works (old clients and models that remember a
   name must not break), and `call` still lists, describes, and dispatches
   all tools.
4. The hosted manual's tool index (`## Callable ws tools`) marks which tools
   are listed directly and says every other tool is reached through
   `call(tool, arguments)`. Keep the added text short.
5. Allowlist content (the lead's working set; adjust names to the real
   canonical names, drop any that do not exist, and say what you changed):
   workflow_manual, workflow_state, agenda.set, agenda.list, agenda.clear,
   whiteboard.update, whiteboard.query, mailbox.send, mailbox.recv, the
   agent.* spawn/send/interrupt/list tools, playbook.read, playbook.render,
   tickets.query, tickets.move, git.status, git.log, git.commit, git.merge,
   worktree.acquire, worktree.release, note.query, note.write,
   config.resolve_agent, todo.list, todo.check. Put the list in one place
   (the agy descriptor) as plain data. Measure the listed set's total size in
   the guard's measure and report it next to the 79-tool total.

## Constraints

- Read first: `ai-docs/manuals/ws-mcp.md`,
  `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/release-compatibility.md`.
- Compatibility: expected T0 (listing change only for agy connections;
  dispatch unchanged; no stored shape, hello, or argument change). Verify
  and state the tier with source lines in `## AI Context`.
- Simplest change: plain data list plus one filter where tools/list is
  built. No config knob, no new mechanism beyond that.
- NEVER run `go test` or any ws/test executable on this PC (antivirus). This
  applies to any subagent you start too. Build and test on remote w5090 only:
  `D:/sys_lock.sh remote w5090 run agy-direct 20 --sync 'D:/Workspace/Repos/.ws-worktrees/devenv/flyaway-fifteen-judgingly' -- 'export PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && go build ./... && go vet ./internal/mcp/... ./cmd/ws-mcp/... ./internal/wsagent/... && go test ./internal/mcp/... ./cmd/ws-mcp/... ./internal/wsagent/...'`
  If w5090 is unavailable, stop and report.
- Temp files only under `D:/Temp/agy-direct/`, deleted after.

## Verification

Tests pin: agy connection lists exactly allowlist + `call`; a non-agy
connection lists the full set without `call`; a direct call to an unlisted
tool on the agy connection still dispatches; the manual index marks direct
vs via-`call`. One passing w5090 run closes it.

## Done when

- One commit on `impl/track/agent-v2/agy-direct-tools` with the change,
  tests, and this plan file. Message `feat(ws-mcp): ...` with `## AI Context`
  (owner (199)=a, the agy largest-first drop evidence, T0 verdict with source
  lines, final allowlist). No Co-Authored-By trailer.
- Report: commit hash, final allowlist, listed-set size vs full size, w5090
  result, any surprise.
