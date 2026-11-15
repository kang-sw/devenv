---
title: "Honor explicit empty ticket updates in git.commit"
blocked-by: 261115-bug-ws-git-commit-cannot-conclude-merge
related:
  261115-research-git-commit-empty-ticket-updates: "source Outcome Ledger; Verified Findings and Confirmed Decisions only"
  261115-bug-ws-git-commit-cannot-conclude-merge: "prerequisite; land before this implementation"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 5c8465ab3d5a9240
sage-review-completeness-reviewed: 5c8465ab3d5a9240
completed: 2026-11-16
---

# Honor explicit empty ticket updates in git.commit

## Background

Derived only from the Verified Findings and Confirmed Decisions in the Outcome
Ledger of `261115-research-git-commit-empty-ticket-updates`. Root authorized
this actionable derivation following MAIN's 2026-11-15 08:44:21 confirmation;
the research record remains separate and non-executable.

VERIFIED in that ledger: a ticket-closure call supplied `updated_tickets: []`,
but the commit still appended inferred `## Ticket Updates` after a co-author
trailer. An implementation-only commit with the same argument emitted no such
section because it changed no ticket files. These are historical observations,
not a reproduction performed for this draft.

VERIFIED source findings from the ledger: the optional argument description
promises detection when omitted
(`agents-plugin-tool/internal/mcp/server.go:4867`), and dispatch passes the
converted list (`server.go:1674`); that conversion already distinguishes an
omitted argument (nil slice) from a JSON empty array (allocated empty slice)
(`agents-plugin-tool/internal/mcp/server.go#L6050-L6063`). Normalization then
collapses that distinction: `trimStrings` allocates an empty slice even for nil
input, and inference checks length rather than supplied state
(`agents-plugin-tool/internal/wsgit/git.go#L749-L755`,
`agents-plugin-tool/internal/wsgit/git.go#L1035-L1044`,
`agents-plugin-tool/internal/wsgit/git.go#L700-L703`). `CommitOptions` already
carries `UpdatedTickets []string` with `json:"updated_tickets,omitempty"`
(`agents-plugin-tool/internal/wsgit/git.go#L565-L570`); the direct CLI caller
passes its initially nil repeated-flag slice, not an MCP request
(`agents-plugin-tool/cmd/ws-mcp/hosted_cli.go#L361-L386`,
`agents-plugin-tool/cmd/ws-mcp/main.go#L132-L135`). The message section and
`CommitResult.TicketChanges` are distinct
(`agents-plugin-tool/internal/wsgit/git.go#L596-L620`,
`agents-plugin-tool/internal/wsgit/git.go#L700-L721`). This locates the loss at
normalization and the length-based inference gate, without selecting a new
internal representation or changing the public contract.

## Decisions

- Match the existing tool description: explicit `updated_tickets: []`
  suppresses inferred message summaries; omission retains inference. Preserve
  nonempty explicit summaries, ticket validation, detection metadata,
  advisories and the rest of the commit record. This is the source ledger's
  owner-confirmed FIX 6 contract, not a new message-format policy.
- (1065) Technical settlement from fact population: add a JSON-excluded
  internal suppression boolean to CommitOptions, set by MCP only when the
  existing converted list is non-nil and zero-length. Preserve it through
  normalization and use it only to suppress inferred message summaries.
  Its zero value retains current direct Go/CLI inference; nonempty explicit
  summaries remain authoritative. Rejected: treating every non-nil empty Go
  slice as suppression, because that changes direct callers outside the
  existing MCP promise. No new wire argument, null/wrong-type semantics or
  CLI contract is selected.
- Land `261115-bug-ws-git-commit-cannot-conclude-merge` first and serialize
  implementation 4→5 (pending 261115-bug-ws-git-commit-cannot-conclude-merge).
  The research ticket is neither renamed nor promoted;
  this bug ticket is its bounded actionable derivation.
- INFERRED compatibility target: **T0 / bug fix**, subject to source-grounded
  implementation review under `ai-docs/manuals/release-compatibility.md`.
  The existing optional argument and valid call shapes remain unchanged
  (`agents-plugin-tool/internal/mcp/server.go:1674,4867`). Old callers omitting
  it must retain inference on the corrected server; callers supplying a
  nonempty list must retain their explicit summaries. The new server changes
  only the explicit-empty inference path currently selected by the length
  check (`agents-plugin-tool/internal/wsgit/git.go:701-703`). Both paths retain
  the existing Git commit/result writer (`wsgit/git.go:700-721`), with no new
  persisted format or historical rewrite: old/current readers and a new reopen
  can read the same Git records without reconstruction. An explicit-empty
  call to an unfixed old server still has the existing inference bug; this
  ticket does not promise suppression there. A required new argument,
  version refusal or incompatible store change is a blocker, not approved T3
  scope. The final tier remains conditional on the actual implementation.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)
