# Plan: review sweep fixes (a656a34b5..develop)

Branch `delegate/sweep-fixes` (worktree
`/Users/kang-sw/.ws-worktrees/devenv/decree-thinness-strum`), based on develop
`1be57a63e`. Commit here (one commit per logical group is fine); do not merge
or push. Do not run git stash.

## Source

Release review sweep `a656a34b5..develop` (five segments x correctness / fit /
test). Full findings with context:
`/Users/kang-sw/.cache/ws@kang-sw-devenv/proj/17da6bdc/review-paths/aee616eb-01-sweep-a656a34b5-findings.md`.
Fix every item tagged `[fix]`, `(fix)` or `IMPORTANT (fix...)` there; the list
below is the same set. Items tagged `accept` stay as they are. Verify each
finding against the code first; if one is wrong, skip it and say why.

Read before editing: `ai-docs/manuals/shipped-surface-boundary.md` (every
path here ships), `ai-docs/manuals/ws-mcp.md` (internal/mcp and the manual
itself).

## Fixes

Dashboard git (wsdashboard):
1. Important - `gitdrawer.go:80,84,88`, `gitpush.go:113`: machine-parsed git
   reads (`status -z`, `diff --numstat -z`, `WorkingTreeCounts` used by pull's
   dirty check) run through `gitBadgeRunner` with `CombinedOutput`, so stderr
   warnings (autocrlf "LF will be replaced by CRLF", unreadable directory)
   corrupt the parse. Read stdout only; keep stderr for the error text. Test
   with a runner/repo that writes a warning to stderr.
2. `gitpush.go:125-130`: after the fetch re-read, an empty `Upstream` (pruned
   upstream or failed rev-list) reports "already up to date"; re-check and
   refuse/fail properly.
3. `gitbadge.go:126`: the timeout error always says 10s; report the bound of
   the context actually used (60s `gitActionTimeout` for pull/push).
4. `gitpush.go:112` `gitPushErrText`: prefer `! [remote rejected]` /
   `! [rejected]` / `error:` / `fatal:` lines over hook `remote: ...` info
   lines.
5. `worktrees.go:245`: bound the cleanup dump below the inbox 64 KiB cap
   (`wsinbox/inbox.go:94`), ending with a "... N more" line.
6. `assets/dashboard.js:1775` `gpOpen`: do not re-enable Push while a push is
   in flight, and the first push's completion must not close a popover the
   user reopened.
7. `assets/dashboard.js:1734` `gitFail`: also clear `#gref` (branch and
   ahead/behind line).
8. `assets/dashboard.js:1711-1720,1818`: page load runs `trackerStart()` with
   `tkLastRev` null, then the stream's first `tracker` event queues a second
   full reload; load once.

Tracker renderer (wsdashboard/assets/tracker.js; keep the mockup's shared
renderer in step if the file is shared with `ai-docs/ref/tracker-mockup/`):
9. Important - `:805` `ctxTree`: the ancestor walk ignores `hidden` (and, in
   listAll, membership in `base`), so a hidden or kind-filtered ancestor shows
   as a context row. Stop at such an ancestor.
10. `:1799` `mentionPick`: with the caret mid-token (`@ma|ry`) replace the
    whole token, not only the part before the caret.

Go fixes elsewhere:
11. `wstrackerhost/host.go:307`: a failure inside `wsledger.Register` after
    the push succeeded (`remote.go` ~518-529) still says "Nothing was
    registered"; report that the remote holds the store (plan 08-1703 item 3).
12. `wswhiteboard/store.go:285-290` and `view.go:399`: a same-batch edit pair
    restoring the batch-start body must not set `Changed`; `docChanged >=
    answered` must not rank a body unread when answered=0 and Doc.Changed=0.
    Rewrite the garbled comment at `view.go:371-375`. Pin both boundaries in
    tests.
13. `wswhiteboard/nudge.go:94-95`: the nudge must tell the agent `wb:close`
    needs `result`.
14. `wstracker/check.go:258` (`notAssignee` via `pendingRec`): a member holding
    only a rule request gets a refusal saying they have not accepted the
    pending assignment; give a rule-request-specific reason.
15. `wsagent/support.go:231`: cache a failed `Choices` listing briefly so
    each GET does not re-wait the 15 s timeout under `choicesMu`, and a
    request canceled by the browser must not leave the cache unfilled for the
    next caller.
16. `mcp/tracker_tools.go:20-27,:41`: the doc comment and
    `trackerLeadOnlyDenial` text must cover `tracker.show`.

Tests and docs:
17. `wsdashboard/access_test.go:105` `guardedRoutes()`: add
    `GET /s/<id>/worktrees` and `POST /s/<id>/worktrees/cleanup`.
18. `mcp/git_search_test.go`: a hostile repo config test - `diff.external`, a
    textconv driver, `pager.log`, `core.fsmonitor` each pointing at a sentinel
    script; assert none runs.
19. `wsinbox`: a concurrent-claim test (two claimers, exactly one wins, the
    loser gets `ErrGone`).
20. `ai-docs/manuals/ws-mcp.md` (around 185-196, 206, 265-275; check the
    current text first): document the git badge position and click, the git
    drawer and `POST /s/<id>/git/push` / pull rules, `GET /s/<id>/worktrees`
    and `POST /s/<id>/worktrees/cleanup`, snapshot follow refresh, bare
    URL/path autolink, and the tracker view answer's `html` field. Match the
    code, terse.
21. `CHANGELOG.md:36-43`: the Unreleased Changed entries still say
    `route.resolve_proceed` is retained and `resolve_implement` names
    `implementer-relay`, contradicting Removed; fix.
22. `wsdashboard/pageview.go:75`: move `ThemedBoard` so `Mode` keeps its doc
    comment.

## Constraints

- i18n: new user-facing strings go to `en.json` and `ko.json`, and also to
  the other nine tables (the release ships now; keep every table at the full
  en key set; machine translation matching each table's tone is fine).
- Keep the whiteboard embed size budget test green.
- If `agents-plugin/rsrc` or manuals mirrored into rsrc change, regenerate:
  from `agents-plugin-tool`, `WSRSRC_REGEN=1 go test ./internal/wsrsrc/...
  -count=1 -run TestGenerateRealManifest`, `WS_REGEN_WSFLOW_RSRC=1 go test
  ./internal/wsrsrc -count=1 -run TestRegenerateWsflowRsrcMirror`, then
  `cp agents-plugin/rsrc/manifest.json agents-plugin-pi/rsrc/manifest.json`.

## Verification

From `agents-plugin-tool`: `gofmt -l .`, `go vet ./...`,
`go test -count=1 ./...` (report any failure with output, and say whether it
is pre-existing/flaky by re-running it alone), `node --check` on
`dashboard.js` and `tracker.js`, `GOOS=windows go vet` on touched packages.

## Return

Commit hashes, per-item result (fixed / skipped with reason), changed files,
test output summary, anything left open.
