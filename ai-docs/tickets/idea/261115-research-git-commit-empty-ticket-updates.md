---
title: "git.commit의 빈 ticket-update 오버라이드 동작 조사"
related:
  261115-feat-ws-dashboard-explorer-path-bar: "dogfood observation during closure"
---

# Investigate empty ticket-update overrides in git.commit

## Background

The path-bar worker was instructed to end commit messages with `## AI Context`
and a co-author trailer. It supplied `updated_tickets: []` to `ws/git.commit`
when committing the ticket's Result and close move. The tool still appended
`## Ticket Updates` after the supplied trailer, requiring a message-only
correction. The tool description describes automatic detection when the
argument is omitted; an explicit empty array reasonably suggests suppression.

## Outcome Ledger

### Verified Findings

- The closure call supplied `updated_tickets: []`, but the resulting commit
  body appended a detected ready-to-done move and Result under
  `## Ticket Updates`. The implementation-only commit with the same argument
  did not append that section because it changed no ticket files.
- The unexpected section followed `Co-Authored-By`, so the returned commit
  hash alone did not prove compliance with the caller's message-format rule.
- VERIFIED by static inspection: the optional argument's description says
  inference occurs when omitted
  (`agents-plugin-tool/internal/mcp/server.go:4867`). Dispatch passes the
  converted list (`server.go:1674`); stringList already distinguishes omitted
  nil from an allocated JSON empty array (`server.go:6050-6063`). Normalization
  collapses that distinction through trimStrings, and inference tests length
  (`agents-plugin-tool/internal/wsgit/git.go:701-703,749-755,1035-1044`).
  This establishes normalization/inference as the loss boundary, not dispatch
  conversion or a new reproduction of the
  historical worker call/result.
- VERIFIED: `CommitResult.TicketChanges` is distinct from the generated message
  section (`agents-plugin-tool/internal/wsgit/git.go:596-620,700-721`).
- VERIFIED: research categories are refused before writes on ready promotion
  (`agents-plugin-tool/internal/wsdoc/tickets_mutate.go:196-208`), consistent
  with ticket conventions' Research Tickets and Status Flow sections.

### Confirmed Decisions

- Owner-approved FIX 6 scope (triage 1060, relayed 2026-11-15): match the
  existing `git.commit` tool description only. Explicit `updated_tickets: []`
  suppresses inferred message summaries; omission retains inference. Preserve
  nonempty explicit summaries, ticket validation/detection metadata/advisories
  and the rest of the commit record. No broader
  protocol/API semantic or trailer-ordering redesign is approved.
- This remains a non-executable research record with no phases; it cannot
  move to `ready/`. Report the smallest actionable bug-ticket derivation to
  the lead, without creating, renaming, dropping or moving tickets now.
- `261115-bug-ws-git-commit-cannot-conclude-merge` must land before that
  derived implementation. The lead owns derivation, fact population and
  readiness gates.

### Proposals

- Historical proposal: investigate whether dispatch distinguishes an omitted
  argument from an explicit empty array, and whether documentation or behavior
  should change. The later Confirmed Decisions settle the narrow behavior;
  this proposal is not authority for a broader redesign.

### Open Questions

- Historical question, now answered by the owner-confirmed matching-description
  scope: should callers be able to suppress detected ticket updates, or should
  the tool document that empty arrays still enable detection?
- Should caller-supplied co-author trailers always remain at the message end?
  This remains outside the approved fix; do not infer a trailer policy from
  the observation.
- How should supplied-versus-omitted state survive dispatch and normalization
  internally? No representation is selected. Retain existing validation and
  caller behavior outside explicit empty arrays.

### Rejected Alternatives

- For FIX 6, changing the description to make empty arrays trigger inference
  does not satisfy the confirmed matching-description scope.
- A broader protocol/API or co-author-trailer ordering change is not approved.

## Decisions

- The contract is solely the Outcome Ledger's Confirmed Decisions above;
  narrative and historical proposals do not enlarge it.
- INFERRED compatibility target for the eventual derivation: **T0 / bug fix**,
  under `ai-docs/manuals/release-compatibility.md`. The existing optional
  argument already says inference occurs when omitted
  (`agents-plugin-tool/internal/mcp/server.go:4867`). Current dispatch passes
  `stringList(arguments["updated_tickets"])`, which already preserves omitted
  nil versus an allocated empty array (`server.go:1674,6050-6063`). Normalization
  collapses that distinction, and `Commit` infers whenever its length is zero
  (`agents-plugin-tool/internal/wsgit/git.go:701-703,749-755,1035-1044`).
  The new handler must preserve that omission path and existing nonempty
  summaries, changing only explicit-empty message inference. Both versions
  write ordinary readable Git commit text (`wsgit/git.go:707-721,998-1018`);
  an old reader/new reopen can still read it with no recovery required.
  A new caller sending `[]` to an old server still encounters the existing
  inference bug: this draft does not promise suppression on unfixed binaries.
  No new argument, hello/schema/version refusal or historical rewrite is
  authorized. Any need for such a boundary blocks this T0 scope.

## Constraints for actionable derivation

- Smallest contract: one bug ticket, one bounded phase carrying supplied-versus-
  omitted intent through the current dispatch/client boundary and fixing only
  inferred message summaries. Proposed title: "Honor explicit empty ticket
  updates in git.commit"; no ticket or new stem is allocated by this draft.
- The derived ticket should carry
  `blocked-by: 261115-bug-ws-git-commit-cannot-conclude-merge` and a `related:`
  link back to this research. Implement only after that prerequisite lands.
- Regression boundary: ticket-changing commits with the argument omitted
  still infer moves and Result/Edition summaries; explicit `[]` emits no
  inferred `## Ticket Updates`; a nonempty list emits only its explicit
  summaries. Cover actual MCP JSON presence, normalization, no-ticket changes,
  retained AI Context, unchanged ticket detection/verification and existing
  caller entry points. Do not define null/wrong-type semantics or a new CLI
  contract as part of this fix; inspect those callers for unintended changes.
- Applicable implementation conventions: read
  `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/ws-mcp.md` and
  `ai-docs/manuals/release-compatibility.md`. Plan verification with affected
  `internal/mcp` and `internal/wsgit` tests and
  `cd agents-plugin-tool && go test ./...`; this preparation ran none.
- No executable phases, new ticket, source implementation, status move,
  review stamp or commit are authorized by this drafting assignment.