- Convention: ai-docs/manuals/release-compatibility.md (declared for agents-plugin-tool/internal/)
- Do not expand into general protocol/API semantics, co-author trailer
  ordering, null/wrong-type semantics or a new CLI contract. The ledger's
  trailer observation is evidence of impact, not authority for redesign.
- Retain existing validation/detection behavior even when inferred message
  summaries are suppressed. Do not infer a presence representation or public
  interface change from the historical observation.
- Applicable implementation conventions: read
  `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/ws-mcp.md` and
  `ai-docs/manuals/release-compatibility.md` before their covered source edits.
- This is a draft only. Root owns fact population, readiness gates and
  dispatch; this preparation changes no source, status, stamp or commit.

## Prior Decisions

- 261115-research-git-commit-empty-ticket-updates (2026-11-15, Confirmed Decisions): "Owner-approved FIX 6 scope (triage 1060, relayed 2026-11-15): match the existing `git.commit` tool description only." — bearing: constrains
- 261115-bug-ws-git-commit-cannot-conclude-merge (2026-11-15, Decisions): "Keep the empty-update behavior correction separate so both changes' regression boundaries remain reviewable." — bearing: constrains
- 9cfa851c (2026-11-15, commit): "the commit tool appended automatic Ticket Updates after the co-author trailer despite updated_tickets: []." — bearing: supports
- d9a7cc55 (2026-10-02, commit): "Types, closed enums (levels minor<important<critical, category review), the bullet writer, and id minting live in wsgit" — bearing: constrains
- 31eedb1e (2026-09-17, commit): "expected_branch is enforced as required both at the MCP schema (required list) and in normalizeCommitOptions so the CLI and direct callers are guarded too" — bearing: constrains
- 247fdfe8 (2026-09-10, commit): "Changed the emitter rather than the conventions: one emitter versus the template, its v0009 migration item, WORKFLOW.md, and AGENTS.md - the emitter was the single outlier" — bearing: constrains
- 5a0d1588 (2026-07-25, commit): "The fix makes the existing nil-vs-empty-vs-null distinction, which stringList(...) already preserved" — bearing: supports
- dcaa0366 (2026-05-04, commit): "Ticket detection: staged ticket moves and added ### Result headings are detected from cached git diffs and returned as ticket_changes" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/mcp/server.go; agents-plugin-tool/internal/wsgit/git.go; affected MCP and client tests |
| scope.surface | public-interface | existing MCP git.commit updated_tickets omission/empty behavior; existing Client.Commit integration, no new wire argument approved |
| scope.new_public_symbol | yes | decision 1065 adds a JSON-excluded field to the existing exported internal-package CommitOptions; no MCP symbol is added |
| scope.new_type_contract | yes | internal optional suppression boolean with zero preserving direct Go/CLI inference; no wire/shared-data contract change |
| scope.test_surface | existing | agents-plugin-tool/internal/wsgit/git_test.go; agents-plugin-tool/internal/mcp/server_test.go; agents-plugin-tool/internal/mcp/tickets_verify_test.go |
| complexity.reuse_points | confirmed | stringList at agents-plugin-tool/internal/mcp/server.go#L6050-L6063; normalizeCommitOptions, Commit and CommitMessage at agents-plugin-tool/internal/wsgit/git.go#L623-L721, #L749-L755 and #L998-L1044 |
| complexity.side_effect_risk | moderate | Commit stages paths and writes Git history; only the inference gate may change, not verification or detection |
| risk.correctness | moderate | nil/empty intent must survive normalization while TicketChanges remains populated independently of message summaries |
| risk.fit | moderate | representation choice must fit existing MCP conversion and direct client/CLI callers without broad helper or public-contract changes |
| risk.test | moderate | existing inference and verification tests need actual JSON omitted/empty/nonempty coverage plus normalization and metadata assertions |
| risk.security_or_contract | moderate | existing optional-argument promise changes observable message output; preserve validation, branch/path guards and compatibility on omission/nonempty calls |

## Phases

### Phase 1: Honor explicit empty summaries without changing omission

