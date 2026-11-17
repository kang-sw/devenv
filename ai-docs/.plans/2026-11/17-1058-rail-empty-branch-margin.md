# Dashboard Sessions rail: tighten spacing around branches with no sessions

## Context

Owner 2026-11-17 10:58, with a screenshot
(`C:\Users\user\.cache\ws@kang-sw-devenv\dashboard\sessions\renewaldevoteetranspirefencing\inbox\attachments\466c9dacc5b7e6a4-1.png`):

- In the Sessions rail, the spacing between branch groups that have sessions
  looks fine.
- A branch row with no sessions has too much space above and below it. In the
  screenshot this is `master · 루트` under the InspectTGV_AIDriven workspace,
  between the workspace header and the `strata` branch.
- The extra space looks awkward.

## Outcome

1. A branch group with no session rows takes only about the height of its
   branch row plus the normal gap between branch groups.
   - The gap above it (after the workspace header or the previous group) and
     below it (before the next branch) matches the gaps around branches that
     have sessions.
2. Branches with sessions, workspace headers, the "보관 / 종료됨" footer rows
   and the "거부된 워크트리" row keep their current spacing.
3. The change is CSS only, or the smallest markup change if the empty space
   comes from an empty container that always renders.
   - Find the actual source first: padding or margin on the group, an empty
     list element, a min-height, or a spacer.

## Constraints

- Read `ai-docs/manuals/shipped-surface-boundary.md` before editing
  `agents-plugin-tool/`.
- The rail lives in `agents-plugin-tool/internal/wsdashboard/assets/`
  (`dashboard.tmpl` `r-rail`, `dashboard-rail.css`, `dashboard-rail.js`).
- A parallel worker is adding a moon button to the Sessions panel header in
  the same files (branch impl/develop/session-sleep-button). Keep this change
  to the branch-group spacing rules; do not touch the header.
- NO local `go test`, Go test exes, ws/ws-mcp executables, or launcher on this
  PC (owner AV rule).
  - Do not start the ws daemon or Electron.
  - Go tests run on remote w5090 only, and only if you change a template or
    Go code:
    `D:/sys_lock.sh remote w5090 run <who> <min> --sync 'D:/Workspace/Repos/.ws-worktrees/devenv/manifesto-charm-obtrusive' -- 'export PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && go test ./internal/wsdashboard'`.
- LOOK:
  - Render the real rail markup with the real CSS in local headless Chrome,
    using a static fixture or the existing test fixture approach.
  - Include one workspace with an empty branch followed by a branch with
    sessions, like the screenshot.
  - Capture before and after, compare them with the owner's screenshot, and
    fix the result before committing.
- Temp files only under `D:/Temp/rail-empty-branch-margin/`. Keep the two PNGs
  for the lead and delete everything else.
- Commit with `## AI Context` and the Co-Authored-By trailer naming your
  model. Do not push.

## Verification

- Before and after screenshots show the empty branch's gaps matching the
  others.
- `go test ./internal/wsdashboard` on w5090, if markup or Go changed.
- Report:
  - the commit;
  - the files changed;
  - the cause of the extra space;
  - the screenshot paths.
