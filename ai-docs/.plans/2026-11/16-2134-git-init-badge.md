# Rail: "git init" button on a non-Git folder bar, with a confirm modal

Owner request (2026-11-16 21:25 KST, verbatim): "일단 지금은 급한대로 git 폴더 아닌 워크트리 뱃지에 "git init하기" 같은 아이콘을 하나 추가하는 게 어떨까 싶습니다. 누르면 모달 뜨고, "이 폴더를 git 폴더로 만들까요?" 뭐 이렇게 ..."

The owner approved the mockup as drawn ((1191)=ok). It is the visual contract:
`ai-docs/assets/mockups/261116/git-init-badge/mockup.html` (with `mockup-light.png` and `mockup-dark.png`). It is already copied into this worktree, untracked; commit it together with the implementation. Read it first: its comments name the real CSS and markup sources it copied (rail CSS, the worktree-delete modal `dialog.rdeldlg` and `rdelMake`, the `r-rail` template, the sprites).

This is a stopgap before the backlog ticket `261116-feat-ws-session-outside-git-folder` (do not implement that ticket).

## What to build

1. **Button.** On a rail workspace bar for a plain folder (not a Git repository; today that bar shows the folder icon and no new-session/add-worktree button, see `RailRow.CanSpawn` in `internal/wsdashboard/pageview_rail.go`), add a `git init` icon button in the slot where a repository bar shows its add-worktree `+`.
   - Icon: the new sprite symbol `i-gitinit` from the mockup, which is the rail's branch glyph plus a small `+`. Add it to `internal/wsdashboard/assets/sprite.svg`.
   - Style: the same 18px slot and 13px glyph as the other rail buttons. Unlike the other `+` buttons it stays visible at rest at reduced opacity (.6) and is full opacity on hover and focus.
   - Visibility: shown only where the new-session and delete buttons are shown, that is, in a browser on the daemon's machine (the existing `html[data-sysopen]` gate).
   - Tooltip and aria-label: "git init하기".
2. **Modal.** Clicking the button opens a confirm modal that reuses the worktree-delete modal frame (`dialog.rdeldlg`, `.pbtn`), recolored blue because nothing is destroyed.
   - Title (with icon): "이 폴더를 git 폴더로 만들까요?"
   - Next line: the folder's full path.
   - Body: "이 폴더에서 git init을 실행합니다. 파일은 커밋하지 않으며, 끝나면 새 세션 버튼이 생깁니다."
   - Buttons: 취소 (reuse the existing cancel string) and `git init` (primary).
   - Errors show inside the modal, in the delete modal's refusal-note box. For example, git not found: "git을 찾을 수 없습니다. git을 설치한 뒤 다시 시도하세요." Other failures show their reason.
3. **Daemon route.** Add a POST route that runs `git init` in that folder. Run exactly `git init` with no other arguments; do not commit, add, or configure anything. Guards:
   - Accept only a folder that the dashboard already lists as a plain-folder workspace. Never accept an arbitrary path from the request body, so the route cannot init random paths.
   - Refuse when the folder is already inside a Git worktree.
   - Guard the route the same way as the existing local-only, state-changing rail routes (worktree delete and new session): same-origin and local checks.
4. **After success.** Show a toast: "<name>을(를) git 폴더로 만들었습니다" (follow the existing `rail.wt.deleted` 을(를) form). The rail refreshes, so the bar now shows the repository icon, the root worktree row (`main · 루트` or whatever branch git created), and the normal new-session button.
5. **Empty-repository check.** Right after `git init` there are no commits. Verify, with a test, that the rail's worktree list and the session-start (spawn) eligibility accept a repository with an unborn HEAD. If either refuses or crashes, fix only that path and say so in the commit's `## AI Context`.
6. **i18n.** Add every new string to every locale table under `internal/wsi18n`; all tables keep the same keys. Korean is the source text above; write proper translations for the other locales.

## Verification

- Go tests (narrow `-run`) for the route:
  - success on a listed plain folder in a temp dir;
  - refusal for an unlisted path;
  - refusal for a folder already inside a repository;
  - the git-not-found error mapping.
- Node or Go tests for the rail markup:
  - the button appears only on non-Git bars under the sysopen gate;
  - the modal strings;
  - the unborn-HEAD rail and spawn-eligibility case.
- i18n key-parity test.
- Screenshots: render the real dashboard pages or templates with headless Chromium (cached Playwright on this machine), light and dark, 1280 px wide. Capture:
  - a non-Git bar with the button at rest;
  - the open modal;
  - the modal error state if reachable with a fake;
  - the after state.

  Save them to a temp directory outside the repo and list the paths in your reply; do not commit them. The lead compares them side by side with the mockup PNGs before merge, so match the mockup.

## Common rules

- Go tests: always `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`. Never run the whole `./internal/wsdashboard` or `./internal/mcp` package; use narrow `-run`. Node is available for the Node-harness tests.
- Read the AGENTS.md `### Implementation Conventions` manuals that cover the paths you edit before editing.
- Do not touch `agents-plugin/rsrc/whiteboard/whiteboard.md` or its wsflow mirror.
- Do not ssh anywhere, do not launch Electron or any GUI window, do not restart or replace the shared dashboard daemon, do not run `scripts/dev.py rerun`.
- A new dashboard HTTP route is additive; state its compatibility tier (expected T0) in the commit `## AI Context`. Stop and report before anything above T2.
- The repository is public: no machine-local paths, hostnames, PIDs, or session keys in code, tests, commits, or this plan.
- Every behavior change lands with a test.
- Commit on your branch only; do not merge. If the work needs a product decision beyond this plan, stop and report instead of expanding.
- Return: commits, tests run with results, screenshot paths, anything left unfixed and why.
