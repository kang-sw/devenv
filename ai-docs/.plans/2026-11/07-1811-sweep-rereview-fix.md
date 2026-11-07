# Plan: review-sweep re-review repairs (F1-F5)

## Outcome

The fix-verification re-review of the sweep range
9768d4607d2a38f22aa3f91f87ac37cb7fd677b6..ba2ee07df returned NEEDS FIX. Fix the
five items F1-F5 in the findings file
`/Users/kang-sw/.cache/ws@kang-sw-devenv/proj/17da6bdc/review-paths/109fe801-01-findings.md`
(read it; it gives file:line, the defect, and the expected fix for each):

- F1: ws-mcp.md + wsdashboard/access.go comment state the m2f changes (LAN
  mermaid asset exemption, Now sandboxed srcdoc frame, Now image data: URLs up
  to 2 MiB with route fallback).
- F2: the three `*dashboard_enabled` config manifest descriptions list lead
  compactions.
- F3: wsgit status counts pass `--untracked-files=normal`, with tests for
  `status.showUntrackedFiles=all` and `=no`.
- F4: dashboard drawer stays attached under the header on the scrolling
  board-first page (open-then-scroll and scroll-then-open).
- F5: the sanitizer's footnote id allowance requires the current text's prefix,
  with a foreign-prefix test.

The "Accepted as follow-ups" section of the findings file is out of scope.

## Settled constraints

- Manuals that apply: `ai-docs/manuals/ws-mcp.md` (internal/mcp),
  `ai-docs/manuals/shipped-surface-boundary.md` (all agents-plugin* paths),
  `ai-docs/manuals/wsflow-mirroring.md` (agents-plugin and wsflow manifests).
- ws and wsflow manifest descriptions stay identical apart from `default`.
- No protocol or behaviour change beyond the five fixes. One commit per logical
  unit is fine; docs may share a commit.

## Boundary

Commit on branch `delegate/sweep-rereview-fix` only; do not merge or push.
Return to the lead if a fix needs a design decision beyond the findings file.

## Verification

In agents-plugin-tool: `go build ./... && go vet ./internal/... ./cmd/...`;
`go test -count=1 ./internal/wsgit/ ./internal/wswhiteboard/ ./internal/wsdashboard/ ./internal/mcp/ ./internal/wsrsrc/`.
`node --check` on dashboard.js. Headless Chromium (cached Playwright under
~/.npm/_npx/*/node_modules/playwright) for F4: board-first served page with
enough content to scroll, open at top then scroll, and scroll then open; light
and dark; 1400 and 420px; the session page drawer still behaves as before.
Report what was not checked.
