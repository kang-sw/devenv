# Plan: 260725-idea-ws-git-commit-rename-and-payload-rejections — Phase 1: Commit a staged ticket rename

## Relevant Ticket Contract

- Diagnosis correction (already settled, must not be re-derived): `validateCommitStatus` already works via stem expansion; the bug is entirely in what `c.verifier()(root, opts.Paths)` receives — the verifier is fed the pre-rename / delete-side path that `expandCommitPathsForTicketMoves` injects, and `wsdoc.TicketVerify`'s `file-exists` guardrail fails trying to read it.
- **Single-guard fix, call-site only.** `Verifier` is `(root, paths) error` with no status argument (`agents-plugin-tool/internal/wsgit/git.go:45`), so filtering must happen in `Commit` before the verifier call, not inside `Verifier`/`TicketVerify` itself.
- Do **not** add rename-pair collapsing to `validateCommitStatus` — it would be dead code (stem expansion already supplies the pair for the unrelated-path guard).
- Do **not** make the `[_index.md]`-only form succeed — that refusal is correct and must stay.
- Must cover the outright staged **deletion** case in the same change (same `file-exists` guardrail fires, no destination path to fall back to).
- Strategy choice must be settled deliberately (not left incidental) between: (a) pass the caller's unexpanded `paths` to the verifier, or (b) filter the expanded list for index-delete-side entries. The ticket flags that the two diverge when a caller passes extra ticket paths explicitly.
- Verification boundary named by the ticket: `tickets.move` promotion (`todo/` -> `ready/`) commits with destination path alone; `tickets.close` to `.dropped/` commits; outright staged deletion commits; a caller passing extra ticket paths alongside a staged rename has its behavior pinned by a test; a genuinely unrelated staged ticket path is still refused; `[_index.md]`-only still fails; content-only ticket edits are unchanged.

## Out of Scope

- Phase 2 (`ai_context` payload-size misreport) — independent, not touched.
- `internal/mcp/server.go`'s `verifyAdapter` / `wsdoc.TicketVerify` internals — the fix must stay at the `wsgit.Client.Commit` call site per the ticket's single-guard framing; the adapter is exercised only indirectly through the existing `Verifier` stub-based test pattern.
- Spec update to `ai-docs/spec/mcp-tools.md` — ticket marks this "contract-first: no," corrected at closeout, not part of this phase's implementation.
- Any change to `expandCommitPathsForTicketMoves`'s expansion behavior itself, or to `validateCommitStatus`'s unrelated-path guard — both are confirmed already-correct and must keep receiving the full expanded `opts.Paths`.

## Codebase Findings

