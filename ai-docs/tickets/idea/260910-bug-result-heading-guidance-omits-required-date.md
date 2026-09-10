---
title: "Align Result heading guidance with the date required by ticket verification"
---

# Align Result heading guidance with the date required by ticket verification

## Background

While closing `260910-chore-session-children-scope-unnoted-filters`, following
both the rendered ticket-worker prompt and `convention.read`'s
`ticket-conventions` example produced `### Result (00f8f976)`.
`tickets.verify` rejected it with `phase-result-heading`, requiring
`### Result (<hash>) - YYYY-MM-DD`. `git.commit` enforces the same rejection.
The close was corrected to the dated form before its commit.

## Phases

### Phase 1: Align authoring guidance with the existing guard

Find the Result-heading instructions in worker playbooks and bundled ticket
conventions, and make their examples include the date the validator already
requires. Preserve validator semantics and regenerate affected manifests and
mirrors. Verify that a heading copied from the shipped guidance passes the
existing ticket guard.
