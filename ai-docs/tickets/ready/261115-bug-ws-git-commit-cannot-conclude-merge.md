---
title: "ws/git.commit cannot conclude a conflicted merge; native fallback strips ## AI Context"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: a75219a810ad218d
sage-review-completeness-reviewed: a75219a810ad218d
---

# ws/git.commit cannot conclude a conflicted merge; native fallback strips ## AI Context

## Background

Dogfood surprise relayed by MAIN on 2026-11-15. EXEC2 hit it while merging
develop into TASK4 (checkpoint `e4d2f4358`). This is a static capture and no
run was made for it.

1. `ws/git.commit` refused to commit a conflict resolution. Its explicit
   path grant covered only the resolved file (`store.go`), while the merge's
   incoming paths were already staged. A conflicted merge therefore cannot be
   concluded through `ws/git.commit` with a narrow grant.
2. The fallback, native `git merge --continue`, reportedly stripped the
   `## AI Context` heading the repository's commit format requires. Git's
   default cleanup strips comment lines when editing, but preserves them when
   not editing; `commit.cleanup` can override this. The historical editing and
   configuration state is not established by the resulting message
   (`e4d2f4358`; local Git 2.50.1 manual:
   `/Applications/Xcode.app/Contents/Developer/usr/share/man/man1/git-commit.1#L419-L477`).

## Original questions

The capture asked whether `git.commit` should understand an in-progress merge's
incoming paths and whether native fallback guidance should choose
`--cleanup=verbatim` or a different `core.commentChar`. The owner-approved scope
below settles the primitive's merge-completion outcome; it does not select a
native fallback or expand into workflow-text changes.

## Decisions

- Owner-approved FIX 6 scope (triage 1060, relayed 2026-11-15): let
  `ws/git.commit` conclude a conflicted merge with explicit resolved paths
  without treating already-staged incoming merge paths as unrelated. Preserve
  refusal of genuinely unrelated staging; `MERGE_HEAD` alone is not permission
  to commit everything.
- Keep the structured commit record, including `## AI Context`, and the merge's
  parents. Preserve remembered-branch, unresolved-conflict, path validation and
  ticket-verification safeguards. Do not replace the primitive with native
  `merge --continue` as the fix.
- Land this ticket before implementation derived from
  `261115-research-git-commit-empty-ticket-updates`. Keep the empty-update
  behavior correction separate so both changes' regression boundaries remain
  reviewable.
- (1063) Technical settlement from fact population: use conservative tree
  equality, not filename provenance. For pinned HEAD, a single MERGE_HEAD and
  one unambiguous merge base, automatically admit an outside-grant staged path
  only when HEAD's object/mode equals the base and the actual staged object/mode
  equals the incoming tree; absence is a value for additions/deletions. Reject
  any unmatched bytes or mode, unresolved index stage, or unsupported
  rename/file-directory relationship. Include every admitted ticket path in
  normal verification. Explicit grants remain the authority for resolved or
  composite paths, and unsupported automatic admission must name the paths
  the caller can include in its existing grant, not invent a new argument.
  Pin/recheck merge heads and admitted index entries before committing; stop
  if they change. Arbitrary concurrent external Git mutation is not serialized
  by this fix. Equality proves the allowed tree content, not who staged
  identical content; the contract is no unapproved tree bytes, not actor
  attribution. Rejected: MERGE_HEAD or incoming-filename whitelists, because
  neither detects same-path contamination; merge-tree reconstruction, because
  strategy/configuration are not retained and it may invoke merge drivers or
  write objects. Conservative fallback to explicit grants is safer and smaller.
- Compatibility: **T0 / bug fix**, under
  `ai-docs/manuals/release-compatibility.md`. Current commit reads status,
  stages explicit paths, validates all staging, then writes an ordinary Git
  commit (`agents-plugin-tool/internal/wsgit/git.go:623-721,907-925`). Merge
  currently leaves Git's conflict state on target for resolution
  (`agents-plugin-tool/internal/mcp/git_merge.go:340-355`). The new path must
  consume that existing merge state and write an ordinary multi-parent commit,
  retaining the message format (`wsgit/git.go:998-1018`) and existing MCP call
  contract. No new metadata format is selected. Old Git/current ws readers
  and a new reopen can read the commit and unchanged history; no reconstruction is
  needed. An unproven provenance mechanism, incompatible store addition or
  old-version refusal is a blocker, not approved T3 scope.

## Current source

- VERIFIED: `Commit` stages the grant before validating every staged path;
  `validateCommitStatus` refuses staged paths outside it, with no merge-state
  exception (`agents-plugin-tool/internal/wsgit/git.go:659-676,907-925`). This
  statically explains the reported narrow-grant failure, without a live repro.
- VERIFIED: the verifier receives delete-side-filtered granted paths
  (`wsgit/git.go:678-699`). Any merge-path admission must still verify relevant
  ticket content rather than hide incoming ticket paths from the verifier.
- VERIFIED: `CommitMessage` emits `## AI Context` and `Commit` calls
  `git commit -m` (`wsgit/git.go:707-712,998-1018`). The reported native cleanup
  loss is historical evidence; no native cleanup experiment was run here.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)
- Convention: ai-docs/manuals/release-compatibility.md (declared for agents-plugin-tool/internal/)
- Beyond the explicit grant, admit only the incoming tree content established
  by decision (1063); reject unrelated staged bytes or modes, including changes
  on an automatically admitted incoming path. Do not silently stage every dirty
  path or weaken non-merge commit behavior.
- Preserve existing merge state and dirty work on refusal; do not auto-abort,
  reset, amend or rewrite history. Unsupported automatic admission remains an
  actionable refusal with the explicit-grant recovery path; do not accept the
  whole index to make a complex merge pass.
