# Fix: file browser find/list syscall storm from per-candidate deny checks

## Context

The owner reports that dashboard file search is close to unusable and that
opening a single folder takes 2-3 s. The owner's PC runs AhnLab V3, so every
syscall costs 10-100x.

Measured at 15:46 on the old daemon. Artifacts are in `D:/Temp/search-slow/`:
`cpu20.pprof`, `top.txt`, `goroutine2.txt`.

- 20 s CPU profile: 1651% of a core, 99.4% of it inside `runtime.cgocall`
  (Windows syscalls).
  - `handleFSFind`: 83.6%.
  - `alwaysDenied`: 75%.
  - `deniedFileIdentity`: 67%.
  - `deniedBaseIdentities`: 31%.
  - `os.Stat`/GetFileAttributesEx: 53%.
  - `os.SameFile` createFile: 15%.
  - `deniedBasePaths` → `CacheRoot` → `EvalSymlinks`: 16%.
- 15 goroutines in `handleFSFind` were runnable at once, all with the stack
  `viewerCandidates → localPath → alwaysDenied → deniedBaseIdentities / sameDeniedFile`.
  Overlapping keystrokes pile up, and nothing cancels a stale query.
- `/fs/list` took 2.0-3.0 s per request, against 1 ms for `/health`.
- Locations, all in `agents-plugin-tool/internal/wsdashboard/`:
  - `fs_search.go`: `handleFSFind` and `/fs/search`. `browseCandidates` reads
    the cached `git ls-files` list from `files_route.go`, about 5.3k files.
  - `fs_local.go:106`: `viewerCandidates` calls `localPath` (`fs_local.go:24`)
    once per candidate.
  - `fs_outside.go:180`: `alwaysDenied`.
  - `fs_denied.go:110`: `deniedFileIdentity`, which has a "No cache" comment.
    It runs `EvalSymlinks`, stats about 30-45 denied bases, and runs
    `os.Stat` + `os.SameFile` on every ancestor directory.
  - The estimate is 100-200 syscalls per candidate, so one keystroke costs
    hundreds of thousands of syscalls.
- Regression origin, all from today: `aa625d106` (00:02, local-viewer
  candidates), `96851381f` (00:23, "harden local file reads after security
  review", which introduced `deniedFileIdentity`), and `63a99affd`. Read their
  commit messages and tests to recover the security intent. The intent is
  roughly: never reveal or serve credential files, including through symlinks
  or hard links. Keep that intent.

## Outcome

Make find, list and search cost about O(candidates) cheap string checks plus
O(1) identity work per request. Keep every security guarantee from those
commits.

1. **Compute the deny set once per request.** Compute the denied base paths,
   the denied base identities and the resolved home/cache roots once per
   request, or cache them process-wide with cheap revalidation. Do not
   recompute them per candidate.
   - Memoize per request the ancestor-directory identity results that
     candidates share.
2. **Name-only routes (`/fs/find`, `/fs/list`).** These must not do
   per-candidate identity syscalls (`EvalSymlinks`, `Stat`, `SameFile`) when
   the candidate comes from the repository's `git ls-files` list or the
   directory listing they already read.
   - A path-level deny over the cleaned path (prefix or segment match against
     the memoized deny set) is enough to hide denied names.
   - The full identity check, covering symlinks and hard links, moves to the
     point where content is actually read or served.
   - If the hardening commits' tests require a name to be hidden on list or
     find through a symlinked directory, keep that behavior: use the
     directory entry's type bits from the listing already read (a symlink
     shows as a symlink without an extra syscall), and resolve only those
     entries.
3. **Content routes** (`/fs/search` content reads, the file read/serve route
   and the raw/relay route).
   - Keep the full identity check for every file actually opened.
   - Prefer checking identity on the handle that is already open
     (GetFileInformationByHandle / `os.File.Stat` + `SameFile` against the
     memoized denied identities) over extra path syscalls.
4. **Cancellation.** `/fs/find` and `/fs/search` must stop work when the
   request context is cancelled, checking `r.Context()` in the candidate loop.
   - If the frontend does not debounce or abort a superseded find request,
     add an AbortController for the previous request plus a short debounce in
     the JS that calls these routes. Find it under `wsdashboard/assets`.
5. **Keep the hardening tests passing unchanged.** That includes the symlink
   and hard-link tests. Add tests showing that:
   - a find over N candidates performs O(1) deny-set computations, using a
     counter seam or an injected stat function;
   - a hard-linked or symlinked credential file is still refused when read;
   - a cancelled request stops early.

## Constraints

- This touches a security boundary. After implementing, spawn one
  independent reviewer subagent (Claude, opus) on your diff. Give it the
  three hardening commits as the contract. Fix any Critical or Important
  finding before you commit.
- Read `ai-docs/manuals/shipped-surface-boundary.md` and
  `ai-docs/manuals/release-compatibility.md`. Expected tier: T0, since
  routes, arguments and responses are unchanged. State it in the commit with
  source lines.
- Simplest complete change. No config knobs.
- Do not run any of these locally on this PC: `go test`, Go test executables,
  ws/ws-mcp executables, the launcher, the daemon, Electron, or a browser
  against the live daemon. This is the owner's antivirus rule. Go runs on
  w5090 only:
  `D:/sys_lock.sh remote w5090 run fs-find-deny-cost 25 --sync 'D:/Workspace/Repos/.ws-worktrees/devenv/flyaway-fifteen-judgingly' -- 'export PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && go vet ./internal/wsdashboard ./internal/mcp && go test ./internal/wsdashboard ./internal/mcp'`.
  - Accepted reds: only TestTopicStateBadgeResearchingMetadata and
    TestTopicStateBadgeClockAndReconciliation.
  - w5090 is Linux. Windows-only code paths (`*_windows.go`) must at least
    compile: also run `GOOS=windows go vet ./internal/wsdashboard` there.
- Temp files only under `D:/Temp/fs-find-deny-cost/`.
- Commit on `impl/develop/fs-find-deny-cost` with your own model's
  Co-Authored-By trailer. Do not push.
- Urgent: the owner cannot use the file browser. Keep momentum.

## Report

- the commit;
- the files;
- the syscalls per candidate and per request before and after, estimated
  from code or counted in a test;
- what moved from list/find to read time, and why the security intent still
  holds;
- the reviewer verdict;
- the w5090 result;
- the tier.
