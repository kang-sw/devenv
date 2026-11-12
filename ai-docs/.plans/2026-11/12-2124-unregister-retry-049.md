# Tracker unregister on Windows: retry the refused rename, then restore and refuse (no orphan)

## Outcome

The 0.49 re-review of the Windows CI repairs (range d56caca44..4060b9029)
found that commit 2c943c519's design orphans store directories. Findings file
(read its T1, C1, C2 items and the "Lead decision" section first):
`/home/swkang/.cache/ws@kang-sw-devenv/proj/dac18b1d/review-paths/b0d5cfdb-01-findings.md`.

Today, in the Windows order (`renameUnderLocks` false), a refused rename in
`internal/wstrackerhost/unregister.go` returns success and leaves
`<root>/<id>` with no registry entry; only a `withNet` call whose fn returns
an error cleans it up, so the background poll / pre-read / session-start
checks (which return nil), non-network handle holders, a dying process, or an
older ws leave a directory that blocks every later Register of that id.

Lead's technical decision (retry-then-restore):

1. In the Windows order, after the locks are released, retry the rename for a
   bounded window (pick a constant of a few seconds that covers a typical
   background fetch, e.g. 5 s, polling at a short interval; make the window
   and the rename hook test-injectable as the existing `renameDir` /
   `renameUnderLocks` vars are). If it succeeds, Unregister succeeds as today.
2. If the rename is still refused at the end of the window (or fails for any
   other reason), restore the registry entry and return an error that says the
   store directory is in use and to try again (wrap the OS error). This is the
   pre-2c943c519 failure mode, made rarer by the retry.
3. Remove the leftover-cleanup machinery 2c943c519 added that no longer has a
   purpose: `removeLeftover`, the withNet hook that calls it, `statDir` /
   `dirInfo` identity capture, unless some piece is still needed by (1)-(2).
   Keep `asidePath` if still shared. Keep the Linux order exactly as it is.
4. Check that restoring the entry after a gap is safe for other processes:
   during the retry window another process may see the store as
   unregistered and retire its handle; confirm a later call reopens it once
   the entry is back (the pre-2c943c519 restore path had the same gap; cite
   the code that handles it, or fix it minimally and test it).
5. Tests (in `internal/wstrackerhost`): rework
   `TestUnregisterDuringHeldFetchDiscardsTheResult`'s "rename refused" case
   and add cases, all runnable on every OS via the injected rename hook and
   window:
   - rename refused for the whole window -> Unregister returns the in-use
     error, the registry entry is back, the directory is untouched, and a
     later Unregister (rename allowed) succeeds;
   - rename refused then allowed within the window -> Unregister succeeds and
     the held fetch's result is discarded (the test's original intent);
   - no network call involved (refusal from a non-network holder) -> same as
     the first case.
   Keep the Windows-real-path subtests passing on Windows CI.
6. Fold-ins (minor):
   - `internal/wsledger/store.go` ~504: rewrap the `Exclusive` doc paragraph
     to the surrounding width.
   - `unregister.go`: make the `moveAside` comment and the Unregister doc
     comment describe the final policy (no contradicting rename-failure
     rules).
   - `internal/wsindex/clone.go`: when the Windows ErrPermission retry reaches
     the deadline, report the last OS error (not only "lock is held").

## Boundaries

- Tier must stay T0: no store shape, registry field, lock location, tool, or
  event change. If the design cannot stay T0, stop and report.
- Read before editing: `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/release-compatibility.md`.
- Never run go build/test/vet locally. Linux:
  `~/.local/bin/ws-rgo -C agents-plugin-tool -m 25 -- test ./internal/wstrackerhost/ ./internal/wsledger/ ./internal/wsindex/ -count=1`
  from the worktree root (and vet on those packages).
- Windows: commit, `git push -u origin impl/develop/unregister-retry-049`
  (only this branch; never develop or main), `gh workflow run ws-mcp-ci.yml
  --ref impl/develop/unregister-retry-049`, confirm the run's headSha is your
  commit, `gh run watch <id> --exit-status`. Every job must be green. A
  failure outside your packages that looks like a known Windows timing flake
  (it passed on the previous runs): rerun only failed jobs once with
  `gh run rerun <id> --failed` and report both attempts.
- Minimal changes; do not touch wsagent, wsholder, mcp, or wsdashboard.

## Verification

- Linux ws-rgo test + vet green on the three packages, with `-v` evidence
  that the new subtests ran.
- Windows CI run fully green (run id + per-job conclusions).

## Return

Commits on `impl/develop/unregister-retry-049` in
`/home/swkang/.ws-worktrees/devenv/aim-hydration-granite`, this plan file in
the first commit. Do not merge. Report commits, the final design (window,
error text), the item-4 finding, test evidence, and the tier.
