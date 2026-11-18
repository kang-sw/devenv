# Sync develop into track/agent-v2 before the daemon swap

## Goal

Complete the in-progress `git merge --no-ff develop` on `track/agent-v2` in
worktree `D:\Workspace\Repos\.ws-dashboard-worktrees\devenv\261118-0937`.
Source develop = 09c9b8d21, target = c01390dff (track with hosted prompt
assembly Phase 1 merged). `wsflow/git.merge` left the merge in progress with
conflicts.

## Intent of each side

- develop: dashboard (drawer, board, frontend JS) and doc work by the main
  session since the last sync (7d19dce8e), plus anything else it landed.
- track/agent-v2: harness catalog, agy adapter, agent.* tools, mailbox
  memory-first store, hosted prompt assembly Phase 1 (daemon-delivered lead
  key and workflow manual, plugin-less hosted Claude, hosted session_key rule).

Resolve keeping both intents. Where both sides changed the same text (likely
`ai-docs/manuals/ws-mcp.md`, i18n catalogs, generated or mirrored resources),
the result carries both sides' content; regenerate generated files with the
repository's own generator rather than hand-merging them when one exists.

## Constraints

- No local `go test` and no running ws exes on this PC (antivirus). Build and
  test Go on remote w5090 only: `D:/sys_lock.sh remote w5090 run <who> <min>
  --sync 'D:/Workspace/Repos/.ws-dashboard-worktrees/devenv/261118-0937' --
  'export PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && go build ./...
  && go vet <touched packages> && go test <touched packages>'`. w5090 has no
  Node; if a conflict touches dashboard JS with tests, run only a syntax-level
  check locally (for example `node --check`) and report that the JS tests were
  not run.
- Temp files only under `D:/Temp/sync-develop-pre-swap/`, deleted after use.
- Do not change behavior beyond conflict resolution. If a conflict needs a
  design decision (both behaviors cannot coexist), stop and report instead of
  choosing.
- Do not use `git stash`.

## Done when

- Conflicts resolved, w5090 build plus tests of the touched Go packages pass
  (a test failing on both parents unchanged may be noted, not fixed).
- Merge commit completed on track/agent-v2 with an `## AI Context` section
  listing each conflicted file and how it was resolved; this plan file is
  included in that commit.
