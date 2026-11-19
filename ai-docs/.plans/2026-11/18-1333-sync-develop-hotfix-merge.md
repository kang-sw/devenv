# Sync develop into track/agent-v2 (hosted-lead key hotfix)

## Goal

Complete the in-progress `git merge --no-ff develop` on `track/agent-v2` in
worktree `D:\Workspace\Repos\.ws-dashboard-worktrees\devenv\261118-0937`.
Source develop = 7d19dce8e (includes hotfix merge 5c7797981 / impl fff872331),
target = 3d1116bf7. `wsflow/git.merge` left the merge in progress with conflicts.

## Intent of each side

- develop: hosted-lead key hotfix. `startLead` (non-attach, Claude backend)
  calls `composeLeadInstructions(c.dir, rec)`, writes `<conv dir>/instructions.md`
  (writeFileAtomic, 0600) into `StartSpec.Instructions`; `claudeLeadArgs` adds
  `--append-system-prompt-file <path>` when Instructions is non-empty. Tests in
  lead_claude_test.go / claude_lead_plugins_test.go / lead_runner_test.go
  (TestClaudeLeadPromptCarriesHostedKey). Plus any other develop fixes.
- track/agent-v2: harness capability catalog (85ef24622) and hosted
  adapter work (agy adapter, lead plugin bundle, etc.).

Resolve keeping both intents. Where both sides changed the same function
(likely `claudeLeadArgs`, `startLead`, or their tests), the result must carry
the track's behavior AND the hotfix's instructions-file flag and tests.

## Constraints

- No local `go test` / no running ws exes on this PC (AV). Build and test on
  remote w5090 only: `D:/sys_lock.sh remote w5090 run <who> <min> --sync
  '<this worktree path>' -- 'export PATH=$HOME/sdk/go/bin:$PATH; cd
  agents-plugin-tool && go build ./... && go test ./internal/wsagent/...
  ./internal/mcp/...'` (adjust packages to those the conflicts touch; run
  `go vet` on them too).
- Temp files only under `D:/Temp/sync-develop-merge/`, deleted after use.
- Do not change behavior beyond conflict resolution. If a conflict needs a
  design decision (both sides' behaviors cannot coexist), stop and report
  instead of choosing.

## Done when

- Conflicts resolved, w5090 build + relevant tests pass (a test failing on
  both parents unchanged may be noted, not fixed).
- Merge commit completed on track/agent-v2 with an `## AI Context` section
  listing each conflicted file and how it was resolved; this plan file is
  included in that commit.
