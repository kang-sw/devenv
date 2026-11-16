# CI: regressions after the four feature merges (panel diff, topic-state pills, select-all, browse roots)

The CI run on develop `3ea9f8e9d` (Go 1.23, `GOTOOLCHAIN=local`) shows failures that were green at `ab2e32876`. These merges landed in between:
- `f9c9e858f`: panel diff gutter.
- `6abc5bc49`: workspace picker roots.
- `afe6c7aab`: select-all scope. It includes a union conflict resolution in `internal/wswhiteboard/render.go`.
- `76705eb1a`: compact topic-state pills.
- Test-only fixes `d0fb7a810` and `3ea9f8e9d`.

The feature workers ran only narrow tests, so these slipped through.

```
internal/mcp (ubuntu; the first and third also on windows mcp-2):
--- FAIL: TestDashboardTopicTicketMoveInvalidatesTheBoard
    dashboard_fingerprint_test.go:233: the board lacks "<span class=\"tname\">t</span>"
    dashboard_fingerprint_test.go:257: the board lacks "<span class=\"tname\">t</span>"
--- FAIL: TestDashboardNudgeFailureLeavesTheToolResultUnchanged/ok
    dashboard_sessions_test.go:306: nudge project = "/tmp/...no_daemon.../004", want "/tmp/...ok.../004"
--- FAIL: TestWhiteboardFilePageResolvesTopicTickets
    whiteboard_panels_test.go:468: the file page lacks "<span class=\"tname\">t</span>"

internal/wsdashboard (ubuntu):
--- FAIL: TestCSSClassesDoNotCollideAcrossModules
    css_collision_test.go:324: class defined in two module stylesheets: .d in tracker, whiteboard ... (rename the newer class with its module's prefix (dashboard d-, memo memo-, tracker tk-, whiteboard wb), or allowlist it with a reason)
    same for: .gone (dashboard, whiteboard), .over (dashboard, whiteboard), .sb (dashboard, whiteboard), .t (dashboard, whiteboard), .wait (dashboard, whiteboard), .who (dashboard, whiteboard)
--- FAIL: TestDependencyPopoverCSSScope
    dependency_popover_test.go:26: popover selectors must all be scoped by .dw, got globals: map[confirm discuss dw ext h i impl ov over research sb-tip tl wait who]
--- FAIL: TestPagesCarryTheTracker
    pages_test.go:395: session frame must reference the assembled dashboard script, not inline it
    pages_test.go:395: index must inline the assembled dashboard script exactly once, without an external reference
--- FAIL: TestPagesCarryTheToastStackAndTitleHost
    toast_test.go:316: session frame must reference the assembled dashboard script, not inline it
    toast_test.go:316: index must inline the assembled dashboard script exactly once, without an external reference
```

## What to do

1. Reproduce each failure locally with a narrow `-run`. Find the merge that caused it with `git log` or `git bisect` across `ab2e32876..3ea9f8e9d`, and name that merge in the commit's `## AI Context`.
2. Decide which side is right, test or product, using the originating plan in `ai-docs/.plans/2026-11/`:
   - `16-2149-panel-diff-gutter.md`
   - `16-2200-topic-state-compact.md`
   - `16-2208-select-all-scope.md`
   - `16-2210-browse-windows-roots.md`

   Then handle each failure:
   - **CSS collisions and popover scope.** These are product defects in newly added CSS.
     - Fix them by renaming the new whiteboard-side classes with the module prefix, or by scoping them under their container, as the tests require. Do not allowlist them.
     - Keep the visual result identical. The topic-state pills were checked against the mockup in `ai-docs/assets/mockups/261116/topic-state-compact/`, so change only selectors and class names, never values.
     - Update every JS and template reference to a renamed class.
   - **Missing `tname` span.** The topic-state header markup most likely changed.
     - If the plan intended the change, update the expectations to the new markup. Keep the assertion's intent: the board shows the ticket-linked topic's name.
     - Otherwise, restore the markup.
   - **Session frame inlines the assembled dashboard script.** The likely cause is the select-all asset or the render.go union resolution.
     - The rule: the session frame references the assembled `dashboard-<hash>.js` and does not inline it. The index inlines it exactly once.
     - Fix the product so the rule holds again. For example, add the new select-all script to the assembled bundle or put it in the right template slot, instead of inlining it a second time. Explain what changed.
   - **Nudge project mismatch** (`TestDashboardNudgeFailureLeavesTheToolResultUnchanged/ok`). The subtest gets the previous subtest's project, which points to state shared across subtests, such as a package-level nudge hook or cache that is never reset.
     - Determine whether it is new in this range, or a latent ordering or flake issue the range only exposed.
     - Fix it with proper isolation, such as a `t.Cleanup` reset, and run it with `-count=20`.
3. Keep every test's intent. Do not delete assertions.

## Verification

Run every command below with `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`.

- `go test ./internal/mcp -run 'TestDashboardTopicTicketMove|TestDashboardNudgeFailure|TestWhiteboardFilePage|TestWhiteboardPanels|TestDashboardFingerprint' -count=3`
- `go test ./internal/wsdashboard -run 'TestCSS|TestDependencyPopover|TestPages|TestToast|TestDashboardFrame|TestWhiteboard' -count=1`
- `go test ./internal/wswhiteboard -count=1`. Running the full package is allowed.
- If CSS or classes changed, render the whiteboard topic-state header and the dependency popover with the cached Playwright Chromium:
  - Set `PLAYWRIGHT_BROWSERS_PATH=$HOME/Library/Caches/ms-playwright`, and set `WS_PLAYWRIGHT_MODULE` to the cached npx playwright module.
  - Compare the renders against the mockup.
  - Save the screenshots outside the repo and list their paths.
- Grep the `internal/wsdashboard` and `internal/mcp` test sources for other assertions about the changed markup, class names or script inlining, and run those tests by name.

## Common rules

- The module is `go 1.23`: use no newer standard-library APIs.
- Never run the whole `./internal/wsdashboard` or `./internal/mcp` package; use a narrow `-run`.
- Before editing, read the AGENTS.md `### Implementation Conventions` manuals that cover the paths you edit.
- Do not touch `agents-plugin/rsrc/whiteboard/whiteboard.md` or its wsflow mirror.
- Do not ssh, launch a GUI or Electron, restart the shared daemon, or run `scripts/dev.py rerun`.
- This repository is public: keep machine-local paths, hostnames, PIDs and session keys out of commits.
- Make one commit per root cause, each with `## AI Context`. Commit on your branch only, and do not merge.
- If a fix needs a product decision, stop and report.
- Return:
  - the commits;
  - the cause and the originating merge of each failure;
  - the tests run, with results;
  - the screenshot paths.
