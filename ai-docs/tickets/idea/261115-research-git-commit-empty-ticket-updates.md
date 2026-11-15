---
title: "Investigate empty ticket-update overrides in git.commit"
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

### Confirmed Decisions

None.

### Proposals

- Investigate whether dispatch distinguishes an omitted argument from an
  explicit empty array, and whether documentation or behavior should change.

### Open Questions

- Should callers be able to suppress detected ticket updates, or should the
  tool document that empty arrays still enable detection?
- Should caller-supplied co-author trailers always remain at the message end?

### Rejected Alternatives

None.