After the prerequisite lands, preserve the distinction between an omitted
argument and an explicitly supplied empty array through the existing dispatch,
normalization and commit-message path. Use it only to control inferred ticket
message summaries. Keep nonempty explicit summaries and all other commit
safeguards and metadata unchanged. Implement the internal suppression flag
settled in decision (1065), leaving direct callers' zero-value behavior intact.

Regression verification: exercise actual MCP JSON calls for a ticket-changing
commit with (a) the argument omitted, (b) `updated_tickets: []`, and (c) a
nonempty explicit list. Assert respectively inferred move/Result/Edition
summaries, no inferred `## Ticket Updates`, and only the supplied summaries.
Cover the normalization boundary, direct Go nil and allocated-empty slices
with the zero-value flag, and the no-ticket-change case; assert retained
AI Context, ticket validation, `TicketChanges` metadata and advisories. Inspect
existing caller entry points for unintended changes without defining new
null, wrong-type or CLI behavior. Run affected `internal/mcp` and
`internal/wsgit` tests and the project Go suite
(`cd agents-plugin-tool && go test ./...`) using local temporary repositories.
This drafting pass records no implementation or runtime-test result.

### Result (e9793acc) - 2026-11-15

- VERIFIED: Implemented decision (1065) in `645c2756`, with MCP boundary
  regressions in `e9793acc`. MCP dispatch sets the JSON-excluded internal
  `SuppressInferredTicketUpdates` boolean only for an existing converted
  non-nil, zero-length list (`internal/mcp/server.go:1671-1680`). Normalization
  preserves it, and only the inference gate consumes it
  (`internal/wsgit/git.go:571-574,717,772-779`). Nonempty explicit summaries,
  detection, validation, advisories and result metadata remain independent.
- VERIFIED: Actual MCP JSON omitted/empty/nonempty calls cover ticket changes
  and no-ticket changes, literal move/Result/Edition summaries, exact
  `TicketChanges`, stored hash/title and AI Context
  (`internal/mcp/server_test.go:2313-2408`). Explicit empty calls retain the
  invalid-ticket veto and advisories (`tickets_verify_test.go:14-62`;
  `server_test.go:2470-2538`). Client tests cover normalization, JSON exclusion,
  direct nil/allocated-empty slices and authoritative nonempty summaries
  (`internal/wsgit/git_test.go:294-373`). Paths here are under
  `agents-plugin-tool/`.
- VERIFIED: Focused offline commands passed; full output was read:
  - `cd agents-plugin-tool && GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test ./internal/wsgit -run 'Test(Commit|NormalizeCommitOptions|DetectTicketChanges|StagingCommandsForCommit|ParseTicket)' -count=1 -v`
    — 14.254s; `/tmp/261115-empty-ticket-updates-wsgit-green.log`.
  - `cd agents-plugin-tool && GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test ./internal/mcp -run "^(TestServeStdioGitToolCalls|TestServeStdioGitCommit.*|TestTicketsVerifyGitCommit.*|TestGitCommit.*)$" -count=1 -v`
    — 12.801s; `/tmp/empty-ticket-mcp-leaf-attempt1.log`, exact command also
    retained in `/tmp/empty-ticket-mcp-leaf-attempt1.command`.
  - The first client command failed only because a new test expected an
    abbreviated Result summary rather than the existing full heading. Blame:
    test expectation, not implementation. Corrected the expectation without
    changing the formatter; original full output is retained in
    `/tmp/261115-empty-ticket-updates-wsgit.log`. No second same-root failure.
- VERIFIED: Independent one-pass Lite review of
  `76d8f797..e9793acc` returned clean, with no remaining findings. Artifact:
  `/Users/kang-sw/.cache/ws@kang-sw-devenv/proj/17da6bdc@a93cfe66/review-paths/6e9f0e7f-01-261115-empty-ticket-updates-lite.md`.
- INFERRED: Final compatibility is T0 bug fix. Old omission/nonempty calls
  still use the unchanged message writer; the new flag never crosses JSON.
  Both versions write ordinary Git history and the same result shape
  (`internal/wsgit/git.go:739-747,1024-1042`), so old readers and a new reopen
  need no reconstruction. Explicit-empty suppression on unfixed old servers
  is not promised. Direct CLI construction leaves the flag at zero
  (`cmd/ws-mcp/hosted_cli.go:365-387`); null/wrong-type behavior, trailer
  ordering and wire schema are untouched.
- Decisions: No scope deviation. MAIN's binding run constraint replaces the
  older full-suite wording with focused `-run` verification only. No full
  suite, network, shared-runtime operation, push or merge was performed.
