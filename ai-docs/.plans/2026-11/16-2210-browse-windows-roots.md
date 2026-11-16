# Rail: add-workspace "찾아보기" picker cannot leave the home folder on Windows

Owner report (2026-11-16 22:07 KST, verbatim): "워크스페이스 추가 시 "찾아보기" 관련 결함도, 윈도우에서 디스크 루트 탐색을 못하다보니까 반쪽자리 기능이더라고요(홈 경로를 못 벗어남). electron으로 볼 때에도요."

## Established facts (verify)

- The server side already offers the drives. On a real Windows host, `GET /s/host0/fs/roots` returned `{"host":"...","home":"C:\\Users\\<user>","volumes":["C:\\"]}`, and a listing with `root=C:\` answered 200. See `discoverBrowseRoots` and `handleBrowseRoots` in `internal/wsdashboard/local_browse.go`, and `browseRootParam` in `fs_routes.go`.
- The client opens the picker in `rpWsBrowse` (`internal/wsdashboard/assets/dashboard-rail-popovers.js`, around 70-115). It calls `window.WsFiles.create(pane, { rid: "host0", local: true, directoriesOnly: true, roots, location: { root: home, dir: "" } })`. So browsing is rooted at home. The user cannot go above it, and on Windows there is apparently no visible way to switch to `C:\`.
- On macOS and Linux, check whether the same limitation exists. There, `/` is not in `volumes`; only `/Volumes/*`, `/mnt/*`, and `/media/*` children are listed.

## What to build

1. Find out in the `WsFiles` component (the files browser asset that `WsFiles.create` lives in) how `roots.volumes` is used: a root switcher, breadcrumbs, or an up button. Find why on Windows the user cannot reach `C:\` (or any other drive) from home. Report the exact cause in the commit `## AI Context`.
2. Make the picker able to reach the whole local filesystem for a local browse (`local: true`, `host0` only):
   - The breadcrumb or up navigation continues above home to the volume root. Going up from `C:\Users\<user>` reaches `C:\Users`, then `C:\`.
   - Every drive in `volumes` is reachable from a visible control: a drive or volume switcher, or a breadcrumb root menu, whichever fits the existing component.
   - Home stays the starting location and stays one click away.
   - On macOS and Linux, `/` must also be reachable. Add `/` as a root on those OSes in `discoverBrowseRoots`, or treat the filesystem root as the top of the up navigation; pick the smaller change that keeps the security guards intact and say which you chose.
3. Keep every server guard as is:
   - `browseRootParam` refusals: network paths, device paths, non-directories;
   - the `localBrowseViewer` same-origin and local checks;
   - session-scoped browsing (non-`host0` routes stay inside the session project).

   This change widens only the local picker's navigation, not what a remote viewer can read.
4. Electron loads the same page, so no shell change is expected.

## Verification

- Go tests (narrow `-run`) for any `discoverBrowseRoots` / `browseRootParam` change, including the Windows drive case through the existing OS seam.
- Node or Chromium tests (cached Playwright; `PLAYWRIGHT_BROWSERS_PATH=$HOME/Library/Caches/ms-playwright`, `WS_PLAYWRIGHT_MODULE` = the cached npx playwright module) of the picker with a fake roots answer shaped like the Windows one, for example home `C:\Users\u` and volumes `["C:\\","D:\\"]`, served by a stub. Navigate from home up to `C:\`, switch to `D:\`, select a directory there, and check that the path filled into the input is the native absolute path. Also run a POSIX variant that reaches `/`.
- Screenshot (light, 1280px) of the picker showing the drive or root control. Save it to a temp dir outside the repo and list the path. The lead will also check on a real Windows host.

## Common rules

- Go tests: always `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`. Never run the whole `./internal/wsdashboard` or `./internal/mcp` package; use narrow `-run`. The module is `go 1.23`.
- Read the AGENTS.md `### Implementation Conventions` manuals that cover the paths you edit before editing.
- Do not ssh, do not launch Electron or any GUI window, do not restart the shared daemon, do not run `scripts/dev.py rerun`.
- Public repository: no machine-local paths, hostnames, PIDs, or session keys in code, tests, commits, or this plan.
- Every behavior change lands with a test. Commit on your branch only; do not merge. If the work needs a product decision beyond this plan, stop and report.
- Return: commits, the root cause, tests run with results, screenshot path, anything left unfixed.
