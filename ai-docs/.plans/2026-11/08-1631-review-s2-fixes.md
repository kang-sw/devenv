# Plan: review sweep S2 fixes

Branch `delegate/review-s2-fixes` (worktree
`/Users/kang-sw/.ws-worktrees/devenv/declared-satiable-proximity`), based on
develop. Commit here; do not merge or push.

Source: review sweep segment S2 (`fbb6d0a90..8e21ac7a5`, board (446) b).
Full findings: `/tmp/review-sweep/S2-correctness.md`, `S2-test.md`,
`S2-fit.md`. Read `ai-docs/manuals/shipped-surface-boundary.md` before
editing under `agents-plugin-tool/`. Authority for the ledger:
`ai-docs/tickets/.done/261107-feat-ws-tracker-ledger-protocol.md` and
`261108-feat-ws-tracker-data-structure.md`. Do not change the ledger's
on-disk or on-wire format, or the payload-agnostic protocol.

## Fix

1. **Important - `Ledger.Events` hides events that left the view.**
   `internal/wsledger/store.go` (`Events` ~L1095-1130, `rebuild`
   ~L716-770). A member ref that `loadRef` resets on a non-ancestor tip, or
   a create-once ref replaced after a lost race (and `MoveToFreshClone`),
   drops events from the view, but `Events` keeps the same `Cursor.Epoch`
   and reports no removal, so a caller that upserts by `EventID`
   (`wstracker.Tracker.Refresh`, `internal/wstracker/tracker.go` ~L43-65)
   keeps the vanished events. Fix inside wsledger: whenever a rebuild drops
   an event id the previous view held, rotate the epoch so the next
   `Events` call is a full re-read (the existing `all := after.Epoch !=
   l.epoch` path). Confirm `wstracker.Tracker.Refresh` resets its cache on a
   full re-read; fix it if not. Test: a scenario that drops an event (the
   lost create-once race or a non-ancestor ref reset; existing remote tests
   have the setup) shows the epoch change and no stale event in the next
   `Events`, plus a `wstracker` test that the folded view holds no duplicate
   after it.
2. **Important (test gap) - roles admin-rule verdicts untested.**
   `internal/wsledger/verify.go` ~L786-836 (`pass.roles`). Add a table of
   forged-commit cases with the existing `forger` / `forgeCommit` helpers:
   non-admin author of a `roles.*` event; add_admin of an existing admin or
   a removed member; remove_admin of a non-admin or the last admin; add_key
   with a key in use or for a removed member; revoke_key of an already
   revoked or unheld key; remove_member of an already removed member or an
   admin; unknown member; malformed payload. Assert the verdict the code
   gives; if a case's verdict contradicts the ledger ticket, stop and report
   it rather than changing the rule.
3. **Important (test gap) - unregistered key keeps writes in the journal.**
   `internal/wsledger/store.go` ~L826, ~L938 (and `smoke_test.go` ~L66).
   Add a test: after the key reads unregistered, an append and a sync land
   no commit, the journal keeps the event, and once the key is added the
   batch closes and commits.

## Out of scope

Recorded by the lead as follow-ups, not fixed here: `MoveToFreshClone`
multi-process clone-id switch (no caller yet); moved / re-issued events
losing intra-batch references (needs a protocol-level design); stale
`wstracker.Dispatch` view (mitigated by `wstrackerhost.DispatchWith`); every
Minor in the three reports.

## Verification

`cd agents-plugin-tool && gofmt -l . && go vet ./internal/wsledger/ ./internal/wstracker/ ./internal/wstrackerhost/ && go test -count=1 ./internal/wsledger/ ./internal/wstracker/ ./internal/wstrackerhost/`

## Return

Commit hashes, changed files, test output (and which new tests failed
before the fix), and anything left open.