- Applicable implementation conventions: read
  `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/ws-mcp.md` and
  `ai-docs/manuals/release-compatibility.md` before their covered source edits.
  Native fallback documentation and general message/trailer redesign are not
  part of this bounded primitive fix.

## Admission evidence

Source boundary for (1063): the existing merge entry checks staging and pins
source/target commits, then invokes Git with `merge.directoryRenames=false`;
it records no per-path baseline for later `Commit` admission
(`agents-plugin-tool/internal/mcp/git_merge.go#L176-L207`,
`agents-plugin-tool/internal/mcp/git_merge.go#L307-L355`). The current commit
check is path membership, not blob/mode equality
(`agents-plugin-tool/internal/wsgit/git.go#L907-L925`). Git exposes staged
mode/object/stage tuples, while `AUTO_MERGE` is an ort-only working-tree
snapshot that includes conflict markers, not a clean-path provenance ledger
(local Git 2.50.1 manuals:
`/Applications/Xcode.app/Contents/Developer/usr/share/man/man1/git-ls-files.1#L88-L105`,
`/Applications/Xcode.app/Contents/Developer/usr/share/man/man1/git-merge.1#L488-L523`).
`merge-tree --write-tree` can compute a candidate merge tree without touching
the index/worktree, but writes Git objects; its conflict output must be parsed,
and an empty conflicted-file list does not prove a conflict-free merge
(`/Applications/Xcode.app/Contents/Developer/usr/share/man/man1/git-merge-tree.1#L39-L106`,
`/Applications/Xcode.app/Contents/Developer/usr/share/man/man1/git-merge-tree.1#L439-L451`).
These artifacts explain the conservative content-equality choice in (1063);
no actor-provenance or universal automatic-merge reconstruction is claimed.

## Prior Decisions

- 08ec9d8a (2026-11-15, commit): "ws/git.commit again refused an approved incoming staged path as unrelated" — bearing: supports
- 206a5830 (2026-11-15, commit): "Use --cleanup=verbatim so this full AI Context remains in the merge commit, and verify exact parents after committing." — bearing: constrains
- bcc26706 (2026-11-08, commit): "Ticket status moves are always explicit, so the heuristic never helps." — bearing: constrains
- 260810-feat-idea-ticket-attention-policy (2026-08-10, commit): "stagingCommandsForCommit threads a new sparseActive parameter that adds --sparse to the `add -A` branch only when true" — bearing: constrains
- 260720-wsdoc-commit-boundary (2026-08-10, commit): "a captured idea/ ticket must still be stageable under an active sparse-checkout scope" — bearing: constrains
- 260726-feat-verify-ticket-graph-advisories (2026-07-27, commit): "Carrier shape: `wsgit.Verifier` returns `([]string, error)`" — bearing: constrains
- 260725-git-commit-ai-context-condition-reporting (2026-07-25, commit): "git.commit's ai_context rejection names absent/empty/all-blank distinctly (no size limit)" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsgit/git.go; agents-plugin-tool/internal/mcp/git_merge.go; affected wsgit and MCP tests |
| scope.surface | cross-module | existing exported Client.Commit and MCP git.commit integration; no new call contract approved |
| scope.new_public_symbol | no | none required by Phase 1; internal mechanism remains open |
| scope.new_type_contract | no | decision 1063 uses existing Git trees/index and explicit path grants, with no wire or shared-data contract |
| scope.test_surface | existing | agents-plugin-tool/internal/wsgit/git_test.go; agents-plugin-tool/internal/mcp/server_test.go; agents-plugin-tool/internal/mcp/tickets_verify_test.go |
| complexity.reuse_points | confirmed | Commit, validateCommitStatus, filterIndexDeleteSidePaths and CommitMessage read in agents-plugin-tool/internal/wsgit/git.go#L623-L721 and #L907-L1018 |
| complexity.side_effect_risk | high | staging and committing change index, merge state and history; current verifier veto leaves granted paths staged |
| risk.correctness | high | admission must compare actual staged content, not just incoming path names, and retain both parents |
| risk.fit | moderate | conservative tree equality and explicit-grant fallback must fit existing Git state without a new API or store |
| risk.test | high | real-Git merge, same-path contamination, ticket moves, sparse paths and refusal preservation need integrated coverage |
| risk.security_or_contract | high | explicit path ownership boundary expands only for proven merge-generated content; a blanket bypass violates the approved contract |

## Phases

### Phase 1: Complete resolved merges without widening unrelated staging

Support a resolved in-progress conflicted merge using the explicit resolution
grant plus incoming staged content allowed by decision (1063). Retain
ordinary commit behavior, ticket verification and the structured merge record.

Regression verification: create a local two-parent merge with one conflict and
at least one clean incoming staged path outside the explicit grant; resolve
only the conflicted file and conclude through MCP and the client. Assert both
parents, intended tree, retained `## AI Context`, cleared merge state and ticket
verification for admitted incoming tickets (including rename/delete handling).
Cover unsupported automatic admission with actionable explicit-grant recovery,
composite automatic content, ambiguous/multiple merge heads/bases, rename and
file-directory cases, and a changed pinned head/index snapshot. Add negative
cases for unresolved entries, unrelated staged additions/edits
(including same-path contamination), missing/invalid merge state, branch
mismatch and verifier veto; ensure no commit or history loss on refusal. Retain
non-merge unrelated-staging, sparse-path and ticket-move regressions. Run the
affected `internal/wsgit` and `internal/mcp` tests and
`cd agents-plugin-tool && go test ./...` in local temporary repositories. This
draft records no implementation, cleanup-policy choice or verification pass.
