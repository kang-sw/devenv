# Plan: review sweep S5 fixes

Branch `delegate/review-s5-fixes` (worktree
`/Users/kang-sw/.ws-worktrees/devenv/reference-outcast-afford`), based on
develop. Commit here; do not merge or push.

Source: review sweep segment S5 (`393650fea..a656a34b5`, board (446) b).
Full findings: `/tmp/review-sweep/S5-correctness.md`, `S5-fit.md`,
`S5-test.md`. Authority: `ai-docs/tickets/.done/261108-feat-ws-tracker-caller.md`.
Read `ai-docs/manuals/shipped-surface-boundary.md` before editing under
`agents-plugin-tool/`, and `ai-docs/manuals/ws-mcp.md` for `internal/mcp/`.
Do not change the ledger's on-disk or on-wire format, or the tracker tool
command syntax.

## Fix (each behavior fix with a test that fails before it)

1. **Important - unknown session key passes the lead-only gate.**
   `internal/mcp/tracker_tools.go` ~L545 and ~L808; gate at `server.go`
   ~L707-721 only runs when `s.sessions.lookup(key)` finds the key. Refuse an
   unknown key, or one that is not a top-level lead, before any registry read
   or write, in both tracker tools (ticket Decision 1). Check whether any
   other lead-only handler has the same shape (a non-empty check without a
   lookup) and report, not fix, what you find. Tests: `tk:register` and
   `tk:item.create` with a made-up key are refused and write nothing.
2. **Important (fit) - `wstrackerhost` copies `wsindex`'s non-interactive git
   environment and process-group kill** (`wstrackerhost/runner.go`,
   `proc_unix.go`, `proc_windows.go`), and the copy has drifted: remote
   commands lack `-c credential.interactive=false`. Extract the shared part
   into one helper both packages use (an internal package; keep each
   package's own runner type), so the flag set cannot drift again. If the
   extraction turns out to need changes to `wsindex`'s behavior, stop at
   aligning the flag and report why.
3. **Re-graded Important - "Nothing was registered" can be false.**
   `wstrackerhost/host.go` ~L288-301, `tracker_tools.go` ~L600-605: failures
   after `wsledger.Register` has pushed (profile or registry write, member id
   taken) still say nothing was registered. Make the refusal state what was
   already pushed and how to recover (re-run register or the remote holds the
   join), without changing the success path.
4. **Re-graded Important - reads stale up to 3 minutes after daemon start.**
   `wstrackerhost/poll.go` `StartPoll` takes `poll.lock` immediately (which
   disables the per-session pre-read fetch) but polls first only after a full
   interval. Run the first poll right away. Test it with the injectable clock
   or interval.
5. **Minor fixed while here - credential redaction misses tokens in a URL
   query string.** Extend the redaction to query-string credentials; test.
6. **Important (test) - all-or-nothing test can't see a partial write.**
   `mcp/tracker_tools_test.go` ~L304-317 queries `status: hidden`; assert that
   no item from the batch exists under any status.
7. **Important (test) - 20 of 25 `tk:` commands are never written through the
   tool.** Add a table-driven test through `tracker.update` covering every
   `tk:` event command (handoff, accept, decline, return, unassign, confirm,
   reject, reopen, comment.edit/hide, member/admin/key commands, priority
   levels, attachment limit), `item.assign` `levels`/`completion`, and
   `item.edit` `clear=due|body`, asserting the resulting folded view or
   event payload.
8. **Important (test) - `wstrackerhost/runner.go` ~L47-93:** test
   `isRemoteCommand`, `batchSSHCommand` precedence (including the plink
   variant) and the batch environment (after item 2, wherever it lives).

## Out of scope

Recorded by the lead as follow-ups: `$N` reference shift under idle-push
(`host.go` `DispatchWith`; related idea ticket from 001f8f786); lock error
printing `<nil>`; every other Minor in the three reports.

## Verification

`cd agents-plugin-tool && gofmt -l . && go vet ./... && go test -count=1 ./internal/wstrackerhost/ ./internal/wsindex/ ./internal/wstracker/ ./internal/wsledger/ && go test -count=1 -run 'Tracker' ./internal/mcp/`

## Return

Commit hashes, changed files, test output (and which new tests failed before
the fix), other handlers found in item 1, and anything left open.
