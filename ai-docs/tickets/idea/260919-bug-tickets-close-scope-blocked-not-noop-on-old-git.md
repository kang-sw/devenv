# bug: scope-blocked TicketsClose is not a no-op on git < 2.42

## Summary

`TicketsClose` into an out-of-scope destination is only a clean no-op when the
destination scope pre-flight can run, which requires
`git sparse-checkout check-rules` (git >= 2.42). On older git the pre-flight
(`scopeDestinationError` -> `ticketScope.includes`) fails OPEN, so the close
proceeds to `appendResolution` — which appends a `## Resolution` section to the
source file — *before* the `git mv` fails on the sparse boundary. The refusal
then arrives via the `wrapScopeMoveError` backstop, but the source file has
already been rewritten. Result on git < 2.42:

1. The blocked close leaves the working tree dirty (`M <source>.md`), violating
   the no-op guarantee the pre-flight is meant to provide.
2. Following the error's own "widen the scope and retry" remedy appends a
   **second** `## Resolution` section, corrupting the ticket — exactly the
   hazard `TestTicketsCloseBlockedByScopeThenWidenedRetryIsClean` documents.

## Evidence

Found while sweeping the full Windows test suite on a git 2.38.1 host
(`ssh ki608@192.168.33.6`) at develop tip 08991c9b. The test now skips this
scenario below git 2.42 (see `internal/wsdoc/tickets_scope_test.go`,
`gitSupportsScopeDestPreflight`); this ticket tracks the underlying gap the skip
exposes. CI (`windows-latest`, git 2.55) is unaffected, and modern-git callers
get the correct no-op — so this is an older-git-only correctness gap, not a
release regression.

## Fix direction (not yet decided)

Make the source-mutating writes reversible or deferred so a scope refusal stays
a true no-op on any git version. Candidates:

- Roll back the `appendResolution` write if the subsequent `git mv` fails.
- Perform the destination-scope check without `check-rules` on old git (e.g.
  match the destination against the active sparse patterns directly) so the
  pre-flight no longer fails open.
- Stage the resolution write and only commit it after the move succeeds.

The shipped mutation ordering is load-bearing (the pre-flight-before-write
ordering is what makes a refusal a no-op), so any change here needs the
`internal/wsdoc` mutation tests green on both a >= 2.42 and a < 2.42 git.