- `agents-plugin-tool/internal/wsgit/git.go#L451-L496` — `Commit`: `preStatus` read (L457-461) feeds `expandCommitPathsForTicketMoves` (L462, defined L635-667), which appends any changed ticket-shaped file (by stem) matching either `Path` or `OldPath` into `opts.Paths`. That same expanded `opts.Paths` then feeds `validateCommitStatus` (L473, defined L615-633) **and** `c.verifier()(root, opts.Paths)` (L480) unfiltered. The fix belongs strictly between the post-staging status computation (L468-472, variable `status`) and the verifier call at L480.
- `agents-plugin-tool/internal/wsgit/git.go#L82-88` — `FileStatus{Path, OldPath, Status, IndexStatus, WorktreeStatus}`; `IndexStatus` is the single first-char code (`R`, `D`, `A`, `M`, …) and `OldPath` is populated only for type-`2` (rename/copy) porcelain-v2 records (`parseFileLine`, L162-173). This is exactly the signal needed to identify "index-delete-side" paths without a new git invocation, confirming the ticket's suggestion that no third `git diff` call is required.
- `agents-plugin-tool/internal/wsgit/git.go#L150-173` (`parseFileLine`) — for a type-`2` record, `Path` is the text **before** the tab (current/new side) and `OldPath` is **after** the tab (previous/original side) — confirmed against `TestCommitStagesRenamedDirectoryWithoutAddingMissingOldRoot` (git_test.go:390-400), whose `want` stages (`add`) the pre-tab path and removes-from-index (`rm --cached`) the post-tab path. So "delete-side" = `OldPath` of a type-`2` record whose `IndexStatus == "R"` (renames delete the source; copies, `IndexStatus == "C"`, do not and must not be filtered), plus `Path` of any record with `IndexStatus == "D"` (outright staged deletion).
- `agents-plugin-tool/internal/wsdoc/tickets_verify.go#L42-45` — `TicketVerify(root, paths)` returns a caller-input **error** (not a `VerifyFinding`) when `len(paths) == 0`: `"paths requires at least one path"`. **Risk signal:** if the verifier-side filter reduces `opts.Paths` to empty (the outright-deletion case, when the deleted ticket is the only path in the commit), calling `c.verifier()(root, [])` would surface this caller-input error and break the commit — the opposite of the ticket's required outcome ("an outright staged ticket deletion commits"). The call site must skip invoking the verifier entirely when the filtered list is empty, treating "nothing left to verify" as vacuously OK. This nuance is not spelled out in the ticket text and must be a deliberate, tested decision in the plan (not incidental).
- `agents-plugin-tool/internal/wsdoc/tickets_verify.go#L102` — the `file-exists` guardrail (`addFinding("file-exists", ...)` after a failed `os.ReadFile`) is the one that fires today; it is untouched by this fix — the fix works by never handing it a delete-side path, per the ticket's stated direction.
- `agents-plugin-tool/internal/wsgit/git.go#L615-633` (`validateCommitStatus`) — its unrelated-path guard already accepts a staged file when either `file.Path` or `file.OldPath` is in the (expanded) `paths` set (L624). This must keep receiving the **unfiltered** expanded `opts.Paths`, since dropping the old path from what it sees would silently reopen the "genuinely unrelated" gap for the delete-side path itself (untested but risky) — the filter is strictly for the verifier call, confirmed by the ticket's "what the verifier receives" framing.
- `agents-plugin-tool/internal/wsgit/git_test.go#L336-352` (`TestCommitExpandsTicketMovePathsByStem`, `TestCommitExpandsTodoToReadyTicketMovePathsByStem`) — existing pure-function test pattern for `expandCommitPathsForTicketMoves`; a new pure-function test for the filter helper should mirror this style (construct `StatusResult` via `ParseStatus([]byte(...))`, assert on the returned `[]string`).
- `agents-plugin-tool/internal/wsgit/git_test.go#L615-688` (`TestCommitBlockedByVerifierNeverReachesCommit`, `TestCommitProceedsWhenVerifierNilDefaultsToNoOp`) — existing `Client.Commit`-level pattern for asserting exactly what paths the `Verifier` stub receives (`gotPaths`), using `sequenceRunner` (L701-…) and `mustWriteGitTestFixture` (L690-699, real `t.TempDir()` root with an on-disk ticket file). New end-to-end tests for rename/close/delete/divergent cases should follow this pattern rather than the fake-`/repo` `sequenceRunner`-only style used for the pure-status tests, since the verifier assertion is the point.
- No existing test reproduces the `[_index.md]`-only-with-a-staged-rename-present scenario (row 3 of the ticket's dogfood matrix) — confirmed by search; this is a genuine coverage gap the ticket explicitly wants pinned, not just re-verified as already covered.

## Implementation Plan

1. In `agents-plugin-tool/internal/wsgit/git.go`, add a new helper near `expandCommitPathsForTicketMoves` (after L667), e.g. `filterIndexDeleteSidePaths(status StatusResult, paths []string) []string`:
   - Build a `map[string]bool` of delete-side paths (cleaned via `filepath.ToSlash(filepath.Clean(...))`, matching the existing normalization used by `pathInCommitSet`/`expandCommitPathsForTicketMoves`) by scanning `status.ChangedFiles`:
     - `IndexStatus == "D"` → mark `file.Path`.
     - `IndexStatus == "R"` and `file.OldPath != ""` → mark `file.OldPath`.
   - Return a new slice containing every entry of `paths` whose cleaned form is **not** in that set, preserving input order (do not mutate `paths`).
2. In `Commit` (`agents-plugin-tool/internal/wsgit/git.go`, current L476-482 region, right after the post-staging `status` is computed at L472 and `validateCommitStatus(status, opts.Paths)` passes at L473-475):
   - Compute `verifyPaths := filterIndexDeleteSidePaths(status, opts.Paths)`.
   - Replace the unconditional `if err := c.verifier()(root, opts.Paths); err != nil { ... }` with a guarded call: only invoke `c.verifier()(root, verifyPaths)` when `len(verifyPaths) > 0`; when it is empty, skip the call and proceed (do not treat "nothing to verify" as an error) — this is what makes the outright-deletion case commit without tripping `wsdoc.TicketVerify`'s empty-paths caller-input error.
   - Leave `validateCommitStatus`'s call and `opts.Paths` (the full expanded list) untouched everywhere else in `Commit` (staging, ticket-change detection, commit message, result).
3. Add a short code comment at the new filter call site referencing the `Verifier` doc comment's existing pointer to `{#260720-wsdoc-commit-boundary}` (git.go:40-44) so the "why only the verifier, not `validateCommitStatus`" decision stays discoverable in-place, mirroring the existing comment style at L476-479.
4. Do not modify `expandCommitPathsForTicketMoves`, `validateCommitStatus`, `wsdoc/tickets_verify.go`, or `internal/mcp/server.go`'s `verifyAdapter` — all three are confirmed correct/out of scope by the ticket and the survey above.

### Recommended strategy (settled)

Filter the **expanded** list for index-delete-side entries (option "b" in the ticket), not the caller's unexpanded `paths` (option "a"). Rationale, grounded in the divergent case the ticket flags:

- If a caller explicitly passes **both** the old and new path of a rename (e.g. `[todo/<stem>.md, ready/<stem>.md]`, matching row 2 of the dogfood matrix), passing the *unexpanded* caller paths straight to the verifier would still hand it the stale old path and reproduce the original bug — option (a) does not actually fix this caller shape.
- Filtering the expanded list by **index status** (delete-side, regardless of whether the path arrived via caller input or via stem-expansion) fixes both the implicit-expansion case and the explicit-both-paths case uniformly, and is also what naturally extends to the outright-deletion case (no rename involved, just `IndexStatus == "D"`).
- This is the same distinction as the ticket's own "natural rule" framing ("skip verification for index-delete-side paths... covers both rename and deletion"), made explicit and testable rather than incidental.

## Verification Plan

- `cd agents-plugin-tool && go test ./internal/wsgit/... ` — run after implementation; all existing tests (including `TestCommitExpandsTicketMovePathsByStem`, `TestCommitExpandsTodoToReadyTicketMovePathsByStem`, `TestCommitRefusesUnrelatedStagedPaths`, `TestCommitBlockedByVerifierNeverReachesCommit`, `TestCommitProceedsWhenVerifierNilDefaultsToNoOp`) must keep passing unchanged, since none of them stage a rename/delete alongside a Verifier stub today.
- Add pure-function tests for `filterIndexDeleteSidePaths` (mirroring `TestCommitExpandsTicketMovePathsByStem`'s `ParseStatus([]byte(...))` + assert-on-slice style), covering:
  1. A staged `todo/` -> `ready/` rename (`IndexStatus == "R"`, type-`2` record) — old path filtered out, new path kept.
  2. A staged rename to `.dropped/` (close case) — same shape, different destination dir, to satisfy the ticket's explicit "close path" regression requirement.
  3. An outright staged deletion (`IndexStatus == "D"`, type-`1` record) — the deleted path itself is filtered, leaving an empty result when it was the only path.
  4. An unrelated path present in `paths` that has no matching delete-side entry in `status` — passed through unchanged (content-only-edit case).
  5. A copy (`IndexStatus == "C"`) with `OldPath` set — old path is **not** filtered (copies don't delete the source), to pin the `R`-vs-`C` distinction called out in the findings above.
- Add `Client.Commit`-level tests (mirroring `TestCommitBlockedByVerifierNeverReachesCommit` / `TestCommitProceedsWhenVerifierNilDefaultsToNoOp`'s `sequenceRunner` + `mustWriteGitTestFixture` + `Verifier` stub pattern, asserting on `gotPaths` seen by the stub) covering the ticket's named verification boundary:
  1. **Promotion**: staged `todo/` -> `ready/` rename, `opts.Paths = [ready/<stem>.md]`; assert `Commit` succeeds and the `Verifier` stub receives only the destination path.
  2. **Close**: staged rename to `.dropped/`, `opts.Paths = [.dropped/<stem>.md]`; assert `Commit` succeeds and the stub receives only the destination path.
  3. **Outright deletion**: staged delete of a ticket, `opts.Paths = [idea/<stem>.md]` with no other paths; assert `Commit` succeeds and the `Verifier` stub is **not invoked at all** (call-count assertion, same style as the "must stop before `git commit`" assertion in `TestCommitBlockedByVerifierNeverReachesCommit`).
  4. **Divergent case (extra paths + staged rename)**: `opts.Paths = [todo/<stemA>.md, ready/<stemA>.md, <other-ticket-path-stemB>]` where `<stemB>` is a separate, independently content-edited ticket in the same commit; assert `Commit` succeeds and the stub receives exactly `[ready/<stemA>.md, <other-ticket-path-stemB>]` (old path of the rename excluded even though the caller passed it explicitly) — this is the test that pins the strategy choice above.
  5. **Unrelated path still refused**: a staged ticket rename present in `status` for a stem the caller did *not* pass, with `opts.Paths` naming only an unrelated ticket path; assert `Commit` fails with `"unrelated staged path"` before the `Verifier` stub is ever called (call-count assertion) — confirms `validateCommitStatus`'s existing guard, which runs before the new filter, is unaffected.
  6. **`[_index.md]`-only still fails**: a staged ticket rename present in `status`, `opts.Paths = ["_index.md"]` only (no ticket stem in `paths`, so no expansion runs); assert `Commit` fails with `"unrelated staged path"` — closes the coverage gap identified in Codebase Findings (row 3 of the dogfood matrix, previously untested).
  7. **Content-only edit unchanged**: a plain `M`/`A` ticket status entry, no rename/delete anywhere in `status`; assert `Commit` succeeds and the stub receives `opts.Paths` unmodified (filter is a no-op) — can reuse the existing `TestCommitBlockedByVerifierNeverReachesCommit`/`TestCommitProceedsWhenVerifierNilDefaultsToNoOp` fixtures' shape as a template.
- No manual verification needed beyond `go test`; this is a pure Go unit-test-covered change with no I/O surface beyond what `sequenceRunner`/`mustWriteGitTestFixture` already exercise in-package.

## Escalations

- None.
