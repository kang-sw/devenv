---
title: "tickets.move refuses a worker in its own assigned implementation checkout"
sage-review-design: skipped
sage-review-completeness: skipped
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

- **T0 confirmed:** only the worker identity predicate changes. Old and new
  callers retain the same arguments and ticket/index formats. `resolveToolRoot`
  (`internal/mcp/server.go:4138-4147`) reads the key's bound root; both versions
  use the unchanged `wsdoc.TicketsMove` and `TicketsClose` writers
  (`internal/wsdoc/tickets_mutate.go:88-177,180-260`). Reopening either version
  reads the same ticket files; no data migration or recovery is needed.
- Accept parentless delegate keys as workers alongside existing parented keys.
  `AgentKeyStore.Mint` (`internal/mcp/agent_keys.go:12-29`) deliberately mints
  those keys; requiring a parent rejects daemon-spawned Pi workers. Do not add
  branch leases or change agent keys to lead scope. Parentless leads remain
  subject to both occupied-root checks. `tickets.close` already allows these
  worker keys and needs regression coverage, not a new guard.
- Sage design/completeness stages are skipped for this directly assigned,
  bounded predicate bug fix; verification is the affected-package regression
  suite. No Sage review was run or claimed.

## Constraints

- `agents-plugin-tool/`: read and follow `ai-docs/manuals/shipped-surface-boundary.md`.
- `agents-plugin-tool/internal/mcp/`: read and follow `ai-docs/manuals/ws-mcp.md`.
- `agents-plugin-tool/internal/`: read and follow `ai-docs/manuals/release-compatibility.md`.
- No local Go, ws executable, launcher, or test-binary execution. Verify the
  affected MCP package remotely on w5090 through the sys_lock controller.

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

### Result (80bac3591) - 2026-11-16

- Fixed `tickets.move`'s `checkRoot` worker predicate: a parentless delegate
  key now receives the same session-bound exemption as a parented worker.
  Parentless lead callers still run the initial and writer-boundary guards,
  before ownership refresh or posture writes. Scope, ownership, review and
  dependency gates are unchanged; `tickets.close` needed no production change.
- Regression coverage includes parentless delegates on rooted/rootless impl
  branches, promotion/demotion staging, and actual `AgentKeyStore.Mint` keys
  moving/closing their own ticket through a server rooted in another worker's
  checkout. Only the bound ticket's paths are staged; the foreign checkout and
  Git admin bytes remain unchanged. Existing foreign-lead refusal and
  inspection-failure tests pass unchanged.
- Verification (w5090):
  `bash D:/sys_lock.sh remote w5090 run exec5 20 --sync 'D:/Workspace/Repos/.ws-worktrees/devenv/manifesto-charm-obtrusive' -- 'export PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && go test ./internal/mcp/...'`
  **PASS**, `github.com/kang-sw/devenv/internal/mcp`, 196.621s, command exit 0.
  No local Go, launcher, ws executable, or test binary was run.
- Dogfood limitation: this worker profile exposes neither `tickets.query` nor
  `tickets.move`/`tickets.close`. Located this ticket by file lookup and promoted
  it with `git mv`; no live tool refusal was observed or claimed. The previous
  diagnostic's occupied-root error path is identified in source, and the
  parentless-worker identity is reproduced in the MCP regression tests.
- The first commit attempt was refused by ticket verification for unset Sage
  postures and a Result heading without a hash. Recorded explicit skipped
  postures and split implementation from this phase record so the heading cites
  the actual implementation commit, rather than a fabricated self-reference.
- No Forward items; the one affected-package check passed.
