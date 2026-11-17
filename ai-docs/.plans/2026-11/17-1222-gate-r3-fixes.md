# Release-gate r3 fixes: dead wb.result i18n key, wb:close retry wording

## Context

The gate r3 review of `07b571a59..0306873fe` returned NEEDS FIX. Findings:
`C:\Users\user\.cache\ws@kang-sw-devenv\proj\de04b4ac\review-paths\ad8ee015-01-gate-r3.md`.
This blocks the provisional v0.50.3 release (owner (1287)=a).

## Outcome

1. **I1: unused i18n key.**
   - f6ba4e087 removed the only use of `{{t "wb.result"}}` from
     `wswhiteboard/assets/page.tmpl`, so `TestEveryEnglishKeyIsUsed`
     (`internal/wsi18n/keys_test.go:103-119`) fails.
   - Remove `"wb.result"` from all 11 `internal/wsi18n/tables/*.json`.
   - If something else still needs it (grep JS too), keep it and say why.
2. **M1: retry wording.**
   - `agents-plugin/rsrc/whiteboard/whiteboard.md:279-281` and its wsflow and
     pi copies tell the lead to retry the whole batch with `result=` when an
     older daemon refuses `<wb:close/>`.
   - The daemon applies commands one by one (`store.go:615-617`), and its
     response says to resend only the commands inside `<wb:failed>`
     (`mcp/whiteboard_tools.go:354-358`). A whole-batch retry therefore
     duplicates `wb:log`/`wb:add`.
   - Change the sentence so the lead resends only the refused `wb:close`,
     adding `result="<one sentence>"`.
   - Regenerate the mirrors and manifests with the repo's usual scripts; do
     not hand-edit generated files.

## Constraints

- Read before editing:
  - `ai-docs/manuals/shipped-surface-boundary.md`;
  - `ai-docs/manuals/skill-authoring.md`;
  - `ai-docs/manuals/wsflow-mirroring.md`.
- Editing the wb:close sentence in whiteboard.md is authorized; touch nothing
  else in that file.
- Other findings (M2 and M3) are out of scope.
- NO local `go test`, Go test executables, ws/ws-mcp executables or the
  launcher on this PC (owner AV rule). Do not start the daemon or Electron.
  - Go tests run on w5090 only:
    `D:/sys_lock.sh remote w5090 run gate-r3-fix 20 --sync 'D:/Workspace/Repos/.ws-worktrees/devenv/groove-animosity-playroom' -- 'export PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && go test ./internal/wsi18n ./internal/wswhiteboard ./internal/mcp ./internal/rsrc/...'`.
  - If `./internal/rsrc/...` is not the resource-mirror test package, use the
    right one.
  - Accepted reds: only TestTopicStateBadgeResearchingMetadata and
    TestTopicStateBadgeClockAndReconciliation.
- Run the local wsflow/mirror Node tests that cover the playbook copies, if
  any.
- Temp files only under `D:/Temp/gate-r3-fixes/`, deleted afterwards.
- Commit with no Co-Authored-By trailer. Do not push.

## Verification

- `go test ./internal/wsi18n` passes on w5090, along with the other listed
  packages (accepted reds only).
- The playbook and its mirrors are byte-identical where the mirror tests
  require it.
- Report the commit hash and the test results.
