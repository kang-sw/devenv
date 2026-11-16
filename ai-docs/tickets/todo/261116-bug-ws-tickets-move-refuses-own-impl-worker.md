---
title: "tickets.move refuses a worker in its own assigned implementation checkout"
related:
  261030-bug-tickets-move-stages-on-occupied-root: "landed protection against another caller staging into an occupied worker index; preserve that boundary"
  261116-bug-ws-dashboard-composer-autosize-forced-layout: "today's worker promotion recorded the refusal and git mv fallback"
---

# tickets.move refuses a worker in its own assigned implementation checkout

## Background

Dogfood #18, 2026-11-16: `tickets.move` refused an occupied checkout even though
the caller was the worker assigned to that implementation branch. The Pi worker
used `git mv` to promote its ticket. The composer ticket's Result records this
at `ai-docs/tickets/ready/261116-bug-ws-dashboard-composer-autosize-forced-layout.md:115-116`.
This capture did not independently reproduce the refusal; recover the exact
arguments, diagnostic and caller/branch identity before assigning a root cause.

Expected behavior: a worker can move tickets in its own assigned impl checkout.
This must not undo the predecessor's protection against an unrelated lead
staging into another worker's index. A blanket impl-branch refusal is not an
adequate distinction between those cases.

## Decisions

Compatibility target: **T0**, restoring permitted worker calls with unchanged
MCP arguments, ticket files and index formats. No new mandatory identity proof,
wire argument or blanket guard removal is selected here.

## Phases

### Phase 1: Distinguish self-worker ticket moves from unsafe occupied-root writes

Reproduce the reported promotion with pinned caller/key, bound root, parent/role
and actual branch facts, then compare an unrelated lead in the same occupied
checkout. Which identity predicate rejected the legitimate worker? Correct that
path while retaining scope, ownership, review and dependency gates. Verify the
worker's own move succeeds and stages only its ticket; unrelated-caller refusal
must leave frontmatter, filesystem and index untouched. Treat git mv as incident
recovery evidence, not a reason to bypass the tool's safety boundary. No move,
guard change or implementation is performed by this docs-only capture.
